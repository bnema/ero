package app

import (
	"context"
	"fmt"

	"github.com/bnema/zerowrap"

	"ero/internal/ports"
)

func buildReviewProviders(ctx context.Context, catalog ports.ReviewProviderCatalog, factory ports.ReviewProviderClientFactory) ([]ports.ReviewProviderClient, error) {
	if catalog == nil || factory == nil {
		return nil, nil
	}
	descriptors, err := catalog.ListReviewProviderDescriptors(ctx)
	if err != nil {
		return nil, err
	}
	log := zerowrap.FromCtx(ctx)
	providers := make([]ports.ReviewProviderClient, 0, len(descriptors))
	failed := 0
	for _, descriptor := range descriptors {
		provider, err := factory.CreateReviewProviderClient(ctx, descriptor)
		if err != nil {
			failed++
			log.Warn().Err(err).Str("provider_key", descriptor.Key).Str("contribution_id", descriptor.ContributionID).Msg("create plugin review provider client failed")
			continue
		}
		providers = append(providers, provider)
	}
	if len(descriptors) > 0 && len(providers) == 0 {
		return nil, fmt.Errorf("create review providers: all %d configured provider(s) failed to load", failed)
	}
	return providers, nil
}
