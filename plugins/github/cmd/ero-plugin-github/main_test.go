package main

import (
	"context"
	"slices"
	"strings"
	"testing"

	"ero/pkg/plugin"
)

func TestGitHubRemoteParsing(t *testing.T) {
	valid := []string{
		"git@github.com:owner/repo.git",
		"git@github.com:owner/repo.git/",
		"https://github.com/owner/repo.git",
		"https://github.com/owner/repo",
		"https://github.com/owner/repo/",
		"ssh://git@github.com/owner/repo.git",
	}
	for _, raw := range valid {
		remote, ok := parseGitHubRemote(raw)
		if !ok || remote.Owner != "owner" || remote.Name != "repo" {
			t.Fatalf("parseGitHubRemote(%q) = %#v, %v", raw, remote, ok)
		}
	}

	invalid := []string{
		"git@example.com:owner/repo.git",
		"http://github.com/owner/repo",
		"git://github.com/owner/repo",
		"https://notgithub.com/owner/repo",
		"https://github.com/owner",
		"https://github.com/owner/repo/extra",
		"github.com/owner/repo",
		"ssh://github.com/owner/repo.git",
		"ssh://user@github.com/owner/repo.git",
	}
	for _, raw := range invalid {
		if remote, ok := parseGitHubRemote(raw); ok {
			t.Fatalf("parseGitHubRemote(%q) unexpectedly matched %#v", raw, remote)
		}
	}
}

func TestDetectContextRequiresMatchingGitHubPullRequest(t *testing.T) {
	list := ghPRListResponse{}
	list.Repository.PullRequests.Nodes = []ghPRNode{{Number: 12, URL: "https://github.com/owner/repo/pull/12", BaseRefName: "main", HeadRefName: "feature"}}
	provider := githubProvider{newGraphQLClient: func() (graphQLDoer, error) { return &fakeGraphQLClient{listPages: []ghPRListResponse{list}}, nil }}
	review := plugin.ReviewContext{Repository: plugin.RepositoryMetadata{Remotes: []plugin.GitRemote{{Name: "origin", URL: "git@github.com:owner/repo.git"}}, CurrentBranch: "feature", DefaultBranch: "main"}, Target: plugin.ReviewTargetMetadata{Mode: "branch"}}
	result, err := provider.DetectContext(context.Background(), plugin.DetectContextRequest{Context: review})
	if err != nil {
		t.Fatalf("DetectContext returned error: %v", err)
	}
	if !result.Result.Applicable {
		t.Fatalf("expected matching GitHub PR to be applicable: %#v", result)
	}

	list.Repository.PullRequests.Nodes = []ghPRNode{{Number: 13, BaseRefName: "main", HeadRefName: "other"}}
	provider = githubProvider{newGraphQLClient: func() (graphQLDoer, error) { return &fakeGraphQLClient{listPages: []ghPRListResponse{list}}, nil }}
	result, err = provider.DetectContext(context.Background(), plugin.DetectContextRequest{Context: review})
	if err != nil {
		t.Fatalf("DetectContext returned error: %v", err)
	}
	if result.Result.Applicable || !strings.Contains(result.Result.Reason, "no matching") {
		t.Fatalf("expected no matching PR to be unavailable: %#v", result)
	}

	result, err = provider.DetectContext(context.Background(), plugin.DetectContextRequest{Context: plugin.ReviewContext{Repository: plugin.RepositoryMetadata{Remotes: []plugin.GitRemote{{Name: "origin", URL: "https://example.com/github.com/owner/repo"}}}}})
	if err != nil {
		t.Fatalf("DetectContext returned error: %v", err)
	}
	if result.Result.Applicable {
		t.Fatalf("expected malformed/non-GitHub remote to be rejected: %#v", result)
	}
}

func TestGitHubPRMatching(t *testing.T) {
	tests := []struct {
		name       string
		ctx        plugin.ReviewContext
		prs        []githubPRCandidate
		wantNumber int
		wantErr    string
	}{
		{
			name:       "branch mode matches current head branch and default base fallback",
			ctx:        plugin.ReviewContext{Repository: plugin.RepositoryMetadata{CurrentBranch: "feature", DefaultBranch: "main"}, Target: plugin.ReviewTargetMetadata{Mode: "branch"}},
			prs:        []githubPRCandidate{{Number: 1, BaseRef: "main", HeadRef: "feature", HeadRepoOwner: "owner", HeadRepoName: "repo"}},
			wantNumber: 1,
		},
		{
			name:       "range mode uses target base and head refs",
			ctx:        plugin.ReviewContext{Repository: plugin.RepositoryMetadata{DefaultBranch: "main"}, Target: plugin.ReviewTargetMetadata{Mode: "range", BaseRef: "release", HeadRef: "topic"}},
			prs:        []githubPRCandidate{{Number: 2, BaseRef: "release", HeadRef: "topic", HeadRepoOwner: "owner", HeadRepoName: "repo"}},
			wantNumber: 2,
		},
		{
			name:       "fork PR matches branch even when head repository differs from base remote",
			ctx:        plugin.ReviewContext{Repository: plugin.RepositoryMetadata{CurrentBranch: "feature", DefaultBranch: "main"}, Target: plugin.ReviewTargetMetadata{Mode: "branch"}},
			prs:        []githubPRCandidate{{Number: 3, BaseRef: "main", HeadRef: "feature", HeadRepoOwner: "forker", HeadRepoName: "repo"}},
			wantNumber: 3,
		},
		{
			name:       "detached range-only SHA matches exact head SHA",
			ctx:        plugin.ReviewContext{Repository: plugin.RepositoryMetadata{DefaultBranch: "main"}, Target: plugin.ReviewTargetMetadata{Mode: "range", HeadSHA: "abc123"}},
			prs:        []githubPRCandidate{{Number: 4, BaseRef: "main", HeadSHA: "abc123"}},
			wantNumber: 4,
		},
		{
			name:    "ambiguous multiple matches returns not applicable",
			ctx:     plugin.ReviewContext{Repository: plugin.RepositoryMetadata{CurrentBranch: "feature", DefaultBranch: "main"}, Target: plugin.ReviewTargetMetadata{Mode: "branch"}},
			prs:     []githubPRCandidate{{Number: 5, BaseRef: "main", HeadRef: "feature"}, {Number: 6, BaseRef: "main", HeadRef: "feature"}},
			wantErr: plugin.ErrorNotApplicable,
		},
		{
			name:    "default branch fallback does not override explicit base ref",
			ctx:     plugin.ReviewContext{Repository: plugin.RepositoryMetadata{CurrentBranch: "feature", DefaultBranch: "main"}, Target: plugin.ReviewTargetMetadata{Mode: "branch", BaseRef: "release"}},
			prs:     []githubPRCandidate{{Number: 7, BaseRef: "main", HeadRef: "feature"}},
			wantErr: plugin.ErrorNotApplicable,
		},
		{
			name:    "no match returns not applicable",
			ctx:     plugin.ReviewContext{Repository: plugin.RepositoryMetadata{CurrentBranch: "feature", DefaultBranch: "main"}, Target: plugin.ReviewTargetMetadata{Mode: "branch"}},
			prs:     []githubPRCandidate{{Number: 8, BaseRef: "main", HeadRef: "other"}},
			wantErr: plugin.ErrorNotApplicable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := matchGitHubPR(tt.ctx, tt.prs)
			if tt.wantErr != "" {
				if plugin.AsError(err) == nil || plugin.AsError(err).Code != tt.wantErr {
					t.Fatalf("expected error %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("matchGitHubPR returned error: %v", err)
			}
			if got.Number != tt.wantNumber {
				t.Fatalf("expected PR %d, got %#v", tt.wantNumber, got)
			}
		})
	}
}

func TestPublishReviewRequiresAssociatedPullRequest(t *testing.T) {
	provider := githubProvider{execGH: func(context.Context, ...string) (string, string, error) {
		return "", "no pull requests found", assertAnError{}
	}}
	_, err := provider.PublishReview(context.Background(), plugin.PublishReviewParams{})
	if plugin.AsError(err) == nil || plugin.AsError(err).Code != plugin.ErrorNotApplicable {
		t.Fatalf("expected not_applicable, got %v", err)
	}
}

func TestPublishReviewClassifiesGitHubCLIAuthFailure(t *testing.T) {
	provider := githubProvider{execGH: func(context.Context, ...string) (string, string, error) {
		return "", "gh auth required", assertAnError{}
	}}
	_, err := provider.PublishReview(context.Background(), plugin.PublishReviewParams{})
	if plugin.AsError(err) == nil || plugin.AsError(err).Code != plugin.ErrorAuthRequired {
		t.Fatalf("expected auth_required, got %v", err)
	}
}

func TestPublishReviewRejectsMalformedGitHubReviewResponse(t *testing.T) {
	calls := 0
	provider := githubProvider{execGH: func(context.Context, ...string) (string, string, error) {
		calls++
		if calls == 1 {
			return `{"number": 12, "url": "https://github.com/owner/repo/pull/12"}`, "", nil
		}
		return `{`, "", nil
	}}
	_, err := provider.PublishReview(context.Background(), plugin.PublishReviewParams{})
	if plugin.AsError(err) == nil || plugin.AsError(err).Code != plugin.ErrorRemoteValidationFailed {
		t.Fatalf("expected remote_validation_failed, got %v", err)
	}
}

func TestPublishReviewUsesGraphQLMatchedPullRequestWhenContextHasRemote(t *testing.T) {
	list := ghPRListResponse{}
	list.Repository.PullRequests.Nodes = []ghPRNode{{Number: 77, URL: "https://github.com/owner/repo/pull/77", BaseRefName: "release", HeadRefName: "topic"}}
	var calls [][]string
	provider := githubProvider{
		newGraphQLClient: func() (graphQLDoer, error) { return &fakeGraphQLClient{listPages: []ghPRListResponse{list}}, nil },
		execGH: func(_ context.Context, args ...string) (string, string, error) {
			calls = append(calls, slices.Clone(args))
			return `{"id": 77, "html_url": "https://github.com/owner/repo/pull/77#pullrequestreview-77"}`, "", nil
		},
	}
	_, err := provider.PublishReview(context.Background(), plugin.PublishReviewParams{Payload: plugin.ReviewPublishPayload{
		Context: plugin.ReviewContext{Repository: plugin.RepositoryMetadata{Remotes: []plugin.GitRemote{{URL: "git@github.com:owner/repo.git"}}}, Target: plugin.ReviewTargetMetadata{Mode: "range", BaseRef: "release", HeadRef: "topic"}},
		Draft:   plugin.ReviewDraftSnapshot{Summary: "summary"},
	}})
	if err != nil {
		t.Fatalf("PublishReview returned error: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected only publish gh call, got %#v", calls)
	}
	joined := strings.Join(calls[0], "\x00")
	if strings.Contains(joined, "pr\x00view") || !strings.Contains(joined, "pulls/77/reviews") {
		t.Fatalf("publish did not use GraphQL-matched PR: %#v", calls[0])
	}
}

func TestPublishReviewSubmitsGitHubReview(t *testing.T) {
	var calls [][]string
	provider := githubProvider{execGH: func(_ context.Context, args ...string) (string, string, error) {
		calls = append(calls, slices.Clone(args))
		if len(calls) == 1 {
			return `{"number": 12, "url": "https://github.com/owner/repo/pull/12"}`, "", nil
		}
		return `{"id": 99, "html_url": "https://github.com/owner/repo/pull/12#pullrequestreview-99"}`, "", nil
	}}
	result, err := provider.PublishReview(context.Background(), plugin.PublishReviewParams{Payload: plugin.ReviewPublishPayload{
		Context: plugin.ReviewContext{Repository: plugin.RepositoryMetadata{HeadSHA: "abc123"}},
		Draft: plugin.ReviewDraftSnapshot{Decision: plugin.ReviewDecisionRequestChanges, Summary: "Please adjust", Comments: []plugin.ReviewComment{{
			ID:       "comment-1",
			FilePath: "docs/plugins.md",
			Body:     "tighten this",
			Range: plugin.ReviewLineRange{
				Start: plugin.ReviewLineRef{NewLineNumber: 9},
				End:   plugin.ReviewLineRef{NewLineNumber: 15},
			},
		}}},
	}})
	if err != nil {
		t.Fatalf("PublishReview returned error: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 gh calls, got %#v", calls)
	}
	publish := strings.Join(calls[1], "\x00")
	for _, want := range []string{
		"api", "-X", "POST", "repos/{owner}/{repo}/pulls/12/reviews",
		"commit_id=abc123", "event=REQUEST_CHANGES", "body=Please adjust",
		"comments[][path]=docs/plugins.md", "comments[][body]=tighten this",
		"comments[][start_line]=9", "comments[][line]=15", "comments[][side]=RIGHT",
	} {
		if !strings.Contains(publish, want) {
			t.Fatalf("publish args missing %q: %#v", want, calls[1])
		}
	}
	if result.Result.ExternalReviewID != "99" || result.Result.ExternalURL == "" || len(result.Result.PublishedRefs) != 1 || result.Result.PublishedRefs[0].LocalCommentID != "comment-1" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestGithubCommentArgsMapsDeletedLinesToLeftSide(t *testing.T) {
	args, err := githubCommentArgs(plugin.ReviewComment{ID: "c", FilePath: "old.go", Body: "remove", Range: plugin.ReviewLineRange{Start: plugin.ReviewLineRef{OldLineNumber: 4}, End: plugin.ReviewLineRef{OldLineNumber: 4}}})
	if err != nil {
		t.Fatalf("githubCommentArgs returned error: %v", err)
	}
	joined := strings.Join(args, "\x00")
	if !strings.Contains(joined, "comments[][line]=4") || !strings.Contains(joined, "comments[][side]=LEFT") {
		t.Fatalf("unexpected args: %#v", args)
	}
}

type assertAnError struct{}

func (assertAnError) Error() string { return "assert error" }
