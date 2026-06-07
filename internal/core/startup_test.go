package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveStartupDecisionChoosesLocalChangeModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state StartupState
		want  StartupDecision
	}{
		{
			name:  "mixed staged and unstaged prompts for user choice",
			state: StartupState{HasStagedChanges: true, HasUnstagedChanges: true},
			want:  StartupDecision{Kind: StartupDecisionPromptLocalChanges, DiffMode: DiffModeStaged},
		},
		{
			name:  "mixed staged and untracked prompts for user choice",
			state: StartupState{HasStagedChanges: true, HasUntrackedFiles: true},
			want:  StartupDecision{Kind: StartupDecisionPromptLocalChanges, DiffMode: DiffModeStaged},
		},
		{
			name:  "unstaged only uses working diff",
			state: StartupState{HasUnstagedChanges: true},
			want:  StartupDecision{Kind: StartupDecisionUseMode, DiffMode: DiffModeWorking},
		},
		{
			name:  "untracked only uses working diff",
			state: StartupState{HasUntrackedFiles: true},
			want:  StartupDecision{Kind: StartupDecisionUseMode, DiffMode: DiffModeWorking},
		},
		{
			name:  "staged only uses staged diff",
			state: StartupState{HasStagedChanges: true},
			want:  StartupDecision{Kind: StartupDecisionUseMode, DiffMode: DiffModeStaged},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, ResolveStartupDecision(tt.state))
		})
	}
}

func TestResolveStartupDecisionChoosesBranchStateModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state StartupState
		want  StartupDecision
	}{
		{
			name:  "ahead of upstream uses branch diff when default branch exists",
			state: StartupState{HasUpstream: true, Ahead: 2, HasDefaultBranch: true},
			want:  StartupDecision{Kind: StartupDecisionUseMode, DiffMode: DiffModeBranch},
		},
		{
			name:  "diverged from upstream uses branch diff when default branch exists",
			state: StartupState{HasUpstream: true, Ahead: 1, Behind: 1, HasDefaultBranch: true},
			want:  StartupDecision{Kind: StartupDecisionUseMode, DiffMode: DiffModeBranch},
		},
		{
			name:  "behind upstream only asks user to pull instead of reviewing remote-only changes",
			state: StartupState{HasUpstream: true, Behind: 2},
			want:  StartupDecision{Kind: StartupDecisionNoReviewableChanges, Message: "branch is behind upstream; pull first or choose an explicit diff mode"},
		},
		{
			name:  "no upstream falls back to default branch diff when available",
			state: StartupState{HasDefaultBranch: true},
			want:  StartupDecision{Kind: StartupDecisionUseMode, DiffMode: DiffModeBranch},
		},
		{
			name:  "detached head with no local changes has no safe inferred diff",
			state: StartupState{DetachedHead: true, HasDefaultBranch: true},
			want:  StartupDecision{Kind: StartupDecisionNoReviewableChanges, Message: "detached HEAD has no safe default diff; choose an explicit diff mode"},
		},
		{
			name:  "no remote and no local changes has no reviewable changes",
			state: StartupState{},
			want:  StartupDecision{Kind: StartupDecisionNoReviewableChanges, Message: "no local changes or upstream/default branch diff detected"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, ResolveStartupDecision(tt.state))
		})
	}
}
