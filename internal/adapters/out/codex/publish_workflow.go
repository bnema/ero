package codex

import (
	"context"
	"fmt"
	"strings"
)

// PublishResult captures the outcome of a Codex publish operation.
type PublishResult struct {
	// ThreadID is the stable Codex thread identifier where the review was published.
	ThreadID string

	// TurnID is the stable turn identifier from the turn/start response.
	TurnID string
}

// publishErrorReason classifies the failure for better error messages.
type publishErrorReason string

const (
	publishErrStartup     publishErrorReason = "startup"
	publishErrInitialize  publishErrorReason = "initialize"
	publishErrListing     publishErrorReason = "listing"
	publishErrOverride    publishErrorReason = "override"
	publishErrIO          publishErrorReason = "io"
	publishErrAmbiguous   publishErrorReason = "ambiguous"
	publishErrResume      publishErrorReason = "resume"
	publishErrCreate      publishErrorReason = "create"
	publishErrPublish     publishErrorReason = "publish"
	publishErrUnsupported publishErrorReason = "unsupported"
)

// PublishReviewError is a structured error returned by PublishReview.
type PublishReviewError struct {
	Reason  publishErrorReason
	Message string
	Cause   error
}

func (e *PublishReviewError) Error() string {
	return e.Message
}

func (e *PublishReviewError) Unwrap() error {
	return e.Cause
}

// PublishReview runs the full Codex publish workflow:
//  1. Connect to the app-server (live session preferred when reachable)
//  2. Initialize JSON-RPC handshake
//  3. List loaded threads (enriched with CWD for accurate auto-select)
//  4. Run thread selection (explicit override, CWD match, or create new)
//  5. Resume or start the selected thread
//  6. Send the review as a user message via turn/start
//  7. Clean up the subprocess
//
// Returns a PublishResult with ThreadID and TurnID when successful.
// Ambiguous matches and I/O errors are returned as actionable errors.
func PublishReview(ctx context.Context, cfg Config, cwd, formattedMessage string) (*PublishResult, error) {
	// 1. Connect to the app-server (live-session preferred when reachable).
	client, err := NewAppServerClient(ctx, cfg)
	if err != nil {
		return nil, &PublishReviewError{
			Reason:  publishErrStartup,
			Message: fmt.Sprintf("codex: start app-server: %s", err),
			Cause:   err,
		}
	}
	defer client.Close()

	// 2. Initialize handshake.
	if err := client.Initialize(ctx); err != nil {
		return nil, &PublishReviewError{
			Reason:  publishErrInitialize,
			Message: fmt.Sprintf("codex: initialize app-server: %s", err),
			Cause:   err,
		}
	}

	// 3. List loaded threads (enriched with CWD via readThread).
	loaded, err := client.ListLoadedThreads(ctx)
	if err != nil {
		return nil, &PublishReviewError{
			Reason:  publishErrListing,
			Message: fmt.Sprintf("codex: list loaded threads: %s", err),
			Cause:   err,
		}
	}

	// 4. Build selection criteria and run selection.
	criteria := ThreadSelectionCriteria{
		CWD: cwd,
	}
	if cfg.ThreadID != "" || cfg.SessionKey != "" {
		criteria.Explicit = &ExplicitOverride{
			ThreadID:   cfg.ThreadID,
			SessionKey: cfg.SessionKey,
		}
	}

	selection := SelectThread(ctx, criteria, loaded, client)
	switch selection.Decision {
	case ThreadDecisionInvalidOverride:
		if cfg.SessionKey != "" && cfg.ThreadID == "" {
			return nil, &PublishReviewError{
				Reason:  publishErrOverride,
				Message: fmt.Sprintf("codex: session key override %q not found: %s", cfg.SessionKey, selection.Reason),
			}
		}
		return nil, &PublishReviewError{
			Reason:  publishErrOverride,
			Message: fmt.Sprintf("codex: thread override %q not found: %s", cfg.ThreadID, selection.Reason),
		}

	case ThreadDecisionIOError:
		return nil, &PublishReviewError{
			Reason:  publishErrIO,
			Message: fmt.Sprintf("codex: cannot list stored threads: %s; set ERO_CODEX_THREAD_ID to target a specific thread or retry", selection.Reason),
		}

	case ThreadDecisionAmbiguous:
		ids := make([]string, 0, len(selection.Matches))
		for _, m := range selection.Matches {
			ids = append(ids, m.ID)
		}
		return nil, &PublishReviewError{
			Reason: publishErrAmbiguous,
			Message: fmt.Sprintf(
				"codex: multiple threads match this workspace: %s; "+
					"set ERO_CODEX_THREAD_ID to one of these IDs to disambiguate",
				strings.Join(ids, ", "),
			),
		}
	}

	// 5. Resume or start the selected thread.
	var threadID string
	switch selection.Decision {
	case ThreadDecisionResume:
		if selection.Candidate == nil {
			return nil, &PublishReviewError{
				Reason:  publishErrResume,
				Message: "codex: resume decision with nil candidate",
			}
		}
		threadID = selection.Candidate.ID
		if err := client.ResumeThread(ctx, threadID); err != nil {
			return nil, &PublishReviewError{
				Reason:  publishErrResume,
				Message: fmt.Sprintf("codex: resume thread %s: %s", threadID, err),
				Cause:   err,
			}
		}
	case ThreadDecisionCreateNew:
		id, err := client.StartThread(ctx, cwd)
		if err != nil {
			return nil, &PublishReviewError{
				Reason:  publishErrCreate,
				Message: fmt.Sprintf("codex: start thread: %s", err),
				Cause:   err,
			}
		}
		threadID = id
	default:
		return nil, &PublishReviewError{
			Reason:  publishErrUnsupported,
			Message: fmt.Sprintf("codex: unexpected thread selection decision %q", selection.Decision),
		}
	}

	// 6. Publish the review message and capture the turn ID.
	turnID, err := client.PublishMessage(ctx, threadID, formattedMessage)
	if err != nil {
		return &PublishResult{ThreadID: threadID},
			&PublishReviewError{
				Reason:  publishErrPublish,
				Message: fmt.Sprintf("codex: publish review to thread %s failed: %s", threadID, err),
				Cause:   err,
			}
	}

	return &PublishResult{ThreadID: threadID, TurnID: turnID}, nil
}
