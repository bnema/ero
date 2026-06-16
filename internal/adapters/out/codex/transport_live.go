package codex

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

// maxLiveMsgSize is the maximum expected JSON-RPC message size for the
// WebSocket live-session path (2 MiB, matching the stdio scanner limit).
const maxLiveMsgSize = 2 * 1024 * 1024

// DialLiveSession connects to an already-running codex app-server via its
// unix control socket and returns an AppServerClient ready for the initialize
// handshake. The underlying transport is WebSocket over the unix socket.
//
// The dial and WebSocket HTTP Upgrade handshake are bounded by cfg's
// effective timeout and ctx cancellation. After a successful handshake the
// connection deadline is cleared; subsequent reads/writes use per-method
// timeouts set by the caller.
//
// Callers should check cfg.EffectiveSocketPath() for the socket path.
// The socket path is taken directly from cfg.SocketPath (via
// EffectiveSocketPath).
func DialLiveSession(ctx context.Context, cfg Config) (*AppServerClient, error) {
	socketPath := cfg.EffectiveSocketPath()
	if socketPath == "" {
		return nil, fmt.Errorf("codex: no socket path for live session")
	}

	timeout := cfg.EffectiveTimeout()

	// Dial the unix socket with caller cancellation and the configured timeout.
	dialCtx, dialCancel := context.WithTimeout(ctx, timeout)
	defer dialCancel()
	rawConn, err := (&net.Dialer{}).DialContext(dialCtx, "unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("codex: dial unix socket %s: %w", socketPath, err)
	}

	// Set a read/write deadline on the raw connection before the WebSocket
	// HTTP Upgrade handshake. Use the earlier of the caller's deadline and the
	// configured timeout; also close the socket if a caller without a deadline
	// cancels while NewClient is blocked in the handshake.
	deadline := time.Now().Add(timeout)
	callerDeadline, hasCallerDeadline := ctx.Deadline()
	if hasCallerDeadline && callerDeadline.Before(deadline) {
		deadline = callerDeadline
	}
	if err := rawConn.SetDeadline(deadline); err != nil {
		_ = rawConn.Close()
		return nil, fmt.Errorf("codex: set deadline on socket: %w", err)
	}

	var closeOnce sync.Once
	closeRaw := func() { closeOnce.Do(func() { _ = rawConn.Close() }) }
	stopWatch := make(chan struct{})
	doneWatch := make(chan struct{})
	go func() {
		defer close(doneWatch)
		select {
		case <-ctx.Done():
			closeRaw()
		case <-stopWatch:
		}
	}()

	// Create a WebSocket client over the raw socket connection.
	// The origin URL is required but not validated for local sockets.
	config, err := websocket.NewConfig("ws://localhost/", "http://localhost/")
	if err != nil {
		close(stopWatch)
		<-doneWatch
		closeRaw()
		return nil, fmt.Errorf("codex: websocket config: %w", err)
	}

	ws, err := websocket.NewClient(config, rawConn)
	close(stopWatch)
	<-doneWatch
	if err != nil {
		closeRaw()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("codex: websocket handshake over %s: %w", socketPath, ctxErr)
		}
		if hasCallerDeadline && !time.Now().Before(callerDeadline) {
			return nil, fmt.Errorf("codex: websocket handshake over %s: %w", socketPath, context.DeadlineExceeded)
		}
		return nil, fmt.Errorf("codex: websocket handshake over %s: %w", socketPath, err)
	}

	// Clear the deadline now that the handshake is complete. Subsequent
	// operations use per-method timeouts via c.timeout / the caller's ctx.
	if err := rawConn.SetDeadline(time.Time{}); err != nil {
		_ = ws.Close()
		return nil, fmt.Errorf("codex: clear deadline on socket: %w", err)
	}

	// Configure for text frames (JSON-RPC messages).
	// The MaxPayloadBytes limit prevents unbounded memory from large frames.
	ws.PayloadType = websocket.TextFrame
	ws.MaxPayloadBytes = maxLiveMsgSize

	return &AppServerClient{
		cfg:        cfg,
		conn:       ws,
		hs:         Handshake{},
		timeout:    timeout,
		maxMsgSize: maxLiveMsgSize,
	}, nil
}

// SocketExists checks whether the given path exists as a Unix socket.
// It is a lightweight OS-level check used by the provider's DetectContext
// for fast failure before attempting a full dial to the Codex app-server.
func SocketExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSocket != 0
}
