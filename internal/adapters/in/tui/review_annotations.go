package tui

import (
	"strings"

	"ero/internal/adapters/in/tui/presenter"
	"ero/internal/core"
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
