package codex

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDialLiveSessionHonorsContextDuringHandshake(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "codex.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	t.Cleanup(func() {
		if err := ln.Close(); err != nil {
			t.Errorf("close listener: %v", err)
		}
	})

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err = DialLiveSession(ctx, Config{SocketPath: sockPath, CommandTimeout: time.Second})
	if err == nil {
		t.Fatal("DialLiveSession should fail when context expires during handshake")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("DialLiveSession error = %v, want context deadline exceeded", err)
	}

	select {
	case conn := <-accepted:
		_ = conn.Close()
	default:
	}
}
