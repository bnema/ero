package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// pipeFakeServer is a fake codex app-server that speaks JSON-RPC over pipes.
// It handles initialize, initialized, thread/loaded/list, thread/resume,
// thread/start, thread/read, thread/list, and turn/start.
func pipeFakeServer(t *testing.T, stdin io.Reader, stdout io.Writer, errCh chan<- error) {
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
					"data": []string{"thr_loaded_1"},
				},
			})

		case "thread/read":
			// Response shape per Codex app-server API:
			// { "id": ..., "result": { "thread": { "id": "...", "sessionId": "...", "cwd": "...", ... } } }
			_ = encoder.Encode(map[string]any{
				"id": req.ID,
				"result": map[string]any{
					"thread": map[string]any{
						"id":        "thr_loaded_1",
						"sessionId": "sess_loaded_1",
						"cwd":       "/home/user/project",
						"preview":   "Loaded review thread",
						"status":    map[string]any{"type": "idle"},
						"createdAt": 1700000000,
						"updatedAt": 1700000000,
					},
				},
			})

		case "thread/list":
			_ = encoder.Encode(map[string]any{
				"id": req.ID,
				"result": map[string]any{
					"data": []map[string]any{
						{
							"id":        "thr_stored_1",
							"sessionId": "sess_stored_1",
							"cwd":       "/home/user/project",
							"preview":   "Stored review",
							"status":    map[string]any{"type": "notLoaded"},
							"createdAt": 1700000000,
							"updatedAt": 1700000000,
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

// TestInitializeAppendsNewline verifies that the initialized notification
// includes a trailing newline (required for JSONL transport).
// It also verifies that Initialize + ReadThread work correctly with the
// proper response shapes.
func TestInitializeAppendsNewline(t *testing.T) {
	serverStdinR, clientStdoutW := io.Pipe()
	clientStdinR, serverStdoutW := io.Pipe()

	serverErr := make(chan error, 1)

	go pipeFakeServer(t, serverStdinR, serverStdoutW, serverErr)
	time.Sleep(10 * time.Millisecond)

	// Construct AppServerClient directly using the pipes.
	scanner := bufio.NewScanner(clientStdinR)
	scanner.Buffer(make([]byte, 2*1024*1024), 2*1024*1024)

	client := &AppServerClient{
		cfg:        Config{CommandTimeout: 5 * time.Second},
		stdin:      clientStdoutW,
		scanner:    scanner,
		timeout:    5 * time.Second,
		maxMsgSize: 2 * 1024 * 1024,
	}

	// Initialize handshake.
	if err := client.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if !client.hs.IsReady() {
		t.Fatal("Initialize should leave handshake in Ready state")
	}

	// readThread should parse the correct wrapped response shape.
	candidate, err := client.readThread(context.Background(), "thr_loaded_1")
	if err != nil {
		t.Fatalf("ReadThread: %v", err)
	}
	if candidate.ID != "thr_loaded_1" {
		t.Errorf("ReadThread ID = %q, want %q", candidate.ID, "thr_loaded_1")
	}
	if candidate.CWD != "/home/user/project" {
		t.Errorf("ReadThread CWD = %q, want %q", candidate.CWD, "/home/user/project")
	}
	if candidate.SessionKey != "sess_loaded_1" {
		t.Errorf("ReadThread SessionKey = %q, want %q", candidate.SessionKey, "sess_loaded_1")
	}
	if candidate.Preview != "Loaded review thread" {
		t.Errorf("ReadThread Preview = %q, want %q", candidate.Preview, "Loaded review thread")
	}
	if candidate.Status != ThreadStatusIdle {
		t.Errorf("ReadThread Status = %q, want %q", candidate.Status, ThreadStatusIdle)
	}

	// ListLoadedThreads should enrich loaded threads with CWD via ReadThread.
	loaded, err := client.ListLoadedThreads(context.Background())
	if err != nil {
		t.Fatalf("ListLoadedThreads: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 loaded thread, got %d", len(loaded))
	}
	if loaded[0].CWD != "/home/user/project" {
		t.Errorf("enriched loaded thread CWD = %q, want %q", loaded[0].CWD, "/home/user/project")
	}
	if loaded[0].SessionKey != "sess_loaded_1" {
		t.Errorf("enriched loaded thread SessionKey = %q, want %q", loaded[0].SessionKey, "sess_loaded_1")
	}

	// Resume + PublishMessage returns the turn ID.
	if err := client.ResumeThread(context.Background(), "thr_loaded_1"); err != nil {
		t.Fatalf("ResumeThread: %v", err)
	}

	turnID, err := client.PublishMessage(context.Background(), "thr_loaded_1", "Test review")
	if err != nil {
		t.Fatalf("PublishMessage: %v", err)
	}
	if turnID != "turn_1" {
		t.Errorf("PublishMessage turn ID = %q, want %q", turnID, "turn_1")
	}

	// Clean up.
	_ = clientStdinR.Close()
	_ = clientStdoutW.Close()

	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatalf("fake server error: %v", err)
		}
	default:
	}
}

// errorFakeServer returns JSON-RPC errors for every request.
// Used to verify that readResponse surfaces server errors correctly.
func errorFakeServer(t *testing.T, stdin io.Reader, stdout io.Writer, errCh chan<- error) {
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

		_ = encoder.Encode(map[string]any{
			"id": req.ID,
			"error": map[string]any{
				"code":    -32002,
				"message": "Not initialized",
			},
		})
	}

	errCh <- scanner.Err()
}

// TestInitializeRejectsErrorResponse verifies that the initialize handshake
// fails when the server returns a JSON-RPC error response.
func TestInitializeRejectsErrorResponse(t *testing.T) {
	serverStdinR, clientStdoutW := io.Pipe()
	clientStdinR, serverStdoutW := io.Pipe()

	serverErr := make(chan error, 1)
	go errorFakeServer(t, serverStdinR, serverStdoutW, serverErr)
	time.Sleep(10 * time.Millisecond)

	scanner := bufio.NewScanner(clientStdinR)
	scanner.Buffer(make([]byte, 2*1024*1024), 2*1024*1024)

	client := &AppServerClient{
		cfg:        Config{CommandTimeout: 5 * time.Second},
		stdin:      clientStdoutW,
		scanner:    scanner,
		timeout:    5 * time.Second,
		maxMsgSize: 2 * 1024 * 1024,
	}

	err := client.Initialize(context.Background())
	if err == nil {
		t.Fatal("Initialize should have failed with server error")
	}
	if rpcErr := RPCErrorFromError(err); rpcErr != nil {
		if rpcErr.Code != -32002 {
			t.Errorf("expected JSON-RPC code -32002, got %d", rpcErr.Code)
		}
		if rpcErr.Message != "Not initialized" {
			t.Errorf("expected message 'Not initialized', got %q", rpcErr.Message)
		}
	} else {
		t.Errorf("expected error to contain *RPCError, got %T: %v", err, err)
	}

	_ = clientStdinR.Close()
	_ = clientStdoutW.Close()

	select {
	case <-serverErr:
	default:
	}
}

// TestResumeThreadRejectsErrorResponse verifies that ResumeThread fails
// when the server returns a JSON-RPC error response.
func TestResumeThreadRejectsErrorResponse(t *testing.T) {
	serverStdinR, clientStdoutW := io.Pipe()
	clientStdinR, serverStdoutW := io.Pipe()

	serverErr := make(chan error, 1)
	go errorFakeServer(t, serverStdinR, serverStdoutW, serverErr)
	time.Sleep(10 * time.Millisecond)

	scanner := bufio.NewScanner(clientStdinR)
	scanner.Buffer(make([]byte, 2*1024*1024), 2*1024*1024)

	client := &AppServerClient{
		cfg:        Config{CommandTimeout: 5 * time.Second},
		stdin:      clientStdoutW,
		scanner:    scanner,
		timeout:    5 * time.Second,
		maxMsgSize: 2 * 1024 * 1024,
	}

	// Hot-wire handshake to bypass initialize for this focused test.
	client.hs = Handshake{}
	_ = client.hs.OnInitializeSent()
	_ = client.hs.OnInitializeResponse()
	_ = client.hs.OnInitializedSent()

	err := client.ResumeThread(context.Background(), "thr_any")
	if err == nil {
		t.Fatal("ResumeThread should have failed with server error")
	}
	if rpcErr := RPCErrorFromError(err); rpcErr != nil {
		if rpcErr.Code != -32002 {
			t.Errorf("expected JSON-RPC code -32002, got %d", rpcErr.Code)
		}
	} else {
		t.Errorf("expected error to contain *RPCError, got %T: %v", err, err)
	}

	_ = clientStdinR.Close()
	_ = clientStdoutW.Close()

	select {
	case <-serverErr:
	default:
	}
}

func TestResumeThreadSendsExcludeTurns(t *testing.T) {
	serverStdinR, clientStdoutW := io.Pipe()
	clientStdinR, serverStdoutW := io.Pipe()

	serverErr := make(chan error, 1)
	seen := make(chan map[string]any, 1)
	go func() {
		scanner := bufio.NewScanner(serverStdinR)
		encoder := json.NewEncoder(serverStdoutW)
		for scanner.Scan() {
			var req struct {
				ID     any             `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params,omitempty"`
			}
			if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
				serverErr <- err
				return
			}
			if req.Method != "thread/resume" {
				continue
			}
			var params map[string]any
			if err := json.Unmarshal(req.Params, &params); err != nil {
				serverErr <- err
				return
			}
			seen <- params
			_ = encoder.Encode(map[string]any{
				"id": req.ID,
				"result": map[string]any{
					"thread": map[string]any{"id": "thr_any"},
				},
			})
			serverErr <- nil
			return
		}
		serverErr <- scanner.Err()
	}()

	scanner := bufio.NewScanner(clientStdinR)
	scanner.Buffer(make([]byte, 2*1024*1024), 2*1024*1024)
	client := &AppServerClient{
		cfg:        Config{CommandTimeout: 5 * time.Second},
		stdin:      clientStdoutW,
		scanner:    scanner,
		timeout:    5 * time.Second,
		maxMsgSize: 2 * 1024 * 1024,
	}
	client.hs = Handshake{}
	_ = client.hs.OnInitializeSent()
	_ = client.hs.OnInitializeResponse()
	_ = client.hs.OnInitializedSent()

	if err := client.ResumeThread(context.Background(), "thr_any"); err != nil {
		t.Fatalf("ResumeThread: %v", err)
	}

	select {
	case params := <-seen:
		if params["threadId"] != "thr_any" {
			t.Fatalf("threadId = %v, want thr_any", params["threadId"])
		}
		if params["excludeTurns"] != true {
			t.Fatalf("excludeTurns = %v, want true", params["excludeTurns"])
		}
	case <-time.After(time.Second):
		t.Fatal("server did not observe thread/resume")
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("server error: %v", err)
	}
	_ = clientStdinR.Close()
	_ = clientStdoutW.Close()
}

type blockingReadConn struct {
	readStarted chan struct{}
	closed      chan struct{}
	closeOnce   sync.Once
}

func (c *blockingReadConn) Read([]byte) (int, error) {
	select {
	case <-c.readStarted:
	default:
		close(c.readStarted)
	}
	<-c.closed
	return 0, io.ErrClosedPipe
}

func (c *blockingReadConn) Write(p []byte) (int, error) { return len(p), nil }

func (c *blockingReadConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func TestDoReadResponseTimeoutClosesTransportAndClient(t *testing.T) {
	conn := &blockingReadConn{readStarted: make(chan struct{}), closed: make(chan struct{})}
	client := &AppServerClient{
		cfg:        Config{CommandTimeout: 20 * time.Millisecond},
		conn:       conn,
		timeout:    20 * time.Millisecond,
		maxMsgSize: 1024,
	}

	err := client.doReadResponse(context.Background(), json.RawMessage("1"), &Message{}, nil)
	if err == nil {
		t.Fatal("doReadResponse should time out")
	}

	select {
	case <-conn.readStarted:
	default:
		t.Fatal("read was not started")
	}
	select {
	case <-conn.closed:
	case <-time.After(time.Second):
		t.Fatal("transport was not closed after timeout")
	}

	if _, err := client.sendRequestRaw(context.Background(), "thread/list", map[string]any{}); err == nil {
		t.Fatal("client should be terminal after read timeout")
	}
}

func TestDoReadResponseTimeoutReapsStdioSubprocess(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	cmd := exec.Command(self, "-test.run=TestStdioBlockingHelperProcess")
	cmd.Env = append(os.Environ(), "ERO_CODEX_BLOCKING_HELPER=1")
	cmd.Stderr = io.Discard

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	scanner := bufio.NewScanner(stdout)
	client := &AppServerClient{
		cmd:        cmd,
		stdin:      stdin,
		scanner:    scanner,
		timeout:    20 * time.Millisecond,
		maxMsgSize: 1024,
	}

	err = client.doReadResponse(context.Background(), json.RawMessage("1"), &Message{}, nil)
	if err == nil {
		t.Fatal("doReadResponse should time out")
	}
	if cmd.ProcessState == nil {
		t.Fatal("timed-out stdio subprocess was not waited/reaped")
	}
	if _, err := client.sendRequestRaw(context.Background(), "thread/list", map[string]any{}); err == nil {
		t.Fatal("client should be terminal after read timeout")
	}
}

type blockingWriteConn struct {
	writeStarted chan struct{}
	closed       chan struct{}
	closeOnce    sync.Once
}

func (c *blockingWriteConn) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }

func (c *blockingWriteConn) Write([]byte) (int, error) {
	select {
	case <-c.writeStarted:
	default:
		close(c.writeStarted)
	}
	<-c.closed
	return 0, io.ErrClosedPipe
}

func (c *blockingWriteConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func TestWriteCancellationClosesTransportAndClient(t *testing.T) {
	conn := &blockingWriteConn{writeStarted: make(chan struct{}), closed: make(chan struct{})}
	client := &AppServerClient{
		cfg:        Config{CommandTimeout: 20 * time.Millisecond},
		conn:       conn,
		timeout:    20 * time.Millisecond,
		maxMsgSize: 1024,
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- client.write(ctx, []byte(`{"id":1,"method":"thread/list"}`))
	}()

	select {
	case <-conn.writeStarted:
	case <-time.After(time.Second):
		t.Fatal("write was not started")
	}
	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("write should be canceled")
		}
	case <-time.After(time.Second):
		t.Fatal("write did not return after cancellation")
	}
	select {
	case <-conn.closed:
	case <-time.After(time.Second):
		t.Fatal("transport was not closed after write cancellation")
	}

	if _, err := client.sendRequestRaw(context.Background(), "thread/list", map[string]any{}); err == nil {
		t.Fatal("client should be terminal after write cancellation")
	}
}

func TestWriteTimeoutReapsStdioSubprocess(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	cmd := exec.Command(self, "-test.run=TestStdioBlockingHelperProcess")
	cmd.Env = append(os.Environ(), "ERO_CODEX_BLOCKING_HELPER=1")
	cmd.Stderr = io.Discard

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	client := &AppServerClient{
		cmd:        cmd,
		stdin:      stdin,
		scanner:    bufio.NewScanner(stdout),
		timeout:    20 * time.Millisecond,
		maxMsgSize: 1024,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := client.write(ctx, bytes.Repeat([]byte("x"), 64*1024*1024)); err == nil {
		t.Fatal("write should time out")
	}
	if cmd.ProcessState == nil {
		t.Fatal("timed-out stdio subprocess was not waited/reaped")
	}
	if _, err := client.sendRequestRaw(context.Background(), "thread/list", map[string]any{}); err == nil {
		t.Fatal("client should be terminal after write timeout")
	}
}

func TestStdioBlockingHelperProcess(t *testing.T) {
	if os.Getenv("ERO_CODEX_BLOCKING_HELPER") != "1" {
		return
	}
	select {}
}

// TestPublishReviewSuccess verifies a full publish workflow via pipe-based fake server,
// proving that a successful publish returns external review/comment IDs with a real turn ID.
func TestPublishReviewSuccess(t *testing.T) {
	serverStdinR, clientStdoutW := io.Pipe()
	clientStdinR, serverStdoutW := io.Pipe()

	serverErr := make(chan error, 1)
	go pipeFakeServer(t, serverStdinR, serverStdoutW, serverErr)
	time.Sleep(10 * time.Millisecond)

	scanner := bufio.NewScanner(clientStdinR)
	scanner.Buffer(make([]byte, 2*1024*1024), 2*1024*1024)

	client := &AppServerClient{
		cfg:        Config{CommandTimeout: 5 * time.Second},
		stdin:      clientStdoutW,
		scanner:    scanner,
		timeout:    5 * time.Second,
		maxMsgSize: 2 * 1024 * 1024,
	}

	if err := client.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// List loaded threads (enriched via readThread).
	loaded, err := client.ListLoadedThreads(context.Background())
	if err != nil {
		t.Fatalf("ListLoadedThreads: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 loaded thread, got %d", len(loaded))
	}
	if loaded[0].SessionKey != "sess_loaded_1" {
		t.Errorf("expected SessionKey sess_loaded_1, got %q", loaded[0].SessionKey)
	}

	// Resume the loaded thread.
	if err := client.ResumeThread(context.Background(), loaded[0].ID); err != nil {
		t.Fatalf("ResumeThread: %v", err)
	}

	// Publish a review message and capture the turn ID.
	turnID, err := client.PublishMessage(context.Background(), loaded[0].ID, "Test review summary")
	if err != nil {
		t.Fatalf("PublishMessage: %v", err)
	}
	if turnID == "" {
		t.Fatal("PublishMessage returned empty turn ID")
	}
	if turnID != "turn_1" {
		t.Errorf("PublishMessage turn ID = %q, want %q", turnID, "turn_1")
	}

	// Verify that external IDs can be built from ThreadID + TurnID.
	reviewID := BuildExternalReviewID(loaded[0].ID)
	if reviewID == "" {
		t.Fatal("BuildExternalReviewID returned empty")
	}
	commentID := BuildExternalCommentID(loaded[0].ID, turnID, 0)
	if commentID == "" {
		t.Fatal("BuildExternalCommentID returned empty")
	}
	if commentID != "codex:turn:thr_loaded_1:turn_1:0" {
		t.Errorf("comment ID = %q, want %q", commentID, "codex:turn:thr_loaded_1:turn_1:0")
	}

	// Clean up.
	_ = clientStdinR.Close()
	_ = clientStdoutW.Close()

	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatalf("fake server error: %v", err)
		}
	default:
	}
}
