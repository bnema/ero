package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"ero/pkg/plugin/protocol"
)

// ProviderRequestHandler dispatches a builtin provider protocol request.
// The CLI owns only the stdin/stdout JSON-lines shell; provider-specific
// orchestration lives behind the injected handler.
type ProviderRequestHandler func(ctx context.Context, providerID string, req protocol.Request) protocol.Response

// NewProviderRuntimeCommand creates the hidden __provider command that runs
// builtin provider subprocesses. It is invoked by the main ero binary when a
// builtin provider client (created by BuiltinAwareFactory) spawns itself.
//
// The command reads JSON-lines requests from stdin and writes JSON-lines
// responses to stdout, using the same protocol that external plugins use.
func NewProviderRuntimeCommand(handler ProviderRequestHandler) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "__provider",
		Short:  "Builtin provider runtime (internal)",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if handler == nil {
				return fmt.Errorf("builtin provider runtime handler is nil")
			}
			return runProviderRuntime(cmd.Context(), args[0], cmd.InOrStdin(), cmd.OutOrStdout(), handler)
		},
	}
	return cmd
}

// runProviderRuntime implements the JSON-lines plugin protocol shell for a
// builtin provider. Each valid request is delegated to the injected handler.
func runProviderRuntime(ctx context.Context, providerID string, in io.Reader, out io.Writer, handler ProviderRequestHandler) error {
	scanner := bufio.NewScanner(in)
	// Allow lines up to 1 MiB.
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	encoder := json.NewEncoder(out)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var req protocol.Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			// Emit an invalid_request response instead of silently skipping.
			// We cannot set the ID because the request envelope could not be
			// parsed, but the host can still correlate by line position.
			if encodeErr := encoder.Encode(protocol.Response{
				Error: protocol.NewError(protocol.ErrorInvalidRequest,
					"malformed request: not valid JSON"),
			}); encodeErr != nil {
				return fmt.Errorf("encode error response: %w", encodeErr)
			}
			continue
		}

		resp := handler(ctx, providerID, req)
		resp.ID = req.ID

		if err := encoder.Encode(resp); err != nil {
			return fmt.Errorf("encode response: %w", err)
		}
	}
	return scanner.Err()
}
