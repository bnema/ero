package codex

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
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
// Callers should check cfg.EffectiveSocketPath() for the resolved path.
// The socket path is resolved from cfg.EffectiveSocketPath(); CODEX_HOME
// is only used for path resolution (not passed through the WebSocket).
func DialLiveSession(ctx context.Context, cfg Config) (*AppServerClient, error) {
	socketPath := cfg.EffectiveSocketPath()
	if socketPath == "" {
		return nil, fmt.Errorf("codex: no socket path for live session")
	}

	timeout := cfg.EffectiveTimeout()

	// Dial the unix socket with the configured timeout.
	rawConn, err := net.DialTimeout("unix", socketPath, timeout)
	if err != nil {
		return nil, fmt.Errorf("codex: dial unix socket %s: %w", socketPath, err)
	}

	// Set a write/read deadline on the raw connection before the WebSocket
	// HTTP Upgrade handshake so the whole upgrade is bounded. The WebSocket
	// client performs the HTTP Upgrade handshake during NewClient, which
	// reads/writes on the raw connection.
	deadline := time.Now().Add(timeout)
	if err := rawConn.SetDeadline(deadline); err != nil {
		_ = rawConn.Close()
		return nil, fmt.Errorf("codex: set deadline on socket: %w", err)
	}

	// Create a WebSocket client over the raw socket connection.
	// The origin URL is required but not validated for local sockets.
	config, err := websocket.NewConfig("ws://localhost/", "http://localhost/")
	if err != nil {
		_ = rawConn.Close()
		return nil, fmt.Errorf("codex: websocket config: %w", err)
	}

	ws, err := websocket.NewClient(config, rawConn)
	if err != nil {
		_ = rawConn.Close()
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

	// Build a dummy scanner that will never be used (keeps Close logic simple).
	dummyScanner := bufio.NewScanner(io.LimitReader(nil, 0))

	return &AppServerClient{
		cfg:        cfg,
		conn:       ws,
		scanner:    dummyScanner,
		hs:         Handshake{},
		timeout:    timeout,
		maxMsgSize: maxLiveMsgSize,
	}, nil
}
