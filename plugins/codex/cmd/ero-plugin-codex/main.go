package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	codexadapter "ero/internal/adapters/out/codex"
	"ero/pkg/plugin"
)

const (
	providerID       = "codex"
	providerName     = "ero-plugin-codex"
	codexCallbackHint = "Codex callback target not configured: set ERO_CODEX_SOCKET_PATH and ERO_CODEX_THREAD_ID to target a specific Codex session"
)

type publishFunc func(ctx context.Context, cfg codexadapter.Config, cwd, formatted string) (*codexadapter.PublishResult, error)

type codexProvider struct {
	publish publishFunc
}

func main() {
	provider := codexProvider{publish: publishCallback}
	if err := plugin.ServeReviewProvider(context.Background(), provider, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (p codexProvider) Initialize(_ context.Context, req plugin.InitializeRequest) (plugin.InitializeResult, error) {
	if req.Protocol != plugin.ProtocolVersion {
		return plugin.InitializeResult{}, plugin.NewErrorf(plugin.ErrorInvalidRequest, "unsupported protocol %q", req.Protocol)
	}
	if req.ContributionID != "" && req.ContributionID != providerID {
		return plugin.InitializeResult{}, plugin.NewErrorf(plugin.ErrorInvalidRequest, "unsupported contribution %q", req.ContributionID)
	}
	return plugin.InitializeResult{
		Protocol: plugin.ProtocolVersion,
		Provider: plugin.ReviewProviderInfo{
			ID:    providerID,
			Label: "Codex",
			Name:  providerName,
			Capabilities: plugin.ReviewProviderCapabilities{
				LoadRemoteComments: false,
				LoadRemoteSnapshot: false,
				PublishReview:      true,
				IdempotentPublish:  false,
				Decisions: []plugin.ReviewDecision{
					plugin.ReviewDecisionComment,
					plugin.ReviewDecisionRequestChanges,
					plugin.ReviewDecisionApprove,
				},
			},
		},
	}, nil
}

func (p codexProvider) DetectContext(_ context.Context, _ plugin.DetectContextRequest) (plugin.DetectContextResult, error) {
	cfg := codexadapter.ConfigFromEnv()

	// Require explicit callback target configuration.
	if err := cfg.ValidateCallbackTarget(); err != nil {
		return plugin.DetectContextResult{
			Result: plugin.DetectionResult{
				Applicable: false,
				Reason:     codexCallbackHint,
			},
		}, nil
	}

	// Lightweight socket existence check.
	if !codexadapter.SocketExists(cfg.SocketPath) {
		return plugin.DetectContextResult{
			Result: plugin.DetectionResult{
				Applicable: false,
				Reason:     fmt.Sprintf("Codex socket %s not found or not a socket", cfg.SocketPath),
			},
		}, nil
	}

	return plugin.DetectContextResult{
		Result: plugin.DetectionResult{
			Applicable: true,
			Reason:     fmt.Sprintf("Codex callback target configured: socket=%s thread=%s", cfg.SocketPath, cfg.ThreadID),
		},
	}, nil
}

func (p codexProvider) LoadRemoteThreads(_ context.Context, _ plugin.LoadRemoteThreadsRequest) (plugin.LoadRemoteThreadsResult, error) {
	return plugin.LoadRemoteThreadsResult{Threads: []plugin.RemoteReviewThread{}}, nil
}

func (p codexProvider) PublishReview(parentCtx context.Context, params plugin.PublishReviewParams) (plugin.PublishReviewResultData, error) {
	payload := params.Payload

	msg := codexadapter.BuildPublishMessage(
		payload.Draft.Summary,
		string(payload.Draft.Decision),
		extractPublishComments(payload.Draft.Comments),
	)
	if codexadapter.PublishMessageIsEmpty(msg) {
		return plugin.PublishReviewResultData{Result: plugin.ReviewPublishResult{ProviderID: providerID, PublishedRefs: []plugin.PublishedReviewCommentRef{}}}, nil
	}

	cfg := codexadapter.ConfigFromEnv()
	publishCtx, cancel := context.WithTimeout(parentCtx, cfg.EffectiveTimeout())
	defer cancel()

	publish := p.publish
	if publish == nil {
		publish = publishCallback
	}
	result, err := publish(publishCtx, cfg, payload.Context.Repository.WorktreeRoot, codexadapter.FormatPublishMessage(msg))
	if err != nil {
		return plugin.PublishReviewResultData{}, classifyCallbackPublishError(result, err)
	}

	refs := make([]plugin.PublishedReviewCommentRef, 0, len(payload.Draft.Comments))
	if result.ThreadID != "" {
		for i, c := range payload.Draft.Comments {
			refs = append(refs, plugin.PublishedReviewCommentRef{
				LocalCommentID: c.ID,
				ExternalID:     codexadapter.BuildExternalCommentID(result.ThreadID, result.TurnID, i),
			})
		}
	}
	return plugin.PublishReviewResultData{Result: plugin.ReviewPublishResult{
		ProviderID:       providerID,
		ExternalReviewID: codexadapter.BuildExternalReviewID(result.ThreadID),
		PublishedRefs:    refs,
	}}, nil
}

// publishCallback delegates to SendCallback for the callback-only publish
// workflow. The old PublishReview orchestration (CWD matching, thread
// selection, stored-thread pagination, create-new fallback) is removed.
func publishCallback(ctx context.Context, cfg codexadapter.Config, _ string, formatted string) (*codexadapter.PublishResult, error) {
	return codexadapter.SendCallback(ctx, cfg, formatted)
}

func classifyCallbackPublishError(result *codexadapter.PublishResult, err error) error {
	if err == nil {
		return nil
	}
	if pe := plugin.AsError(err); pe != nil {
		return pe
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "codex callback publish failed"
	}
	if publishErr, ok := errors.AsType[*codexadapter.PublishReviewError](err); ok {
		switch publishErr.Reason {
		case codexadapter.PublishErrorPublish:
			if result != nil && result.ThreadID != "" {
				return plugin.NewError(plugin.ErrorPartialPublishUnknown, message)
			}
		case codexadapter.PublishErrorUnsupported:
			return plugin.NewError(plugin.ErrorRemoteValidationFailed, message)
		}
	}
	return plugin.NewError(classifyCallbackErrorCode(message), message)
}

func classifyCallbackErrorCode(message string) string {
	lower := strings.ToLower(strings.TrimSpace(message))
	switch {
	case lower == "":
		return plugin.ErrorInternal
	case strings.Contains(lower, "rate limit") || strings.Contains(lower, "too many requests"):
		return plugin.ErrorRemoteRateLimited
	case strings.Contains(lower, "auth") || strings.Contains(lower, "login") || strings.Contains(lower, "credential") || strings.Contains(lower, "api key") || strings.Contains(lower, "401") || strings.Contains(lower, "403"):
		return plugin.ErrorAuthRequired
	case strings.Contains(lower, "dial ") || strings.Contains(lower, "connect ") || strings.Contains(lower, "socket") || strings.Contains(lower, "websocket") || strings.Contains(lower, "broken pipe") || strings.Contains(lower, "connection refused") || strings.Contains(lower, "deadline exceeded") || strings.Contains(lower, "timeout") || strings.Contains(lower, " eof") || strings.HasSuffix(lower, "eof"):
		return plugin.ErrorNetwork
	case strings.Contains(lower, "invalid") || strings.Contains(lower, "decode") || strings.Contains(lower, "marshal") || strings.Contains(lower, "protocol") || strings.Contains(lower, "unexpected"):
		return plugin.ErrorRemoteValidationFailed
	default:
		return plugin.ErrorInternal
	}
}

func extractPublishComments(draftComments []plugin.ReviewComment) []codexadapter.PublishComment {
	if len(draftComments) == 0 {
		return nil
	}
	result := make([]codexadapter.PublishComment, 0, len(draftComments))
	for _, dc := range draftComments {
		result = append(result, codexadapter.PublishComment{
			FilePath:     dc.FilePath,
			OldLineStart: dc.Range.Start.OldLineNumber,
			NewLineStart: dc.Range.Start.NewLineNumber,
			Body:         dc.Body,
		})
	}
	return result
}
