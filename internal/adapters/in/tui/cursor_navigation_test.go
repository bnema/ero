package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ero/internal/core"
)

func TestModelCursorNavigationKeepsCursorVisibleWithoutRebuildingDocument(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		keys            []tea.KeyPressMsg
		wantCursor      int
		wantViewportTop int
	}{
		{
			name: "j moves cursor down and scrolls only when needed",
			keys: []tea.KeyPressMsg{
				keyPress("j"), keyPress("j"), keyPress("j"), keyPress("j"), keyPress("j"),
				keyPress("j"), keyPress("j"), keyPress("j"), keyPress("j"), keyPress("j"),
			},
			wantCursor:      12,
			wantViewportTop: 4,
		},
		{
			name: "down moves cursor down and scrolls only when needed",
			keys: []tea.KeyPressMsg{
				{Code: tea.KeyDown}, {Code: tea.KeyDown}, {Code: tea.KeyDown}, {Code: tea.KeyDown}, {Code: tea.KeyDown},
				{Code: tea.KeyDown}, {Code: tea.KeyDown}, {Code: tea.KeyDown}, {Code: tea.KeyDown}, {Code: tea.KeyDown},
			},
			wantCursor:      12,
			wantViewportTop: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model := NewModel([]core.ReviewFile{reviewFileWithLines("demo.go", 80)})
			updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
			model = updated.(Model)
			initialRows := model.reviewRows
			initialContent := model.reviewViewport.GetContent()

			for _, key := range tt.keys {
				updated, _ = model.Update(key)
				model = updated.(Model)
			}

			require.Equal(t, tt.wantCursor, model.cursorRow)
			assert.Equal(t, tt.wantViewportTop, model.reviewViewport.YOffset())
			assert.Same(t, &initialRows[0], &model.reviewRows[0])
			assert.Equal(t, initialContent, model.reviewViewport.GetContent())
			assert.LessOrEqual(t, model.reviewViewport.YOffset(), model.cursorRow)
			assert.Less(t, model.cursorRow, model.reviewViewport.YOffset()+model.reviewViewport.Height())
		})
	}
}

func TestModelMouseWheelScrollsReviewDocument(t *testing.T) {
	t.Parallel()

	model := NewModel([]core.ReviewFile{reviewFileWithLines("demo.go", 80)})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	model = updated.(Model)

	for range 10 {
		updated, _ = model.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelDown}))
		model = updated.(Model)
	}

	assert.Equal(t, 12, model.cursorRow)
	assert.Equal(t, 4, model.reviewViewport.YOffset())

	updated, _ = model.Update(tea.MouseWheelMsg(tea.Mouse{Button: tea.MouseWheelUp}))
	model = updated.(Model)

	assert.Equal(t, 11, model.cursorRow)
	assert.Equal(t, 4, model.reviewViewport.YOffset())
}

func TestModelCursorNavigationPreservesAbsoluteAndPageViewportSemantics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		setupKeys       []tea.KeyPressMsg
		key             tea.KeyPressMsg
		wantCursor      int
		wantViewportTop int
	}{
		{name: "home clamps to true viewport top after scrolling", setupKeys: repeatKey(tea.KeyPressMsg{Code: tea.KeyDown}, 10), key: tea.KeyPressMsg{Code: tea.KeyHome}, wantCursor: 2, wantViewportTop: 0},
		{name: "end clamps to last selectable row and shows document bottom", key: tea.KeyPressMsg{Code: tea.KeyEnd}, wantCursor: 21, wantViewportTop: 17},
		{name: "page down moves viewport by one page from top", key: tea.KeyPressMsg{Code: tea.KeyPgDown}, wantCursor: 7, wantViewportTop: 5},
		{name: "page up moves viewport by one page after scrolling", setupKeys: repeatKey(tea.KeyPressMsg{Code: tea.KeyPgDown}, 2), key: tea.KeyPressMsg{Code: tea.KeyPgUp}, wantCursor: 7, wantViewportTop: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model := NewModel([]core.ReviewFile{reviewFileWithLines("demo.go", 20)})
			updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 6})
			model = updated.(Model)

			for _, key := range tt.setupKeys {
				updated, _ = model.Update(key)
				model = updated.(Model)
			}

			updated, _ = model.Update(tt.key)
			model = updated.(Model)

			assert.Equal(t, tt.wantCursor, model.cursorRow)
			assert.Equal(t, tt.wantViewportTop, model.reviewViewport.YOffset())
		})
	}
}

func repeatKey(key tea.KeyPressMsg, count int) []tea.KeyPressMsg {
	keys := make([]tea.KeyPressMsg, count)
	for i := range keys {
		keys[i] = key
	}
	return keys
}

func TestModelJumpsUpdateCursorRow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		searchModeKey tea.KeyPressMsg
		query         string
		wantAnchor    func(Model) int
	}{
		{
			name:          "file search moves cursor to file header",
			searchModeKey: keyPress("f"),
			query:         "zeta",
			wantAnchor: func(model Model) int {
				return model.reviewAnchors.FileRows[1]
			},
		},
		{
			name:          "grep search moves cursor to matched line",
			searchModeKey: keyPress("/"),
			query:         "needle",
			wantAnchor: func(model Model) int {
				return model.reviewAnchors.LineRows[ReviewLineAnchor{FileIndex: 1, SectionIndex: 0, LineIndex: 3}]
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model := NewModel([]core.ReviewFile{
				reviewFileWithLines("alpha.go", 20),
				{
					Path: "zeta.go",
					Sections: []core.ReviewSection{{ID: "changed", Kind: core.SectionKindChanged, Lines: []core.ReviewLine{
						{NewLineNumber: 1, Content: "one", Kind: core.LineKindAdded},
						{NewLineNumber: 2, Content: "two", Kind: core.LineKindAdded},
						{NewLineNumber: 3, Content: "three", Kind: core.LineKindAdded},
						{NewLineNumber: 4, Content: "needle", Kind: core.LineKindAdded},
					}}},
				},
			})
			updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 8})
			model = updated.(Model)
			updated, _ = model.Update(tt.searchModeKey)
			model = updated.(Model)
			model = typeQuery(t, model, tt.query)

			updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			model = updated.(Model)

			wantCursor := tt.wantAnchor(model)
			assert.Equal(t, wantCursor, model.cursorRow)
			assert.LessOrEqual(t, model.reviewViewport.YOffset(), model.cursorRow)
			assert.Less(t, model.cursorRow, model.reviewViewport.YOffset()+model.reviewViewport.Height())
		})
	}
}
