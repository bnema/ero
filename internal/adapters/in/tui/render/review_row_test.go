package render

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ero/internal/adapters/in/tui/presenter"
	"ero/internal/core"
)

func TestReviewRowRendererRendersDocumentChromeDiffAndAnnotations(t *testing.T) {
	t.Parallel()

	cache := NewReviewLineCacheWithRenderer(func(line core.ReviewLine, numberWidth int) string {
		return "cached diff line"
	})
	renderer := NewReviewRowRenderer(ReviewRowRendererConfig{
		Width:            80,
		EnterKeyLabel:    "enter",
		LineNumberWidths: map[int]int{0: 4},
		LineCache:        cache,
		CommentIcon:      "comment-icon",
		EditorLineRenderer: func(_ presenter.ReviewEditorAnnotation, lineIndex, availableWidth int) string {
			return "editor line"
		},
	})

	fileRow := presenter.ReviewRow{Kind: presenter.ReviewRowKindFile, FileIndex: 0, FilePath: "demo.go", FileStats: presenter.ReviewFileStats{Added: 2, Deleted: 1}}
	assert.Contains(t, stripANSIForRenderTest(renderer.Render(fileRow, 0, ReviewVisualState{})), "demo.go")
	assert.Contains(t, stripANSIForRenderTest(renderer.Render(fileRow, 0, ReviewVisualState{})), "+2 -1")

	rule := stripANSIForRenderTest(renderer.Render(presenter.ReviewRow{Kind: presenter.ReviewRowKindRule}, 1, ReviewVisualState{}))
	assert.True(t, strings.HasPrefix(rule, strings.Repeat("─", 10)))

	line := renderer.Render(presenter.ReviewRow{Kind: presenter.ReviewRowKindLine, FileIndex: 0, SectionIndex: 1, LineIndex: 2, Line: core.ReviewLine{Content: "ignored"}}, 2, ReviewVisualState{})
	assert.Equal(t, "cached diff line", line)

	expanderAnchor := presenter.ReviewExpanderAnchor{FileIndex: 0, SectionIndex: 2}
	expander := renderer.Render(presenter.ReviewRow{Kind: presenter.ReviewRowKindExpander, FileIndex: 0, SectionIndex: 2, Expander: presenter.ReviewExpander{HiddenLines: 3, Position: presenter.ReviewContextBetweenChanges}}, 3, ReviewVisualState{SelectedExpander: &expanderAnchor})
	assert.Contains(t, stripANSIForRenderTest(expander), "⋯ 3 hidden lines between changes · [enter] show more · [a] show all")
	assert.Contains(t, expander, "\x1b[1;")

	commentHeader := renderer.Render(presenter.ReviewRow{Kind: presenter.ReviewRowKindComment, Annotation: presenter.ReviewAnnotation{Comment: core.ReviewComment{ID: "comment-2"}, LineIndex: 0}}, 4, ReviewVisualState{})
	assert.Contains(t, stripANSIForRenderTest(commentHeader), "comment-icon #2")
	commentBody := renderer.Render(presenter.ReviewRow{Kind: presenter.ReviewRowKindComment, Annotation: presenter.ReviewAnnotation{LineIndex: 1, Body: "body line"}}, 5, ReviewVisualState{})
	assert.Contains(t, stripANSIForRenderTest(commentBody), "body line")

	remoteHeader := renderer.Render(presenter.ReviewRow{Kind: presenter.ReviewRowKindRemoteThread, Annotation: presenter.ReviewAnnotation{RemoteThread: core.RemoteReviewThread{ProviderID: "github"}, LineIndex: 0}}, 6, ReviewVisualState{})
	assert.Contains(t, stripANSIForRenderTest(remoteHeader), "[github] remote read-only")
	remoteBody := renderer.Render(presenter.ReviewRow{Kind: presenter.ReviewRowKindRemoteThread, Annotation: presenter.ReviewAnnotation{LineIndex: 1, Author: "octocat", Body: "remote body"}}, 7, ReviewVisualState{})
	assert.Contains(t, stripANSIForRenderTest(remoteBody), "octocat: remote body")

	editorHeader := renderer.Render(presenter.ReviewRow{Kind: presenter.ReviewRowKindEditor, Annotation: presenter.ReviewAnnotation{Editor: presenter.ReviewEditorAnnotation{Range: core.ReviewLineRange{Start: core.ReviewLineRef{NewLineNumber: 7}, End: core.ReviewLineRef{NewLineNumber: 8}}}, LineIndex: 0}}, 8, ReviewVisualState{})
	assert.Contains(t, stripANSIForRenderTest(editorHeader), "commenting lines 7-8")
	assert.Equal(t, "            editor line", renderer.Render(presenter.ReviewRow{Kind: presenter.ReviewRowKindEditor, Annotation: presenter.ReviewAnnotation{Editor: presenter.ReviewEditorAnnotation{}, LineIndex: 1}}, 9, ReviewVisualState{}))
}

func TestReviewRowRendererRendersUnmappedRemoteThread(t *testing.T) {
	t.Parallel()

	renderer := NewReviewRowRenderer(ReviewRowRendererConfig{Width: 80})
	row := presenter.ReviewRow{Kind: presenter.ReviewRowKindRemoteThread, Annotation: presenter.ReviewAnnotation{RemoteThread: core.RemoteReviewThread{ProviderID: "github", Unmapped: true, Comments: []core.RemoteReviewComment{{Body: strings.Repeat("x", 90)}}}}}

	view := stripANSIForRenderTest(renderer.Render(row, 0, ReviewVisualState{}))

	assert.Contains(t, view, "[github] unmapped: ")
	assert.Contains(t, view, "…")
}

func TestReviewRowRendererGutterAndStylePreserveVisualStatePrecedence(t *testing.T) {
	t.Parallel()

	renderer := NewReviewRowRenderer(ReviewRowRendererConfig{CursorMarker: "> ", SelectionMarker: "┃ ", CommentStartMarker: "╭ ", CommentBodyMarker: "│ ", CommentEndMarker: "╰ "})
	row := presenter.ReviewRow{Kind: presenter.ReviewRowKindLine}

	assert.Equal(t, "╭ ", stripANSIForRenderTest(renderer.Gutter(row, 3, ReviewVisualState{CursorRow: 3, SelectionActive: true, SelectionStart: 2, SelectionEnd: 4, CommentMarkers: map[int]CommentMarker{3: CommentMarkerStart}})))
	assert.Equal(t, "> ", stripANSIForRenderTest(renderer.Gutter(row, 3, ReviewVisualState{CursorRow: 3})))
	assert.Equal(t, "┃ ", stripANSIForRenderTest(renderer.Gutter(row, 3, ReviewVisualState{SelectionActive: true, SelectionStart: 2, SelectionEnd: 4})))
	assert.Equal(t, "  ", renderer.Gutter(row, 3, ReviewVisualState{}))

	selected := renderer.Style(3, ReviewVisualState{CursorRow: 3, SelectionActive: true, SelectionStart: 3, SelectionEnd: 3, CommentMarkers: map[int]CommentMarker{3: CommentMarkerStart}}).Render("x")
	cursor := renderer.Style(3, ReviewVisualState{CursorRow: 3}).Render("x")
	comment := renderer.Style(3, ReviewVisualState{CommentMarkers: map[int]CommentMarker{3: CommentMarkerBody}}).Render("x")

	require.Contains(t, selected, "\x1b[")
	require.Contains(t, cursor, "\x1b[")
	require.Contains(t, comment, "\x1b[")
	assert.NotEqual(t, selected, cursor)
	assert.NotEqual(t, cursor, comment)
}

func stripANSIForRenderTest(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}
