package codex

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

// startTestWSEchoServer starts a minimal WebSocket echo server on a unix
// socket. It uses the standard HTTP server with websocket.Server to accept
// WebSocket upgrades, then echoes any text message back.
// The returned listener is ready to accept connections. The caller must close
// the listener when done (typically via defer).
func startTestWSEchoServer(t *testing.T, socketPath string) net.Listener {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(socketPath), 0755); err != nil {
		t.Fatalf("mkdir socket dir: %v", err)
	}
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove stale socket: %v", err)
	}

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}

	// WebSocket echo handler.
	wsHandler := websocket.Server{
		Handler: func(ws *websocket.Conn) {
			ws.PayloadType = websocket.TextFrame
			ws.MaxPayloadBytes = 2 * 1024 * 1024
			for {
				var msg string
				if err := websocket.Message.Receive(ws, &msg); err != nil {
					return
				}
				if err := websocket.Message.Send(ws, msg); err != nil {
					return
				}
			}
		},
		Handshake: func(cfg *websocket.Config, req *http.Request) error {
			return nil // accept all origins
		},
	}

	// Start HTTP server that handles WebSocket upgrades.
	go func() {
		if err := http.Serve(ln, wsHandler); err != nil && err != http.ErrServerClosed {
			t.Logf("echo server error: %v", err)
		}
	}()

	return ln
}

// TestWebSocketMessageRoundTrip verifies that the corrected readMessage and
// writeLocked paths work correctly with websocket.Message.Receive/Send.
// A real WebSocket server over a unix socket echoes messages back,
// exercising the exact same code paths used by DialLiveSession.
func TestWebSocketMessageRoundTrip(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test-ws.sock")

	ln := startTestWSEchoServer(t, sockPath)
	t.Cleanup(func() {
		if err := ln.Close(); err != nil {
			t.Errorf("close listener: %v", err)
		}
	})
	// Give the server a moment to start listening.
	time.Sleep(20 * time.Millisecond)

	// Connect as a WebSocket client (same approach as DialLiveSession).
	rawConn, err := net.DialTimeout("unix", sockPath, 5*time.Second)
	if err != nil {
		t.Fatalf("dial unix: %v", err)
	}
	t.Cleanup(func() {
		if err := rawConn.Close(); err != nil {
			t.Errorf("close raw conn: %v", err)
		}
	})

	config, err := websocket.NewConfig("ws://localhost/", "http://localhost/")
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	ws, err := websocket.NewClient(config, rawConn)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	ws.PayloadType = websocket.TextFrame
	ws.MaxPayloadBytes = 2 * 1024 * 1024

	// Create an AppServerClient with the WebSocket connection (same setup
	// as DialLiveSession uses).
	client := &AppServerClient{
		cfg:        Config{CommandTimeout: 5 * time.Second},
		conn:       ws,
		timeout:    5 * time.Second,
		maxMsgSize: 2 * 1024 * 1024,
	}

	// Send a JSON-RPC request via writeLocked (uses Message.Send internally).
	request := `{"id":1,"method":"initialize","params":{"protocolVersion":"2.0"}}`
	ctx := context.Background()
	if err := client.write(ctx, []byte(request)); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Read the echoed response via readMessage (uses Message.Receive internally).
	response, err := client.readMessage()
	if err != nil {
		t.Fatalf("readMessage: %v", err)
	}
	if string(response) != request {
		t.Errorf("echo mismatch:\n  sent:    %s\n  received: %s", request, string(response))
	}

	// Send a second message to verify the connection is still usable.
	request2 := `{"id":2,"method":"thread/list","params":{}}`
	if err := client.write(ctx, []byte(request2)); err != nil {
		t.Fatalf("write2: %v", err)
	}
	response2, err := client.readMessage()
	if err != nil {
		t.Fatalf("readMessage2: %v", err)
	}
	if string(response2) != request2 {
		t.Errorf("echo mismatch on second message:\n  sent:    %s\n  received: %s", request2, string(response2))
	}
}

// TestWebSocketLargeMessage verifies that messages near the max payload size
// are correctly handled by the readMessage path with websocket.Message.Receive.
func TestWebSocketLargeMessage(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test-large.sock")

	ln := startTestWSEchoServer(t, sockPath)
	t.Cleanup(func() {
		if err := ln.Close(); err != nil {
			t.Errorf("close listener: %v", err)
		}
	})
	time.Sleep(20 * time.Millisecond)

	rawConn, err := net.DialTimeout("unix", sockPath, 5*time.Second)
	if err != nil {
		t.Fatalf("dial unix: %v", err)
	}
	t.Cleanup(func() {
		if err := rawConn.Close(); err != nil {
			t.Errorf("close raw conn: %v", err)
		}
	})

	config, err := websocket.NewConfig("ws://localhost/", "http://localhost/")
	if err != nil {
		t.Fatalf("new config: %v", err)
	}
	ws, err := websocket.NewClient(config, rawConn)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	ws.PayloadType = websocket.TextFrame
	ws.MaxPayloadBytes = 2 * 1024 * 1024

	client := &AppServerClient{
		cfg:        Config{CommandTimeout: 5 * time.Second},
		conn:       ws,
		timeout:    5 * time.Second,
		maxMsgSize: 2 * 1024 * 1024,
	}

	// Build a message with 50KB of JSON data.
	var b strings.Builder
	b.WriteString(`{"id":1,"method":"test","params":{"data":"`)
	for i := 0; i < 50*1024; i++ {
		b.WriteByte('x')
	}
	b.WriteString(`"}}`)

	largeMsg := b.String()
	ctx := context.Background()

	if err := client.write(ctx, []byte(largeMsg)); err != nil {
		t.Fatalf("write large message: %v", err)
	}
	response, err := client.readMessage()
	if err != nil {
		t.Fatalf("readMessage large: %v", err)
	}
	if string(response) != largeMsg {
		t.Fatalf("large message echo mismatch: len sent=%d, len recv=%d", len(largeMsg), len(response))
	}
}

// TestWebSocketServerWebSocketURL verifies that the WebSocket URL used in
// DialLiveSession is valid and accepted by the x/net/websocket library.
func TestWebSocketServerWebSocketURL(t *testing.T) {
	_, err := websocket.NewConfig("ws://localhost/", "http://localhost/")
	if err != nil {
		t.Fatalf("NewConfig with ws://localhost/: %v", err)
	}
}
