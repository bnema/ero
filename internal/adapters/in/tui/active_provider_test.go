package tui

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"ero/internal/core"
	"ero/internal/ports"
)

type fakeActiveProviderController struct {
	catalog         []ports.ReviewProviderDescriptor
	startState      ActiveProviderState
	refreshState    ActiveProviderState
	switchStates    map[string]ActiveProviderState
	publishResult   core.PublishReviewResult
	startErr        error
	refreshErr      error
	switchErrs      map[string]error
	publishErr      error
	startCalls      int
	refreshManual   []bool
	switchKeys      []string
	publishRequests []core.PublishReviewRequest
	closed          bool
}

func (f *fakeActiveProviderController) Catalog(context.Context) ([]ports.ReviewProviderDescriptor, error) {
	return append([]ports.ReviewProviderDescriptor(nil), f.catalog...), nil
}
func (f *fakeActiveProviderController) Start(context.Context, core.ReviewContext) (ActiveProviderState, error) {
	f.startCalls++
	return f.startState, f.startErr
}
func (f *fakeActiveProviderController) Refresh(_ context.Context, _ core.ReviewContext, manual bool) (ActiveProviderState, error) {
	f.refreshManual = append(f.refreshManual, manual)
	return f.refreshState, f.refreshErr
}
func (f *fakeActiveProviderController) Switch(_ context.Context, _ core.ReviewContext, key string) (ActiveProviderState, error) {
	f.switchKeys = append(f.switchKeys, key)
	if err := f.switchErrs[key]; err != nil {
		return ActiveProviderState{}, err
	}
	return f.switchStates[key], nil
}
func (f *fakeActiveProviderController) PublishReview(_ context.Context, request core.PublishReviewRequest) (core.PublishReviewResult, error) {
	f.publishRequests = append(f.publishRequests, request)
	return f.publishResult, f.publishErr
}
func (f *fakeActiveProviderController) Generation() int64 {
	return int64(len(f.refreshManual) + len(f.switchKeys) + f.startCalls)
}
func (f *fakeActiveProviderController) CompleteTimer(ctx context.Context, review core.ReviewContext, _ int64) (ActiveProviderState, error) {
	return f.Refresh(ctx, review, false)
}
func (f *fakeActiveProviderController) Close() error { f.closed = true; return nil }

func TestActiveProviderStartupLoadsOnlyActiveProviderState(t *testing.T) {
	controller := &fakeActiveProviderController{
		catalog:    []ports.ReviewProviderDescriptor{{Key: "github", Label: "GitHub"}, {Key: "other", Label: "Other"}},
		startState: ActiveProviderState{StableProviderKey: "github", RuntimeProviderID: "github-runtime", RuntimeInfo: core.ReviewProviderInfo{ID: "github-runtime", Label: "GitHub"}, Snapshot: core.ProviderSnapshot{Threads: []core.RemoteReviewThread{{ProviderID: "github-runtime", ExternalID: "t1"}}, Sync: core.ProviderSyncState{Status: core.ProviderSyncStatusSynced}}},
	}
	m := NewModelWithActiveProviderContext(context.Background(), []core.ReviewFile{reviewFile("demo.go", "package main")}, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, controller, []ports.ReviewProviderClient{fakeProviderAsPort{&fakeReviewProvider{}}})

	cmd := m.Init()
	require.NotNil(t, cmd)
	updated, refreshCmd := m.Update(cmd())
	m = updated.(Model)

	require.NotNil(t, refreshCmd)
	require.Equal(t, 1, controller.startCalls)
	require.Len(t, m.providerCatalog, 2)
	require.Equal(t, "github", m.activeProviderKey)
	require.Equal(t, "github-runtime", m.activeRuntimeID)
	require.Equal(t, core.ProviderSyncStatusSynced, m.providerSyncState.Status)
	require.Len(t, m.remoteThreads, 1)
	require.Empty(t, m.providerInfoByClient)
}

func TestProviderStartupRefreshesRemoteThreadsAfterCacheState(t *testing.T) {
	controller := &fakeActiveProviderController{
		catalog:      []ports.ReviewProviderDescriptor{{Key: "github", Label: "GitHub"}},
		startState:   ActiveProviderState{StableProviderKey: "github", RuntimeProviderID: "github", RuntimeInfo: core.ReviewProviderInfo{ID: "github", Label: "GitHub"}, Snapshot: core.ProviderSnapshot{Threads: []core.RemoteReviewThread{{ExternalID: "cached"}}}},
		refreshState: ActiveProviderState{StableProviderKey: "github", RuntimeProviderID: "github", RuntimeInfo: core.ReviewProviderInfo{ID: "github", Label: "GitHub"}, Snapshot: core.ProviderSnapshot{Threads: []core.RemoteReviewThread{{ExternalID: "fresh"}}}},
	}
	m := NewModelWithActiveProviderContext(context.Background(), nil, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, controller, nil)

	started, refreshCmd := m.Update(m.Init()())
	m = started.(Model)
	require.Len(t, m.remoteThreads, 1)
	require.Equal(t, "cached", m.remoteThreads[0].ExternalID)
	require.NotNil(t, refreshCmd)

	refreshed, _ := m.Update(refreshCmd())
	m = refreshed.(Model)
	require.Equal(t, []bool{false}, controller.refreshManual)
	require.Len(t, m.remoteThreads, 1)
	require.Equal(t, "fresh", m.remoteThreads[0].ExternalID)
}

func TestProviderRefreshManualReplacesRemoteThreads(t *testing.T) {
	controller := &fakeActiveProviderController{refreshState: ActiveProviderState{StableProviderKey: "github", RuntimeProviderID: "github", Snapshot: core.ProviderSnapshot{Threads: []core.RemoteReviewThread{{ExternalID: "new"}}}}}
	m := NewModelWithActiveProviderContext(context.Background(), nil, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, controller, nil)
	m.remoteThreads = []core.RemoteReviewThread{{ExternalID: "old"}}

	msg := m.refreshActiveProviderCmd(true)()
	updated, _ := m.Update(msg)
	m = updated.(Model)

	require.Equal(t, []bool{true}, controller.refreshManual)
	require.Len(t, m.remoteThreads, 1)
	require.Equal(t, "new", m.remoteThreads[0].ExternalID)
}

func TestProviderSwitchReplacesRemoteData(t *testing.T) {
	controller := &fakeActiveProviderController{switchStates: map[string]ActiveProviderState{"other": {StableProviderKey: "other", RuntimeProviderID: "other-runtime", RuntimeInfo: core.ReviewProviderInfo{ID: "other-runtime"}, Snapshot: core.ProviderSnapshot{Threads: []core.RemoteReviewThread{{ExternalID: "other-thread"}}}}}, switchErrs: map[string]error{}}
	m := NewModelWithActiveProviderContext(context.Background(), nil, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, controller, nil)
	m.remoteThreads = []core.RemoteReviewThread{{ExternalID: "old"}}

	msg := m.switchActiveProviderCmd("other")()
	updated, _ := m.Update(msg)
	m = updated.(Model)

	require.Equal(t, []string{"other"}, controller.switchKeys)
	require.Equal(t, "other", m.activeProviderKey)
	require.Len(t, m.remoteThreads, 1)
	require.Equal(t, "other-thread", m.remoteThreads[0].ExternalID)
}

func TestActiveProviderPollTimerRefreshesWithGeneration(t *testing.T) {
	controller := &fakeActiveProviderController{refreshState: ActiveProviderState{StableProviderKey: "github", RuntimeProviderID: "github", Snapshot: core.ProviderSnapshot{Threads: []core.RemoteReviewThread{{ExternalID: "polled"}}}}}
	m := NewModelWithActiveProviderContext(context.Background(), nil, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, controller, nil)

	msg := m.completeActiveProviderTimerCmd(42)()
	updated, _ := m.Update(msg)
	m = updated.(Model)

	require.Equal(t, []bool{false}, controller.refreshManual)
	require.Len(t, m.remoteThreads, 1)
	require.Equal(t, "polled", m.remoteThreads[0].ExternalID)
}

func TestActiveProviderPublishUsesActiveProviderClient(t *testing.T) {
	controller := &fakeActiveProviderController{publishResult: core.PublishReviewResult{ProviderID: "github", ExternalReviewID: "review-1"}}
	m := NewModelWithActiveProviderContext(context.Background(), nil, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, controller, nil)
	m.activeRuntimeInfo = core.ReviewProviderInfo{ID: "github", Label: "GitHub", Capabilities: core.ReviewProviderCapabilities{PublishReview: true}}
	m.activeRuntimeID = "github"
	m.providerInfos = []core.ReviewProviderInfo{m.activeRuntimeInfo}

	m, _ = m.openPublishReview()
	updated, cmd := m.publishSelectedProviders()
	m = updated
	require.NotNil(t, cmd)
	msg := cmd().(publishReviewCompletedMsg)
	m, _ = m.handlePublishReviewCompleted(msg)

	require.Len(t, controller.publishRequests, 1)
	require.Equal(t, "github", controller.publishRequests[0].ProviderID)
	require.False(t, m.publish.active)
}

func TestProviderSwitchFailureClearsRemoteThreads(t *testing.T) {
	controller := &fakeActiveProviderController{switchStates: map[string]ActiveProviderState{}, switchErrs: map[string]error{"other": errors.New("auth")}}
	m := NewModelWithActiveProviderContext(context.Background(), nil, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, controller, nil)
	m.activeProviderKey = "github"
	m.activeRuntimeID = "github"
	m.providerInfos = []core.ReviewProviderInfo{{ID: "github"}}
	m.remoteThreads = []core.RemoteReviewThread{{ExternalID: "old"}}

	msg := m.switchActiveProviderCmd("other")()
	updated, _ := m.Update(msg)
	m = updated.(Model)

	require.Equal(t, "other", m.activeProviderKey)
	require.Empty(t, m.activeRuntimeID)
	require.Empty(t, m.providerInfos)
	require.Empty(t, m.remoteThreads)
}
