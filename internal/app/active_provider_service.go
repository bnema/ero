package app

import (
	"context"
	"strings"
	"sync"
	"time"

	"ero/internal/core"
	"ero/internal/ports"
)

type ProviderPollingConfig struct{ Interval, MinBackoff, MaxBackoff time.Duration }

func DefaultProviderPollingConfig() ProviderPollingConfig {
	return ProviderPollingConfig{Interval: 2 * time.Minute, MinBackoff: 5 * time.Second, MaxBackoff: time.Minute}
}

type ActiveProviderState struct {
	StableProviderKey string
	RuntimeProviderID string
	RuntimeInfo       core.ReviewProviderInfo
	Snapshot          core.ProviderSnapshot
	FromCache         bool
	Syncing           bool
	LastError         error
	NextSyncAt        time.Time
}

type ActiveProviderService struct {
	catalog ports.ReviewProviderCatalog
	factory ports.ReviewProviderClientFactory
	cache   ports.ProviderSnapshotCache
	prefs   ports.ActiveProviderPreferenceStore
	poll    ProviderPollingConfig

	mu         sync.Mutex
	client     ports.ReviewProviderClient
	stableKey  string
	runtimeID  string
	generation int64
	backoff    time.Duration
	state      ActiveProviderState
}

func NewActiveProviderService(catalog ports.ReviewProviderCatalog, factory ports.ReviewProviderClientFactory, cache ports.ProviderSnapshotCache, prefs ports.ActiveProviderPreferenceStore, poll ProviderPollingConfig) *ActiveProviderService {
	if poll.Interval == 0 {
		poll = DefaultProviderPollingConfig()
	}
	if poll.MinBackoff == 0 {
		poll.MinBackoff = 5 * time.Second
	}
	if poll.MaxBackoff == 0 {
		poll.MaxBackoff = time.Minute
	}
	return &ActiveProviderService{catalog: catalog, factory: factory, cache: cache, prefs: prefs, poll: poll}
}

func (s *ActiveProviderService) Start(ctx context.Context, review core.ReviewContext) (ActiveProviderState, error) {
	descs, err := s.catalog.ListReviewProviderDescriptors(ctx)
	if err != nil {
		return ActiveProviderState{}, err
	}
	ordered := s.orderCandidates(ctx, descs, review)
	s.mu.Lock()
	s.closeLocked()
	s.stableKey = ""
	s.runtimeID = ""
	s.generation++
	startGen := s.generation
	s.state = ActiveProviderState{}
	s.mu.Unlock()
	var lastErr error
	for _, d := range ordered {
		client, info, err := s.probe(ctx, d, review)
		if err != nil {
			lastErr = err
			continue
		}
		s.mu.Lock()
		s.closeLocked()
		s.client = client
		s.stableKey = d.Key
		s.runtimeID = info.ID
		s.generation++
		gen := s.generation
		s.backoff = 0
		s.mu.Unlock()
		if s.prefs != nil {
			_ = s.prefs.SaveActiveProviderKey(ctx, core.RepositoryIdentity(review.Repository), d.Key)
		}
		st := s.loadCachedState(ctx, review, d.Key, info.ID, info)
		s.setState(gen, st)
		return st, nil
	}
	failed := failedProviderState(lastErr)
	s.setState(startGen, failed)
	return failed, lastErr
}

func (s *ActiveProviderService) Switch(ctx context.Context, review core.ReviewContext, stableKey string) (ActiveProviderState, error) {
	descs, err := s.catalog.ListReviewProviderDescriptors(ctx)
	if err != nil {
		return ActiveProviderState{}, err
	}
	for _, d := range descs {
		if d.Key == stableKey {
			s.mu.Lock()
			s.closeLocked()
			s.stableKey = ""
			s.runtimeID = ""
			s.generation++
			switchGen := s.generation
			s.state = ActiveProviderState{}
			s.mu.Unlock()
			client, info, err := s.probe(ctx, d, review)
			if err != nil {
				failed := failedProviderState(err)
				s.setState(switchGen, failed)
				return failed, err
			}
			s.mu.Lock()
			s.closeLocked()
			s.client = client
			s.stableKey = d.Key
			s.runtimeID = info.ID
			s.generation++
			gen := s.generation
			s.backoff = 0
			s.mu.Unlock()
			if s.prefs != nil {
				_ = s.prefs.SaveActiveProviderKey(ctx, core.RepositoryIdentity(review.Repository), d.Key)
			}
			st := s.loadCachedState(ctx, review, d.Key, info.ID, info)
			s.setState(gen, st)
			return st, nil
		}
	}
	return ActiveProviderState{}, core.NewProviderError(core.ProviderErrorNotApplicable, "provider descriptor not found", nil)
}

func (s *ActiveProviderService) PublishReview(ctx context.Context, request core.PublishReviewRequest) (core.PublishReviewResult, error) {
	s.mu.Lock()
	client := s.client
	runtimeID := s.runtimeID
	s.mu.Unlock()
	if client == nil {
		return core.PublishReviewResult{}, core.NewProviderError(core.ProviderErrorNotApplicable, "no active provider", nil)
	}
	if request.ProviderID == "" {
		request.ProviderID = runtimeID
	}
	return client.PublishReview(ctx, request)
}

func (s *ActiveProviderService) Refresh(ctx context.Context, review core.ReviewContext, manual bool) (ActiveProviderState, error) {
	s.mu.Lock()
	client := s.client
	key := s.stableKey
	runtimeID := s.runtimeID
	s.generation++
	gen := s.generation
	prev := s.state
	if manual {
		s.backoff = 0
	}
	s.mu.Unlock()
	if client == nil {
		return prev, core.NewProviderError(core.ProviderErrorNotApplicable, "no active provider", nil)
	}
	threads, err := client.LoadRemoteThreads(ctx, review)
	if err != nil {
		st := prev
		st.LastError = err
		st.Syncing = false
		st.Snapshot.Sync.LastError = err.Error()
		st.NextSyncAt = s.nextBackoff(err)
		if st.NextSyncAt.IsZero() {
			st.Snapshot.Sync.Status = core.ProviderSyncStatusFailed
			st.Snapshot.Sync.NextSyncAt = nil
		} else {
			st.Snapshot.Sync.Status = core.ProviderSyncStatusBackingOff
			st.Snapshot.Sync.NextSyncAt = new(st.NextSyncAt)
		}
		s.setState(gen, st)
		return st, err
	}
	now := time.Now().UTC()
	next := now.Add(s.poll.Interval)
	snap := core.ProviderSnapshot{StableProviderKey: key, RuntimeProviderID: runtimeID, ContextKey: core.NewReviewContextKey(key, review), Threads: threads, FetchedAt: now, Sync: core.ProviderSyncState{Status: core.ProviderSyncStatusSynced, LastSyncAt: new(now), NextSyncAt: new(next)}}
	if s.cache != nil {
		_ = s.cache.SaveProviderSnapshot(ctx, snap)
	}
	st := ActiveProviderState{StableProviderKey: key, RuntimeProviderID: runtimeID, RuntimeInfo: prev.RuntimeInfo, Snapshot: snap, NextSyncAt: next}
	s.setState(gen, st)
	return st, nil
}

func (s *ActiveProviderService) CompleteTimer(ctx context.Context, review core.ReviewContext, generation int64) (ActiveProviderState, error) {
	s.mu.Lock()
	cur := s.generation
	s.mu.Unlock()
	if generation != cur {
		return s.State(), nil
	}
	return s.Refresh(ctx, review, false)
}
func (s *ActiveProviderService) Generation() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.generation
}
func (s *ActiveProviderService) State() ActiveProviderState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}
func (s *ActiveProviderService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeLocked()
}

func (s *ActiveProviderService) orderCandidates(ctx context.Context, descs []ports.ReviewProviderDescriptor, review core.ReviewContext) []ports.ReviewProviderDescriptor {
	out := make([]ports.ReviewProviderDescriptor, 0, len(descs))
	used := map[string]bool{}
	preferenceFound := false
	if s.prefs != nil {
		if key, ok, _ := s.prefs.LoadActiveProviderKey(ctx, core.RepositoryIdentity(review.Repository)); ok {
			for _, d := range descs {
				if d.Key == key {
					out = append(out, d)
					used[d.Key] = true
					preferenceFound = true
					break
				}
			}
		}
	}
	if !preferenceFound {
		for _, d := range descs {
			if plausibleGitHub(review, d) {
				out = append(out, d)
				used[d.Key] = true
				break
			}
		}
	}
	for _, d := range descs {
		if !used[d.Key] {
			out = append(out, d)
		}
	}
	return out
}

func plausibleGitHub(review core.ReviewContext, d ports.ReviewProviderDescriptor) bool {
	descriptorText := strings.ToLower(strings.Join([]string{d.Key, d.PluginName, d.PluginSource, d.ContributionID, d.Label, d.Type}, " "))
	if !strings.Contains(descriptorText, "github") {
		return false
	}
	if strings.Contains(d.PluginSource, "github") || strings.Contains(strings.ToLower(d.ContributionID), "github") || strings.Contains(strings.ToLower(d.Label), "github") || strings.Contains(strings.ToLower(d.PluginName), "github") {
		return true
	}
	for _, r := range review.Repository.Remotes {
		if strings.Contains(strings.ToLower(r.URL), "github.com") {
			return true
		}
	}
	return false
}

func (s *ActiveProviderService) probe(ctx context.Context, d ports.ReviewProviderDescriptor, review core.ReviewContext) (ports.ReviewProviderClient, core.ReviewProviderInfo, error) {
	client, err := s.factory.CreateReviewProviderClient(ctx, d)
	if err != nil {
		return nil, core.ReviewProviderInfo{}, err
	}
	info, err := client.Initialize(ctx)
	if err != nil {
		_ = client.Close()
		return nil, info, err
	}
	det, err := client.DetectContext(ctx, review)
	if err != nil {
		_ = client.Close()
		return nil, info, err
	}
	if !det.Applicable {
		_ = client.Close()
		return nil, info, core.NewProviderError(core.ProviderErrorNotApplicable, det.Reason, nil)
	}
	return client, info, nil
}
func (s *ActiveProviderService) loadCachedState(ctx context.Context, review core.ReviewContext, key, runtimeID string, info ...core.ReviewProviderInfo) ActiveProviderState {
	st := ActiveProviderState{StableProviderKey: key, RuntimeProviderID: runtimeID}
	if len(info) > 0 {
		st.RuntimeInfo = info[0]
	}
	if s.cache != nil {
		if snap, ok, _ := s.cache.LoadProviderSnapshot(ctx, core.NewReviewContextKey(key, review)); ok {
			snap.Cached = true
			st.Snapshot = snap
			st.FromCache = true
			if snap.Sync.NextSyncAt != nil {
				st.NextSyncAt = *snap.Sync.NextSyncAt
			}
		}
	}
	return st
}
func (s *ActiveProviderService) setState(gen int64, st ActiveProviderState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if gen == s.generation {
		s.state = st
	}
}
func (s *ActiveProviderService) nextBackoff(err error) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !core.IsRetryableProviderError(err) {
		return time.Time{}
	}
	if s.backoff == 0 {
		s.backoff = s.poll.MinBackoff
	} else {
		s.backoff *= 2
		if s.backoff > s.poll.MaxBackoff {
			s.backoff = s.poll.MaxBackoff
		}
	}
	return time.Now().UTC().Add(s.backoff)
}
func (s *ActiveProviderService) closeLocked() error {
	if s.client == nil {
		return nil
	}
	err := s.client.Close()
	s.client = nil
	return err
}

func failedProviderState(err error) ActiveProviderState {
	st := ActiveProviderState{LastError: err}
	if err != nil {
		st.Snapshot.Sync.Status = core.ProviderSyncStatusFailed
		st.Snapshot.Sync.LastError = err.Error()
	}
	return st
}
