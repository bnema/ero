package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"ero/internal/adapters/in/tui/presenter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ero/internal/core"
)

func TestReviewDocumentRendersContextualContextBars(t *testing.T) {
	t.Parallel()

	files := []core.ReviewFile{{
		Path: "demo.go",
		Sections: []core.ReviewSection{
			{
				ID:   "start-context",
				Kind: core.SectionKindContext,
				Lines: []core.ReviewLine{
					{NewLineNumber: 1, Content: "start 1", Kind: core.LineKindUnchanged},
					{NewLineNumber: 2, Content: "start 2", Kind: core.LineKindUnchanged},
				},
			},
			{
				ID:    "changed-1",
				Kind:  core.SectionKindChanged,
				Lines: []core.ReviewLine{{NewLineNumber: 3, Content: "changed 1", Kind: core.LineKindAdded}},
			},
			{
				ID:   "middle-context",
				Kind: core.SectionKindContext,
				Lines: []core.ReviewLine{
					{NewLineNumber: 4, Content: "middle 1", Kind: core.LineKindUnchanged},
					{NewLineNumber: 5, Content: "middle 2", Kind: core.LineKindUnchanged},
				},
			},
			{
				ID:    "changed-2",
				Kind:  core.SectionKindChanged,
				Lines: []core.ReviewLine{{NewLineNumber: 6, Content: "changed 2", Kind: core.LineKindAdded}},
			},
			{
				ID:   "end-context",
				Kind: core.SectionKindContext,
				Lines: []core.ReviewLine{
					{NewLineNumber: 7, Content: "end 1", Kind: core.LineKindUnchanged},
					{NewLineNumber: 8, Content: "end 2", Kind: core.LineKindUnchanged},
				},
			},
		},
	}}

	content := stripANSI(renderReviewForTest(files, 120, -1, -1, presenter.ReviewAnnotations{}).Content)

	assert.Contains(t, content, "⋯ 2 hidden lines from beginning of file · ["+enterKeyLabel()+"] show more · [a] show all")
	assert.Contains(t, content, "⋯ 2 hidden lines between changes · ["+enterKeyLabel()+"] show more · [a] show all")
	assert.Contains(t, content, "⋯ 2 hidden lines to end of file · ["+enterKeyLabel()+"] show more · [a] show all")
}

func TestReviewDocumentRendersSingularOnlyContextBar(t *testing.T) {
	t.Parallel()

	files := []core.ReviewFile{{
		Path: "unchanged.go",
		Sections: []core.ReviewSection{{
			ID:    "only-context",
			Kind:  core.SectionKindContext,
			Lines: []core.ReviewLine{{NewLineNumber: 1, Content: "only", Kind: core.LineKindUnchanged}},
		}},
	}}

	content := stripANSI(renderReviewForTest(files, 120, -1, -1, presenter.ReviewAnnotations{}).Content)

	assert.Contains(t, content, "⋯ 1 hidden line in file · ["+enterKeyLabel()+"] show more · [a] show all")
}

func TestModelCursorSelectsNearestContextBar(t *testing.T) {
	t.Parallel()

	model := NewModel([]core.ReviewFile{{
		Path: "demo.go",
		Sections: []core.ReviewSection{
			{
				ID:   "start-context",
				Kind: core.SectionKindContext,
				Lines: []core.ReviewLine{
					{NewLineNumber: 1, Content: "start 1", Kind: core.LineKindUnchanged},
					{NewLineNumber: 2, Content: "start 2", Kind: core.LineKindUnchanged},
				},
			},
			{
				ID:   "changed-1",
				Kind: core.SectionKindChanged,
				Lines: []core.ReviewLine{
					{NewLineNumber: 3, Content: "changed 1", Kind: core.LineKindAdded},
					{NewLineNumber: 4, Content: "changed 2", Kind: core.LineKindAdded},
					{NewLineNumber: 5, Content: "changed 3", Kind: core.LineKindAdded},
				},
			},
			{
				ID:   "end-context",
				Kind: core.SectionKindContext,
				Lines: []core.ReviewLine{
					{NewLineNumber: 6, Content: "end 1", Kind: core.LineKindUnchanged},
					{NewLineNumber: 7, Content: "end 2", Kind: core.LineKindUnchanged},
				},
			},
		},
	}})

	model.cursorRow = expanderRowForSection(t, model, 0, 2) - 1
	model.selectNearestContextToCursor()

	assert.Equal(t, 1, model.selectedContext)
}

func TestModelEnterShowsMoreContextBarUnderCursor(t *testing.T) {
	t.Parallel()

	model := NewModel([]core.ReviewFile{{
		Path: "demo.go",
		Sections: []core.ReviewSection{
			{
				ID:   "first-context",
				Kind: core.SectionKindContext,
				Lines: []core.ReviewLine{
					{NewLineNumber: 1, Content: "first 1", Kind: core.LineKindUnchanged},
					{NewLineNumber: 2, Content: "first 2", Kind: core.LineKindUnchanged},
				},
			},
			{
				ID:    "changed-1",
				Kind:  core.SectionKindChanged,
				Lines: []core.ReviewLine{{NewLineNumber: 3, Content: "changed 1", Kind: core.LineKindAdded}},
			},
			{
				ID:   "second-context",
				Kind: core.SectionKindContext,
				Lines: []core.ReviewLine{
					{NewLineNumber: 4, Content: "second 1", Kind: core.LineKindUnchanged},
					{NewLineNumber: 5, Content: "second 2", Kind: core.LineKindUnchanged},
				},
			},
		},
	}})

	for rowIndex, row := range model.reviewRows {
		if row.Kind == ReviewRowKindExpander && row.SectionIndex == 2 {
			model.cursorRow = rowIndex
			break
		}
	}
	require.Equal(t, 0, model.files[0].Sections[0].ExpandedAbove)
	require.Equal(t, 0, model.files[0].Sections[2].ExpandedAbove)

	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	updatedModel, ok := updated.(Model)
	require.True(t, ok)
	model = updatedModel

	assert.Equal(t, 0, model.files[0].Sections[0].ExpandedAbove)
	assert.Equal(t, 2, model.files[0].Sections[2].ExpandedAbove)
}

func TestModelEnterShowsMoreAtFileEdges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		sectionIndex int
		assertState  func(t *testing.T, section core.ReviewSection)
	}{
		{
			name:         "start context reveals nearest changed lines with a",
			sectionIndex: 0,
			assertState: func(t *testing.T, section core.ReviewSection) {
				t.Helper()
				assert.Equal(t, 0, section.ExpandedAbove)
				assert.Equal(t, 2, section.ExpandedBelow)
			},
		},
		{
			name:         "end context reveals nearest changed lines with b",
			sectionIndex: 2,
			assertState: func(t *testing.T, section core.ReviewSection) {
				t.Helper()
				assert.Equal(t, 2, section.ExpandedAbove)
				assert.Equal(t, 0, section.ExpandedBelow)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model := NewModel([]core.ReviewFile{contextBarReviewFile("demo.go")})
			model.cursorRow = expanderRowForSection(t, model, 0, tt.sectionIndex)

			updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			updatedModel, ok := updated.(Model)
			require.True(t, ok)
			model = updatedModel

			tt.assertState(t, model.files[0].Sections[tt.sectionIndex])
		})
	}
}

func TestModelAShowsAllContextBarUnderCursor(t *testing.T) {
	t.Parallel()

	model := NewModel([]core.ReviewFile{contextBarReviewFile("demo.go")})
	model.cursorRow = expanderRowForSection(t, model, 0, 0)

	updated, _ := model.Update(keyPress("a"))
	updatedModel, ok := updated.(Model)
	require.True(t, ok)
	model = updatedModel

	assert.Equal(t, 0, model.files[0].Sections[0].HiddenLineCount())
}

func TestModelDoesNotExpandStaleContextWhenCursorFileHasNoExpanders(t *testing.T) {
	t.Parallel()

	model := NewModel([]core.ReviewFile{
		contextBarReviewFile("a.go"),
		reviewFile("b.go", "package b"),
	})
	model.cursorRow = expanderRowForSection(t, model, 0, 0)
	model.selectNearestContextToCursor()
	require.Equal(t, 0, model.selectedFile)
	require.Equal(t, 0, model.selectedContext)

	model.cursorRow = model.reviewAnchors.FileRows[1]
	model.selectNearestContextToCursor()

	updated, _ := model.Update(keyPress("a"))
	updatedModel, ok := updated.(Model)
	require.True(t, ok)
	model = updatedModel

	assert.Equal(t, 0, model.files[0].Sections[0].ExpandedAbove)
	assert.Equal(t, 0, model.files[0].Sections[0].ExpandedBelow)
	assert.Equal(t, 1, model.selectedFile)
	assert.Equal(t, -1, model.selectedContext)
}

func TestContextBarViewDoesNotWrapOnNarrowWidths(t *testing.T) {
	t.Parallel()

	bar := NewContextBar(20)
	rendered := stripANSI(bar.View(ContextBarViewModel{HiddenLines: 13, Position: ContextBarAtFileStart}, false))

	assert.NotContains(t, rendered, "\n")
}

func TestModelEnterShowsMoreContextBarUnderCursorAcrossFiles(t *testing.T) {
	t.Parallel()

	model := NewModel([]core.ReviewFile{
		contextBarReviewFile("a.go"),
		contextBarReviewFile("b.go"),
	})
	model.cursorRow = expanderRowForSection(t, model, 1, 2)

	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	updatedModel, ok := updated.(Model)
	require.True(t, ok)
	model = updatedModel

	assert.Equal(t, 0, model.files[0].Sections[2].ExpandedAbove)
	assert.Equal(t, 2, model.files[1].Sections[2].ExpandedAbove)
	assert.Equal(t, 1, model.selectedFile)
}

func contextBarReviewFile(path string) core.ReviewFile {
	return core.ReviewFile{
		Path: path,
		Sections: []core.ReviewSection{
			{
				ID:   "start-context",
				Kind: core.SectionKindContext,
				Lines: []core.ReviewLine{
					{NewLineNumber: 1, Content: "start 1", Kind: core.LineKindUnchanged},
					{NewLineNumber: 2, Content: "start 2", Kind: core.LineKindUnchanged},
				},
			},
			{
				ID:    "changed",
				Kind:  core.SectionKindChanged,
				Lines: []core.ReviewLine{{NewLineNumber: 3, Content: "changed", Kind: core.LineKindAdded}},
			},
			{
				ID:   "end-context",
				Kind: core.SectionKindContext,
				Lines: []core.ReviewLine{
					{NewLineNumber: 4, Content: "end 1", Kind: core.LineKindUnchanged},
					{NewLineNumber: 5, Content: "end 2", Kind: core.LineKindUnchanged},
				},
			},
		},
	}
}

func expanderRowForSection(t *testing.T, model Model, fileIndex, sectionIndex int) int {
	t.Helper()
	for rowIndex, row := range model.reviewRows {
		if row.Kind == ReviewRowKindExpander && row.FileIndex == fileIndex && row.SectionIndex == sectionIndex {
			return rowIndex
		}
	}
	t.Fatalf("expander row not found for file %d section %d", fileIndex, sectionIndex)
	return 0
}

func TestReviewDocumentOmitsContextBarWhenNoHiddenLinesRemain(t *testing.T) {
	t.Parallel()

	files := []core.ReviewFile{{
		Path: "demo.go",
		Sections: []core.ReviewSection{{
			ID:            "expanded-context",
			Kind:          core.SectionKindContext,
			ExpandedAbove: 1,
			Lines:         []core.ReviewLine{{NewLineNumber: 1, Content: "visible", Kind: core.LineKindUnchanged}},
		}},
	}}

	content := stripANSI(renderReviewForTest(files, 120, -1, -1, presenter.ReviewAnnotations{}).Content)

	assert.NotContains(t, content, "hidden line")
	assert.Contains(t, content, "visible")
}
