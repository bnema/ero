package ports

import (
	"context"

	"ero/internal/core"
)

// ProviderSnapshotCache stores normalized Ero provider snapshots, not raw provider payloads.
type ProviderSnapshotCache interface {
	LoadProviderSnapshot(ctx context.Context, key core.ReviewContextKey) (core.ProviderSnapshot, bool, error)
	SaveProviderSnapshot(ctx context.Context, snapshot core.ProviderSnapshot) error
}

// ActiveProviderPreferenceStore stores the user's active stable provider key per repository identity.
type ActiveProviderPreferenceStore interface {
	LoadActiveProviderKey(ctx context.Context, repositoryIdentity string) (string, bool, error)
	SaveActiveProviderKey(ctx context.Context, repositoryIdentity string, stableProviderKey string) error
}
