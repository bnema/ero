package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	"ero/internal/core"
)

func TestModelHelpModalShowsShortcutsAndCloses(t *testing.T) {
	t.Parallel()

	model := NewModel([]core.ReviewFile{reviewFile("demo.go", "package main")})
	updated, _ := model.Update(keyPress("?"))
	model = updated.(Model)

	view := stripANSI(model.View().Content)
	assert.Contains(t, view, "Keyboard shortcuts")
	assert.Contains(t, view, "find file")
	assert.Contains(t, view, "grep references")
	assert.Contains(t, view, "select result")
	assert.Contains(t, view, "a             expand all")
	assert.Contains(t, view, "expand more")
	assert.Contains(t, view, "switch provider")
	assert.Contains(t, view, "cycle provider")
	assert.NotContains(t, view, "g/G")
	assert.Contains(t, view, "refresh provider")
	assert.Contains(t, view, "PR sheet")
	assert.NotContains(t, view, "a/b")

	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	model = updated.(Model)

	assert.False(t, model.helpActive)
	assert.NotContains(t, stripANSI(model.View().Content), "Keyboard shortcuts")
}

func TestModelHelpModalQQuitsInsteadOfClosingHelp(t *testing.T) {
	t.Parallel()

	model := NewModel([]core.ReviewFile{reviewFile("demo.go", "package main")})
	updated, _ := model.Update(keyPress("?"))
	model = updated.(Model)

	updated, cmd := model.Update(keyPress("q"))
	model = updated.(Model)

	assert.True(t, model.helpActive)
	assert.NotNil(t, cmd)
}

func TestModelSearchAcceptsQuestionMarkInput(t *testing.T) {
	t.Parallel()

	model := NewModel([]core.ReviewFile{reviewFile("demo.go", "why?")})
	updated, _ := model.Update(keyPress("f"))
	model = updated.(Model)
	updated, _ = model.Update(keyPress("?"))
	model = updated.(Model)

	assert.True(t, model.search.active())
	assert.False(t, model.helpActive)
	assert.Equal(t, "?", model.search.query())
}

func TestRenderHelpPaneFitsWidthAndHeight(t *testing.T) {
	t.Parallel()

	view := stripANSI(renderHelpPane(40, 12))
	lines := strings.Split(view, "\n")

	assert.LessOrEqual(t, len(lines), 12)
	assert.Contains(t, view, "Keyboard shortcuts")
	assert.Contains(t, view, "Review")
	assert.Contains(t, view, "switch diff")
	assert.Contains(t, view, "close help")
}
