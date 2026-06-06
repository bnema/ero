package tui

import (
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"ero/internal/core"
)

type prSheetToggledMsg struct{}

type prSheetScrolledMsg struct {
	delta int
}

type prSheetState struct {
	open    bool
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
	paneWidth := prSheetWidth(width)
	pane := m.renderPRSheet(width, height)

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
		row := "│ " + ansi.Truncate(text, contentWidth, "")
		rows[i] = padRightANSI(row, sheetWidth)
	}
	return strings.Join(rows, "\n")
}

func (m Model) prSheetLines() []string {
	provider := "No active provider"
	if m.activeRuntimeInfo.ID != "" || m.activeRuntimeInfo.Label != "" || m.activeRuntimeInfo.Name != "" {
		provider = providerDisplayLabel(m.activeRuntimeInfo)
	}

	lines := []string{
		"Pull request",
		"",
		"Provider: " + provider,
	}
	if m.providerSyncState.Status != "" {
		lines = append(lines, "Sync: "+string(m.providerSyncState.Status))
	}
	lines = append(lines, "Remote threads: "+pluralCount(len(m.remoteThreads), "thread"), "")

	if m.providerOverview == nil {
		return append(lines,
			"No provider overview loaded.",
			"Open an active provider that supports PR snapshots to show PR metadata, body, comments, and reviews here.",
		)
	}
	return append(lines, m.providerOverviewLines(m.providerOverview)...)
}

func (m Model) providerOverviewLines(overview *core.ProviderOverview) []string {
	contentWidth := max(prSheetWidth(m.width)-2, 1)
	lines := []string{}
	if strings.TrimSpace(overview.Title) != "" {
		lines = append(lines, overview.Title)
	} else {
		lines = append(lines, "Untitled pull request")
	}
	metadata := providerOverviewMetadata(overview)
	if len(metadata) > 0 {
		lines = append(lines, metadata...)
	}
	if strings.TrimSpace(overview.ExternalURL) != "" {
		lines = append(lines, overview.ExternalURL)
	}
	lines = append(lines, "")

	if strings.TrimSpace(overview.Body) != "" {
		lines = append(lines, "Body", "")
		lines = append(lines, renderPRSheetMarkdown(m.markdownRenderer, overview.Body, contentWidth)...)
		lines = append(lines, "")
	}

	lines = append(lines, "Issue comments: "+strconv.Itoa(len(overview.Comments)))
	for _, comment := range overview.Comments {
		lines = append(lines, "", commentHeader(comment.Author, comment.CreatedAt))
		lines = append(lines, renderPRSheetMarkdown(m.markdownRenderer, comment.Body, contentWidth)...)
	}

	lines = append(lines, "", "Review summaries: "+strconv.Itoa(len(overview.Reviews)))
	for _, review := range overview.Reviews {
		lines = append(lines, "", reviewSummaryHeader(review))
		if strings.TrimSpace(review.Body) != "" {
			lines = append(lines, renderPRSheetMarkdown(m.markdownRenderer, review.Body, contentWidth)...)
		}
	}
	return trimTrailingBlankLines(lines)
}

func (m Model) prSheetLineCount() int {
	return len(m.prSheetLines())
}

func renderPRSheetMarkdown(renderer *MarkdownRenderer, markdown string, width int) []string {
	rendered := sanitizeRenderedMarkdown(renderer.Render(markdown, width, MarkdownThemeDark))
	if strings.TrimSpace(safeMarkdownFallback(rendered)) == "" {
		return []string{"(empty)"}
	}
	return trimRenderedMarkdownBlankLines(strings.Split(rendered, "\n"))
}

func trimRenderedMarkdownBlankLines(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(safeMarkdownFallback(lines[0])) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(safeMarkdownFallback(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return []string{"(empty)"}
	}
	return lines
}

func providerOverviewMetadata(overview *core.ProviderOverview) []string {
	if overview == nil {
		return nil
	}
	parts := make([]string, 0, 4)
	if overview.Number > 0 {
		parts = append(parts, "#"+strconv.Itoa(overview.Number))
	}
	if strings.TrimSpace(overview.State) != "" {
		parts = append(parts, strings.TrimSpace(overview.State))
	}
	if strings.TrimSpace(overview.Author) != "" {
		parts = append(parts, "by "+strings.TrimSpace(overview.Author))
	}
	if strings.TrimSpace(overview.BaseRef) != "" || strings.TrimSpace(overview.HeadRef) != "" {
		parts = append(parts, strings.TrimSpace(overview.BaseRef)+" ← "+strings.TrimSpace(overview.HeadRef))
	}
	lines := []string{}
	if len(parts) > 0 {
		lines = append(lines, strings.Join(parts, " · "))
	}
	if overview.UpdatedAt != nil && !overview.UpdatedAt.IsZero() {
		lines = append(lines, "Updated "+overview.UpdatedAt.Local().Format("2006-01-02 15:04"))
	}
	return lines
}

func commentHeader(author string, createdAt time.Time) string {
	parts := []string{"• Comment"}
	if strings.TrimSpace(author) != "" {
		parts = append(parts, "by "+strings.TrimSpace(author))
	}
	if !createdAt.IsZero() {
		parts = append(parts, createdAt.Local().Format("2006-01-02 15:04"))
	}
	return strings.Join(parts, " ")
}

func reviewSummaryHeader(review core.ProviderReviewSummary) string {
	marker := reviewStateMarker(review.State)
	parts := []string{marker}
	if strings.TrimSpace(review.State) != "" {
		parts = append(parts, strings.ToUpper(strings.TrimSpace(review.State)))
	}
	if strings.TrimSpace(review.Author) != "" {
		parts = append(parts, "by "+strings.TrimSpace(review.Author))
	}
	if !review.SubmittedAt.IsZero() {
		parts = append(parts, review.SubmittedAt.Local().Format("2006-01-02 15:04"))
	}
	return strings.Join(parts, " ")
}

func reviewStateMarker(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "approved", "approve":
		return "✓"
	case "changes_requested", "request_changes", "changes requested":
		return "!"
	case "commented", "comment":
		return "•"
	case "dismissed":
		return "×"
	default:
		return "-"
	}
}

func trimTrailingBlankLines(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func prSheetWidth(totalWidth int) int {
	if totalWidth <= 1 {
		return 1
	}
	return max(totalWidth/2, 1)
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

func padRightANSI(s string, width int) string {
	if ansi.StringWidth(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-ansi.StringWidth(s))
}
