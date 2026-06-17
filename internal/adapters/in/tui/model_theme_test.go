package tui

import (
	"context"
	"image/color"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ero/internal/adapters/in/tui/theme"
	"ero/internal/core"
)

func TestModelAutoThemeFollowsBackgroundColorMessages(t *testing.T) {
	theme.ApplyAppearance(core.ThemeAppearanceDark)
	t.Cleanup(func() { theme.ApplyAppearance(core.ThemeAppearanceDark) })

	model := NewModelWithActiveProviderContextConfig(context.Background(), nil, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, nil, nil, ModelConfig{ThemeMode: core.ThemeModeAuto})
	updated, _ := model.Update(tea.BackgroundColorMsg{Color: color.RGBA{R: 255, G: 255, B: 255, A: 255}})
	model = updated.(Model)

	assert.Equal(t, core.ThemeAppearanceLight, model.ThemeAppearance())
	assert.Equal(t, core.ThemeAppearanceLight, theme.CurrentAppearance())

	updated, _ = model.Update(tea.BackgroundColorMsg{Color: color.RGBA{A: 255}})
	model = updated.(Model)

	assert.Equal(t, core.ThemeAppearanceDark, model.ThemeAppearance())
	assert.Equal(t, core.ThemeAppearanceDark, theme.CurrentAppearance())
}

func TestModelForcedThemeIgnoresBackgroundColorMessages(t *testing.T) {
	theme.ApplyAppearance(core.ThemeAppearanceDark)
	t.Cleanup(func() { theme.ApplyAppearance(core.ThemeAppearanceDark) })

	model := NewModelWithActiveProviderContextConfig(context.Background(), nil, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, nil, nil, ModelConfig{ThemeMode: core.ThemeModeDark})
	updated, _ := model.Update(tea.BackgroundColorMsg{Color: color.RGBA{R: 255, G: 255, B: 255, A: 255}})
	model = updated.(Model)

	assert.Equal(t, core.ThemeAppearanceDark, model.ThemeAppearance())
	assert.Equal(t, core.ThemeAppearanceDark, theme.CurrentAppearance())
}

func TestModelIgnoresStaleAutoThemeDetectionTicks(t *testing.T) {
	theme.ApplyAppearance(core.ThemeAppearanceDark)
	t.Cleanup(func() { theme.ApplyAppearance(core.ThemeAppearanceDark) })

	model := NewModelWithActiveProviderContextConfig(context.Background(), nil, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, nil, nil, ModelConfig{ThemeMode: core.ThemeModeAuto})
	initialGeneration := model.themeDetectionGeneration

	updated, _ := model.Update(themeConfigChangedMsg{mode: core.ThemeModeAuto, ok: true})
	model = updated.(Model)
	assert.Equal(t, initialGeneration, model.themeDetectionGeneration)

	updated, _ = model.Update(themeConfigChangedMsg{mode: core.ThemeModeDark, ok: true})
	model = updated.(Model)
	updated, _ = model.Update(themeConfigChangedMsg{mode: core.ThemeModeAuto, ok: true})
	model = updated.(Model)
	require.NotEqual(t, initialGeneration, model.themeDetectionGeneration)

	updated, cmd := model.Update(themeDetectionTickMsg{generation: initialGeneration})
	model = updated.(Model)
	assert.Nil(t, cmd)
	assert.Equal(t, core.ThemeModeAuto, model.ThemeMode())

	updated, cmd = model.Update(themeDetectionTickMsg{generation: model.themeDetectionGeneration})
	model = updated.(Model)
	assert.NotNil(t, cmd)
	assert.Equal(t, core.ThemeModeAuto, model.ThemeMode())
}

func TestModelAppliesLiveThemeConfigChanges(t *testing.T) {
	theme.ApplyAppearance(core.ThemeAppearanceDark)
	t.Cleanup(func() { theme.ApplyAppearance(core.ThemeAppearanceDark) })

	changes := make(chan core.ThemeMode, 1)
	model := NewModelWithActiveProviderContextConfig(context.Background(), nil, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, nil, nil, ModelConfig{ThemeMode: core.ThemeModeDark, ThemeModeChanges: changes})
	changes <- core.ThemeModeLight

	msg := model.watchThemeConfigCmd()()
	changed, ok := msg.(themeConfigChangedMsg)
	require.True(t, ok)
	require.True(t, changed.ok)
	assert.Equal(t, core.ThemeModeLight, changed.mode)

	updated, cmd := model.Update(changed)
	model = updated.(Model)

	assert.Equal(t, core.ThemeModeLight, model.ThemeMode())
	assert.Equal(t, core.ThemeAppearanceLight, model.ThemeAppearance())
	assert.Equal(t, core.ThemeAppearanceLight, theme.CurrentAppearance())
	assert.NotNil(t, cmd)
}
