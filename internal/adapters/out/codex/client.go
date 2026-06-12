package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

// ---------------------------------------------------------------------------
// AppServerClient
//
// AppServerClient manages a codex app-server subprocess via --stdio JSON-RPC
// communication. It is designed for one-shot publish-only use: connect,
// negotiate the handshake, list/select threads, send a turn message, and
// disconnect cleanly.
// ---------------------------------------------------------------------------

// providerVersion identifies this adapter to the app-server during
// the initialize handshake.
const providerVersion = "ero.plugin.v1"

// AppServerClient communicates with the codex app-server over stdio JSON-RPC
// or WebSocket (live session). Each public method serialises its full
// request/response round-trip via an internal flight mutex so that concurrent
// calls are serialised. After a context/timeout error the transport state is
// indeterminate and the caller must discard the client (call Close).
//
// Nested calls from within a public method (e.g. ListLoadedThreads calling
// readThread) do not re-acquire the flight lock; callers must hold the lock.
type AppServerClient struct {
	cfg      Config
	cmd      *exec.Cmd
	conn     io.ReadWriteCloser
	stdin    io.WriteCloser
	scanner  *bufio.Scanner
	hs       Handshake
	mu       sync.Mutex // write + nextID serialisation
	flightMu sync.Mutex // serialises full round-trips
	nextID   int
	closed   bool
	timeout  time.Duration

	// maxMsgSize is the maximum expected JSON-RPC message size.
	// Used for buffer allocation in the WebSocket read path.
	maxMsgSize int
}

// NewAppServerClient starts an app-server connection and returns a client
// ready for the initialize handshake. Connection strategy://   - TransportModeLive or TransportModeProxy: dial the existing live session
//
//	  via direct unix-websocket; fails with an actionable error if the session
//	  is not reachable.
//	- TransportModeAuto: probe the control socket; dial live session when
//	  reachable, otherwise start a fresh app-server --stdio subprocess.
//	- TransportModeStdio (or default): always start a subprocess.
//
// When a subprocess is started, cfg.CodexHome is passed as CODEX_HOME while
// preserving the parent environment. When dialing a live session, the socket
// path is resolved from cfg (EffectiveSocketPath).
func NewAppServerClient(ctx context.Context, cfg Config) (*AppServerClient, error) {
	// First, determine the live-session path (if applicable).
	shouldDialLive := false

	switch cfg.Transport {
	case TransportModeLive, TransportModeProxy:
		// Live/proxy mode: must dial the live session or fail with an error.
		shouldDialLive = true
	case TransportModeAuto:
		// Auto mode: probe and dial when reachable.
		sockPath := cfg.EffectiveSocketPath()
		if sockPath != "" && ProbeSocket(sockPath) {
			if err := DialSocket(sockPath, SocketAvailabilityTimeout); err == nil {
				shouldDialLive = true
			}
		}
	default: // TransportModeStdio or unknown
		shouldDialLive = false
	}

	// Dial live session when requested.
	if shouldDialLive {
		client, err := DialLiveSession(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("codex: connect to live session: %w", err)
		}
		return client, nil
	}

	// Fall back to stdio subprocess.
	execPath, err := cfg.ResolveExecPath()
	if err != nil {
		return nil, fmt.Errorf("codex: resolve codex binary: %w", err)
	}

	args := []string{"app-server", "--stdio"}
	cmd := exec.CommandContext(ctx, execPath, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("codex: stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("codex: stdout pipe: %w", err)
	}

	// Preserve the parent process environment when setting CODEX_HOME.
	cmd.Env = os.Environ()
	if cfg.CodexHome != "" {
		cmd.Env = append(cmd.Env, "CODEX_HOME="+cfg.CodexHome)
	}

	// Capture stderr for diagnostics but discard (too noisy during tests).
	cmd.Stderr = io.Discard

	// Ensure the child process is terminated when the parent (builtin runtime)
	// dies, preventing orphaned subprocesses on abrupt exit.
	setParentDeathSignal(cmd)

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("codex: start app-server: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	// Allow lines up to 2 MiB.
	scanner.Buffer(make([]byte, 2*1024*1024), 2*1024*1024)

	return &AppServerClient{
		cfg:        cfg,
		cmd:        cmd,
		stdin:      stdin,
		scanner:    scanner,
		timeout:    cfg.EffectiveTimeout(),
		maxMsgSize: 2 * 1024 * 1024,
	}, nil
}

// Initialize performs the JSON-RPC initialize handshake with the app-server.
// It sends an initialize request, awaits the response, then sends the
// initialized notification. After Initialize returns successfully, the client
// is ready for regular requests.
func (c *AppServerClient) Initialize(ctx context.Context) error {
	c.flightMu.Lock()
	defer c.flightMu.Unlock()

	if !c.hs.CanSendInitialize() {
		return fmt.Errorf("codex: cannot initialize in phase %s", c.hs.Phase())
	}

	// Step 1: send initialize request.
	_ = c.hs.OnInitializeSent()

	params := map[string]any{
		"protocolVersion": "2.0",
		"clientInfo": map[string]string{
			"name":    "ero",
			"title":   "Ero Review Provider",
			"version": providerVersion,
		},
		"capabilities": map[string]any{},
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	id, err := c.sendRequestRaw(ctx, MethodInitialize, params)
	if err != nil {
		return err
	}

	// Step 2: read initialize response (just check for errors).
	if err := c.readResponse(ctx, id); err != nil {
		return fmt.Errorf("codex: initialize response: %w", err)
	}

	// Step 3: advance handshake after receiving initialize response.
	if err := c.hs.OnInitializeResponse(); err != nil {
		return err
	}

	// Step 4: send initialized notification. The stdio transport is JSONL,
	// so every message (including notifications) must be newline-delimited.
	notifData, err := EncodeNotification(MethodInitialized, nil)
	if err != nil {
		return fmt.Errorf("codex: marshal initialized notification: %w", err)
	}
	notifData = append(notifData, '\n')
	if err := c.write(ctx, notifData); err != nil {
		return fmt.Errorf("codex: send initialized notification: %w", err)
	}

	if err := c.hs.OnInitializedSent(); err != nil {
		return err
	}
	return nil
}

// ListLoadedThreads returns the set of thread IDs currently loaded in the
// app-server. Each loaded thread is returned as a ThreadCandidate with
// IsLoaded=true and details populated via readThread so that CWD-based
// matching works correctly (thread/loaded/list only returns IDs).
func (c *AppServerClient) ListLoadedThreads(ctx context.Context) ([]ThreadCandidate, error) {
	c.flightMu.Lock()
	defer c.flightMu.Unlock()

	if !c.hs.IsReady() {
		return nil, fmt.Errorf("codex: client not initialized")
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	id, err := c.sendRequestRaw(ctx, "thread/loaded/list", nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data []string `json:"data"`
	}
	if err := c.readResponseJSON(ctx, id, &resp); err != nil {
		return nil, err
	}

	candidates := make([]ThreadCandidate, len(resp.Data))
	for i, tid := range resp.Data {
		candidates[i] = ThreadCandidate{
			ID:       tid,
			IsLoaded: true,
			Status:   ThreadStatusIdle,
		}
		// Enrich with details (CWD, preview, status) so loaded-thread
		// CWD matching works correctly for live session auto-select.
		if details, err := c.readThread(ctx, tid); err == nil {
			candidates[i].CWD = details.CWD
			candidates[i].SessionKey = details.SessionKey
			candidates[i].Preview = details.Preview
			candidates[i].Status = details.Status
			candidates[i].CreatedAt = details.CreatedAt
			candidates[i].UpdatedAt = details.UpdatedAt
		}
	}
	return candidates, nil
}

// readThread fetches the details for a single thread by its stable identifier.
// It uses the thread/read JSON-RPC method. When the endpoint is not supported
// by the server, it returns an error gracefully — callers should treat this
// as a best-effort enrichment.
//
// NOTE: readThread does NOT acquire flightMu; the caller must already hold it.
func (c *AppServerClient) readThread(ctx context.Context, threadID string) (ThreadCandidate, error) {
	if !c.hs.IsReady() {
		return ThreadCandidate{}, fmt.Errorf("codex: client not initialized")
	}
	if threadID == "" {
		return ThreadCandidate{}, fmt.Errorf("codex: empty thread id")
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	params := map[string]any{"threadId": threadID}
	id, err := c.sendRequestRaw(ctx, "thread/read", params)
	if err != nil {
		return ThreadCandidate{}, err
	}

	// thread/read returns the thread nested under result.thread, not at the
	// top level of the result object (per Codex app-server API contract).
	var raw struct {
		Thread struct {
			ID        string `json:"id"`
			SessionID string `json:"sessionId"`
			CWD       string `json:"cwd"`
			Preview   string `json:"preview"`
			Status    any    `json:"status"`
			CreatedAt int64  `json:"createdAt"`
			UpdatedAt int64  `json:"updatedAt"`
		} `json:"thread"`
	}
	if err := c.readResponseJSON(ctx, id, &raw); err != nil {
		return ThreadCandidate{}, err
	}

	thr := raw.Thread
	return ThreadCandidate{
		ID:         thr.ID,
		CWD:        thr.CWD,
		SessionKey: thr.SessionID,
		Preview:    thr.Preview,
		IsLoaded:   true,
		Status:     ThreadStatusFromWire(thr.Status),
		CreatedAt:  unixMillisToTime(thr.CreatedAt),
		UpdatedAt:  unixMillisToTime(thr.UpdatedAt),
	}, nil
}

// ListStoredThreads implements StoredThreadLister. It returns a single page
// of stored (not loaded) thread candidates from the app-server's thread/list
// endpoint. When page is empty, the first page is returned.
//
// The returned ThreadPage includes a NextPage token for pagination.
// This method is designed to be used with the SelectThread / collectStoredThreads
// functions from this package.
func (c *AppServerClient) ListStoredThreads(ctx context.Context, page PageToken) (ThreadPage, error) {
	c.flightMu.Lock()
	defer c.flightMu.Unlock()

	if !c.hs.IsReady() {
		return ThreadPage{}, fmt.Errorf("codex: client not initialized")
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	params := map[string]any{}
	if page != "" {
		params["cursor"] = string(page)
	}
	// Limit stored thread results so we don't read forever.
	params["limit"] = 50

	id, err := c.sendRequestRaw(ctx, "thread/list", params)
	if err != nil {
		return ThreadPage{}, err
	}

	var raw struct {
		Data []struct {
			ID        string `json:"id"`
			SessionID string `json:"sessionId"`
			CWD       string `json:"cwd"`
			Preview   string `json:"preview"`
			Status    any    `json:"status"`
			CreatedAt int64  `json:"createdAt"`
			UpdatedAt int64  `json:"updatedAt"`
		} `json:"data"`
		NextCursor *string `json:"nextCursor"`
	}
	if err := c.readResponseJSON(ctx, id, &raw); err != nil {
		return ThreadPage{}, err
	}

	items := make([]ThreadCandidate, 0, len(raw.Data))
	for _, t := range raw.Data {
		status := ThreadStatusFromWire(t.Status)
		cwd := t.CWD
		// The app-server includes cwd for threads started with one.
		// When the field is absent we leave it empty; the selector will
		// treat it as non-matching.
		items = append(items, ThreadCandidate{
			ID:         t.ID,
			CWD:        cwd,
			SessionKey: t.SessionID,
			Preview:    t.Preview,
			IsLoaded:   false,
			Status:     status,
		})
	}

	nextPage := PageToken("")
	if raw.NextCursor != nil && *raw.NextCursor != "" {
		nextPage = PageToken(*raw.NextCursor)
	}

	return ThreadPage{
		Items:    items,
		NextPage: nextPage,
	}, nil
}

// ResumeThread reopens an existing thread by its stable identifier. After a
// successful resume, subsequent turn/start calls append to this thread.
// This method does not wait for the thread/started notification — the caller
// can immediately proceed with turn/start.
func (c *AppServerClient) ResumeThread(ctx context.Context, threadID string) error {
	c.flightMu.Lock()
	defer c.flightMu.Unlock()

	if !c.hs.IsReady() {
		return fmt.Errorf("codex: client not initialized")
	}
	if threadID == "" {
		return fmt.Errorf("codex: empty thread id")
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	params := map[string]any{"threadId": threadID}
	id, err := c.sendRequestRaw(ctx, "thread/resume", params)
	if err != nil {
		return fmt.Errorf("codex: resume thread %s: %w", threadID, err)
	}

	// Read the resume response. Discard any interleaved notifications.
	if err := c.readResponse(ctx, id); err != nil {
		return fmt.Errorf("codex: resume thread %s response: %w", threadID, err)
	}
	return nil
}

// StartThread creates a new Codex thread and returns its stable identifier.
// The cwd parameter sets the thread's working directory. After a successful
// start, the caller can use the returned threadID for turn/start.
func (c *AppServerClient) StartThread(ctx context.Context, cwd string) (string, error) {
	c.flightMu.Lock()
	defer c.flightMu.Unlock()

	if !c.hs.IsReady() {
		return "", fmt.Errorf("codex: client not initialized")
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	params := map[string]any{"cwd": cwd}
	id, err := c.sendRequestRaw(ctx, "thread/start", params)
	if err != nil {
		return "", fmt.Errorf("codex: start thread: %w", err)
	}

	// Read the start response and extract the thread ID.
	var raw struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := c.readResponseJSON(ctx, id, &raw); err != nil {
		return "", fmt.Errorf("codex: start thread response: %w", err)
	}
	if raw.Thread.ID == "" {
		return "", fmt.Errorf("codex: start thread response missing thread id")
	}
	return raw.Thread.ID, nil
}

// PublishMessage sends a user message to an existing thread via turn/start.
// The message is the formatted review text. After the turn is submitted, this
// method reads the immediate turn response and returns the turn ID without
// waiting for the turn to complete.
func (c *AppServerClient) PublishMessage(ctx context.Context, threadID, message string) (string, error) {
	c.flightMu.Lock()
	defer c.flightMu.Unlock()

	if !c.hs.IsReady() {
		return "", fmt.Errorf("codex: client not initialized")
	}
	if threadID == "" {
		return "", fmt.Errorf("codex: empty thread id")
	}
	if message == "" {
		return "", fmt.Errorf("codex: empty publish message")
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	params := map[string]any{
		"threadId": threadID,
		"input": []map[string]string{
			{"type": "text", "text": message},
		},
	}
	id, err := c.sendRequestRaw(ctx, "turn/start", params)
	if err != nil {
		return "", fmt.Errorf("codex: turn/start: %w", err)
	}

	// Read the immediate turn response. The server confirms the turn was
	// accepted before any streaming notifications begin.
	var raw struct {
		Turn struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"turn"`
	}
	if err := c.readResponseJSON(ctx, id, &raw); err != nil {
		return "", fmt.Errorf("codex: turn/start response: %w", err)
	}
	if raw.Turn.ID == "" {
		return "", fmt.Errorf("codex: turn/start response missing turn id")
	}
	return raw.Turn.ID, nil
}

// Close shuts down the transport. For stdio connections, it closes stdin to
// signal EOF, then waits for the process to exit with a 5-second grace period.
// For WebSocket connections, it closes the connection directly.
func (c *AppServerClient) Close() error {
	// Acquire flightMu first to ensure no in-flight round-trip races with
	// transport teardown. The write mutex is also acquired for closed flag.
	c.flightMu.Lock()
	defer c.flightMu.Unlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	// WebSocket connection: close directly.
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}

	// Stdio subprocess: close stdin to signal EOF.
	if c.stdin != nil {
		_ = c.stdin.Close()
		c.stdin = nil
	}

	if c.cmd != nil && c.cmd.Process != nil {
		done := make(chan error, 1)
		go func() { done <- c.cmd.Wait() }()

		select {
		case err := <-done:
			return err
		case <-time.After(5 * time.Second):
			_ = c.cmd.Process.Kill()
			<-done
			return fmt.Errorf("codex: app-server killed after 5s timeout")
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// sendRequest marshals params, sends a JSON-RPC request, and returns the
// allocated request ID. The caller must call readResponse or readResponseJSON
// to consume the matching response.
func (c *AppServerClient) sendRequest(ctx context.Context, method string, params any) error {
	_, err := c.sendRequestRaw(ctx, method, params)
	return err
}

// sendRequestRaw is like sendRequest but also returns the allocated request ID.
func (c *AppServerClient) sendRequestRaw(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, fmt.Errorf("codex: client is closed")
	}

	c.nextID++
	id := json.RawMessage(fmt.Sprintf("%d", c.nextID))

	data, err := EncodeRequest(id, method, params)
	if err != nil {
		return nil, err
	}
	// Append newline for JSONL.
	data = append(data, '\n')

	if err := c.writeLocked(ctx, data); err != nil {
		return nil, err
	}

	return id, nil
}

// readResponse reads one JSON-RPC response matching id from the scanner.
// Notifications are silently discarded. The response Message is stored in msg.
// Returns the JSON-RPC error object when the server responds with an error.
func (c *AppServerClient) readResponse(ctx context.Context, expectedID json.RawMessage) error {
	var msg Message
	return c.doReadResponse(ctx, expectedID, &msg, func() error {
		if msg.Error != nil {
			return msg.Error
		}
		return nil
	})
}

// readResponseJSON is like readResponse but also unmarshals the result into target.
func (c *AppServerClient) readResponseJSON(ctx context.Context, expectedID json.RawMessage, target any) error {
	var msg Message
	return c.doReadResponse(ctx, expectedID, &msg, func() error {
		if msg.Error != nil {
			return msg.Error
		}
		return json.Unmarshal(msg.Result, target)
	})
}

// readMessage reads one complete JSON-RPC message from the transport.
// For stdio (JSONL) this is one newline-delimited line.
// For WebSocket connections this is one complete WebSocket text frame,
// using websocket.Message.Receive which handles continuation frames and
// message reassembly correctly (unlike raw Conn.Read which is frame-level).
// The returned slice is a copy and safe to reuse.
func (c *AppServerClient) readMessage() ([]byte, error) {
	if c.conn != nil {
		// WebSocket: use Message.Receive for safe complete-message assembly.
		// Message.Receive reads until a complete FIN frame is received,
		// handling continuation frames transparently.
		if ws, ok := c.conn.(*websocket.Conn); ok {
			var msgStr string
			if err := websocket.Message.Receive(ws, &msgStr); err != nil {
				return nil, fmt.Errorf("codex: receive websocket message: %w", err)
			}
			if len(msgStr) == 0 {
				return nil, io.ErrUnexpectedEOF
			}
			return []byte(msgStr), nil
		}
		// Fallback for non-websocket conn (should not happen).
		buf := make([]byte, c.maxMsgSize)
		n, err := c.conn.Read(buf)
		if err != nil {
			return nil, fmt.Errorf("codex: read from conn: %w", err)
		}
		if n == 0 {
			return nil, io.ErrUnexpectedEOF
		}
		line := make([]byte, n)
		copy(line, buf[:n])
		return line, nil
	}

	// Stdio (JSONL): read one newline-delimited line.
	if !c.scanner.Scan() {
		err := c.scanner.Err()
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	line := c.scanner.Bytes()
	// scanner.Bytes() is valid only until next Scan(); copy it.
	cp := make([]byte, len(line))
	copy(cp, line)
	return cp, nil
}

// doReadResponse reads messages from the transport until it finds a response
// matching expectedID, skipping notifications. When okFn is non-nil, it is
// called after a successful response match.
//
// Uses the configured per-request timeout (c.timeout) as the read deadline.
// The caller's context cancellation takes precedence. The background read
// goroutine writes its result to a buffered channel so it can complete
// cleanly even after the caller has returned due to timeout.
func (c *AppServerClient) doReadResponse(ctx context.Context, expectedID json.RawMessage, msg *Message, okFn func() error) error {
	readCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	ch := make(chan readResult, 1)

	go func() {
		for {
			line, err := c.readMessage()
			if err != nil {
				ch <- readResult{err: fmt.Errorf("codex: read response: %w", err)}
				return
			}

			if len(line) == 0 {
				continue // skip empty messages
			}

			var m Message
			if err := json.Unmarshal(line, &m); err != nil {
				ch <- readResult{err: fmt.Errorf("codex: decode response line: %w", err)}
				return
			}

			kind := ClassifyMessage(m)
			if kind == MsgNotification {
				continue // discard notifications
			}

			// Must be a response.
			if kind != MsgResponse {
				ch <- readResult{err: fmt.Errorf("codex: expected response, got %s", kind)}
				return
			}

			// Check for ID mismatch.
			if string(m.ID) != string(expectedID) {
				ch <- readResult{err: fmt.Errorf("codex: response id mismatch: got %s, want %s", string(m.ID), string(expectedID))}
				return
			}

			// Capture the full message so caller can inspect result/error.
			*msg = m

			if okFn != nil {
				if err := okFn(); err != nil {
					ch <- readResult{err: err}
					return
				}
			}

			ch <- readResult{}
			return
		}
	}()

	select {
	case <-readCtx.Done():
		return fmt.Errorf("codex: response read timeout: %w", readCtx.Err())
	case r := <-ch:
		return r.err
	}
}

// write writes data to the transport (stdio pipe or WebSocket conn) with
// context support.
func (c *AppServerClient) write(ctx context.Context, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeLocked(ctx, data)
}

// writeLocked writes data to the transport assuming the mutex is already held.
// For WebSocket connections it uses websocket.Message.Send which sends a
// complete text frame (the WebSocket message-level API). For stdio it writes
// directly to the stdin pipe (JSONL).
func (c *AppServerClient) writeLocked(ctx context.Context, data []byte) error {
	ch := make(chan error, 1)

	switch {
	case c.conn != nil:
		if ws, ok := c.conn.(*websocket.Conn); ok {
			go func() {
				// Message.Send with string sends a complete text frame.
				ch <- websocket.Message.Send(ws, string(data))
			}()
		} else {
			go func() {
				_, err := c.conn.Write(data)
				ch <- err
			}()
		}
	case c.stdin != nil:
		go func() {
			_, err := c.stdin.Write(data)
			ch <- err
		}()
	default:
		return fmt.Errorf("codex: no write target (closed?)")
	}

	select {
	case <-ctx.Done():
		return fmt.Errorf("codex: write timeout: %w", ctx.Err())
	case err := <-ch:
		return err
	}
}

// readResult carries the outcome of a background scan.
type readResult struct {
	err error
}

// unixMillisToTime converts a Unix timestamp in milliseconds to time.Time.
func unixMillisToTime(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

// ThreadStatusFromWire converts the app-server's wire-format status field
// (which can be a string like "notLoaded" or an object like {"type":"idle"})
// into a ThreadStatus constant.
func ThreadStatusFromWire(status any) ThreadStatus {
	if status == nil {
		return ""
	}
	switch v := status.(type) {
	case string:
		return ThreadStatusFromString(v)
	case map[string]any:
		if t, ok := v["type"].(string); ok {
			return ThreadStatusFromString(t)
		}
	}
	return ""
}

// Ensure AppServerClient implements StoredThreadLister.
var _ StoredThreadLister = (*AppServerClient)(nil)
