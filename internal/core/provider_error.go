package core

import (
	"errors"
	"fmt"
)

// ProviderErrorKind classifies provider failures across app/port/adapter boundaries.
type ProviderErrorKind string

const (
	ProviderErrorAuthenticationRequired ProviderErrorKind = "authentication_required"
	ProviderErrorNotApplicable          ProviderErrorKind = "not_applicable"
	ProviderErrorUnsupportedCapability  ProviderErrorKind = "unsupported_capability"
	ProviderErrorRateLimited            ProviderErrorKind = "rate_limited"
	ProviderErrorTransientNetwork       ProviderErrorKind = "transient_network"
	ProviderErrorRemoteValidation       ProviderErrorKind = "remote_validation"
	ProviderErrorInternal               ProviderErrorKind = "internal"
)

// ProviderError is the common classified provider error type returned by ports/adapters.
type ProviderError struct {
	Kind    ProviderErrorKind
	Message string
	Err     error
}

func (e *ProviderError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return string(e.Kind)
}

func (e *ProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func NewProviderError(kind ProviderErrorKind, message string, err error) *ProviderError {
	return &ProviderError{Kind: kind, Message: message, Err: err}
}

func ClassifyProviderError(err error) ProviderErrorKind {
	if err == nil {
		return ""
	}
	var providerErr *ProviderError
	if errors.As(err, &providerErr) && providerErr.Kind != "" {
		return providerErr.Kind
	}
	return ProviderErrorInternal
}

func IsRetryableProviderError(err error) bool {
	switch ClassifyProviderError(err) {
	case ProviderErrorRateLimited, ProviderErrorTransientNetwork:
		return true
	default:
		return false
	}
}

func FormatProviderError(kind ProviderErrorKind, format string, args ...any) *ProviderError {
	return NewProviderError(kind, fmt.Sprintf(format, args...), nil)
}
