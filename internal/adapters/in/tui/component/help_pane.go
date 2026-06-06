package component

import (
	"strings"

	"ero/internal/adapters/in/tui/theme"
)

func RenderHelpPane(width, height int, enterKeyLabel, commentSubmitKeyLabel string) string {
	paneWidth := min(max(width-8, 28), 64)
	contentWidth := max(paneWidth-6, 1)
	lines := []string{
		theme.HelpPaneTitleStyle.Render("Keyboard shortcuts"),
		"",
		theme.HelpSectionStyle.Render("Review"),
		renderHelpShortcut("↑↓/j/k", "scroll", contentWidth),
		renderHelpShortcut("pgup/pgdown", "page", contentWidth),
		renderHelpShortcut("home/end", "jump to start/end", contentWidth),
		renderHelpShortcut("f", "find file", contentWidth),
		renderHelpShortcut("/", "grep references", contentWidth),
		renderHelpShortcut("d", "switch diff mode", contentWidth),
		renderHelpShortcut("p", "switch provider", contentWidth),
		renderHelpShortcut("alt+p", "cycle provider", contentWidth),
		renderHelpShortcut("r/o", "refresh provider/PR sheet", contentWidth),
		"",
		theme.HelpSectionStyle.Render("Search"),
		renderHelpShortcut("↑↓", "select result", contentWidth),
		renderHelpShortcut(enterKeyLabel, "jump to result", contentWidth),
		renderHelpShortcut("esc", "cancel search", contentWidth),
		"",
		renderHelpShortcut("h/l", "previous/next chunk", contentWidth),
		renderHelpShortcut("a", "expand all context", contentWidth),
		renderHelpShortcut(enterKeyLabel, "expand more context", contentWidth),
		renderHelpShortcut("s/space", "select lines", contentWidth),
		renderHelpShortcut("c", "comment selection", contentWidth),
		renderHelpShortcut("P", "publish review", contentWidth),
		renderHelpShortcut("C/x", "copy/clear review", contentWidth),
		renderHelpShortcut("y/Y", "copy plain/rich", contentWidth),
		renderHelpShortcut("q", "quit", contentWidth),
		"",
		theme.HelpSectionStyle.Render("Comment editor"),
		renderHelpShortcut(enterKeyLabel, "new line", contentWidth),
		renderHelpShortcut(commentSubmitKeyLabel, "submit comment", contentWidth),
		renderHelpShortcut("esc", "cancel comment", contentWidth),
		"",
		renderHelpShortcut("?/esc", "close help", contentWidth),
	}
	lines = fitHelpLines(lines, max(height-2, 1))
	return theme.HelpPaneStyle.Width(paneWidth).Render(strings.Join(lines, "\n"))
}

func fitHelpLines(lines []string, maxLines int) []string {
	if len(lines) <= maxLines {
		return lines
	}
	if maxLines <= 1 {
		return lines[:maxLines]
	}
	result := append([]string(nil), lines[:maxLines-1]...)
	result = append(result, lines[len(lines)-1])
	return result
}

func renderHelpShortcut(key, label string, width int) string {
	keyWidth := min(14, max(width/2, 1))
	labelWidth := max(width-keyWidth, 1)
	return theme.HelpKeyStyle.Width(keyWidth).Render(TruncateRunes(key, keyWidth)) + theme.HelpLabelStyle.Render(TruncateRunes(label, labelWidth))
}
