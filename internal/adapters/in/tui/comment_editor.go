package tui

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"ero/internal/adapters/in/tui/theme"
)

func commentEditorStyle() lipgloss.Style {
	return theme.SearchPaneStyle
}

func commentEditorTitleStyle() lipgloss.Style {
	return theme.SearchPaneTitleStyle
}

type CommentEditorAction string

const (
	CommentEditorActionNone   CommentEditorAction = ""
	CommentEditorActionSubmit CommentEditorAction = "submit"
	CommentEditorActionCancel CommentEditorAction = "cancel"
)

type CommentEditor struct {
	input textarea.Model
	width int
}

func NewCommentEditor(width int) CommentEditor {
	input := textarea.New()
	input.Prompt = "│ "
	input.Placeholder = "Write a review comment"
	input.ShowLineNumbers = false
	input.DynamicHeight = true
	input.MinHeight = 3
	input.MaxHeight = 8
	input.SetWidth(max(width-8, 20))
	input.SetHeight(3)
	return CommentEditor{input: input, width: width}
}

func (e *CommentEditor) Focus() tea.Cmd {
	return e.input.Focus()
}

func (e CommentEditor) Value() string {
	return e.input.Value()
}

func (e CommentEditor) Update(msg tea.Msg) (CommentEditor, CommentEditorAction, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if ok {
		switch {
		case key.Code == tea.KeyEsc:
			return e, CommentEditorActionCancel, nil
		case commentEditorSubmitKey(key):
			return e, CommentEditorActionSubmit, nil
		}
	}

	var cmd tea.Cmd
	e.input, cmd = e.input.Update(msg)
	return e, CommentEditorActionNone, cmd
}

func commentEditorSubmitKey(key tea.KeyPressMsg) bool {
	return (key.Code == tea.KeyEnter && key.Mod == tea.ModCtrl) || key.String() == "ctrl+j" || key.String() == "ctrl+s"
}

func (e CommentEditor) View() string {
	return e.ViewWithWidth(e.width)
}

func (e CommentEditor) ViewWithWidth(availableWidth int) string {
	width := max(availableWidth-8, 24)
	input := e.input
	input.SetWidth(max(width-4, 20))
	lines := []string{
		commentEditorTitleStyle().Render("Add review comment"),
		input.View(),
		renderKeyHints([]KeyHint{{Key: commentSubmitKeyLabel(), Label: "submit"}, {Key: "esc", Label: "cancel"}}),
	}
	return commentEditorStyle().Width(width).Render(strings.Join(lines, "\n"))
}
