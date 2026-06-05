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
	"time"

	"github.com/bnema/zerowrap"
	"github.com/google/shlex"

	"ero/internal/ports"
	pluginsdk "ero/pkg/plugin"
)

// ReviewProviderLoader builds review provider clients from installed plugin manifests.
type ReviewProviderLoader struct {
	registry      ports.PluginRegistry
	timeout       time.Duration
	clientFactory func(context.Context, ports.ReviewProviderDescriptor) (ports.ReviewProviderClient, error)
}

// NewReviewProviderLoader creates a loader backed by an installed plugin registry.
func NewReviewProviderLoader(registry ports.PluginRegistry) *ReviewProviderLoader {
	loader := &ReviewProviderLoader{registry: registry, timeout: DefaultPluginTimeout}
	loader.clientFactory = loader.createReviewProviderClient
	return loader
}

// ListReviewProviderDescriptors implements ports.ReviewProviderCatalog.
func (l *ReviewProviderLoader) ListReviewProviderDescriptors(ctx context.Context) ([]ports.ReviewProviderDescriptor, error) {
	descriptors, err := l.registry.InstalledPlugins(ctx)
	if err != nil {
		return nil, err
	}
	providers := make([]ports.ReviewProviderDescriptor, 0)
	for _, descriptor := range descriptors {
		for _, contribution := range descriptor.Contributions {
			if contribution.Type != pluginsdk.ContributionReviewProvider {
				continue
			}
			providers = append(providers, ports.ReviewProviderDescriptor{
				Key:            stableReviewProviderKey(descriptor, contribution),
				PluginName:     descriptor.Name,
				PluginVersion:  descriptor.Version,
				PluginSource:   descriptor.Source,
				PluginPath:     descriptor.Path,
				ContributionID: contribution.ID,
				Label:          contribution.Label,
				Type:           contribution.Type,
			})
		}
	}
	return providers, nil
}

// CreateReviewProviderClient implements ports.ReviewProviderClientFactory.
func (l *ReviewProviderLoader) CreateReviewProviderClient(ctx context.Context, descriptor ports.ReviewProviderDescriptor) (ports.ReviewProviderClient, error) {
	return l.clientFactory(ctx, descriptor)
}

// LoadReviewProviders implements ports.ReviewProviderLoader as a temporary compatibility shim.
func (l *ReviewProviderLoader) LoadReviewProviders(ctx context.Context) ([]ports.ReviewProviderClient, error) {
	descriptors, err := l.ListReviewProviderDescriptors(ctx)
	if err != nil {
		return nil, err
	}
	log := zerowrap.FromCtx(ctx)
	providers := make([]ports.ReviewProviderClient, 0, len(descriptors))
	for _, descriptor := range descriptors {
		client, err := l.CreateReviewProviderClient(ctx, descriptor)
		if err != nil {
			log.Warn().Err(err).Str("plugin_path", descriptor.PluginPath).Str("contribution_id", descriptor.ContributionID).Msg("create plugin review provider client failed")
			continue
		}
		providers = append(providers, client)
	}
	return providers, nil
}

func (l *ReviewProviderLoader) createReviewProviderClient(ctx context.Context, descriptor ports.ReviewProviderDescriptor) (ports.ReviewProviderClient, error) {
	manifest, err := LoadManifest(descriptor.PluginPath)
	if err != nil {
		return nil, err
	}
	command, args := splitRuntimeCommand(manifest.Runtime.Command)
	if command == "" {
		return nil, fmt.Errorf("plugin runtime command is empty")
	}
	if shouldBuildRuntime(command, descriptor.PluginPath, manifest.Build.Command) {
		if err := runPluginBuildCommand(ctx, descriptor.PluginPath, manifest.Build.Command, l.timeout); err != nil {
			log := zerowrap.FromCtx(ctx)
			log.Warn().Err(err).Str("plugin_path", descriptor.PluginPath).Msg("build plugin runtime failed")
		}
	}
	if !strings.Contains(command, "/") {
		if resolved, err := exec.LookPath(command); err == nil {
			command = resolved
		}
	}
	return NewClientForContribution(command, args, descriptor.PluginPath, descriptor.ContributionID, l.timeout)
}

func stableReviewProviderKey(descriptor ports.PluginDescriptor, contribution ports.PluginContribution) string {
	identity := canonicalInstalledPluginIdentity(descriptor)
	digest := sha256.Sum256([]byte(identity))
	return "plugin:" + hex.EncodeToString(digest[:8]) + "#review_provider:" + contribution.ID
}

func canonicalInstalledPluginIdentity(descriptor ports.PluginDescriptor) string {
	if source, err := ParseSource(descriptor.Source); err == nil {
		switch source.Type {
		case SourceTypeGit:
			return strings.Join([]string{"git", strings.ToLower(source.Host), strings.TrimSuffix(source.Path, ".git"), source.Ref}, ":")
		case SourceTypeLocal:
			return "local:" + filepath.Clean(source.LocalPath)
		}
	}
	if descriptor.Path != "" {
		if abs, err := filepath.Abs(descriptor.Path); err == nil {
			return "path:" + filepath.Clean(abs)
		}
		return "path:" + filepath.Clean(descriptor.Path)
	}
	return strings.Join([]string{"manifest", descriptor.Name, descriptor.Version, descriptor.Source}, ":")
}

func runtimeCommandAvailable(command, pluginDir string) bool {
	_, ok := runtimeCommandInfo(command, pluginDir)
	return ok
}

func runtimeCommandInfo(command, pluginDir string) (os.FileInfo, bool) {
	if command == "" {
		return nil, false
	}
	if !strings.Contains(command, "/") {
		_, err := exec.LookPath(command)
		return nil, err == nil
	}
	path := runtimeCommandPath(command, pluginDir)
	info, err := os.Stat(path)
	return info, err == nil && !info.IsDir()
}

func runtimeCommandPath(command, pluginDir string) string {
	path := command
	if !filepath.IsAbs(path) {
		path = filepath.Join(pluginDir, path)
	}
	return filepath.Clean(path)
}

func shouldBuildRuntime(command, pluginDir, buildCommand string) bool {
	buildCommand = strings.TrimSpace(buildCommand)
	if buildCommand == "" {
		return false
	}
	runtimeInfo, available := runtimeCommandInfo(command, pluginDir)
	if !available {
		return true
	}
	if !strings.Contains(command, "/") {
		return false
	}
	return pluginSourceNewerThanRuntime(pluginDir, runtimeCommandPath(command, pluginDir), runtimeInfo.ModTime(), buildCommand)
}

func pluginSourceNewerThanRuntime(pluginDir, runtimePath string, runtimeModTime time.Time, buildCommand string) bool {
	runtimePath = filepath.Clean(runtimePath)
	newer := false
	_ = filepath.WalkDir(pluginDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || newer {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		path = filepath.Clean(path)
		if path == runtimePath || !isPluginSourcePath(path) {
			return nil
		}
		info, err := d.Info()
		if err == nil && info.ModTime().After(runtimeModTime) {
			newer = true
		}
		return nil
	})
	if newer {
		return true
	}
	buildCommandName, _ := splitRuntimeCommand(buildCommand)
	if buildCommandName == "" || !strings.Contains(buildCommandName, "/") {
		return false
	}
	buildPath := runtimeCommandPath(buildCommandName, pluginDir)
	info, err := os.Stat(buildPath)
	return err == nil && !info.IsDir() && info.ModTime().After(runtimeModTime)
}

func isPluginSourcePath(path string) bool {
	switch filepath.Base(path) {
	case "ero-plugin.toml", "go.mod", "go.sum":
		return true
	}
	return filepath.Ext(path) == ".go"
}

func runPluginBuildCommand(ctx context.Context, pluginDir, buildCommand string, timeout time.Duration) error {
	command, args := splitRuntimeCommand(buildCommand)
	if command == "" {
		return fmt.Errorf("plugin build command is empty")
	}
	if timeout <= 0 {
		timeout = DefaultPluginTimeout
	}
	buildCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(buildCtx, command, args...)
	cmd.Dir = pluginDir
	output, err := cmd.CombinedOutput()
	if buildCtx.Err() != nil {
		return buildCtx.Err()
	}
	if err != nil {
		if len(output) > 0 {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
		}
		return err
	}
	return nil
}

func splitRuntimeCommand(command string) (string, []string) {
	fields, err := shlex.Split(command)
	if err != nil {
		fields = strings.Fields(command)
	}
	if len(fields) == 0 {
		return "", nil
	}
	return fields[0], fields[1:]
}
