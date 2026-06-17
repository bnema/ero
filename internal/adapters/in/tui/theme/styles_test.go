package theme

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"ero/internal/core"
)

func TestApplyAppearanceKeepsDarkPaletteAsDefault(t *testing.T) {
	ApplyAppearance(core.ThemeAppearanceDark)
	changed := ApplyAppearance(core.ThemeAppearanceDark)
	t.Cleanup(func() { ApplyAppearance(core.ThemeAppearanceDark) })

	assert.False(t, changed)
	assert.Equal(t, core.ThemeAppearanceDark, CurrentAppearance())
	assert.Equal(t, "#c9d1d9", ColorText)
	assert.Equal(t, "#011209", CurrentPalette().AddedLineBg)
	assert.Equal(t, "github-dark", CurrentPalette().ChromaStyle)
}

func TestApplyAppearanceSwitchesToCompleteLightPalette(t *testing.T) {
	ApplyAppearance(core.ThemeAppearanceDark)
	t.Cleanup(func() { ApplyAppearance(core.ThemeAppearanceDark) })

	changed := ApplyAppearance(core.ThemeAppearanceLight)

	assert.True(t, changed)
	assert.Equal(t, core.ThemeAppearanceLight, CurrentAppearance())
	assert.Equal(t, "#24292f", ColorText)
	assert.Equal(t, "#dafbe1", CurrentPalette().AddedLineBg)
	assert.Equal(t, "#0969da", ColorAccent)
	assert.Equal(t, "github", CurrentPalette().ChromaStyle)
	assert.Equal(t, "github", CurrentPalette().MarkdownCodeTheme)
}
