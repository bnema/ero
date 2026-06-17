package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"ero/internal/core"
)

type RunFunc func() error

func NewRootCommand(cfg *viper.Viper, run RunFunc) (*cobra.Command, error) {
	if cfg == nil {
		cfg = viper.New()
	}

	runCommand := func(cmd *cobra.Command, args []string) error {
		if run == nil {
			return nil
		}
		return run()
	}
	runWithReviewConfig := func(mode core.DiffMode, options reviewConfig) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, args []string) error {
			setReviewConfig(cfg, mode, options)
			cfg.Set("startup-detect", true)
			return runCommand(cmd, args)
		}
	}

	cmd := &cobra.Command{
		Use:          "ero",
		Short:        "Review branch diffs in a GitHub-style TUI",
		SilenceUsage: true,
		RunE:         runWithReviewConfig(core.DiffModeBranch, reviewConfig{}),
	}

	flags := cmd.PersistentFlags()
	flags.String("repo-path", ".", "Path to the git repository to review")
	flags.Int("context-lines", 3, "Number of unchanged context lines to keep around changes")
	flags.String("log-level", "info", "Log level (trace, debug, info, warn, error, disabled)")
	flags.String("log-file", "", "Write logs to this file instead of the default XDG state log")
	flags.String("theme", "auto", "Theme mode: auto, dark, or light")
	flags.String("config", "", "Path to a config file (defaults to XDG config when present)")
	flags.Duration("provider-sync-interval", 2*time.Minute, "Interval for active review provider background sync")
	cfg.SetDefault("repo-path", ".")
	cfg.SetDefault("context-lines", 3)
	cfg.SetDefault("diff-mode", string(core.DiffModeBranch))
	cfg.SetDefault("startup-detect", true)
	cfg.SetDefault("log-level", "info")
	cfg.SetDefault("theme", "auto")
	cfg.SetDefault("provider-sync-interval", 2*time.Minute)
	if err := cfg.BindPFlag("repo-path", flags.Lookup("repo-path")); err != nil {
		return nil, fmt.Errorf("bind repo-path flag: %w", err)
	}
	if err := cfg.BindPFlag("context-lines", flags.Lookup("context-lines")); err != nil {
		return nil, fmt.Errorf("bind context-lines flag: %w", err)
	}
	if err := cfg.BindPFlag("log-level", flags.Lookup("log-level")); err != nil {
		return nil, fmt.Errorf("bind log-level flag: %w", err)
	}
	if err := cfg.BindPFlag("log-file", flags.Lookup("log-file")); err != nil {
		return nil, fmt.Errorf("bind log-file flag: %w", err)
	}
	if err := cfg.BindPFlag("theme", flags.Lookup("theme")); err != nil {
		return nil, fmt.Errorf("bind theme flag: %w", err)
	}
	if err := cfg.BindPFlag("config", flags.Lookup("config")); err != nil {
		return nil, fmt.Errorf("bind config flag: %w", err)
	}
	if err := cfg.BindPFlag("provider-sync-interval", flags.Lookup("provider-sync-interval")); err != nil {
		return nil, fmt.Errorf("bind provider-sync-interval flag: %w", err)
	}
	cfg.SetEnvPrefix("ERO")
	cfg.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	cfg.AutomaticEnv()

	cmd.AddCommand(diffModeCommands(cfg, runCommand)...)

	return cmd, nil
}

type reviewConfig struct {
	revision     string
	baseRevision string
	headRevision string
	upstreamRef  string
}

func diffModeCommands(cfg *viper.Viper, runCommand func(*cobra.Command, []string) error) []*cobra.Command {
	runWithReviewConfig := func(mode core.DiffMode, options func([]string) reviewConfig) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, args []string) error {
			setReviewConfig(cfg, mode, options(args))
			cfg.Set("startup-detect", false)
			return runCommand(cmd, args)
		}
	}

	return []*cobra.Command{
		{
			Use:   "branch",
			Short: "Review the working branch against the default branch",
			Args:  cobra.NoArgs,
			RunE:  runWithReviewConfig(core.DiffModeBranch, emptyReviewConfig),
		},
		{
			Use:   "working",
			Short: "Review unstaged working tree changes",
			Args:  cobra.NoArgs,
			RunE:  runWithReviewConfig(core.DiffModeWorking, emptyReviewConfig),
		},
		{
			Use:   "staged",
			Short: "Review staged changes",
			Args:  cobra.NoArgs,
			RunE:  runWithReviewConfig(core.DiffModeStaged, emptyReviewConfig),
		},
		{
			Use:   "local",
			Short: "Review all local uncommitted changes",
			Args:  cobra.NoArgs,
			RunE:  runWithReviewConfig(core.DiffModeLocal, emptyReviewConfig),
		},
		{
			Use:   "upstream [ref]",
			Short: "Review changes against the upstream branch",
			Args:  cobra.RangeArgs(0, 1),
			RunE: runWithReviewConfig(core.DiffModeUpstream, func(args []string) reviewConfig {
				config := reviewConfig{}
				if len(args) > 0 {
					config.upstreamRef = args[0]
				}
				return config
			}),
		},
		{
			Use:   "commit <revision>",
			Short: "Review one commit's changes",
			Args:  cobra.MaximumNArgs(1),
			RunE: runWithRequiredReviewConfig(cfg, runCommand, core.DiffModeCommit, 1, func(args []string) reviewConfig {
				return reviewConfig{revision: args[0]}
			}),
		},
		{
			Use:   "range <base> <head>",
			Short: "Review changes across a commit or branch range",
			Args:  cobra.MaximumNArgs(2),
			RunE: runWithRequiredReviewConfig(cfg, runCommand, core.DiffModeRange, 2, func(args []string) reviewConfig {
				return reviewConfig{baseRevision: args[0], headRevision: args[1]}
			}),
		},
	}
}

func emptyReviewConfig([]string) reviewConfig {
	return reviewConfig{}
}

func runWithRequiredReviewConfig(cfg *viper.Viper, runCommand func(*cobra.Command, []string) error, mode core.DiffMode, requiredArgs int, options func([]string) reviewConfig) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < requiredArgs {
			setReviewConfig(cfg, mode, reviewConfig{})
			return cmd.Help()
		}
		setReviewConfig(cfg, mode, options(args))
		cfg.Set("startup-detect", false)
		return runCommand(cmd, args)
	}
}

func setReviewConfig(cfg *viper.Viper, mode core.DiffMode, options reviewConfig) {
	cfg.Set("diff-mode", string(mode))
	cfg.Set("revision", options.revision)
	cfg.Set("base-revision", options.baseRevision)
	cfg.Set("head-revision", options.headRevision)
	cfg.Set("upstream-ref", options.upstreamRef)
}
