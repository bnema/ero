package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ero/internal/core"
)

func TestPRSheetOverlaysRightSideFullHeightWithLeftSeparatorOnly(t *testing.T) {
	t.Parallel()

	model := NewModel([]core.ReviewFile{reviewFile("demo.go", "package demo")})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 60, Height: 8})
	model = updated.(Model).TogglePRSheet()

	view := stripANSI(model.View().Content)
	lines := strings.Split(view, "\n")
	require.Len(t, lines, 8)

	sheetWidth := prSheetWidth(model.width)
	separatorColumn := model.width - sheetWidth
	separatorRows := 0
	for _, line := range lines {
		runes := []rune(line)
		if len(runes) <= separatorColumn {
			continue
		}
		if runes[separatorColumn] == '│' {
			separatorRows++
		}
		if len(runes) > separatorColumn+1 {
			assert.NotEqual(t, '│', runes[len(runes)-1], "right edge should not be bordered")
		}
	}
	assert.GreaterOrEqual(t, separatorRows, 6)
	assert.Contains(t, view, "Pull request")
	assert.NotContains(t, view, "┌")
	assert.NotContains(t, view, "┐")
	assert.NotContains(t, view, "└")
	assert.NotContains(t, view, "┘")
}

func TestPRSheetOverlayDoesNotReflowUnderlyingDiffContent(t *testing.T) {
	t.Parallel()

	model := NewModel([]core.ReviewFile{reviewFile("demo.go", "package demo")})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	model = updated.(Model)
	closedReviewWidth := model.reviewViewport.Width()
	closedFirstLine := strings.Split(stripANSI(model.View().Content), "\n")[0]

	model = model.TogglePRSheet()
	openReviewWidth := model.reviewViewport.Width()
	openFirstLine := strings.Split(stripANSI(model.View().Content), "\n")[0]

	assert.Equal(t, closedReviewWidth, openReviewWidth)
	sheetStart := model.width - prSheetWidth(model.width)
	assert.Equal(t, string([]rune(closedFirstLine)[:sheetStart]), string([]rune(openFirstLine)[:sheetStart]))
}

func TestPRSheetCanToggleByMethodAndMessage(t *testing.T) {
	t.Parallel()

	model := NewModel(nil)
	assert.False(t, model.prSheet.open)
	model = model.TogglePRSheet()
	assert.True(t, model.prSheet.open)

	updated, _ := model.Update(prSheetToggledMsg{})
	model = updated.(Model)
	assert.False(t, model.prSheet.open)
}

func TestPRSheetKeyboardAndWheelScrollSheetNotReview(t *testing.T) {
	model := NewModel([]core.ReviewFile{reviewFileWithLines("demo.go", 30)})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 4})
	model = updated.(Model).TogglePRSheet()
	reviewOffset := model.reviewViewport.YOffset()

	updated, _ = model.Update(keyPress("j"))
	model = updated.(Model)
	updated, _ = model.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown}))
	model = updated.(Model)

	assert.Equal(t, 2, model.prSheet.yOffset)
	assert.Equal(t, reviewOffset, model.reviewViewport.YOffset())
}

func TestPRSheetHasIndependentScrollState(t *testing.T) {
	t.Parallel()

	model := NewModel([]core.ReviewFile{reviewFileWithLines("demo.go", 20)})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 4})
	model = updated.(Model).TogglePRSheet()
	model.moveCursor(5)
	reviewOffset := model.reviewViewport.YOffset()

	updated, _ = model.Update(prSheetScrolledMsg{delta: 2})
	model = updated.(Model)

	assert.Equal(t, 2, model.prSheet.yOffset)
	assert.Equal(t, reviewOffset, model.reviewViewport.YOffset())
	assert.Contains(t, stripANSI(model.renderPRSheet(model.width, model.height)), "Provider:")
	assert.NotContains(t, stripANSI(model.renderPRSheet(model.width, model.height)), "Pull request")
}

func TestPRSheetRendersOverviewMarkdownCommentsAndReviews(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)
	model := NewModel(nil)
	model.width = 180
	model.height = 40
	model.activeRuntimeInfo = core.ReviewProviderInfo{ID: "github", Label: "GitHub"}
	model.providerOverview = &core.ProviderOverview{
		Title:       "Add provider snapshots",
		Number:      42,
		State:       "OPEN",
		ExternalURL: "https://example.invalid/pull/1",
		Author:      "author",
		Body:        "This **adds** snapshots.\n\n```go\nfmt.Println(\"hi\")\n```",
		BaseRef:     "main",
		HeadRef:     "feature",
		UpdatedAt:   &createdAt,
		Comments: []core.ProviderIssueComment{{
			Author:    "reviewer",
			Body:      "Please update the **docs**.",
			CreatedAt: createdAt,
		}},
		Reviews: []core.ProviderReviewSummary{{
			Author:      "maintainer",
			State:       "approved",
			Body:        "Looks **good** to me.",
			SubmittedAt: createdAt,
		}},
	}

	plain := stripANSI(model.renderPRSheet(model.width, model.height))

	assert.Contains(t, plain, "Add provider snapshots")
	assert.Contains(t, plain, "#42 · OPEN · by author · main ← feature")
	assert.Contains(t, plain, "Updated "+createdAt.Local().Format("2006-01-02 15:04"))
	assert.Contains(t, plain, "https://example.invalid/pull/1")
	assert.Contains(t, plain, "This adds snapshots.")
	assert.Contains(t, plain, "fmt.Println")
	assert.Contains(t, plain, "Issue comments: 1")
	assert.Contains(t, plain, "Comment by reviewer")
	assert.Contains(t, plain, "Please update the docs.")
	assert.Contains(t, plain, "Review summaries: 1")
	assert.Contains(t, plain, "✓ APPROVED by maintainer")
	assert.Contains(t, plain, "Looks good to me.")
}

func TestPRSheetRendersNilOverviewFallback(t *testing.T) {
	t.Parallel()

	model := NewModel(nil)
	model.width = 100
	model.height = 12
	plain := stripANSI(model.renderPRSheet(model.width, model.height))

	assert.Contains(t, plain, "No provider overview loaded.")
	assert.Contains(t, plain, "Remote threads: 0 threads")
}
