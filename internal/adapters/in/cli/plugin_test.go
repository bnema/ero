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
	assert.Equal(t, "List available plugins", listCmd.Short)
}

func TestPluginCommandLongHelpMentionsBundledCodexPackagingConstraints(t *testing.T) {
	t.Parallel()

	manager := mocks.NewMockPluginLifecycle(t)
	cmd := NewPluginCommand(manager, nil)

	assert.Contains(t, cmd.Long, "packaged releases and source checkouts")
	assert.Contains(t, cmd.Long, "go install ./cmd/ero installs only the ero binary")
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
	assert.Contains(t, out.String(), "No plugins available")
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

// ---------- regression: bundled Codex lifecycle UX ----------

func TestPluginListShowsBundledCodex(t *testing.T) {
	t.Parallel()

	plugins := []ports.InstalledPlugin{
		{Name: "Codex", Version: "bundled", Source: "bundled:ero-plugin-codex", Bundled: true, Contributions: []string{"review_provider:codex"}},
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

	output := stripANSI(out.String())
	assert.Contains(t, output, "Codex vbundled (bundled/default)")
	assert.Contains(t, output, "review_provider:codex")
	assert.Contains(t, output, "bundled:ero-plugin-codex")
	assert.Contains(t, output, "github")
}

func TestPluginRemoveWithBundledCodexFailsClearly(t *testing.T) {
	t.Parallel()

	manager := mocks.NewMockPluginLifecycle(t)
	manager.EXPECT().Remove(mock.Anything, "bundled:ero-plugin-codex").
		Return(ports.PluginRemoveResult{}, errors.New("bundled/default plugin \"bundled:ero-plugin-codex\" is included with ero and cannot be removed"))

	cmd := NewPluginCommand(manager, nil)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"remove", "bundled:ero-plugin-codex"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bundled/default")
	assert.Contains(t, err.Error(), "cannot be removed")
}

func TestPluginUpdateWithBundledCodexReportsSkipped(t *testing.T) {
	t.Parallel()

	manager := mocks.NewMockPluginLifecycle(t)
	manager.EXPECT().Update(mock.Anything, "bundled:ero-plugin-codex").Return([]ports.PluginUpdateResult{{
		Source:  "bundled:ero-plugin-codex",
		Name:    "ero-plugin-codex",
		Message: "bundled/default plugin is included with ero and cannot be updated by plugin update",
	}}, nil)

	cmd := NewPluginCommand(manager, nil)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"update", "bundled:ero-plugin-codex"})

	err := cmd.Execute()
	require.NoError(t, err)
	output := stripANSI(out.String())
	assert.Contains(t, output, "ero-plugin-codex")
	assert.Contains(t, output, "bundled/default")
	assert.Contains(t, output, "cannot be updated")
}

func TestPluginInstallWithBundledCodexIdentifier(t *testing.T) {
	t.Parallel()

	manager := mocks.NewMockPluginLifecycle(t)
	manager.EXPECT().Install(mock.Anything, "bundled:ero-plugin-codex").
		Return(ports.PluginInstallResult{}, errors.New("bundled/default plugin \"bundled:ero-plugin-codex\" is included with ero and cannot be installed"))

	cmd := NewPluginCommand(manager, nil)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"install", "bundled:ero-plugin-codex"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bundled/default")
	assert.Contains(t, err.Error(), "cannot be installed")
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
