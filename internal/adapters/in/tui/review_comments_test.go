package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ero/internal/adapters/in/tui/presenter"
	"ero/internal/adapters/in/tui/render"
	"ero/internal/core"
)

func TestReviewDocumentRendersCommentsAndEditorBelowAnchoredRange(t *testing.T) {
	t.Parallel()

	draft := core.NewReviewDraft()
	_, err := draft.AddComment(core.ReviewCommentInput{
		FilePath: "demo.go",
		Range: core.ReviewLineRange{
			Start: core.ReviewLineRef{NewLineNumber: 1, Kind: core.LineKindAdded},
			End:   core.ReviewLineRef{NewLineNumber: 2, Kind: core.LineKindAdded},
		},
		Body: "Looks good\nwith one note",
	})
	require.NoError(t, err)
	editor := NewCommentEditor(80)

	files := []core.ReviewFile{{
		Path: "demo.go",
		Sections: []core.ReviewSection{{ID: "changed", Kind: core.SectionKindChanged, Lines: []core.ReviewLine{
			{NewLineNumber: 1, Content: "one", Kind: core.LineKindAdded},
			{NewLineNumber: 2, Content: "two", Kind: core.LineKindAdded},
			{NewLineNumber: 3, Content: "three", Kind: core.LineKindAdded},
		}}},
	}}
	inlineEditor := InlineCommentEditor{FilePath: "demo.go", Range: core.ReviewLineRange{Start: core.ReviewLineRef{NewLineNumber: 3, Kind: core.LineKindAdded}, End: core.ReviewLineRef{NewLineNumber: 3, Kind: core.LineKindAdded}}, Editor: editor}
	editorAnnotation := inlineEditor.PresenterAnnotation(68)
	rendered := renderReviewForTest(files, 80, -1, -1, presenter.ReviewAnnotations{
		Comments: draft.Comments(),
		Editor:   &editorAnnotation,
	})

	view := stripANSI(rendered.Content)
	assertLineOrder(t, view, "+ two", "Looks good", "+ three", "Add review comment")
	assert.Contains(t, view, "with one note")
	assert.Contains(t, view, "#1")
	assert.NotContains(t, view, "comment-1")
	assert.Contains(t, rendered.Content, render.InlineCommentIconStyle.Render(nerdIconComment))
	assert.Contains(t, rendered.Content, render.InlineCommentIDStyle.Render("#1"))
	assert.Contains(t, rendered.Content, render.InlineCommentBodyStyle.Render("Looks good"))
	lines := strings.Split(view, "\n")
	commentLine := lineContaining(t, lines, "Looks good")
	editorLine := lineContaining(t, lines, "Add review comment")
	assert.True(t, strings.HasPrefix(commentLine, strings.Repeat(" ", 12)), "comment should start under code content column: %q", commentLine)
	assert.True(t, strings.HasPrefix(editorLine, strings.Repeat(" ", 12)), "editor should start under code content column: %q", editorLine)
	for _, line := range lines {
		assert.LessOrEqual(t, len([]rune(line)), 80, "rendered annotation line should fit review width: %q", line)
	}
	assert.Len(t, rendered.Rows, len(rendered.Lines))
}

func lineContaining(t *testing.T, lines []string, part string) string {
	t.Helper()
	for _, line := range lines {
		if strings.Contains(line, part) {
			return line
		}
	}
	require.Failf(t, "missing line", "missing %q", part)
	return ""
}

func assertLineOrder(t *testing.T, content string, parts ...string) {
	t.Helper()
	last := -1
	for _, part := range parts {
		index := strings.Index(content, part)
		require.NotEqual(t, -1, index, "missing %q in %s", part, content)
		assert.Greater(t, index, last, "%q should appear after previous part", part)
		last = index
	}
}
