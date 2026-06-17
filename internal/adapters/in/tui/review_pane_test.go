package tui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"

	"ero/internal/adapters/in/tui/presenter"
	"ero/internal/adapters/in/tui/render"
)

func TestReviewPaneRendersOnlyVisibleRowsWithGutterAndStyle(t *testing.T) {
	t.Parallel()

	calls := make([]int, 0)
	pane := NewReviewPane(ReviewPaneConfig{
		Width:  20,
		Height: 3,
		Renderer: reviewPaneRendererFunc(func(row presenter.ReviewRow, rowIndex int, state render.ReviewVisualState) string {
			calls = append(calls, rowIndex)
			return fmt.Sprintf("row-%02d", rowIndex)
		}),
	})
	pane.SetRows(reviewPaneRows(10))
	pane.SetYOffset(4)

	view := stripANSI(pane.View(render.ReviewVisualState{CursorRow: 5}))

	assert.Equal(t, []int{4, 5, 6}, calls)
	assert.Contains(t, view, "row-04")
	assert.Contains(t, view, "row-05")
	assert.Contains(t, view, "row-06")
	assert.NotContains(t, view, "row-03")
	assert.NotContains(t, view, "row-07")
	assert.Contains(t, view, nerdIconArrowRight)
}

func TestReviewPaneClampsOffsetAndTracksScrollPercent(t *testing.T) {
	t.Parallel()

	pane := NewReviewPane(ReviewPaneConfig{Width: 20, Height: 4, Renderer: plainPaneRenderer{}})
	pane.SetRows(reviewPaneRows(10))
	pane.SetYOffset(99)

	assert.Equal(t, 6, pane.YOffset())
	assert.Equal(t, 1.0, pane.ScrollPercent())

	pane.SetHeight(20)
	assert.Equal(t, 0, pane.YOffset())
	assert.Equal(t, 1.0, pane.ScrollPercent())
}

func TestReviewPaneKeepsCursorVisible(t *testing.T) {
	t.Parallel()

	pane := NewReviewPane(ReviewPaneConfig{Width: 20, Height: 4, Renderer: plainPaneRenderer{}})
	pane.SetRows(reviewPaneRows(10))

	pane.KeepVisible(6)
	assert.Equal(t, 3, pane.YOffset())
	pane.KeepVisible(2)
	assert.Equal(t, 2, pane.YOffset())
}

func TestReviewPaneSyncsRendererToContentWidthOnCreateAndResize(t *testing.T) {
	t.Parallel()

	renderer := &widthTrackingPaneRenderer{}
	pane := NewReviewPane(ReviewPaneConfig{Width: 20, Height: 2, Renderer: renderer})
	assert.Equal(t, 18, renderer.widths[len(renderer.widths)-1])

	pane.SetWidth(12)
	assert.Equal(t, 10, renderer.widths[len(renderer.widths)-1])
}

func TestReviewPaneFillsStyledRowsAcrossContentWidth(t *testing.T) {
	t.Parallel()

	pane := NewReviewPane(ReviewPaneConfig{
		Width:  8,
		Height: 1,
		Renderer: reviewPaneRendererFunc(func(row presenter.ReviewRow, rowIndex int, state render.ReviewVisualState) string {
			return "x"
		}),
	})
	pane.SetRows(reviewPaneRows(1))

	view := stripANSI(pane.View(render.ReviewVisualState{CursorRow: 0}))

	assert.Equal(t, nerdIconArrowRight+" x     ", view)
}

func TestReviewPaneTruncatesContentWithinRemainingGutterWidth(t *testing.T) {
	t.Parallel()

	pane := NewReviewPane(ReviewPaneConfig{
		Width:  8,
		Height: 1,
		Renderer: reviewPaneRendererFunc(func(row presenter.ReviewRow, rowIndex int, state render.ReviewVisualState) string {
			return "abcdef"
		}),
	})
	pane.SetRows(reviewPaneRows(1))

	assert.Equal(t, "  abcdef", stripANSI(pane.View(render.ReviewVisualState{CursorRow: -1})))
}

func TestReviewPaneTruncatesLongLinesAndFillsHeight(t *testing.T) {
	t.Parallel()

	pane := NewReviewPane(ReviewPaneConfig{
		Width:  12,
		Height: 4,
		Renderer: reviewPaneRendererFunc(func(row presenter.ReviewRow, rowIndex int, state render.ReviewVisualState) string {
			return strings.Repeat("x", 80)
		}),
	})
	pane.SetRows(reviewPaneRows(1))

	lines := strings.Split(stripANSI(pane.View(render.ReviewVisualState{})), "\n")

	assert.Len(t, lines, 4)
	for _, line := range lines {
		assert.LessOrEqual(t, len([]rune(line)), 12)
	}
}

func reviewPaneRows(count int) []presenter.ReviewRow {
	rows := make([]presenter.ReviewRow, count)
	for i := range rows {
		rows[i] = presenter.ReviewRow{Kind: presenter.ReviewRowKindLine, FileIndex: 0, SectionIndex: 0, LineIndex: i, Selectable: true}
	}
	return rows
}

type reviewPaneRendererFunc func(presenter.ReviewRow, int, render.ReviewVisualState) string

func (f reviewPaneRendererFunc) Render(row presenter.ReviewRow, rowIndex int, state render.ReviewVisualState) string {
	return f(row, rowIndex, state)
}

func (f reviewPaneRendererFunc) Gutter(row presenter.ReviewRow, rowIndex int, state render.ReviewVisualState) string {
	if rowIndex == state.CursorRow {
		return nerdIconArrowRight + " "
	}
	return "  "
}

func (f reviewPaneRendererFunc) Style(rowIndex int, state render.ReviewVisualState) lipgloss.Style {
	if rowIndex == state.CursorRow {
		return lipgloss.NewStyle().Background(lipgloss.Color("#eeeeee"))
	}
	return lipgloss.NewStyle()
}

type widthTrackingPaneRenderer struct {
	widths []int
}

func (r *widthTrackingPaneRenderer) SetWidth(width int) {
	r.widths = append(r.widths, width)
}

func (r *widthTrackingPaneRenderer) Render(row presenter.ReviewRow, rowIndex int, state render.ReviewVisualState) string {
	return fmt.Sprintf("width-%d", r.widths[len(r.widths)-1])
}

func (r *widthTrackingPaneRenderer) Gutter(row presenter.ReviewRow, rowIndex int, state render.ReviewVisualState) string {
	return "  "
}

func (r *widthTrackingPaneRenderer) Style(rowIndex int, state render.ReviewVisualState) lipgloss.Style {
	return lipgloss.NewStyle()
}

type plainPaneRenderer struct{}

func (plainPaneRenderer) Render(row presenter.ReviewRow, rowIndex int, state render.ReviewVisualState) string {
	return fmt.Sprintf("row-%02d", rowIndex)
}

func (plainPaneRenderer) Gutter(row presenter.ReviewRow, rowIndex int, state render.ReviewVisualState) string {
	return "  "
}

func (plainPaneRenderer) Style(rowIndex int, state render.ReviewVisualState) lipgloss.Style {
	return lipgloss.NewStyle()
}
