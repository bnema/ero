package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ero/pkg/plugin"
)

func TestPiCapabilities(t *testing.T) {
	provider := piProvider{}
	result, err := provider.Initialize(context.Background(), plugin.InitializeRequest{Protocol: plugin.ProtocolVersion, ContributionID: providerID})
	if err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	if result.Provider.ID != providerID || !result.Provider.Capabilities.PublishReview || result.Provider.Capabilities.LoadRemoteComments {
		t.Fatalf("unexpected provider info: %#v", result.Provider)
	}
}

func TestDetectContextRequiresBridgeSession(t *testing.T) {
	bridgeDir := secureTempDir(t)
	statePath := writeBridgeStateAt(t, filepath.Join(bridgeDir, bridgeStateFile), bridgeState{Sessions: []bridgeSession{{SessionID: "s1", Cwd: mustAbs(t, "."), SocketPath: filepath.Join(bridgeDir, "bridge.sock"), Token: "token"}}})
	provider := piProvider{statePath: statePath}
	result, err := provider.DetectContext(context.Background(), plugin.DetectContextRequest{Context: plugin.ReviewContext{Repository: plugin.RepositoryMetadata{RepoPath: "."}}})
	if err != nil {
		t.Fatalf("DetectContext returned error: %v", err)
	}
	if !result.Result.Applicable {
		t.Fatalf("expected active bridge session, got %#v", result.Result)
	}
}

func TestPublishReviewSendsMessageToBridge(t *testing.T) {
	var got bridgeReviewMessage
	bridgeDir := secureTempDir(t)
	socketPath := filepath.Join(bridgeDir, "bridge.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix socket: %v", err)
	}
	t.Cleanup(func() {
		if err := listener.Close(); err != nil {
			t.Fatalf("close listener: %v", err)
		}
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() {
			if err := conn.Close(); err != nil {
				t.Errorf("close connection: %v", err)
			}
		}()
		if err := json.NewDecoder(conn).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if err := json.NewEncoder(conn).Encode(bridgeResponse{OK: true}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}()

	statePath := writeBridgeStateAt(t, filepath.Join(bridgeDir, bridgeStateFile), bridgeState{Sessions: []bridgeSession{{SessionID: "s1", Cwd: mustAbs(t, "."), SocketPath: socketPath, Token: "secret"}}})
	provider := piProvider{statePath: statePath}
	result, err := provider.PublishReview(context.Background(), plugin.PublishReviewParams{Payload: plugin.ReviewPublishPayload{
		Context: plugin.ReviewContext{Repository: plugin.RepositoryMetadata{RepoPath: "."}, Session: plugin.ReviewSessionMetadata{LocalReviewID: "local"}},
		Draft: plugin.ReviewDraftSnapshot{Comments: []plugin.ReviewComment{{FilePath: "main.go", Body: "Please fix", Range: plugin.ReviewLineRange{
			Start: plugin.ReviewLineRef{NewLineNumber: 12},
			End:   plugin.ReviewLineRef{NewLineNumber: 14},
		}}}},
	}})
	if err != nil {
		t.Fatalf("PublishReview returned error: %v", err)
	}
	if result.Result.ProviderID != providerID || !strings.Contains(result.Result.ExternalReviewID, "s1") {
		t.Fatalf("unexpected publish result: %#v", result)
	}
	<-done
	if got.Token != "secret" || got.SessionID != "s1" || !strings.Contains(got.Message, "main.go:12-14") || !strings.Contains(got.Message, "Please fix") {
		t.Fatalf("unexpected bridge request: %#v", got)
	}
}

func TestSelectBridgeSessionPrefersEnvironmentSessionID(t *testing.T) {
	session, ok := selectBridgeSession([]bridgeSession{{SessionID: "wrong", Cwd: mustAbs(t, ".")}, {SessionID: "wanted", Cwd: "/other"}}, plugin.RepositoryMetadata{RepoPath: "."}, "wanted")
	if !ok || session.SessionID != "wanted" {
		t.Fatalf("expected preferred session, got %#v ok=%v", session, ok)
	}
}

func TestSelectBridgeSessionAllowsStaleBranchForSameWorktree(t *testing.T) {
	root := mustAbs(t, ".")
	session, ok := selectBridgeSession([]bridgeSession{
		{SessionID: "same-worktree-stale-branch", WorktreeRoot: root, CurrentBranch: "main"},
	}, plugin.RepositoryMetadata{WorktreeRoot: root, CurrentBranch: "feature"}, "")
	if !ok || session.SessionID != "same-worktree-stale-branch" {
		t.Fatalf("expected stale same-worktree session, got %#v ok=%v", session, ok)
	}
}

func TestSelectBridgeSessionMatchesBranchWhenKnown(t *testing.T) {
	root := mustAbs(t, ".")
	session, ok := selectBridgeSession([]bridgeSession{
		{SessionID: "main-session", WorktreeRoot: root, CurrentBranch: "main"},
		{SessionID: "feature-session", WorktreeRoot: root, CurrentBranch: "feature"},
	}, plugin.RepositoryMetadata{WorktreeRoot: root, CurrentBranch: "feature"}, "")
	if !ok || session.SessionID != "feature-session" {
		t.Fatalf("expected feature session, got %#v ok=%v", session, ok)
	}
}

func TestSelectBridgeSessionPrefersMostSpecificMatch(t *testing.T) {
	root := mustAbs(t, ".")
	session, ok := selectBridgeSession([]bridgeSession{
		{SessionID: "stale", WorktreeRoot: root},
		{SessionID: "exact", WorktreeRoot: root, CurrentBranch: "feature", HeadSHA: "abc"},
	}, plugin.RepositoryMetadata{WorktreeRoot: root, CurrentBranch: "feature", HeadSHA: "abc"}, "")
	if !ok || session.SessionID != "exact" {
		t.Fatalf("expected exact session, got %#v ok=%v", session, ok)
	}
}

func TestSelectBridgeSessionAcceptsSameBranchWithDifferentHead(t *testing.T) {
	root := mustAbs(t, ".")
	session, ok := selectBridgeSession([]bridgeSession{
		{SessionID: "same-branch", WorktreeRoot: root, CurrentBranch: "feature", HeadSHA: "old"},
	}, plugin.RepositoryMetadata{WorktreeRoot: root, CurrentBranch: "feature", HeadSHA: "new"}, "")
	if !ok || session.SessionID != "same-branch" {
		t.Fatalf("expected same branch session, got %#v ok=%v", session, ok)
	}
}

func writeBridgeStateAt(t *testing.T, path string, state bridgeState) string {
	t.Helper()
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write state: %v", err)
	}
	return path
}

func secureTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod temp dir: %v", err)
	}
	return dir
}

func mustAbs(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	return abs
}
