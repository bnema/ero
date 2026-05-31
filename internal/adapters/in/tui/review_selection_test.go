package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"ero/internal/adapters/in/tui/presenter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ero/internal/core"
)

func TestReviewDocumentHighlightsSelectedContextOnlyInSelectedFile(t *testing.T) {
	t.Parallel()

	files := []core.ReviewFile{
		{
			Path: "a.go",
			Sections: []core.ReviewSection{
				{ID: "a-context", Kind: core.SectionKindContext, Lines: []core.ReviewLine{{NewLineNumber: 1, Content: "a hidden", Kind: core.LineKindUnchanged}}},
			},
		},
		{
			Path: "b.go",
			Sections: []core.ReviewSection{
				{ID: "b-context", Kind: core.SectionKindContext, Lines: []core.ReviewLine{{NewLineNumber: 1, Content: "b hidden", Kind: core.LineKindUnchanged}}},
			},
		},
	}

	rendered := renderReviewForTest(files, 80, 1, 0, presenter.ReviewAnnotations{}).Content
	lines := strings.Split(rendered, "\n")

	assert.NotContains(t, lines[2], "\x1b[1;")
	assert.Contains(t, lines[6], "\x1b[1;")
}

func TestModelNavigationRerendersWhenNearestContextSelectionChanges(t *testing.T) {
	t.Parallel()

	model := NewModel([]core.ReviewFile{contextBarReviewFile("demo.go")})
	model.cursorRow = expanderRowForSection(t, model, 0, 0)
	model.selectNearestContextToCursor()
	model.syncReviewViewport()
	firstView := model.reviewViewport.View()
	require.Equal(t, 0, model.selectedContext)
	require.Contains(t, firstView, "\x1b[1;")

	model.cursorRow = expanderRowForSection(t, model, 0, 2) - 1
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(Model)

	require.Equal(t, 1, model.selectedContext)
	secondView := model.reviewViewport.View()
	assert.Contains(t, secondView, "\x1b[1;")
	assert.NotEqual(t, firstView, secondView)

	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(Model)
	assert.Equal(t, 2, model.files[0].Sections[2].ExpandedAbove)
	assert.Equal(t, 0, model.files[0].Sections[0].ExpandedAbove)
}

func TestModelSelectedContextHighlightSurvivesGrepJumpThatExpandsEarlierContext(t *testing.T) {
	t.Parallel()

	model := NewModel([]core.ReviewFile{{
		Path: "demo.go",
		Sections: []core.ReviewSection{
			{ID: "first", Kind: core.SectionKindContext, Lines: []core.ReviewLine{{NewLineNumber: 1, Content: "needle", Kind: core.LineKindUnchanged}}},
			{ID: "changed", Kind: core.SectionKindChanged, Lines: []core.ReviewLine{{NewLineNumber: 2, Content: "changed", Kind: core.LineKindAdded}}},
			{ID: "second", Kind: core.SectionKindContext, Lines: []core.ReviewLine{{NewLineNumber: 3, Content: "later hidden", Kind: core.LineKindUnchanged}}},
		},
	}})

	model.jumpToLine(SearchResult{FileIndex: 0, SectionIndex: 0, LineIndex: 0})

	view := model.reviewViewport.View()
	assert.Contains(t, stripANSI(view), "1 hidden line")
	assert.Contains(t, view, "\x1b[1;")
}
