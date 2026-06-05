package app

import (
	"context"

	"ero/internal/adapters/in/tui"
	"ero/internal/core"
	"ero/internal/ports"
)

type tuiActiveProviderController struct {
	catalog ports.ReviewProviderCatalog
	service *ActiveProviderService
}

func (c *tuiActiveProviderController) Catalog(ctx context.Context) ([]ports.ReviewProviderDescriptor, error) {
	if c == nil || c.catalog == nil {
		return nil, nil
	}
	return c.catalog.ListReviewProviderDescriptors(ctx)
}

func (c *tuiActiveProviderController) Start(ctx context.Context, review core.ReviewContext) (tui.ActiveProviderState, error) {
	state, err := c.service.Start(ctx, review)
	return c.toTUIState(state), err
}

func (c *tuiActiveProviderController) Refresh(ctx context.Context, review core.ReviewContext, manual bool) (tui.ActiveProviderState, error) {
	state, err := c.service.Refresh(ctx, review, manual)
	return c.toTUIState(state), err
}

func (c *tuiActiveProviderController) PublishReview(ctx context.Context, request core.PublishReviewRequest) (core.PublishReviewResult, error) {
	if c == nil || c.service == nil {
		return core.PublishReviewResult{}, core.NewProviderError(core.ProviderErrorNotApplicable, "no active provider", nil)
	}
	return c.service.PublishReview(ctx, request)
}

func (c *tuiActiveProviderController) Generation() int64 {
	if c == nil || c.service == nil {
		return 0
	}
	return c.service.Generation()
}

func (c *tuiActiveProviderController) CompleteTimer(ctx context.Context, review core.ReviewContext, generation int64) (tui.ActiveProviderState, error) {
	if c == nil || c.service == nil {
		return tui.ActiveProviderState{}, core.NewProviderError(core.ProviderErrorNotApplicable, "no active provider", nil)
	}
	state, err := c.service.CompleteTimer(ctx, review, generation)
	return c.toTUIState(state), err
}

func (c *tuiActiveProviderController) Switch(ctx context.Context, review core.ReviewContext, stableKey string) (tui.ActiveProviderState, error) {
	state, err := c.service.Switch(ctx, review, stableKey)
	return c.toTUIState(state), err
}

func (c *tuiActiveProviderController) Close() error {
	if c == nil || c.service == nil {
		return nil
	}
	return c.service.Close()
}

func (c *tuiActiveProviderController) toTUIState(state ActiveProviderState) tui.ActiveProviderState {
	return tui.ActiveProviderState{
		StableProviderKey: state.StableProviderKey,
		RuntimeProviderID: state.RuntimeProviderID,
		RuntimeInfo:       state.RuntimeInfo,
		Snapshot:          state.Snapshot,
		FromCache:         state.FromCache,
		Syncing:           state.Syncing,
		LastError:         state.LastError,
	}
}
