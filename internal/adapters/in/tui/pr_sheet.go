package tui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type prSheetToggledMsg struct{}

type prSheetScrolledMsg struct {
	delta int
}

type prSheetState struct {
	open   bool
	yOffset int
}

func (m Model) TogglePRSheet() Model {
	m.prSheet.open = !m.prSheet.open
	return m
}

func (m Model) ScrollPRSheet(delta int) Model {
	m.prSheet.yOffset = clampPRSheetOffset(m.prSheet.yOffset+delta, m.prSheetLineCount())
	return m
}

func (m Model) renderPRSheetOverlay(content string) string {
	width := max(m.width, 1)
	height := max(m.height, 1)
	pane := m.renderPRSheet(width, height)
	paneWidth := lipgloss.Width(pane)

	canvas := lipgloss.NewCanvas(width, height)
	compositor := lipgloss.NewCompositor(
		lipgloss.NewLayer(content),
		lipgloss.NewLayer(pane).X(max(width-paneWidth, 0)).Y(0).Z(1),
	)
	canvas.Compose(compositor)
	return canvas.Render()
}

func (m Model) renderPRSheet(width, height int) string {
	sheetWidth := prSheetWidth(width)
	contentWidth := max(sheetWidth-2, 1)
	lines := m.prSheetLines()
	m.prSheet.yOffset = clampPRSheetOffset(m.prSheet.yOffset, len(lines))
	visibleLines := visiblePRSheetLines(lines, m.prSheet.yOffset, height)

	rows := make([]string, height)
	for i := range height {
		text := ""
		if i < len(visibleLines) {
			text = visibleLines[i]
		}
		row := "│ " + truncatePlainRow(text, contentWidth)
		rows[i] = padRight(row, sheetWidth)
	}
	return strings.Join(rows, "\n")
}

func (m Model) prSheetLines() []string {
	provider := "No active provider"
	if m.activeRuntimeInfo.ID != "" || m.activeRuntimeInfo.Label != "" || m.activeRuntimeInfo.Name != "" {
		provider = providerDisplayLabel(m.activeRuntimeInfo)
	}
	return []string{
		"Pull request",
		"",
		"Provider: " + provider,
		"",
		"Overview placeholder",
		"Phase 4 will render PR markdown and metadata here.",
		"",
		"Remote threads: " + pluralCount(len(m.remoteThreads), "thread"),
	}
}

func (m Model) prSheetLineCount() int {
	return len(m.prSheetLines())
}

func prSheetWidth(totalWidth int) int {
	if totalWidth <= 1 {
		return 1
	}
	return min(max(totalWidth/3, 32), totalWidth)
}

func visiblePRSheetLines(lines []string, offset, height int) []string {
	if height <= 0 || len(lines) == 0 {
		return nil
	}
	offset = clampPRSheetOffset(offset, len(lines))
	end := min(offset+height, len(lines))
	return lines[offset:end]
}

func clampPRSheetOffset(offset, lineCount int) int {
	if lineCount <= 0 {
		return 0
	}
	return min(max(offset, 0), lineCount-1)
}

func pluralCount(count int, singular string) string {
	return strconv.Itoa(count) + " " + pluralize(singular, count)
}

func padRight(s string, width int) string {
	if lipgloss.Width(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-lipgloss.Width(s))
}

func togglePRSheetCmd() tea.Cmd {
	return func() tea.Msg { return prSheetToggledMsg{} }
}
