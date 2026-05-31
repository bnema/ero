package presenter

import "ero/internal/core"

type ReviewRowKind string

const (
	ReviewRowKindBlank        ReviewRowKind = "blank"
	ReviewRowKindFile         ReviewRowKind = "file"
	ReviewRowKindRule         ReviewRowKind = "rule"
	ReviewRowKindLine         ReviewRowKind = "line"
	ReviewRowKindExpander     ReviewRowKind = "expander"
	ReviewRowKindMessage      ReviewRowKind = "message"
	ReviewRowKindComment      ReviewRowKind = "comment"
	ReviewRowKindRemoteThread ReviewRowKind = "remote_thread"
	ReviewRowKindEditor       ReviewRowKind = "editor"
)

type ReviewContextPosition string

const (
	ReviewContextBetweenChanges ReviewContextPosition = "between_changes"
	ReviewContextAtFileStart    ReviewContextPosition = "file_start"
	ReviewContextAtFileEnd      ReviewContextPosition = "file_end"
	ReviewContextOnlySection    ReviewContextPosition = "only_section"
)

type ReviewFileStats struct {
	Added   int
	Deleted int
}

type ReviewExpander struct {
	HiddenLines int
	Position    ReviewContextPosition
}

type ReviewAnnotation struct {
	Comment      core.ReviewComment
	RemoteThread core.RemoteReviewThread
	Editor       ReviewEditorAnnotation
	LineIndex    int
	Author       string
	Body         string
}

type ReviewEditorAnnotation struct {
	FilePath  string
	Range     core.ReviewLineRange
	LineCount int
}

type ReviewRow struct {
	Kind         ReviewRowKind
	FileIndex    int
	SectionIndex int
	LineIndex    int
	FilePath     string
	Line         core.ReviewLine
	FileStats    ReviewFileStats
	Expander     ReviewExpander
	Annotation   ReviewAnnotation
	Message      string
	Text         string
	Selectable   bool
}

type ReviewAnchors struct {
	FileRows map[int]int
	LineRows map[ReviewLineAnchor]int
}

type ReviewLineAnchor struct {
	FileIndex    int
	SectionIndex int
	LineIndex    int
}

type ReviewExpanderAnchor struct {
	FileIndex    int
	SectionIndex int
}
