package pluginadapter

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"ero/internal/ports"
	"ero/internal/ports/mocks"
)

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
