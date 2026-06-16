package codex

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Environment variable names
//
// These environment variables control the bundled Codex provider's connection
// to the codex app-server for a callback into a specific session.
// They are set externally (by the user, the CI runner, or a wrapping script)
// and read by the provider on start-up.
// ---------------------------------------------------------------------------

const (
	// EnvCodexSocketPath specifies the explicit Codex app-server control
	// socket path. Must be set for callback mode.
	EnvCodexSocketPath = "ERO_CODEX_SOCKET_PATH"

	// EnvCodexThreadID specifies the explicit Codex thread/session target
	// to send the callback into. Must be set for callback mode.
	EnvCodexThreadID = "ERO_CODEX_THREAD_ID"

	// EnvCodexTimeout sets the overall timeout for all per-RPC operations.
	// Parsed as a Go duration (e.g. "30s", "2m", "90s").
	// When unset or invalid, DefaultCommandTimeout (30s) is used.
	EnvCodexTimeout = "ERO_CODEX_TIMEOUT"
)

// Config controls how the bundled Codex provider connects to the codex
// app-server for a callback into a specific Codex session.
type Config struct {
	// ThreadID is an explicit Codex thread identifier to target.
	// Required for callback mode.
	ThreadID string

	// SocketPath is the explicit Codex app-server control socket path.
	// Required for callback mode.
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

// EffectiveSocketPath returns the configured socket path. In the callback-only
// Config this simply returns c.SocketPath — there is no ambient fallback.
func (c Config) EffectiveSocketPath() string {
	return c.SocketPath
}

// ConfigFromEnv builds a Config from the standard ERO_CODEX_* environment
// variables. Fields that are not set are left at their zero value so that
// downstream code can apply its own defaults or validation.
func ConfigFromEnv() Config {
	var timeout time.Duration
	if timeoutStr := strings.TrimSpace(os.Getenv(EnvCodexTimeout)); timeoutStr != "" {
		if d, err := time.ParseDuration(timeoutStr); err == nil && d > 0 {
			timeout = d
		}
	}

	return Config{
		ThreadID:       strings.TrimSpace(os.Getenv(EnvCodexThreadID)),
		SocketPath:     strings.TrimSpace(os.Getenv(EnvCodexSocketPath)),
		CommandTimeout: timeout,
	}
}

// ErrMissingCallbackConfig is returned by ValidateCallbackTarget when
// required configuration is missing.
var ErrMissingCallbackConfig = errors.New("codex: missing required callback configuration")

// ValidateCallbackTarget checks that the Config has the required fields
// for a callback into a specific Codex session. Returns nil when both
// SocketPath and ThreadID are set, or an error wrapping
// ErrMissingCallbackConfig if either is empty.
func (c Config) ValidateCallbackTarget() error {
	var missing []string
	if c.SocketPath == "" {
		missing = append(missing, EnvCodexSocketPath)
	}
	if c.ThreadID == "" {
		missing = append(missing, EnvCodexThreadID)
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: set %s", ErrMissingCallbackConfig, strings.Join(missing, ", "))
	}
	return nil
}
