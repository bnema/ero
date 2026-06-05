package app

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/bnema/zerowrap"

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

type remoteSnapshotLoader interface {
	LoadRemoteSnapshot(ctx context.Context, review core.ReviewContext) (core.ProviderSnapshot, error)
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
	log := zerowrap.FromCtx(ctx)
	descs, err := s.catalog.ListReviewProviderDescriptors(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("active provider catalog load failed")
		return ActiveProviderState{}, err
	}
	ordered := s.orderCandidates(ctx, descs, review)
	log.Info().Int("descriptor_count", len(descs)).Int("candidate_count", len(ordered)).Str("repo", core.RepositoryIdentity(review.Repository)).Msg("active provider start")
	s.mu.Lock()
	if err := s.closeLocked(); err != nil {
		log.Warn().Err(err).Msg("active provider close before start failed")
	}
	s.stableKey = ""
	s.runtimeID = ""
	s.generation++
	startGen := s.generation
	s.state = ActiveProviderState{}
	s.mu.Unlock()
	var lastErr error
	for _, d := range ordered {
		log.Debug().Str("provider_key", d.Key).Str("contribution_id", d.ContributionID).Str("plugin", d.PluginName).Msg("active provider probe candidate")
		client, info, err := s.probe(ctx, d, review)
		if err != nil {
			log.Debug().Err(err).Str("provider_key", d.Key).Str("contribution_id", d.ContributionID).Msg("active provider probe failed")
			lastErr = err
			continue
		}
		s.mu.Lock()
		if err := s.closeLocked(); err != nil {
			log.Warn().Err(err).Msg("active provider close before activate failed")
		}
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
		log.Info().Str("provider_key", d.Key).Str("runtime_provider_id", info.ID).Bool("from_cache", st.FromCache).Int("remote_thread_count", len(st.Snapshot.Threads)).Msg("active provider selected")
		s.setState(gen, st)
		return st, nil
	}
	log.Warn().Err(lastErr).Msg("active provider start found no applicable provider")
	failed := failedProviderState(lastErr)
	s.setState(startGen, failed)
	return failed, lastErr
}

func (s *ActiveProviderService) Switch(ctx context.Context, review core.ReviewContext, stableKey string) (ActiveProviderState, error) {
	log := zerowrap.FromCtx(ctx)
	descs, err := s.catalog.ListReviewProviderDescriptors(ctx)
	if err != nil {
		log.Warn().Err(err).Str("provider_key", stableKey).Msg("active provider switch catalog load failed")
		return ActiveProviderState{}, err
	}
	for _, d := range descs {
		if d.Key == stableKey {
			log.Info().Str("provider_key", stableKey).Str("contribution_id", d.ContributionID).Msg("active provider switch start")
			s.mu.Lock()
			if err := s.closeLocked(); err != nil {
				log.Warn().Err(err).Str("provider_key", stableKey).Msg("active provider close before switch failed")
			}
			s.stableKey = ""
			s.runtimeID = ""
			s.generation++
			switchGen := s.generation
			s.state = ActiveProviderState{}
			s.mu.Unlock()
			client, info, err := s.probe(ctx, d, review)
			if err != nil {
				log.Warn().Err(err).Str("provider_key", stableKey).Msg("active provider switch probe failed")
				failed := failedProviderState(err)
				s.setState(switchGen, failed)
				return failed, err
			}
			s.mu.Lock()
			if err := s.closeLocked(); err != nil {
				log.Warn().Err(err).Str("provider_key", stableKey).Msg("active provider close before switched activate failed")
			}
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
			log.Info().Str("provider_key", d.Key).Str("runtime_provider_id", info.ID).Bool("from_cache", st.FromCache).Int("remote_thread_count", len(st.Snapshot.Threads)).Msg("active provider switch complete")
			s.setState(gen, st)
			return st, nil
		}
	}
	log.Warn().Str("provider_key", stableKey).Msg("active provider switch descriptor not found")
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
	log := zerowrap.FromCtx(ctx)
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
		log.Warn().Bool("manual", manual).Msg("active provider refresh requested with no provider")
		return prev, core.NewProviderError(core.ProviderErrorNotApplicable, "no active provider", nil)
	}
	log.Info().Str("provider_key", key).Str("runtime_provider_id", runtimeID).Bool("manual", manual).Msg("active provider refresh start")
	var snap core.ProviderSnapshot
	var err error
	if loader, ok := client.(remoteSnapshotLoader); ok {
		log.Debug().Str("provider_key", key).Msg("active provider loading remote snapshot")
		snap, err = loader.LoadRemoteSnapshot(ctx, review)
	} else {
		log.Debug().Str("provider_key", key).Msg("active provider loading remote threads")
		var threads []core.RemoteReviewThread
		threads, err = client.LoadRemoteThreads(ctx, review)
		snap.Threads = threads
	}
	if err != nil {
		st := prev
		st.LastError = err
		st.Syncing = false
		st.Snapshot.Sync.LastError = err.Error()
		st.NextSyncAt = s.nextBackoff(err)
		if st.NextSyncAt.IsZero() {
			st.Snapshot.Sync.Status = core.ProviderSyncStatusFailed
			st.Snapshot.Sync.NextSyncAt = nil
			log.Warn().Err(err).Str("provider_key", key).Str("runtime_provider_id", runtimeID).Bool("manual", manual).Msg("active provider refresh failed")
		} else {
			st.Snapshot.Sync.Status = core.ProviderSyncStatusBackingOff
			st.Snapshot.Sync.NextSyncAt = new(st.NextSyncAt)
			log.Warn().Err(err).Str("provider_key", key).Str("runtime_provider_id", runtimeID).Bool("manual", manual).Time("next_sync_at", st.NextSyncAt).Msg("active provider refresh backing off")
		}
		s.setState(gen, st)
		return st, err
	}
	now := time.Now().UTC()
	next := now.Add(s.poll.Interval)
	if snap.RuntimeProviderID == "" {
		snap.RuntimeProviderID = runtimeID
	}
	snap.StableProviderKey = key
	snap.ContextKey = core.NewReviewContextKey(key, review)
	if snap.FetchedAt.IsZero() {
		snap.FetchedAt = now
	}
	snap.Sync = core.ProviderSyncState{Status: core.ProviderSyncStatusSynced, LastSyncAt: new(now), NextSyncAt: new(next)}
	if s.cache != nil {
		_ = s.cache.SaveProviderSnapshot(ctx, snap)
	}
	log.Info().Str("provider_key", key).Str("runtime_provider_id", snap.RuntimeProviderID).Bool("manual", manual).Int("remote_thread_count", len(snap.Threads)).Bool("has_overview", snap.Overview != nil).Time("next_sync_at", next).Msg("active provider refresh synced")
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
	log := zerowrap.FromCtx(ctx)
	st := ActiveProviderState{StableProviderKey: key, RuntimeProviderID: runtimeID}
	if len(info) > 0 {
		st.RuntimeInfo = info[0]
	}
	if s.cache != nil {
		contextKey := core.NewReviewContextKey(key, review)
		if snap, ok, _ := s.cache.LoadProviderSnapshot(ctx, contextKey); ok {
			snap.Cached = true
			st.Snapshot = snap
			st.FromCache = true
			if snap.Sync.NextSyncAt != nil {
				st.NextSyncAt = *snap.Sync.NextSyncAt
			}
			log.Debug().Str("provider_key", key).Any("context_key", contextKey).Int("remote_thread_count", len(snap.Threads)).Bool("has_overview", snap.Overview != nil).Msg("active provider cache hit")
		} else {
			log.Debug().Str("provider_key", key).Any("context_key", contextKey).Msg("active provider cache miss")
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
