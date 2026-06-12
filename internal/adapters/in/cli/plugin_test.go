package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"ero/internal/ports"
	"ero/internal/ports/mocks"
)

func TestPluginCommandRegisteredUnderParent(t *testing.T) {
	t.Parallel()

	manager := mocks.NewMockPluginLifecycle(t)
	cmd := NewPluginCommand(manager, nil)
	require.NotNil(t, cmd)

	assert.Equal(t, "plugin", cmd.Use)

	listCmd, _, err := cmd.Find([]string{"list"})
	require.NoError(t, err)
	assert.Equal(t, "list", listCmd.Use)
	assert.Equal(t, "List installed plugins", listCmd.Short)
}

func TestPluginListEmpty(t *testing.T) {
	t.Parallel()

	manager := mocks.NewMockPluginLifecycle(t)
	manager.EXPECT().List(mock.Anything).Return(nil, nil)
	cmd := NewPluginCommand(manager, nil)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"list"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, out.String(), "No plugins installed")
}

func TestPluginListHumanOutput(t *testing.T) {
	t.Parallel()

	plugins := []ports.InstalledPlugin{
		{
			Name:          "github",
			Version:       "0.1.0",
			Source:        "git:github.com/ero-plugins/github@v0.1.0",
			Path:          "/data/plugins/github",
			Contributions: []string{"review_provider:github"},
		},
		{
			Name:          "pi-coding-agent",
			Version:       "0.2.0",
			Source:        "/home/user/dev/ero-plugin-pi-coding-agent",
			Path:          "/home/user/dev/ero-plugin-pi-coding-agent",
			Contributions: []string{"review_provider:pi-coding-agent"},
		},
	}
	manager := mocks.NewMockPluginLifecycle(t)
	manager.EXPECT().List(mock.Anything).Return(plugins, nil)

	cmd := NewPluginCommand(manager, nil)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"list"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := out.String()
	assert.Contains(t, output, "github v0.1.0")
	assert.Contains(t, output, "review_provider:github")
	assert.Contains(t, output, "git:github.com/ero-plugins/github@v0.1.0")
	assert.Contains(t, output, "pi-coding-agent v0.2.0")
}

func TestPluginListJSONOutput(t *testing.T) {
	t.Parallel()

	plugins := []ports.InstalledPlugin{{Name: "github", Version: "0.1.0", Source: "git:github.com/ero-plugins/github@v0.1.0", Contributions: []string{"review_provider:github"}}}
	manager := mocks.NewMockPluginLifecycle(t)
	manager.EXPECT().List(mock.Anything).Return(plugins, nil)

	cmd := NewPluginCommand(manager, nil)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"list", "--json"})

	err := cmd.Execute()
	require.NoError(t, err)

	var result []ports.InstalledPlugin
	err = json.Unmarshal(out.Bytes(), &result)
	require.NoError(t, err)

	require.Len(t, result, 1)
	assert.Equal(t, "github", result[0].Name)
	assert.Equal(t, "0.1.0", result[0].Version)
}

func TestPluginInstallHumanOutput(t *testing.T) {
	t.Parallel()

	result := ports.PluginInstallResult{Name: "github", Version: "0.1.0", Source: "git:github.com/ero-plugins/github@v0.1.0", Path: "/data/plugins/github"}
	manager := mocks.NewMockPluginLifecycle(t)
	manager.EXPECT().Install(mock.Anything, "git:github.com/ero-plugins/github@v0.1.0").Return(result, nil)

	cmd := NewPluginCommand(manager, nil)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"install", "git:github.com/ero-plugins/github@v0.1.0"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := stripANSI(out.String())
	assert.Contains(t, output, "Installed plugin github v0.1.0")
	assert.Contains(t, output, "git:github.com/ero-plugins/github@v0.1.0")
}

func TestPluginInstallJSONOutput(t *testing.T) {
	t.Parallel()

	result := ports.PluginInstallResult{Name: "github", Version: "0.1.0", Source: "git:github.com/ero-plugins/github@v0.1.0", Path: "/data/plugins/github"}
	manager := mocks.NewMockPluginLifecycle(t)
	manager.EXPECT().Install(mock.Anything, "git:github.com/ero-plugins/github@v0.1.0").Return(result, nil)

	cmd := NewPluginCommand(manager, nil)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"install", "--json", "git:github.com/ero-plugins/github@v0.1.0"})

	err := cmd.Execute()
	require.NoError(t, err)

	var decoded ports.PluginInstallResult
	err = json.Unmarshal(out.Bytes(), &decoded)
	require.NoError(t, err)
	assert.Equal(t, "github", decoded.Name)
	assert.Equal(t, "0.1.0", decoded.Version)
}

func TestPluginInstallMissingArgs(t *testing.T) {
	t.Parallel()

	manager := mocks.NewMockPluginLifecycle(t)
	cmd := NewPluginCommand(manager, nil)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"install"})

	err := cmd.Execute()
	require.Error(t, err)
}

func TestPluginUpdateHumanOutput(t *testing.T) {
	t.Parallel()

	results := []ports.PluginUpdateResult{
		{Name: "github", PreviousRef: "abc1234def", UpdatedRef: "xyz5678abc"},
		{Name: "pi-coding-agent", Message: "pinned to v0.1.0, skipping update"},
	}
	manager := mocks.NewMockPluginLifecycle(t)
	manager.EXPECT().Update(mock.Anything, "").Return(results, nil)

	cmd := NewPluginCommand(manager, nil)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"update"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := stripANSI(out.String())
	assert.Contains(t, output, "github abc1234 → xyz5678")
	assert.Contains(t, output, "pi-coding-agent — pinned")
}

func TestPluginUpdateJSONOutput(t *testing.T) {
	t.Parallel()

	results := []ports.PluginUpdateResult{{Name: "github", PreviousRef: "abc1234def", UpdatedRef: "xyz5678abc"}}
	manager := mocks.NewMockPluginLifecycle(t)
	manager.EXPECT().Update(mock.Anything, "").Return(results, nil)

	cmd := NewPluginCommand(manager, nil)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"update", "--json"})

	err := cmd.Execute()
	require.NoError(t, err)

	var decoded []ports.PluginUpdateResult
	err = json.Unmarshal(out.Bytes(), &decoded)
	require.NoError(t, err)
	require.Len(t, decoded, 1)
	assert.Equal(t, "github", decoded[0].Name)
}

func TestPluginRemoveHumanOutput(t *testing.T) {
	t.Parallel()

	result := ports.PluginRemoveResult{Name: "github", Source: "git:github.com/ero-plugins/github@v0.1.0", RemovedRepo: true}
	manager := mocks.NewMockPluginLifecycle(t)
	manager.EXPECT().Remove(mock.Anything, "github").Return(result, nil)

	cmd := NewPluginCommand(manager, nil)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"remove", "github"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, stripANSI(out.String()), "Removed plugin github")
}

func TestPluginRemoveJSONOutput(t *testing.T) {
	t.Parallel()

	result := ports.PluginRemoveResult{Name: "github", Source: "git:github.com/ero-plugins/github@v0.1.0", RemovedRepo: false}
	manager := mocks.NewMockPluginLifecycle(t)
	manager.EXPECT().Remove(mock.Anything, "github").Return(result, nil)

	cmd := NewPluginCommand(manager, nil)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"remove", "--json", "github"})

	err := cmd.Execute()
	require.NoError(t, err)

	var decoded ports.PluginRemoveResult
	err = json.Unmarshal(out.Bytes(), &decoded)
	require.NoError(t, err)
	assert.Equal(t, "github", decoded.Name)
	assert.False(t, decoded.RemovedRepo)
}

func TestPluginUpdateFiltered(t *testing.T) {
	t.Parallel()

	results := []ports.PluginUpdateResult{{Name: "github", PreviousRef: "abc", UpdatedRef: "def"}}
	manager := mocks.NewMockPluginLifecycle(t)
	manager.EXPECT().Update(mock.Anything, "git:github.com/ero-plugins/github").Return(results, nil)

	cmd := NewPluginCommand(manager, nil)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"update", "git:github.com/ero-plugins/github"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, stripANSI(out.String()), "github")
}

func stripANSI(s string) string {
	return regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(s, "")
}

// ---------- regression: builtin providers are not managed by plugin lifecycle ----------

func TestPluginListOnlyShowsInstalledPlugins(t *testing.T) {
	t.Parallel()

	plugins := []ports.InstalledPlugin{
		{Name: "github", Version: "0.1.0", Source: "git:github.com/ero-plugins/github@v0.1.0", Contributions: []string{"review_provider:github"}},
	}
	manager := mocks.NewMockPluginLifecycle(t)
	manager.EXPECT().List(mock.Anything).Return(plugins, nil)

	cmd := NewPluginCommand(manager, nil)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"list"})

	err := cmd.Execute()
	require.NoError(t, err)

	// The installed plugin is present in the output.
	assert.Contains(t, out.String(), "github")
	// Builtin provider identifiers MUST NOT appear: PluginLifecycle.List only
	// returns plugins tracked in the config file, not builtin descriptors.
	assert.NotContains(t, out.String(), "builtin:", "builtin provider keys must not appear in plugin list")
	assert.NotContains(t, out.String(), "Codex", "builtin provider names must not appear in plugin list")
}

func TestPluginRemoveWithBuiltinKeyFails(t *testing.T) {
	t.Parallel()

	manager := mocks.NewMockPluginLifecycle(t)
	// A builtin provider key is not known to PluginLifecycle, so Remove returns
	// a "not found" error — the same as any other unknown name or source.
	manager.EXPECT().Remove(mock.Anything, "builtin:codex").
		Return(ports.PluginRemoveResult{}, errors.New("plugin \"builtin:codex\" not found in config"))

	cmd := NewPluginCommand(manager, nil)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"remove", "builtin:codex"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestPluginUpdateWithBuiltinSourceFails(t *testing.T) {
	t.Parallel()

	manager := mocks.NewMockPluginLifecycle(t)
	// A builtin source is not tracked in the plugin config, so Update returns
	// no results with no error — no installed plugin matched the filter.
	manager.EXPECT().Update(mock.Anything, "builtin:codex").Return(nil, nil)

	cmd := NewPluginCommand(manager, nil)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"update", "builtin:codex"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, stripANSI(out.String()), "No plugins to update")
}

func TestPluginInstallWithBuiltinIdentifier(t *testing.T) {
	t.Parallel()

	manager := mocks.NewMockPluginLifecycle(t)
	// The install command does not treat builtin provider identifiers specially.
	// It delegates directly to PluginLifecycle.Install with the user-supplied
	// argument. A builtin provider key is not a valid plugin source, so Install
	// returns an error — the same as any other unresolvable source.
	manager.EXPECT().Install(mock.Anything, "builtin:codex").
		Return(ports.PluginInstallResult{}, errors.New("unsupported plugin source: builtin:codex"))

	cmd := NewPluginCommand(manager, nil)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"install", "builtin:codex"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported plugin source")
}

func TestPluginCommandWiresContext(t *testing.T) {
	t.Parallel()

	manager := mocks.NewMockPluginLifecycle(t)
	cmd := NewPluginCommand(manager, nil)
	require.NotNil(t, cmd)

	assert.Len(t, cmd.Commands(), 4)
	names := make([]string, len(cmd.Commands()))
	for i, c := range cmd.Commands() {
		names[i] = c.Use
	}

	for _, name := range []string{"list", "install", "update", "remove"} {
		found := false
		for _, n := range names {
			if strings.HasPrefix(n, name) {
				found = true
				break
			}
		}
		assert.True(t, found, "expected subcommand %q", name)
	}
}
