package core

import (
	"testing"
	"time"
)

func TestReviewContextKeyStableAcrossSessionFields(t *testing.T) {
	ctx := sampleProviderReviewContext()
	key := NewReviewContextKey("plugin:github#review_provider:github", ctx)

	ctx.Session.LocalReviewID = "different"
	ctx.Session.IdempotencyKey = "different-key"
	ctx.Session.CreatedAt = time.Now().Add(24 * time.Hour)

	if got := NewReviewContextKey("plugin:github#review_provider:github", ctx); got != key {
		t.Fatalf("key changed for runtime session fields\nwant: %#v\n got: %#v", key, got)
	}
}

func TestReviewContextKeyIncludesIdentityInputs(t *testing.T) {
	base := sampleProviderReviewContext()
	baseKey := NewReviewContextKey("provider-a", base)

	cases := map[string]func(*ReviewContext){
		"provider": nil,
		"remote": func(c *ReviewContext) { c.Repository.Remotes[0].URL = "git@github.com:owner/other.git" },
		"mode": func(c *ReviewContext) { c.Target.Mode = DiffModeRange },
		"base ref": func(c *ReviewContext) { c.Target.BaseRef = "main" },
		"head ref": func(c *ReviewContext) { c.Target.HeadRef = "feature-2" },
		"base sha": func(c *ReviewContext) { c.Target.BaseSHA = "base2" },
		"head sha": func(c *ReviewContext) { c.Target.HeadSHA = "head2" },
		"merge base": func(c *ReviewContext) { c.Target.MergeBaseSHA = "merge2" },
	}

	for name, mutate := range cases {
		ctx := base
		provider := "provider-a"
		if mutate == nil { provider = "provider-b" } else { mutate(&ctx) }
		if got := NewReviewContextKey(provider, ctx); got == baseKey {
			t.Fatalf("%s did not change key", name)
		}
	}
}

func TestRepositoryIdentityPrefersRemotesAndFallsBackToPath(t *testing.T) {
	ctx := sampleProviderReviewContext()
	remoteID := NewReviewContextKey("provider", ctx).RepositoryIdentity

	ctx.Repository.RepoPath = "/different/path"
	ctx.Repository.WorktreeRoot = "/different/worktree"
	if got := NewReviewContextKey("provider", ctx).RepositoryIdentity; got != remoteID {
		t.Fatalf("path changed remote-backed identity: %q != %q", got, remoteID)
	}

	ctx.Repository.Remotes = nil
	got := NewReviewContextKey("provider", ctx).RepositoryIdentity
	if got == "" || got == remoteID {
		t.Fatalf("expected path fallback identity, got %q", got)
	}
}

func sampleProviderReviewContext() ReviewContext {
	return ReviewContext{
		Repository: RepositoryMetadata{RepoPath: "/repo", WorktreeRoot: "/repo", Remotes: []GitRemote{{Name: "origin", URL: "git@github.com:owner/repo.git"}}},
		Target: ReviewTargetMetadata{Mode: DiffModeBranch, BaseRef: "origin/main", HeadRef: "feature", BaseSHA: "base1", HeadSHA: "head1", MergeBaseSHA: "merge1"},
		Session: ReviewSessionMetadata{LocalReviewID: "local", IdempotencyKey: "idem", CreatedAt: time.Unix(1, 0)},
	}
}
