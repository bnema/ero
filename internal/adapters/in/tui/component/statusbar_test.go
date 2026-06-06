package component

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"ero/internal/core"
)

func TestStatusbarProviderSync(t *testing.T) {
	baseTime := time.Date(2026, 6, 5, 12, 34, 0, 0, time.UTC)
	nextTime := baseTime.Add(5 * time.Minute)

	tests := []struct {
		name    string
		model   StatusModel
		want    []string
		wantNot []string
	}{
		{
			name:    "no provider",
			model:   noProviderStatusModel(),
			want:    []string{"ero", "branch", "1 file", "no provider"},
			wantNot: []string{"GitHub", "synced", "syncing", "cache", "failed", "backoff"},
		},
		{
			name:  "cache",
			model: syncStatusModel("GitHub", "gh-runtime", core.ProviderSyncState{Status: core.ProviderSyncStatusLoadingCache}),
			want:  []string{"GitHub/gh-runtime", "cache"},
		},
		{
			name:  "syncing",
			model: syncStatusModel("GitHub", "gh-runtime", core.ProviderSyncState{Status: core.ProviderSyncStatusSyncing}),
			want:  []string{"GitHub/gh-runtime", "syncing"},
		},
		{
			name:  "synced",
			model: syncStatusModel("GitHub", "gh-runtime", core.ProviderSyncState{Status: core.ProviderSyncStatusSynced, LastSyncAt: &baseTime, NextSyncAt: &nextTime}),
			want:  []string{"GitHub/gh-runtime", "synced", "last 12:34", "next 12:39"},
		},
		{
			name:  "failed",
			model: syncStatusModel("GitHub", "gh-runtime", core.ProviderSyncState{Status: core.ProviderSyncStatusFailed, LastSyncAt: &baseTime, LastError: "boom"}),
			want:  []string{"GitHub/gh-runtime", "failed", "boom", "last 12:34"},
		},
		{
			name:  "backing-off",
			model: syncStatusModel("GitHub", "gh-runtime", core.ProviderSyncState{Status: core.ProviderSyncStatusBackingOff, LastSyncAt: &baseTime, NextSyncAt: &nextTime}),
			want:  []string{"GitHub/gh-runtime", "backoff", "last 12:34", "next 12:39"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := stripANSIForStatusbarTest(NewStatusBar(120).Render(tt.model))
			for _, want := range tt.want {
				require.Contains(t, view, want)
			}
			for _, wantNot := range tt.wantNot {
				require.NotContains(t, view, wantNot)
			}
		})
	}
}

func TestStatusbarProviderSyncUsesNerdFontProviderGlyphAndStatusDot(t *testing.T) {
	for _, status := range []core.ProviderSyncStatus{
		core.ProviderSyncStatusLoadingCache,
		core.ProviderSyncStatusSyncing,
		core.ProviderSyncStatusSynced,
		core.ProviderSyncStatusFailed,
		core.ProviderSyncStatusBackingOff,
	} {
		model := syncStatusModel("GitHub", "gh-runtime", core.ProviderSyncState{Status: status})
		model.NerdFont = true

		view := stripANSIForStatusbarTest(NewStatusBar(120).Render(model))

		require.Contains(t, view, "●")
	}
}

func TestStatusbarProviderSyncOmitsStatusWordWhenNerdFontSymbolIsShown(t *testing.T) {
	model := syncStatusModel("GitHub", "github", core.ProviderSyncState{Status: core.ProviderSyncStatusSynced})
	model.NerdFont = true

	view := stripANSIForStatusbarTest(NewStatusBar(120).Render(model))

	require.Contains(t, view, "●")
	require.NotContains(t, view, "")
	require.NotContains(t, view, "synced")
}

func TestStatusbarShowsCompactActiveProviderAndAdditionalCount(t *testing.T) {
	model := syncStatusModel("GitHub", "github", core.ProviderSyncState{Status: core.ProviderSyncStatusSynced})
	model.ProviderCount = 2
	model.ProviderSwitch = true
	model.NerdFont = true

	view := stripANSIForStatusbarTest(NewStatusBar(120).Render(model))

	require.Contains(t, view, "● +1")
	require.NotContains(t, view, "2 providers")
	require.Contains(t, view, "p provider")
	require.Contains(t, view, "P publish")
}

func TestStatusbarShowsDraftCommentCount(t *testing.T) {
	model := baseStatusModel()
	model.DraftCommentCount = 2

	view := stripANSIForStatusbarTest(NewStatusBar(120).Render(model))

	require.Contains(t, view, "2 draft comments")
}

func TestStatusbarProviderSyncNarrowWidthDegradesGracefully(t *testing.T) {
	last := time.Date(2026, 6, 5, 12, 34, 0, 0, time.UTC)
	next := last.Add(5 * time.Minute)
	model := syncStatusModel("VeryLongProvider", "very-long-runtime-name", core.ProviderSyncState{Status: core.ProviderSyncStatusBackingOff, LastSyncAt: &last, NextSyncAt: &next})

	view := stripANSIForStatusbarTest(NewStatusBar(32).Render(model))

	require.NotContains(t, view, "\n")
	require.LessOrEqual(t, len([]rune(view)), 32)
	require.True(t, strings.Contains(view, "Very") || strings.Contains(view, "back") || strings.Contains(view, "? help"))
}

func baseStatusModel() StatusModel {
	return StatusModel{AppName: "ero", Mode: "branch", FileCount: 1, CurrentFile: "demo.go"}
}

func noProviderStatusModel() StatusModel {
	model := baseStatusModel()
	model.ShowNoProvider = true
	return model
}

func syncStatusModel(label, runtime string, sync core.ProviderSyncState) StatusModel {
	model := baseStatusModel()
	model.ProviderCount = 1
	model.ActiveProviderLabel = label
	model.ActiveRuntimeName = runtime
	model.ProviderSync = sync
	return model
}

var statusbarANSIPattern = regexp.MustCompile(`\x1b\[[0-9;:]*[A-Za-z]`)

func stripANSIForStatusbarTest(s string) string {
	return statusbarANSIPattern.ReplaceAllString(s, "")
}
