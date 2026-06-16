package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

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

// mapCache is a map-backed ProviderSnapshotCache for tests that need to
// hold multiple snapshots with distinct context keys.
type mapCache struct {
	mu    sync.Mutex
	snaps map[core.ReviewContextKey]core.ProviderSnapshot
}

func (m *mapCache) LoadProviderSnapshot(_ context.Context, key core.ReviewContextKey) (core.ProviderSnapshot, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.snaps[key]
	return s, ok, nil
}

func (m *mapCache) SaveProviderSnapshot(_ context.Context, s core.ProviderSnapshot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.snaps == nil {
		m.snaps = make(map[core.ReviewContextKey]core.ProviderSnapshot)
	}
	m.snaps[s.ContextKey] = s
	return nil
}

// pluginDescriptor returns a test descriptor with the given key.
func pluginDescriptor(key string) ports.ReviewProviderDescriptor {
	return ports.ReviewProviderDescriptor{
		Key: key, PluginName: "Plugin", PluginSource: "git:example.com/plugin",
		ContributionID: "plugin", Label: "Plugin", Type: "review_provider",
	}
}

const bundledCodexProviderKey = "plugin:codex"

// builtinDescriptor returns a descriptor for the bundled Codex provider.
func builtinDescriptor() ports.ReviewProviderDescriptor {
	return ports.ReviewProviderDescriptor{
		Key: bundledCodexProviderKey, PluginName: "Codex", PluginSource: "bundled:ero-plugin-codex",
		ContributionID: "codex", Label: "Codex", Type: "review_provider",
	}
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

func TestActiveProviderServiceCloseInvalidatesInFlightStart(t *testing.T) {
	review := testReviewContext()
	started := make(chan struct{})
	release := make(chan struct{})
	provider := mocks.NewMockReviewProviderClient(t)
	provider.EXPECT().Initialize(mock.Anything).Run(func(context.Context) {
		close(started)
		<-release
	}).Return(core.ReviewProviderInfo{ID: "github", Capabilities: core.ReviewProviderCapabilities{LoadRemoteComments: true}}, nil).Once()
	provider.EXPECT().DetectContext(mock.Anything, mock.Anything).Return(core.DetectionResult{Applicable: true}, nil).Once()
	provider.EXPECT().Close().Return(nil).Once()
	factory := mocks.NewMockReviewProviderClientFactory(t)
	expectFactoryClient(factory, "github", provider)
	svc := NewActiveProviderService(mockCatalog(t, ports.ReviewProviderDescriptor{Key: "github", Type: "github"}), factory, nil, nil, ProviderPollingConfig{})

	var wg sync.WaitGroup
	var startState ActiveProviderState
	var startErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		startState, startErr = svc.Start(context.Background(), review)
	}()
	<-started
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	close(release)
	wg.Wait()

	if startErr != nil {
		t.Fatalf("stale start should be discarded without surfacing old error: %v", startErr)
	}
	if startState.StableProviderKey != "" {
		t.Fatalf("stale start returned active provider state: %#v", startState)
	}
	if state := svc.State(); state.StableProviderKey != "" {
		t.Fatalf("close should win over stale start, got %#v", state)
	}
}

func TestActiveProviderServiceMissingSwitchDescriptorClearsOldProviderState(t *testing.T) {
	review := testReviewContext()
	provider := mocks.NewMockReviewProviderClient(t)
	expectProbe(provider, "github", true)
	provider.EXPECT().Close().Return(nil).Once()
	factory := mocks.NewMockReviewProviderClientFactory(t)
	expectFactoryClient(factory, "github", provider)
	svc := NewActiveProviderService(mockCatalog(t, ports.ReviewProviderDescriptor{Key: "github", Type: "github"}), factory, nil, nil, ProviderPollingConfig{})
	if _, err := svc.Start(context.Background(), review); err != nil {
		t.Fatal(err)
	}

	st, err := svc.Switch(context.Background(), review, "missing")

	if got := core.ClassifyProviderError(err); got != core.ProviderErrorNotApplicable {
		t.Fatalf("expected not applicable, got %q (%v)", got, err)
	}
	if st.StableProviderKey != "" || st.LastError == nil {
		t.Fatalf("missing descriptor should return failed empty state, got %#v", st)
	}
	if state := svc.State(); state.StableProviderKey != "" || state.LastError == nil {
		t.Fatalf("missing descriptor should clear service state, got %#v", state)
	}
}

func TestActiveProviderServiceStartWithoutCandidatesReturnsNotApplicable(t *testing.T) {
	svc := NewActiveProviderService(mockCatalog(t), mocks.NewMockReviewProviderClientFactory(t), nil, nil, ProviderPollingConfig{})

	st, err := svc.Start(context.Background(), testReviewContext())

	if got := core.ClassifyProviderError(err); got != core.ProviderErrorNotApplicable {
		t.Fatalf("expected not applicable error, got %q state %#v error %v", got, st, err)
	}
}

func TestActiveProviderServiceCloseInvalidatesInFlightRefresh(t *testing.T) {
	review := testReviewContext()
	started := make(chan struct{})
	release := make(chan struct{})
	provider := mocks.NewMockReviewProviderClient(t)
	expectProbe(provider, "github", true)
	provider.EXPECT().LoadRemoteThreads(mock.Anything, mock.Anything).Run(func(context.Context, core.ReviewContext) {
		close(started)
		<-release
	}).Return([]core.RemoteReviewThread{{ExternalID: "stale"}}, nil).Once()
	provider.EXPECT().Close().Return(nil).Once()
	factory := mocks.NewMockReviewProviderClientFactory(t)
	expectFactoryClient(factory, "github", provider)
	svc := NewActiveProviderService(mockCatalog(t, ports.ReviewProviderDescriptor{Key: "github", Type: "github"}), factory, nil, nil, ProviderPollingConfig{})
	if _, err := svc.Start(context.Background(), review); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	var refreshState ActiveProviderState
	var refreshErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		refreshState, refreshErr = svc.Refresh(context.Background(), review, false)
	}()
	<-started
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	close(release)
	wg.Wait()

	if refreshErr != nil {
		t.Fatalf("stale refresh should be discarded without surfacing old error: %v", refreshErr)
	}
	if refreshState.StableProviderKey != "" || len(refreshState.Snapshot.Threads) != 0 {
		t.Fatalf("stale refresh returned closed provider state: %#v", refreshState)
	}
	if state := svc.State(); state.StableProviderKey != "" || len(state.Snapshot.Threads) != 0 {
		t.Fatalf("close should clear service state, got %#v", state)
	}
}

func TestActiveProviderServiceAutomaticFallbackDoesNotOverwriteStoredPreference(t *testing.T) {
	bad := mocks.NewMockReviewProviderClient(t)
	expectProbe(bad, "bad", false)
	bad.EXPECT().Close().Return(nil).Once()
	good := mocks.NewMockReviewProviderClient(t)
	expectProbe(good, "good", true)
	factory := mocks.NewMockReviewProviderClientFactory(t)
	expectFactoryClient(factory, "preferred", bad)
	expectFactoryClient(factory, "github", good)
	prefs := &memPrefs{key: "preferred", ok: true}
	svc := NewActiveProviderService(mockCatalog(t, ports.ReviewProviderDescriptor{Key: "preferred"}, ports.ReviewProviderDescriptor{Key: "github", Type: "github"}), factory, nil, prefs, ProviderPollingConfig{})

	st, err := svc.Start(context.Background(), testReviewContext())
	if err != nil {
		t.Fatal(err)
	}
	if st.StableProviderKey != "github" {
		t.Fatalf("got %q", st.StableProviderKey)
	}
	if prefs.key != "preferred" {
		t.Fatalf("automatic fallback overwrote explicit preference with %q", prefs.key)
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

func TestActiveProviderServicePrefersPluginOverBuiltinInCatalogOrder(t *testing.T) {
	review := testReviewContext()
	// The plugin key contains "github", so plausibleGitHub matches it first.
	// The bundled Codex descriptor is in the catalog but never probed because
	// the plugin succeeds on first probe.
	pluginProv := mocks.NewMockReviewProviderClient(t)
	expectProbe(pluginProv, "plugin:github", true)
	factory := mocks.NewMockReviewProviderClientFactory(t)
	expectFactoryClient(factory, "plugin:github", pluginProv)

	svc := NewActiveProviderService(
		mockCatalog(t, pluginDescriptor("plugin:github"), builtinDescriptor()),
		factory, nil, nil, ProviderPollingConfig{},
	)
	st, err := svc.Start(context.Background(), review)
	require.NoError(t, err)
	assert.Equal(t, "plugin:github", st.StableProviderKey)
}

func TestActiveProviderServiceMixedBuiltinAndPluginPreferenceIsolation(t *testing.T) {
	review := testReviewContext()
	descs := []ports.ReviewProviderDescriptor{pluginDescriptor("plugin:github"), builtinDescriptor()}
	repoIdentity := core.RepositoryIdentity(review.Repository)

	t.Run("builtin preference is respected and saved", func(t *testing.T) {
		builtinProv := mocks.NewMockReviewProviderClient(t)
		expectProbe(builtinProv, bundledCodexProviderKey, true)
		factory := mocks.NewMockReviewProviderClientFactory(t)
		expectFactoryClient(factory, bundledCodexProviderKey, builtinProv)
		prefs := &memPrefs{key: bundledCodexProviderKey, ok: true}
		svc := NewActiveProviderService(mockCatalog(t, descs...), factory, nil, prefs, ProviderPollingConfig{})
		st, err := svc.Start(context.Background(), review)
		require.NoError(t, err)
		assert.Equal(t, bundledCodexProviderKey, st.StableProviderKey)
		saved, _, _ := prefs.LoadActiveProviderKey(context.Background(), repoIdentity)
		assert.Equal(t, bundledCodexProviderKey, saved)
	})

	t.Run("plugin preference is respected and saved", func(t *testing.T) {
		pluginProv := mocks.NewMockReviewProviderClient(t)
		expectProbe(pluginProv, "plugin:github", true)
		factory := mocks.NewMockReviewProviderClientFactory(t)
		expectFactoryClient(factory, "plugin:github", pluginProv)
		prefs := &memPrefs{key: "plugin:github", ok: true}
		svc := NewActiveProviderService(mockCatalog(t, descs...), factory, nil, prefs, ProviderPollingConfig{})
		st, err := svc.Start(context.Background(), review)
		require.NoError(t, err)
		assert.Equal(t, "plugin:github", st.StableProviderKey)
		saved, _, _ := prefs.LoadActiveProviderKey(context.Background(), repoIdentity)
		assert.Equal(t, "plugin:github", saved)
	})

	t.Run("auto-fallback from plugin to builtin preserves plugin preference", func(t *testing.T) {
		pluginProv := mocks.NewMockReviewProviderClient(t)
		expectProbe(pluginProv, "plugin:github", false)
		pluginProv.EXPECT().Close().Return(nil).Once()
		builtinProv := mocks.NewMockReviewProviderClient(t)
		expectProbe(builtinProv, bundledCodexProviderKey, true)
		factory := mocks.NewMockReviewProviderClientFactory(t)
		expectFactoryClient(factory, "plugin:github", pluginProv)
		expectFactoryClient(factory, bundledCodexProviderKey, builtinProv)
		prefs := &memPrefs{key: "plugin:github", ok: true}
		svc := NewActiveProviderService(mockCatalog(t, descs...), factory, nil, prefs, ProviderPollingConfig{})
		st, err := svc.Start(context.Background(), review)
		require.NoError(t, err)
		assert.Equal(t, bundledCodexProviderKey, st.StableProviderKey, "should fall back to builtin")
		saved, _, _ := prefs.LoadActiveProviderKey(context.Background(), repoIdentity)
		assert.Equal(t, "plugin:github", saved, "auto-fallback must not overwrite explicit plugin preference")
	})

	t.Run("auto-fallback from builtin to plugin preserves builtin preference", func(t *testing.T) {
		builtinProv := mocks.NewMockReviewProviderClient(t)
		expectProbe(builtinProv, bundledCodexProviderKey, false)
		builtinProv.EXPECT().Close().Return(nil).Once()
		pluginProv := mocks.NewMockReviewProviderClient(t)
		expectProbe(pluginProv, "plugin:github", true)
		factory := mocks.NewMockReviewProviderClientFactory(t)
		expectFactoryClient(factory, bundledCodexProviderKey, builtinProv)
		expectFactoryClient(factory, "plugin:github", pluginProv)
		prefs := &memPrefs{key: bundledCodexProviderKey, ok: true}
		svc := NewActiveProviderService(mockCatalog(t, descs...), factory, nil, prefs, ProviderPollingConfig{})
		st, err := svc.Start(context.Background(), review)
		require.NoError(t, err)
		assert.Equal(t, "plugin:github", st.StableProviderKey, "should fall back to plugin")
		saved, _, _ := prefs.LoadActiveProviderKey(context.Background(), repoIdentity)
		assert.Equal(t, bundledCodexProviderKey, saved, "auto-fallback must not overwrite explicit builtin preference")
	})
}

func TestActiveProviderServiceCacheIsolationBetweenBuiltinAndPlugin(t *testing.T) {
	review := testReviewContext()
	pluginKey := "plugin:github"
	builtinKey := bundledCodexProviderKey

	cache := &mapCache{}
	_ = cache.SaveProviderSnapshot(context.Background(), core.ProviderSnapshot{
		StableProviderKey: builtinKey,
		ContextKey:        core.NewReviewContextKey(builtinKey, review),
		Threads:           []core.RemoteReviewThread{{ExternalID: "builtin-thread"}},
		FetchedAt:         time.Now().UTC(),
	})
	_ = cache.SaveProviderSnapshot(context.Background(), core.ProviderSnapshot{
		StableProviderKey: pluginKey,
		ContextKey:        core.NewReviewContextKey(pluginKey, review),
		Threads:           []core.RemoteReviewThread{{ExternalID: "plugin-thread"}},
		FetchedAt:         time.Now().UTC(),
	})

	// Both subtests activate the provider via preference so the other
	// provider's factory/client expectations are never reached.
	// Each provider uses its own cache key (stable provider key is part of
	// ReviewContextKey), so they never collide.

	t.Run("builtin loads its own cache entry", func(t *testing.T) {
		builtinProv := mocks.NewMockReviewProviderClient(t)
		expectProbe(builtinProv, builtinKey, true)
		factory := mocks.NewMockReviewProviderClientFactory(t)
		expectFactoryClient(factory, builtinKey, builtinProv)
		prefs := &memPrefs{key: builtinKey, ok: true}
		svc := NewActiveProviderService(
			mockCatalog(t, pluginDescriptor(pluginKey), builtinDescriptor()),
			factory, cache, prefs, ProviderPollingConfig{},
		)
		st, err := svc.Start(context.Background(), review)
		require.NoError(t, err)
		assert.True(t, st.FromCache)
		assert.Equal(t, builtinKey, st.StableProviderKey)
		require.Len(t, st.Snapshot.Threads, 1)
		assert.Equal(t, "builtin-thread", st.Snapshot.Threads[0].ExternalID)
	})

	t.Run("plugin loads its own cache entry", func(t *testing.T) {
		pluginProv := mocks.NewMockReviewProviderClient(t)
		expectProbe(pluginProv, pluginKey, true)
		factory := mocks.NewMockReviewProviderClientFactory(t)
		expectFactoryClient(factory, pluginKey, pluginProv)
		prefs := &memPrefs{key: pluginKey, ok: true}
		svc := NewActiveProviderService(
			mockCatalog(t, pluginDescriptor(pluginKey), builtinDescriptor()),
			factory, cache, prefs, ProviderPollingConfig{},
		)
		st, err := svc.Start(context.Background(), review)
		require.NoError(t, err)
		assert.True(t, st.FromCache)
		assert.Equal(t, pluginKey, st.StableProviderKey)
		require.Len(t, st.Snapshot.Threads, 1)
		assert.Equal(t, "plugin-thread", st.Snapshot.Threads[0].ExternalID)
	})
}

func TestActiveProviderServiceSwitchBetweenBuiltinAndPluginUpdatesPreference(t *testing.T) {
	review := testReviewContext()
	repoIdentity := core.RepositoryIdentity(review.Repository)
	descs := []ports.ReviewProviderDescriptor{pluginDescriptor("plugin:github"), builtinDescriptor()}

	t.Run("switch from plugin to builtin", func(t *testing.T) {
		pluginProv := mocks.NewMockReviewProviderClient(t)
		expectProbe(pluginProv, "plugin:github", true)
		pluginProv.EXPECT().Close().Return(nil).Once()
		builtinProv := mocks.NewMockReviewProviderClient(t)
		expectProbe(builtinProv, bundledCodexProviderKey, true)
		factory := mocks.NewMockReviewProviderClientFactory(t)
		expectFactoryClient(factory, "plugin:github", pluginProv)
		expectFactoryClient(factory, bundledCodexProviderKey, builtinProv)
		prefs := &memPrefs{key: "plugin:github", ok: true}
		svc := NewActiveProviderService(mockCatalog(t, descs...), factory, nil, prefs, ProviderPollingConfig{})
		st, err := svc.Start(context.Background(), review)
		require.NoError(t, err)
		assert.Equal(t, "plugin:github", st.StableProviderKey)

		st2, err := svc.Switch(context.Background(), review, bundledCodexProviderKey)
		require.NoError(t, err)
		assert.Equal(t, bundledCodexProviderKey, st2.StableProviderKey)
		saved, _, _ := prefs.LoadActiveProviderKey(context.Background(), repoIdentity)
		assert.Equal(t, bundledCodexProviderKey, saved, "Switch must update preference to builtin")
	})

	t.Run("switch from builtin to plugin", func(t *testing.T) {
		builtinProv := mocks.NewMockReviewProviderClient(t)
		expectProbe(builtinProv, bundledCodexProviderKey, true)
		builtinProv.EXPECT().Close().Return(nil).Once()
		pluginProv := mocks.NewMockReviewProviderClient(t)
		expectProbe(pluginProv, "plugin:github", true)
		factory := mocks.NewMockReviewProviderClientFactory(t)
		expectFactoryClient(factory, bundledCodexProviderKey, builtinProv)
		expectFactoryClient(factory, "plugin:github", pluginProv)
		prefs := &memPrefs{key: bundledCodexProviderKey, ok: true}
		svc := NewActiveProviderService(mockCatalog(t, descs...), factory, nil, prefs, ProviderPollingConfig{})
		st, err := svc.Start(context.Background(), review)
		require.NoError(t, err)
		assert.Equal(t, bundledCodexProviderKey, st.StableProviderKey)

		st2, err := svc.Switch(context.Background(), review, "plugin:github")
		require.NoError(t, err)
		assert.Equal(t, "plugin:github", st2.StableProviderKey)
		saved, _, _ := prefs.LoadActiveProviderKey(context.Background(), repoIdentity)
		assert.Equal(t, "plugin:github", saved, "Switch must update preference to plugin")
	})
}
