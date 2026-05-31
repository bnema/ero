package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"ero/internal/core"
)

func (m Model) openCommentEditor() (Model, tea.Cmd) {
	filePath, lineRange, ok := reviewCommentRangeFromRows(m.selectedRowsForComment())
	if !ok {
		m.setCopyFeedback("Select contiguous lines in one file to comment")
		return m, nil
	}
	editor := NewCommentEditor(m.reviewWidth())
	m.commentEditor = &InlineCommentEditor{FilePath: filePath, Range: lineRange, Editor: editor}
	m.clearSelection()
	m.syncReviewViewport()
	return m, m.commentEditor.Editor.Focus()
}

func (m Model) updateCommentEditor(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.commentEditor == nil {
		return m, nil
	}
	updatedEditor, action, cmd := m.commentEditor.Editor.Update(msg)
	m.commentEditor.Editor = updatedEditor
	switch action {
	case CommentEditorActionCancel:
		m.cancelCommentEditor()
		return m, nil
	case CommentEditorActionSubmit:
		return m.submitCommentEditor()
	default:
		m.syncReviewViewport()
		return m, cmd
	}
}

func (m Model) submitCommentEditor() (Model, tea.Cmd) {
	if m.commentEditor == nil {
		return m, nil
	}
	body := strings.TrimSpace(m.commentEditor.Editor.Value())
	if body == "" {
		m.setCopyFeedback("Comment is empty")
		return m, nil
	}
	if m.reviewDraft == nil {
		m.reviewDraft = core.NewReviewDraft()
	}
	_, err := m.reviewDraft.AddComment(core.ReviewCommentInput{FilePath: m.commentEditor.FilePath, Range: m.commentEditor.Range, Body: body})
	if err != nil {
		m.setCopyFeedback(err.Error())
		return m, nil
	}
	m.cancelCommentEditor()
	if len(m.providerInfos) == 0 {
		return m.copyReviewJSONToClipboard()
	}
	m.setCopyFeedback("Comment added; press P to publish")
	return m, m.expireCopyFeedbackCmd()
}

func (m Model) copyReviewJSONToClipboard() (Model, tea.Cmd) {
	if m.clipboardWriter == nil {
		m.setCopyFeedback("Copy failed: clipboard unavailable")
		return m, m.expireCopyFeedbackCmd()
	}
	if m.reviewDraft == nil {
		m.reviewDraft = core.NewReviewDraft()
	}
	payload, err := m.reviewDraft.ExportJSON()
	if err != nil {
		m.setCopyFeedback("Copy failed: " + err.Error())
		return m, m.expireCopyFeedbackCmd()
	}
	text := string(payload)
	commentCount := len(m.reviewDraft.Comments())
	m.setCopyFeedback("Copying review JSON…")
	writer := m.clipboardWriter
	return m, func() tea.Msg {
		ctx, cancel := m.clipboardContext()
		defer cancel()
		if err := writer.WriteClipboard(ctx, text); err != nil {
			return clipboardCopyFailedMsg{err: err}
		}
		return clipboardCopiedMsg{text: text, reviewJSON: true, commentCount: commentCount}
	}
}

func (m *Model) clearReviewDraft() {
	if m.reviewDraft == nil {
		m.reviewDraft = core.NewReviewDraft()
	}
	m.reviewDraft.Clear()
	m.cancelCommentEditor()
	m.setCopyFeedback("Cleared review")
	m.syncReviewViewport()
}

func (m Model) selectedRowsForComment() []ReviewRow {
	start, end, ok := m.selectedRange()
	if !ok {
		start, end = m.cursorRow, m.cursorRow
	}
	if len(m.reviewRows) == 0 {
		return nil
	}
	start = min(max(start, 0), len(m.reviewRows)-1)
	end = min(max(end, 0), len(m.reviewRows)-1)
	rows := m.reviewRows[start : end+1]
	for _, row := range rows {
		if row.Kind != ReviewRowKindLine {
			return nil
		}
	}
	return append([]ReviewRow(nil), rows...)
}

func reviewCommentRangeFromRows(rows []ReviewRow) (string, core.ReviewLineRange, bool) {
	if len(rows) == 0 {
		return "", core.ReviewLineRange{}, false
	}
	filePath := rows[0].FilePath
	for _, row := range rows {
		if row.Kind != ReviewRowKindLine || row.FilePath != filePath {
			return "", core.ReviewLineRange{}, false
		}
	}
	return filePath, core.ReviewLineRange{
		Start: reviewLineRef(rows[0].Line),
		End:   reviewLineRef(rows[len(rows)-1].Line),
	}, true
}

func reviewLineRef(line core.ReviewLine) core.ReviewLineRef {
	return core.ReviewLineRef{
		OldLineNumber: line.OldLineNumber,
		NewLineNumber: line.NewLineNumber,
		Kind:          line.Kind,
	}
}
