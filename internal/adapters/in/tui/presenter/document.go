package presenter

import (
	"fmt"
	"strings"

	"ero/internal/core"
)

type ReviewDocumentInput struct {
	Files       []core.ReviewFile
	Annotations ReviewAnnotations
}

type ReviewAnnotations struct {
	Comments      []core.ReviewComment
	RemoteThreads []core.RemoteReviewThread
	Editor        *ReviewEditorAnnotation
}

type ReviewDocument struct {
	Rows             []ReviewRow
	Anchors          ReviewAnchors
	ExpanderRows     map[ReviewExpanderAnchor]int
	LineNumberWidths map[int]int
}

func BuildReviewDocument(input ReviewDocumentInput) ReviewDocument {
	builder := reviewDocumentBuilder{
		input:            input,
		lineNumberWidths: make(map[int]int, len(input.Files)),
	}
	return builder.build()
}

type reviewDocumentBuilder struct {
	input            ReviewDocumentInput
	rows             []ReviewRow
	lineNumberWidths map[int]int
}

func (b *reviewDocumentBuilder) build() ReviewDocument {
	b.appendUnmappedRemoteThreads()
	if len(b.input.Files) == 0 {
		b.rows = append(b.rows,
			ReviewRow{Kind: ReviewRowKindMessage, Message: "Review"},
			ReviewRow{Kind: ReviewRowKindMessage, Message: "No files to review"},
		)
		return b.document()
	}

	for fileIndex, file := range b.input.Files {
		b.appendFile(fileIndex, file)
	}
	return b.document()
}

func (b *reviewDocumentBuilder) appendUnmappedRemoteThreads() {
	for _, thread := range b.input.Annotations.RemoteThreads {
		if !thread.Unmapped && thread.FilePath != "" {
			continue
		}
		b.rows = append(b.rows, ReviewRow{Kind: ReviewRowKindRemoteThread, FileIndex: -1, SectionIndex: -1, LineIndex: -1, Annotation: ReviewAnnotation{RemoteThread: thread}})
	}
}

func (b *reviewDocumentBuilder) appendFile(fileIndex int, file core.ReviewFile) {
	if fileIndex > 0 {
		b.rows = append(b.rows, ReviewRow{Kind: ReviewRowKindBlank, FileIndex: fileIndex, FilePath: file.Path})
	}

	b.lineNumberWidths[fileIndex] = lineNumberWidth(file)
	b.rows = append(b.rows,
		ReviewRow{Kind: ReviewRowKindFile, FileIndex: fileIndex, FilePath: file.Path, FileStats: fileStats(file)},
		ReviewRow{Kind: ReviewRowKindRule, FileIndex: fileIndex, FilePath: file.Path},
	)

	for sectionIndex, section := range file.Sections {
		switch section.Kind {
		case core.SectionKindChanged:
			b.appendChangedSection(fileIndex, sectionIndex, file.Path, section)
		case core.SectionKindContext:
			b.appendContextSection(fileIndex, sectionIndex, file, section)
		}
	}
}

func (b *reviewDocumentBuilder) appendChangedSection(fileIndex, sectionIndex int, filePath string, section core.ReviewSection) {
	for lineIndex, line := range section.VisibleLines() {
		b.appendLine(fileIndex, sectionIndex, lineIndex, filePath, line)
	}
}

func (b *reviewDocumentBuilder) appendContextSection(fileIndex, sectionIndex int, file core.ReviewFile, section core.ReviewSection) {
	aboveCount := min(section.ExpandedAbove, len(section.Lines))
	for lineIndex := range aboveCount {
		b.appendLine(fileIndex, sectionIndex, lineIndex, file.Path, section.Lines[lineIndex])
	}

	if section.HiddenLineCount() > 0 {
		b.rows = append(b.rows, ReviewRow{
			Kind:         ReviewRowKindExpander,
			FileIndex:    fileIndex,
			SectionIndex: sectionIndex,
			LineIndex:    -1,
			FilePath:     file.Path,
			Expander: ReviewExpander{
				HiddenLines: section.HiddenLineCount(),
				Position:    contextPosition(file, sectionIndex),
			},
			Selectable: true,
		})
	}

	belowCount := min(section.ExpandedBelow, len(section.Lines)-aboveCount)
	belowStart := len(section.Lines) - belowCount
	for lineIndex := belowStart; lineIndex < len(section.Lines); lineIndex++ {
		b.appendLine(fileIndex, sectionIndex, lineIndex, file.Path, section.Lines[lineIndex])
	}
}

func (b *reviewDocumentBuilder) appendLine(fileIndex, sectionIndex, lineIndex int, filePath string, line core.ReviewLine) {
	row := ReviewRow{
		Kind:         ReviewRowKindLine,
		FileIndex:    fileIndex,
		SectionIndex: sectionIndex,
		LineIndex:    lineIndex,
		FilePath:     filePath,
		Line:         line,
		Selectable:   true,
	}
	b.rows = append(b.rows, row)
	b.appendAnnotationsAfter(row)
}

func (b *reviewDocumentBuilder) appendAnnotationsAfter(row ReviewRow) {
	for _, comment := range b.input.Annotations.Comments {
		if commentBelongsAfterRow(comment, row) {
			b.appendCommentAnnotationRows(row, comment)
		}
	}
	for _, thread := range b.input.Annotations.RemoteThreads {
		if remoteThreadBelongsAfterRow(thread, row) {
			b.appendRemoteThreadAnnotationRows(row, thread)
		}
	}
	if b.input.Annotations.Editor != nil && editorBelongsAfterRow(*b.input.Annotations.Editor, row) {
		b.appendEditorAnnotationRows(row, *b.input.Annotations.Editor)
	}
}

func (b *reviewDocumentBuilder) appendCommentAnnotationRows(row ReviewRow, comment core.ReviewComment) {
	b.rows = append(b.rows, ReviewRow{Kind: ReviewRowKindComment, FileIndex: row.FileIndex, SectionIndex: row.SectionIndex, LineIndex: row.LineIndex, FilePath: row.FilePath, Annotation: ReviewAnnotation{Comment: comment, LineIndex: 0}})
	for lineIndex, body := range strings.Split(comment.Body, "\n") {
		b.rows = append(b.rows, ReviewRow{Kind: ReviewRowKindComment, FileIndex: row.FileIndex, SectionIndex: row.SectionIndex, LineIndex: row.LineIndex, FilePath: row.FilePath, Annotation: ReviewAnnotation{Comment: comment, LineIndex: lineIndex + 1, Body: body}})
	}
}

func (b *reviewDocumentBuilder) appendRemoteThreadAnnotationRows(row ReviewRow, thread core.RemoteReviewThread) {
	b.rows = append(b.rows, ReviewRow{Kind: ReviewRowKindRemoteThread, FileIndex: row.FileIndex, SectionIndex: row.SectionIndex, LineIndex: row.LineIndex, FilePath: row.FilePath, Annotation: ReviewAnnotation{RemoteThread: thread, LineIndex: 0}})
	lineIndex := 1
	for _, comment := range thread.Comments {
		author := comment.Author
		if author == "" {
			author = "remote"
		}
		for _, body := range strings.Split(comment.Body, "\n") {
			b.rows = append(b.rows, ReviewRow{Kind: ReviewRowKindRemoteThread, FileIndex: row.FileIndex, SectionIndex: row.SectionIndex, LineIndex: row.LineIndex, FilePath: row.FilePath, Annotation: ReviewAnnotation{RemoteThread: thread, LineIndex: lineIndex, Author: author, Body: body}})
			lineIndex++
		}
	}
}

func (b *reviewDocumentBuilder) appendEditorAnnotationRows(row ReviewRow, editor ReviewEditorAnnotation) {
	b.rows = append(b.rows, ReviewRow{Kind: ReviewRowKindEditor, FileIndex: row.FileIndex, SectionIndex: row.SectionIndex, LineIndex: row.LineIndex, FilePath: row.FilePath, Annotation: ReviewAnnotation{Editor: editor, LineIndex: 0}})
	for lineIndex := 1; lineIndex <= editor.LineCount; lineIndex++ {
		b.rows = append(b.rows, ReviewRow{Kind: ReviewRowKindEditor, FileIndex: row.FileIndex, SectionIndex: row.SectionIndex, LineIndex: row.LineIndex, FilePath: row.FilePath, Annotation: ReviewAnnotation{Editor: editor, LineIndex: lineIndex}})
	}
}

func (b *reviewDocumentBuilder) document() ReviewDocument {
	return ReviewDocument{
		Rows:             b.rows,
		Anchors:          AnchorsFromRows(b.rows),
		ExpanderRows:     ExpanderRowsFromRows(b.rows),
		LineNumberWidths: b.lineNumberWidths,
	}
}

func AnchorsFromRows(rows []ReviewRow) ReviewAnchors {
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

func ExpanderRowsFromRows(rows []ReviewRow) map[ReviewExpanderAnchor]int {
	expanders := map[ReviewExpanderAnchor]int{}
	for rowIndex, row := range rows {
		if row.Kind == ReviewRowKindExpander {
			expanders[ReviewExpanderAnchor{FileIndex: row.FileIndex, SectionIndex: row.SectionIndex}] = rowIndex
		}
	}
	return expanders
}

func fileStats(file core.ReviewFile) ReviewFileStats {
	stats := ReviewFileStats{}
	for _, section := range file.Sections {
		for _, line := range section.Lines {
			switch line.Kind {
			case core.LineKindAdded:
				stats.Added++
			case core.LineKindDeleted:
				stats.Deleted++
			}
		}
	}
	return stats
}

func lineNumberWidth(file core.ReviewFile) int {
	width := 4
	for _, section := range file.Sections {
		for _, line := range section.Lines {
			if digits := len(fmt.Sprintf("%d", max(line.OldLineNumber, line.NewLineNumber))); digits > width {
				width = digits
			}
		}
	}
	return width
}

func contextPosition(file core.ReviewFile, sectionIndex int) ReviewContextPosition {
	switch {
	case len(file.Sections) == 1:
		return ReviewContextOnlySection
	case sectionIndex == 0:
		return ReviewContextAtFileStart
	case sectionIndex == len(file.Sections)-1:
		return ReviewContextAtFileEnd
	default:
		return ReviewContextBetweenChanges
	}
}

func commentBelongsAfterRow(comment core.ReviewComment, row ReviewRow) bool {
	if row.Kind != ReviewRowKindLine || row.FilePath != comment.FilePath {
		return false
	}
	return reviewLineMatchesRef(row.Line, comment.Range.End)
}

func remoteThreadBelongsAfterRow(thread core.RemoteReviewThread, row ReviewRow) bool {
	if thread.Unmapped || row.Kind != ReviewRowKindLine || row.FilePath != thread.FilePath {
		return false
	}
	return reviewLineMatchesRef(row.Line, thread.Range.End)
}

func editorBelongsAfterRow(editor ReviewEditorAnnotation, row ReviewRow) bool {
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
