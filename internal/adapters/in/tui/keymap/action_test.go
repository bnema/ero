package keymap

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReviewAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
		want Action
	}{
		{name: "quit", key: "q", want: ActionQuit},
		{name: "quit ctrl+c alias", key: "ctrl+c", want: ActionQuit},
		{name: "move up arrow", key: "up", want: ActionMoveUp},
		{name: "move up vim alias", key: "k", want: ActionMoveUp},
		{name: "move down arrow", key: "down", want: ActionMoveDown},
		{name: "move down vim alias", key: "j", want: ActionMoveDown},
		{name: "page up", key: "pgup", want: ActionPageUp},
		{name: "page down", key: "pgdown", want: ActionPageDown},
		{name: "move start", key: "home", want: ActionMoveStart},
		{name: "move end", key: "end", want: ActionMoveEnd},
		{name: "toggle selection", key: "s", want: ActionToggleSelection},
		{name: "toggle selection space alias", key: "space", want: ActionToggleSelection},
		{name: "clear selection", key: "esc", want: ActionClearSelection},
		{name: "open comment", key: "c", want: ActionOpenComment},
		{name: "clear review binding", key: "x", want: ActionClearReview},
		{name: "publish review shifted binding", key: "P", want: ActionPublishReview},
		{name: "copy review json shifted binding", key: "C", want: ActionCopyReviewJSON},
		{name: "copy plain", key: "y", want: ActionCopyPlain},
		{name: "copy with metadata shifted binding", key: "Y", want: ActionCopyWithMetadata},
		{name: "open file search", key: "f", want: ActionOpenFileSearch},
		{name: "open grep search", key: "/", want: ActionOpenGrepSearch},
		{name: "open diff mode", key: "d", want: ActionOpenDiffMode},
		{name: "previous file left arrow", key: "left", want: ActionPreviousFile},
		{name: "previous file vim alias", key: "h", want: ActionPreviousFile},
		{name: "previous file p alias", key: "p", want: ActionPreviousFile},
		{name: "next file right arrow", key: "right", want: ActionNextFile},
		{name: "next file vim alias", key: "l", want: ActionNextFile},
		{name: "next file n alias", key: "n", want: ActionNextFile},
		{name: "expand all context", key: "a", want: ActionExpandAllContext},
		{name: "expand more context enter binding", key: "enter", want: ActionExpandMoreContext},
		{name: "cycle provider", key: "g", want: ActionCycleProvider},
		{name: "open provider picker", key: "G", want: ActionOpenProviderPicker},
		{name: "refresh provider", key: "r", want: ActionRefreshProvider},
		{name: "toggle pr sheet", key: "o", want: ActionTogglePRSheet},
		{name: "open help", key: "?", want: ActionOpenHelp},
		{name: "unknown", key: "R", want: ActionNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, ReviewAction(tt.key))
		})
	}
}
