package tui

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"ero/internal/core"
	"ero/internal/ports"
	portmocks "ero/internal/ports/mocks"
)

type mockActiveProviderController struct{ mock.Mock }

func (m *mockActiveProviderController) Catalog(ctx context.Context) ([]ports.ReviewProviderDescriptor, error) {
	args := m.Called(ctx)
	return args.Get(0).([]ports.ReviewProviderDescriptor), args.Error(1)
}
func (m *mockActiveProviderController) Start(ctx context.Context, review core.ReviewContext) (ActiveProviderState, error) {
	args := m.Called(ctx, review)
	return args.Get(0).(ActiveProviderState), args.Error(1)
}
func (m *mockActiveProviderController) Refresh(ctx context.Context, review core.ReviewContext, manual bool) (ActiveProviderState, error) {
	args := m.Called(ctx, review, manual)
	return args.Get(0).(ActiveProviderState), args.Error(1)
}
func (m *mockActiveProviderController) Switch(ctx context.Context, review core.ReviewContext, stableKey string) (ActiveProviderState, error) {
	args := m.Called(ctx, review, stableKey)
	return args.Get(0).(ActiveProviderState), args.Error(1)
}
func (m *mockActiveProviderController) PublishReview(ctx context.Context, request core.PublishReviewRequest) (core.PublishReviewResult, error) {
	args := m.Called(ctx, request)
	return args.Get(0).(core.PublishReviewResult), args.Error(1)
}
func (m *mockActiveProviderController) Generation() int64 {
	args := m.Called()
	return args.Get(0).(int64)
}
func (m *mockActiveProviderController) CompleteTimer(ctx context.Context, review core.ReviewContext, generation int64) (ActiveProviderState, error) {
	args := m.Called(ctx, review, generation)
	return args.Get(0).(ActiveProviderState), args.Error(1)
}
func (m *mockActiveProviderController) Close() error { return m.Called().Error(0) }

func TestActiveProviderReloadToNonBranchModeClearsAndClosesProvider(t *testing.T) {
	controller := &mockActiveProviderController{}
	controller.On("Close").Return(nil).Once()
	m := NewModelWithActiveProviderContext(context.Background(), []core.ReviewFile{reviewFile("branch.go", "package branch")}, nil, nil, core.ReviewRequest{DiffMode: core.DiffModeBranch}, nil, core.ReviewContext{Target: core.ReviewTargetMetadata{Mode: core.DiffModeBranch}}, controller, nil)
	m.activeProviderKey = "github"
	m.activeRuntimeID = "github"
	m.activeRuntimeInfo = core.ReviewProviderInfo{ID: "github"}
	m.providerOverview = &core.ProviderOverview{Title: "PR"}
	m.providerSyncState = core.ProviderSyncState{Status: core.ProviderSyncStatusSynced}
	m.remoteThreads = []core.RemoteReviewThread{{ExternalID: "old"}}

	updated, cmd := m.Update(reviewLoadedMsg{mode: core.DiffModeWorking, files: []core.ReviewFile{reviewFile("working.go", "package working")}})
	m = updated.(Model)
	if cmd != nil {
		_ = cmd()
	}

	require.Empty(t, m.activeProviderKey)
	require.Empty(t, m.activeRuntimeID)
	require.Nil(t, m.providerOverview)
	require.Empty(t, m.remoteThreads)
	require.Equal(t, core.DiffModeWorking, m.reviewContext.Target.Mode)
	controller.AssertExpectations(t)
}

func TestActiveProviderDoesNotStartOutsideBranchMode(t *testing.T) {
	controller := &mockActiveProviderController{}
	m := NewModelWithActiveProviderContext(context.Background(), nil, nil, nil, core.ReviewRequest{DiffMode: core.DiffModeUpstream}, nil, core.ReviewContext{Target: core.ReviewTargetMetadata{Mode: core.DiffModeUpstream}}, controller, nil)

	cmd := m.Init()

	require.Nil(t, cmd)
	controller.AssertNotCalled(t, "Catalog", mock.Anything)
	controller.AssertNotCalled(t, "Start", mock.Anything, mock.Anything)
}

func TestActiveProviderStartupLoadsOnlyActiveProviderState(t *testing.T) {
	controller := &mockActiveProviderController{}
	catalog := []ports.ReviewProviderDescriptor{{Key: "github", Label: "GitHub"}, {Key: "other", Label: "Other"}}
	startState := ActiveProviderState{StableProviderKey: "github", RuntimeProviderID: "github-runtime", RuntimeInfo: core.ReviewProviderInfo{ID: "github-runtime", Label: "GitHub"}, Snapshot: core.ProviderSnapshot{Threads: []core.RemoteReviewThread{{ProviderID: "github-runtime", ExternalID: "t1"}}, Sync: core.ProviderSyncState{Status: core.ProviderSyncStatusSynced}}}
	controller.On("Catalog", mock.Anything).Return(catalog, nil).Once()
	controller.On("Start", mock.Anything, mock.Anything).Return(startState, nil).Once()
	legacyProvider := portmocks.NewMockReviewProviderClient(t)
	m := NewModelWithActiveProviderContext(context.Background(), []core.ReviewFile{reviewFile("demo.go", "package main")}, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, controller, []ports.ReviewProviderClient{legacyProvider})

	cmd := m.Init()
	require.NotNil(t, cmd)
	updated, refreshCmd := m.Update(cmd())
	m = updated.(Model)

	require.NotNil(t, refreshCmd)
	controller.AssertNumberOfCalls(t, "Start", 1)
	require.Len(t, m.providerCatalog, 2)
	require.Equal(t, "github", m.activeProviderKey)
	require.Equal(t, "github-runtime", m.activeRuntimeID)
	require.Equal(t, core.ProviderSyncStatusSynced, m.providerSyncState.Status)
	require.Len(t, m.remoteThreads, 1)
	require.Empty(t, m.providerInfoByClient)
	controller.AssertExpectations(t)
}

func TestProviderStartupRefreshesRemoteThreadsAfterCacheState(t *testing.T) {
	controller := &mockActiveProviderController{}
	startState := ActiveProviderState{StableProviderKey: "github", RuntimeProviderID: "github", RuntimeInfo: core.ReviewProviderInfo{ID: "github", Label: "GitHub"}, Snapshot: core.ProviderSnapshot{Threads: []core.RemoteReviewThread{{ExternalID: "cached"}}}}
	refreshState := ActiveProviderState{StableProviderKey: "github", RuntimeProviderID: "github", RuntimeInfo: core.ReviewProviderInfo{ID: "github", Label: "GitHub"}, Snapshot: core.ProviderSnapshot{Threads: []core.RemoteReviewThread{{ExternalID: "fresh"}}}}
	controller.On("Catalog", mock.Anything).Return([]ports.ReviewProviderDescriptor{{Key: "github", Label: "GitHub"}}, nil).Once()
	controller.On("Start", mock.Anything, mock.Anything).Return(startState, nil).Once()
	controller.On("Refresh", mock.Anything, mock.Anything, false).Return(refreshState, nil).Once()
	m := NewModelWithActiveProviderContext(context.Background(), nil, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, controller, nil)

	started, refreshCmd := m.Update(m.Init()())
	m = started.(Model)
	require.Len(t, m.remoteThreads, 1)
	require.Equal(t, "cached", m.remoteThreads[0].ExternalID)
	require.NotNil(t, refreshCmd)

	refreshed, _ := m.Update(refreshCmd())
	m = refreshed.(Model)
	controller.AssertCalled(t, "Refresh", mock.Anything, mock.Anything, false)
	require.Len(t, m.remoteThreads, 1)
	require.Equal(t, "fresh", m.remoteThreads[0].ExternalID)
	controller.AssertExpectations(t)
}

func TestProviderRefreshManualReplacesRemoteThreads(t *testing.T) {
	controller := &mockActiveProviderController{}
	refreshState := ActiveProviderState{StableProviderKey: "github", RuntimeProviderID: "github", Snapshot: core.ProviderSnapshot{Threads: []core.RemoteReviewThread{{ExternalID: "new"}}}}
	controller.On("Refresh", mock.Anything, mock.Anything, true).Return(refreshState, nil).Once()
	m := NewModelWithActiveProviderContext(context.Background(), nil, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, controller, nil)
	m.remoteThreads = []core.RemoteReviewThread{{ExternalID: "old"}}

	msg := m.refreshActiveProviderCmd(true)()
	updated, _ := m.Update(msg)
	m = updated.(Model)

	controller.AssertCalled(t, "Refresh", mock.Anything, mock.Anything, true)
	require.Len(t, m.remoteThreads, 1)
	require.Equal(t, "new", m.remoteThreads[0].ExternalID)
	controller.AssertExpectations(t)
}

func TestProviderSwitchReplacesRemoteData(t *testing.T) {
	controller := &mockActiveProviderController{}
	switchState := ActiveProviderState{StableProviderKey: "other", RuntimeProviderID: "other-runtime", RuntimeInfo: core.ReviewProviderInfo{ID: "other-runtime"}, Snapshot: core.ProviderSnapshot{Threads: []core.RemoteReviewThread{{ExternalID: "other-thread"}}}}
	controller.On("Switch", mock.Anything, mock.Anything, "other").Return(switchState, nil).Once()
	m := NewModelWithActiveProviderContext(context.Background(), nil, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, controller, nil)
	m.remoteThreads = []core.RemoteReviewThread{{ExternalID: "old"}}

	msg := m.switchActiveProviderCmd("other")()
	updated, _ := m.Update(msg)
	m = updated.(Model)

	controller.AssertCalled(t, "Switch", mock.Anything, mock.Anything, "other")
	require.Equal(t, "other", m.activeProviderKey)
	require.Len(t, m.remoteThreads, 1)
	require.Equal(t, "other-thread", m.remoteThreads[0].ExternalID)
	controller.AssertExpectations(t)
}

func TestActiveProviderPollTimerRefreshesWithGeneration(t *testing.T) {
	controller := &mockActiveProviderController{}
	refreshed := ActiveProviderState{StableProviderKey: "github", RuntimeProviderID: "github", Snapshot: core.ProviderSnapshot{Threads: []core.RemoteReviewThread{{ExternalID: "polled"}}}}
	controller.On("CompleteTimer", mock.Anything, mock.Anything, int64(42)).Return(refreshed, nil).Once()
	m := NewModelWithActiveProviderContext(context.Background(), nil, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, controller, nil)

	msg := m.completeActiveProviderTimerCmd(42)()
	updated, _ := m.Update(msg)
	m = updated.(Model)

	controller.AssertCalled(t, "CompleteTimer", mock.Anything, mock.Anything, int64(42))
	require.Len(t, m.remoteThreads, 1)
	require.Equal(t, "polled", m.remoteThreads[0].ExternalID)
	controller.AssertExpectations(t)
}

func TestActiveProviderPublishUsesActiveProviderClient(t *testing.T) {
	controller := &mockActiveProviderController{}
	publishResult := core.PublishReviewResult{ProviderID: "github", ExternalReviewID: "review-1"}
	var publishedRequest core.PublishReviewRequest
	controller.On("PublishReview", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		publishedRequest = args.Get(1).(core.PublishReviewRequest)
	}).Return(publishResult, nil).Once()
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

	controller.AssertNumberOfCalls(t, "PublishReview", 1)
	require.Equal(t, "github", publishedRequest.ProviderID)
	require.False(t, m.publish.active)
	controller.AssertExpectations(t)
}

func TestProviderSwitchFailureClearsRemoteThreads(t *testing.T) {
	controller := &mockActiveProviderController{}
	controller.On("Switch", mock.Anything, mock.Anything, "other").Return(ActiveProviderState{}, errors.New("auth")).Once()
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
	controller.AssertExpectations(t)
}
