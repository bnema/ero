package codex

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Timeout and cancellation tests
// ---------------------------------------------------------------------------

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

	if _, err := client.sendRequestRaw(context.Background(), "turn/start", map[string]any{}); err == nil {
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
		errCh <- client.write(ctx, []byte(`{"id":1,"method":"turn/start"}`))
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

	if _, err := client.sendRequestRaw(context.Background(), "turn/start", map[string]any{}); err == nil {
		t.Fatal("client should be terminal after write cancellation")
	}
}
