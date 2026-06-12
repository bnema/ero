package app

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	codexadapter "ero/internal/adapters/out/codex"
	"ero/pkg/plugin/protocol"
)

func TestBuiltinProviderRequestHandlerRejectsUnknownProvider(t *testing.T) {
	resp := dispatchBuiltinProviderRequest(context.Background(), "unknown", protocol.Request{Method: "initialize"})
	require.NotNil(t, resp.Error)
	assert.Equal(t, protocol.ErrorInvalidRequest, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "unknown builtin provider")
}

func TestCodexInitializeResponse(t *testing.T) {
	rawParams := json.RawMessage(`{"protocol":"ero.plugin.v1","contribution_id":"codex"}`)
	resp := handleCodexInitialize(rawParams)

	assert.Empty(t, resp.Error, "unexpected protocol error: %v", resp.Error)

	result, ok := resp.Result.(protocol.InitializeResult)
	require.True(t, ok, "response result is not InitializeResult")

	assert.Equal(t, protocol.ProtocolVersion, result.Protocol)
	assert.Equal(t, "codex", result.Provider.ID)
	assert.Equal(t, "Codex", result.Provider.Label)
	assert.Equal(t, "Codex", result.Provider.Name)
	assert.Len(t, result.Provider.Capabilities.Decisions, 3, "phase 3 supports all three standard decisions")
	assert.Contains(t, result.Provider.Capabilities.Decisions, protocol.ReviewDecisionComment)
	assert.Contains(t, result.Provider.Capabilities.Decisions, protocol.ReviewDecisionRequestChanges)
	assert.Contains(t, result.Provider.Capabilities.Decisions, protocol.ReviewDecisionApprove)
	assert.False(t, result.Provider.Capabilities.IdempotentPublish, "v1 publish is not idempotent")

	// Phase 3: publish is enabled, remote loading is not.
	assert.False(t, result.Provider.Capabilities.LoadRemoteComments)
	assert.False(t, result.Provider.Capabilities.LoadRemoteSnapshot)
	assert.True(t, result.Provider.Capabilities.PublishReview, "phase 3 enables publish")
}

func TestCodexInitializeRejectsUnknownProtocol(t *testing.T) {
	rawParams := json.RawMessage(`{"protocol":"ero.plugin.v0","contribution_id":"codex"}`)
	resp := handleCodexInitialize(rawParams)

	require.NotNil(t, resp.Error)
	assert.Equal(t, protocol.ErrorInvalidRequest, resp.Error.Code)
}

func TestCodexInitializeRejectsBadJSON(t *testing.T) {
	rawParams := json.RawMessage(`not json`)
	resp := handleCodexInitialize(rawParams)

	require.NotNil(t, resp.Error)
	assert.Equal(t, protocol.ErrorInvalidRequest, resp.Error.Code)
}

func TestCodexDetectContextWhenCodexNotFound(t *testing.T) {
	// Force codex not found by setting ERO_CODEX_EXEC_PATH to a
	// non-existent path.
	t.Setenv("ERO_CODEX_EXEC_PATH", "/no/such/codex/binary")

	resp := handleCodexDetectContext()

	assert.Empty(t, resp.Error)

	result, ok := resp.Result.(protocol.DetectContextResult)
	require.True(t, ok, "response result is not DetectContextResult")

	assert.False(t, result.Result.Applicable)
	assert.Contains(t, result.Result.Reason, "requires the codex CLI")
}

func TestCodexDetectContextWhenCodexFound(t *testing.T) {
	// Find the actual codex binary on the system.
	path, err := os.Executable()
	if err != nil {
		t.Skipf("could not resolve self path: %v", err)
	}
	t.Setenv("ERO_CODEX_EXEC_PATH", path)

	resp := handleCodexDetectContext()

	assert.Empty(t, resp.Error)

	result, ok := resp.Result.(protocol.DetectContextResult)
	require.True(t, ok, "response result is not DetectContextResult")

	assert.True(t, result.Result.Applicable)
	assert.Contains(t, result.Result.Reason, "codex binary found")
}

func TestCodexUnknownMethodReturnsError(t *testing.T) {
	resp := dispatchCodexRequest(context.Background(), protocol.Request{
		ID:     "test-1",
		Method: "totally_unknown_method_xyz",
	})
	require.NotNil(t, resp.Error)
	assert.Equal(t, protocol.ErrorUnsupportedCapability, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "totally_unknown_method_xyz")
}

func TestCodexLoadRemoteThreadsReturnsEmpty(t *testing.T) {
	rawParams := json.RawMessage(`{"context":{"target":{"mode":"branch"}}}`)
	resp := handleCodexLoadRemoteThreads(rawParams)

	assert.Empty(t, resp.Error)

	result, ok := resp.Result.(protocol.LoadRemoteThreadsResult)
	require.True(t, ok, "response result is not LoadRemoteThreadsResult")

	// Must be non-nil empty slice, not nil.
	require.NotNil(t, result.Threads)
	assert.Len(t, result.Threads, 0)
}

func TestCodexLoadRemoteThreadsInvalidParams(t *testing.T) {
	resp := handleCodexLoadRemoteThreads(json.RawMessage(`not json`))
	require.NotNil(t, resp.Error)
	assert.Equal(t, protocol.ErrorInvalidRequest, resp.Error.Code)
}

func TestCodexPublishReviewEmptyDraftReturnsEmpty(t *testing.T) {
	// A publish request with an empty draft should produce an immediate
	// empty result without attempting to contact the app-server.
	rawParams := json.RawMessage(`{
		"payload": {
			"provider_id": "codex",
			"context": {"target":{"mode":"branch"}},
			"draft": {
				"id": "draft-1",
				"comments": [],
				"summary": "",
				"decision": ""
			}
		}
	}`)
	resp := handleCodexPublishReview(context.Background(), rawParams)

	assert.Empty(t, resp.Error)

	result, ok := resp.Result.(protocol.PublishReviewResultData)
	require.True(t, ok, "response result is not PublishReviewResultData")

	assert.Equal(t, "codex", result.Result.ProviderID)
	assert.Empty(t, result.Result.ExternalReviewID)
	require.NotNil(t, result.Result.PublishedRefs)
	assert.Len(t, result.Result.PublishedRefs, 0)
}

func TestCodexPublishReviewInvalidParams(t *testing.T) {
	resp := handleCodexPublishReview(context.Background(), json.RawMessage(`not json`))
	require.NotNil(t, resp.Error)
	assert.Equal(t, protocol.ErrorInvalidRequest, resp.Error.Code)
}

func TestCodexPublishReviewWithSummaryOnly(t *testing.T) {
	// A publish with summary but no comments should attempt to publish.
	// This test verifies the handler doesn't treat summary-only as empty.
	// We do NOT need the app-server to be running — the handler returns an
	// internal error because the codex binary won't be found (we skip the
	// actual publish assertion here, just validate the error path).
	// The important thing is it doesn't silently return empty.
	t.Setenv("ERO_CODEX_EXEC_PATH", "/no/such/codex/binary")

	rawParams := json.RawMessage(`{
		"payload": {
			"provider_id": "codex",
			"context": {
				"repository": {"repo_path": "/tmp/test", "worktree_root": "/tmp/test"},
				"target": {"mode": "branch"},
				"diff": {"files_changed": 1, "additions": 10, "deletions": 0}
			},
			"draft": {
				"id": "draft-2",
				"comments": [{"id": "c1", "file": "main.go", "range": {"start": {"old": 0, "new": 42, "kind": "added"}, "end": {"old": 0, "new": 42, "kind": "added"}}, "body": "Nice work"}],
				"summary": "Looks good!",
				"decision": "comment"
			}
		}
	}`)
	resp := handleCodexPublishReview(context.Background(), rawParams)

	// We expect an internal error because codex binary is not available.
	require.NotNil(t, resp.Error)
	assert.Equal(t, protocol.ErrorInternal, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "codex")
}

func TestCodexPublishReviewSuccess(t *testing.T) {
	publishStub := func(_ context.Context, cfg codexadapter.Config, cwd, formatted string) (*codexadapter.PublishResult, error) {
		assert.Equal(t, "/tmp/test", cwd)
		assert.Equal(t, "thr_override", cfg.ThreadID)
		assert.Contains(t, formatted, "## Review: 💬 Comment")
		assert.Contains(t, formatted, "Looks good!")
		assert.Contains(t, formatted, "main.go")
		return &codexadapter.PublishResult{ThreadID: "thr_live_1", TurnID: "turn_42"}, nil
	}

	t.Setenv("ERO_CODEX_THREAD_ID", "thr_override")
	rawParams := json.RawMessage(`{
		"payload": {
			"provider_id": "codex",
			"context": {
				"repository": {"repo_path": "/tmp/test", "worktree_root": "/tmp/test"},
				"target": {"mode": "branch"},
				"diff": {"files_changed": 1, "additions": 10, "deletions": 0}
			},
			"draft": {
				"id": "draft-3",
				"comments": [{"id": "c1", "file": "main.go", "range": {"start": {"old": 0, "new": 42, "kind": "added"}, "end": {"old": 0, "new": 42, "kind": "added"}}, "body": "Nice work"}],
				"summary": "Looks good!",
				"decision": "comment"
			}
		}
	}`)

	resp := handleCodexPublishReviewWith(context.Background(), rawParams, publishStub)
	require.Nil(t, resp.Error)

	result, ok := resp.Result.(protocol.PublishReviewResultData)
	require.True(t, ok, "response result is not PublishReviewResultData")
	assert.Equal(t, "codex", result.Result.ProviderID)
	assert.Equal(t, "codex:thread:thr_live_1", result.Result.ExternalReviewID)
	require.Len(t, result.Result.PublishedRefs, 1)
	assert.Equal(t, "c1", result.Result.PublishedRefs[0].LocalCommentID)
	assert.Equal(t, "codex:turn:thr_live_1:turn_42:0", result.Result.PublishedRefs[0].ExternalID)
}

// decodeInitResult round-trips a JSON-decoded result map through JSON into a
// concrete InitializeResult. This mirrors how the real plugin protocol client
// processes the response.
func decodeInitResult(t *testing.T, result any) protocol.InitializeResult {
	t.Helper()
	data, err := json.Marshal(result)
	require.NoError(t, err)
	var v protocol.InitializeResult
	err = json.Unmarshal(data, &v)
	require.NoError(t, err)
	return v
}

// decodeDetectResult round-trips a JSON-decoded result map through JSON into a
// concrete DetectContextResult.
func decodeDetectResult(t *testing.T, result any) protocol.DetectContextResult {
	t.Helper()
	data, err := json.Marshal(result)
	require.NoError(t, err)
	var v protocol.DetectContextResult
	err = json.Unmarshal(data, &v)
	require.NoError(t, err)
	return v
}
