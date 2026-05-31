package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"ero/internal/adapters/in/tui/presenter"
	"ero/internal/adapters/in/tui/render"
)

type reviewPaneRenderer interface {
	Render(presenter.ReviewRow, int, render.ReviewVisualState) string
	Gutter(presenter.ReviewRow, int, render.ReviewVisualState) string
	Style(int, render.ReviewVisualState) lipgloss.Style
}

type reviewPaneWidthSetter interface {
	SetWidth(int)
}

type ReviewPaneConfig struct {
	Width    int
	Height   int
	Renderer reviewPaneRenderer
}

type ReviewPane struct {
	width       int
	height      int
	yOffset     int
	rows        []presenter.ReviewRow
	renderer    reviewPaneRenderer
	visualState render.ReviewVisualState
}

func NewReviewPane(config ReviewPaneConfig) ReviewPane {
	pane := ReviewPane{width: config.Width, height: config.Height, renderer: config.Renderer}
	if pane.width <= 0 {
		pane.width = 80
	}
	if pane.height <= 0 {
		pane.height = 1
	}
	pane.syncRendererWidth()
	return pane
}

func (p *ReviewPane) SetRows(rows []presenter.ReviewRow) {
	p.rows = rows
	p.clampYOffset()
}

func (p *ReviewPane) Rows() []presenter.ReviewRow {
	return p.rows
}

func (p *ReviewPane) SetWidth(width int) {
	p.width = max(width, 0)
	p.syncRendererWidth()
}

func (p *ReviewPane) Width() int {
	return p.width
}

func (p *ReviewPane) SetHeight(height int) {
	p.height = max(height, 0)
	p.clampYOffset()
}

func (p *ReviewPane) Height() int {
	return p.height
}

func (p *ReviewPane) SetYOffset(offset int) {
	p.yOffset = offset
	p.clampYOffset()
}

func (p *ReviewPane) YOffset() int {
	return p.yOffset
}

func (p *ReviewPane) VisibleRange() (int, int) {
	if p.height <= 0 || len(p.rows) == 0 {
		return 0, 0
	}
	start := min(max(p.yOffset, 0), len(p.rows))
	end := min(start+p.height, len(p.rows))
	return start, end
}

func (p *ReviewPane) GotoTop() {
	p.SetYOffset(0)
}

func (p *ReviewPane) SetVisualState(state render.ReviewVisualState) {
	p.visualState = state
}

func (p *ReviewPane) GetContent() string {
	parts := make([]string, len(p.rows))
	for i, row := range p.rows {
		parts[i] = fmt.Sprintf("%s:%d:%d:%d:%s", row.Kind, row.FileIndex, row.SectionIndex, row.LineIndex, row.Message)
	}
	return strings.Join(parts, "\n")
}

func (p *ReviewPane) ScrollPercent() float64 {
	if len(p.rows) <= p.height || p.maxYOffset() == 0 {
		return 1.0
	}
	return float64(p.yOffset) / float64(p.maxYOffset())
}

func (p *ReviewPane) KeepVisible(row int) {
	if p.height <= 0 {
		return
	}
	top := p.yOffset
	bottom := top + p.height - 1
	switch {
	case row < top:
		p.SetYOffset(row)
	case row > bottom:
		p.SetYOffset(row - p.height + 1)
	}
}

func (p ReviewPane) View(states ...render.ReviewVisualState) string {
	state := p.visualState
	if len(states) > 0 {
		state = states[0]
	}
	height := max(p.height, 0)
	if height == 0 || p.width == 0 {
		return ""
	}
	lines := make([]string, 0, height)
	start, end := p.VisibleRange()
	for rowIndex := start; rowIndex < end; rowIndex++ {
		row := p.rows[rowIndex]
		line := p.renderRow(row, rowIndex, state)
		lines = append(lines, p.truncate(line))
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (p ReviewPane) renderRow(row presenter.ReviewRow, rowIndex int, state render.ReviewVisualState) string {
	if p.renderer == nil {
		return ""
	}
	gutter := p.renderer.Gutter(row, rowIndex, state)
	content := p.truncateContent(p.renderer.Render(row, rowIndex, state), lipgloss.Width(gutter))
	style := p.renderer.Style(rowIndex, state)
	content = style.Render(content)
	return gutter + content
}

func (p ReviewPane) truncate(line string) string {
	if p.width <= 0 || ansi.StringWidth(line) <= p.width {
		return line
	}
	return ansi.Cut(line, 0, p.width)
}

func (p ReviewPane) truncateContent(content string, gutterWidth int) string {
	contentWidth := max(p.width-gutterWidth, 0)
	if contentWidth == 0 {
		return ""
	}
	if ansi.StringWidth(content) <= contentWidth {
		return content
	}
	return ansi.Cut(content, 0, contentWidth)
}

func (p *ReviewPane) syncRendererWidth() {
	setter, ok := p.renderer.(reviewPaneWidthSetter)
	if !ok {
		return
	}
	gutterWidth := 0
	if p.renderer != nil {
		gutterWidth = lipgloss.Width(p.renderer.Gutter(presenter.ReviewRow{}, 0, render.ReviewVisualState{}))
	}
	setter.SetWidth(max(p.width-gutterWidth, 0))
}

func (p ReviewPane) maxYOffset() int {
	return max(0, len(p.rows)-max(p.height, 0))
}

func (p *ReviewPane) clampYOffset() {
	p.yOffset = min(max(p.yOffset, 0), p.maxYOffset())
}
