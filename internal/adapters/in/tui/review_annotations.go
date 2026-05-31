package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"ero/internal/adapters/in/tui/presenter"
	"ero/internal/adapters/in/tui/render"
	"ero/internal/core"
)

var (
	inlineCommentStyle      = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, false, true).BorderForeground(lipgloss.Color("62")).PaddingLeft(1)
	inlineCommentIconStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
	inlineCommentIDStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true)
	inlineCommentBodyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))
	inlineCommentMutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

type ReviewAnnotations struct {
	Comments      []core.ReviewComment
	RemoteThreads []core.RemoteReviewThread
	Editor        *InlineCommentEditor
}

type InlineCommentEditor struct {
	FilePath string
	Range    core.ReviewLineRange
	Editor   CommentEditor
}

func (e *InlineCommentEditor) PresenterAnnotation(availableWidth int) presenter.ReviewEditorAnnotation {
	if e == nil {
		return presenter.ReviewEditorAnnotation{}
	}
	return presenter.ReviewEditorAnnotation{
		FilePath:  e.FilePath,
		Range:     e.Range,
		LineCount: len(strings.Split(e.Editor.ViewWithWidth(availableWidth), "\n")),
	}
}

func (c ReviewDocument) RenderWithAnnotations(files []core.ReviewFile, selectedFile, selectedContext int, annotations ReviewAnnotations) ReviewDocumentRender {
	rendered := c.RenderWithAnchors(files, selectedFile, selectedContext)
	if len(annotations.Comments) == 0 && len(annotations.RemoteThreads) == 0 && annotations.Editor == nil {
		return rendered
	}

	lineIndents := reviewLineContentIndents(files)
	lines := make([]string, 0, len(rendered.Lines)+len(annotations.Comments)*2+4)
	rows := make([]ReviewRow, 0, len(rendered.Rows)+len(annotations.Comments)*2+4)
	for _, thread := range annotations.RemoteThreads {
		if thread.Unmapped || thread.FilePath == "" {
			for _, line := range renderUnmappedRemoteThread(thread) {
				lines = append(lines, line)
				rows = append(rows, ReviewRow{Kind: ReviewRowKindMessage, Text: line})
			}
		}
	}
	for index, row := range rendered.Rows {
		lines = append(lines, rendered.Lines[index])
		rows = append(rows, row)
		indent := inlineAnnotationIndent(row, lineIndents)
		for _, comment := range annotations.Comments {
			if commentBelongsAfterRow(comment, row) {
				for _, commentLine := range renderInlineComment(comment, indent) {
					lines = append(lines, commentLine)
					rows = append(rows, ReviewRow{Kind: ReviewRowKindMessage, FileIndex: row.FileIndex, SectionIndex: row.SectionIndex, FilePath: row.FilePath, Text: commentLine})
				}
			}
		}
		for _, thread := range annotations.RemoteThreads {
			if remoteThreadBelongsAfterRow(thread, row) {
				for _, commentLine := range renderRemoteThread(thread, indent) {
					lines = append(lines, commentLine)
					rows = append(rows, ReviewRow{Kind: ReviewRowKindMessage, FileIndex: row.FileIndex, SectionIndex: row.SectionIndex, FilePath: row.FilePath, Text: commentLine})
				}
			}
		}
		if annotations.Editor != nil && editorBelongsAfterRow(*annotations.Editor, row) {
			availableWidth := max(c.width-len([]rune(indent)), 1)
			header := renderEditorRangeHeader(*annotations.Editor, indent)
			lines = append(lines, header)
			rows = append(rows, ReviewRow{Kind: ReviewRowKindMessage, FileIndex: row.FileIndex, SectionIndex: row.SectionIndex, FilePath: row.FilePath, Text: header})
			for editorLine := range strings.SplitSeq(annotations.Editor.Editor.ViewWithWidth(availableWidth), "\n") {
				indentedLine := indent + editorLine
				lines = append(lines, indentedLine)
				rows = append(rows, ReviewRow{Kind: ReviewRowKindMessage, FileIndex: row.FileIndex, SectionIndex: row.SectionIndex, FilePath: row.FilePath, Text: indentedLine})
			}
		}
	}
	rendered.Content = strings.Join(lines, "\n")
	rendered.Lines = lines
	rendered.Rows = rows
	rendered.Anchors = reviewAnchorsFromRows(rows)
	return rendered
}

func reviewAnchorsFromRows(rows []ReviewRow) ReviewAnchors {
	anchors := ReviewAnchors{FileRows: map[int]int{}, LineRows: map[ReviewLineAnchor]int{}}
	for rowIndex, row := range rows {
		switch row.Kind {
		case ReviewRowKindFile:
			if _, exists := anchors.FileRows[row.FileIndex]; !exists {
				anchors.FileRows[row.FileIndex] = rowIndex
			}
		case ReviewRowKindLine:
			anchors.LineRows[ReviewLineAnchor{FileIndex: row.FileIndex, SectionIndex: row.SectionIndex, LineIndex: row.LineIndex}] = rowIndex
		}
	}
	return anchors
}

func commentBelongsAfterRow(comment core.ReviewComment, row ReviewRow) bool {
	if row.Kind != ReviewRowKindLine || row.FilePath != comment.FilePath {
		return false
	}
	return reviewLineMatchesRef(row.Line, comment.Range.End)
}

func editorBelongsAfterRow(editor InlineCommentEditor, row ReviewRow) bool {
	if row.Kind != ReviewRowKindLine || row.FilePath != editor.FilePath {
		return false
	}
	return reviewLineMatchesRef(row.Line, editor.Range.End)
}

func reviewLineMatchesRef(line core.ReviewLine, ref core.ReviewLineRef) bool {
	if ref.NewLineNumber > 0 && line.NewLineNumber == ref.NewLineNumber {
		return true
	}
	if ref.OldLineNumber > 0 && line.OldLineNumber == ref.OldLineNumber {
		return true
	}
	return false
}

func reviewLineContentIndents(files []core.ReviewFile) map[int]string {
	indents := make(map[int]string, len(files))
	for fileIndex, file := range files {
		indents[fileIndex] = strings.Repeat(" ", render.LineNumberWidth(file)*2+4)
	}
	return indents
}

func inlineAnnotationIndent(row ReviewRow, lineIndents map[int]string) string {
	if row.Kind != ReviewRowKindLine {
		return ""
	}
	return lineIndents[row.FileIndex]
}

func remoteThreadBelongsAfterRow(thread core.RemoteReviewThread, row ReviewRow) bool {
	if thread.Unmapped || row.Kind != ReviewRowKindLine || row.FilePath != thread.FilePath {
		return false
	}
	return reviewLineMatchesRef(row.Line, thread.Range.End)
}

func renderEditorRangeHeader(editor InlineCommentEditor, indent string) string {
	return indent + inlineCommentMutedStyle.Render("commenting "+formatReviewLineRange(editor.Range))
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

func renderInlineComment(comment core.ReviewComment, indent string) []string {
	bodyLines := strings.Split(comment.Body, "\n")
	lines := make([]string, 0, len(bodyLines)+1)
	header := inlineCommentIconStyle.Render(nerdIconComment) + " " + inlineCommentIDStyle.Render(displayReviewCommentID(comment.ID))
	lines = append(lines, indent+inlineCommentStyle.Render(header))
	for _, bodyLine := range bodyLines {
		lines = append(lines, indent+inlineCommentStyle.Render(inlineCommentBodyStyle.Render(bodyLine)))
	}
	return lines
}

func renderRemoteThread(thread core.RemoteReviewThread, indent string) []string {
	lines := []string{indent + inlineCommentStyle.Render(inlineCommentIconStyle.Render(nerdIconComment)+" "+inlineCommentIDStyle.Render(providerThreadLabel(thread))+" "+inlineCommentMutedStyle.Render("remote read-only"))}
	for _, comment := range thread.Comments {
		prefix := comment.Author
		if prefix == "" {
			prefix = "remote"
		}
		for bodyLine := range strings.SplitSeq(comment.Body, "\n") {
			lines = append(lines, indent+inlineCommentStyle.Render(inlineCommentBodyStyle.Render(prefix+": "+bodyLine)))
		}
	}
	return lines
}

func renderUnmappedRemoteThread(thread core.RemoteReviewThread) []string {
	label := providerThreadLabel(thread)
	if len(thread.Comments) == 0 {
		return []string{inlineCommentStyle.Render(label + " unmapped remote thread")}
	}
	first := thread.Comments[0].Body
	if len([]rune(first)) > 80 {
		first = string([]rune(first)[:80]) + "…"
	}
	return []string{inlineCommentStyle.Render(label + " unmapped: " + first)}
}

func providerThreadLabel(thread core.RemoteReviewThread) string {
	if thread.ProviderID != "" {
		return "[" + thread.ProviderID + "]"
	}
	return "[remote]"
}

func displayReviewCommentID(id string) string {
	if number, ok := strings.CutPrefix(id, "comment-"); ok && number != "" {
		return "#" + number
	}
	return inlineCommentMutedStyle.Render(id)
}
