package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"ero/internal/core"
	"ero/internal/ports"
)

type memCatalog []ports.ReviewProviderDescriptor

func (m memCatalog) ListReviewProviderDescriptors(context.Context) ([]ports.ReviewProviderDescriptor, error) {
	return []ports.ReviewProviderDescriptor(m), nil
}

type memFactory struct {
	clients      map[string]*fakeProvider
	made         []string
	beforeCreate func(string)
}

func (f *memFactory) CreateReviewProviderClient(_ context.Context, d ports.ReviewProviderDescriptor) (ports.ReviewProviderClient, error) {
	if f.beforeCreate != nil {
		f.beforeCreate(d.Key)
	}
	f.made = append(f.made, d.Key)
	return f.clients[d.Key], nil
}

type fakeProvider struct {
	id                   string
	applicable           bool
	detectErr, errorLoad error
	closed               int
	threads              []core.RemoteReviewThread
}

func (f *fakeProvider) Initialize(context.Context) (core.ReviewProviderInfo, error) {
	return core.ReviewProviderInfo{ID: f.id, Capabilities: core.ReviewProviderCapabilities{LoadRemoteComments: true}}, nil
}
func (f *fakeProvider) DetectContext(context.Context, core.ReviewContext) (core.DetectionResult, error) {
	if f.detectErr != nil {
		return core.DetectionResult{}, f.detectErr
	}
	return core.DetectionResult{Applicable: f.applicable, Reason: "nope"}, nil
}
func (f *fakeProvider) LoadRemoteThreads(context.Context, core.ReviewContext) ([]core.RemoteReviewThread, error) {
	if f.errorLoad != nil {
		return nil, f.errorLoad
	}
	return f.threads, nil
}
func (f *fakeProvider) PublishReview(context.Context, core.PublishReviewRequest) (core.PublishReviewResult, error) {
	return core.PublishReviewResult{}, nil
}
func (f *fakeProvider) Close() error { f.closed++; return nil }

type memCache struct {
	snap core.ProviderSnapshot
	ok   bool
}

func (m *memCache) LoadProviderSnapshot(context.Context, core.ReviewContextKey) (core.ProviderSnapshot, bool, error) {
	return m.snap, m.ok, nil
}
func (m *memCache) SaveProviderSnapshot(_ context.Context, s core.ProviderSnapshot) error {
	m.snap = s
	m.ok = true
	return nil
}

type memPrefs struct {
	key string
	ok  bool
}

func (m *memPrefs) LoadActiveProviderKey(context.Context, string) (string, bool, error) {
	return m.key, m.ok, nil
}
func (m *memPrefs) SaveActiveProviderKey(_ context.Context, _ string, k string) error {
	m.key = k
	m.ok = true
	return nil
}

func testReviewContext() core.ReviewContext {
	return core.ReviewContext{Repository: core.RepositoryMetadata{Remotes: []core.GitRemote{{URL: "https://github.com/acme/repo.git"}}}, Target: core.ReviewTargetMetadata{Mode: core.DiffModeWorking}}
}

func TestActiveProviderServicePreferenceFallbackAndClosesFailedClients(t *testing.T) {
	bad := &fakeProvider{id: "bad", applicable: false}
	good := &fakeProvider{id: "good", applicable: true}
	fac := &memFactory{clients: map[string]*fakeProvider{"preferred": bad, "github": good}}
	svc := NewActiveProviderService(memCatalog{{Key: "preferred"}, {Key: "github", Type: "github"}}, fac, nil, &memPrefs{key: "preferred", ok: true}, ProviderPollingConfig{})
	st, err := svc.Start(context.Background(), testReviewContext())
	if err != nil {
		t.Fatal(err)
	}
	if st.StableProviderKey != "github" {
		t.Fatalf("got %q", st.StableProviderKey)
	}
	if bad.closed != 1 {
		t.Fatalf("failed client not closed")
	}
	if good.closed != 0 {
		t.Fatalf("active client was closed")
	}
}

func TestActiveProviderServiceCacheFirstRefreshPreservesCacheOnRetryableFailure(t *testing.T) {
	review := testReviewContext()
	key := core.NewReviewContextKey("github", review)
	cached := core.ProviderSnapshot{StableProviderKey: "github", ContextKey: key, Threads: []core.RemoteReviewThread{{ExternalID: "old"}}}
	cache := &memCache{snap: cached, ok: true}
	p := &fakeProvider{id: "rt", applicable: true, errorLoad: core.NewProviderError(core.ProviderErrorTransientNetwork, "offline", errors.New("dial"))}
	svc := NewActiveProviderService(memCatalog{{Key: "github", Type: "github"}}, &memFactory{clients: map[string]*fakeProvider{"github": p}}, cache, nil, ProviderPollingConfig{Interval: time.Minute, MinBackoff: time.Second, MaxBackoff: time.Second})
	st, err := svc.Start(context.Background(), review)
	if err != nil {
		t.Fatal(err)
	}
	if !st.FromCache || len(st.Snapshot.Threads) != 1 {
		t.Fatalf("expected cached snapshot first")
	}
	st, err = svc.Refresh(context.Background(), review, false)
	if err == nil {
		t.Fatal("expected refresh error")
	}
	if len(st.Snapshot.Threads) != 1 || st.Snapshot.Threads[0].ExternalID != "old" {
		t.Fatalf("cache not preserved")
	}
	if st.NextSyncAt.IsZero() {
		t.Fatalf("retryable error should set next sync")
	}
	if st.Snapshot.Sync.Status != core.ProviderSyncStatusBackingOff {
		t.Fatalf("retryable error status = %q, want %q", st.Snapshot.Sync.Status, core.ProviderSyncStatusBackingOff)
	}
}

func TestActiveProviderServiceSwitchGenerationIgnoresStaleTimer(t *testing.T) {
	review := testReviewContext()
	a := &fakeProvider{id: "a", applicable: true}
	b := &fakeProvider{id: "b", applicable: true}
	svc := NewActiveProviderService(memCatalog{{Key: "a"}, {Key: "b"}}, &memFactory{clients: map[string]*fakeProvider{"a": a, "b": b}}, nil, nil, ProviderPollingConfig{})
	st, err := svc.Start(context.Background(), review)
	if err != nil {
		t.Fatal(err)
	}
	target := "b"
	oldProvider := a
	if st.StableProviderKey == "b" {
		target = "a"
		oldProvider = b
	}
	old := svc.Generation()
	if _, err := svc.Switch(context.Background(), review, target); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CompleteTimer(context.Background(), review, old); err != nil {
		t.Fatal(err)
	}
	if got := svc.State().StableProviderKey; got != target {
		t.Fatalf("stale timer changed state to %q", got)
	}
	if oldProvider.closed != 1 {
		t.Fatalf("switch should close old client")
	}
}

func TestActiveProviderServiceSwitchClosesCurrentBeforeStartingTarget(t *testing.T) {
	review := testReviewContext()
	a := &fakeProvider{id: "a", applicable: true}
	b := &fakeProvider{id: "b", applicable: true}
	factory := &memFactory{clients: map[string]*fakeProvider{"a": a, "b": b}}
	svc := NewActiveProviderService(memCatalog{{Key: "a"}, {Key: "b"}}, factory, nil, nil, ProviderPollingConfig{})
	if _, err := svc.Start(context.Background(), review); err != nil {
		t.Fatal(err)
	}
	factory.beforeCreate = func(key string) {
		if key == "b" && a.closed != 1 {
			t.Fatalf("current provider was still live when target provider started")
		}
	}
	if _, err := svc.Switch(context.Background(), review, "b"); err != nil {
		t.Fatal(err)
	}
}

func TestActiveProviderServiceFailedSwitchClearsOldProviderState(t *testing.T) {
	review := testReviewContext()
	a := &fakeProvider{id: "a", applicable: true, threads: []core.RemoteReviewThread{{ExternalID: "old"}}}
	b := &fakeProvider{id: "b", applicable: false}
	svc := NewActiveProviderService(memCatalog{{Key: "a"}, {Key: "b"}}, &memFactory{clients: map[string]*fakeProvider{"a": a, "b": b}}, nil, nil, ProviderPollingConfig{})
	if _, err := svc.Start(context.Background(), review); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(context.Background(), review, false); err != nil {
		t.Fatal(err)
	}
	if state := svc.State(); state.StableProviderKey != "a" || len(state.Snapshot.Threads) != 1 {
		t.Fatalf("expected old active provider state before switch, got %#v", state)
	}

	st, err := svc.Switch(context.Background(), review, "b")
	if err == nil {
		t.Fatal("expected switch failure")
	}
	if st.StableProviderKey != "" || len(st.Snapshot.Threads) != 0 {
		t.Fatalf("failed switch returned old provider state: %#v", st)
	}
	if state := svc.State(); state.StableProviderKey != "" || len(state.Snapshot.Threads) != 0 || state.LastError == nil {
		t.Fatalf("failed switch left incoherent service state: %#v", state)
	}
	if a.closed != 1 {
		t.Fatalf("old provider should be closed, got %d", a.closed)
	}
}

func TestActiveProviderServiceUsesStableCatalogOrder(t *testing.T) {
	review := testReviewContext()
	fac := &memFactory{clients: map[string]*fakeProvider{
		"first":  {id: "first", applicable: true},
		"github": {id: "github", applicable: true},
		"second": {id: "second", applicable: true},
	}}
	svc := NewActiveProviderService(memCatalog{{Key: "first"}, {Key: "github", ContributionID: "github"}, {Key: "second"}}, fac, nil, nil, ProviderPollingConfig{})
	if _, err := svc.Start(context.Background(), review); err != nil {
		t.Fatal(err)
	}
	if got := fac.made; len(got) != 1 || got[0] != "github" {
		t.Fatalf("github fallback should be selected in catalog order, got %v", got)
	}

	fac = &memFactory{clients: map[string]*fakeProvider{
		"z": {id: "z", applicable: false},
		"a": {id: "a", applicable: true},
	}}
	svc = NewActiveProviderService(memCatalog{{Key: "z"}, {Key: "a"}}, fac, nil, nil, ProviderPollingConfig{})
	if _, err := svc.Start(context.Background(), review); err != nil {
		t.Fatal(err)
	}
	if got := fac.made; len(got) != 2 || got[0] != "z" || got[1] != "a" {
		t.Fatalf("remaining candidates should preserve catalog order, got %v", got)
	}
}
