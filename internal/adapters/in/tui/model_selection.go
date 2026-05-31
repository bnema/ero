package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"ero/internal/core"
)

func (m *Model) toggleSelection() {
	if m.selectionAnchorRow != nil {
		m.clearSelection()
		return
	}
	anchor := m.cursorRow
	m.selectionAnchorRow = &anchor
}

func (m *Model) clearSelection() {
	m.selectionAnchorRow = nil
}

func (m *Model) cancelCommentEditor() {
	if m.commentEditor == nil {
		return
	}
	m.commentEditor = nil
	m.syncReviewViewport()
}

func (m Model) copyToClipboard(withMetadata bool) (Model, tea.Cmd) {
	rows := m.selectedRows()
	if len(rows) == 0 {
		m.setCopyFeedback("No diff lines to copy")
		return m, nil
	}
	if m.clipboardWriter == nil {
		m.setCopyFeedback("Copy failed: clipboard unavailable")
		return m, m.expireCopyFeedbackCmd()
	}

	var text string
	if withMetadata {
		text = copyMetadataText(rows)
	} else {
		text = copyPlainText(rows)
	}
	lineCount := len(rows)
	writer := m.clipboardWriter
	return m, func() tea.Msg {
		ctx, cancel := m.clipboardContext()
		defer cancel()
		if err := writer.WriteClipboard(ctx, text); err != nil {
			return clipboardCopyFailedMsg{err: err}
		}
		return clipboardCopiedMsg{text: text, lineCount: lineCount, withMetadata: withMetadata}
	}
}

func (m *Model) setCopyFeedback(message string) {
	m.copyFeedbackID++
	m.copyFeedback = message
}

func (m Model) expireCopyFeedbackCmd() tea.Cmd {
	id := m.copyFeedbackID
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return copyFeedbackExpiredMsg{id: id}
	})
}

func (m Model) selectedRange() (int, int, bool) {
	if m.selectionAnchorRow == nil {
		return m.cursorRow, m.cursorRow, false
	}
	start := min(*m.selectionAnchorRow, m.cursorRow)
	end := max(*m.selectionAnchorRow, m.cursorRow)
	return start, end, true
}

func (m Model) selectedRows() []ReviewRow {
	start, end, _ := m.selectedRange()
	if len(m.reviewRows) == 0 {
		return nil
	}
	start = min(max(start, 0), len(m.reviewRows)-1)
	end = min(max(end, 0), len(m.reviewRows)-1)
	rows := make([]ReviewRow, 0, end-start+1)
	for _, row := range m.reviewRows[start : end+1] {
		if row.Kind == ReviewRowKindLine {
			rows = append(rows, row)
		}
	}
	return rows
}

func copyPlainText(rows []ReviewRow) string {
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, plainDiffLine(row.Line))
	}
	return strings.Join(lines, "\n")
}

func copyMetadataText(rows []ReviewRow) string {
	if len(rows) == 0 {
		return ""
	}

	var b strings.Builder
	for i := 0; i < len(rows); {
		filePath := rows[i].FilePath
		j := i + 1
		for j < len(rows) && rows[j].FilePath == filePath {
			j++
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("File: ")
		b.WriteString(filePath)
		b.WriteString("\n")
		b.WriteString("Lines: ")
		b.WriteString(lineRef(rows[i].Line))
		if j-i > 1 {
			b.WriteString(" to ")
			b.WriteString(lineRef(rows[j-1].Line))
		}
		b.WriteString("\n\n```diff\n")
		for _, row := range rows[i:j] {
			b.WriteString(plainDiffLine(row.Line))
			b.WriteString("\n")
		}
		b.WriteString("```")
		i = j
	}
	return b.String()
}

func plainDiffLine(line core.ReviewLine) string {
	prefix := " "
	switch line.Kind {
	case core.LineKindAdded:
		prefix = "+"
	case core.LineKindDeleted:
		prefix = "-"
	}
	return prefix + " " + line.Content
}

func pluralize(word string, count int) string {
	if count == 1 {
		return word
	}
	return word + "s"
}

func lineRef(line core.ReviewLine) string {
	parts := make([]string, 0, 2)
	if line.OldLineNumber > 0 {
		parts = append(parts, fmt.Sprintf("-%d", line.OldLineNumber))
	}
	if line.NewLineNumber > 0 {
		parts = append(parts, fmt.Sprintf("+%d", line.NewLineNumber))
	}
	return strings.Join(parts, "/")
}

func (m Model) activeLocation() string {
	if m.activeFilePath == "" {
		return ""
	}
	if m.cursorRow < 0 || m.cursorRow >= len(m.reviewRows) {
		return m.activeFilePath
	}
	row := m.reviewRows[m.cursorRow]
	if row.Kind != ReviewRowKindLine {
		return m.activeFilePath
	}
	lineNumber := displayLineNumber(row.Line)
	if lineNumber <= 0 {
		return m.activeFilePath
	}
	return fmt.Sprintf("%s:%d", m.activeFilePath, lineNumber)
}

func (m Model) reviewWidth() int {
	if m.width <= 0 {
		return defaultWidth
	}
	return max(m.width, 1)
}

func sortedReviewFiles(files []core.ReviewFile) []core.ReviewFile {
	result := append([]core.ReviewFile(nil), files...)
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].Path < result[j].Path
	})
	return result
}
