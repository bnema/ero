// Package codex provides pure Go types, helpers, and contracts for
// integrating with the Codex app-server JSON-RPC surface. It is the contract
// foundation for the bundled Codex review provider.
//
// Transport rule of thumb: prefer a live-session capable transport (direct
// unix-websocket — also called "live" mode, with "proxy" as a backward-
// compatible alias) when one is available; fall back to --stdio JSONL for
// stored-thread resume or new-thread delivery when no live control-plane
// connection is reachable.
//
// The types in this package carry no I/O dependencies so integration tests
// can substitute any transport implementation.
package codex

import (
	"fmt"
	"net"
	"os"
	"time"
)

// TransportKind identifies the mechanism for communicating with codex app-server.
type TransportKind string

const (
	// TransportStdio represents codex app-server --stdio mode. Messages are
	// JSONL (newline-delimited JSON) on the subprocess stdin/stdout. Suitable
	// for one-shot request/response patterns such as thread resume or
	// single-turn delivery.
	TransportStdio TransportKind = "stdio"

	// TransportProxy is a backward-compatible alias name for the live
	// transport that connects directly to the app-server unix control socket.
	// The actual implementation is a direct unix-websocket connection (see
	// TransportModeLive / DialLiveSession), not a spawned proxy subprocess.
	// Live-session capable.
	TransportProxy TransportKind = "proxy"

	// TransportUnix represents direct connectivity over the app-server unix
	// socket ($CODEX_HOME/app-server-control/app-server-control.sock by
	// default). Messages are WebSocket frames. Live-session capable.
	TransportUnix TransportKind = "unix"
)

// SessionCapability describes whether a transport can maintain a persistent
// interactive session with the app-server.
type SessionCapability int

const (
	// CapUnknown is the zero value; treat as one-shot.
	CapUnknown SessionCapability = iota
	// CapOneShot transports are suitable for single request/response exchanges.
	// The connection is ephemeral and typically backed by a child process.
	CapOneShot
	// CapLiveSession transports can maintain a persistent connection with
	// bidirectional streaming, subscriptions, and long-running turn/item
	// notifications.
	CapLiveSession
)

// String returns a human-readable label for the capability.
func (c SessionCapability) String() string {
	switch c {
	case CapOneShot:
		return "one-shot"
	case CapLiveSession:
		return "live-session"
	default:
		return "unknown"
	}
}

// TransportConfig describes how to connect to a codex app-server instance.
// A zero-value TransportConfig is invalid — use one of the constructor
// helpers (StdioConfig, ProxyConfig, UnixConfig).
type TransportConfig struct {
	// Kind identifies the transport mechanism.
	Kind TransportKind

	// SocketPath is set only for TransportUnix. It holds the unix socket path.
	SocketPath string

	// CodexHome is the resolved CODEX_HOME directory, used for locating the
	// default control-plane socket and other app-server state. When empty,
	// helpers default to $CODEX_HOME or the platform default.
	CodexHome string
}

// IsLiveSession returns true when the transport can maintain a persistent
// interactive session. Capability is derived from Kind so that
// Kind and the capability check are always consistent.
func (t TransportConfig) IsLiveSession() bool {
	return IsLiveSessionCapable(t.Kind)
}

// StdioConfig returns a TransportConfig for --stdio JSONL mode. This is a
// one-shot transport suitable for thread resume and single-turn delivery.
func StdioConfig() TransportConfig {
	return TransportConfig{
		Kind: TransportStdio,
	}
}

// ProxyConfig returns a TransportConfig for live-session mode (backward-
// compatible name). The transport is a direct unix-websocket connection to
// the app-server control socket, not a spawned proxy subprocess.
func ProxyConfig() TransportConfig {
	return TransportConfig{
		Kind: TransportProxy,
	}
}

// UnixConfig returns a TransportConfig for direct unix socket connectivity.
// When socketPath is empty the helper looks up the default path under
// codexHome/app-server-control/.
func UnixConfig(codexHome, socketPath string) TransportConfig {
	return TransportConfig{
		Kind:       TransportUnix,
		SocketPath: socketPath,
		CodexHome:  codexHome,
	}
}

// BestAvailableTransport applies the fallback rules and assumes the
// preferred transport is always reachable:
//  1. If a live-session capable config (proxy or unix) is provided, return it.
//  2. Otherwise return the stdio fallback.
//
// When the caller has reachability information (socket stat, proxy probe),
// prefer SelectTransport which makes the availability check explicit.
func BestAvailableTransport(preferred, fallback TransportConfig) TransportConfig {
	if preferred.IsLiveSession() {
		return preferred
	}
	return fallback
}

// DefaultSocketPath returns the default app-server control-plane socket path
// under codexHome. Returns empty string when codexHome is empty.
func DefaultSocketPath(codexHome string) string {
	if codexHome == "" {
		return ""
	}
	return codexHome + "/app-server-control/app-server-control.sock"
}

// ProbeSocket checks whether the app-server control socket is reachable at
// the given path. Returns true when the socket exists and can be connected to.
// The check is a simple stat + socket mode bit test (no actual connection).
func ProbeSocket(socketPath string) bool {
	if socketPath == "" {
		return false
	}
	fi, err := os.Stat(socketPath)
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeSocket != 0
}

// DialSocket attempts a brief TCP-style dial to the unix socket at the given
// path to verify it is actually accepting connections (not just stale).
// Returns nil when the socket accepts the connection (immediately closed).
func DialSocket(socketPath string, timeout time.Duration) error {
	if socketPath == "" {
		return fmt.Errorf("codex: empty socket path")
	}
	conn, err := net.DialTimeout("unix", socketPath, timeout)
	if err != nil {
		return fmt.Errorf("codex: socket %s: %w", socketPath, err)
	}
	_ = conn.Close()
	return nil
}

// SocketAvailabilityTimeout is the default timeout for DialSocket.
const SocketAvailabilityTimeout = 500 * time.Millisecond

// ResolveSocketPath returns the effective socket path for the given codexHome
// and optional override. When override is non-empty it takes precedence.
// Otherwise the default path under codexHome is returned.
func ResolveSocketPath(codexHome, override string) string {
	if override != "" {
		return override
	}
	return DefaultSocketPath(codexHome)
}

// IsLiveSessionCapable returns true when the transport can maintain a
// persistent interactive session.
func IsLiveSessionCapable(kind TransportKind) bool {
	switch kind {
	case TransportProxy, TransportUnix:
		return true
	default:
		return false
	}
}

// TransportAvailability describes whether a transport endpoint is reachable
// and usable for establishing a connection.
type TransportAvailability int

const (
	// AvailUnknown is the zero value; no availability information has been
	// obtained. Callers should treat this as unreachable for selection.
	AvailUnknown TransportAvailability = iota
	// AvailReachable means the transport endpoint can be reached and is
	// usable for establishing a connection.
	AvailReachable
	// AvailUnreachable means the transport endpoint could not be reached.
	AvailUnreachable
)

// String returns a human-readable label for the availability value.
func (a TransportAvailability) String() string {
	switch a {
	case AvailReachable:
		return "reachable"
	case AvailUnreachable:
		return "unreachable"
	default:
		return "unknown"
	}
}

// SelectTransport implements the canonical fallback rule with explicit
// availability awareness:
//  1. If preferred is live-session capable AND reachable, return preferred.
//  2. Otherwise return fallback (typically stdio for thread resume/delivery).
//
// This is the rich form callers should use after obtaining reachability
// information via I/O checks (socket stat, proxy probe, ping).
// See BestAvailableTransport for the simplified form that assumes preferred
// is always reachable.
func SelectTransport(preferred, fallback TransportConfig, avail TransportAvailability) TransportConfig {
	if preferred.IsLiveSession() && avail == AvailReachable {
		return preferred
	}
	return fallback
}
