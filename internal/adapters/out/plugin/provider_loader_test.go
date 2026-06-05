package pluginadapter

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"ero/internal/ports"
	"ero/internal/ports/mocks"
)

func TestReviewProviderLoaderListsReviewProviderDescriptorsWithoutStartingClients(t *testing.T) {
	t.Parallel()

	clientFactoryCalls := 0
	multiDescriptor := ports.PluginDescriptor{
		Name:    "multi",
		Version: "0.1.0",
		Source:  "git:example.com/owner/multi@v0.1.0",
		Path:    "plugins/multi",
		Contributions: []ports.PluginContribution{
			{Type: "review_provider", ID: "github", Label: "GitHub"},
			{Type: "theme", ID: "dark", Label: "Dark"},
			{Type: "review_provider", ID: "gitlab", Label: "GitLab"},
		},
	}
	registry := mocks.NewMockPluginRegistry(t)
	registry.EXPECT().InstalledPlugins(context.Background()).Return([]ports.PluginDescriptor{multiDescriptor, {
		Name:    "other",
		Version: "2.0.0",
		Source:  "git:example.com/owner/other@v2.0.0",
		Path:    "plugins/other",
		Contributions: []ports.PluginContribution{
			{Type: "workflow", ID: "triage", Label: "Triage"},
		},
	}}, nil)

	loader := NewReviewProviderLoader(registry)
	loader.clientFactory = func(context.Context, ports.ReviewProviderDescriptor) (ports.ReviewProviderClient, error) {
		clientFactoryCalls++
		return nil, nil
	}

	descriptors, err := loader.ListReviewProviderDescriptors(context.Background())
	require.NoError(t, err)
	require.Equal(t, []ports.ReviewProviderDescriptor{{
		Key:            stableReviewProviderKey(multiDescriptor, multiDescriptor.Contributions[0]),
		PluginName:     "multi",
		PluginVersion:  "0.1.0",
		PluginSource:   "git:example.com/owner/multi@v0.1.0",
		PluginPath:     "plugins/multi",
		ContributionID: "github",
		Label:          "GitHub",
		Type:           "review_provider",
	}, {
		Key:            stableReviewProviderKey(multiDescriptor, multiDescriptor.Contributions[2]),
		PluginName:     "multi",
		PluginVersion:  "0.1.0",
		PluginSource:   "git:example.com/owner/multi@v0.1.0",
		PluginPath:     "plugins/multi",
		ContributionID: "gitlab",
		Label:          "GitLab",
		Type:           "review_provider",
	}}, descriptors)
	require.Zero(t, clientFactoryCalls)
}

func TestStableReviewProviderKeyUsesCanonicalInstalledPluginIdentity(t *testing.T) {
	t.Parallel()

	first := ports.PluginDescriptor{Name: "same", Version: "1.0.0", Source: "git:example.com/owner/one@v1"}
	second := ports.PluginDescriptor{Name: "same", Version: "1.0.0", Source: "git:example.com/owner/two@v1"}
	contribution := ports.PluginContribution{Type: "review_provider", ID: "github", Label: "GitHub"}

	firstKey := stableReviewProviderKey(first, contribution)
	secondKey := stableReviewProviderKey(second, contribution)

	require.NotEqual(t, firstKey, secondKey)
	require.NotContains(t, firstKey, "same@1.0.0")
	require.Contains(t, firstKey, "#review_provider:github")
}

func TestReviewProviderLoaderBuildsMissingRuntimeBeforeStartingProvider(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	buildScript := filepath.Join(dir, "build-runtime.sh")
	err := os.WriteFile(buildScript, []byte("#!/bin/sh\ncat > ./runtime-plugin <<'EOF'\n#!/bin/sh\ncat\nEOF\nchmod +x ./runtime-plugin\n"), 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "ero-plugin.toml"), []byte(`name = "buildable"
version = "0.1.0"
manifest_version = "1"
protocol = "ero.plugin.v1"

[runtime]
command = "./runtime-plugin"

[build]
command = "`+buildScript+`"

[[contributions]]
type = "review_provider"
id = "pi-coding-agent"
label = "pi-coding-agent"
`), 0o644)
	require.NoError(t, err)

	registry := mocks.NewMockPluginRegistry(t)
	registry.EXPECT().InstalledPlugins(context.Background()).Return([]ports.PluginDescriptor{{
		Name: "buildable",
		Path: dir,
		Contributions: []ports.PluginContribution{
			{Type: "review_provider", ID: "pi-coding-agent", Label: "pi-coding-agent"},
		},
	}}, nil)

	providers, err := NewReviewProviderLoader(registry).LoadReviewProviders(context.Background())
	require.NoError(t, err)
	require.Len(t, providers, 1)
	require.FileExists(t, filepath.Join(dir, "runtime-plugin"))
	require.NoError(t, providers[0].Close())
}

func TestReviewProviderLoaderRebuildsStaleLocalRuntimeBeforeStartingProvider(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	runtimePath := filepath.Join(dir, "runtime-plugin")
	buildMarker := filepath.Join(dir, "build-ran")
	buildScript := filepath.Join(dir, "build-runtime.sh")
	err := os.WriteFile(buildScript, []byte("#!/bin/sh\necho rebuilt > ./build-ran\ncat > ./runtime-plugin <<'EOF'\n#!/bin/sh\ncat\nEOF\nchmod +x ./runtime-plugin\n"), 0o755)
	require.NoError(t, err)
	err = os.WriteFile(runtimePath, []byte("#!/bin/sh\necho stale-runtime\n"), 0o755)
	require.NoError(t, err)
	oldTime := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(runtimePath, oldTime, oldTime))
	err = os.WriteFile(filepath.Join(dir, "cmd-source.go"), []byte("package main\n"), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(dir, "ero-plugin.toml"), []byte(`name = "buildable"
version = "0.1.0"
manifest_version = "1"
protocol = "ero.plugin.v1"

[runtime]
command = "./runtime-plugin"

[build]
command = "`+buildScript+`"

[[contributions]]
type = "review_provider"
id = "github"
label = "GitHub"
`), 0o644)
	require.NoError(t, err)

	registry := mocks.NewMockPluginRegistry(t)
	registry.EXPECT().InstalledPlugins(context.Background()).Return([]ports.PluginDescriptor{{
		Name: "buildable",
		Path: dir,
		Contributions: []ports.PluginContribution{
			{Type: "review_provider", ID: "github", Label: "GitHub"},
		},
	}}, nil)

	providers, err := NewReviewProviderLoader(registry).LoadReviewProviders(context.Background())
	require.NoError(t, err)
	require.Len(t, providers, 1)
	require.FileExists(t, buildMarker)
	require.NoError(t, providers[0].Close())
}

func TestReviewProviderLoaderStartsOneClientPerReviewProviderContribution(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "ero-plugin.toml"), []byte(`name = "multi"
version = "0.1.0"
manifest_version = "1"
protocol = "ero.plugin.v1"

[runtime]
command = "cat"

[[contributions]]
type = "review_provider"
id = "github"
label = "GitHub"

[[contributions]]
type = "review_provider"
id = "gitlab"
label = "GitLab"
`), 0o644)
	require.NoError(t, err)

	registry := mocks.NewMockPluginRegistry(t)
	registry.EXPECT().InstalledPlugins(context.Background()).Return([]ports.PluginDescriptor{{
		Name: "multi",
		Path: dir,
		Contributions: []ports.PluginContribution{
			{Type: "review_provider", ID: "github", Label: "GitHub"},
			{Type: "review_provider", ID: "gitlab", Label: "GitLab"},
			{Type: "other", ID: "ignored", Label: "Ignored"},
		},
	}}, nil)

	providers, err := NewReviewProviderLoader(registry).LoadReviewProviders(context.Background())
	require.NoError(t, err)
	require.Len(t, providers, 2)
	for _, provider := range providers {
		require.NoError(t, provider.Close())
	}
}
