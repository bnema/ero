package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"ero/internal/adapters/in/cli/render"
	"ero/internal/ports"
)

// PluginManager is the interface the CLI needs from the application boundary.
type PluginManager = ports.PluginLifecycle

// NewPluginCommand creates the "plugin" parent command with list, install,
// update, and remove subcommands. Each supports --json for machine-readable
// output.
func NewPluginCommand(manager PluginManager, out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage Ero review provider plugins",
		Long: `Manage Ero plugins that provide review provider capabilities.

Plugins can be installed from Git URLs or local Git repositories. The plugin
system stores metadata in your Ero config directory and manages plugin data
under the Ero data directory. Local plugins are tracked by reference; their
repositories are never deleted by Ero.

Codex is bundled/default in packaged releases and source checkouts: it is
available without installation, appears in provider discovery and plugin list,
runs through the normal plugin/provider path, and cannot be installed,
removed, or updated through plugin lifecycle commands. go install ./cmd/ero installs only the ero binary; bundled Codex assets come from packaged release
layouts or from plugins/codex when you run inside a source checkout. The
lifecycle commands reject the bundled identifiers codex, Codex,
ero-plugin-codex, and bundled:ero-plugin-codex unless one of those names
resolves to a real user-installed or local-path plugin.`,
	}

	var jsonOutput bool
	cmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	cmd.AddCommand(newPluginListCommand(manager, out, &jsonOutput))
	cmd.AddCommand(newPluginInstallCommand(manager, out, &jsonOutput))
	cmd.AddCommand(newPluginUpdateCommand(manager, out, &jsonOutput))
	cmd.AddCommand(newPluginRemoveCommand(manager, out, &jsonOutput))

	return cmd
}

func newPluginListCommand(manager PluginManager, out io.Writer, jsonOutput *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available plugins",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			plugins, err := manager.List(ctx)
			if err != nil {
				return err
			}

			writer := commandOut(cmd, out)
			if *jsonOutput {
				return encodeJSON(writer, plugins)
			}

			return renderPluginList(writer, plugins)
		},
	}
}

func newPluginInstallCommand(manager PluginManager, out io.Writer, jsonOutput *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "install <source>",
		Short: "Install a plugin from a Git URL or local path",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			result, err := manager.Install(ctx, args[0])
			if err != nil {
				return err
			}

			writer := commandOut(cmd, out)
			if *jsonOutput {
				return encodeJSON(writer, result)
			}

			_, err = fmt.Fprintln(writer, render.PluginInstall(result))
			return err
		},
	}
}

func newPluginUpdateCommand(manager PluginManager, out io.Writer, jsonOutput *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "update [source]",
		Short: "Update user-installed plugins to their latest version",
		Long: `Update user-installed plugins to the latest version from their source.

When a source is specified, only that plugin is updated. When no source is
given, all user-installed plugins are updated. Pinned plugins, local sources,
and bundled/default plugins are reported as skipped.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			filter := ""
			if len(args) > 0 {
				filter = args[0]
			}

			results, err := manager.Update(ctx, filter)
			if err != nil {
				return err
			}

			writer := commandOut(cmd, out)
			if *jsonOutput {
				return encodeJSON(writer, results)
			}

			_, err = fmt.Fprintln(writer, render.PluginUpdates(results))
			return err
		},
	}
}

func newPluginRemoveCommand(manager PluginManager, out io.Writer, jsonOutput *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name|source>",
		Short: "Remove a user-installed plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			result, err := manager.Remove(ctx, args[0])
			if err != nil {
				return err
			}

			writer := commandOut(cmd, out)
			if *jsonOutput {
				return encodeJSON(writer, result)
			}

			_, err = fmt.Fprintln(writer, render.PluginRemove(result))
			return err
		},
	}
}

func encodeJSON(writer io.Writer, v any) error {
	enc := json.NewEncoder(writer)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func commandOut(cmd *cobra.Command, configured io.Writer) io.Writer {
	if configured != nil {
		return configured
	}
	return cmd.OutOrStdout()
}

// renderPluginList writes a human-readable table of available plugins.
func renderPluginList(out io.Writer, plugins []ports.InstalledPlugin) error {
	_, err := fmt.Fprintln(out, render.PluginList(plugins, 100))
	return err
}
