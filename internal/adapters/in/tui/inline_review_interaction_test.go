package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"ero/internal/core"
	"ero/internal/ports/mocks"
)

type deadlineRecordingClipboardWriter struct {
	hadDeadline bool
}

func (w *deadlineRecordingClipboardWriter) WriteClipboard(ctx context.Context, _ string) error {
	_, w.hadDeadline = ctx.Deadline()
	return nil
}

func TestModelOpenCommentEditorUsesSelectedLineRange(t *testing.T) {
	model := NewModel([]core.ReviewFile{reviewFileWithLines("demo.go", 3)})
	updated, _ := model.Update(keyPress("s"))
	model = updated.(Model)
	updated, _ = model.Update(keyPress("j"))
	model = updated.(Model)
	updated, _ = model.Update(keyPress("j"))
	model = updated.(Model)

	updated, _ = model.Update(keyPress("c"))
	model = updated.(Model)

	require.NotNil(t, model.commentEditor)
	require.Equal(t, core.ReviewLineRef{NewLineNumber: 1, Kind: core.LineKindAdded}, model.commentEditor.Range.Start)
	require.Equal(t, core.ReviewLineRef{NewLineNumber: 3, Kind: core.LineKindAdded}, model.commentEditor.Range.End)
}

func TestModelInlineCommentSubmitCopiesReviewJSON(t *testing.T) {
	t.Parallel()

	clipboard := mocks.NewMockClipboardWriter(t)
	model := NewModelWithClipboardWriter([]core.ReviewFile{reviewFileWithLines("demo.go", 3)}, nil, nil, core.ReviewRequest{DiffMode: core.DiffModeBranch}, clipboard)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	model = updated.(Model)

	updated, _ = model.Update(keyPress("s"))
	model = updated.(Model)
	updated, _ = model.Update(keyPress("j"))
	model = updated.(Model)
	updated, _ = model.Update(keyPress("c"))
	model = updated.(Model)
	require.NotNil(t, model.commentEditor)

	for _, r := range "review note" {
		updated, _ = model.Update(tea.KeyPressMsg{Text: string(r), Code: r})
		model = updated.(Model)
	}

	clipboard.EXPECT().WriteClipboard(mock.Anything, mock.MatchedBy(func(text string) bool {
		return strings.Contains(text, `"file": "demo.go"`) && strings.Contains(text, `"body": "review note"`) && strings.Contains(text, `"new": 1`) && strings.Contains(text, `"new": 2`)
	})).Return(nil).Once()
	updated, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
	model = updated.(Model)
	require.NotNil(t, cmd)
	updated, _ = model.Update(cmd())
	model = updated.(Model)

	assert.Nil(t, model.commentEditor)
	assert.Len(t, model.reviewDraft.Comments(), 1)
	assert.Contains(t, model.copyFeedback, "Review JSON copied (1 comment)")
	assert.Contains(t, model.lastCopiedText, "review note")
	assert.Contains(t, stripANSI(model.View().Content), "review note")
}

func TestModelInlineCommentSubmitCopiesReviewJSONWithClipboardDeadline(t *testing.T) {
	writer := &deadlineRecordingClipboardWriter{}
	model := NewModelWithClipboardWriter([]core.ReviewFile{reviewFileWithLines("demo.go", 1)}, nil, nil, core.ReviewRequest{DiffMode: core.DiffModeBranch}, writer)

	updated, _ := model.Update(keyPress("c"))
	model = updated.(Model)
	for _, r := range "review note" {
		updated, _ = model.Update(tea.KeyPressMsg{Text: string(r), Code: r})
		model = updated.(Model)
	}

	updated, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
	model = updated.(Model)
	require.NotNil(t, cmd)
	updated, _ = model.Update(cmd())
	model = updated.(Model)

	assert.True(t, writer.hadDeadline, "clipboard writes should use a deadline so status feedback cannot remain stuck on Copying review JSON… forever")
	assert.Contains(t, model.copyFeedback, "Review JSON copied")
}

func TestModelInlineCommentSubmitWithProviderDoesNotCopyReviewJSON(t *testing.T) {
	clipboard := mocks.NewMockClipboardWriter(t)
	model := NewModelWithClipboardWriter([]core.ReviewFile{reviewFileWithLines("demo.go", 1)}, nil, nil, core.ReviewRequest{DiffMode: core.DiffModeBranch}, clipboard)
	model.providerInfos = []core.ReviewProviderInfo{{ID: "pi-coding-agent", Label: "pi-coding-agent", Capabilities: core.ReviewProviderCapabilities{PublishReview: true}}}

	updated, _ := model.Update(keyPress("c"))
	model = updated.(Model)
	for _, r := range "review note" {
		updated, _ = model.Update(tea.KeyPressMsg{Text: string(r), Code: r})
		model = updated.(Model)
	}

	updated, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
	model = updated.(Model)

	require.NotNil(t, cmd)
	assert.Nil(t, model.commentEditor)
	assert.Len(t, model.reviewDraft.Comments(), 1)
	assert.Contains(t, model.copyFeedback, "Comment added; press P to publish")
	assert.Empty(t, model.lastCopiedText)
}

func TestModelCopyReviewShortcutCopiesCurrentReviewJSON(t *testing.T) {
	t.Parallel()

	clipboard := mocks.NewMockClipboardWriter(t)
	model := NewModelWithClipboardWriter([]core.ReviewFile{reviewFileWithLines("demo.go", 1)}, nil, nil, core.ReviewRequest{DiffMode: core.DiffModeBranch}, clipboard)
	_, err := model.reviewDraft.AddComment(core.ReviewCommentInput{FilePath: "demo.go", Range: core.ReviewLineRange{Start: core.ReviewLineRef{NewLineNumber: 1, Kind: core.LineKindAdded}, End: core.ReviewLineRef{NewLineNumber: 1, Kind: core.LineKindAdded}}, Body: "manual copy"})
	require.NoError(t, err)

	clipboard.EXPECT().WriteClipboard(mock.Anything, mock.MatchedBy(func(text string) bool {
		return strings.Contains(text, `"body": "manual copy"`) && strings.Contains(text, `"comments"`)
	})).Return(nil).Once()
	updated, cmd := model.Update(keyPress("C"))
	model = updated.(Model)
	require.NotNil(t, cmd)
	assert.Equal(t, "Copying review JSON…", model.copyFeedback)
	assert.Contains(t, stripANSI(model.View().Content), "Copying review JSON")
	updated, _ = model.Update(cmd())
	model = updated.(Model)

	assert.Contains(t, model.copyFeedback, "Review JSON copied (1 comment)")
	assert.Contains(t, model.lastCopiedText, "manual copy")
}

func TestModelCopyReviewShortcutReportsClipboardFailure(t *testing.T) {
	t.Parallel()

	clipboard := mocks.NewMockClipboardWriter(t)
	model := NewModelWithClipboardWriter([]core.ReviewFile{reviewFileWithLines("demo.go", 1)}, nil, nil, core.ReviewRequest{DiffMode: core.DiffModeBranch}, clipboard)
	_, err := model.reviewDraft.AddComment(core.ReviewCommentInput{FilePath: "demo.go", Range: core.ReviewLineRange{Start: core.ReviewLineRef{NewLineNumber: 1, Kind: core.LineKindAdded}, End: core.ReviewLineRef{NewLineNumber: 1, Kind: core.LineKindAdded}}, Body: "manual copy"})
	require.NoError(t, err)

	clipboard.EXPECT().WriteClipboard(mock.Anything, mock.Anything).Return(assert.AnError).Once()
	updated, cmd := model.Update(keyPress("C"))
	model = updated.(Model)
	require.NotNil(t, cmd)
	updated, _ = model.Update(cmd())
	model = updated.(Model)

	assert.Contains(t, model.copyFeedback, "Copy failed")
	assert.Contains(t, model.copyFeedback, assert.AnError.Error())
}

func TestModelInlineCommentCancelAndClear(t *testing.T) {
	t.Parallel()

	model := NewModel([]core.ReviewFile{reviewFileWithLines("demo.go", 1)})
	updated, _ := model.Update(keyPress("c"))
	model = updated.(Model)
	require.NotNil(t, model.commentEditor)

	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	model = updated.(Model)
	assert.Nil(t, model.commentEditor)

	_, err := model.reviewDraft.AddComment(core.ReviewCommentInput{FilePath: "demo.go", Range: core.ReviewLineRange{Start: core.ReviewLineRef{NewLineNumber: 1, Kind: core.LineKindAdded}, End: core.ReviewLineRef{NewLineNumber: 1, Kind: core.LineKindAdded}}, Body: "old"})
	require.NoError(t, err)
	updated, _ = model.Update(keyPress("x"))
	model = updated.(Model)
	assert.Empty(t, model.reviewDraft.Comments())
	assert.Contains(t, model.copyFeedback, "Cleared review")
}
