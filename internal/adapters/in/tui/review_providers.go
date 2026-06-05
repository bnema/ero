package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/bnema/zerowrap"

	"ero/internal/core"
	"ero/internal/ports"
)

type reviewProvidersLoadedMsg struct {
	infos   []core.ReviewProviderInfo
	threads []core.RemoteReviewThread
	clients map[ports.ReviewProviderClient]core.ReviewProviderInfo
	errs    []string
}

func (m Model) closeReviewProvidersCmd() tea.Cmd {
	if m.activeProvider != nil {
		activeProvider := m.activeProvider
		return func() tea.Msg { _ = activeProvider.Close(); return nil }
	}
	providers := append([]ports.ReviewProviderClient(nil), m.reviewProviders...)
	return func() tea.Msg {
		for _, provider := range providers {
			_ = provider.Close()
		}
		return nil
	}
}

func (m Model) startActiveProviderCmd() tea.Cmd {
	activeProvider := m.activeProvider
	if activeProvider == nil {
		return nil
	}
	ctx := m.ctx
	reviewContext := m.reviewContext
	return func() tea.Msg {
		catalog, catalogErr := activeProvider.Catalog(ctx)
		state, err := activeProvider.Start(ctx, reviewContext)
		if err == nil {
			err = catalogErr
		}
		return activeProviderStartedMsg{catalog: catalog, state: state, err: err}
	}
}

func (m Model) refreshActiveProviderCmd(manual bool) tea.Cmd {
	activeProvider := m.activeProvider
	if activeProvider == nil {
		return nil
	}
	ctx := m.ctx
	reviewContext := m.reviewContext
	return func() tea.Msg {
		state, err := activeProvider.Refresh(ctx, reviewContext, manual)
		return activeProviderRefreshedMsg{state: state, err: err}
	}
}

func (m Model) switchActiveProviderCmd(stableKey string) tea.Cmd {
	activeProvider := m.activeProvider
	if activeProvider == nil {
		return nil
	}
	ctx := m.ctx
	reviewContext := m.reviewContext
	return func() tea.Msg {
		state, err := activeProvider.Switch(ctx, reviewContext, stableKey)
		return activeProviderSwitchedMsg{stableKey: stableKey, state: state, err: err}
	}
}

func (m Model) scheduleActiveProviderPollCmd() tea.Cmd {
	if m.activeProvider == nil || m.providerSyncState.NextSyncAt == nil {
		return nil
	}
	delay := max(time.Until(*m.providerSyncState.NextSyncAt), 0)
	generation := m.activeProvider.Generation()
	return tea.Tick(delay, func(time.Time) tea.Msg { return activeProviderPollDueMsg{generation: generation} })
}

func (m Model) completeActiveProviderTimerCmd(generation int64) tea.Cmd {
	activeProvider := m.activeProvider
	if activeProvider == nil {
		return nil
	}
	ctx := m.ctx
	reviewContext := m.reviewContext
	return func() tea.Msg {
		state, err := activeProvider.CompleteTimer(ctx, reviewContext, generation)
		return activeProviderRefreshedMsg{state: state, err: err}
	}
}

func (m Model) statusProviderCount() int {
	if m.activeProvider != nil {
		return len(m.providerCatalog)
	}
	return len(m.providerInfos)
}

func (m *Model) applyActiveProviderState(state ActiveProviderState) {
	m.activeProviderKey = state.StableProviderKey
	m.activeRuntimeID = state.RuntimeProviderID
	m.activeRuntimeInfo = state.RuntimeInfo
	m.providerSyncState = state.Snapshot.Sync
	m.providerOverview = state.Snapshot.Overview
	m.remoteThreads = append([]core.RemoteReviewThread(nil), state.Snapshot.Threads...)
	if state.RuntimeInfo.ID != "" {
		m.providerInfos = []core.ReviewProviderInfo{state.RuntimeInfo}
	} else {
		m.providerInfos = nil
	}
	m.providerInfoByClient = map[ports.ReviewProviderClient]core.ReviewProviderInfo{}
}

func (m *Model) clearActiveProviderRemoteData() {
	m.activeRuntimeID = ""
	m.activeRuntimeInfo = core.ReviewProviderInfo{}
	m.providerSyncState = core.ProviderSyncState{}
	m.providerOverview = nil
	m.remoteThreads = nil
	m.providerInfos = nil
	m.providerInfoByClient = map[ports.ReviewProviderClient]core.ReviewProviderInfo{}
}

func (m Model) loadReviewProvidersCmd() tea.Cmd {
	providers := make([]ports.ReviewProviderClient, len(m.reviewProviders))
	copy(providers, m.reviewProviders)
	reviewContext := m.reviewContext
	ctx := m.ctx
	return func() tea.Msg {
		log := zerowrap.FromCtx(ctx)
		log.Info().Int("provider_count", len(providers)).Msg("loading review providers")
		var infos []core.ReviewProviderInfo
		var threads []core.RemoteReviewThread
		clients := map[ports.ReviewProviderClient]core.ReviewProviderInfo{}
		var errs []string
		for _, provider := range providers {
			info, err := provider.Initialize(ctx)
			if err != nil {
				log.Error().Err(err).Msg("review provider init failed")
				errs = append(errs, fmt.Sprintf("provider init failed: %v", err))
				continue
			}
			providerLog := log.WithField("provider_id", info.ID).WithField("provider_label", providerDisplayLabel(info))
			providerLog.Info().Msg("review provider initialized")
			detection, err := provider.DetectContext(ctx, reviewContext)
			if err != nil {
				providerLog.Error().Err(err).Msg("review provider detect failed")
				errs = append(errs, fmt.Sprintf("%s detect failed: %v", providerDisplayLabel(info), err))
				continue
			}
			providerLog.Info().Bool("applicable", detection.Applicable).Str("reason", detection.Reason).Msg("review provider detection completed")
			if !detection.Applicable {
				if detection.Reason != "" {
					errs = append(errs, fmt.Sprintf("%s unavailable: %s", providerDisplayLabel(info), detection.Reason))
				}
				continue
			}
			infos = append(infos, info)
			clients[provider] = info
			if info.Capabilities.LoadRemoteComments {
				loaded, err := provider.LoadRemoteThreads(ctx, reviewContext)
				if err != nil {
					providerLog.Error().Err(err).Msg("review provider remote comments failed")
					errs = append(errs, fmt.Sprintf("%s remote comments failed: %v", providerDisplayLabel(info), err))
					continue
				}
				providerLog.Info().Int("thread_count", len(loaded)).Msg("review provider remote comments loaded")
				for _, thread := range loaded {
					if thread.ProviderID == "" {
						thread.ProviderID = info.ID
					}
					threads = append(threads, thread)
				}
			}
		}
		log.Info().Int("available_provider_count", len(infos)).Int("remote_thread_count", len(threads)).Int("error_count", len(errs)).Msg("review providers loaded")
		return reviewProvidersLoadedMsg{infos: infos, threads: threads, clients: clients, errs: errs}
	}
}
