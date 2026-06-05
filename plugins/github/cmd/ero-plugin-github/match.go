package main

import (
	"fmt"
	"strings"

	"ero/pkg/plugin"
)

type githubPRCandidate struct {
	Number        int
	URL           string
	Title         string
	State         string
	BaseRef       string
	HeadRef       string
	HeadRepoOwner string
	HeadRepoName  string
	HeadSHA       string
}

func matchGitHubPR(ctx plugin.ReviewContext, candidates []githubPRCandidate) (githubPRCandidate, error) {
	matches := make([]githubPRCandidate, 0, 1)
	for _, pr := range candidates {
		if githubPRMatches(ctx, pr) {
			matches = append(matches, pr)
		}
	}
	if len(matches) == 0 {
		return githubPRCandidate{}, plugin.NewError(plugin.ErrorNotApplicable, "no matching GitHub pull request found")
	}
	if len(matches) > 1 {
		return githubPRCandidate{}, plugin.NewErrorf(plugin.ErrorNotApplicable, "ambiguous GitHub pull request match: %d candidates matched", len(matches))
	}
	return matches[0], nil
}

func githubPRMatches(ctx plugin.ReviewContext, pr githubPRCandidate) bool {
	base := strings.TrimSpace(ctx.Target.BaseRef)
	if base == "" {
		base = strings.TrimSpace(ctx.Repository.DefaultBranch)
	}
	if base != "" && !refEqual(pr.BaseRef, base) {
		return false
	}

	headSHA := firstNonEmpty(ctx.Target.HeadSHA, ctx.Repository.HeadSHA)
	headRef := strings.TrimSpace(ctx.Target.HeadRef)
	if headRef == "" && strings.EqualFold(ctx.Target.Mode, "branch") {
		headRef = strings.TrimSpace(ctx.Repository.CurrentBranch)
	}
	if headRef != "" {
		if !refEqual(pr.HeadRef, headRef) {
			return false
		}
		if headSHA != "" && pr.HeadSHA != "" && !strings.EqualFold(pr.HeadSHA, headSHA) {
			return false
		}
		return true
	}

	return headSHA != "" && pr.HeadSHA != "" && strings.EqualFold(pr.HeadSHA, headSHA)
}

func refEqual(a, b string) bool {
	return normalizeRef(a) == normalizeRef(b)
}

func normalizeRef(ref string) string {
	ref = strings.TrimSpace(ref)
	for _, prefix := range []string{"refs/heads/", "origin/"} {
		ref = strings.TrimPrefix(ref, prefix)
	}
	return strings.ToLower(ref)
}

func githubPRSummary(pr githubPRCandidate) string {
	return fmt.Sprintf("#%d %s", pr.Number, pr.URL)
}
