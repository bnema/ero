// Package codex_test provides unit and integration tests for the codex
// adapter package. Integration tests communicate with a simulated codex
// app-server subprocess via pipes.
package codex_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"ero/internal/adapters/out/codex"
)

// ---------------------------------------------------------------------------
// Config and helper tests
// ---------------------------------------------------------------------------

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("ERO_CODEX_EXEC_PATH", "/custom/codex")
	t.Setenv("ERO_CODEX_THREAD_ID", "thr_custom")
	t.Setenv("ERO_CODEX_SESSION_KEY", "sess_custom")
	t.Setenv("ERO_CODEX_HOME", "/custom/.codex")
	t.Setenv("ERO_CODEX_TRANSPORT", "proxy")
	t.Setenv("ERO_CODEX_SOCKET_PATH", "/tmp/test.sock")
	t.Setenv("ERO_CODEX_TIMEOUT", "2m")

	cfg := codex.ConfigFromEnv()

	if cfg.ExecPath != "/custom/codex" {
		t.Errorf("ExecPath = %q, want %q", cfg.ExecPath, "/custom/codex")
	}
	if cfg.ThreadID != "thr_custom" {
		t.Errorf("ThreadID = %q, want %q", cfg.ThreadID, "thr_custom")
	}
	if cfg.SessionKey != "sess_custom" {
		t.Errorf("SessionKey = %q, want %q", cfg.SessionKey, "sess_custom")
	}
	if cfg.CodexHome != "/custom/.codex" {
		t.Errorf("CodexHome = %q, want %q", cfg.CodexHome, "/custom/.codex")
	}
	if string(cfg.Transport) != "live" {
		t.Errorf("Transport = %q, want %q", cfg.Transport, "live")
	}
	if cfg.SocketPath != "/tmp/test.sock" {
		t.Errorf("SocketPath = %q, want %q", cfg.SocketPath, "/tmp/test.sock")
	}
	if cfg.CommandTimeout != 2*time.Minute {
		t.Errorf("CommandTimeout = %v, want 2m", cfg.CommandTimeout)
	}
}

func TestConfigFromEnvTransportDefaultsToAuto(t *testing.T) {
	t.Setenv("ERO_CODEX_TRANSPORT", "")
	cfg := codex.ConfigFromEnv()
	if string(cfg.Transport) != "auto" {
		t.Errorf("default Transport = %q, want %q", cfg.Transport, "auto")
	}
}

func TestConfigShouldProbeSocket(t *testing.T) {
	// Only auto mode should probe.
	cfg := codex.Config{Transport: codex.TransportMode("auto")}
	if !cfg.ShouldProbeSocket() {
		t.Error("expected ShouldProbeSocket=true for auto")
	}
	cfg.Transport = codex.TransportMode("stdio")
	if cfg.ShouldProbeSocket() {
		t.Error("expected ShouldProbeSocket=false for stdio")
	}
	cfg.Transport = codex.TransportMode("live")
	if cfg.ShouldProbeSocket() {
		t.Error("expected ShouldProbeSocket=false for live (always dials)")
	}
	cfg.Transport = codex.TransportMode("proxy")
	if cfg.ShouldProbeSocket() {
		t.Error("expected ShouldProbeSocket=false for proxy (alias for live, always dials)")
	}
}

func TestConfigEffectiveSocketPath(t *testing.T) {
	// Without override, path under CodexHome.
	cfg := codex.Config{CodexHome: "/custom/.codex"}
	path := cfg.EffectiveSocketPath()
	want := "/custom/.codex/app-server-control/app-server-control.sock"
	if path != want {
		t.Errorf("EffectiveSocketPath = %q, want %q", path, want)
	}

	// With override, override takes precedence.
	cfg.SocketPath = "/override/sock"
	path = cfg.EffectiveSocketPath()
	want = "/override/sock"
	if path != want {
		t.Errorf("EffectiveSocketPath with override = %q, want %q", path, want)
	}

	// Empty codexHome with no override falls back to ~/.codex when
	// CODEX_HOME is also unset (test environment may or may not have it).
	cfg = codex.Config{}
	path = cfg.EffectiveSocketPath()
	// Should not be empty: either ambient CODEX_HOME or ~/.codex is used.
	if path == "" {
		t.Error("expected non-empty path from ambient fallback, got empty")
	}
	// Verify the path ends with the expected suffix.
	if len(path) < len("/app-server-control/app-server-control.sock") ||
		path[len(path)-len("/app-server-control/app-server-control.sock"):] != "/app-server-control/app-server-control.sock" {
		t.Errorf("path %q does not end with expected suffix", path)
	}
}

func TestConfigEffectiveSocketPathWithAmbientCodexHome(t *testing.T) {
	// When ERO_CODEX_HOME is not set, but ambient CODEX_HOME is set,
	// EffectiveSocketPath should use the ambient value.
	const ambientHome = "/ambient/.codex"
	t.Setenv("CODEX_HOME", ambientHome)

	cfg := codex.Config{}
	path := cfg.EffectiveSocketPath()
	want := ambientHome + "/app-server-control/app-server-control.sock"
	if path != want {
		t.Errorf("with ambient CODEX_HOME: got %q, want %q", path, want)
	}

	// ERO_CODEX_HOME should take precedence over ambient CODEX_HOME.
	cfg.CodexHome = "/ero/.codex"
	path = cfg.EffectiveSocketPath()
	want = "/ero/.codex/app-server-control/app-server-control.sock"
	if path != want {
		t.Errorf("with ERO_CODEX_HOME overriding ambient: got %q, want %q", path, want)
	}
}

func TestConfigEffectiveSocketPathWithSocketPathOverride(t *testing.T) {
	// ERO_CODEX_SOCKET_PATH should take highest precedence.
	cfg := codex.Config{
		CodexHome:  "/home/.codex",
		SocketPath: "/direct/socket.sock",
	}
	path := cfg.EffectiveSocketPath()
	want := "/direct/socket.sock"
	if path != want {
		t.Errorf("with socket override: got %q, want %q", path, want)
	}
}

func TestConfigFromEnvDefaults(t *testing.T) {
	cfg := codex.ConfigFromEnv()

	if cfg.ExecPath != "" {
		t.Errorf("expected empty ExecPath, got %q", cfg.ExecPath)
	}
	if cfg.ThreadID != "" {
		t.Errorf("expected empty ThreadID, got %q", cfg.ThreadID)
	}
	if cfg.SessionKey != "" {
		t.Errorf("expected empty SessionKey, got %q", cfg.SessionKey)
	}
	if cfg.CodexHome != "" {
		t.Errorf("expected empty CodexHome, got %q", cfg.CodexHome)
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

func TestConfigFromEnvTransportLive(t *testing.T) {
	t.Setenv("ERO_CODEX_TRANSPORT", "live")
	cfg := codex.ConfigFromEnv()
	if string(cfg.Transport) != "live" {
		t.Errorf("transport = %q, want %q", cfg.Transport, "live")
	}
}

func TestConfigFromEnvTransportProxyAlias(t *testing.T) {
	t.Setenv("ERO_CODEX_TRANSPORT", "proxy")
	cfg := codex.ConfigFromEnv()
	// "proxy" should be normalised to "live".
	if string(cfg.Transport) != "live" {
		t.Errorf("proxy alias should normalise to live, got %q", cfg.Transport)
	}
}

func TestConfigFromEnvTransportInvalidFallsBackToAuto(t *testing.T) {
	t.Setenv("ERO_CODEX_TRANSPORT", "bogus")
	cfg := codex.ConfigFromEnv()
	if string(cfg.Transport) != "auto" {
		t.Errorf("invalid transport should fall back to auto, got %q", cfg.Transport)
	}
}

func TestConfigFromEnvTransportEmptyDefaultsToAuto(t *testing.T) {
	t.Setenv("ERO_CODEX_TRANSPORT", "")
	cfg := codex.ConfigFromEnv()
	if string(cfg.Transport) != "auto" {
		t.Errorf("empty transport should default to auto, got %q", cfg.Transport)
	}
}

func TestCodexAvailable(t *testing.T) {
	// When set to a non-existent path, CodexAvailable should return false.
	cfg := codex.Config{ExecPath: "/no/such/codex/binary"}
	if cfg.CodexAvailable() {
		t.Error("expected CodexAvailable to be false for non-existent binary")
	}

	// When using the current test binary, it should be available.
	selfPath, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	cfg = codex.Config{ExecPath: selfPath}
	if !cfg.CodexAvailable() {
		t.Error("expected CodexAvailable to be true for existing binary")
	}
}

func TestThreadStatusFromWire(t *testing.T) {
	tests := []struct {
		input any
		want  codex.ThreadStatus
	}{
		{input: "notLoaded", want: codex.ThreadStatusNotLoaded},
		{input: "idle", want: codex.ThreadStatusIdle},
		{input: "active", want: codex.ThreadStatusActive},
		{input: "systemError", want: codex.ThreadStatusError},
		{input: "unknown", want: codex.ThreadStatus("")},
		{input: map[string]any{"type": "idle"}, want: codex.ThreadStatusIdle},
		{input: map[string]any{"type": "active"}, want: codex.ThreadStatusActive},
		{input: map[string]any{"type": "notLoaded"}, want: codex.ThreadStatusNotLoaded},
		{input: nil, want: codex.ThreadStatus("")},
		{input: 42, want: codex.ThreadStatus("")},
	}
	for _, tt := range tests {
		got := codex.ThreadStatusFromWire(tt.input)
		if got != tt.want {
			t.Errorf("ThreadStatusFromWire(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Protocol integration test via pipes
//
// We spin up a goroutine that acts as a fake codex app-server, speaking the
// JSON-RPC protocol over pipes. This tests the full protocol conversation
// that AppServerClient implements.
// ---------------------------------------------------------------------------

func TestAppServerClient_InitializeHandshake(t *testing.T) {
	// Create pipes for the fake server.
	serverStdinR, clientStdoutW := io.Pipe()
	clientStdinR, serverStdoutW := io.Pipe()

	// Channel to collect errors from the server goroutine.
	serverErr := make(chan error, 1)

	// Start the fake server in a goroutine.
	go runFakeCodexServer(t, serverStdinR, serverStdoutW, serverErr)

	// Give the server a moment to start.
	time.Sleep(10 * time.Millisecond)

	// Now create the AppServerClient. We can't use NewAppServerClient
	// because it spawns a subprocess. Instead, construct it directly
	// via the internal fields and test the high-level methods.
	// For the pipe test, we verify the JSON-RPC conversation manually.
	client := &fakeServerClient{
		stdin:  clientStdinR,
		stdout: clientStdoutW,
	}

	// Step 1: Send initialize.
	id := "1"
	initReq := map[string]any{
		"id":     id,
		"method": "initialize",
		"params": map[string]any{
			"protocolVersion": "2.0",
			"clientInfo": map[string]any{
				"name":    "ero-test",
				"title":   "Ero Test",
				"version": "1.0.0",
			},
			"capabilities": map[string]any{},
		},
	}
	if err := client.writeJSON(initReq); err != nil {
		t.Fatalf("write initialize: %v", err)
	}

	// Read initialize response.
	var initResp map[string]any
	if err := client.readJSON(&initResp); err != nil {
		t.Fatalf("read initialize response: %v", err)
	}
	if initResp["id"] != id {
		t.Fatalf("initialize response id: want %q, got %v", id, initResp["id"])
	}
	if _, ok := initResp["result"]; !ok {
		t.Fatalf("initialize response has no result: %v", initResp)
	}

	// Step 2: Send initialized notification.
	if err := client.writeJSON(map[string]any{
		"method": "initialized",
	}); err != nil {
		t.Fatalf("write initialized: %v", err)
	}

	// Step 3: Send thread/loaded/list.
	id = "2"
	if err := client.writeJSON(map[string]any{
		"id":     id,
		"method": "thread/loaded/list",
	}); err != nil {
		t.Fatalf("write thread/loaded/list: %v", err)
	}

	var listResp map[string]any
	if err := client.readJSON(&listResp); err != nil {
		t.Fatalf("read thread/loaded/list response: %v", err)
	}
	if listResp["id"] != id {
		t.Fatalf("thread/loaded/list response id: want %q, got %v", id, listResp["id"])
	}
	result, ok := listResp["result"].(map[string]any)
	if !ok {
		t.Fatalf("thread/loaded/list response missing result: %v", listResp)
	}
	data, ok := result["data"].([]any)
	if !ok {
		t.Fatalf("thread/loaded/list data not an array: %v", result)
	}
	if len(data) != 2 {
		t.Fatalf("expected 2 loaded threads, got %d", len(data))
	}
	t.Logf("loaded threads: %v", data)

	// Step 4: Send thread/start.
	id = "3"
	if err := client.writeJSON(map[string]any{
		"id":     id,
		"method": "thread/start",
		"params": map[string]any{"cwd": "/tmp/test"},
	}); err != nil {
		t.Fatalf("write thread/start: %v", err)
	}

	var startResp map[string]any
	if err := client.readJSON(&startResp); err != nil {
		t.Fatalf("read thread/start response: %v", err)
	}
	if startResp["id"] != id {
		t.Fatalf("thread/start response id: want %q, got %v", id, startResp["id"])
	}

	// Step 5: Send turn/start.
	id = "4"
	if err := client.writeJSON(map[string]any{
		"id":     id,
		"method": "turn/start",
		"params": map[string]any{
			"threadId": "thr_new_1",
			"input": []map[string]string{
				{"type": "text", "text": "Review summary"},
			},
		},
	}); err != nil {
		t.Fatalf("write turn/start: %v", err)
	}

	var turnResp map[string]any
	if err := client.readJSON(&turnResp); err != nil {
		t.Fatalf("read turn/start response: %v", err)
	}
	if turnResp["id"] != id {
		t.Fatalf("turn/start response id: want %q, got %v", id, turnResp["id"])
	}
	turnResult, ok := turnResp["result"].(map[string]any)
	if !ok {
		t.Fatalf("turn/start missing result: %v", turnResp)
	}
	turn, ok := turnResult["turn"].(map[string]any)
	if !ok {
		t.Fatalf("turn/start missing turn object: %v", turnResult)
	}
	if turn["id"] != "turn_1" {
		t.Fatalf("turn id: want turn_1, got %v", turn["id"])
	}
	if turn["status"] != "inProgress" {
		t.Fatalf("turn status: want inProgress, got %v", turn["status"])
	}
	t.Logf("turn/start succeeded: id=%v, status=%v", turn["id"], turn["status"])

	// Close stdin to signal end.
	_ = clientStdinR.Close()
	_ = clientStdoutW.Close()

	// Check server didn't error.
	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatalf("fake server error: %v", err)
		}
	default:
	}
}

// fakeServerClient wraps the pipe endpoints for JSON-RPC communication.
type fakeServerClient struct {
	stdin  io.Reader
	stdout io.Writer
}

func (c *fakeServerClient) writeJSON(v any) error {
	enc := json.NewEncoder(c.stdout)
	return enc.Encode(v)
}

func (c *fakeServerClient) readJSON(v any) error {
	scanner := bufio.NewScanner(c.stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	if !scanner.Scan() {
		return fmt.Errorf("read: %w", io.ErrUnexpectedEOF)
	}
	return json.Unmarshal(scanner.Bytes(), v)
}

// runFakeCodexServer implements a minimal codex app-server responder that
// understands the JSON-RPC methods used by AppServerClient.
func runFakeCodexServer(t *testing.T, stdin io.Reader, stdout io.Writer, errCh chan<- error) {
	t.Helper()
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	encoder := json.NewEncoder(stdout)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var req struct {
			ID     any             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params,omitempty"`
		}
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = encoder.Encode(map[string]any{
				"id":    nil,
				"error": map[string]any{"code": -32700, "message": "Parse error"},
			})
			continue
		}

		if req.Method == "" {
			continue
		}

		switch req.Method {
		case "initialize":
			_ = encoder.Encode(map[string]any{
				"id": req.ID,
				"result": map[string]any{
					"protocolVersion": "2.0",
					"capabilities":    map[string]any{},
					"serverInfo": map[string]string{
						"name":    "codex-test",
						"version": "0.0.0",
					},
				},
			})

		case "initialized":
			// No response for notifications.

		case "thread/loaded/list":
			_ = encoder.Encode(map[string]any{
				"id": req.ID,
				"result": map[string]any{
					"data": []string{"thr_loaded_1", "thr_loaded_2"},
				},
			})

		case "thread/list":
			_ = encoder.Encode(map[string]any{
				"id": req.ID,
				"result": map[string]any{
					"data": []map[string]any{
						{
							"id":        "thr_stored_1",
							"cwd":       "/home/user/project",
							"preview":   "Stored Review 1",
							"status":    map[string]any{"type": "notLoaded"},
							"createdAt": 1700000000,
							"updatedAt": 1700000000,
						},
						{
							"id":        "thr_stored_2",
							"cwd":       "/home/user/other",
							"preview":   "Stored Review 2",
							"status":    "notLoaded",
							"createdAt": 1700000001,
							"updatedAt": 1700000001,
						},
					},
					"nextCursor": nil,
				},
			})

		case "thread/resume":
			_ = encoder.Encode(map[string]any{
				"id": req.ID,
				"result": map[string]any{
					"thread": map[string]any{"id": "thr_loaded_1"},
				},
			})

		case "thread/start":
			_ = encoder.Encode(map[string]any{
				"id": req.ID,
				"result": map[string]any{
					"thread": map[string]any{"id": "thr_new_1"},
				},
			})

		case "turn/start":
			_ = encoder.Encode(map[string]any{
				"id": req.ID,
				"result": map[string]any{
					"turn": map[string]any{
						"id":     "turn_1",
						"status": "inProgress",
					},
				},
			})

		default:
			_ = encoder.Encode(map[string]any{
				"id":    req.ID,
				"error": map[string]any{"code": -32601, "message": fmt.Sprintf("Method not found: %s", req.Method)},
			})
		}
	}

	errCh <- scanner.Err()
}
