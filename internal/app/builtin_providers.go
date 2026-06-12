package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	pluginadapter "ero/internal/adapters/out/plugin"
	"ero/internal/ports"
)

const (
	// BuiltinProviderKeyPrefix is the key namespace for all builtin/bundled
	// providers. Descriptors whose Key starts with this prefix are routed to
	// the builtin subprocess factory regardless of PluginSource.
	BuiltinProviderKeyPrefix = "builtin:"

	// BuiltinProviderKeyCodex is the stable provider key for the bundled Codex
	// review provider. It lives in the "builtin:" key namespace to avoid
	// collisions with installed plugin keys (which use "plugin:").
	BuiltinProviderKeyCodex = "builtin:codex"

	// PluginSourceBuiltin is the PluginSource value for bundled providers so
	// the provider picker and CLI render them as "builtin" rather than a git
	// URL or local path.
	PluginSourceBuiltin = "builtin"
)

// builtinProviderDescriptor returns the descriptor for the Codex builtin
// review provider. It is safe to call at any time — the runtime may not be
// functional yet, but the descriptor is always available for discovery.
func builtinProviderDescriptor() ports.ReviewProviderDescriptor {
	return ports.ReviewProviderDescriptor{
		Key:            BuiltinProviderKeyCodex,
		PluginName:     "Codex",
		PluginVersion:  version,
		PluginSource:   PluginSourceBuiltin,
		PluginPath:     "",
		ContributionID: "codex",
		Label:          "Codex",
		Type:           "review_provider",
	}
}

// MergedProviderCatalog wraps an installed-plugin catalog and appends builtin
// provider descriptors so they appear in provider discovery alongside installed
// plugins.
type MergedProviderCatalog struct {
	installed ports.ReviewProviderCatalog
}

// NewMergedProviderCatalog wraps an existing installed-plugin catalog and adds
// builtin/bundled provider descriptors to the discovery list.
func NewMergedProviderCatalog(installed ports.ReviewProviderCatalog) *MergedProviderCatalog {
	return &MergedProviderCatalog{installed: installed}
}

// ListReviewProviderDescriptors returns descriptors from the installed catalog
// followed by the builtin provider descriptors.
func (c *MergedProviderCatalog) ListReviewProviderDescriptors(ctx context.Context) ([]ports.ReviewProviderDescriptor, error) {
	var descriptors []ports.ReviewProviderDescriptor
	if c.installed != nil {
		var err error
		descriptors, err = c.installed.ListReviewProviderDescriptors(ctx)
		if err != nil {
			return nil, fmt.Errorf("installed plugin catalog: %w", err)
		}
	}
	if descriptors == nil {
		descriptors = make([]ports.ReviewProviderDescriptor, 0, 1)
	}
	descriptors = append(descriptors, builtinProviderDescriptor())
	return descriptors, nil
}

// BuiltinAwareFactory wraps a client factory and routes builtin provider
// descriptors to a subprocess based on the current ero binary, while
// delegating all other descriptors to the wrapped factory.
type BuiltinAwareFactory struct {
	delegate             ports.ReviewProviderClientFactory
	builtinClientFactory func(ports.ReviewProviderDescriptor) (ports.ReviewProviderClient, error)
}

// NewBuiltinAwareFactory creates a factory that handles builtin provider
// descriptors by spawning the current binary as the provider subprocess.
// All other descriptors are forwarded to delegate.
func NewBuiltinAwareFactory(delegate ports.ReviewProviderClientFactory) *BuiltinAwareFactory {
	return &BuiltinAwareFactory{
		delegate:             delegate,
		builtinClientFactory: createBuiltinClient,
	}
}

// CreateReviewProviderClient creates a provider client for the descriptor.
// Builtin descriptors spawn the current ero binary as a subprocess; all
// other descriptors are delegated to the wrapped factory.
func (f *BuiltinAwareFactory) CreateReviewProviderClient(ctx context.Context, descriptor ports.ReviewProviderDescriptor) (ports.ReviewProviderClient, error) {
	if isBuiltinDescriptor(descriptor) {
		return f.builtinClientFactory(descriptor)
	}
	if f.delegate == nil {
		return nil, fmt.Errorf("builtin-aware factory: no delegate factory configured for non-builtin provider %q", descriptor.Key)
	}
	return f.delegate.CreateReviewProviderClient(ctx, descriptor)
}

// isBuiltinDescriptor returns true when the descriptor represents a builtin
// provider. It uses the authoritative key namespace ("builtin:" prefix) as the
// primary discriminator rather than relying solely on PluginSource, which is
// a display/UX field.
func isBuiltinDescriptor(desc ports.ReviewProviderDescriptor) bool {
	return strings.HasPrefix(desc.Key, BuiltinProviderKeyPrefix)
}

// createBuiltinClient creates a plugin-protocol subprocess client using the
// current ero binary and the __provider hidden command.
func createBuiltinClient(descriptor ports.ReviewProviderDescriptor) (ports.ReviewProviderClient, error) {
	selfPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("builtin provider: resolve self path: %w", err)
	}
	return pluginadapter.NewClientForContribution(
		selfPath,
		[]string{"__provider", descriptor.ContributionID},
		"", // dir – no plugin directory needed for builtin
		descriptor.ContributionID,
		pluginadapter.DefaultPluginTimeout,
	)
}
