package ports

import (
	"context"

	"ero/internal/core"
)

// PluginLifecycle manages plugin lifecycle commands for CLI/application entrypoints.
type PluginLifecycle interface {
	Install(ctx context.Context, source string) (PluginInstallResult, error)
	List(ctx context.Context) ([]InstalledPlugin, error)
	Update(ctx context.Context, source string) ([]PluginUpdateResult, error)
	Remove(ctx context.Context, nameOrSource string) (PluginRemoveResult, error)
}

// PluginRegistry discovers bundled and installed plugins and their contributions.
type PluginRegistry interface {
	InstalledPlugins(ctx context.Context) ([]PluginDescriptor, error)
}

// PluginInstallResult describes the outcome of a plugin install.
type PluginInstallResult struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Source  string `json:"source"`
	Path    string `json:"path"`
}

// InstalledPlugin represents a plugin discovered from config.
type InstalledPlugin struct {
	Name          string   `json:"name"`
	Version       string   `json:"version"`
	Source        string   `json:"source"`
	Path          string   `json:"path"`
	Bundled       bool     `json:"bundled,omitempty"`
	Contributions []string `json:"contributions"`
}

// PluginUpdateResult describes the outcome of a plugin update.
type PluginUpdateResult struct {
	Source      string `json:"source"`
	Name        string `json:"name"`
	PreviousRef string `json:"previous_ref,omitempty"`
	UpdatedRef  string `json:"updated_ref,omitempty"`
	Message     string `json:"message,omitempty"`
}

// PluginRemoveResult describes the outcome of a plugin removal.
type PluginRemoveResult struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	RemovedRepo bool   `json:"removed_repo"`
}

// ReviewProviderDescriptor describes a review_provider contribution without starting its runtime.
type ReviewProviderDescriptor struct {
	Key            string
	PluginName     string
	PluginVersion  string
	PluginSource   string
	PluginPath     string
	ContributionID string
	Label          string
	Type           string
}

// ReviewProviderCatalog discovers review_provider contribution descriptors.
type ReviewProviderCatalog interface {
	ListReviewProviderDescriptors(ctx context.Context) ([]ReviewProviderDescriptor, error)
}

// ReviewProviderClientFactory creates live provider clients for selected descriptors.
type ReviewProviderClientFactory interface {
	CreateReviewProviderClient(ctx context.Context, descriptor ReviewProviderDescriptor) (ReviewProviderClient, error)
}

// PluginDescriptor holds static plugin metadata.
type PluginDescriptor struct {
	Name          string
	Version       string
	Source        string
	Path          string
	Bundled       bool
	Contributions []PluginContribution
}

// PluginContribution describes a single capability that a plugin provides.
type PluginContribution struct {
	Type  string
	ID    string
	Label string
}

// ReviewProviderClient is the interface the app/TUI uses to interact with a
// single review provider instance. It is implemented by plugin adapters.
type ReviewProviderClient interface {
	Initialize(ctx context.Context) (core.ReviewProviderInfo, error)
	DetectContext(ctx context.Context, review core.ReviewContext) (core.DetectionResult, error)
	LoadRemoteThreads(ctx context.Context, review core.ReviewContext) ([]core.RemoteReviewThread, error)
	PublishReview(ctx context.Context, request core.PublishReviewRequest) (core.PublishReviewResult, error)
	Close() error
}

// ReviewProviderSnapshotClient is implemented by providers that can load a
// normalized PR/provider snapshot in one call instead of remote threads only.
type ReviewProviderSnapshotClient interface {
	LoadRemoteSnapshot(ctx context.Context, review core.ReviewContext) (core.ProviderSnapshot, error)
}
