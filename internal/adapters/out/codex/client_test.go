// Package codex_test provides unit and integration tests for the codex
// adapter package. Integration tests communicate with a simulated codex
// app-server via pipes.
package codex_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/websocket"

	"ero/internal/adapters/out/codex"
)

// ---------------------------------------------------------------------------
// Config and helper tests
// ---------------------------------------------------------------------------

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("ERO_CODEX_THREAD_ID", "thr_custom")
	t.Setenv("ERO_CODEX_SOCKET_PATH", "/tmp/test.sock")
	t.Setenv("ERO_CODEX_TIMEOUT", "2m")

	cfg := codex.ConfigFromEnv()

	if cfg.ThreadID != "thr_custom" {
		t.Errorf("ThreadID = %q, want %q", cfg.ThreadID, "thr_custom")
	}
	if cfg.SocketPath != "/tmp/test.sock" {
		t.Errorf("SocketPath = %q, want %q", cfg.SocketPath, "/tmp/test.sock")
	}
	if cfg.CommandTimeout != 2*time.Minute {
		t.Errorf("CommandTimeout = %v, want 2m", cfg.CommandTimeout)
	}
}

func TestConfigFromEnvDefaults(t *testing.T) {
	cfg := codex.ConfigFromEnv()

	if cfg.ThreadID != "" {
		t.Errorf("expected empty ThreadID, got %q", cfg.ThreadID)
	}
	if cfg.SocketPath != "" {
		t.Errorf("expected empty SocketPath, got %q", cfg.SocketPath)
	}
}

func TestEffectiveTimeout(t *testing.T) {
	cfg := codex.Config{}
	if d := cfg.EffectiveTimeout(); d != 30*time.Second {
		t.Errorf("default timeout = %v, want 30s", d)
	}

	cfg = codex.Config{CommandTimeout: 10 * time.Second}
	if d := cfg.EffectiveTimeout(); d != 10*time.Second {
		t.Errorf("custom timeout = %v, want 10s", d)
	}
}

func TestConfigFromEnvTimeoutParsesDuration(t *testing.T) {
	t.Setenv("ERO_CODEX_TIMEOUT", "45s")
	cfg := codex.ConfigFromEnv()
	if cfg.CommandTimeout != 45*time.Second {
		t.Errorf("expected 45s timeout, got %v", cfg.CommandTimeout)
	}
}

func TestConfigFromEnvTimeoutInvalidDuration(t *testing.T) {
	t.Setenv("ERO_CODEX_TIMEOUT", "not-a-duration")
	cfg := codex.ConfigFromEnv()
	if cfg.CommandTimeout != 0 {
		t.Errorf("expected zero timeout for invalid input, got %v", cfg.CommandTimeout)
	}
	// EffectiveTimeout should fall back to default.
	if d := cfg.EffectiveTimeout(); d != 30*time.Second {
		t.Errorf("expected default 30s, got %v", d)
	}
}

func TestConfigFromEnvTimeoutNegativeValue(t *testing.T) {
	t.Setenv("ERO_CODEX_TIMEOUT", "-10s")
	cfg := codex.ConfigFromEnv()
	if cfg.CommandTimeout != 0 {
		t.Errorf("expected zero timeout for negative value, got %v", cfg.CommandTimeout)
	}
}

func TestEffectiveSocketPath(t *testing.T) {
	// EffectiveSocketPath returns the SocketPath field directly.
	cfg := codex.Config{}.EffectiveSocketPath()
	if cfg != "" {
		t.Errorf("expected empty socket path for zero Config, got %q", cfg)
	}

	path := codex.Config{SocketPath: "/tmp/codex.sock"}.EffectiveSocketPath()
	if path != "/tmp/codex.sock" {
		t.Errorf("Expected /tmp/codex.sock, got %q", path)
	}
}

func TestValidateCallbackTarget(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		cfg := codex.Config{ThreadID: "thr_abc", SocketPath: "/tmp/sock"}
		if err := cfg.ValidateCallbackTarget(); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("missing both", func(t *testing.T) {
		cfg := codex.Config{}
		err := cfg.ValidateCallbackTarget()
		if err == nil {
			t.Fatal("expected error for empty config")
		}
		if !errors.Is(err, codex.ErrMissingCallbackConfig) {
			t.Errorf("expected ErrMissingCallbackConfig, got %v", err)
		}
	})

	t.Run("missing socket", func(t *testing.T) {
		cfg := codex.Config{ThreadID: "thr_abc"}
		err := cfg.ValidateCallbackTarget()
		if err == nil {
			t.Fatal("expected error when SocketPath is empty")
		}
		if !errors.Is(err, codex.ErrMissingCallbackConfig) {
			t.Errorf("expected ErrMissingCallbackConfig, got %v", err)
		}
	})

	t.Run("missing thread", func(t *testing.T) {
		cfg := codex.Config{SocketPath: "/tmp/sock"}
		err := cfg.ValidateCallbackTarget()
		if err == nil {
			t.Fatal("expected error when ThreadID is empty")
		}
		if !errors.Is(err, codex.ErrMissingCallbackConfig) {
			t.Errorf("expected ErrMissingCallbackConfig, got %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// Callback-only production workflow tests
// ---------------------------------------------------------------------------

func TestSendCallbackUsesExplicitThreadOverLiveSession(t *testing.T) {
	socketPath, server := startCallbackWSServer(t)

	result, err := codex.SendCallback(context.Background(), codex.Config{
		SocketPath:     socketPath,
		ThreadID:       "thr_target",
		CommandTimeout: 2 * time.Second,
	}, "Review summary")

	if err != nil {
		t.Fatalf("SendCallback returned error: %v", err)
	}
	if result.ThreadID != "thr_target" {
		t.Fatalf("ThreadID = %q, want thr_target", result.ThreadID)
	}
	if result.TurnID != "turn_1" {
		t.Fatalf("TurnID = %q, want turn_1", result.TurnID)
	}

	requests := server.requests()
	methods := make([]string, 0, len(requests))
	for _, req := range requests {
		methods = append(methods, req.Method)
	}
	wantMethods := []string{"initialize", "initialized", "turn/start"}
	if !equalStrings(methods, wantMethods) {
		t.Fatalf("methods = %v, want %v", methods, wantMethods)
	}

	var turnParams struct {
		ThreadID string `json:"threadId"`
		Input    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"input"`
	}
	if err := json.Unmarshal(requests[2].Params, &turnParams); err != nil {
		t.Fatalf("decode turn/start params: %v", err)
	}
	if turnParams.ThreadID != "thr_target" {
		t.Fatalf("turn/start threadId = %q, want thr_target", turnParams.ThreadID)
	}
	if len(turnParams.Input) != 1 || turnParams.Input[0].Type != "text" || turnParams.Input[0].Text != "Review summary" {
		t.Fatalf("turn/start input = %#v", turnParams.Input)
	}
}

func TestSendCallbackMissingTargetFailsBeforeDial(t *testing.T) {
	_, err := codex.SendCallback(context.Background(), codex.Config{
		SocketPath:     filepath.Join(t.TempDir(), "missing.sock"),
		CommandTimeout: time.Second,
	}, "Review summary")
	if err == nil {
		t.Fatal("expected missing target error")
	}
	var publishErr *codex.PublishReviewError
	if !errors.As(err, &publishErr) {
		t.Fatalf("expected PublishReviewError, got %T: %v", err, err)
	}
	if publishErr.Reason != codex.PublishErrorUnsupported {
		t.Fatalf("Reason = %q, want %q", publishErr.Reason, codex.PublishErrorUnsupported)
	}
	if !errors.Is(err, codex.ErrMissingCallbackConfig) {
		t.Fatalf("expected ErrMissingCallbackConfig in chain, got %v", err)
	}
}

type observedCallbackRequest struct {
	ID     any             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type callbackWSServer struct {
	mu       sync.Mutex
	observed []observedCallbackRequest
}

func (s *callbackWSServer) record(req observedCallbackRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observed = append(s.observed, req)
}

func (s *callbackWSServer) requests() []observedCallbackRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]observedCallbackRequest(nil), s.observed...)
}

func startCallbackWSServer(t *testing.T) (string, *callbackWSServer) {
	t.Helper()

	dir := t.TempDir()
	socketPath := filepath.Join(dir, "codex.sock")
	if err := os.MkdirAll(filepath.Dir(socketPath), 0755); err != nil {
		t.Fatalf("mkdir socket dir: %v", err)
	}
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	server := &callbackWSServer{}
	wsHandler := websocket.Server{
		Handler: func(ws *websocket.Conn) {
			ws.PayloadType = websocket.TextFrame
			ws.MaxPayloadBytes = 2 * 1024 * 1024
			for {
				var message string
				if err := websocket.Message.Receive(ws, &message); err != nil {
					return
				}
				var req observedCallbackRequest
				if err := json.Unmarshal([]byte(message), &req); err != nil {
					_ = websocket.Message.Send(ws, `{"error":{"code":-32700,"message":"Parse error"}}`)
					continue
				}
				server.record(req)
				switch req.Method {
				case "initialize":
					respondWS(t, ws, req.ID, map[string]any{
						"protocolVersion": "2.0",
						"capabilities":    map[string]any{},
						"serverInfo":      map[string]string{"name": "codex-test", "version": "0.0.0"},
					})
				case "initialized":
					// Notification: no response.
				case "turn/start":
					respondWS(t, ws, req.ID, map[string]any{
						"turn": map[string]any{"id": "turn_1", "status": "inProgress"},
					})
				default:
					respondWSError(t, ws, req.ID, -32601, "Method not found")
				}
			}
		},
		Handshake: func(*websocket.Config, *http.Request) error { return nil },
	}
	go func() {
		if err := http.Serve(ln, wsHandler); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Logf("callback websocket server error: %v", err)
		}
	}()
	return socketPath, server
}

func respondWS(t *testing.T, ws *websocket.Conn, id any, result any) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"id": id, "result": result})
	if err != nil {
		t.Errorf("marshal websocket response: %v", err)
		return
	}
	if err := websocket.Message.Send(ws, string(payload)); err != nil {
		t.Errorf("send websocket response: %v", err)
	}
}

func respondWSError(t *testing.T, ws *websocket.Conn, id any, code int, message string) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"id": id, "error": map[string]any{"code": code, "message": message}})
	if err != nil {
		t.Errorf("marshal websocket error: %v", err)
		return
	}
	if err := websocket.Message.Send(ws, string(payload)); err != nil {
		t.Errorf("send websocket error: %v", err)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
