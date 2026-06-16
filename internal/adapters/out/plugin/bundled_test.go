package pluginadapter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeBundledTestPlugin(t *testing.T, root, dirName, name, contributionID string) string {
	t.Helper()
	pluginDir := filepath.Join(root, dirName)
	require.NoError(t, os.MkdirAll(filepath.Join(pluginDir, "bin"), 0o755))
	manifest := `name = "` + name + `"
version = "0.1.0"
manifest_version = "1"
protocol = "ero.plugin.v1"

[runtime]
command = "./bin/` + name + `"

[[contributions]]
type = "review_provider"
id = "` + contributionID + `"
label = "` + contributionID + `"
`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "ero-plugin.toml"), []byte(manifest), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "bin", name), []byte("#!/bin/sh\ncat\n"), 0o755))
	return pluginDir
}

func TestBundledPluginsDiscoveredGenericallyFromManifestRootsViaEnvOverride(t *testing.T) {
	root := t.TempDir()
	firstDir := writeBundledTestPlugin(t, root, "first", "ero-plugin-first", "first")
	secondDir := writeBundledTestPlugin(t, root, "second", "ero-plugin-second", "second")
	t.Setenv("ERO_BUNDLED_PLUGIN_DIRS", root)

	descriptors := bundledPlugins()
	require.Len(t, descriptors, 2)
	require.Equal(t, "bundled:ero-plugin-first", descriptors[0].Source)
	require.Equal(t, firstDir, descriptors[0].Path)
	require.Equal(t, "first", descriptors[0].Contributions[0].ID)
	require.Equal(t, "bundled:ero-plugin-second", descriptors[1].Source)
	require.Equal(t, secondDir, descriptors[1].Path)
	require.Equal(t, "second", descriptors[1].Contributions[0].ID)
}

func TestBundledPluginsDefaultDiscoveryIncludesMaintainedShippedPlugins(t *testing.T) {
	t.Setenv("ERO_BUNDLED_PLUGIN_DIRS", "")

	descriptors := bundledPlugins()
	require.Len(t, descriptors, 3)
	byName := map[string]bool{}
	for _, descriptor := range descriptors {
		byName[descriptor.Name] = true
		require.True(t, descriptor.Bundled)
		require.NotEmpty(t, descriptor.Path)
		require.Equal(t, bundledSource(descriptor.Name), descriptor.Source)
	}
	require.True(t, byName["ero-plugin-codex"])
	require.True(t, byName["ero-plugin-github"])
	require.True(t, byName["ero-plugin-pi-coding-agent"])
}

func TestBundledCodexManifestUsesPackagedRuntimeLayoutAndLocalBuildCommand(t *testing.T) {
	manifest, err := LoadManifest(filepath.Join("..", "..", "..", "..", "plugins", "codex"))
	require.NoError(t, err)
	require.Equal(t, "./bin/ero-plugin-codex", manifest.Runtime.Command)
	require.Equal(t, "go build -o ./bin/ero-plugin-codex ./cmd/ero-plugin-codex", manifest.Build.Command)
}
