package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	"ero/internal/core"
	"ero/internal/ports"
	"ero/internal/ports/mocks"
)

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

func mockCatalog(t *testing.T, descriptors ...ports.ReviewProviderDescriptor) *mocks.MockReviewProviderCatalog {
	catalog := mocks.NewMockReviewProviderCatalog(t)
	catalog.EXPECT().ListReviewProviderDescriptors(mock.Anything).Return(descriptors, nil)
	return catalog
}

func expectFactoryClient(factory *mocks.MockReviewProviderClientFactory, key string, client ports.ReviewProviderClient) {
	factory.EXPECT().CreateReviewProviderClient(mock.Anything, mock.MatchedBy(func(d ports.ReviewProviderDescriptor) bool { return d.Key == key })).Return(client, nil).Once()
}

func expectProbe(provider *mocks.MockReviewProviderClient, id string, applicable bool) {
	provider.EXPECT().Initialize(mock.Anything).Return(core.ReviewProviderInfo{ID: id, Capabilities: core.ReviewProviderCapabilities{LoadRemoteComments: true}}, nil).Once()
	provider.EXPECT().DetectContext(mock.Anything, mock.Anything).Return(core.DetectionResult{Applicable: applicable, Reason: "nope"}, nil).Once()
}

func TestActiveProviderServicePreferenceFallbackAndClosesFailedClients(t *testing.T) {
	bad := mocks.NewMockReviewProviderClient(t)
	expectProbe(bad, "bad", false)
	bad.EXPECT().Close().Return(nil).Once()
	good := mocks.NewMockReviewProviderClient(t)
	expectProbe(good, "good", true)
	factory := mocks.NewMockReviewProviderClientFactory(t)
	expectFactoryClient(factory, "preferred", bad)
	expectFactoryClient(factory, "github", good)
	svc := NewActiveProviderService(mockCatalog(t, ports.ReviewProviderDescriptor{Key: "preferred"}, ports.ReviewProviderDescriptor{Key: "github", Type: "github"}), factory, nil, &memPrefs{key: "preferred", ok: true}, ProviderPollingConfig{})
	st, err := svc.Start(context.Background(), testReviewContext())
	if err != nil {
		t.Fatal(err)
	}
	if st.StableProviderKey != "github" {
		t.Fatalf("got %q", st.StableProviderKey)
	}
}

func TestActiveProviderServiceCacheFirstRefreshPreservesCacheOnRetryableFailure(t *testing.T) {
	review := testReviewContext()
	key := core.NewReviewContextKey("github", review)
	cached := core.ProviderSnapshot{StableProviderKey: "github", ContextKey: key, Threads: []core.RemoteReviewThread{{ExternalID: "old"}}}
	cache := &memCache{snap: cached, ok: true}
	provider := mocks.NewMockReviewProviderClient(t)
	expectProbe(provider, "rt", true)
	provider.EXPECT().LoadRemoteThreads(mock.Anything, mock.Anything).Return(nil, core.NewProviderError(core.ProviderErrorTransientNetwork, "offline", errors.New("dial"))).Once()
	factory := mocks.NewMockReviewProviderClientFactory(t)
	expectFactoryClient(factory, "github", provider)
	svc := NewActiveProviderService(mockCatalog(t, ports.ReviewProviderDescriptor{Key: "github", Type: "github"}), factory, cache, nil, ProviderPollingConfig{Interval: time.Minute, MinBackoff: time.Second, MaxBackoff: time.Second})
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
	a := mocks.NewMockReviewProviderClient(t)
	expectProbe(a, "a", true)
	aClosed := false
	a.EXPECT().Close().Run(func() { aClosed = true }).Return(nil).Once()
	b := mocks.NewMockReviewProviderClient(t)
	expectProbe(b, "b", true)
	factory := mocks.NewMockReviewProviderClientFactory(t)
	expectFactoryClient(factory, "a", a)
	expectFactoryClient(factory, "b", b)
	svc := NewActiveProviderService(mockCatalog(t, ports.ReviewProviderDescriptor{Key: "a"}, ports.ReviewProviderDescriptor{Key: "b"}), factory, nil, nil, ProviderPollingConfig{})
	st, err := svc.Start(context.Background(), review)
	if err != nil {
		t.Fatal(err)
	}
	target := "b"
	if st.StableProviderKey == "b" {
		t.Fatal("expected deterministic initial provider a")
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
	if !aClosed {
		t.Fatalf("switch should close old client")
	}
}

func TestActiveProviderServiceSwitchClosesCurrentBeforeStartingTarget(t *testing.T) {
	review := testReviewContext()
	a := mocks.NewMockReviewProviderClient(t)
	expectProbe(a, "a", true)
	aClosed := false
	a.EXPECT().Close().Run(func() { aClosed = true }).Return(nil).Once()
	b := mocks.NewMockReviewProviderClient(t)
	expectProbe(b, "b", true)
	factory := mocks.NewMockReviewProviderClientFactory(t)
	expectFactoryClient(factory, "a", a)
	factory.EXPECT().CreateReviewProviderClient(mock.Anything, mock.MatchedBy(func(d ports.ReviewProviderDescriptor) bool { return d.Key == "b" })).Run(func(_ context.Context, _ ports.ReviewProviderDescriptor) {
		if !aClosed {
			t.Fatalf("current provider was still live when target provider started")
		}
	}).Return(b, nil).Once()
	svc := NewActiveProviderService(mockCatalog(t, ports.ReviewProviderDescriptor{Key: "a"}, ports.ReviewProviderDescriptor{Key: "b"}), factory, nil, nil, ProviderPollingConfig{})
	if _, err := svc.Start(context.Background(), review); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Switch(context.Background(), review, "b"); err != nil {
		t.Fatal(err)
	}
}

func TestActiveProviderServiceFailedSwitchClearsOldProviderState(t *testing.T) {
	review := testReviewContext()
	a := mocks.NewMockReviewProviderClient(t)
	expectProbe(a, "a", true)
	a.EXPECT().LoadRemoteThreads(mock.Anything, mock.Anything).Return([]core.RemoteReviewThread{{ExternalID: "old"}}, nil).Once()
	a.EXPECT().Close().Return(nil).Once()
	b := mocks.NewMockReviewProviderClient(t)
	expectProbe(b, "b", false)
	b.EXPECT().Close().Return(nil).Once()
	factory := mocks.NewMockReviewProviderClientFactory(t)
	expectFactoryClient(factory, "a", a)
	expectFactoryClient(factory, "b", b)
	svc := NewActiveProviderService(mockCatalog(t, ports.ReviewProviderDescriptor{Key: "a"}, ports.ReviewProviderDescriptor{Key: "b"}), factory, nil, nil, ProviderPollingConfig{})
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
}

func TestActiveProviderServiceUsesStableCatalogOrder(t *testing.T) {
	review := testReviewContext()
	github := mocks.NewMockReviewProviderClient(t)
	expectProbe(github, "github", true)
	factory := mocks.NewMockReviewProviderClientFactory(t)
	made := []string{}
	factory.EXPECT().CreateReviewProviderClient(mock.Anything, mock.MatchedBy(func(d ports.ReviewProviderDescriptor) bool { return d.Key == "github" })).Run(func(_ context.Context, d ports.ReviewProviderDescriptor) { made = append(made, d.Key) }).Return(github, nil).Once()
	svc := NewActiveProviderService(mockCatalog(t, ports.ReviewProviderDescriptor{Key: "first"}, ports.ReviewProviderDescriptor{Key: "github", ContributionID: "github"}, ports.ReviewProviderDescriptor{Key: "second"}), factory, nil, nil, ProviderPollingConfig{})
	if _, err := svc.Start(context.Background(), review); err != nil {
		t.Fatal(err)
	}
	if got := made; len(got) != 1 || got[0] != "github" {
		t.Fatalf("github fallback should be selected in catalog order, got %v", got)
	}

	z := mocks.NewMockReviewProviderClient(t)
	expectProbe(z, "z", false)
	z.EXPECT().Close().Return(nil).Once()
	a := mocks.NewMockReviewProviderClient(t)
	expectProbe(a, "a", true)
	factory = mocks.NewMockReviewProviderClientFactory(t)
	made = []string{}
	factory.EXPECT().CreateReviewProviderClient(mock.Anything, mock.MatchedBy(func(d ports.ReviewProviderDescriptor) bool { return d.Key == "z" })).Run(func(_ context.Context, d ports.ReviewProviderDescriptor) { made = append(made, d.Key) }).Return(z, nil).Once()
	factory.EXPECT().CreateReviewProviderClient(mock.Anything, mock.MatchedBy(func(d ports.ReviewProviderDescriptor) bool { return d.Key == "a" })).Run(func(_ context.Context, d ports.ReviewProviderDescriptor) { made = append(made, d.Key) }).Return(a, nil).Once()
	svc = NewActiveProviderService(mockCatalog(t, ports.ReviewProviderDescriptor{Key: "z"}, ports.ReviewProviderDescriptor{Key: "a"}), factory, nil, nil, ProviderPollingConfig{})
	if _, err := svc.Start(context.Background(), review); err != nil {
		t.Fatal(err)
	}
	if got := made; len(got) != 2 || got[0] != "z" || got[1] != "a" {
		t.Fatalf("remaining candidates should preserve catalog order, got %v", got)
	}
}
