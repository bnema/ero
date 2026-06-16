package main

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	codexadapter "ero/internal/adapters/out/codex"
	"ero/pkg/plugin"
)

func TestInitialize(t *testing.T) {
	provider := codexProvider{}

	result, err := provider.Initialize(context.Background(), plugin.InitializeRequest{Protocol: plugin.ProtocolVersion, ContributionID: providerID})

	require.NoError(t, err)
	assert.Equal(t, providerID, result.Provider.ID)
	assert.Equal(t, "Codex", result.Provider.Label)
	assert.Equal(t, providerName, result.Provider.Name)
	assert.True(t, result.Provider.Capabilities.PublishReview)
	assert.False(t, result.Provider.Capabilities.LoadRemoteComments)
	assert.False(t, result.Provider.Capabilities.LoadRemoteSnapshot)
	assert.Contains(t, result.Provider.Capabilities.Decisions, plugin.ReviewDecisionComment)
	assert.Contains(t, result.Provider.Capabilities.Decisions, plugin.ReviewDecisionRequestChanges)
	assert.Contains(t, result.Provider.Capabilities.Decisions, plugin.ReviewDecisionApprove)
}

func TestDetectContext_Unavailable_MissingSocketPath(t *testing.T) {
	t.Setenv(codexadapter.EnvCodexThreadID, "thr_test")

	provider := codexProvider{}
	result, err := provider.DetectContext(context.Background(), plugin.DetectContextRequest{})

	require.NoError(t, err)
	assert.False(t, result.Result.Applicable)
	assert.Equal(t, codexCallbackHint, result.Result.Reason)
}

func TestDetectContext_Unavailable_MissingThreadID(t *testing.T) {
	t.Setenv(codexadapter.EnvCodexSocketPath, "/tmp/test.sock")

	provider := codexProvider{}
	result, err := provider.DetectContext(context.Background(), plugin.DetectContextRequest{})

	require.NoError(t, err)
	assert.False(t, result.Result.Applicable)
	assert.Equal(t, codexCallbackHint, result.Result.Reason)
}

func TestDetectContext_Unavailable_SocketNotReachable(t *testing.T) {
	t.Setenv(codexadapter.EnvCodexSocketPath, "/tmp/ero-test-nonexistent.sock")
	t.Setenv(codexadapter.EnvCodexThreadID, "thr_test")

	provider := codexProvider{}
	result, err := provider.DetectContext(context.Background(), plugin.DetectContextRequest{})

	require.NoError(t, err)
	assert.False(t, result.Result.Applicable)
	assert.Contains(t, result.Result.Reason, "/tmp/ero-test-nonexistent.sock")
}

func TestDetectContext_Applicable_WhenSocketReachable(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "codex.sock")
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	t.Setenv(codexadapter.EnvCodexSocketPath, socketPath)
	t.Setenv(codexadapter.EnvCodexThreadID, "thr_test")

	provider := codexProvider{}
	result, err := provider.DetectContext(context.Background(), plugin.DetectContextRequest{})

	require.NoError(t, err)
	assert.True(t, result.Result.Applicable)
	assert.Contains(t, result.Result.Reason, socketPath)
	assert.Contains(t, result.Result.Reason, "thr_test")
}

func TestPublishReview_EmptyDraft_DoesNotCallPublish(t *testing.T) {
	called := false
	provider := codexProvider{publish: func(context.Context, codexadapter.Config, string) (*codexadapter.PublishResult, error) {
		called = true
		return nil, nil
	}}

	result, err := provider.PublishReview(context.Background(), plugin.PublishReviewParams{Payload: plugin.ReviewPublishPayload{ProviderID: providerID}})

	require.NoError(t, err)
	assert.False(t, called)
	assert.Equal(t, providerID, result.Result.ProviderID)
	assert.Empty(t, result.Result.PublishedRefs)
}

func TestPublishReview_Success_MapsCodexRefs(t *testing.T) {
	provider := codexProvider{publish: func(_ context.Context, _ codexadapter.Config, formatted string) (*codexadapter.PublishResult, error) {
		assert.Contains(t, formatted, "Review")
		assert.Contains(t, formatted, "hello")
		return &codexadapter.PublishResult{ThreadID: "thr_live_1", TurnID: "turn_42"}, nil
	}}

	result, err := provider.PublishReview(context.Background(), plugin.PublishReviewParams{Payload: plugin.ReviewPublishPayload{
		ProviderID: providerID,
		Context:    plugin.ReviewContext{Repository: plugin.RepositoryMetadata{WorktreeRoot: "/repo"}},
		Draft: plugin.ReviewDraftSnapshot{
			Summary:  "hello",
			Comments: []plugin.ReviewComment{{ID: "c1", FilePath: "main.go", Body: "fix it", Range: plugin.ReviewLineRange{Start: plugin.ReviewLineRef{NewLineNumber: 7}}}},
		},
	}})

	require.NoError(t, err)
	assert.Equal(t, providerID, result.Result.ProviderID)
	assert.Equal(t, "codex:thread:thr_live_1", result.Result.ExternalReviewID)
	require.Len(t, result.Result.PublishedRefs, 1)
	assert.Equal(t, "c1", result.Result.PublishedRefs[0].LocalCommentID)
	assert.Equal(t, "codex:turn:thr_live_1:turn_42:0", result.Result.PublishedRefs[0].ExternalID)
}

func TestPublishReview_ClassifiesPartialPublishAsUnknown(t *testing.T) {
	provider := codexProvider{publish: func(context.Context, codexadapter.Config, string) (*codexadapter.PublishResult, error) {
		return &codexadapter.PublishResult{ThreadID: "thr_live_1"}, &codexadapter.PublishReviewError{Reason: codexadapter.PublishErrorPublish, Message: "codex: publish review to thread thr_live_1 failed: EOF", Cause: errors.New("EOF")}
	}}

	_, err := provider.PublishReview(context.Background(), plugin.PublishReviewParams{Payload: plugin.ReviewPublishPayload{
		ProviderID: providerID,
		Context:    plugin.ReviewContext{Repository: plugin.RepositoryMetadata{WorktreeRoot: "/repo"}},
		Draft:      plugin.ReviewDraftSnapshot{Summary: "hello"},
	}})

	require.Error(t, err)
	pe := plugin.AsError(err)
	require.NotNil(t, pe)
	assert.Equal(t, plugin.ErrorPartialPublishUnknown, pe.Code)
}

func TestClassifyCallbackErrorCode(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{name: "auth", message: "authentication required: login again", want: plugin.ErrorAuthRequired},
		{name: "rate limit", message: "rate limit exceeded", want: plugin.ErrorRemoteRateLimited},
		{name: "network dial", message: "dial unix /tmp/c.sock: connection refused", want: plugin.ErrorNetwork},
		{name: "network timeout", message: "deadline exceeded", want: plugin.ErrorNetwork},
		{name: "validation", message: "invalid response payload", want: plugin.ErrorRemoteValidationFailed},
		{name: "validation decode", message: "decode error: unexpected field", want: plugin.ErrorRemoteValidationFailed},
		{name: "fallback", message: "something else", want: plugin.ErrorInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, classifyCallbackErrorCode(tt.message))
		})
	}
}
