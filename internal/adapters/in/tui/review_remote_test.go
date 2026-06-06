package tui

import (
	"testing"

	"github.com/stretchr/testify/require"

	"ero/internal/adapters/in/tui/presenter"
	"ero/internal/core"
)

func TestRemoteReviewAnnotations(t *testing.T) {
	rendered := renderReviewForTest([]core.ReviewFile{reviewFile("demo.go", "package main")}, 80, -1, -1, presenter.ReviewAnnotations{
		RemoteThreads: []core.RemoteReviewThread{{
			ProviderID: "github",
			FilePath:   "demo.go",
			Range:      core.ReviewLineRange{Start: core.ReviewLineRef{NewLineNumber: 1, Kind: core.LineKindAdded}, End: core.ReviewLineRef{NewLineNumber: 1, Kind: core.LineKindAdded}},
			Comments:   []core.RemoteReviewComment{{Author: "octocat", Body: "remote note"}},
		}},
	})
	view := stripANSI(rendered.Content)
	require.Contains(t, view, "[github]")
	require.Contains(t, view, "remote read-only")
	require.Contains(t, view, "octocat: remote note")
}

func TestRemoteReviewAnnotationsUnmappedAreHiddenFromDiff(t *testing.T) {
	rendered := renderReviewForTest([]core.ReviewFile{reviewFile("demo.go", "package main")}, 80, -1, -1, presenter.ReviewAnnotations{
		RemoteThreads: []core.RemoteReviewThread{{ProviderID: "github", Unmapped: true, Comments: []core.RemoteReviewComment{{Body: "orphaned"}}}},
	})
	view := stripANSI(rendered.Content)
	require.NotContains(t, view, "[github] unmapped")
	require.NotContains(t, view, "orphaned")
	require.Contains(t, view, "demo.go")
}
