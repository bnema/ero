package render

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"ero/internal/adapters/in/tui/presenter"
	"ero/internal/adapters/in/tui/theme"
	"ero/internal/core"
)

func TestReviewLineCacheReusesRenderedLineForStableRowKey(t *testing.T) {
	t.Parallel()

	calls := 0
	cache := NewReviewLineCacheWithRenderer(func(line core.ReviewLine, numberWidth int) string {
		calls++
		return fmt.Sprintf("%d:%s", numberWidth, line.Content)
	})
	row := presenter.ReviewRow{Kind: presenter.ReviewRowKindLine, FileIndex: 0, SectionIndex: 0, LineIndex: 0, Line: core.ReviewLine{NewLineNumber: 1, Content: "same", Kind: core.LineKindAdded}}

	assert.Equal(t, "4:same", cache.Render(row, 4))
	assert.Equal(t, "4:same", cache.Render(row, 4))
	assert.Equal(t, 1, calls)
}

func TestReviewLineCacheInvalidatesWhenLineContentTokensOrWidthChange(t *testing.T) {
	t.Parallel()

	calls := 0
	cache := NewReviewLineCacheWithRenderer(func(line core.ReviewLine, numberWidth int) string {
		calls++
		return fmt.Sprintf("%d:%s:%d", numberWidth, line.Content, len(line.SyntaxTokens))
	})
	row := presenter.ReviewRow{Kind: presenter.ReviewRowKindLine, FileIndex: 0, SectionIndex: 0, LineIndex: 0, Line: core.ReviewLine{NewLineNumber: 1, Content: "before", Kind: core.LineKindAdded}}

	_ = cache.Render(row, 4)
	row.Line.Content = "after"
	assert.Equal(t, "4:after:0", cache.Render(row, 4))
	row.Line.SyntaxTokens = []core.SyntaxToken{{Start: 0, End: 5, Type: core.SemanticTokenKeyword}}
	assert.Equal(t, "4:after:1", cache.Render(row, 4))
	assert.Equal(t, "8:after:1", cache.Render(row, 8))
	assert.Equal(t, 4, calls)
}

func TestReviewLineCacheInvalidatesWhenThemeAppearanceChanges(t *testing.T) {
	originalAppearance := theme.CurrentAppearance()
	t.Cleanup(func() { theme.ApplyAppearance(originalAppearance) })

	row := presenter.ReviewRow{Kind: presenter.ReviewRowKindLine, FileIndex: 0, SectionIndex: 0, LineIndex: 0, Line: core.ReviewLine{
		NewLineNumber: 1,
		Content:       "func main() {}",
		Kind:          core.LineKindAdded,
		SyntaxTokens:  []core.SyntaxToken{{Start: 0, End: 4, Type: core.SemanticTokenText, SourceType: "KeywordDeclaration"}},
	}}
	cache := NewReviewLineCache()

	theme.ApplyAppearance(core.ThemeAppearanceDark)
	dark := cache.Render(row, 4)
	theme.ApplyAppearance(core.ThemeAppearanceLight)
	light := cache.Render(row, 4)

	assert.NotEqual(t, dark, light)
	assert.Contains(t, dark, "38;2;255;123;114")
	assert.Contains(t, light, "38;2;207;34;46")
}

func TestReviewLineCacheClearDropsEntries(t *testing.T) {
	t.Parallel()

	calls := 0
	cache := NewReviewLineCacheWithRenderer(func(line core.ReviewLine, numberWidth int) string {
		calls++
		return line.Content
	})
	row := presenter.ReviewRow{Kind: presenter.ReviewRowKindLine, FileIndex: 1, SectionIndex: 2, LineIndex: 3, Line: core.ReviewLine{NewLineNumber: 4, Content: "cached", Kind: core.LineKindAdded}}

	_ = cache.Render(row, 4)
	cache.Clear()
	_ = cache.Render(row, 4)

	assert.Equal(t, 2, calls)
}
