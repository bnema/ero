// Package plugin provides the public SDK for Ero plugins. Plugin authors
// import this package to implement review providers and use the server loop.
//
// Types are re-exported from pkg/plugin/protocol so plugin authors only need
// this single import.
package plugin

import "ero/pkg/plugin/protocol"

// Protocol constants
const (
	ProtocolVersion            = protocol.ProtocolVersion
	ContributionReviewProvider = protocol.ContributionReviewProvider
)

// Envelope types
type (
	Request  = protocol.Request
	Response = protocol.Response
)

// Initialize types
type (
	InitializeRequest = protocol.InitializeRequest
	InitializeResult  = protocol.InitializeResult
)

// Provider info types
type (
	ReviewProviderInfo         = protocol.ReviewProviderInfo
	ReviewProviderCapabilities = protocol.ReviewProviderCapabilities
	ReviewDecision             = protocol.ReviewDecision
)

// Review decision constants
const (
	ReviewDecisionComment        = protocol.ReviewDecisionComment
	ReviewDecisionRequestChanges = protocol.ReviewDecisionRequestChanges
	ReviewDecisionApprove        = protocol.ReviewDecisionApprove
)

// Detect context types
type (
	DetectContextRequest = protocol.DetectContextRequest
	DetectContextResult  = protocol.DetectContextResult
	DetectionResult      = protocol.DetectionResult
)

// Remote thread and snapshot types
type (
	LoadRemoteThreadsRequest  = protocol.LoadRemoteThreadsRequest
	LoadRemoteThreadsResult   = protocol.LoadRemoteThreadsResult
	LoadRemoteSnapshotRequest = protocol.LoadRemoteSnapshotRequest
	LoadRemoteSnapshotResult  = protocol.LoadRemoteSnapshotResult
	ProviderOverview          = protocol.ProviderOverview
	ProviderIssueComment      = protocol.ProviderIssueComment
	ProviderReviewSummary     = protocol.ProviderReviewSummary
	RemoteReviewThread        = protocol.RemoteReviewThread
	RemoteReviewComment       = protocol.RemoteReviewComment
)

// Publish types
type (
	PublishReviewParams       = protocol.PublishReviewParams
	PublishReviewResultData   = protocol.PublishReviewResultData
	ReviewPublishPayload      = protocol.ReviewPublishPayload
	ReviewPublishResult       = protocol.ReviewPublishResult
	PublishedReviewCommentRef = protocol.PublishedReviewCommentRef
)

// Review context metadata types
type (
	ReviewContext         = protocol.ReviewContext
	RepositoryMetadata    = protocol.RepositoryMetadata
	GitRemote             = protocol.GitRemote
	ReviewTargetMetadata  = protocol.ReviewTargetMetadata
	DiffMetadata          = protocol.DiffMetadata
	ReviewFileMetadata    = protocol.ReviewFileMetadata
	ReviewHunkAnchor      = protocol.ReviewHunkAnchor
	ReviewLineAnchor      = protocol.ReviewLineAnchor
	ReviewSessionMetadata = protocol.ReviewSessionMetadata
)

// Draft types
type (
	ReviewDraftSnapshot = protocol.ReviewDraftSnapshot
	ReviewComment       = protocol.ReviewComment
	ReviewLineRange     = protocol.ReviewLineRange
	ReviewLineRef       = protocol.ReviewLineRef
	ProviderCommentRef  = protocol.ProviderCommentRef
)

// Error types
type (
	Error = protocol.Error
)

// Error codes
const (
	ErrorAuthRequired           = protocol.ErrorAuthRequired
	ErrorNotApplicable          = protocol.ErrorNotApplicable
	ErrorUnsupportedCapability  = protocol.ErrorUnsupportedCapability
	ErrorInvalidRequest         = protocol.ErrorInvalidRequest
	ErrorRemoteRateLimited      = protocol.ErrorRemoteRateLimited
	ErrorRemoteValidationFailed = protocol.ErrorRemoteValidationFailed
	ErrorNetwork                = protocol.ErrorNetwork
	ErrorPartialPublishUnknown  = protocol.ErrorPartialPublishUnknown
	ErrorInternal               = protocol.ErrorInternal
)

// NewError creates a protocol error with the given code and message.
func NewError(code, message string) *Error { return protocol.NewError(code, message) }

// NewErrorf creates a protocol error with a formatted message.
func NewErrorf(code, format string, args ...any) *Error {
	return protocol.NewErrorf(code, format, args...)
}

// AsError unwraps err to a *Error, returning nil if it is not a protocol error.
func AsError(err error) *Error { return protocol.AsError(err) }
