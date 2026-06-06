package presenter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ero/internal/core"
)

func TestBuildReviewDocumentProjectsRowsAnchorsAndExpandersWithoutRenderedText(t *testing.T) {
	t.Parallel()

	doc := BuildReviewDocument(ReviewDocumentInput{Files: []core.ReviewFile{
		{
			Path: "a.go",
			Sections: []core.ReviewSection{
				{ID: "changed", Kind: core.SectionKindChanged, Lines: []core.ReviewLine{{NewLineNumber: 1, Content: "added", Kind: core.LineKindAdded}}},
				{ID: "context", Kind: core.SectionKindContext, ExpandedBelow: 1, Lines: []core.ReviewLine{
					{OldLineNumber: 2, NewLineNumber: 2, Content: "hidden", Kind: core.LineKindUnchanged},
					{OldLineNumber: 3, NewLineNumber: 3, Content: "visible", Kind: core.LineKindUnchanged},
				}},
			},
		},
		{
			Path:     "b.go",
			Sections: []core.ReviewSection{{ID: "changed", Kind: core.SectionKindChanged, Lines: []core.ReviewLine{{OldLineNumber: 12, Content: "deleted", Kind: core.LineKindDeleted}}}},
		},
	}})

	require.Len(t, doc.Rows, 9)
	assert.Equal(t, []ReviewRowKind{
		ReviewRowKindFile,
		ReviewRowKindRule,
		ReviewRowKindLine,
		ReviewRowKindExpander,
		ReviewRowKindLine,
		ReviewRowKindBlank,
		ReviewRowKindFile,
		ReviewRowKindRule,
		ReviewRowKindLine,
	}, rowKinds(doc.Rows))
	assert.Equal(t, 0, doc.Anchors.FileRows[0])
	assert.Equal(t, 6, doc.Anchors.FileRows[1])
	assert.Equal(t, 2, doc.Anchors.LineRows[ReviewLineAnchor{FileIndex: 0, SectionIndex: 0, LineIndex: 0}])
	assert.Equal(t, 4, doc.Anchors.LineRows[ReviewLineAnchor{FileIndex: 0, SectionIndex: 1, LineIndex: 1}])
	_, hiddenAnchored := doc.Anchors.LineRows[ReviewLineAnchor{FileIndex: 0, SectionIndex: 1, LineIndex: 0}]
	assert.False(t, hiddenAnchored)
	expanderRow, ok := doc.ExpanderRows[ReviewExpanderAnchor{FileIndex: 0, SectionIndex: 1}]
	require.True(t, ok)
	assert.Equal(t, 3, expanderRow)
	assert.Equal(t, 1, doc.Rows[expanderRow].Expander.HiddenLines)
	assert.Equal(t, ReviewContextAtFileEnd, doc.Rows[expanderRow].Expander.Position)
	assert.Equal(t, 4, doc.LineNumberWidths[0])
	assert.Equal(t, 4, doc.LineNumberWidths[1])

	for _, row := range doc.Rows {
		assert.Empty(t, row.Text, "presenter projection must not pre-render Lip Gloss strings")
	}
}

func TestBuildReviewDocumentProjectsAnnotationRowsAndRebuildsAnchors(t *testing.T) {
	t.Parallel()

	doc := BuildReviewDocument(ReviewDocumentInput{
		Files: []core.ReviewFile{{
			Path: "demo.go",
			Sections: []core.ReviewSection{{ID: "changed", Kind: core.SectionKindChanged, Lines: []core.ReviewLine{
				{NewLineNumber: 1, Content: "one", Kind: core.LineKindAdded},
				{NewLineNumber: 2, Content: "two", Kind: core.LineKindAdded},
				{NewLineNumber: 3, Content: "three", Kind: core.LineKindAdded},
			}}},
		}},
		Annotations: ReviewAnnotations{
			Comments: []core.ReviewComment{{
				ID:       "comment-1",
				FilePath: "demo.go",
				Range: core.ReviewLineRange{
					Start: core.ReviewLineRef{NewLineNumber: 1, Kind: core.LineKindAdded},
					End:   core.ReviewLineRef{NewLineNumber: 1, Kind: core.LineKindAdded},
				},
				Body: "local note",
			}},
			RemoteThreads: []core.RemoteReviewThread{
				{ProviderID: "github", Unmapped: true, Comments: []core.RemoteReviewComment{{Body: "orphaned"}}},
				{ProviderID: "github", FilePath: "demo.go", Range: core.ReviewLineRange{Start: core.ReviewLineRef{NewLineNumber: 2, Kind: core.LineKindAdded}, End: core.ReviewLineRef{NewLineNumber: 2, Kind: core.LineKindAdded}}, Comments: []core.RemoteReviewComment{{Author: "octocat", Body: "remote note"}}},
			},
			Editor: &ReviewEditorAnnotation{FilePath: "demo.go", Range: core.ReviewLineRange{Start: core.ReviewLineRef{NewLineNumber: 3, Kind: core.LineKindAdded}, End: core.ReviewLineRef{NewLineNumber: 3, Kind: core.LineKindAdded}}, LineCount: 1},
		},
	})

	assert.Equal(t, []ReviewRowKind{
		ReviewRowKindFile,
		ReviewRowKindRule,
		ReviewRowKindLine,
		ReviewRowKindComment,
		ReviewRowKindComment,
		ReviewRowKindLine,
		ReviewRowKindRemoteThread,
		ReviewRowKindRemoteThread,
		ReviewRowKindLine,
		ReviewRowKindEditor,
		ReviewRowKindEditor,
	}, rowKinds(doc.Rows))
	assert.Equal(t, 0, doc.Anchors.FileRows[0])
	assert.Equal(t, 2, doc.Anchors.LineRows[ReviewLineAnchor{FileIndex: 0, SectionIndex: 0, LineIndex: 0}])
	assert.Equal(t, 5, doc.Anchors.LineRows[ReviewLineAnchor{FileIndex: 0, SectionIndex: 0, LineIndex: 1}])
	assert.Equal(t, 8, doc.Anchors.LineRows[ReviewLineAnchor{FileIndex: 0, SectionIndex: 0, LineIndex: 2}])
	assert.Equal(t, "comment-1", doc.Rows[3].Annotation.Comment.ID)
	assert.Equal(t, 0, doc.Rows[3].Annotation.LineIndex)
	assert.Equal(t, "local note", doc.Rows[4].Annotation.Body)
	assert.Equal(t, "github", doc.Rows[6].Annotation.RemoteThread.ProviderID)
	assert.Equal(t, "octocat", doc.Rows[7].Annotation.Author)
	assert.Equal(t, "remote note", doc.Rows[7].Annotation.Body)
	assert.Equal(t, "demo.go", doc.Rows[9].Annotation.Editor.FilePath)
	assert.Equal(t, 0, doc.Rows[9].Annotation.LineIndex)
	assert.Equal(t, 1, doc.Rows[10].Annotation.LineIndex)
	assert.False(t, doc.Rows[3].Selectable)
}

func TestBuildReviewDocumentDoesNotProjectUnmappedOrFilelessRemoteThreads(t *testing.T) {
	t.Parallel()

	doc := BuildReviewDocument(ReviewDocumentInput{
		Files: []core.ReviewFile{{
			Path:     "demo.go",
			Sections: []core.ReviewSection{{Kind: core.SectionKindChanged, Lines: []core.ReviewLine{{NewLineNumber: 1, Content: "one", Kind: core.LineKindAdded}}}},
		}},
		Annotations: ReviewAnnotations{RemoteThreads: []core.RemoteReviewThread{
			{ProviderID: "github", ExternalID: "unmapped", Unmapped: true, Comments: []core.RemoteReviewComment{{Body: "orphaned"}}},
			{ProviderID: "github", ExternalID: "fileless", Comments: []core.RemoteReviewComment{{Body: "no file"}}},
			{ProviderID: "github", ExternalID: "mapped", FilePath: "demo.go", Range: core.ReviewLineRange{Start: core.ReviewLineRef{NewLineNumber: 1, Kind: core.LineKindAdded}, End: core.ReviewLineRef{NewLineNumber: 1, Kind: core.LineKindAdded}}, Comments: []core.RemoteReviewComment{{Author: "octocat", Body: "mapped note"}}},
		}},
	})

	require.Equal(t, ReviewRowKindFile, doc.Rows[0].Kind)
	var remoteIDs []string
	for _, row := range doc.Rows {
		if row.Kind == ReviewRowKindRemoteThread {
			remoteIDs = append(remoteIDs, row.Annotation.RemoteThread.ExternalID)
		}
	}
	require.Equal(t, []string{"mapped", "mapped"}, remoteIDs)
}

func TestBuildReviewDocumentAnnotationRowsMatchRenderedLineCountsBeforeLaterAnchors(t *testing.T) {
	t.Parallel()

	doc := BuildReviewDocument(ReviewDocumentInput{
		Files: []core.ReviewFile{{
			Path: "demo.go",
			Sections: []core.ReviewSection{{ID: "changed", Kind: core.SectionKindChanged, Lines: []core.ReviewLine{
				{NewLineNumber: 1, Content: "one", Kind: core.LineKindAdded},
				{NewLineNumber: 2, Content: "two", Kind: core.LineKindAdded},
				{NewLineNumber: 3, Content: "three", Kind: core.LineKindAdded},
				{NewLineNumber: 4, Content: "four", Kind: core.LineKindAdded},
			}}},
		}},
		Annotations: ReviewAnnotations{
			Comments:      []core.ReviewComment{{ID: "comment-1", FilePath: "demo.go", Range: core.ReviewLineRange{Start: core.ReviewLineRef{NewLineNumber: 1, Kind: core.LineKindAdded}, End: core.ReviewLineRef{NewLineNumber: 1, Kind: core.LineKindAdded}}, Body: "first body\nsecond body"}},
			RemoteThreads: []core.RemoteReviewThread{{ProviderID: "github", FilePath: "demo.go", Range: core.ReviewLineRange{Start: core.ReviewLineRef{NewLineNumber: 2, Kind: core.LineKindAdded}, End: core.ReviewLineRef{NewLineNumber: 2, Kind: core.LineKindAdded}}, Comments: []core.RemoteReviewComment{{Author: "octocat", Body: "remote one\nremote two"}}}},
			Editor:        &ReviewEditorAnnotation{FilePath: "demo.go", Range: core.ReviewLineRange{Start: core.ReviewLineRef{NewLineNumber: 3, Kind: core.LineKindAdded}, End: core.ReviewLineRef{NewLineNumber: 3, Kind: core.LineKindAdded}}, LineCount: 2},
		},
	})

	assert.Equal(t, 2, doc.Anchors.LineRows[ReviewLineAnchor{FileIndex: 0, SectionIndex: 0, LineIndex: 0}])
	assert.Equal(t, 6, doc.Anchors.LineRows[ReviewLineAnchor{FileIndex: 0, SectionIndex: 0, LineIndex: 1}])
	assert.Equal(t, 10, doc.Anchors.LineRows[ReviewLineAnchor{FileIndex: 0, SectionIndex: 0, LineIndex: 2}])
	assert.Equal(t, 14, doc.Anchors.LineRows[ReviewLineAnchor{FileIndex: 0, SectionIndex: 0, LineIndex: 3}])
	assert.Equal(t, "first body", doc.Rows[4].Annotation.Body)
	assert.Equal(t, "second body", doc.Rows[5].Annotation.Body)
	assert.Equal(t, "remote one", doc.Rows[8].Annotation.Body)
	assert.Equal(t, "remote two", doc.Rows[9].Annotation.Body)
	assert.Equal(t, 2, doc.Rows[13].Annotation.LineIndex)
}

func TestBuildReviewDocumentProjectsEmptyReviewMessage(t *testing.T) {
	t.Parallel()

	doc := BuildReviewDocument(ReviewDocumentInput{})

	require.Len(t, doc.Rows, 2)
	assert.Equal(t, ReviewRowKindMessage, doc.Rows[0].Kind)
	assert.Equal(t, "Review", doc.Rows[0].Message)
	assert.Equal(t, "No files to review", doc.Rows[1].Message)
	assert.Empty(t, doc.Anchors.FileRows)
	assert.Empty(t, doc.Anchors.LineRows)
}

func rowKinds(rows []ReviewRow) []ReviewRowKind {
	kinds := make([]ReviewRowKind, len(rows))
	for i, row := range rows {
		kinds[i] = row.Kind
	}
	return kinds
}
