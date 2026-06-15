package codex

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Environment variable names
//
// These environment variables control the bundled Codex provider's connection
// to the codex app-server. They are set externally (by the user, the CI
// runner, or a wrapping script) and read by the provider on start-up.
// ---------------------------------------------------------------------------

const (
	// EnvCodexExecPath overrides the path to the codex binary. When unset,
	// the provider looks up "codex" on $PATH.
	EnvCodexExecPath = "ERO_CODEX_EXEC_PATH"

	// EnvCodexThreadID forces the provider to resume a specific Codex thread
	// by its stable identifier (e.g. "thr_abc123"). When unset the provider
	// auto-selects a thread by matching the CWD of the review repository.
	EnvCodexThreadID = "ERO_CODEX_THREAD_ID"

	// EnvCodexOverrides CODEX_HOME for the codex app-server process. When
	// unset the app-server uses its own default discovery.
	EnvCodexHome = "ERO_CODEX_HOME"

	// EnvCodexTransport controls the transport mechanism. Supported values:
	//   auto  (default) — probe for live session, fall back to stdio
	//   stdio            — always start a fresh app-server subprocess
	//   proxy            — connect via proxy to an existing session
	EnvCodexTransport = "ERO_CODEX_TRANSPORT"

	// EnvCodexSocketPath overrides the app-server control socket path.
	// When unset, defaults to $CODEX_HOME/app-server-control/app-server-control.sock.
	EnvCodexSocketPath = "ERO_CODEX_SOCKET_PATH"

	// EnvCodexSessionKey forces the provider to match a thread by its session
	// key rather than thread ID or CWD. When set (and ERO_CODEX_THREAD_ID is
	// empty), selection matches the unique thread whose SessionKey equals the
	// given value. Zero matches yields InvalidOverride; multiple yields
	// Ambiguous.
	EnvCodexSessionKey = "ERO_CODEX_SESSION_KEY"

	// EnvCodexTimeout sets the overall timeout for a publish workflow and all
	// per-RPC operations. Parsed as a Go duration (e.g. "30s", "2m", "90s").
	// When unset or invalid, DefaultCommandTimeout (30s) is used.
	EnvCodexTimeout = "ERO_CODEX_TIMEOUT"
)

// TransportMode describes the configured transport selection strategy.
type TransportMode string

const (
	// TransportModeAuto probes for a live session and falls back to stdio.
	TransportModeAuto TransportMode = "auto"
	// TransportModeStdio always starts a fresh app-server subprocess.
	TransportModeStdio TransportMode = "stdio"
	// TransportModeLive connects to an existing live Codex session via its
	// unix control socket (direct WebSocket transport, not a proxy subprocess).
	TransportModeLive TransportMode = "live"
	// TransportModeProxy is a backward-compatible alias for TransportModeLive.
	TransportModeProxy TransportMode = "proxy"
)

// Config controls how the bundled Codex provider connects to the codex
// app-server. A zero-value Config is resolved from the environment at the
// point of use via ConfigFromEnv.
type Config struct {
	// ExecPath is the path to the codex binary. When empty the provider
	// searches $PATH for "codex".
	ExecPath string

	// ThreadID is an explicit Codex thread identifier override. When
	// non-empty the provider resumes this specific thread instead of
	// auto-selecting by working-directory match.
	ThreadID string

	// SessionKey is an explicit session key override. When set (and ThreadID
	// is empty), selection matches the unique thread whose SessionKey equals
	// this value.
	SessionKey string

	// CodexHome overrides CODEX_HOME for the app-server subprocess. When
	// empty the app-server inherits the calling process's CODEX_HOME or
	// uses its built-in default.
	CodexHome string

	// Transport specifies the transport selection strategy.
	// Empty or "auto" probes for a live session and falls back to stdio.
	Transport TransportMode

	// SocketPath overrides the app-server control socket path.
	// When empty the default under CodexHome is used.
	SocketPath string

	// CommandTimeout is the per-request timeout for JSON-RPC round-trips
	// to the app-server. Zero uses DefaultCommandTimeout.
	CommandTimeout time.Duration
}

// DefaultCommandTimeout is the timeout used for each JSON-RPC request when
// Config.CommandTimeout is zero.
const DefaultCommandTimeout = 30 * time.Second

// EffectiveTimeout returns the non-zero timeout to use for request calls.
func (c Config) EffectiveTimeout() time.Duration {
	if c.CommandTimeout > 0 {
		return c.CommandTimeout
	}
	return DefaultCommandTimeout
}

// ConfigFromEnv builds a Config from the standard ERO_CODEX_* environment
// variables. Fields that are not set are left at their zero value so that
// downstream code can apply its own defaults.
func ConfigFromEnv() Config {
	transport := TransportMode(strings.TrimSpace(os.Getenv(EnvCodexTransport)))
	if transport == "" {
		transport = TransportModeAuto
	} else {
		// Normalise aliases: "proxy" is a backward-compatible name for "live".
		// Unknown values fall back to auto (safe default).
		switch transport {
		case TransportModeLive, TransportModeProxy:
			transport = TransportModeLive
		case TransportModeStdio:
			// keep as-is
		default:
			transport = TransportModeAuto
		}
	}

	// Parse timeout from env. Invalid or missing values are silently ignored.
	var timeout time.Duration
	if timeoutStr := strings.TrimSpace(os.Getenv(EnvCodexTimeout)); timeoutStr != "" {
		if d, err := time.ParseDuration(timeoutStr); err == nil && d > 0 {
			timeout = d
		}
	}

	return Config{
		ExecPath:       strings.TrimSpace(os.Getenv(EnvCodexExecPath)),
		ThreadID:       strings.TrimSpace(os.Getenv(EnvCodexThreadID)),
		SessionKey:     strings.TrimSpace(os.Getenv(EnvCodexSessionKey)),
		CodexHome:      strings.TrimSpace(os.Getenv(EnvCodexHome)),
		Transport:      transport,
		SocketPath:     strings.TrimSpace(os.Getenv(EnvCodexSocketPath)),
		CommandTimeout: timeout,
	}
}

// ResolveExecPath returns the path to the codex binary. If Config.ExecPath
// is set, it is checked with os.Stat first to verify existence, then returned.
// Otherwise exec.LookPath("codex") is used.
// Returns an error when the binary cannot be found.
func (c Config) ResolveExecPath() (string, error) {
	if c.ExecPath != "" {
		if _, err := os.Stat(c.ExecPath); err != nil {
			return "", fmt.Errorf("codex exec path %q: %w", c.ExecPath, err)
		}
		return c.ExecPath, nil
	}
	return exec.LookPath("codex")
}

// CodexAvailable reports whether the codex binary can be found on the system
// at the configured or default path. This is a lightweight check suitable for
// detect_context — it does not start the app-server.
func (c Config) CodexAvailable() bool {
	_, err := c.ResolveExecPath()
	return err == nil
}

// ShouldProbeSocket returns true when the transport config requires probing
// for a live session before deciding how to connect.
func (c Config) ShouldProbeSocket() bool {
	return c.Transport == TransportModeAuto
}

// EffectiveSocketPath returns the resolved socket path with ambient fallback.
// Precedence:
//  1. explicit ERO_CODEX_SOCKET_PATH (c.SocketPath)
//  2. explicit ERO_CODEX_HOME (c.CodexHome)
//  3. ambient CODEX_HOME env var
//  4. default ~/.codex
func (c Config) EffectiveSocketPath() string {
	// 1. Explicit socket path takes highest precedence.
	if c.SocketPath != "" {
		return c.SocketPath
	}

	// 2–4. Resolve Codex home via the full chain.
	home := resolveCodexHome(c.CodexHome)
	if home == "" {
		return ""
	}
	return DefaultSocketPath(home)
}

// resolveCodexHome returns the effective Codex home directory using the
// standard fallback chain:
//  1. explicit path (e.g. from ERO_CODEX_HOME)
//  2. ambient CODEX_HOME env var
//  3. default ~/.codex
//
// Returns empty string when none of the sources yields a usable path.
func resolveCodexHome(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if ambient := os.Getenv("CODEX_HOME"); ambient != "" {
		return ambient
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".codex")
}
