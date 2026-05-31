package tui

import (
	"strings"

	"ero/internal/adapters/in/tui/presenter"
	"ero/internal/adapters/in/tui/render"
	"ero/internal/core"
)

type renderedReviewDocument struct {
	Content string
	Lines   []string
	Rows    []ReviewRow
	Anchors ReviewAnchors
}

func renderReviewForTest(files []core.ReviewFile, width, selectedFile, selectedContext int, annotations presenter.ReviewAnnotations) renderedReviewDocument {
	doc := presenter.BuildReviewDocument(presenter.ReviewDocumentInput{Files: files, Annotations: annotations})
	selectedExpander := selectedExpanderAnchorForTest(files, selectedFile, selectedContext)
	renderer := render.NewReviewRowRenderer(render.ReviewRowRendererConfig{
		Width:            max(width-2, 1),
		EnterKeyLabel:    enterKeyLabel(),
		LineNumberWidths: doc.LineNumberWidths,
		CommentIcon:      nerdIconComment,
		LineCache:        render.NewReviewLineCache(),
		EditorLineRenderer: func(_ presenter.ReviewEditorAnnotation, lineIndex, availableWidth int) string {
			lines := strings.Split(NewCommentEditor(width).ViewWithWidth(availableWidth), "\n")
			if lineIndex < 0 || lineIndex >= len(lines) {
				return ""
			}
			return lines[lineIndex]
		},
	})
	state := render.ReviewVisualState{CursorRow: -1, CommentMarkers: map[int]render.CommentMarker{}, SelectedExpander: selectedExpander}
	lines := make([]string, len(doc.Rows))
	for rowIndex, row := range doc.Rows {
		lines[rowIndex] = renderer.Render(row, rowIndex, state)
	}
	return renderedReviewDocument{Content: strings.Join(lines, "\n"), Lines: lines, Rows: doc.Rows, Anchors: doc.Anchors}
}

func selectedExpanderAnchorForTest(files []core.ReviewFile, selectedFile, selectedContext int) *presenter.ReviewExpanderAnchor {
	if selectedFile < 0 || selectedFile >= len(files) || selectedContext < 0 {
		return nil
	}
	ordinal := 0
	for sectionIndex, section := range files[selectedFile].Sections {
		if section.Kind != core.SectionKindContext || section.HiddenLineCount() == 0 {
			continue
		}
		if ordinal == selectedContext {
			return &presenter.ReviewExpanderAnchor{FileIndex: selectedFile, SectionIndex: sectionIndex}
		}
		ordinal++
	}
	return nil
}
