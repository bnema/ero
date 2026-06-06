package tui

import (
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ero/internal/adapters/in/tui/render"
	"ero/internal/core"
)

func TestModelViewUsesAlternateScreenAndMouseWheelEvents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		files []core.ReviewFile
	}{
		{name: "review view", files: []core.ReviewFile{reviewFile("demo.go", "package main")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			model := NewModel(tt.files)
			view := model.View()
			assert.True(t, view.AltScreen)
			assert.Equal(t, tea.MouseModeCellMotion, view.MouseMode)
		})
	}
}

func TestModelViewRendersSequentialReviewDocumentWithoutFileExplorer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		files []core.ReviewFile
	}{
		{
			name: "sorts files by path in a single review document",
			files: []core.ReviewFile{
				reviewFile("zeta.go", "package zeta"),
				reviewFile("alpha.go", "package alpha"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model := NewModel(tt.files)
			view := model.View().Content

			assert.Contains(t, view, "ero")
			assert.NotContains(t, view, "Files")
			assert.NotContains(t, view, "▸")
			assert.Less(t, strings.Index(view, "alpha.go"), strings.Index(view, "zeta.go"))
			assert.Contains(t, view, "package alpha")
			assert.Contains(t, view, "package zeta")
		})
	}
}

func TestModelMovesCursorThroughSequentialReviewDocumentWithJKAndArrows(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		keys []tea.KeyPressMsg
	}{
		{
			name: "j k and down update viewport offset",
			keys: []tea.KeyPressMsg{{Text: "j", Code: 'j'}, {Text: "k", Code: 'k'}, {Code: tea.KeyDown}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model := NewModel([]core.ReviewFile{reviewFileWithLines("demo.go", 80)})
			updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
			model = updated.(Model)

			initial := model.cursorRow
			updated, _ = model.Update(tt.keys[0])
			model = updated.(Model)
			assert.Greater(t, model.cursorRow, initial)

			afterDown := model.cursorRow
			updated, _ = model.Update(tt.keys[1])
			model = updated.(Model)
			assert.Less(t, model.cursorRow, afterDown)

			updated, _ = model.Update(tt.keys[2])
			model = updated.(Model)
			assert.Greater(t, model.cursorRow, initial)
		})
	}
}

func TestModelStatusBarShowsActiveFileWhenScrolling(t *testing.T) {
	t.Parallel()

	model := NewModel([]core.ReviewFile{
		reviewFileWithLines("alpha.go", 40),
		reviewFileWithLines("beta.go", 40),
	})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 10})
	model = updated.(Model)

	assert.Contains(t, stripANSI(model.View().Content), "alpha.go")

	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	model = updated.(Model)

	assert.Contains(t, stripANSI(model.View().Content), "beta.go")
}

func TestStatusBarFitsLongModeLabelToOneLine(t *testing.T) {
	t.Parallel()

	view := stripANSI(NewStatusBar(32).Render(StatusModel{
		AppName:       "ero",
		Mode:          "upstream diff",
		FileCount:     42,
		CurrentFile:   "internal/adapters/in/tui/a-very-long-file-name.go",
		ScrollPercent: 0.42,
	}))
	lines := strings.Split(view, "\n")

	assert.Len(t, lines, 1)
	assert.LessOrEqual(t, len([]rune(lines[0])), 32)
}

func TestStatusBarTruncatesLongCurrentFileToOneLine(t *testing.T) {
	t.Parallel()

	model := StatusModel{
		AppName:       "ero",
		Mode:          "review",
		FileCount:     2,
		CurrentFile:   "internal/adapters/in/tui/a-very-long-file-name.go",
		ScrollPercent: 0.42,
	}
	wide := stripANSI(NewStatusBar(60).Render(model))
	wideLines := strings.Split(wide, "\n")
	assert.Len(t, wideLines, 1)
	assert.LessOrEqual(t, len([]rune(wideLines[0])), 60)
	assert.Contains(t, wide, "…")
	assert.Contains(t, wide, "? help")

	narrow := stripANSI(NewStatusBar(40).Render(model))
	narrowLines := strings.Split(narrow, "\n")
	assert.Len(t, narrowLines, 1)
	assert.LessOrEqual(t, len([]rune(narrowLines[0])), 40)
	assert.Contains(t, narrow, "? help")
}

func TestModelViewRendersPolishedStatusBar(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		width  int
		height int
	}{
		{name: "status bar fits configured width", width: 90, height: 12},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model := NewModel([]core.ReviewFile{reviewFile("demo.go", "package main")})
			updated, _ := model.Update(tea.WindowSizeMsg{Width: tt.width, Height: tt.height})
			model = updated.(Model)

			view := stripANSI(model.View().Content)

			assert.Contains(t, view, "ero")
			assert.Contains(t, view, "1 file")
			assert.Contains(t, view, "? help")
			assert.NotContains(t, view, "j/k")
			assert.NotContains(t, view, "expand")
			assert.NotContains(t, view, "quit")
			for line := range strings.SplitSeq(view, "\n") {
				assert.LessOrEqual(t, len([]rune(line)), tt.width)
			}
		})
	}
}

func TestFormatReviewLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		line        core.ReviewLine
		contains    []string
		notContains []string
		stripped    string
	}{
		{
			name: "applies syntax highlighting and line numbers",
			line: core.ReviewLine{
				OldLineNumber: 4,
				NewLineNumber: 5,
				Content:       "func main() {}",
				Kind:          core.LineKindAdded,
				SyntaxTokens: []core.SyntaxToken{
					{Start: 0, End: 4, Type: core.SemanticTokenKeyword},
					{Start: 5, End: 9, Type: core.SemanticTokenFunction},
				},
			},
			contains: []string{"   4", "   5", "+", "func", "main", "\x1b["},
		},
		{
			name: "preserves faded diff background with chroma syntax foregrounds",
			line: core.ReviewLine{
				NewLineNumber: 5,
				Content:       "func main() {}",
				Kind:          core.LineKindAdded,
				SyntaxTokens: []core.SyntaxToken{
					{Start: 0, End: 4, Type: core.SemanticTokenText, SourceType: "KeywordDeclaration"},
					{Start: 5, End: 9, Type: core.SemanticTokenText, SourceType: "NameFunction"},
				},
			},
			contains:    []string{"48;2;1;18;9", "38;2;255;123;114", "38;2;210;168;255"},
			notContains: []string{"48;2;3;47;23", "48;2;218;251;225"},
			stripped:    "+ func main() {}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rendered := render.ReviewLine(tt.line, 4)
			for _, expected := range tt.contains {
				assert.Contains(t, rendered, expected)
			}
			for _, unexpected := range tt.notContains {
				assert.NotContains(t, rendered, unexpected)
			}
			if tt.stripped != "" {
				assert.Contains(t, stripANSI(rendered), tt.stripped)
			}
		})
	}
}

func TestModelUpdateKeepsExpanderFocusVisibleAfterSectionExpands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		key               tea.KeyPressMsg
		expectedSectionID string
	}{
		{name: "enter expands selected context and advances focus", key: tea.KeyPressMsg{Code: tea.KeyEnter}, expectedSectionID: "context-2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model := NewModel([]core.ReviewFile{{
				Path: "demo.go",
				Sections: []core.ReviewSection{
					{
						ID:   "context-1",
						Kind: core.SectionKindContext,
						Lines: []core.ReviewLine{
							{NewLineNumber: 1, Content: "line 1", Kind: core.LineKindUnchanged},
							{NewLineNumber: 2, Content: "line 2", Kind: core.LineKindUnchanged},
						},
					},
					{
						ID:    "changed-1",
						Kind:  core.SectionKindChanged,
						Lines: []core.ReviewLine{{NewLineNumber: 3, Content: "changed", Kind: core.LineKindAdded}},
					},
					{
						ID:   "context-2",
						Kind: core.SectionKindContext,
						Lines: []core.ReviewLine{
							{NewLineNumber: 4, Content: "line 4", Kind: core.LineKindUnchanged},
							{NewLineNumber: 5, Content: "line 5", Kind: core.LineKindUnchanged},
						},
					},
				},
			}})

			updated, _ := model.Update(tt.key)
			model = updated.(Model)

			section := model.selectedContextSection()
			require.NotNil(t, section)
			assert.Equal(t, tt.expectedSectionID, section.ID)
		})
	}
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

func reviewFile(path, content string) core.ReviewFile {
	return core.ReviewFile{
		Path: path,
		Sections: []core.ReviewSection{{
			ID:    path + "-changed",
			Kind:  core.SectionKindChanged,
			Lines: []core.ReviewLine{{NewLineNumber: 1, Content: content, Kind: core.LineKindAdded}},
		}},
	}
}

func reviewFileWithLines(path string, count int) core.ReviewFile {
	lines := make([]core.ReviewLine, 0, count)
	for i := 1; i <= count; i++ {
		lines = append(lines, core.ReviewLine{NewLineNumber: i, Content: "line", Kind: core.LineKindAdded})
	}
	return core.ReviewFile{
		Path:     path,
		Sections: []core.ReviewSection{{ID: "changed", Kind: core.SectionKindChanged, Lines: lines}},
	}
}
