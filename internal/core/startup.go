package core

// StartupState is the Git repository state used to infer what ero should
// show when the user starts the app without an explicit diff-mode command.
type StartupState struct {
	HasStagedChanges   bool
	HasUnstagedChanges bool
	HasUntrackedFiles  bool
	HasUpstream        bool
	Ahead              int
	Behind             int
	HasDefaultBranch   bool
	DetachedHead       bool
}

// StartupDecisionKind describes how startup detection should proceed.
type StartupDecisionKind string

const (
	// StartupDecisionUseMode means the app can safely load DiffMode directly.
	StartupDecisionUseMode StartupDecisionKind = "use-mode"
	// StartupDecisionPromptLocalChanges means both commit-ready and worktree
	// changes exist, so the user must choose the local review scope.
	StartupDecisionPromptLocalChanges StartupDecisionKind = "prompt-local-changes"
	// StartupDecisionNoReviewableChanges means there is no safe inferred diff.
	StartupDecisionNoReviewableChanges StartupDecisionKind = "no-reviewable-changes"
)

// StartupDecision is the result of resolving StartupState into app behavior.
type StartupDecision struct {
	Kind     StartupDecisionKind
	DiffMode DiffMode
	Message  string
}

func ResolveStartupDecision(state StartupState) StartupDecision {
	hasWorktreeChanges := state.HasUnstagedChanges || state.HasUntrackedFiles

	switch {
	case state.HasStagedChanges && hasWorktreeChanges:
		// DiffModeStaged is the prompt's default selection, not a forced final choice.
		return StartupDecision{Kind: StartupDecisionPromptLocalChanges, DiffMode: DiffModeStaged}
	case hasWorktreeChanges:
		return StartupDecision{Kind: StartupDecisionUseMode, DiffMode: DiffModeWorking}
	case state.HasStagedChanges:
		return StartupDecision{Kind: StartupDecisionUseMode, DiffMode: DiffModeStaged}
	case state.DetachedHead:
		return StartupDecision{Kind: StartupDecisionNoReviewableChanges, Message: "detached HEAD has no safe default diff; choose an explicit diff mode"}
	case state.HasDefaultBranch:
		return StartupDecision{Kind: StartupDecisionUseMode, DiffMode: DiffModeBranch}
	case state.HasUpstream && state.Ahead > 0:
		return StartupDecision{Kind: StartupDecisionUseMode, DiffMode: DiffModeUpstream}
	case state.HasUpstream && state.Behind > 0:
		return StartupDecision{Kind: StartupDecisionNoReviewableChanges, Message: "branch is behind upstream; pull first or choose an explicit diff mode"}
	default:
		return StartupDecision{Kind: StartupDecisionNoReviewableChanges, Message: "no local changes or upstream/default branch diff detected"}
	}
}
