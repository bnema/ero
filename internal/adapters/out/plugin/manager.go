package pluginadapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"ero/internal/ports"
)

// ---------- config model ----------

type pluginConfig struct {
	Plugins []pluginEntry `toml:"plugins"`
}

type pluginEntry struct {
	Source string `toml:"source"`
}

// ---------- Manager ----------

// Manager handles plugin lifecycle: install, list, update, remove.
// Plugins are stored in the ero data directory and tracked in the ero
// config file.
type Manager struct {
	configPath string
	dataDir    string
	// gitCommand is the path to git; injected for testing.
	gitCommand string
}

// NewManager creates a Manager using XDG paths.
func NewManager() *Manager {
	return newManager(ConfigDir(), DataDir(), "git")
}

// newManager is the injectable constructor used for testing.
func newManager(configDir, dataDir, gitCommand string) *Manager {
	configPath := filepath.Join(configDir, "config.toml")
	return &Manager{
		configPath: configPath,
		dataDir:    dataDir,
		gitCommand: gitCommand,
	}
}

// Install clones or registers a plugin from source.
func (m *Manager) Install(ctx context.Context, rawSource string) (ports.PluginInstallResult, error) {
	rawSource = strings.TrimSpace(rawSource)
	if isBundledSource(rawSource) {
		return ports.PluginInstallResult{}, bundledLifecycleError("installed", rawSource)
	}
	if source, err := ParseLocalSource(rawSource); err == nil {
		return m.installLocal(source)
	}
	if descriptor, ok := bundledDescriptorForInput(rawSource); ok {
		return ports.PluginInstallResult{}, bundledLifecycleError("installed", descriptor.Source)
	}
	source, err := ParseSource(rawSource)
	if err != nil {
		return ports.PluginInstallResult{}, err
	}

	switch source.Type {
	case SourceTypeGit:
		return m.installGit(ctx, source)
	case SourceTypeLocal:
		return m.installLocal(source)
	default:
		return ports.PluginInstallResult{}, fmt.Errorf("unsupported source type: %s", source.Type)
	}
}

// List returns shipped plugins plus user-installed plugins loaded
// from config and plugin manifests.
func (m *Manager) List(ctx context.Context) ([]ports.InstalledPlugin, error) {
	entries, err := m.loadConfig()
	if err != nil {
		return nil, err
	}

	plugins := bundledInstalledPlugins()
	shippedPaths := shippedPluginPathSet(plugins)
	for _, entry := range entries.Plugins {
		source, err := ParseSource(entry.Source)
		if err != nil {
			continue
		}

		pluginDir := m.pluginDir(source)
		if _, shipped := shippedPaths[cleanPathKey(pluginDir)]; shipped {
			continue
		}
		manifest, err := LoadManifest(pluginDir)
		if err != nil {
			// Plugin dir exists but manifest is broken — still list it.
			plugins = append(plugins, ports.InstalledPlugin{
				Name:   m.pluginName(source),
				Source: entry.Source,
				Path:   pluginDir,
			})
			continue
		}

		contributions := make([]string, len(manifest.Contributions))
		for i, c := range manifest.Contributions {
			contributions[i] = c.Type + ":" + c.ID
		}

		plugins = append(plugins, ports.InstalledPlugin{
			Name:          manifest.Name,
			Version:       manifest.Version,
			Source:        entry.Source,
			Path:          pluginDir,
			Contributions: contributions,
		})
	}

	return plugins, nil
}

// InstalledPlugins implements ports.PluginRegistry for app-level discovery.
func (m *Manager) InstalledPlugins(ctx context.Context) ([]ports.PluginDescriptor, error) {
	entries, err := m.loadConfig()
	if err != nil {
		return nil, err
	}
	descriptors := bundledPlugins()
	shippedPaths := shippedDescriptorPathSet(descriptors)
	for _, entry := range entries.Plugins {
		source, err := ParseSource(entry.Source)
		if err != nil {
			continue
		}
		pluginDir := m.pluginDir(source)
		if _, shipped := shippedPaths[cleanPathKey(pluginDir)]; shipped {
			continue
		}
		manifest, err := LoadManifest(pluginDir)
		if err != nil {
			continue
		}
		contributions := make([]ports.PluginContribution, 0, len(manifest.Contributions))
		for _, contribution := range manifest.Contributions {
			contributions = append(contributions, ports.PluginContribution{Type: contribution.Type, ID: contribution.ID, Label: contribution.Label})
		}
		descriptors = append(descriptors, ports.PluginDescriptor{Name: manifest.Name, Version: manifest.Version, Source: entry.Source, Path: pluginDir, Contributions: contributions})
	}
	return descriptors, nil
}

func shippedPluginPathSet(plugins []ports.InstalledPlugin) map[string]struct{} {
	paths := make(map[string]struct{}, len(plugins))
	for _, plugin := range plugins {
		if plugin.Bundled && plugin.Path != "" {
			paths[cleanPathKey(plugin.Path)] = struct{}{}
		}
	}
	return paths
}

func shippedDescriptorPathSet(descriptors []ports.PluginDescriptor) map[string]struct{} {
	paths := make(map[string]struct{}, len(descriptors))
	for _, descriptor := range descriptors {
		if descriptor.Bundled && descriptor.Path != "" {
			paths[cleanPathKey(descriptor.Path)] = struct{}{}
		}
	}
	return paths
}

// Update fetches and resets non-pinned user-installed plugins to the latest
// upstream. Pinned plugins and shipped plugins are reported as skipped.
func (m *Manager) Update(ctx context.Context, rawSource string) ([]ports.PluginUpdateResult, error) {
	rawSource = strings.TrimSpace(rawSource)
	if isBundledSource(rawSource) {
		return bundledUpdateResults(rawSource), nil
	}
	entries, err := m.loadConfig()
	if err != nil {
		return nil, err
	}

	targetIdx := -1
	if rawSource != "" {
		idx, _, _, ok := m.findInstalledPluginEntry(entries, rawSource)
		if ok {
			targetIdx = idx
		} else if descriptor, bundled := bundledDescriptorForInput(rawSource); bundled {
			return bundledUpdateResults(descriptor.Source), nil
		}
	}

	var results []ports.PluginUpdateResult
	if rawSource == "" {
		results = append(results, bundledUpdateResults("")...)
	}
	for i, entry := range entries.Plugins {
		if rawSource != "" && i != targetIdx {
			continue
		}

		source, err := ParseSource(entry.Source)
		if err != nil {
			results = append(results, ports.PluginUpdateResult{
				Source:  entry.Source,
				Message: fmt.Sprintf("parse error: %v", err),
			})
			continue
		}

		if source.Type != SourceTypeGit {
			results = append(results, ports.PluginUpdateResult{
				Source:  entry.Source,
				Name:    m.pluginName(source),
				Message: "local sources are not updated automatically",
			})
			continue
		}

		if source.Pinned {
			results = append(results, ports.PluginUpdateResult{
				Source:  entry.Source,
				Name:    m.pluginName(source),
				Message: fmt.Sprintf("pinned to %s, skipping update", source.Ref),
			})
			continue
		}

		pluginDir := m.pluginDir(source)
		previousRef, err := m.currentRef(ctx, pluginDir)
		if err != nil {
			results = append(results, ports.PluginUpdateResult{
				Source:  entry.Source,
				Name:    m.pluginName(source),
				Message: fmt.Sprintf("could not read current ref: %v", err),
			})
			continue
		}

		if err := m.fetchAndReset(ctx, pluginDir); err != nil {
			results = append(results, ports.PluginUpdateResult{
				Source:  entry.Source,
				Name:    m.pluginName(source),
				Message: fmt.Sprintf("update failed: %v", err),
			})
			continue
		}

		updatedRef, _ := m.currentRef(ctx, pluginDir)
		results = append(results, ports.PluginUpdateResult{
			Source:      entry.Source,
			Name:        m.pluginName(source),
			PreviousRef: previousRef,
			UpdatedRef:  updatedRef,
		})
	}

	return results, nil
}

// Remove deletes a plugin from config and, for managed git clones, removes
// the data directory. Local source repos are never deleted.
func (m *Manager) Remove(ctx context.Context, nameOrSource string) (ports.PluginRemoveResult, error) {
	nameOrSource = strings.TrimSpace(nameOrSource)
	if isBundledSource(nameOrSource) {
		return ports.PluginRemoveResult{}, bundledLifecycleError("removed", nameOrSource)
	}
	entries, err := m.loadConfig()
	if err != nil {
		return ports.PluginRemoveResult{}, err
	}

	targetIdx, targetEntry, source, ok := m.findInstalledPluginEntry(entries, nameOrSource)
	if !ok {
		if descriptor, bundled := bundledDescriptorForInput(nameOrSource); bundled {
			return ports.PluginRemoveResult{}, bundledLifecycleError("removed", descriptor.Source)
		}
		return ports.PluginRemoveResult{}, fmt.Errorf("plugin %q not found in config", nameOrSource)
	}

	pluginDir := m.pluginDir(source)

	// Remove config entry.
	entries.Plugins = append(entries.Plugins[:targetIdx], entries.Plugins[targetIdx+1:]...)
	if err := m.saveConfig(entries); err != nil {
		return ports.PluginRemoveResult{}, fmt.Errorf("save config: %w", err)
	}

	removedRepo := false
	if source.Type == SourceTypeGit {
		if err := os.RemoveAll(pluginDir); err != nil && !os.IsNotExist(err) {
			return ports.PluginRemoveResult{}, fmt.Errorf("remove plugin dir: %w", err)
		}
		removedRepo = true
	}

	return ports.PluginRemoveResult{
		Name:        m.pluginName(source),
		Source:      targetEntry.Source,
		RemovedRepo: removedRepo,
	}, nil
}

// ---------- internal helpers ----------

func (m *Manager) findInstalledPluginEntry(entries *pluginConfig, nameOrSource string) (int, pluginEntry, Source, bool) {
	for i, entry := range entries.Plugins {
		source, err := ParseSource(entry.Source)
		if err != nil {
			continue
		}
		// Match by source string, repo path, directory name, or manifest name.
		if strings.EqualFold(entry.Source, nameOrSource) || strings.EqualFold(source.Path, nameOrSource) || strings.EqualFold(source.Repo, nameOrSource) || strings.EqualFold(m.pluginName(source), nameOrSource) {
			return i, entry, source, true
		}
	}
	return -1, pluginEntry{}, Source{}, false
}

func (m *Manager) installGit(ctx context.Context, source Source) (ports.PluginInstallResult, error) {
	pluginDir := m.pluginDir(source)

	// Build clone URL from source.
	cloneURL := buildCloneURL(source)

	// Check if already installed.
	if _, err := os.Stat(filepath.Join(pluginDir, ".git")); err == nil {
		return ports.PluginInstallResult{}, fmt.Errorf("plugin %q is already installed at %s", source.Repo, pluginDir)
	}

	// Clone.
	args := []string{"clone", cloneURL, pluginDir}
	if err := m.runGit(ctx, "", args...); err != nil {
		return ports.PluginInstallResult{}, fmt.Errorf("clone %s: %w", source.Repo, err)
	}

	// Checkout pinned ref.
	if source.Pinned {
		if err := m.runGit(ctx, pluginDir, "checkout", source.Ref); err != nil {
			_ = os.RemoveAll(pluginDir)
			return ports.PluginInstallResult{}, fmt.Errorf("checkout ref %q: %w", source.Ref, err)
		}
	}

	// Load manifest to get name/version.
	manifest, err := LoadManifest(pluginDir)
	if err != nil {
		// Clean up on manifest failure.
		_ = os.RemoveAll(pluginDir)
		return ports.PluginInstallResult{}, fmt.Errorf("%s: invalid manifest: %w", source.Repo, err)
	}

	// Write config entry.
	if err := m.addConfigEntry(source); err != nil {
		_ = os.RemoveAll(pluginDir)
		return ports.PluginInstallResult{}, err
	}

	return ports.PluginInstallResult{
		Name:    manifest.Name,
		Version: manifest.Version,
		Source:  sourceConfigString(source),
		Path:    pluginDir,
	}, nil
}

func (m *Manager) installLocal(source Source) (ports.PluginInstallResult, error) {
	manifest, err := LoadManifest(source.LocalPath)
	if err != nil {
		return ports.PluginInstallResult{}, err
	}

	if err := m.addConfigEntry(source); err != nil {
		return ports.PluginInstallResult{}, err
	}

	return ports.PluginInstallResult{
		Name:    manifest.Name,
		Version: manifest.Version,
		Source:  sourceConfigString(source),
		Path:    source.LocalPath,
	}, nil
}

func (m *Manager) fetchAndReset(ctx context.Context, pluginDir string) error {
	if err := m.runGit(ctx, pluginDir, "fetch", "origin"); err != nil {
		return err
	}

	// Get the default branch.
	branch, err := m.defaultBranch(ctx, pluginDir)
	if err != nil {
		return err
	}

	return m.runGit(ctx, pluginDir, "reset", "--hard", "origin/"+branch)
}

func (m *Manager) currentRef(ctx context.Context, pluginDir string) (string, error) {
	cmd := exec.CommandContext(ctx, m.gitCommand, "rev-parse", "HEAD")
	cmd.Dir = pluginDir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (m *Manager) defaultBranch(ctx context.Context, pluginDir string) (string, error) {
	cmd := exec.CommandContext(ctx, m.gitCommand, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	cmd.Dir = pluginDir
	out, err := cmd.Output()
	if err == nil {
		branch := strings.TrimSpace(string(out))
		return strings.TrimPrefix(branch, "origin/"), nil
	}

	cmd = exec.CommandContext(ctx, m.gitCommand, "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = pluginDir
	out, err = cmd.Output()
	if err != nil {
		return "", err
	}
	branch := strings.TrimSpace(string(out))
	if branch == "HEAD" {
		return "", fmt.Errorf("could not determine default branch")
	}
	return branch, nil
}

func (m *Manager) pluginDir(source Source) string {
	if source.Type == SourceTypeLocal {
		return source.LocalPath
	}
	// Use a readable prefix plus a hash to avoid destructive collisions between
	// different repo names that sanitize to the same string.
	name := strings.NewReplacer("/", "-", ":", "-", "@", "-").Replace(source.Repo)
	if len(name) > 80 {
		name = name[:80]
	}
	sum := sha256.Sum256([]byte(source.Repo))
	return filepath.Join(m.dataDir, "plugins", name+"-"+hex.EncodeToString(sum[:])[:12])
}

// pluginName returns the plugin name from the manifest, falling back to
// source.Path if the manifest cannot be loaded.
func (m *Manager) pluginName(source Source) string {
	dir := m.pluginDir(source)
	manifest, err := LoadManifest(dir)
	if err != nil {
		return source.Path
	}
	return manifest.Name
}

func sourceConfigString(source Source) string {
	if source.Type == SourceTypeLocal && source.LocalPath != "" {
		return source.LocalPath
	}
	if source.Raw != "" {
		return source.Raw
	}
	return source.Repo
}

func buildCloneURL(source Source) string {
	if source.Raw == "" {
		return "https://" + source.Repo + ".git"
	}
	raw := source.Raw
	if strings.HasPrefix(raw, "git:") {
		return "https://" + source.Repo + ".git"
	}
	if strings.Contains(raw, "://") || strings.Contains(raw, "@") {
		return stripPinnedRef(raw, source.Ref)
	}
	return "https://" + source.Repo + ".git"
}

func stripPinnedRef(raw, ref string) string {
	if ref == "" {
		return raw
	}
	suffix := "@" + ref
	return strings.TrimSuffix(raw, suffix)
}

// ---------- config file management ----------

func (m *Manager) loadConfig() (*pluginConfig, error) {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &pluginConfig{}, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg pluginConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return &cfg, nil
}

func (m *Manager) saveConfig(cfg *pluginConfig) error {
	if err := os.MkdirAll(filepath.Dir(m.configPath), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(m.configPath, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

func (m *Manager) addConfigEntry(source Source) error {
	cfg, err := m.loadConfig()
	if err != nil {
		return err
	}

	configSource := sourceConfigString(source)
	// Check for duplicate.
	for _, entry := range cfg.Plugins {
		if entry.Source == configSource || entry.Source == source.Raw || entry.Source == source.Repo {
			return fmt.Errorf("plugin source %q is already in config", entry.Source)
		}
	}

	cfg.Plugins = append(cfg.Plugins, pluginEntry{Source: configSource})

	return m.saveConfig(cfg)
}

func (m *Manager) runGit(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, m.gitCommand, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %v: %s: %w", args, strings.TrimSpace(string(out)), err)
	}
	return nil
}
