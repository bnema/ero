package tui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"ero/internal/adapters/in/tui/render"
	"ero/internal/core"
)

func BenchmarkLargeReviewDownNavigation(b *testing.B) {
	model := NewModel(largeReviewFilesForBenchmark(60, 136))
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	model = updated.(Model)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		model = updated.(Model)
	}
}

func BenchmarkLargeReviewView(b *testing.B) {
	model := NewModel(largeReviewFilesForBenchmark(60, 136))
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	model = updated.(Model)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = model.View()
	}
}

func TestReviewPaneViewRendersOnlyVisibleRows(t *testing.T) {
	calls := 0
	pane := NewReviewPane(ReviewPaneConfig{
		Width:  80,
		Height: 5,
		Renderer: reviewPaneRendererFunc(func(row ReviewRow, rowIndex int, state render.ReviewVisualState) string {
			calls++
			return fmt.Sprintf("row-%d", rowIndex)
		}),
	})
	pane.SetRows(reviewPaneRows(100))
	pane.SetYOffset(40)

	_ = pane.View()

	if calls != 5 {
		t.Fatalf("expected visible-only rendering to touch 5 rows, touched %d", calls)
	}
}

func largeReviewFilesForBenchmark(fileCount, renderedLinesPerFile int) []core.ReviewFile {
	files := make([]core.ReviewFile, fileCount)
	for fileIndex := range files {
		sections := make([]core.ReviewSection, 0, renderedLinesPerFile/8)
		lineNumber := 1
		for sectionIndex := 0; sectionIndex < renderedLinesPerFile/8; sectionIndex++ {
			contextLines := make([]core.ReviewLine, 20)
			for i := range contextLines {
				contextLines[i] = core.ReviewLine{OldLineNumber: lineNumber, NewLineNumber: lineNumber, Content: "package demo // unchanged context line with tokens", Kind: core.LineKindUnchanged}
				lineNumber++
			}
			sections = append(sections, core.ReviewSection{ID: fmt.Sprintf("ctx-%d", sectionIndex), Kind: core.SectionKindContext, Lines: contextLines})

			changedLines := make([]core.ReviewLine, 0, 7)
			for i := 0; i < 7; i++ {
				changedLines = append(changedLines, core.ReviewLine{
					OldLineNumber: lineNumber,
					NewLineNumber: lineNumber,
					Content:       fmt.Sprintf("func Demo%d_%d() string { return \"abcdefghijklmnopqrstuvwxyz0123456789\" }", fileIndex, lineNumber),
					Kind:          core.LineKindAdded,
					SyntaxTokens: []core.SyntaxToken{
						{Start: 0, End: 4, SourceType: "Keyword"},
						{Start: 5, End: 14, SourceType: "NameFunction"},
						{Start: 33, End: 77, SourceType: "LiteralString"},
					},
				})
				lineNumber++
			}
			sections = append(sections, core.ReviewSection{ID: fmt.Sprintf("chg-%d", sectionIndex), Kind: core.SectionKindChanged, Lines: changedLines})
		}
		files[fileIndex] = core.ReviewFile{Path: fmt.Sprintf("pkg/file_%03d.go", fileIndex), Sections: sections}
	}
	return files
}
