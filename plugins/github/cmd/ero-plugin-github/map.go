package main

import (
	"time"

	"ero/pkg/plugin"
)

func candidateFromNode(n ghPRNode) githubPRCandidate {
	owner := ""
	if n.HeadRepositoryOwner != nil {
		owner = n.HeadRepositoryOwner.Login
	}
	repo := ""
	if n.HeadRepository != nil {
		repo = n.HeadRepository.Name
	}
	return githubPRCandidate{Number: n.Number, URL: n.URL, Title: n.Title, State: n.State, BaseRef: n.BaseRefName, HeadRef: n.HeadRefName, HeadRepoOwner: owner, HeadRepoName: repo, HeadSHA: n.HeadRefOid}
}

func mapGitHubPRPage(n ghPRNode) githubPRSnapshot {
	s := mapGitHubPRMetadata(n)
	s.IssueComments = mapGitHubIssueComments(n.Comments.Nodes)
	s.Reviews = mapGitHubReviews(n.Reviews.Nodes)
	for _, t := range n.ReviewThreads.Nodes {
		s.Threads = append(s.Threads, mapGitHubThread(t))
	}
	return s
}

func mapGitHubPRMetadata(n ghPRNode) githubPRSnapshot {
	s := githubPRSnapshot{Number: n.Number, URL: n.URL, Title: n.Title, State: n.State, Body: n.Body, BaseRef: n.BaseRefName, HeadRef: n.HeadRefName, HeadSHA: n.HeadRefOid, UpdatedAt: n.UpdatedAt}
	if n.Author != nil {
		s.Author = n.Author.Login
	}
	if n.HeadRepositoryOwner != nil {
		s.HeadRepoOwner = n.HeadRepositoryOwner.Login
	}
	if n.HeadRepository != nil {
		s.HeadRepoName = n.HeadRepository.Name
	}
	return s
}

func mapGitHubIssueComments(comments []ghIssueComment) []plugin.ProviderIssueComment {
	out := make([]plugin.ProviderIssueComment, 0, len(comments))
	for _, c := range comments {
		author := ""
		if c.Author != nil {
			author = c.Author.Login
		}
		out = append(out, plugin.ProviderIssueComment{ExternalID: c.ID, Author: author, Body: c.Body, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, ExternalURL: c.URL})
	}
	return out
}

func mapGitHubReviews(reviews []ghReview) []plugin.ProviderReviewSummary {
	out := make([]plugin.ProviderReviewSummary, 0, len(reviews))
	for _, r := range reviews {
		author := ""
		if r.Author != nil {
			author = r.Author.Login
		}
		out = append(out, plugin.ProviderReviewSummary{ExternalID: r.ID, Author: author, State: r.State, Body: r.Body, SubmittedAt: r.SubmittedAt, ExternalURL: r.URL})
	}
	return out
}

func mapGitHubThread(t ghReviewThread) plugin.RemoteReviewThread {
	// Mark clearly stale or unanchored threads as unmapped before line conversion.
	thread := plugin.RemoteReviewThread{ProviderID: providerID, ExternalID: t.ID, FilePath: t.Path, ExternalURL: firstThreadURL(t), Unmapped: t.IsOutdated || t.Path == "" || t.Line <= 0}
	thread.Range = plugin.ReviewLineRange{End: lineRef(t.Line, t.Side)}
	if t.StartLine > 0 {
		thread.Range.Start = lineRef(t.StartLine, firstNonEmpty(t.StartSide, t.Side))
	} else {
		thread.Range.Start = thread.Range.End
	}
	// Also mark unmapped when GitHub supplied a line but no supported side mapping resolved.
	if thread.Range.End.NewLineNumber == 0 && thread.Range.End.OldLineNumber == 0 {
		thread.Unmapped = true
	}
	for _, c := range t.Comments.Nodes {
		author := ""
		if c.Author != nil {
			author = c.Author.Login
		}
		thread.Comments = append(thread.Comments, plugin.RemoteReviewComment{ExternalID: c.ID, Author: author, Body: c.Body, CreatedAt: c.CreatedAt})
	}
	return thread
}

func lineRef(line int, side string) plugin.ReviewLineRef {
	if line <= 0 {
		return plugin.ReviewLineRef{}
	}
	if side == "LEFT" {
		return plugin.ReviewLineRef{OldLineNumber: line, Kind: "LEFT"}
	}
	return plugin.ReviewLineRef{NewLineNumber: line, Kind: "RIGHT"}
}

func firstThreadURL(t ghReviewThread) string {
	if len(t.Comments.Nodes) == 0 {
		return ""
	}
	return t.Comments.Nodes[0].URL
}

func snapshotResultFromGitHub(s githubPRSnapshot) plugin.LoadRemoteSnapshotResult {
	now := pluginNow()
	return plugin.LoadRemoteSnapshotResult{RuntimeProviderID: providerID, Threads: s.Threads, FetchedAt: &now, Overview: &plugin.ProviderOverview{RuntimeProviderID: providerID, Title: s.Title, Number: s.Number, State: s.State, ExternalURL: s.URL, Author: s.Author, Body: s.Body, BaseRef: s.BaseRef, HeadRef: s.HeadRef, UpdatedAt: s.UpdatedAt, Comments: s.IssueComments, Reviews: s.Reviews}, Metadata: map[string]string{"provider": "github"}}
}

var pluginNow = time.Now
