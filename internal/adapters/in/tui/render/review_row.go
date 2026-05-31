package render

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"ero/internal/adapters/in/tui/presenter"
	"ero/internal/adapters/in/tui/theme"
	"ero/internal/core"
)

type CommentMarker string

const (
	CommentMarkerStart CommentMarker = "start"
	CommentMarkerBody  CommentMarker = "body"
	CommentMarkerEnd   CommentMarker = "end"
)

type ReviewVisualState struct {
	CursorRow        int
	SelectionActive  bool
	SelectionStart   int
	SelectionEnd     int
	SelectedExpander *presenter.ReviewExpanderAnchor
	CommentMarkers   map[int]CommentMarker
}

type ReviewRowRendererConfig struct {
	Width              int
	EnterKeyLabel      string
	LineNumberWidths   map[int]int
	LineCache          *ReviewLineCache
	CommentIcon        string
	CursorMarker       string
	SelectionMarker    string
	CommentStartMarker string
	CommentBodyMarker  string
	CommentEndMarker   string
	EditorLineRenderer func(presenter.ReviewEditorAnnotation, int, int) string
}

type ReviewRowRenderer struct {
	config ReviewRowRendererConfig
}

func NewReviewRowRenderer(config ReviewRowRendererConfig) *ReviewRowRenderer {
	if config.Width <= 0 {
		config.Width = 80
	}
	if config.EnterKeyLabel == "" {
		config.EnterKeyLabel = "enter"
	}
	if config.LineCache == nil {
		config.LineCache = NewReviewLineCache()
	}
	if config.CommentIcon == "" {
		config.CommentIcon = "󰅺"
	}
	if config.CursorMarker == "" {
		config.CursorMarker = "➜ "
	}
	if config.SelectionMarker == "" {
		config.SelectionMarker = "┃ "
	}
	if config.CommentStartMarker == "" {
		config.CommentStartMarker = "╭ "
	}
	if config.CommentBodyMarker == "" {
		config.CommentBodyMarker = "│ "
	}
	if config.CommentEndMarker == "" {
		config.CommentEndMarker = "╰ "
	}
	return &ReviewRowRenderer{config: config}
}

func (r *ReviewRowRenderer) SetWidth(width int) {
	if r == nil {
		return
	}
	r.config.Width = max(width, 0)
}

func (r *ReviewRowRenderer) Render(row presenter.ReviewRow, rowIndex int, state ReviewVisualState) string {
	switch row.Kind {
	case presenter.ReviewRowKindBlank:
		return ""
	case presenter.ReviewRowKindFile:
		return r.renderFile(row)
	case presenter.ReviewRowKindRule:
		return theme.FileRuleStyle.Render(strings.Repeat("─", max(r.config.Width, 1)))
	case presenter.ReviewRowKindLine:
		return r.config.LineCache.Render(row, r.lineNumberWidth(row.FileIndex))
	case presenter.ReviewRowKindExpander:
		return r.renderExpander(row, state)
	case presenter.ReviewRowKindMessage:
		return r.renderMessage(row)
	case presenter.ReviewRowKindComment:
		return r.renderComment(row)
	case presenter.ReviewRowKindRemoteThread:
		return r.renderRemoteThread(row)
	case presenter.ReviewRowKindEditor:
		return r.renderEditor(row)
	default:
		return row.Text
	}
}

func (r *ReviewRowRenderer) Gutter(_ presenter.ReviewRow, rowIndex int, state ReviewVisualState) string {
	if marker, ok := state.CommentMarkers[rowIndex]; ok {
		switch marker {
		case CommentMarkerStart:
			return inlineCommentIconStyle.Render(r.config.CommentStartMarker)
		case CommentMarkerEnd:
			return inlineCommentIconStyle.Render(r.config.CommentEndMarker)
		default:
			return inlineCommentIconStyle.Render(r.config.CommentBodyMarker)
		}
	}
	if rowIndex == state.CursorRow {
		return theme.StatusKeyStyle.Render(r.config.CursorMarker)
	}
	if state.SelectionActive && rowIndex >= state.SelectionStart && rowIndex <= state.SelectionEnd {
		return theme.SelectedExpander.Render(r.config.SelectionMarker)
	}
	return "  "
}

func (r *ReviewRowRenderer) Style(rowIndex int, state ReviewVisualState) lipgloss.Style {
	if state.SelectionActive && rowIndex >= state.SelectionStart && rowIndex <= state.SelectionEnd {
		return theme.SelectedRowStyle
	}
	if rowIndex == state.CursorRow {
		return theme.CursorRowStyle
	}
	if _, ok := state.CommentMarkers[rowIndex]; ok {
		return theme.CommentRangeRowStyle
	}
	return lipgloss.NewStyle()
}

func (r *ReviewRowRenderer) renderFile(row presenter.ReviewRow) string {
	left := theme.FileHeaderStyle.Render(row.FilePath)
	right := theme.MutedStyle.Render(fmt.Sprintf("+%d -%d", row.FileStats.Added, row.FileStats.Deleted))
	space := max(r.config.Width-lipgloss.Width(left)-lipgloss.Width(right), 1)
	return left + strings.Repeat(" ", space) + right
}

func (r *ReviewRowRenderer) renderExpander(row presenter.ReviewRow, state ReviewVisualState) string {
	label := "⋯ " + hiddenLinesLabel(row.Expander.HiddenLines) + contextLocationLabel(row.Expander.Position) + " · [" + r.config.EnterKeyLabel + "] show more · [a] show all"
	style := theme.MutedStyle
	if state.SelectedExpander != nil && state.SelectedExpander.FileIndex == row.FileIndex && state.SelectedExpander.SectionIndex == row.SectionIndex {
		style = theme.SelectedExpander
	}
	return style.Inline(true).MaxWidth(r.config.Width).Render(label)
}

func (r *ReviewRowRenderer) renderMessage(row presenter.ReviewRow) string {
	if row.Message == "Review" {
		return theme.PanelTitleStyle.Render(row.Message)
	}
	return theme.MutedStyle.Render(row.Message)
}

func (r *ReviewRowRenderer) renderComment(row presenter.ReviewRow) string {
	if row.Annotation.LineIndex == 0 {
		return r.annotationIndent(row) + inlineCommentStyle.Render(inlineCommentIconStyle.Render(r.config.CommentIcon)+" "+inlineCommentIDStyle.Render(displayReviewCommentID(row.Annotation.Comment.ID)))
	}
	return r.annotationIndent(row) + inlineCommentStyle.Render(inlineCommentBodyStyle.Render(row.Annotation.Body))
}

func (r *ReviewRowRenderer) renderRemoteThread(row presenter.ReviewRow) string {
	thread := row.Annotation.RemoteThread
	if thread.Unmapped {
		return inlineCommentStyle.Render(unmappedRemoteThreadSummary(thread))
	}
	if row.Annotation.LineIndex == 0 {
		return r.annotationIndent(row) + inlineCommentStyle.Render(inlineCommentIconStyle.Render(r.config.CommentIcon)+" "+inlineCommentIDStyle.Render(providerThreadLabel(thread))+" "+inlineCommentMutedStyle.Render("remote read-only"))
	}
	author := row.Annotation.Author
	if author == "" {
		author = "remote"
	}
	return r.annotationIndent(row) + inlineCommentStyle.Render(inlineCommentBodyStyle.Render(author+": "+row.Annotation.Body))
}

func (r *ReviewRowRenderer) renderEditor(row presenter.ReviewRow) string {
	if row.Annotation.LineIndex == 0 {
		return r.annotationIndent(row) + inlineCommentMutedStyle.Render("commenting "+formatReviewLineRange(row.Annotation.Editor.Range))
	}
	if r.config.EditorLineRenderer == nil {
		return ""
	}
	return r.annotationIndent(row) + r.config.EditorLineRenderer(row.Annotation.Editor, row.Annotation.LineIndex-1, max(r.config.Width-len([]rune(r.annotationIndent(row))), 1))
}

func (r *ReviewRowRenderer) annotationIndent(row presenter.ReviewRow) string {
	return strings.Repeat(" ", r.lineNumberWidth(row.FileIndex)*2+4)
}

func (r *ReviewRowRenderer) lineNumberWidth(fileIndex int) int {
	if width, ok := r.config.LineNumberWidths[fileIndex]; ok && width > 0 {
		return width
	}
	return 4
}

func hiddenLinesLabel(hidden int) string {
	if hidden == 1 {
		return "1 hidden line"
	}
	return fmt.Sprintf("%d hidden lines", hidden)
}

func contextLocationLabel(position presenter.ReviewContextPosition) string {
	switch position {
	case presenter.ReviewContextAtFileStart:
		return " from beginning of file"
	case presenter.ReviewContextAtFileEnd:
		return " to end of file"
	case presenter.ReviewContextOnlySection:
		return " in file"
	default:
		return " between changes"
	}
}

var (
	inlineCommentStyle      = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, false, true).BorderForeground(lipgloss.Color("62")).PaddingLeft(1)
	inlineCommentIconStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
	inlineCommentIDStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true)
	inlineCommentBodyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))
	inlineCommentMutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

func displayReviewCommentID(id string) string {
	if number, ok := strings.CutPrefix(id, "comment-"); ok && number != "" {
		return "#" + number
	}
	return inlineCommentMutedStyle.Render(id)
}

func providerThreadLabel(thread core.RemoteReviewThread) string {
	if thread.ProviderID != "" {
		return "[" + thread.ProviderID + "]"
	}
	return "[remote]"
}

func unmappedRemoteThreadSummary(thread core.RemoteReviewThread) string {
	label := providerThreadLabel(thread)
	if len(thread.Comments) == 0 {
		return label + " unmapped remote thread"
	}
	first := thread.Comments[0].Body
	if len([]rune(first)) > 80 {
		first = string([]rune(first)[:80]) + "…"
	}
	return label + " unmapped: " + first
}

func formatReviewLineRange(lineRange core.ReviewLineRange) string {
	if lineRange.Start.NewLineNumber > 0 {
		if lineRange.End.NewLineNumber > 0 && lineRange.End.NewLineNumber != lineRange.Start.NewLineNumber {
			return fmt.Sprintf("lines %d-%d", lineRange.Start.NewLineNumber, lineRange.End.NewLineNumber)
		}
		return fmt.Sprintf("line %d", lineRange.Start.NewLineNumber)
	}
	if lineRange.Start.OldLineNumber > 0 {
		if lineRange.End.OldLineNumber > 0 && lineRange.End.OldLineNumber != lineRange.Start.OldLineNumber {
			return fmt.Sprintf("old lines %d-%d", lineRange.Start.OldLineNumber, lineRange.End.OldLineNumber)
		}
		return fmt.Sprintf("old line %d", lineRange.Start.OldLineNumber)
	}
	return "selected range"
}
