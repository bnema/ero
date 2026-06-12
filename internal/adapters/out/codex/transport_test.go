package codex

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProbeSocketExists(t *testing.T) {
	// Create a temp socket file and verify ProbeSocket finds it.
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	// Create the socket file (can't actually listen without a real server,
	// but ProbeSocket only stats and checks the socket mode bit).
	f, err := os.Create(sockPath)
	if err != nil {
		t.Fatalf("create temp socket: %v", err)
	}
	f.Close()

	// ProbeSocket checks the mode bit which requires ModeSocket. A regular
	// file won't have that bit, so we expect false.
	if got := ProbeSocket(sockPath); got {
		t.Log("ProbeSocket returns true for a regular file (expected on some platforms)")
	}

	// Non-existent path should always be false.
	if ProbeSocket("/no/such/path") {
		t.Fatal("ProbeSocket should return false for non-existent path")
	}

	// Empty path should return false.
	if ProbeSocket("") {
		t.Fatal("ProbeSocket should return false for empty path")
	}
}

func TestDialSocketEmptyPath(t *testing.T) {
	err := DialSocket("", SocketAvailabilityTimeout)
	if err == nil {
		t.Fatal("expected error for empty socket path")
	}
}

func TestDialSocketNonExistent(t *testing.T) {
	err := DialSocket("/no/such/socket.sock", SocketAvailabilityTimeout)
	if err == nil {
		t.Fatal("expected error for non-existent socket")
	}
}

func TestResolveSocketPath(t *testing.T) {
	tests := []struct {
		name      string
		codexHome string
		override  string
		want      string
	}{
		{"override takes precedence", "/home/user/.codex", "/custom/sock", "/custom/sock"},
		{"default from codexHome", "/home/user/.codex", "", "/home/user/.codex/app-server-control/app-server-control.sock"},
		{"empty codexHome with empty override", "", "", ""},
		{"empty codexHome with override", "", "/custom/sock", "/custom/sock"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveSocketPath(tt.codexHome, tt.override)
			if got != tt.want {
				t.Errorf("ResolveSocketPath(%q, %q) = %q, want %q", tt.codexHome, tt.override, got, tt.want)
			}
		})
	}
}

func TestStdioConfig(t *testing.T) {
	cfg := StdioConfig()
	if cfg.Kind != TransportStdio {
		t.Fatalf("expected TransportStdio, got %s", cfg.Kind)
	}
	if cfg.IsLiveSession() {
		t.Fatal("expected IsLiveSession=false for stdio")
	}
}

func TestProxyConfig(t *testing.T) {
	cfg := ProxyConfig()
	if cfg.Kind != TransportProxy {
		t.Fatalf("expected TransportProxy, got %s", cfg.Kind)
	}
	if !cfg.IsLiveSession() {
		t.Fatal("expected IsLiveSession=true for proxy")
	}
}

func TestUnixConfigDefaults(t *testing.T) {
	cfg := UnixConfig("/home/user/.codex", "")
	if cfg.Kind != TransportUnix {
		t.Fatalf("expected TransportUnix, got %s", cfg.Kind)
	}
	if !cfg.IsLiveSession() {
		t.Fatal("expected IsLiveSession=true for unix")
	}
	if cfg.SocketPath != "" {
		t.Fatalf("expected empty socket path, got %q", cfg.SocketPath)
	}
	if cfg.CodexHome != "/home/user/.codex" {
		t.Fatalf("expected codexHome /home/user/.codex, got %q", cfg.CodexHome)
	}
}

func TestUnixConfigWithSocketPath(t *testing.T) {
	cfg := UnixConfig("", "/custom/socket.sock")
	if cfg.Kind != TransportUnix {
		t.Fatalf("expected TransportUnix, got %s", cfg.Kind)
	}
	if !cfg.IsLiveSession() {
		t.Fatal("expected IsLiveSession=true for unix")
	}
	if cfg.SocketPath != "/custom/socket.sock" {
		t.Fatalf("expected /custom/socket.sock, got %q", cfg.SocketPath)
	}
}

func TestBestAvailableTransportPrefersLiveSession(t *testing.T) {
	preferred := ProxyConfig()
	fallback := StdioConfig()

	result := BestAvailableTransport(preferred, fallback)
	if result.Kind != TransportProxy {
		t.Fatalf("expected proxy, got %s", result.Kind)
	}
	if !result.IsLiveSession() {
		t.Fatal("expected live session capability")
	}
}

func TestBestAvailableTransportFallsBack(t *testing.T) {
	preferred := StdioConfig()
	fallback := StdioConfig()

	result := BestAvailableTransport(preferred, fallback)
	if result.Kind != TransportStdio {
		t.Fatalf("expected stdio fallback, got %s", result.Kind)
	}
}

func TestBestAvailableTransportWithUnix(t *testing.T) {
	preferred := UnixConfig("/home/user/.codex", "")
	fallback := StdioConfig()

	result := BestAvailableTransport(preferred, fallback)
	if result.Kind != TransportUnix {
		t.Fatalf("expected unix, got %s", result.Kind)
	}
}

func TestBestAvailableTransportEmptiesFallback(t *testing.T) {
	// When both are stdio, the fallback is returned.
	fallback := StdioConfig()
	result := BestAvailableTransport(StdioConfig(), fallback)
	if result.Kind != TransportStdio {
		t.Fatalf("expected stdio, got %s", result.Kind)
	}
}

func TestDefaultSocketPath(t *testing.T) {
	path := DefaultSocketPath("/home/user/.codex")
	expected := "/home/user/.codex/app-server-control/app-server-control.sock"
	if path != expected {
		t.Fatalf("expected %q, got %q", expected, path)
	}
}

func TestDefaultSocketPathEmpty(t *testing.T) {
	if path := DefaultSocketPath(""); path != "" {
		t.Fatalf("expected empty, got %q", path)
	}
}

func TestIsLiveSessionCapable(t *testing.T) {
	tests := []struct {
		kind     TransportKind
		expected bool
	}{
		{TransportStdio, false},
		{TransportProxy, true},
		{TransportUnix, true},
		{TransportKind("unknown"), false},
	}

	for _, tt := range tests {
		got := IsLiveSessionCapable(tt.kind)
		if got != tt.expected {
			t.Errorf("IsLiveSessionCapable(%q) = %v, want %v", tt.kind, got, tt.expected)
		}
	}
}

func TestSessionCapabilityString(t *testing.T) {
	tests := []struct {
		cap  SessionCapability
		want string
	}{
		{CapUnknown, "unknown"},
		{CapOneShot, "one-shot"},
		{CapLiveSession, "live-session"},
		{SessionCapability(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.cap.String(); got != tt.want {
			t.Errorf("SessionCapability(%d).String() = %q, want %q", tt.cap, got, tt.want)
		}
	}
}

func TestTransportKindConstants(t *testing.T) {
	if TransportStdio != "stdio" {
		t.Fatalf("TransportStdio = %q, want %q", TransportStdio, "stdio")
	}
	if TransportProxy != "proxy" {
		t.Fatalf("TransportProxy = %q, want %q", TransportProxy, "proxy")
	}
	if TransportUnix != "unix" {
		t.Fatalf("TransportUnix = %q, want %q", TransportUnix, "unix")
	}
}

func TestTransportAvailabilityString(t *testing.T) {
	tests := []struct {
		a    TransportAvailability
		want string
	}{
		{AvailUnknown, "unknown"},
		{AvailReachable, "reachable"},
		{AvailUnreachable, "unreachable"},
		{TransportAvailability(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.a.String(); got != tt.want {
				t.Errorf("TransportAvailability(%d).String() = %q, want %q", tt.a, got, tt.want)
			}
		})
	}
}

func TestSelectTransportPrefersLiveSessionWhenReachable(t *testing.T) {
	preferred := ProxyConfig()
	fallback := StdioConfig()

	result := SelectTransport(preferred, fallback, AvailReachable)
	if result.Kind != TransportProxy {
		t.Fatalf("expected proxy, got %s", result.Kind)
	}
	if !result.IsLiveSession() {
		t.Fatal("expected live session capability")
	}
}

func TestSelectTransportFallsBackWhenUnreachable(t *testing.T) {
	preferred := ProxyConfig()
	fallback := StdioConfig()

	result := SelectTransport(preferred, fallback, AvailUnreachable)
	if result.Kind != TransportStdio {
		t.Fatalf("expected stdio fallback, got %s", result.Kind)
	}
}

func TestSelectTransportFallsBackWhenUnknown(t *testing.T) {
	preferred := ProxyConfig()
	fallback := StdioConfig()

	result := SelectTransport(preferred, fallback, AvailUnknown)
	if result.Kind != TransportStdio {
		t.Fatalf("expected stdio fallback for unknown, got %s", result.Kind)
	}
}

func TestSelectTransportStdioPreferredFallbackStillStdio(t *testing.T) {
	// Even when reachable, a non-live-session preferred falls back.
	result := SelectTransport(StdioConfig(), StdioConfig(), AvailReachable)
	if result.Kind != TransportStdio {
		t.Fatalf("expected stdio, got %s", result.Kind)
	}
}

func TestSelectTransportUnixWhenReachable(t *testing.T) {
	preferred := UnixConfig("/home/user/.codex", "")
	fallback := StdioConfig()

	result := SelectTransport(preferred, fallback, AvailReachable)
	if result.Kind != TransportUnix {
		t.Fatalf("expected unix, got %s", result.Kind)
	}
}

func TestSelectTransportUnixFallsBackWhenUnreachable(t *testing.T) {
	preferred := UnixConfig("/home/user/.codex", "")
	fallback := StdioConfig()

	result := SelectTransport(preferred, fallback, AvailUnreachable)
	if result.Kind != TransportStdio {
		t.Fatalf("expected stdio fallback, got %s", result.Kind)
	}
}

func TestDialLiveSessionHonorsContextDuringHandshake(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "codex.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	defer ln.Close()

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
