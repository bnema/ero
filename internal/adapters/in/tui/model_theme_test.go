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

func TestModelAutoUsesInitialSystemThemePreference(t *testing.T) {
	tests := []struct {
		name       string
		preference core.SystemThemePreference
		want       core.ThemeAppearance
	}{
		{name: "dark system", preference: core.SystemThemePreferDark, want: core.ThemeAppearanceDark},
		{name: "light system", preference: core.SystemThemePreferLight, want: core.ThemeAppearanceLight},
		{name: "unknown system falls back light", preference: core.SystemThemeUnknown, want: core.ThemeAppearanceLight},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalAppearance := theme.CurrentAppearance()
			t.Cleanup(func() { theme.ApplyAppearance(originalAppearance) })

			model := NewModelWithActiveProviderContextConfig(context.Background(), nil, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, nil, nil, ModelConfig{ThemeMode: core.ThemeModeAuto, SystemTheme: tt.preference})

			assert.Equal(t, tt.want, model.ThemeAppearance())
		})
	}
}

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

func TestModelLightViewSetsLightBackgroundForUnstyledAreas(t *testing.T) {
	theme.ApplyAppearance(core.ThemeAppearanceDark)
	t.Cleanup(func() { theme.ApplyAppearance(core.ThemeAppearanceDark) })

	model := NewModelWithActiveProviderContextConfig(context.Background(), []core.ReviewFile{{
		Path:     "demo.go",
		Sections: []core.ReviewSection{{ID: "changed", Kind: core.SectionKindChanged, Lines: []core.ReviewLine{{NewLineNumber: 1, Kind: core.LineKindUnchanged, Content: "short"}}}},
	}}, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, nil, nil, ModelConfig{ThemeMode: core.ThemeModeLight})

	view := model.View()

	require.NotNil(t, view.BackgroundColor)
	assert.Contains(t, view.Content, "48;2;234;238;242")
	assert.Contains(t, view.Content, "48;5;252")
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
