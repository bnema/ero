package app

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"

	"ero/internal/core"
)

func loadRuntimeConfig(cfg *viper.Viper) error {
	if cfg == nil {
		return nil
	}
	if configPath := cfg.GetString("config"); configPath != "" {
		cfg.SetConfigFile(configPath)
	} else if cfg.ConfigFileUsed() == "" {
		cfg.SetConfigName("config")
		cfg.SetConfigType("toml")
		if configDir, err := os.UserConfigDir(); err == nil && configDir != "" {
			cfg.AddConfigPath(filepath.Join(configDir, "ero"))
		}
	}

	err := cfg.ReadInConfig()
	if err == nil {
		return nil
	}
	var notFound viper.ConfigFileNotFoundError
	if errors.As(err, &notFound) && cfg.GetString("config") == "" {
		return nil
	}
	return err
}

// watchThemeConfigChanges starts Viper's process-lifetime config watcher and
// emits the latest theme mode whenever the config file changes. Viper does not
// expose a watcher stop hook, so Ero intentionally keeps this alive until exit.
func watchThemeConfigChanges(cfg *viper.Viper) <-chan core.ThemeMode {
	if cfg == nil || cfg.ConfigFileUsed() == "" {
		return nil
	}
	changes := make(chan core.ThemeMode, 1)
	cfg.OnConfigChange(func(fsnotify.Event) {
		if err := cfg.ReadInConfig(); err != nil {
			return
		}
		mode := core.ParseThemeMode(cfg.GetString("theme"))
		select {
		case changes <- mode:
		default:
			select {
			case <-changes:
			default:
			}
			select {
			case changes <- mode:
			default:
			}
		}
	})
	cfg.WatchConfig()
	return changes
}
