package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"ero/internal/core"
	"ero/internal/ports"
)

func TestProviderPickerDisplaysDescriptorRowsWithoutStartingInactiveProviders(t *testing.T) {
	controller := &mockActiveProviderController{}
	catalog := []ports.ReviewProviderDescriptor{
		{Key: "github", Label: "GitHub", PluginName: "gh-plugin", PluginSource: "builtin"},
		{Key: "gitlab", Label: "GitLab", PluginName: "gl-plugin", PluginSource: "local"},
	}
	startState := ActiveProviderState{StableProviderKey: "github", RuntimeProviderID: "github", RuntimeInfo: core.ReviewProviderInfo{ID: "github", Label: "GitHub"}, Snapshot: core.ProviderSnapshot{Sync: core.ProviderSyncState{Status: core.ProviderSyncStatusFailed, LastError: "missing token"}}}
	controller.On("Catalog", mock.Anything).Return(catalog, nil).Once()
	controller.On("Start", mock.Anything, mock.Anything).Return(startState, nil).Once()
	m := NewModelWithActiveProviderContext(context.Background(), nil, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, controller, nil)
	updated, _ := m.Update(m.Init()())
	m = updated.(Model)

	updated, _ = m.Update(keyPress("p"))
	m = updated.(Model)

	require.True(t, m.providerPicker.open)
	controller.AssertNumberOfCalls(t, "Start", 1)
	view := stripANSI(m.View().Content)
	require.Contains(t, view, "Provider")
	require.Contains(t, view, "● GitHub")
	require.Contains(t, view, "active")
	require.Contains(t, view, "gh-plugin builtin")
	require.Contains(t, view, "missing token")
	require.Contains(t, view, "GitLab")
	require.Contains(t, view, "gl-plugin local")
	controller.AssertExpectations(t)
}

func TestProviderPickerSelectEmitsSwitchCommandWithStableKey(t *testing.T) {
	controller := &mockActiveProviderController{}
	controller.On("Catalog", mock.Anything).Return([]ports.ReviewProviderDescriptor{{Key: "github", Label: "GitHub"}, {Key: "gitlab", Label: "GitLab"}}, nil).Once()
	controller.On("Start", mock.Anything, mock.Anything).Return(ActiveProviderState{StableProviderKey: "github"}, nil).Once()
	controller.On("Switch", mock.Anything, mock.Anything, "gitlab").Return(ActiveProviderState{StableProviderKey: "gitlab"}, nil).Once()
	m := NewModelWithActiveProviderContext(context.Background(), nil, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, controller, nil)
	updated, _ := m.Update(m.Init()())
	m = updated.(Model)
	updated, _ = m.Update(keyPress("p"))
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	require.NotNil(t, cmd)
	updated, _ = m.Update(cmd())
	m = updated.(Model)

	require.False(t, m.providerPicker.open)
	controller.AssertCalled(t, "Switch", mock.Anything, mock.Anything, "gitlab")
	require.Equal(t, "gitlab", m.activeProviderKey)
	controller.AssertExpectations(t)
}

func TestProviderPickerRendersActiveAndAvailableStates(t *testing.T) {
	model := NewModel(nil)
	model.providerCatalog = []ports.ReviewProviderDescriptor{{Key: "github", Label: "GitHub", PluginName: "gh-plugin", PluginSource: "builtin"}, {Key: "gitlab", Label: "GitLab", PluginName: "gl-plugin", PluginSource: "local"}}
	model.activeProviderKey = "github"
	model.providerPicker = model.openProviderPicker().providerPicker

	view := stripANSI(model.renderProviderPicker(80, 20))

	require.Contains(t, view, "● GitHub")
	require.Contains(t, view, "active")
	require.Contains(t, view, "○ GitLab")
	require.Contains(t, view, "available")
	require.Contains(t, view, "alt+p cycle")
}

func TestProviderPickerAltPCyclesAndClosesPicker(t *testing.T) {
	controller := &mockActiveProviderController{}
	controller.On("Catalog", mock.Anything).Return([]ports.ReviewProviderDescriptor{{Key: "github", Label: "GitHub"}, {Key: "gitlab", Label: "GitLab"}}, nil).Once()
	controller.On("Start", mock.Anything, mock.Anything).Return(ActiveProviderState{StableProviderKey: "github"}, nil).Once()
	controller.On("Switch", mock.Anything, mock.Anything, "gitlab").Return(ActiveProviderState{StableProviderKey: "gitlab"}, nil).Once()
	m := NewModelWithActiveProviderContext(context.Background(), nil, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, controller, nil)
	updated, _ := m.Update(m.Init()())
	m = updated.(Model)
	updated, _ = m.Update(keyPress("p"))
	m = updated.(Model)
	require.True(t, m.providerPicker.open)

	updated, cmd := m.Update(tea.KeyPressMsg{Text: "p", Code: 'p', Mod: tea.ModAlt})
	m = updated.(Model)
	require.NotNil(t, cmd)
	require.False(t, m.providerPicker.open)
	updated, _ = m.Update(cmd())
	m = updated.(Model)

	controller.AssertCalled(t, "Switch", mock.Anything, mock.Anything, "gitlab")
	controller.AssertExpectations(t)
}

func TestProviderCycleAndRefreshShortcuts(t *testing.T) {
	controller := &mockActiveProviderController{}
	controller.On("Catalog", mock.Anything).Return([]ports.ReviewProviderDescriptor{{Key: "github", Label: "GitHub"}, {Key: "gitlab", Label: "GitLab"}}, nil).Once()
	controller.On("Start", mock.Anything, mock.Anything).Return(ActiveProviderState{StableProviderKey: "github"}, nil).Once()
	controller.On("Switch", mock.Anything, mock.Anything, "gitlab").Return(ActiveProviderState{StableProviderKey: "gitlab"}, nil).Once()
	controller.On("Refresh", mock.Anything, mock.Anything, true).Return(ActiveProviderState{StableProviderKey: "github"}, nil).Once()
	m := NewModelWithActiveProviderContext(context.Background(), nil, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, controller, nil)
	updated, _ := m.Update(m.Init()())
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyPressMsg{Text: "p", Code: 'p', Mod: tea.ModAlt})
	m = updated.(Model)
	require.NotNil(t, cmd)
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	controller.AssertCalled(t, "Switch", mock.Anything, mock.Anything, "gitlab")

	_, cmd = m.Update(keyPress("r"))
	require.NotNil(t, cmd)
	_ = cmd()
	controller.AssertCalled(t, "Refresh", mock.Anything, mock.Anything, true)
	controller.AssertExpectations(t)
}
