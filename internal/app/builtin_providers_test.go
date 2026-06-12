package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"ero/internal/ports"
	"ero/internal/ports/mocks"
)

func TestBuiltinProviderDescriptorAlwaysAvailable(t *testing.T) {
	desc := builtinProviderDescriptor()
	assert.Equal(t, BuiltinProviderKeyCodex, desc.Key)
	assert.Equal(t, "Codex", desc.PluginName)
	assert.Equal(t, version, desc.PluginVersion) // version variable is "dev" in tests
	assert.Equal(t, PluginSourceBuiltin, desc.PluginSource)
	assert.Equal(t, "builtin", desc.PluginSource)
	assert.Equal(t, "codex", desc.ContributionID)
	assert.Equal(t, "review_provider", desc.Type)
	assert.Equal(t, "Codex", desc.Label)
}

func TestMergedProviderCatalogReturnsBothInstalledAndBuiltin(t *testing.T) {
	installed := mocks.NewMockReviewProviderCatalog(t)
	installed.EXPECT().ListReviewProviderDescriptors(mock.Anything).
		Return([]ports.ReviewProviderDescriptor{
			{Key: "plugin:abc123", PluginName: "GitHub", PluginSource: "git:github.com/ero-plugins/github", ContributionID: "github"},
		}, nil)

	merged := NewMergedProviderCatalog(installed)
	descs, err := merged.ListReviewProviderDescriptors(context.Background())
	require.NoError(t, err)
	require.Len(t, descs, 2)

	// Installed plugin should come first.
	assert.Equal(t, "plugin:abc123", descs[0].Key)
	assert.Equal(t, "git:github.com/ero-plugins/github", descs[0].PluginSource)

	// Builtin descriptor should be appended.
	assert.Equal(t, BuiltinProviderKeyCodex, descs[1].Key)
	assert.Equal(t, PluginSourceBuiltin, descs[1].PluginSource)
	assert.Equal(t, "Codex", descs[1].PluginName)
}

func TestMergedProviderCatalogReturnsBuiltinOnlyWhenInstalledEmpty(t *testing.T) {
	installed := mocks.NewMockReviewProviderCatalog(t)
	installed.EXPECT().ListReviewProviderDescriptors(mock.Anything).Return([]ports.ReviewProviderDescriptor{}, nil)

	merged := NewMergedProviderCatalog(installed)
	descs, err := merged.ListReviewProviderDescriptors(context.Background())
	require.NoError(t, err)
	require.Len(t, descs, 1)
	assert.Equal(t, BuiltinProviderKeyCodex, descs[0].Key)
}

func TestMergedProviderCatalogWithNilInstalled(t *testing.T) {
	merged := NewMergedProviderCatalog(nil)
	descs, err := merged.ListReviewProviderDescriptors(context.Background())
	require.NoError(t, err)
	require.Len(t, descs, 1)
	assert.Equal(t, BuiltinProviderKeyCodex, descs[0].Key)
}

func TestMergedProviderCatalogPropagatesInstalledErrors(t *testing.T) {
	installed := mocks.NewMockReviewProviderCatalog(t)
	installed.EXPECT().ListReviewProviderDescriptors(mock.Anything).
		Return(nil, assert.AnError)

	merged := NewMergedProviderCatalog(installed)
	_, err := merged.ListReviewProviderDescriptors(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "installed plugin catalog")
}

func TestBuiltinAwareFactoryRoutesNonBuiltinDescriptors(t *testing.T) {
	delegate := mocks.NewMockReviewProviderClientFactory(t)
	factory := NewBuiltinAwareFactory(delegate)
	ctx := context.Background()

	pluginDesc := ports.ReviewProviderDescriptor{
		Key:          "plugin:abc123",
		PluginSource: "git:github.com/ero-plugins/github",
	}
	mockClient := mocks.NewMockReviewProviderClient(t)
	delegate.EXPECT().CreateReviewProviderClient(ctx, pluginDesc).Return(mockClient, nil)

	client, err := factory.CreateReviewProviderClient(ctx, pluginDesc)
	require.NoError(t, err)
	assert.Equal(t, mockClient, client)
}

func TestBuiltinAwareFactoryDoesNotDelegateBuiltinDescriptors(t *testing.T) {
	delegate := mocks.NewMockReviewProviderClientFactory(t)
	factory := NewBuiltinAwareFactory(delegate)
	// Override the builtin client factory with a stub so we never call
	// os.Executable() or start a real subprocess.
	var calledBuiltinFactory bool
	factory.builtinClientFactory = func(_ ports.ReviewProviderDescriptor) (ports.ReviewProviderClient, error) {
		calledBuiltinFactory = true
		return nil, assert.AnError
	}
	ctx := context.Background()

	builtinDesc := ports.ReviewProviderDescriptor{
		Key:            BuiltinProviderKeyCodex,
		PluginSource:   PluginSourceBuiltin,
		ContributionID: "codex",
	}

	client, err := factory.CreateReviewProviderClient(ctx, builtinDesc)
	// The delegate must never be called for a builtin descriptor — we
	// verify this indirectly: the delegate mock has no expectations and
	// testify would fail on unexpected calls, AND our stub was called.
	assert.True(t, calledBuiltinFactory, "builtin client factory should have been called")
	assert.Nil(t, client)
	assert.ErrorIs(t, err, assert.AnError)
}

func TestBuiltinAwareFactoryWithNilDelegateReturnsError(t *testing.T) {
	factory := NewBuiltinAwareFactory(nil)
	ctx := context.Background()

	pluginDesc := ports.ReviewProviderDescriptor{
		Key:          "plugin:abc123",
		PluginSource: "git:github.com/ero-plugins/github",
	}
	client, err := factory.CreateReviewProviderClient(ctx, pluginDesc)
	require.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "no delegate factory")
}
