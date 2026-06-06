package tui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"ero/internal/core"
	"ero/internal/ports"
	"ero/internal/ports/mocks"
)

func TestUppercasePOpensPublishOverlayWithProvider(t *testing.T) {
	info := core.ReviewProviderInfo{ID: "pi-coding-agent", Label: "pi-coding-agent", Capabilities: core.ReviewProviderCapabilities{PublishReview: true}}
	provider := mocks.NewMockReviewProviderClient(t)
	m := NewModelWithReviewProviders([]core.ReviewFile{reviewFile("demo.go", "package main")}, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, nil)
	attachProviderClient(&m, provider, info)

	updated, cmd := m.Update(keyPress("P"))
	m = updated.(Model)

	require.Nil(t, cmd)
	require.True(t, m.publish.active)
	require.True(t, m.publish.selected["pi-coding-agent"])
	require.Contains(t, stripANSI(m.View().Content), "Publish review")
}

func TestPublishOverlaySupportsKeyboardFocusAndToggle(t *testing.T) {
	first := core.ReviewProviderInfo{ID: "github", Label: "GitHub", Capabilities: core.ReviewProviderCapabilities{PublishReview: true}}
	second := core.ReviewProviderInfo{ID: "pi-coding-agent", Label: "pi-coding-agent", Capabilities: core.ReviewProviderCapabilities{PublishReview: true}}
	m := NewModelWithReviewProviders([]core.ReviewFile{reviewFile("demo.go", "package main")}, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, nil)
	m.providerInfos = []core.ReviewProviderInfo{first, second}
	m, _ = m.openPublishReview()

	require.Equal(t, 0, m.publish.focused)
	require.True(t, m.publish.selected["github"])

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(Model)
	require.Equal(t, 1, m.publish.focused)
	require.True(t, m.publish.selected["github"])
	require.False(t, m.publish.selected["pi-coding-agent"])

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)
	require.True(t, m.publish.selected["github"])
	require.True(t, m.publish.selected["pi-coding-agent"])

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)
	require.True(t, m.publish.selected["github"])
	require.False(t, m.publish.selected["pi-coding-agent"])

	view := stripANSI(m.View().Content)
	require.Contains(t, view, "↑↓/j/k move")
	require.Contains(t, view, "space select")
}

func TestPublishReviewSuccess(t *testing.T) {
	info := core.ReviewProviderInfo{ID: "github", Label: "GitHub", Capabilities: core.ReviewProviderCapabilities{PublishReview: true, Decisions: []core.ReviewDecision{core.ReviewDecisionComment}}}
	provider := mocks.NewMockReviewProviderClient(t)
	provider.EXPECT().PublishReview(mock.Anything, mock.MatchedBy(func(req core.PublishReviewRequest) bool {
		return req.ProviderID == "github" && req.Draft.Decision == core.ReviewDecisionComment
	})).Return(core.PublishReviewResult{ProviderID: "github", ExternalReviewID: "review-1"}, nil).Once()
	m := NewModelWithReviewProviders([]core.ReviewFile{reviewFile("demo.go", "package main")}, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, nil)
	attachProviderClient(&m, provider, info)
	m.reviewDraft.SetDecision(core.ReviewDecisionComment)
	m, _ = m.openPublishReview()
	updated, cmd := m.publishSelectedProviders()
	m = updated
	require.NotNil(t, cmd)
	msg := cmd().(publishReviewCompletedMsg)
	msg.results = []core.PublishReviewResult{{ProviderID: "github", PublishedRefs: []core.PublishedReviewCommentRef{{LocalCommentID: "comment-1", ExternalID: "remote-1"}}}}
	m.reviewDraft.ApplyPublishedRefs("github", nil)
	_, err := m.reviewDraft.AddComment(core.ReviewCommentInput{FilePath: "demo.go", Range: publishTestRange(), Body: "body"})
	require.NoError(t, err)
	model, _ := m.Update(msg)
	m = model.(Model)
	require.False(t, m.publish.active)
	provider.AssertNumberOfCalls(t, "PublishReview", 1)
	comments := m.reviewDraft.Comments()
	require.Len(t, comments, 1)
	require.Len(t, comments[0].ProviderRefs, 1)
	require.Equal(t, "remote-1", comments[0].ProviderRefs[0].ExternalID)
}

func TestPublishReviewFailedProvider(t *testing.T) {
	info := core.ReviewProviderInfo{ID: "github", Capabilities: core.ReviewProviderCapabilities{PublishReview: true}}
	provider := mocks.NewMockReviewProviderClient(t)
	provider.EXPECT().PublishReview(mock.Anything, mock.MatchedBy(func(req core.PublishReviewRequest) bool { return req.ProviderID == "github" })).Return(core.PublishReviewResult{}, errors.New("auth required")).Once()
	m := NewModelWithReviewProviders([]core.ReviewFile{reviewFile("demo.go", "package main")}, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, nil)
	attachProviderClient(&m, provider, info)
	m, _ = m.openPublishReview()
	updated, cmd := m.publishSelectedProviders()
	m = updated
	model, _ := m.Update(cmd())
	m = model.(Model)
	require.True(t, m.publish.active)
	require.Contains(t, m.publish.message, "auth required")
}

func TestStatusBarShowsProviderPublishHint(t *testing.T) {
	info := core.ReviewProviderInfo{ID: "pi-coding-agent", Label: "pi-coding-agent", Capabilities: core.ReviewProviderCapabilities{PublishReview: true}}
	m := NewModelWithReviewProviders([]core.ReviewFile{reviewFile("demo.go", "package main")}, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, nil)
	m.providerInfos = []core.ReviewProviderInfo{info}

	view := stripANSI(m.View().Content)
	require.Contains(t, view, "1 provider")
	require.Contains(t, view, "P publish")
}

func TestProviderUnavailableReasonAppearsInStatusBar(t *testing.T) {
	info := core.ReviewProviderInfo{ID: "pi-coding-agent", Label: "pi-coding-agent", Capabilities: core.ReviewProviderCapabilities{PublishReview: true}}
	provider := mocks.NewMockReviewProviderClient(t)
	provider.EXPECT().Initialize(mock.Anything).Return(info, nil).Once()
	provider.EXPECT().DetectContext(mock.Anything, mock.Anything).Return(core.DetectionResult{Applicable: false, Reason: "no active bridge session"}, nil).Once()
	m := NewModelWithReviewProviders([]core.ReviewFile{reviewFile("demo.go", "package main")}, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, []ports.ReviewProviderClient{provider})

	cmd := m.Init()
	require.NotNil(t, cmd)
	updated, expire := m.Update(cmd())
	m = updated.(Model)

	require.NotNil(t, expire)
	require.Empty(t, m.providerInfos)
	require.Contains(t, stripANSI(m.View().Content), "pi-coding-agent unavailable: no active bridge session")
}

func publishTestRange() core.ReviewLineRange {
	return core.ReviewLineRange{Start: core.ReviewLineRef{NewLineNumber: 1, Kind: core.LineKindAdded}, End: core.ReviewLineRef{NewLineNumber: 1, Kind: core.LineKindAdded}}
}

func TestPublishReviewUsesClientMatchedByProviderID(t *testing.T) {
	firstInfo := core.ReviewProviderInfo{ID: "first", Capabilities: core.ReviewProviderCapabilities{PublishReview: true}}
	selectedInfo := core.ReviewProviderInfo{ID: "selected", Capabilities: core.ReviewProviderCapabilities{PublishReview: true}}
	firstClient := mocks.NewMockReviewProviderClient(t)
	selectedClient := mocks.NewMockReviewProviderClient(t)
	selectedClient.EXPECT().PublishReview(mock.Anything, mock.MatchedBy(func(req core.PublishReviewRequest) bool { return req.ProviderID == "selected" })).Return(core.PublishReviewResult{ProviderID: "selected"}, nil).Once()
	m := NewModelWithReviewProviders([]core.ReviewFile{reviewFile("demo.go", "package main")}, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, nil)
	m.reviewProviders = []ports.ReviewProviderClient{firstClient, selectedClient}
	m.providerInfos = []core.ReviewProviderInfo{selectedInfo}
	m.providerInfoByClient = map[ports.ReviewProviderClient]core.ReviewProviderInfo{m.reviewProviders[0]: firstInfo, m.reviewProviders[1]: selectedInfo}
	m, _ = m.openPublishReview()
	updated, cmd := m.publishSelectedProviders()
	m = updated
	require.NotNil(t, cmd)
	_ = cmd().(publishReviewCompletedMsg)
	firstClient.AssertNotCalled(t, "PublishReview", mock.Anything, mock.Anything)
	selectedClient.AssertNumberOfCalls(t, "PublishReview", 1)
}

func TestPublishReviewUnsupportedDecisionWarning(t *testing.T) {
	info := core.ReviewProviderInfo{ID: "pi-coding-agent", Capabilities: core.ReviewProviderCapabilities{PublishReview: true, Decisions: []core.ReviewDecision{core.ReviewDecisionComment}}}
	provider := mocks.NewMockReviewProviderClient(t)
	provider.EXPECT().PublishReview(mock.Anything, mock.MatchedBy(func(req core.PublishReviewRequest) bool {
		return req.ProviderID == "pi-coding-agent" && req.Draft.Decision == ""
	})).Return(core.PublishReviewResult{ProviderID: "pi-coding-agent"}, nil).Once()
	m := NewModelWithReviewProviders([]core.ReviewFile{reviewFile("demo.go", "package main")}, nil, nil, core.ReviewRequest{}, nil, core.ReviewContext{}, nil)
	attachProviderClient(&m, provider, info)
	m.reviewDraft.SetDecision(core.ReviewDecisionApprove)
	m, _ = m.openPublishReview()
	updated, cmd := m.publishSelectedProviders()
	m = updated
	require.Nil(t, cmd)
	require.Contains(t, m.publish.message, "Decision unsupported")
	provider.AssertNotCalled(t, "PublishReview", mock.Anything, mock.Anything)

	updated, cmd = m.publishSelectedProviders()
	m = updated
	require.NotNil(t, cmd)
	_ = cmd().(publishReviewCompletedMsg)
	provider.AssertNumberOfCalls(t, "PublishReview", 1)
}

func attachProviderClient(m *Model, provider ports.ReviewProviderClient, info core.ReviewProviderInfo) {
	m.reviewProviders = []ports.ReviewProviderClient{provider}
	m.providerInfos = []core.ReviewProviderInfo{info}
	m.providerInfoByClient = map[ports.ReviewProviderClient]core.ReviewProviderInfo{provider: info}
}
