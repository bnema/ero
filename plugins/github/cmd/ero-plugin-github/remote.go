package main

import (
	"net/url"
	"regexp"
	"strings"

	"ero/pkg/plugin"
)

type githubRemote struct {
	Owner string
	Name  string
}

var scpGitHubRemotePattern = regexp.MustCompile(`^git@github\.com:([^/]+)/(.+)$`)

func parseGitHubRemote(raw string) (githubRemote, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return githubRemote{}, false
	}
	if m := scpGitHubRemotePattern.FindStringSubmatch(raw); m != nil {
		return cleanGitHubRepo(m[1], m[2])
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Hostname(), "github.com") {
		return githubRemote{}, false
	}
	if u.Scheme != "https" && u.Scheme != "ssh" {
		return githubRemote{}, false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 {
		return githubRemote{}, false
	}
	if u.Scheme == "ssh" && (u.User == nil || u.User.Username() != "git") {
		return githubRemote{}, false
	}
	return cleanGitHubRepo(parts[0], parts[1])
}

func cleanGitHubRepo(owner, repo string) (githubRemote, bool) {
	owner = strings.TrimSpace(owner)
	repo = strings.TrimSuffix(strings.TrimSpace(repo), "/")
	repo = strings.TrimSuffix(repo, ".git")
	if owner == "" || repo == "" || strings.Contains(owner, "/") || strings.Contains(repo, "/") || strings.Contains(owner, " ") || strings.Contains(repo, " ") {
		return githubRemote{}, false
	}
	return githubRemote{Owner: owner, Name: repo}, true
}

func githubRemotes(remotes []plugin.GitRemote) []githubRemote {
	out := make([]githubRemote, 0, len(remotes))
	seen := map[string]bool{}
	for _, remote := range remotes {
		parsed, ok := parseGitHubRemote(remote.URL)
		if !ok {
			continue
		}
		key := strings.ToLower(parsed.Owner + "/" + parsed.Name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, parsed)
	}
	return out
}
