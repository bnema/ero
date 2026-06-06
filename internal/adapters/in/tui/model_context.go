package tui

import "ero/internal/core"

func (m *Model) showMoreContext(count int) {
	fileIndex, sectionIndex, ok := m.contextSectionLocationForExpansion()
	if !ok || !m.contextBarActionAllowed(fileIndex, sectionIndex, ContextBarActionShowMore) {
		return
	}
	m.expandContextSectionNearCursor(fileIndex, sectionIndex, count)
	m.clearSelection()
	m.normalizeContextSelection()
	m.syncReviewViewport()
}

func (m *Model) showAllContext() {
	fileIndex, sectionIndex, ok := m.contextSectionLocationForExpansion()
	if !ok || !m.contextBarActionAllowed(fileIndex, sectionIndex, ContextBarActionShowAll) {
		return
	}
	m.files[fileIndex].Sections[sectionIndex].ExpandAll()
	m.clearSelection()
	m.normalizeContextSelection()
	m.syncReviewViewport()
}

func (m *Model) expandContextSectionNearCursor(fileIndex, sectionIndex, count int) {
	section := &m.files[fileIndex].Sections[sectionIndex]
	switch m.resolveShowMoreDirection(fileIndex, sectionIndex) {
	case contextExpansionAbove:
		section.ExpandAbove(count)
	case contextExpansionBelow:
		section.ExpandBelow(count)
	}
}

func (m *Model) resetContextSelection() {
	if len(m.files) == 0 {
		m.selectedFile = 0
		m.selectedContext = -1
		return
	}

	m.selectedFile = min(max(m.selectedFile, 0), len(m.files)-1)
	if len(m.selectableContextSectionIndexes()) == 0 {
		m.selectedContext = -1
		return
	}
	m.selectedContext = 0
}

func (m *Model) normalizeContextSelection() {
	indexes := m.selectableContextSectionIndexes()
	if len(indexes) == 0 {
		m.selectedContext = -1
		return
	}
	if m.selectedContext < 0 {
		m.selectedContext = 0
		return
	}
	m.selectedContext = min(m.selectedContext, len(indexes)-1)
}

func (m Model) currentFile() *core.ReviewFile {
	if len(m.files) == 0 || m.selectedFile < 0 || m.selectedFile >= len(m.files) {
		return nil
	}
	return &m.files[m.selectedFile]
}

func (m Model) selectableContextSectionIndexes() []int {
	file := m.currentFile()
	if file == nil {
		return nil
	}

	var indexes []int
	for i, section := range file.Sections {
		if section.Kind == core.SectionKindContext && section.HiddenLineCount() > 0 {
			indexes = append(indexes, i)
		}
	}
	return indexes
}

func (m *Model) selectedContextSection() *core.ReviewSection {
	indexes := m.selectableContextSectionIndexes()
	if len(indexes) == 0 || m.selectedContext < 0 || m.selectedContext >= len(indexes) {
		return nil
	}
	return &m.files[m.selectedFile].Sections[indexes[m.selectedContext]]
}

func (m *Model) contextSectionLocationForExpansion() (int, int, bool) {
	if m.cursorRow >= 0 && m.cursorRow < len(m.reviewRows) {
		row := m.reviewRows[m.cursorRow]
		if row.Kind == ReviewRowKindExpander && m.selectContextSection(row.FileIndex, row.SectionIndex) {
			return row.FileIndex, row.SectionIndex, true
		}
	}
	return m.selectedContextLocation()
}

func (m *Model) selectedContextLocation() (int, int, bool) {
	indexes := m.selectableContextSectionIndexes()
	if len(indexes) == 0 || m.selectedContext < 0 || m.selectedContext >= len(indexes) {
		return 0, 0, false
	}
	return m.selectedFile, indexes[m.selectedContext], true
}

type contextExpansionDirection int

const (
	contextExpansionAbove contextExpansionDirection = iota
	contextExpansionBelow
)

func (m Model) contextBarActionAllowed(fileIndex, sectionIndex int, action ContextBarAction) bool {
	if fileIndex < 0 || fileIndex >= len(m.files) || sectionIndex < 0 || sectionIndex >= len(m.files[fileIndex].Sections) {
		return false
	}

	model := NewContextBarViewModel(m.files[fileIndex], sectionIndex)
	if model.HiddenLines == 0 {
		return false
	}

	switch action {
	case ContextBarActionShowMore, ContextBarActionShowAll:
		return true
	default:
		return false
	}
}

func (m Model) resolveShowMoreDirection(fileIndex, sectionIndex int) contextExpansionDirection {
	position := NewContextBarViewModel(m.files[fileIndex], sectionIndex).Position
	switch position {
	case ContextBarAtFileStart:
		return contextExpansionBelow
	case ContextBarAtFileEnd:
		return contextExpansionAbove
	}

	expanderRow := -1
	for rowIndex, row := range m.reviewRows {
		if row.FileIndex == fileIndex && row.SectionIndex == sectionIndex && row.Kind == ReviewRowKindExpander {
			expanderRow = rowIndex
			break
		}
	}
	if expanderRow >= 0 && m.cursorRow > expanderRow {
		return contextExpansionBelow
	}
	return contextExpansionAbove
}

func (m *Model) selectContextSection(fileIndex, sectionIndex int) bool {
	if fileIndex < 0 || fileIndex >= len(m.files) || sectionIndex < 0 || sectionIndex >= len(m.files[fileIndex].Sections) {
		return false
	}
	section := m.files[fileIndex].Sections[sectionIndex]
	if section.Kind != core.SectionKindContext || section.HiddenLineCount() == 0 {
		return false
	}

	ordinal := 0
	for i, candidate := range m.files[fileIndex].Sections {
		if candidate.Kind != core.SectionKindContext || candidate.HiddenLineCount() == 0 {
			continue
		}
		if i == sectionIndex {
			m.selectedFile = fileIndex
			m.selectedContext = ordinal
			return true
		}
		ordinal++
	}
	return false
}

func (m *Model) selectNearestContextToCursor() {
	if m.cursorRow < 0 || m.cursorRow >= len(m.reviewRows) {
		return
	}

	fileIndex := m.reviewRows[m.cursorRow].FileIndex
	if fileIndex < 0 || fileIndex >= len(m.files) {
		return
	}

	nearestRow := -1
	nearestSection := -1
	nearestDistance := 0
	for rowIndex, row := range m.reviewRows {
		if row.FileIndex != fileIndex || row.Kind != ReviewRowKindExpander {
			continue
		}
		distance := rowIndex - m.cursorRow
		if distance < 0 {
			distance = -distance
		}
		if nearestRow == -1 || distance < nearestDistance {
			nearestRow = rowIndex
			nearestSection = row.SectionIndex
			nearestDistance = distance
		}
	}
	if nearestRow == -1 {
		m.selectedFile = fileIndex
		m.selectedContext = -1
		return
	}
	m.selectContextSection(fileIndex, nearestSection)
}
