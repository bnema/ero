package app

import (
	"context"
	"encoding/json"
	"fmt"

	"ero/internal/adapters/in/cli"
	codexadapter "ero/internal/adapters/out/codex"
	"ero/pkg/plugin/protocol"
)

// publishFunc is the signature used by handleCodexPublishReviewWith to
// perform the actual Codex publish. Production uses codexadapter.PublishReview;
// tests may provide a stub.
type publishFunc func(ctx context.Context, cfg codexadapter.Config, cwd, formatted string) (*codexadapter.PublishResult, error)

// NewBuiltinProviderRequestHandler returns the app-level dispatcher used by
// the CLI __provider protocol shell.
func NewBuiltinProviderRequestHandler() cli.ProviderRequestHandler {
	return dispatchBuiltinProviderRequest
}

func dispatchBuiltinProviderRequest(ctx context.Context, providerID string, req protocol.Request) protocol.Response {
	switch providerID {
	case "codex":
		return dispatchCodexRequest(ctx, req)
	default:
		return protocol.Response{
			Error: protocol.NewError(protocol.ErrorInvalidRequest,
				fmt.Sprintf("unknown builtin provider: %s", providerID)),
		}
	}
}

// dispatchCodexRequest routes a single JSON-lines request to the appropriate
// handler and returns a response. The ctx parameter is passed through to
// handlers that perform I/O so their lifetime is bounded.
func dispatchCodexRequest(ctx context.Context, req protocol.Request) protocol.Response {
	switch req.Method {
	case "initialize":
		return handleCodexInitialize(req.Params)
	case "detect_context":
		return handleCodexDetectContext()
	case "load_remote_threads":
		return handleCodexLoadRemoteThreads(req.Params)
	case "publish_review":
		return handleCodexPublishReview(ctx, req.Params)
	default:
		return protocol.Response{
			Error: protocol.NewError(protocol.ErrorUnsupportedCapability,
				fmt.Sprintf("method %q not implemented in phase 3", req.Method)),
		}
	}
}

// handleCodexInitialize handles the initialize request for the Codex builtin
// provider. It returns provider info with publish capability enabled.
func handleCodexInitialize(rawParams json.RawMessage) protocol.Response {
	var params protocol.InitializeRequest
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return protocol.Response{
			Error: protocol.NewError(protocol.ErrorInvalidRequest, "invalid initialize params"),
		}
	}
	if params.Protocol != protocol.ProtocolVersion {
		return protocol.Response{
			Error: protocol.NewError(protocol.ErrorInvalidRequest,
				fmt.Sprintf("unsupported protocol %q", params.Protocol)),
		}
	}
	return protocol.Response{
		Result: protocol.InitializeResult{
			Protocol: protocol.ProtocolVersion,
			Provider: protocol.ReviewProviderInfo{
				ID:    "codex",
				Label: "Codex",
				Name:  "Codex",
				Capabilities: protocol.ReviewProviderCapabilities{
					// Publish-only in v1: comment threads are posted into an
					// existing or new Codex thread via turn/start.
					LoadRemoteComments: false,
					LoadRemoteSnapshot: false,
					PublishReview:      true,
					IdempotentPublish:  false,
					// Codex publish formatting handles all three standard
					// decisions (comment, request_changes, approve).
					// Declaring them here prevents the host from stripping
					// the decision from the publish payload.
					Decisions: []protocol.ReviewDecision{
						protocol.ReviewDecisionComment,
						protocol.ReviewDecisionRequestChanges,
						protocol.ReviewDecisionApprove,
					},
				},
			},
		},
	}
}

// handleCodexDetectContext checks whether the codex binary is available on
// the system. This is a lightweight check — it does not start the app-server.
// When the binary cannot be found the provider is marked not applicable with
// an actionable message.
func handleCodexDetectContext() protocol.Response {
	cfg := codexadapter.ConfigFromEnv()
	if cfg.CodexAvailable() {
		return protocol.Response{
			Result: protocol.DetectContextResult{
				Result: protocol.DetectionResult{
					Applicable: true,
					Reason:     "Codex AI review: codex binary found",
				},
			},
		}
	}
	// The binary will be available when codex is installed; suggest how
	// to install it when the user expected it to work.
	return protocol.Response{
		Result: protocol.DetectContextResult{
			Result: protocol.DetectionResult{
				Applicable: false,
				Reason:     "Codex AI review requires the codex CLI to be installed and on $PATH (see https://codex.ai/install)",
			},
		},
	}
}

// handleCodexLoadRemoteThreads returns an empty thread list. In v1 the Codex
// provider is publish-only and does not load remote comments or snapshots.
func handleCodexLoadRemoteThreads(rawParams json.RawMessage) protocol.Response {
	// Validate params minimally to be a good protocol citizen.
	var params protocol.LoadRemoteThreadsRequest
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return protocol.Response{
			Error: protocol.NewError(protocol.ErrorInvalidRequest, "invalid load_remote_threads params"),
		}
	}
	_ = params // context available for future use
	return protocol.Response{
		Result: protocol.LoadRemoteThreadsResult{
			Threads: []protocol.RemoteReviewThread{},
		},
	}
}

// handleCodexPublishReview handles the publish_review request by connecting
// to the codex app-server, selecting or creating a thread, and delivering
// the review as a user message via turn/start.
//
// It derives a bounded per-request context from the parent ctx so that
// the publish workflow does not outlive its expected lifetime.
func handleCodexPublishReview(parentCtx context.Context, rawParams json.RawMessage) protocol.Response {
	return handleCodexPublishReviewWith(parentCtx, rawParams, codexadapter.PublishReview)
}

// handleCodexPublishReviewWith is like handleCodexPublishReview but accepts
// an explicit publish function for test injection.
func handleCodexPublishReviewWith(parentCtx context.Context, rawParams json.RawMessage, publish publishFunc) protocol.Response {
	var params protocol.PublishReviewParams
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return protocol.Response{
			Error: protocol.NewError(protocol.ErrorInvalidRequest, "invalid publish_review params"),
		}
	}

	payload := params.Payload

	// Extract the review context for thread selection.
	cwd := payload.Context.Repository.WorktreeRoot
	if cwd == "" {
		cwd = payload.Context.Repository.RepoPath
	}

	// Build the formatted message to publish.
	comments := extractPublishComments(payload.Draft.Comments)
	msg := codexadapter.BuildPublishMessage(
		payload.Draft.Summary,
		string(payload.Draft.Decision),
		comments,
	)

	if codexadapter.PublishMessageIsEmpty(msg) {
		return protocol.Response{
			Result: protocol.PublishReviewResultData{
				Result: protocol.ReviewPublishResult{
					ProviderID:    payload.ProviderID,
					PublishedRefs: []protocol.PublishedReviewCommentRef{},
				},
			},
		}
	}

	formatted := codexadapter.FormatPublishMessage(msg)

	// Build config from environment and publish via the adapter.
	cfg := codexadapter.ConfigFromEnv()

	// Bound the publish workflow lifetime: min(parent deadline, configured timeout).
	publishCtx, cancel := context.WithTimeout(parentCtx, cfg.EffectiveTimeout())
	defer cancel()

	result, err := publish(publishCtx, cfg, cwd, formatted)
	if err != nil {
		return protocol.Response{
			Error: protocol.NewError(protocol.ErrorInternal, err.Error()),
		}
	}

	// Build Ero protocol response from the Codex result.
	refs := make([]protocol.PublishedReviewCommentRef, 0)
	if result.ThreadID != "" {
		for i, c := range payload.Draft.Comments {
			refs = append(refs, protocol.PublishedReviewCommentRef{
				LocalCommentID: c.ID,
				ExternalID:     codexadapter.BuildExternalCommentID(result.ThreadID, result.TurnID, i),
			})
		}
	}

	return protocol.Response{
		Result: protocol.PublishReviewResultData{
			Result: protocol.ReviewPublishResult{
				ProviderID:       payload.ProviderID,
				ExternalReviewID: codexadapter.BuildExternalReviewID(result.ThreadID),
				PublishedRefs:    refs,
			},
		},
	}
}

// extractPublishComments converts draft comments into codex adapter publish comments.
func extractPublishComments(draftComments []protocol.ReviewComment) []codexadapter.PublishComment {
	if len(draftComments) == 0 {
		return nil
	}
	result := make([]codexadapter.PublishComment, 0, len(draftComments))
	for _, dc := range draftComments {
		c := codexadapter.PublishComment{
			FilePath:     dc.FilePath,
			OldLineStart: dc.Range.Start.OldLineNumber,
			NewLineStart: dc.Range.Start.NewLineNumber,
			Body:         dc.Body,
		}
		result = append(result, c)
	}
	return result
}
