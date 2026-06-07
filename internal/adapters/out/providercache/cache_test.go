package providercache

import (
	"context"
	"testing"
	"time"

	"ero/internal/core"
)

func TestCacheRoundTripNormalizedSnapshot(t *testing.T) {
	store := NewStore(t.TempDir(), t.TempDir())
	key := core.ReviewContextKey{StableProviderKey: "plugin:github#review_provider:github", RepositoryIdentity: "remotes:github.com/o/r", TargetMode: core.DiffModeBranch, BaseRef: "main", HeadRef: "feature", BaseSHA: "b", HeadSHA: "h"}
	snapshot := core.ProviderSnapshot{StableProviderKey: key.StableProviderKey, RuntimeProviderID: "github", ContextKey: key, Threads: []core.RemoteReviewThread{{ProviderID: "github", ExternalID: "thread-1"}}, FetchedAt: time.Unix(10, 0).UTC()}

	if err := store.SaveProviderSnapshot(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.LoadProviderSnapshot(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected cached snapshot")
	}
	if got.StableProviderKey != snapshot.StableProviderKey {
		t.Fatalf("StableProviderKey mismatch: got %q, want %q", got.StableProviderKey, snapshot.StableProviderKey)
	}
	if got.RuntimeProviderID != "github" {
		t.Fatalf("RuntimeProviderID mismatch: got %q, want %q", got.RuntimeProviderID, "github")
	}
	if len(got.Threads) != 1 {
		t.Fatalf("unexpected thread count: got %d, want 1", len(got.Threads))
	}
}

func TestPreferenceRoundTrip(t *testing.T) {
	store := NewStore(t.TempDir(), t.TempDir())
	repoID := "remotes:github.com/o/r"
	if _, ok, err := store.LoadActiveProviderKey(context.Background(), repoID); err != nil || ok {
		t.Fatalf("empty load = ok %v err %v", ok, err)
	}
	if err := store.SaveActiveProviderKey(context.Background(), repoID, "provider-key"); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.LoadActiveProviderKey(context.Background(), repoID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got != "provider-key" {
		t.Fatalf("got %q ok %v", got, ok)
	}
}
