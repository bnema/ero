package tui

import (
	"ero/internal/adapters/in/tui/component"
	"ero/internal/adapters/in/tui/presenter"
)

type ReviewRowKind = presenter.ReviewRowKind

const (
	ReviewRowKindBlank        = presenter.ReviewRowKindBlank
	ReviewRowKindFile         = presenter.ReviewRowKindFile
	ReviewRowKindRule         = presenter.ReviewRowKindRule
	ReviewRowKindLine         = presenter.ReviewRowKindLine
	ReviewRowKindExpander     = presenter.ReviewRowKindExpander
	ReviewRowKindMessage      = presenter.ReviewRowKindMessage
	ReviewRowKindComment      = presenter.ReviewRowKindComment
	ReviewRowKindRemoteThread = presenter.ReviewRowKindRemoteThread
	ReviewRowKindEditor       = presenter.ReviewRowKindEditor
)

type ReviewRow = presenter.ReviewRow
type ReviewAnchors = presenter.ReviewAnchors
type ReviewLineAnchor = presenter.ReviewLineAnchor

type StatusModel = component.StatusModel
type StatusBar = component.StatusBar
type KeyHint = component.KeyHint

func NewStatusBar(width int) StatusBar {
	return component.NewStatusBar(width)
}

func renderKeyHints(hints []KeyHint) string {
	return component.RenderKeyHints(hints)
}
