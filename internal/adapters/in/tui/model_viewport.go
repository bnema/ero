package tui

import (
	"strings"

	"ero/internal/adapters/in/tui/presenter"
	"ero/internal/adapters/in/tui/render"
	"ero/internal/core"
)

func (m *Model) syncReviewViewport() {
	currentCursor := m.cursorRow
	m.rebuildReviewProjection()
	m.cursorRow = m.clampCursorRow(currentCursor)
	m.centerViewportOnCursor()
	m.updateActiveFileFromCursor()
}

func (m *Model) rebuildReviewProjection() {
	width := m.reviewWidth()
	height := max(m.height-1, 1)
	annotations := presenter.ReviewAnnotations{RemoteThreads: m.remoteThreads}
	if m.reviewDraft != nil {
		annotations.Comments = m.reviewDraft.Comments()
	}
	if m.commentEditor != nil {
		editor := m.commentEditor.PresenterAnnotation(max(width-14, 1))
		annotations.Editor = &editor
	}
	doc := presenter.BuildReviewDocument(presenter.ReviewDocumentInput{Files: m.files, Annotations: annotations})
	renderer := render.NewReviewRowRenderer(render.ReviewRowRendererConfig{
		Width:              width - 2,
		EnterKeyLabel:      enterKeyLabel(),
		LineNumberWidths:   doc.LineNumberWidths,
		LineCache:          m.reviewLineCache,
		CommentIcon:        nerdIconComment,
		CursorMarker:       nerdIconArrowRight + " ",
		SelectionMarker:    "┃ ",
		CommentStartMarker: "╭ ",
		CommentBodyMarker:  "│ ",
		CommentEndMarker:   "╰ ",
		EditorLineRenderer: m.renderEditorLine,
	})
	m.reviewViewport = NewReviewPane(ReviewPaneConfig{Width: width, Height: height, Renderer: renderer})
	m.reviewViewport.SetRows(doc.Rows)
	m.reviewAnchors = doc.Anchors
	m.reviewRows = doc.Rows
	m.reviewExpanderRows = doc.ExpanderRows
	m.selectableRows = selectableRowsFromRows(doc.Rows)
	m.syncReviewVisualState()
}

func (m Model) renderEditorLine(editor presenter.ReviewEditorAnnotation, lineIndex, availableWidth int) string {
	if m.commentEditor == nil {
		return ""
	}
	lines := strings.Split(m.commentEditor.Editor.ViewWithWidth(availableWidth), "\n")
	if lineIndex < 0 || lineIndex >= len(lines) {
		return ""
	}
	return lines[lineIndex]
}

func (m *Model) syncReviewVisualState() {
	m.reviewViewport.SetVisualState(m.reviewVisualState())
}

func (m Model) reviewVisualState() render.ReviewVisualState {
	start, end, selected := m.selectedRange()
	state := render.ReviewVisualState{CursorRow: m.cursorRow, SelectionActive: selected, SelectionStart: start, SelectionEnd: end, CommentMarkers: map[int]render.CommentMarker{}}
	if fileIndex, sectionIndex, ok := m.selectedContextLocation(); ok {
		state.SelectedExpander = &presenter.ReviewExpanderAnchor{FileIndex: fileIndex, SectionIndex: sectionIndex}
	}
	for rowIndex, row := range m.reviewRows {
		if row.Kind != ReviewRowKindLine {
			continue
		}
		if marker, ok := m.commentRangeMarker(rowIndex); ok {
			state.CommentMarkers[rowIndex] = marker
		}
	}
	return state
}

func (m *Model) moveCursor(delta int) {
	m.cursorRow = m.selectableRowFrom(m.cursorRow, delta)
	m.updateAfterCursorMove()
}

func (m *Model) moveCursorToStart() {
	m.cursorRow = m.firstSelectableRow()
	m.updateAfterCursorMoveWithOffset(0)
}

func (m *Model) moveCursorToEnd() {
	m.cursorRow = m.lastSelectableRow()
	m.updateAfterCursorMoveWithOffset(m.cursorRow - m.reviewViewport.Height() + 1)
}

func (m *Model) pageCursor(direction int) {
	if direction == 0 {
		return
	}
	height := max(m.reviewViewport.Height(), 1)
	m.cursorRow = m.selectableRowFrom(m.cursorRow, direction*height)
	m.updateAfterCursorMoveWithOffset(m.reviewViewport.YOffset() + direction*height)
}

func (m *Model) updateAfterCursorMove() {
	m.updateAfterCursorMoveWithOffset(m.reviewViewport.YOffset())
}

func (m *Model) updateAfterCursorMoveWithOffset(preferredOffset int) {
	m.selectNearestContextToCursor()
	m.reviewViewport.SetYOffset(preferredOffset)
	m.keepCursorVisible()
	m.updateActiveFileFromCursor()
	m.syncReviewVisualState()
}

func (m Model) clampCursorRow(row int) int {
	return clampRowWithBounds(row, m.firstSelectableRow(), m.lastSelectableRow())
}

func (m Model) selectableRowFrom(row, delta int) int {
	if len(m.selectableRows) == 0 {
		return 0
	}
	currentIndex := m.nearestSelectableIndex(row)
	if delta == 0 {
		return m.selectableRows[currentIndex]
	}
	nextIndex := min(max(currentIndex+delta, 0), len(m.selectableRows)-1)
	return m.selectableRows[nextIndex]
}

func (m Model) nearestSelectableIndex(row int) int {
	if len(m.selectableRows) == 0 {
		return 0
	}
	for i, selectableRow := range m.selectableRows {
		if selectableRow >= row {
			return i
		}
	}
	return len(m.selectableRows) - 1
}

func clampRowWithBounds(row, first, last int) int {
	if last < first {
		return 0
	}
	return min(max(row, first), last)
}

func (m Model) firstSelectableRow() int {
	if len(m.selectableRows) == 0 {
		return 0
	}
	return m.selectableRows[0]
}

func (m Model) lastSelectableRow() int {
	if len(m.selectableRows) == 0 {
		return 0
	}
	return m.selectableRows[len(m.selectableRows)-1]
}

func selectableRowsFromRows(rows []ReviewRow) []int {
	selectableRows := make([]int, 0)
	for rowIndex, row := range rows {
		if row.Selectable {
			selectableRows = append(selectableRows, rowIndex)
		}
	}
	return selectableRows
}

func (m *Model) centerViewportOnCursor() {
	m.reviewViewport.SetYOffset(m.cursorRow - m.reviewViewport.Height()/2)
}

func (m *Model) keepCursorVisible() {
	height := m.reviewViewport.Height()
	if height <= 0 {
		return
	}
	top := m.reviewViewport.YOffset()
	bottom := top + height - 1
	switch {
	case m.cursorRow < top:
		m.reviewViewport.SetYOffset(m.cursorRow)
	case m.cursorRow > bottom:
		m.reviewViewport.SetYOffset(m.cursorRow - height + 1)
	}
}

func (m *Model) updateActiveFileFromCursor() {
	if len(m.files) == 0 || len(m.reviewRows) == 0 {
		m.activeFilePath = ""
		return
	}

	rowIndex := min(max(m.cursorRow, 0), len(m.reviewRows)-1)
	fileIndex := m.reviewRows[rowIndex].FileIndex
	if fileIndex < 0 || fileIndex >= len(m.files) {
		m.activeFilePath = ""
		return
	}
	m.activeFilePath = m.files[fileIndex].Path
}

func (m Model) commentRangeMarker(rowIndex int) (render.CommentMarker, bool) {
	if rowIndex < 0 || rowIndex >= len(m.reviewRows) {
		return "", false
	}
	if m.rowHasActiveEditorRange(rowIndex) {
		return commentBlockMarkerKind(m.reviewRows[rowIndex].Line, m.commentEditor.Range), true
	}
	if m.reviewDraft == nil {
		return "", false
	}
	row := m.reviewRows[rowIndex]
	if row.Kind != ReviewRowKindLine {
		return "", false
	}
	for _, comment := range m.reviewDraft.Comments() {
		if comment.FilePath == row.FilePath && lineInReviewRange(row.Line, comment.Range) {
			return commentBlockMarkerKind(row.Line, comment.Range), true
		}
	}
	return "", false
}

func (m Model) rowHasCommentRange(rowIndex int) bool {
	if rowIndex < 0 || rowIndex >= len(m.reviewRows) || m.reviewDraft == nil {
		return false
	}
	row := m.reviewRows[rowIndex]
	if row.Kind != ReviewRowKindLine {
		return false
	}
	for _, comment := range m.reviewDraft.Comments() {
		if comment.FilePath == row.FilePath && lineInReviewRange(row.Line, comment.Range) {
			return true
		}
	}
	return false
}

func (m Model) rowHasActiveEditorRange(rowIndex int) bool {
	if rowIndex < 0 || rowIndex >= len(m.reviewRows) || m.commentEditor == nil {
		return false
	}
	row := m.reviewRows[rowIndex]
	return row.Kind == ReviewRowKindLine && row.FilePath == m.commentEditor.FilePath && lineInReviewRange(row.Line, m.commentEditor.Range)
}

func commentBlockMarkerKind(line core.ReviewLine, lineRange core.ReviewLineRange) render.CommentMarker {
	if reviewLineMatchesRef(line, lineRange.Start) {
		return render.CommentMarkerStart
	}
	if reviewLineMatchesRef(line, lineRange.End) {
		return render.CommentMarkerEnd
	}
	return render.CommentMarkerBody
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

func lineInReviewRange(line core.ReviewLine, lineRange core.ReviewLineRange) bool {
	if line.NewLineNumber > 0 && lineRange.Start.NewLineNumber > 0 && lineRange.End.NewLineNumber > 0 {
		start := min(lineRange.Start.NewLineNumber, lineRange.End.NewLineNumber)
		end := max(lineRange.Start.NewLineNumber, lineRange.End.NewLineNumber)
		return line.NewLineNumber >= start && line.NewLineNumber <= end
	}
	if line.OldLineNumber > 0 && lineRange.Start.OldLineNumber > 0 && lineRange.End.OldLineNumber > 0 {
		start := min(lineRange.Start.OldLineNumber, lineRange.End.OldLineNumber)
		end := max(lineRange.Start.OldLineNumber, lineRange.End.OldLineNumber)
		return line.OldLineNumber >= start && line.OldLineNumber <= end
	}
	return reviewLineMatchesRef(line, lineRange.Start) || reviewLineMatchesRef(line, lineRange.End)
}
