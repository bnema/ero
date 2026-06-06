package main

import (
	"context"
	"maps"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"ero/pkg/plugin"
)

type fakeGraphQLClient struct {
	listPages     []ghPRListResponse
	snapshotPages []ghPRSnapshotResponse
	listCalls     int
	snapshotCalls int
	vars          []map[string]any
}

func (f *fakeGraphQLClient) DoWithContext(_ context.Context, query string, variables map[string]any, response any) error {
	copied := make(map[string]any, len(variables))
	maps.Copy(copied, variables)
	f.vars = append(f.vars, copied)
	switch r := response.(type) {
	case *ghPRListResponse:
		*r = f.listPages[f.listCalls]
		f.listCalls++
	case *ghPRSnapshotResponse:
		*r = f.snapshotPages[f.snapshotCalls]
		f.snapshotCalls++
	default:
		panic("unexpected GraphQL response type")
	}
	_ = query
	return nil
}

func TestGitHubPRSnapshotQueryUsesReviewThreadSchemaFieldNames(t *testing.T) {
	if strings.Contains(githubPRSnapshotQuery, "\n          side\n") || strings.Contains(githubPRSnapshotQuery, "\n          startSide\n") {
		t.Fatalf("review thread query must use GitHub schema fields diffSide/startDiffSide, not side/startSide:\n%s", githubPRSnapshotQuery)
	}
	if !strings.Contains(githubPRSnapshotQuery, "\n          diffSide\n") || !strings.Contains(githubPRSnapshotQuery, "\n          startDiffSide\n") {
		t.Fatalf("review thread query is missing diffSide/startDiffSide:\n%s", githubPRSnapshotQuery)
	}
}

func TestGitHubGraphQLMapping(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	actor := &ghActor{Login: "octo"}
	pr := ghPRNode{Number: 12, URL: "https://github.com/owner/repo/pull/12", Title: "Add feature", State: "OPEN", Body: "**markdown**", Author: actor, BaseRefName: "main", HeadRefName: "feature", HeadRefOid: "abc", UpdatedAt: &now, HeadRepositoryOwner: actor, HeadRepository: &struct {
		Name string `json:"name"`
	}{Name: "repo"}}
	pr.Comments.Nodes = []ghIssueComment{{ID: "ic1", URL: "https://c", Body: "issue body", Author: actor, CreatedAt: now, UpdatedAt: now}}
	pr.Reviews.Nodes = []ghReview{{ID: "rv1", URL: "https://r", State: "COMMENTED", Body: "summary", Author: actor, SubmittedAt: now}}
	pr.ReviewThreads.Nodes = []ghReviewThread{
		thread("t1", "file.go", 10, "RIGHT", 0, "", false, "c1", "single"),
		thread("t2", "file.go", 20, "RIGHT", 18, "RIGHT", false, "c2", "multi"),
		thread("t3", "old.go", 7, "LEFT", 0, "", false, "c3", "deleted"),
		thread("t4", "gone.go", 0, "RIGHT", 0, "", true, "c4", "outdated"),
	}
	s := mapGitHubPRPage(pr)
	if s.Number != 12 || s.Body != "**markdown**" || s.Author != "octo" || s.BaseRef != "main" || s.HeadRef != "feature" || s.UpdatedAt != &now {
		t.Fatalf("metadata not mapped: %#v", s)
	}
	if len(s.IssueComments) != 1 || s.IssueComments[0].Body != "issue body" || len(s.Reviews) != 1 || s.Reviews[0].Body != "summary" {
		t.Fatalf("overview comments/reviews not mapped: %#v", s)
	}
	if len(s.Threads) != 4 || s.Threads[0].Range.End.NewLineNumber != 10 || s.Threads[1].Range.Start.NewLineNumber != 18 || s.Threads[2].Range.End.OldLineNumber != 7 || !s.Threads[3].Unmapped {
		t.Fatalf("threads not mapped/classified: %#v", s.Threads)
	}
}

func TestLoadRemoteSnapshotAgainstGitHubWhenEnabled(t *testing.T) {
	prNumber := strings.TrimSpace(os.Getenv("ERO_GITHUB_INTEGRATION_PR"))
	if prNumber == "" {
		t.Skip("set ERO_GITHUB_INTEGRATION_PR to run GitHub integration snapshot fetch")
	}
	number, err := strconv.Atoi(prNumber)
	if err != nil {
		t.Fatalf("invalid ERO_GITHUB_INTEGRATION_PR: %v", err)
	}
	client, err := defaultGraphQLClient()
	if err != nil {
		t.Fatalf("defaultGraphQLClient: %v", err)
	}
	got, err := fetchGitHubPRSnapshot(context.Background(), client, githubRemote{Owner: "bnema", Name: "ero"}, number)
	if err != nil {
		t.Fatalf("fetchGitHubPRSnapshot(%d): %v", number, err)
	}
	if got.Number != number || got.Title == "" {
		t.Fatalf("unexpected snapshot metadata: %#v", got)
	}
	if len(got.Threads) == 0 {
		t.Fatalf("expected at least one review thread on PR %d", number)
	}
}

func TestLoadRemoteSnapshotPaginatesAndMaps(t *testing.T) {
	now := time.Now().UTC()
	actor := &ghActor{Login: "octo"}
	owner := &ghActor{Login: "owner"}
	list1 := ghPRListResponse{}
	list1.Repository.PullRequests.Nodes = []ghPRNode{{Number: 12, BaseRefName: "main", HeadRefName: "other"}}
	list1.Repository.PullRequests.PageInfo = ghPageInfo{HasNextPage: true, EndCursor: "p2"}
	list2 := ghPRListResponse{}
	list2.Repository.PullRequests.Nodes = []ghPRNode{{Number: 13, BaseRefName: "main", HeadRefName: "feature", HeadRepositoryOwner: owner, HeadRepository: &struct {
		Name string `json:"name"`
	}{Name: "repo"}}}
	page1 := ghPRSnapshotResponse{}
	page1.Repository.PullRequest = ghPRNode{Number: 13, URL: "https://pr", Title: "PR", State: "OPEN", Author: actor, BaseRefName: "main", HeadRefName: "feature", UpdatedAt: &now}
	page1.Repository.PullRequest.Comments.Nodes = []ghIssueComment{{ID: "i1", Body: "first", CreatedAt: now, UpdatedAt: now}}
	page1.Repository.PullRequest.Comments.PageInfo = ghPageInfo{HasNextPage: true, EndCursor: "c2"}
	page1.Repository.PullRequest.Reviews.PageInfo = ghPageInfo{HasNextPage: true, EndCursor: "r2"}
	page1.Repository.PullRequest.ReviewThreads.PageInfo = ghPageInfo{HasNextPage: true, EndCursor: "t2"}
	page2 := ghPRSnapshotResponse{}
	page2.Repository.PullRequest = page1.Repository.PullRequest
	page2.Repository.PullRequest.Comments.PageInfo = ghPageInfo{}
	page2.Repository.PullRequest.Reviews.PageInfo = ghPageInfo{}
	page2.Repository.PullRequest.ReviewThreads.PageInfo = ghPageInfo{}
	page2.Repository.PullRequest.Comments.Nodes = []ghIssueComment{{ID: "i2", Body: "second", CreatedAt: now, UpdatedAt: now}}
	page2.Repository.PullRequest.Reviews.Nodes = []ghReview{{ID: "r1", Body: "review", SubmittedAt: now}}
	page2.Repository.PullRequest.ReviewThreads.Nodes = []ghReviewThread{thread("t", "file.go", 3, "RIGHT", 0, "", false, "tc", "body")}
	fake := &fakeGraphQLClient{listPages: []ghPRListResponse{list1, list2}, snapshotPages: []ghPRSnapshotResponse{page1, page2}}
	provider := githubProvider{newGraphQLClient: func() (graphQLDoer, error) { return fake, nil }}
	got, err := provider.LoadRemoteSnapshot(context.Background(), plugin.LoadRemoteSnapshotRequest{Context: plugin.ReviewContext{Repository: plugin.RepositoryMetadata{Remotes: []plugin.GitRemote{{URL: "git@github.com:owner/repo.git"}}, CurrentBranch: "feature", DefaultBranch: "main"}, Target: plugin.ReviewTargetMetadata{Mode: "branch"}}})
	if err != nil {
		t.Fatalf("LoadRemoteSnapshot returned error: %v", err)
	}
	if fake.listCalls != 2 || fake.snapshotCalls != 2 || got.Overview.Number != 13 || len(got.Overview.Comments) != 2 || len(got.Overview.Reviews) != 1 || len(got.Threads) != 1 || got.Threads[0].Comments[0].Body != "body" {
		t.Fatalf("unexpected snapshot: calls %d/%d %#v", fake.listCalls, fake.snapshotCalls, got)
	}
}

func TestLoadRemoteSnapshotDoesNotRefetchCompletedCollections(t *testing.T) {
	now := time.Now().UTC()
	list := ghPRListResponse{}
	list.Repository.PullRequests.Nodes = []ghPRNode{{Number: 1, BaseRefName: "main", HeadRefName: "feature"}}
	page1 := ghPRSnapshotResponse{}
	page1.Repository.PullRequest = ghPRNode{Number: 1, Title: "PR"}
	page1.Repository.PullRequest.Comments.Nodes = []ghIssueComment{{ID: "i1", Body: "first", CreatedAt: now, UpdatedAt: now}}
	page1.Repository.PullRequest.Comments.PageInfo = ghPageInfo{}
	page1.Repository.PullRequest.ReviewThreads.Nodes = []ghReviewThread{thread("t1", "file.go", 1, "RIGHT", 0, "", false, "c1", "first thread")}
	page1.Repository.PullRequest.ReviewThreads.PageInfo = ghPageInfo{HasNextPage: true, EndCursor: "t2"}
	page2 := ghPRSnapshotResponse{}
	page2.Repository.PullRequest = ghPRNode{Number: 1, Title: "PR"}
	page2.Repository.PullRequest.Comments.Nodes = []ghIssueComment{{ID: "i1-duplicate-if-refetched", Body: "duplicate", CreatedAt: now, UpdatedAt: now}}
	page2.Repository.PullRequest.Comments.PageInfo = ghPageInfo{}
	page2.Repository.PullRequest.ReviewThreads.Nodes = []ghReviewThread{thread("t2", "file.go", 2, "RIGHT", 0, "", false, "c2", "second thread")}
	fake := &fakeGraphQLClient{listPages: []ghPRListResponse{list}, snapshotPages: []ghPRSnapshotResponse{page1, page2}}
	provider := githubProvider{newGraphQLClient: func() (graphQLDoer, error) { return fake, nil }}
	got, err := provider.LoadRemoteSnapshot(context.Background(), plugin.LoadRemoteSnapshotRequest{Context: plugin.ReviewContext{Repository: plugin.RepositoryMetadata{Remotes: []plugin.GitRemote{{URL: "https://github.com/owner/repo"}}, CurrentBranch: "feature", DefaultBranch: "main"}, Target: plugin.ReviewTargetMetadata{Mode: "branch"}}})
	if err != nil {
		t.Fatalf("LoadRemoteSnapshot returned error: %v", err)
	}
	if len(got.Overview.Comments) != 1 || got.Overview.Comments[0].ExternalID != "i1" || len(got.Threads) != 2 {
		t.Fatalf("completed comments should not be re-appended while threads page: %#v", got)
	}
	if len(fake.vars) < 3 || fake.vars[1]["commentsAfter"] != nil || fake.vars[2]["commentsAfter"] != nil || fake.vars[2]["threadsAfter"] != "t2" {
		t.Fatalf("unexpected pagination variables: %#v", fake.vars)
	}
}

func TestLoadRemoteSnapshotKeepsPartialThreadWhenNestedCommentsArePaginated(t *testing.T) {
	list := ghPRListResponse{}
	list.Repository.PullRequests.Nodes = []ghPRNode{{Number: 1, BaseRefName: "main", HeadRefName: "feature"}}
	page := ghPRSnapshotResponse{}
	page.Repository.PullRequest = ghPRNode{Number: 1}
	thread := thread("t", "file.go", 1, "RIGHT", 0, "", false, "c", "body")
	thread.Comments.PageInfo = ghPageInfo{HasNextPage: true, EndCursor: "more"}
	page.Repository.PullRequest.ReviewThreads.Nodes = []ghReviewThread{thread}
	fake := &fakeGraphQLClient{listPages: []ghPRListResponse{list}, snapshotPages: []ghPRSnapshotResponse{page}}
	provider := githubProvider{newGraphQLClient: func() (graphQLDoer, error) { return fake, nil }}
	got, err := provider.LoadRemoteSnapshot(context.Background(), plugin.LoadRemoteSnapshotRequest{Context: plugin.ReviewContext{Repository: plugin.RepositoryMetadata{Remotes: []plugin.GitRemote{{URL: "https://github.com/owner/repo"}}, CurrentBranch: "feature", DefaultBranch: "main"}, Target: plugin.ReviewTargetMetadata{Mode: "branch"}}})
	if err != nil {
		t.Fatalf("LoadRemoteSnapshot returned error: %v", err)
	}
	if len(got.Threads) != 1 || !got.Threads[0].Unmapped || len(got.Threads[0].Comments) != 1 {
		t.Fatalf("expected partial thread first page marked unmapped, got %#v", got.Threads)
	}
}

func TestLoadRemoteThreadsUsesSnapshotThreads(t *testing.T) {
	list := ghPRListResponse{}
	list.Repository.PullRequests.Nodes = []ghPRNode{{Number: 1, BaseRefName: "main", HeadRefName: "feature"}}
	page := ghPRSnapshotResponse{}
	page.Repository.PullRequest = ghPRNode{Number: 1}
	page.Repository.PullRequest.ReviewThreads.Nodes = []ghReviewThread{thread("t", "f", 1, "RIGHT", 0, "", false, "c", "b")}
	fake := &fakeGraphQLClient{listPages: []ghPRListResponse{list}, snapshotPages: []ghPRSnapshotResponse{page}}
	provider := githubProvider{newGraphQLClient: func() (graphQLDoer, error) { return fake, nil }}
	got, err := provider.LoadRemoteThreads(context.Background(), plugin.LoadRemoteThreadsRequest{Context: plugin.ReviewContext{Repository: plugin.RepositoryMetadata{Remotes: []plugin.GitRemote{{URL: "https://github.com/owner/repo"}}, CurrentBranch: "feature", DefaultBranch: "main"}, Target: plugin.ReviewTargetMetadata{Mode: "branch"}}})
	if err != nil || len(got.Threads) != 1 {
		t.Fatalf("LoadRemoteThreads = %#v, %v", got, err)
	}
}

func thread(id, path string, line int, side string, start int, startSide string, outdated bool, cid, body string) ghReviewThread {
	t := ghReviewThread{ID: id, Path: path, Line: line, Side: side, StartLine: start, StartSide: startSide, IsOutdated: outdated}
	t.Comments.Nodes = []ghReviewThreadComment{{ID: cid, URL: "https://thread", Body: body, Author: &ghActor{Login: "reviewer"}, CreatedAt: time.Now().UTC()}}
	return t
}
