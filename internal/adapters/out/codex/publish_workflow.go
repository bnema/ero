package codex

import (
	"context"
	"fmt"
)

// PublishResult captures the outcome of a Codex publish operation.
type PublishResult struct {
	// ThreadID is the stable Codex thread identifier where the review was published.
	ThreadID string

	// TurnID is the stable turn identifier from the turn/start response.
	TurnID string
}

// PublishErrorReason classifies the failure for better error messages.
type PublishErrorReason string

const (
	PublishErrorStartup     PublishErrorReason = "startup"
	PublishErrorInitialize  PublishErrorReason = "initialize"
	PublishErrorPublish     PublishErrorReason = "publish"
	PublishErrorUnsupported PublishErrorReason = "unsupported"
)

// PublishReviewError is a structured error returned by the callback publish
// workflow. It wraps a reason, human-readable message, and optional cause.
type PublishReviewError struct {
	Reason  PublishErrorReason
	Message string
	Cause   error
}

func (e *PublishReviewError) Error() string {
	return e.Message
}

func (e *PublishReviewError) Unwrap() error {
	return e.Cause
}

// SendCallback is the callback-only publish workflow.
//
// It validates the explicit callback target from cfg (SocketPath and ThreadID
// must be set), dials the live session, initializes the JSON-RPC handshake,
// and sends the formatted review message as a user turn (turn/start) to the
// configured ThreadID on the running Codex app-server.
//
// Returns a PublishResult with the configured ThreadID and the server-assigned
// TurnID. Missing target configuration results in a structured
// PublishReviewError with reason PublishErrorUnsupported. Dial and handshake
// failures are returned as PublishErrorStartup / PublishErrorInitialize.
// Publish failures return PublishErrorPublish with a partial result carrying
// the ThreadID.
//
// SendCallback does NOT fall back to CWD matching, stored thread scan, thread
// list, or thread creation. It is the direct counterpart to the old
// PublishReview which handled selection and fallback.
func SendCallback(ctx context.Context, cfg Config, formattedMessage string) (*PublishResult, error) {
	// 1. Validate explicit callback target.
	if err := cfg.ValidateCallbackTarget(); err != nil {
		return nil, &PublishReviewError{
			Reason:  PublishErrorUnsupported,
			Message: fmt.Sprintf("codex: missing callback target: %s", err),
			Cause:   err,
		}
	}

	// 2. Dial the live session. NewAppServerClient validates callback target
	//    internally as well, but we check above for a dedicated error path.
	client, err := NewAppServerClient(ctx, cfg)
	if err != nil {
		return nil, &PublishReviewError{
			Reason:  PublishErrorStartup,
			Message: fmt.Sprintf("codex: connect to app-server at %s: %s", cfg.EffectiveSocketPath(), err),
			Cause:   err,
		}
	}
	defer func() {
		_ = client.Close()
	}()

	// 3. Initialize JSON-RPC handshake.
	if err := client.Initialize(ctx); err != nil {
		return nil, &PublishReviewError{
			Reason:  PublishErrorInitialize,
			Message: fmt.Sprintf("codex: initialize handshake: %s", err),
			Cause:   err,
		}
	}

	// 4. Publish the review as a user message on the configured thread.
	//    The server resumes the thread implicitly when processing a
	//    turn/start request for an existing thread ID.
	turnID, err := client.PublishMessage(ctx, cfg.ThreadID, formattedMessage)
	if err != nil {
		return &PublishResult{ThreadID: cfg.ThreadID},
			&PublishReviewError{
				Reason:  PublishErrorPublish,
				Message: fmt.Sprintf("codex: publish review to thread %s failed: %s", cfg.ThreadID, err),
				Cause:   err,
			}
	}

	return &PublishResult{ThreadID: cfg.ThreadID, TurnID: turnID}, nil
}
