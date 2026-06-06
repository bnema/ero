package keymap

type Action string

const (
	ActionNone               Action = ""
	ActionQuit               Action = "quit"
	ActionMoveUp             Action = "move_up"
	ActionMoveDown           Action = "move_down"
	ActionPageUp             Action = "page_up"
	ActionPageDown           Action = "page_down"
	ActionMoveStart          Action = "move_start"
	ActionMoveEnd            Action = "move_end"
	ActionToggleSelection    Action = "toggle_selection"
	ActionClearSelection     Action = "clear_selection"
	ActionOpenComment        Action = "open_comment"
	ActionClearReview        Action = "clear_review"
	ActionPublishReview      Action = "publish_review"
	ActionCopyReviewJSON     Action = "copy_review_json"
	ActionCopyPlain          Action = "copy_plain"
	ActionCopyWithMetadata   Action = "copy_with_metadata"
	ActionOpenFileSearch     Action = "open_file_search"
	ActionOpenGrepSearch     Action = "open_grep_search"
	ActionOpenDiffMode       Action = "open_diff_mode"
	ActionPreviousFile       Action = "previous_file"
	ActionNextFile           Action = "next_file"
	ActionExpandAllContext   Action = "expand_all_context"
	ActionExpandMoreContext  Action = "expand_more_context"
	ActionCycleProvider      Action = "cycle_provider"
	ActionOpenProviderPicker Action = "open_provider_picker"
	ActionRefreshProvider    Action = "refresh_provider"
	ActionTogglePRSheet      Action = "toggle_pr_sheet"
	ActionOpenHelp           Action = "open_help"
)

func ReviewAction(key string) Action {
	switch key {
	case "q", "ctrl+c":
		return ActionQuit
	case "up", "k":
		return ActionMoveUp
	case "down", "j":
		return ActionMoveDown
	case "pgup":
		return ActionPageUp
	case "pgdown":
		return ActionPageDown
	case "home":
		return ActionMoveStart
	case "end":
		return ActionMoveEnd
	case "s", "space":
		return ActionToggleSelection
	case "esc":
		return ActionClearSelection
	case "c":
		return ActionOpenComment
	case "x":
		return ActionClearReview
	case "P", "shift+p":
		return ActionPublishReview
	case "C", "shift+c":
		return ActionCopyReviewJSON
	case "y":
		return ActionCopyPlain
	case "Y", "shift+y":
		return ActionCopyWithMetadata
	case "f":
		return ActionOpenFileSearch
	case "/":
		return ActionOpenGrepSearch
	case "d":
		return ActionOpenDiffMode
	case "left", "h":
		return ActionPreviousFile
	case "right", "l":
		return ActionNextFile
	case "a":
		return ActionExpandAllContext
	case "enter":
		return ActionExpandMoreContext
	case "alt+p":
		return ActionCycleProvider
	case "p":
		return ActionOpenProviderPicker
	case "r":
		return ActionRefreshProvider
	case "o":
		return ActionTogglePRSheet
	case "?", "shift+/":
		return ActionOpenHelp
	default:
		return ActionNone
	}
}
