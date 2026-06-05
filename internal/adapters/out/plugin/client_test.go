package pluginadapter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"ero/internal/core"
	pluginsdk "ero/pkg/plugin"
	"ero/pkg/plugin/protocol"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePluginCommand returns the test binary path and args to run the fake
// plugin when FAKE_PLUGIN=1 env is set.
func fakePluginCommand(t *testing.T) (string, []string) {
	t.Helper()
	return os.Args[0], []string{"-test.run=TestFakePluginProcess", "--"}
}

// TestFakePluginProcess is the entry point for the fake plugin process. It
// reads JSON-lines from stdin and responds with canned responses based on the
// method.
func TestFakePluginProcess(t *testing.T) {
	if os.Getenv("FAKE_PLUGIN") != "1" {
		return
	}

	switch os.Getenv("FAKE_PLUGIN_MODE") {
	case "malformed":
		_, _ = fmt.Fprintln(os.Stdout, "not json")
		os.Exit(0)
	case "stderr":
		_, _ = fmt.Fprintln(os.Stderr, "diagnostic from fake plugin")
	case "timeout":
		select {}
	}

	// Run the actual plugin server.
	srv := &fakePluginServer{protocolOverride: os.Getenv("FAKE_PLUGIN_PROTOCOL")}
	if err := pluginsdk.ServeReviewProvider(context.Background(), srv, os.Stdin, os.Stdout); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "fake plugin error: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

// fakePluginServer is a minimal ReviewProvider for testing the client.
type fakePluginServer struct {
	protocolOverride string
}

func (s *fakePluginServer) Initialize(_ context.Context, req pluginsdk.InitializeRequest) (pluginsdk.InitializeResult, error) {
	if req.Protocol != protocol.ProtocolVersion {
		return pluginsdk.InitializeResult{}, protocol.NewError(protocol.ErrorInvalidRequest, "unsupported protocol")
	}
	if want := os.Getenv("FAKE_PLUGIN_CONTRIBUTION_ID"); want != "" && req.ContributionID != want {
		return pluginsdk.InitializeResult{}, protocol.NewError(protocol.ErrorInvalidRequest, "unexpected contribution id: "+req.ContributionID)
	}
	protocolVersion := protocol.ProtocolVersion
	if s.protocolOverride != "" {
		protocolVersion = s.protocolOverride
	}
	return pluginsdk.InitializeResult{
		Protocol: protocolVersion,
		Provider: pluginsdk.ReviewProviderInfo{
			ID:    "fake",
			Label: "Fake Plugin",
			Name:  "fake-plugin",
			Capabilities: pluginsdk.ReviewProviderCapabilities{
				LoadRemoteComments: true,
				PublishReview:      true,
				Decisions:          []pluginsdk.ReviewDecision{pluginsdk.ReviewDecisionComment, pluginsdk.ReviewDecisionApprove},
			},
		},
	}, nil
}

func (s *fakePluginServer) DetectContext(_ context.Context, req pluginsdk.DetectContextRequest) (pluginsdk.DetectContextResult, error) {
	return pluginsdk.DetectContextResult{
		Result: pluginsdk.DetectionResult{Applicable: true},
	}, nil
}

func (s *fakePluginServer) LoadRemoteThreads(_ context.Context, req pluginsdk.LoadRemoteThreadsRequest) (pluginsdk.LoadRemoteThreadsResult, error) {
	if code := os.Getenv("FAKE_PLUGIN_LOAD_ERROR_CODE"); code != "" {
		return pluginsdk.LoadRemoteThreadsResult{}, protocol.NewError(code, "remote load failed")
	}
	return pluginsdk.LoadRemoteThreadsResult{
		Threads: []pluginsdk.RemoteReviewThread{
			{
				ProviderID: "fake",
				ExternalID: "thread-1",
				FilePath:   "main.go",
				Comments: []pluginsdk.RemoteReviewComment{
					{ExternalID: "c1", Author: "bot", Body: "LGTM", CreatedAt: time.Now()},
				},
			},
		},
	}, nil
}

func (s *fakePluginServer) PublishReview(_ context.Context, req pluginsdk.PublishReviewParams) (pluginsdk.PublishReviewResultData, error) {
	return pluginsdk.PublishReviewResultData{
		Result: pluginsdk.ReviewPublishResult{
			ProviderID:       "fake",
			ExternalReviewID: "ext-1",
			ExternalURL:      "https://example.com/review/1",
			PublishedRefs: []pluginsdk.PublishedReviewCommentRef{
				{LocalCommentID: "comment-1", ExternalID: "ext-c1"},
			},
		},
	}, nil
}

func setupFakeClient(t *testing.T) *Client {
	t.Helper()
	return setupFakeClientWithEnv(t, DefaultPluginTimeout)
}

func setupFakeClientWithEnv(t *testing.T, timeout time.Duration, extraEnv ...string) *Client {
	t.Helper()

	cmd, args := fakePluginCommand(t)
	env := append(os.Environ(), "FAKE_PLUGIN=1")
	env = append(env, extraEnv...)

	client, err := NewClientWithEnv(cmd, args, "", timeout, env)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	return client
}

// NewClientWithEnv is a test-only constructor that allows passing custom
// environment variables to the subprocess.
func NewClientWithEnv(command string, args []string, dir string, timeout time.Duration, env []string) (*Client, error) {
	return NewClientWithContributionEnv(command, args, dir, "", timeout, env)
}

func NewClientWithContributionEnv(command string, args []string, dir, contributionID string, timeout time.Duration, env []string) (*Client, error) {
	if timeout <= 0 {
		timeout = DefaultPluginTimeout
	}

	cmd := exec.Command(command, args...)
	cmd.Dir = dir
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("plugin stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("plugin stdout pipe: %w", err)
	}

	stderrBuf := &syncBuffer{}
	cmd.Stderr = stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("plugin start: %w", err)
	}

	decoder := json.NewDecoder(bufio.NewReader(stdout))
	return &Client{
		cmd:            cmd,
		stdin:          stdin,
		decoder:        decoder,
		stderr:         stderrBuf,
		timeout:        timeout,
		contributionID: contributionID,
	}, nil
}

func TestClientInitialize(t *testing.T) {
	client := setupFakeClient(t)

	ctx := context.Background()
	info, err := client.Initialize(ctx)
	require.NoError(t, err)

	assert.Equal(t, "fake", info.ID)
	assert.Equal(t, "Fake Plugin", info.Label)
	assert.Equal(t, "fake-plugin", info.Name)
	assert.True(t, info.Capabilities.LoadRemoteComments)
	assert.True(t, info.Capabilities.PublishReview)
	assert.Len(t, info.Capabilities.Decisions, 2)
}

func TestClientInitializeSendsContributionID(t *testing.T) {
	cmd, args := fakePluginCommand(t)
	env := append(os.Environ(), "FAKE_PLUGIN=1", "FAKE_PLUGIN_CONTRIBUTION_ID=github")
	client, err := NewClientWithContributionEnv(cmd, args, "", "github", DefaultPluginTimeout, env)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	_, err = client.Initialize(context.Background())
	require.NoError(t, err)
}

func TestClientDetectContext(t *testing.T) {
	client := setupFakeClient(t)

	ctx := context.Background()

	reviewCtx := core.ReviewContext{
		Repository: core.RepositoryMetadata{RepoPath: ".", WorktreeRoot: "/tmp/repo"},
	}

	result, err := client.DetectContext(ctx, reviewCtx)
	require.NoError(t, err)

	assert.True(t, result.Applicable)
}

func TestClientLoadRemoteThreads(t *testing.T) {
	client := setupFakeClient(t)

	ctx := context.Background()

	reviewCtx := core.ReviewContext{
		Repository: core.RepositoryMetadata{RepoPath: ".", WorktreeRoot: "/tmp/repo"},
	}

	threads, err := client.LoadRemoteThreads(ctx, reviewCtx)
	require.NoError(t, err)

	require.Len(t, threads, 1)
	assert.Equal(t, "fake", threads[0].ProviderID)
	assert.Equal(t, "thread-1", threads[0].ExternalID)
	assert.Equal(t, "main.go", threads[0].FilePath)
	require.Len(t, threads[0].Comments, 1)
	assert.Equal(t, "LGTM", threads[0].Comments[0].Body)
}

func TestClientMapsProtocolErrorsToProviderErrors(t *testing.T) {
	client := setupFakeClientWithEnv(t, DefaultPluginTimeout, "FAKE_PLUGIN_LOAD_ERROR_CODE="+protocol.ErrorRemoteRateLimited)

	_, err := client.LoadRemoteThreads(context.Background(), core.ReviewContext{})
	require.Error(t, err)
	assert.Equal(t, core.ProviderErrorRateLimited, core.ClassifyProviderError(err))
	assert.True(t, core.IsRetryableProviderError(err))
}

func TestClientPublishReview(t *testing.T) {
	client := setupFakeClient(t)

	ctx := context.Background()

	req := core.PublishReviewRequest{
		ProviderID: "fake",
		Context: core.ReviewContext{
			Repository: core.RepositoryMetadata{RepoPath: "."},
		},
		Draft: core.ReviewDraftSnapshot{
			ID: "draft-1",
			Comments: []core.ReviewComment{
				{
					ID:       "comment-1",
					FilePath: "main.go",
					Range:    validRange(),
					Body:     "looks good",
				},
			},
			IdempotencyKey: "key-1",
		},
	}

	result, err := client.PublishReview(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, "fake", result.ProviderID)
	assert.Equal(t, "ext-1", result.ExternalReviewID)
	assert.Equal(t, "https://example.com/review/1", result.ExternalURL)
	require.Len(t, result.PublishedRefs, 1)
	assert.Equal(t, "comment-1", result.PublishedRefs[0].LocalCommentID)
	assert.Equal(t, "ext-c1", result.PublishedRefs[0].ExternalID)
}

func TestClientClose(t *testing.T) {
	client := setupFakeClient(t)

	err := client.Close()
	assert.NoError(t, err)
}

func TestClientInitializeRejectsProtocolMismatch(t *testing.T) {
	client := setupFakeClientWithEnv(t, DefaultPluginTimeout, "FAKE_PLUGIN_PROTOCOL=wrong.protocol")

	_, err := client.Initialize(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "protocol mismatch")
}

func TestClientMalformedStdoutMarksClientClosed(t *testing.T) {
	client := setupFakeClientWithEnv(t, DefaultPluginTimeout, "FAKE_PLUGIN_MODE=malformed")

	_, err := client.Initialize(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode response")

	_, err = client.Initialize(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

func TestClientCapturesStderrDiagnostics(t *testing.T) {
	client := setupFakeClientWithEnv(t, DefaultPluginTimeout, "FAKE_PLUGIN_MODE=stderr")

	_, err := client.Initialize(context.Background())
	require.NoError(t, err)
	assert.Contains(t, client.Stderr(), "diagnostic from fake plugin")
}

func TestClientRequestTimeoutMarksClientClosed(t *testing.T) {
	client := setupFakeClientWithEnv(t, 20*time.Millisecond, "FAKE_PLUGIN_MODE=timeout")

	_, err := client.Initialize(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")

	_, err = client.Initialize(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

func validRange() core.ReviewLineRange {
	return core.ReviewLineRange{
		Start: core.ReviewLineRef{OldLineNumber: 1, NewLineNumber: 1, Kind: core.LineKindUnchanged},
		End:   core.ReviewLineRef{OldLineNumber: 1, NewLineNumber: 1, Kind: core.LineKindUnchanged},
	}
}
