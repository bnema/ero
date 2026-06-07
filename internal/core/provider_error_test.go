package core

import (
	"fmt"
	"testing"
)

func TestClassifyProviderError(t *testing.T) {
	tests := []ProviderErrorKind{
		ProviderErrorAuthenticationRequired,
		ProviderErrorNotApplicable,
		ProviderErrorUnsupportedCapability,
		ProviderErrorRateLimited,
		ProviderErrorTransientNetwork,
		ProviderErrorRemoteValidation,
		ProviderErrorInternal,
	}
	for _, kind := range tests {
		t.Run(string(kind), func(t *testing.T) {
			err := fmt.Errorf("client boundary: %w", NewProviderError(kind, "boom", nil))
			if got := ClassifyProviderError(err); got != kind {
				t.Fatalf("got %q want %q", got, kind)
			}
		})
	}
}

func TestIsRetryableProviderError(t *testing.T) {
	if !IsRetryableProviderError(NewProviderError(ProviderErrorRateLimited, "", nil)) {
		t.Fatal("rate limited should retry")
	}
	if !IsRetryableProviderError(NewProviderError(ProviderErrorTransientNetwork, "", nil)) {
		t.Fatal("network should retry")
	}
	if IsRetryableProviderError(NewProviderError(ProviderErrorAuthenticationRequired, "", nil)) {
		t.Fatal("auth should not retry")
	}
}
