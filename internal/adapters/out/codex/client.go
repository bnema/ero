package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

// providerVersion identifies this adapter to the app-server during
// the initialize handshake.
const providerVersion = "ero.plugin.v1"

// AppServerClient communicates with the codex app-server over a WebSocket
// live-session connection. After the initialize handshake, the client is
// ready for turn/start requests via PublishMessage.
//
// Each public method serialises its full request/response round-trip via an
// internal flight mutex so that concurrent calls are serialised. After a
// context/timeout error the transport state is indeterminate and the caller
// must discard the client (call Close).
type AppServerClient struct {
	cfg      Config
	conn     io.ReadWriteCloser
	hs       Handshake
	mu       sync.Mutex // write + nextID serialisation
	flightMu sync.Mutex // serialises full round-trips
	nextID   int
	closed   bool
	timeout  time.Duration

	// maxMsgSize is the maximum expected JSON-RPC message size.
	// Used for buffer allocation in the non-WebSocket read path.
	maxMsgSize int
}

// NewAppServerClient connects to a running codex app-server via its unix
// control socket (WebSocket) and returns a client ready for the initialize
// handshake. The config must have SocketPath and ThreadID set — call
// Config.ValidateCallbackTarget to verify.
//
// Unlike the previous multi-mode implementation, this simplified version
// always dials a live session and does not support stdio subprocess fallback
// or transport-mode selection.
func NewAppServerClient(ctx context.Context, cfg Config) (*AppServerClient, error) {
	if err := cfg.ValidateCallbackTarget(); err != nil {
		return nil, fmt.Errorf("codex: %w", err)
	}
	client, err := DialLiveSession(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("codex: connect to live session: %w", err)
	}
	return client, nil
}

// Initialize performs the JSON-RPC initialize handshake with the app-server.
// It sends an initialize request, awaits the response, then sends the
// initialized notification. After Initialize returns successfully, the client
// is ready for turn/start requests.
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

	// Step 4: send initialized notification.
	notifData, err := EncodeNotification(MethodInitialized, nil)
	if err != nil {
		return fmt.Errorf("codex: marshal initialized notification: %w", err)
	}
	if err := c.write(ctx, notifData); err != nil {
		return fmt.Errorf("codex: send initialized notification: %w", err)
	}

	if err := c.hs.OnInitializedSent(); err != nil {
		return err
	}
	return nil
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

// Close shuts down the WebSocket connection.
func (c *AppServerClient) Close() error {
	c.flightMu.Lock()
	defer c.flightMu.Unlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// sendRequestRaw marshals params, sends a JSON-RPC request, and returns the
// allocated request ID. The caller must call readResponse or readResponseJSON
// to consume the matching response.
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

	if err := c.writeLocked(ctx, data); err != nil {
		return nil, err
	}

	return id, nil
}

// readResponse reads one JSON-RPC response matching id from the transport.
// Notifications are silently discarded. Returns the JSON-RPC error object
// when the server responds with an error.
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

// readTransport is a stable snapshot of the active read side. Background read
// goroutines must use this instead of re-reading AppServerClient fields because
// cancellation/timeout aborts can concurrently nil those fields.
type readTransport struct {
	conn       io.ReadWriteCloser
	maxMsgSize int
}

// snapshotReadTransportLocked captures the active read side. c.mu must be held.
func (c *AppServerClient) snapshotReadTransportLocked() (readTransport, error) {
	if c.closed {
		return readTransport{}, fmt.Errorf("codex: client is closed")
	}
	if c.conn != nil {
		return readTransport{conn: c.conn, maxMsgSize: c.maxMsgSize}, nil
	}
	return readTransport{}, fmt.Errorf("codex: no read target (closed?)")
}

// readMessage reads one complete JSON-RPC message from the transport.
// For WebSocket connections, it uses websocket.Message.Receive which handles
// continuation frames and message reassembly correctly (unlike raw Conn.Read
// which is frame-level). For non-WebSocket connections (test pipes), it
// performs a single read.
// The returned slice is a copy and safe to reuse.
func (c *AppServerClient) readMessage() ([]byte, error) {
	c.mu.Lock()
	transport, err := c.snapshotReadTransportLocked()
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return transport.readMessage()
}

func (t readTransport) readMessage() ([]byte, error) {
	if ws, ok := t.conn.(*websocket.Conn); ok {
		var msgStr string
		if err := websocket.Message.Receive(ws, &msgStr); err != nil {
			return nil, fmt.Errorf("codex: receive websocket message: %w", err)
		}
		if len(msgStr) == 0 {
			return nil, io.ErrUnexpectedEOF
		}
		return []byte(msgStr), nil
	}
	// Fallback for non-websocket conn (test pipes).
	buf := make([]byte, t.maxMsgSize)
	n, err := t.conn.Read(buf)
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

// doReadResponse reads messages from the transport until it finds a response
// matching expectedID, skipping notifications. When okFn is non-nil, it is
// called after a successful response match.
//
// Uses the configured per-request timeout (c.timeout) as the read deadline.
// The caller's context cancellation takes precedence. If the deadline fires,
// the transport is closed before returning: a timed-out read leaves the stream
// position indeterminate, so the client is no longer safe to reuse.
func (c *AppServerClient) doReadResponse(ctx context.Context, expectedID json.RawMessage, msg *Message, okFn func() error) error {
	readCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	c.mu.Lock()
	transport, err := c.snapshotReadTransportLocked()
	c.mu.Unlock()
	if err != nil {
		return err
	}

	ch := make(chan readResult, 1)

	go func() {
		for {
			line, err := transport.readMessage()
			if err != nil {
				ch <- readResult{err: fmt.Errorf("codex: read response: %w", err)}
				return
			}

			if len(line) == 0 {
				continue
			}

			var m Message
			if err := json.Unmarshal(line, &m); err != nil {
				ch <- readResult{err: fmt.Errorf("codex: decode response line: %w", err)}
				return
			}

			kind := ClassifyMessage(m)
			if kind == MsgNotification {
				continue
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
		c.abortTransport()
		return fmt.Errorf("codex: response read timeout: %w", readCtx.Err())
	case r := <-ch:
		return r.err
	}
}

// abortTransport closes the underlying connection after a read timeout or
// cancellation without taking flightMu. Public methods already hold flightMu
// while waiting for responses, and the client must become terminal because the
// response stream may be partially consumed.
func (c *AppServerClient) abortTransport() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.abortTransportLocked()
}

// abortTransportLocked closes the underlying connection and marks the client
// terminal. c.mu must be held.
func (c *AppServerClient) abortTransportLocked() {
	if c.closed {
		return
	}
	c.closed = true

	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}

// write writes data to the transport (WebSocket or test pipe) with
// context support.
func (c *AppServerClient) write(ctx context.Context, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeLocked(ctx, data)
}

// writeLocked writes data to the transport assuming the mutex is already held.
// For WebSocket connections it uses websocket.Message.Send which sends a
// complete text frame (the WebSocket message-level API). For test pipes it
// writes directly. If the write is canceled, the transport is closed before
// returning because stream state is no longer safe to reuse.
func (c *AppServerClient) writeLocked(ctx context.Context, data []byte) error {
	ch := make(chan error, 1)

	conn := c.conn
	if ws, ok := conn.(*websocket.Conn); ok {
		go func() {
			// Message.Send with string sends a complete text frame.
			ch <- websocket.Message.Send(ws, string(data))
		}()
	} else {
		go func() {
			_, err := conn.Write(data)
			ch <- err
		}()
	}

	select {
	case <-ctx.Done():
		c.abortTransportLocked()
		return fmt.Errorf("codex: write timeout: %w", ctx.Err())
	case err := <-ch:
		return err
	}
}

// readResult carries the outcome of a background read.
type readResult struct {
	err error
}
