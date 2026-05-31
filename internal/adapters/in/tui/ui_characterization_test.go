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

func TestReviewDocumentCharacterizesCurrentVisualChromeAndAnnotations(t *testing.T) {
	t.Parallel()

	draft := core.NewReviewDraft()
	_, err := draft.AddComment(core.ReviewCommentInput{
		FilePath: "demo.go",
		Range: core.ReviewLineRange{
			Start: core.ReviewLineRef{NewLineNumber: 10, Kind: core.LineKindAdded},
			End:   core.ReviewLineRef{NewLineNumber: 10, Kind: core.LineKindAdded},
		},
		Body: "Looks good",
	})
	require.NoError(t, err)

	files := []core.ReviewFile{{
		Path: "demo.go",
		Sections: []core.ReviewSection{
			{ID: "hidden-start", Kind: core.SectionKindContext, Lines: []core.ReviewLine{{OldLineNumber: 9, NewLineNumber: 9, Content: "hidden before", Kind: core.LineKindUnchanged}}},
			{ID: "changed", Kind: core.SectionKindChanged, Lines: []core.ReviewLine{
				{NewLineNumber: 10, Content: "func addedCall()", Kind: core.LineKindAdded, SyntaxTokens: []core.SyntaxToken{{Start: 0, End: 4, Type: core.SemanticTokenKeyword}, {Start: 5, End: 14, Type: core.SemanticTokenFunction}}},
				{OldLineNumber: 11, Content: "removedCall()", Kind: core.LineKindDeleted},
			}},
		},
	}}
	inlineEditor := InlineCommentEditor{FilePath: "demo.go", Range: core.ReviewLineRange{Start: core.ReviewLineRef{OldLineNumber: 11, Kind: core.LineKindDeleted}, End: core.ReviewLineRef{OldLineNumber: 11, Kind: core.LineKindDeleted}}, Editor: NewCommentEditor(80)}
	// Width 86 is the 100-column review width minus the two-column pane gutter and twelve-column annotation indent.
	const presenterAnnotationWidth = 86
	editorAnnotation := inlineEditor.PresenterAnnotation(presenterAnnotationWidth)
	rendered := renderReviewForTest(files, 100, 0, 0, presenter.ReviewAnnotations{
		Comments: draft.Comments(),
		RemoteThreads: []core.RemoteReviewThread{{
			ProviderID: "github",
			FilePath:   "demo.go",
			Range:      core.ReviewLineRange{Start: core.ReviewLineRef{OldLineNumber: 11, Kind: core.LineKindDeleted}, End: core.ReviewLineRef{OldLineNumber: 11, Kind: core.LineKindDeleted}},
			Comments:   []core.RemoteReviewComment{{Author: "octocat", Body: "remote note"}},
		}},
		Editor: &editorAnnotation,
	})

	view := stripANSI(rendered.Content)
	assert.Contains(t, view, "demo.go")
	assert.Contains(t, view, "+1 -1")
	assert.Contains(t, view, strings.Repeat("─", 10))
	assert.Contains(t, view, "⋯ 1 hidden line from beginning of file · ["+enterKeyLabel()+"] show more · [a] show all")
	assertLineOrder(t, view, "+ func addedCall()", "#1", "Looks good", "- removedCall()", "[github]", "octocat: remote note", "commenting old line 11", "Add review comment")
	assert.Contains(t, rendered.Content, "\x1b[", "document should keep ANSI styling for line numbers, syntax, diff backgrounds, annotations, and selected context")
	assert.Len(t, rendered.Rows, len(rendered.Lines))

	firstLineRow, ok := rendered.Anchors.LineRows[ReviewLineAnchor{FileIndex: 0, SectionIndex: 1, LineIndex: 0}]
	require.True(t, ok)
	assert.Contains(t, rendered.Lines[firstLineRow], "\x1b[")

	expanderRow := -1
	for rowIndex, row := range rendered.Rows {
		if row.Kind == ReviewRowKindExpander {
			expanderRow = rowIndex
			break
		}
	}
	require.NotEqual(t, -1, expanderRow)
	assert.Contains(t, rendered.Lines[expanderRow], "\x1b[1;", "selected context expander should be visibly highlighted")
}

func TestModelCharacterizesCurrentCursorSelectionAndCommentGutters(t *testing.T) {
	t.Parallel()

	model := NewModel([]core.ReviewFile{reviewFileWithLines("demo.go", 4)})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	model = updated.(Model)
	_, err := model.reviewDraft.AddComment(core.ReviewCommentInput{
		FilePath: "demo.go",
		Range: core.ReviewLineRange{
			Start: core.ReviewLineRef{NewLineNumber: 4, Kind: core.LineKindAdded},
			End:   core.ReviewLineRef{NewLineNumber: 4, Kind: core.LineKindAdded},
		},
		Body: "commented range",
	})
	require.NoError(t, err)
	model.syncReviewViewport()

	updated, _ = model.Update(keyPress("s"))
	model = updated.(Model)
	updated, _ = model.Update(keyPress("j"))
	model = updated.(Model)

	start, end, selected := model.selectedRange()
	require.True(t, selected)
	assert.Equal(t, start+1, end)
	commentRow := model.reviewAnchors.LineRows[ReviewLineAnchor{FileIndex: 0, SectionIndex: 0, LineIndex: 3}]
	require.GreaterOrEqual(t, commentRow, 0)

	view := stripANSI(model.View().Content)
	assert.Contains(t, view, nerdIconArrowRight)
	assert.Contains(t, view, "┃")
	assert.Contains(t, view, "commented range")
	assert.Contains(t, view, "demo.go:2")
}

func TestModelCharacterizesMiddleContextExpansionDirectionFromCursorPosition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		cursorFromExpander int
		wantExpandedAbove  int
		wantExpandedBelow  int
	}{
		{name: "cursor above middle expander reveals context above", cursorFromExpander: -1, wantExpandedAbove: 3, wantExpandedBelow: 0},
		{name: "cursor below middle expander reveals context below", cursorFromExpander: 1, wantExpandedAbove: 0, wantExpandedBelow: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model := NewModel([]core.ReviewFile{middleContextReviewFile()})
			expanderRow := expanderRowForSection(t, model, 0, 1)
			model.cursorRow = expanderRow + tt.cursorFromExpander
			model.selectNearestContextToCursor()
			require.Equal(t, 0, model.selectedContext)

			updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			model = updated.(Model)

			section := model.files[0].Sections[1]
			assert.Equal(t, tt.wantExpandedAbove, section.ExpandedAbove)
			assert.Equal(t, tt.wantExpandedBelow, section.ExpandedBelow)
		})
	}
}

func TestModelCharacterizesOverlaysDoNotDisturbReviewCursorOrSelection(t *testing.T) {
	t.Parallel()

	provider := core.ReviewProviderInfo{ID: "github", Label: "GitHub", Capabilities: core.ReviewProviderCapabilities{PublishReview: true}}
	model := NewModelWithReviewProviders([]core.ReviewFile{reviewFileWithLines("demo.go", 8)}, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, nil)
	model.providerInfos = []core.ReviewProviderInfo{provider}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 8})
	model = updated.(Model)
	updated, _ = model.Update(keyPress("s"))
	model = updated.(Model)
	updated, _ = model.Update(keyPress("j"))
	model = updated.(Model)
	cursor := model.cursorRow
	selectionStart := *model.selectionAnchorRow

	updated, _ = model.Update(keyPress("?"))
	model = updated.(Model)
	updated, _ = model.Update(keyPress("j"))
	model = updated.(Model)
	assert.Equal(t, cursor, model.cursorRow)
	assert.Equal(t, selectionStart, *model.selectionAnchorRow)
	assert.True(t, model.helpActive)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	model = updated.(Model)

	updated, _ = model.Update(keyPress("f"))
	model = updated.(Model)
	updated, _ = model.Update(keyPress("d"))
	model = updated.(Model)
	assert.Equal(t, cursor, model.cursorRow)
	assert.Equal(t, selectionStart, *model.selectionAnchorRow)
	assert.True(t, model.search.active())
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	model = updated.(Model)

	updated, _ = model.Update(keyPress("P"))
	model = updated.(Model)
	require.True(t, model.publish.active)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	model = updated.(Model)
	assert.Equal(t, cursor, model.cursorRow)
	assert.Equal(t, selectionStart, *model.selectionAnchorRow)
	assert.Contains(t, stripANSI(model.View().Content), "Publish review")
}

func middleContextReviewFile() core.ReviewFile {
	return core.ReviewFile{
		Path: "demo.go",
		Sections: []core.ReviewSection{
			{ID: "before", Kind: core.SectionKindChanged, Lines: []core.ReviewLine{{NewLineNumber: 1, Content: "before", Kind: core.LineKindAdded}}},
			{ID: "middle", Kind: core.SectionKindContext, Lines: []core.ReviewLine{
				{OldLineNumber: 2, NewLineNumber: 2, Content: "context 1", Kind: core.LineKindUnchanged},
				{OldLineNumber: 3, NewLineNumber: 3, Content: "context 2", Kind: core.LineKindUnchanged},
				{OldLineNumber: 4, NewLineNumber: 4, Content: "context 3", Kind: core.LineKindUnchanged},
			}},
			{ID: "after", Kind: core.SectionKindChanged, Lines: []core.ReviewLine{{NewLineNumber: 5, Content: "after", Kind: core.LineKindAdded}}},
		},
	}
}
