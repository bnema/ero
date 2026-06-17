package app

import (
	"context"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"
	"github.com/bnema/zerowrap"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"ero/internal/adapters/in/cli"
	"ero/internal/adapters/in/terminal"
	"ero/internal/adapters/in/tui"
	clipboardadapter "ero/internal/adapters/out/clipboard"
	gitadapter "ero/internal/adapters/out/git"
	pluginadapter "ero/internal/adapters/out/plugin"
	providercache "ero/internal/adapters/out/providercache"
	chromatokenizer "ero/internal/adapters/out/syntax/chroma"
	"ero/internal/core"
	"ero/internal/logging"
	"ero/internal/ports"
)

type reviewLoader interface {
	LoadReview(request core.ReviewRequest) ([]core.ReviewFile, error)
}

type startupPrompt interface {
	PromptLocalChangeMode() (core.DiffMode, error)
}

type themedStartupPrompt interface {
	startupPrompt
	WithAppearance(core.ThemeAppearance) tui.StartupPrompt
}

type tuiRunner interface {
	Run(model tea.Model) error
}

type App struct {
	config *viper.Viper
	root   *cobra.Command
}

func New() (*App, error) {
	cfg := viper.New()
	repositoryLoader := gitadapter.NewRepositoryLoader()
	syntaxTokenizer := chromatokenizer.NewTokenizer()
	reviewLoader := core.NewReviewLoader(repositoryLoader, repositoryLoader, syntaxTokenizer, repositoryLoader)
	runner := tui.NewRunner()
	clipboardWriter := clipboardadapter.NewSystemWriter()
	return newAppWithClipboard(cfg, reviewLoader, runner, repositoryLoader, tui.NewStartupPrompt(), terminal.IsInteractive, clipboardWriter)
}

func newApp(cfg *viper.Viper, loader reviewLoader, runner tuiRunner) (*App, error) {
	return newAppWithStartup(cfg, loader, runner, nil, nil, func() bool { return false })
}

func newAppWithStartup(cfg *viper.Viper, loader reviewLoader, runner tuiRunner, startupReader ports.StartupStateReader[core.StartupState], prompt startupPrompt, isInteractive func() bool) (*App, error) {
	return newAppWithClipboard(cfg, loader, runner, startupReader, prompt, isInteractive, nil)
}

func newAppWithClipboard(cfg *viper.Viper, loader reviewLoader, runner tuiRunner, startupReader ports.StartupStateReader[core.StartupState], prompt startupPrompt, isInteractive func() bool, clipboardWriter ports.ClipboardWriter) (*App, error) {
	if cfg == nil {
		cfg = viper.New()
	}
	if loader == nil {
		return nil, fmt.Errorf("review loader is nil")
	}
	if runner == nil {
		return nil, fmt.Errorf("runner is nil")
	}

	root, err := cli.NewRootCommand(cfg, func() error {
		if err := loadRuntimeConfig(cfg); err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		themeModeChanges := watchThemeConfigChanges(cfg)
		log, logPath, cleanupLogs, err := logging.Init(logging.Config{Level: cfg.GetString("log-level"), Path: cfg.GetString("log-file")})
		if err != nil {
			return fmt.Errorf("initialize logging: %w", err)
		}
		defer cleanupLogs()
		ctx := zerowrap.WithCtx(context.Background(), log)
		log.Info().Str("log_path", logPath).Msg("ero started")

		initialThemeMode := core.ParseThemeMode(cfg.GetString("theme"))

		initialRequest := core.ReviewRequest{
			RepoPath:     cfg.GetString("repo-path"),
			ContextLines: cfg.GetInt("context-lines"),
			DiffMode:     core.DiffMode(cfg.GetString("diff-mode")),
			Revision:     cfg.GetString("revision"),
			BaseRevision: cfg.GetString("base-revision"),
			HeadRevision: cfg.GetString("head-revision"),
			UpstreamRef:  cfg.GetString("upstream-ref"),
		}
		if cfg.GetBool("startup-detect") {
			startupPrompt := prompt
			if themed, ok := prompt.(themedStartupPrompt); ok {
				startupPrompt = themed.WithAppearance(core.ResolveThemeAppearance(initialThemeMode, core.SystemThemeUnknown, core.ThemeAppearanceLight))
			}
			request, err := resolveStartupRequest(initialRequest, startupReader, startupPrompt, isInteractive)
			if err != nil {
				return err
			}
			initialRequest = request
		}
		log.Info().Str("diff_mode", string(initialRequest.DiffMode)).Str("repo_path", initialRequest.RepoPath).Msg("loading review")
		files, err := loader.LoadReview(initialRequest)
		if err != nil {
			log.Error().Err(err).Msg("load review failed")
			return err
		}
		log.Info().Int("files", len(files)).Msg("review loaded")
		pluginManager := pluginadapter.NewManager()
		providerLoader := pluginadapter.NewReviewProviderLoader(pluginManager)
		providerStore := providercache.NewXDGStore()
		activeProviderService := NewActiveProviderService(providerLoader, providerLoader, providerStore, providerStore, providerPollingConfigFromConfig(cfg))
		activeProvider := &tuiActiveProviderController{catalog: providerLoader, service: activeProviderService}
		var metadata ports.GitMetadataReader
		if reader, ok := loader.(ports.GitMetadataReader); ok {
			metadata = reader
		} else if reader, ok := startupReader.(ports.GitMetadataReader); ok {
			metadata = reader
		}
		reviewContext := buildReviewContext(initialRequest, files, metadata, version)
		var compatibilityProviders []ports.ReviewProviderClient
		err = runner.Run(tui.NewModelWithActiveProviderContextConfig(ctx, files, terminal.NewCapabilities(), loader, initialRequest, clipboardWriter, reviewContext, activeProvider, compatibilityProviders, tui.ModelConfig{
			ThemeMode:        initialThemeMode,
			ThemeModeChanges: themeModeChanges,
		}))
		if err != nil {
			log.Error().Err(err).Msg("tui exited with error")
			return err
		}
		log.Info().Msg("ero exited")
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("build root command: %w", err)
	}
	root.AddCommand(versionCommand())
	root.AddCommand(cli.NewPluginCommand(pluginadapter.NewManager(), nil))

	return &App{
		config: cfg,
		root:   root,
	}, nil
}

func (a *App) RootCommand() *cobra.Command {
	return a.root
}

func (a *App) Run(args []string, stdout, stderr io.Writer) error {
	if err := resetCommandFlags(a.root); err != nil {
		return err
	}
	a.root.SetOut(stdout)
	a.root.SetErr(stderr)
	a.root.SetArgs(args)
	return a.root.Execute()
}

func resetCommandFlags(cmd *cobra.Command) error {
	if err := resetFlags(cmd.PersistentFlags()); err != nil {
		return err
	}
	if err := resetFlags(cmd.Flags()); err != nil {
		return err
	}
	for _, child := range cmd.Commands() {
		if err := resetCommandFlags(child); err != nil {
			return err
		}
	}
	return nil
}

func resetFlags(flags *pflag.FlagSet) error {
	var resetErr error
	flags.VisitAll(func(flag *pflag.Flag) {
		if resetErr != nil {
			return
		}
		if err := flag.Value.Set(flag.DefValue); err != nil {
			resetErr = fmt.Errorf("reset flag %s: %w", flag.Name, err)
			return
		}
		flag.Changed = false
	})
	return resetErr
}
