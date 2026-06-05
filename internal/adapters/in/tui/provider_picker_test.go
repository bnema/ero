package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"

	"ero/internal/core"
	"ero/internal/ports"
)

func TestProviderPickerDisplaysDescriptorRowsWithoutStartingInactiveProviders(t *testing.T) {
	controller := &fakeActiveProviderController{
		catalog: []ports.ReviewProviderDescriptor{
			{Key: "github", Label: "GitHub", PluginName: "gh-plugin", PluginSource: "builtin"},
			{Key: "gitlab", Label: "GitLab", PluginName: "gl-plugin", PluginSource: "local"},
		},
		startState: ActiveProviderState{StableProviderKey: "github", RuntimeProviderID: "github", RuntimeInfo: core.ReviewProviderInfo{ID: "github", Label: "GitHub"}, Snapshot: core.ProviderSnapshot{Sync: core.ProviderSyncState{Status: core.ProviderSyncStatusFailed, LastError: "missing token"}}},
	}
	m := NewModelWithActiveProviderContext(context.Background(), nil, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, controller, nil)
	updated, _ := m.Update(m.Init()())
	m = updated.(Model)

	updated, _ = m.Update(keyPress("G"))
	m = updated.(Model)

	require.True(t, m.providerPicker.open)
	require.Equal(t, 1, controller.startCalls)
	view := stripANSI(m.View().Content)
	require.Contains(t, view, "Review providers")
	require.Contains(t, view, "* GitHub")
	require.Contains(t, view, "gh-plugin builtin")
	require.Contains(t, view, "missing token")
	require.Contains(t, view, "GitLab")
	require.Contains(t, view, "gl-plugin local")
}

func TestProviderPickerSelectEmitsSwitchCommandWithStableKey(t *testing.T) {
	controller := &fakeActiveProviderController{
		catalog:      []ports.ReviewProviderDescriptor{{Key: "github", Label: "GitHub"}, {Key: "gitlab", Label: "GitLab"}},
		startState:   ActiveProviderState{StableProviderKey: "github"},
		switchStates: map[string]ActiveProviderState{"gitlab": {StableProviderKey: "gitlab"}},
		switchErrs:   map[string]error{},
	}
	m := NewModelWithActiveProviderContext(context.Background(), nil, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, controller, nil)
	updated, _ := m.Update(m.Init()())
	m = updated.(Model)
	updated, _ = m.Update(keyPress("G"))
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	require.NotNil(t, cmd)
	updated, _ = m.Update(cmd())
	m = updated.(Model)

	require.False(t, m.providerPicker.open)
	require.Equal(t, []string{"gitlab"}, controller.switchKeys)
	require.Equal(t, "gitlab", m.activeProviderKey)
}

func TestProviderCycleAndRefreshShortcuts(t *testing.T) {
	controller := &fakeActiveProviderController{
		catalog:      []ports.ReviewProviderDescriptor{{Key: "github", Label: "GitHub"}, {Key: "gitlab", Label: "GitLab"}},
		startState:   ActiveProviderState{StableProviderKey: "github"},
		refreshState: ActiveProviderState{StableProviderKey: "github"},
		switchStates: map[string]ActiveProviderState{"gitlab": {StableProviderKey: "gitlab"}},
		switchErrs:   map[string]error{},
	}
	m := NewModelWithActiveProviderContext(context.Background(), nil, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, controller, nil)
	updated, _ := m.Update(m.Init()())
	m = updated.(Model)

	updated, cmd := m.Update(keyPress("g"))
	m = updated.(Model)
	require.NotNil(t, cmd)
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	require.Equal(t, []string{"gitlab"}, controller.switchKeys)

	_, cmd = m.Update(keyPress("r"))
	require.NotNil(t, cmd)
	_ = cmd()
	require.Equal(t, []bool{true}, controller.refreshManual)
}
