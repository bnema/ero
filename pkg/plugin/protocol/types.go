// Package protocol defines the public wire types for the Ero plugin protocol.
// It must not import any internal packages so that third-party plugins can
// compile against it without pulling in the full Ero source tree.
package protocol

import (
	"encoding/json"
	"time"
)

// ProtocolVersion is the current plugin protocol version.
const ProtocolVersion = "ero.plugin.v1"

// ContributionReviewProvider is the contribution type for review providers.
const ContributionReviewProvider = "review_provider"

// ---- envelope types ----

// Request is a JSON-lines request envelope from the host to the plugin.
type Request struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-lines response envelope from the plugin to the host.
type Response struct {
	ID     string `json:"id"`
	Result any    `json:"result,omitempty"`
	Error  *Error `json:"error,omitempty"`
}

// ---- initialize ----

// InitializeRequest is sent by the host to negotiate the protocol and bind the
// subprocess instance to a manifest contribution.
type InitializeRequest struct {
	Protocol       string `json:"protocol"`
	ContributionID string `json:"contribution_id,omitempty"`
}

// InitializeResult is returned by the plugin after successful initialization.
type InitializeResult struct {
	Protocol string             `json:"protocol"`
	Provider ReviewProviderInfo `json:"provider"`
}

// ---- review provider info ----

// ReviewProviderInfo carries static metadata about a review provider instance.
type ReviewProviderInfo struct {
	ID           string                     `json:"id"`
	Label        string                     `json:"label"`
	Name         string                     `json:"name"`
	Capabilities ReviewProviderCapabilities `json:"capabilities"`
}

// ReviewProviderCapabilities declares what a provider can do.
type ReviewProviderCapabilities struct {
	LoadRemoteComments bool             `json:"load_remote_comments"`
	LoadRemoteSnapshot bool             `json:"load_remote_snapshot,omitempty"`
	PublishReview      bool             `json:"publish_review"`
	Decisions          []ReviewDecision `json:"decisions"`
	IdempotentPublish  bool             `json:"idempotent_publish"`
}

// ReviewDecision is the top-level verdict on a review.
type ReviewDecision string

const (
	ReviewDecisionComment        ReviewDecision = "comment"
	ReviewDecisionRequestChanges ReviewDecision = "request_changes"
	ReviewDecisionApprove        ReviewDecision = "approve"
)

// ---- detect_context ----

// DetectContextRequest asks whether a provider considers a review applicable.
type DetectContextRequest struct {
	Context ReviewContext `json:"context"`
}

// DetectContextResult wraps the detection outcome.
type DetectContextResult struct {
	Result DetectionResult `json:"result"`
}

// DetectionResult reports whether the context is applicable and why.
type DetectionResult struct {
	Applicable bool   `json:"applicable"`
	Reason     string `json:"reason,omitempty"`
}

// ---- load_remote_threads ----

// LoadRemoteThreadsRequest asks the provider to fetch remote review threads.
type LoadRemoteThreadsRequest struct {
	Context ReviewContext `json:"context"`
}

// LoadRemoteThreadsResult returns the imported remote threads.
type LoadRemoteThreadsResult struct {
	Threads []RemoteReviewThread `json:"threads"`
}

// ---- load_remote_snapshot ----

type LoadRemoteSnapshotRequest struct {
	Context ReviewContext `json:"context"`
}

type LoadRemoteSnapshotResult struct {
	RuntimeProviderID string               `json:"runtime_provider_id,omitempty"`
	Threads           []RemoteReviewThread `json:"threads"`
	Overview          *ProviderOverview    `json:"overview,omitempty"`
	Metadata          map[string]string    `json:"metadata,omitempty"`
	FetchedAt         *time.Time           `json:"fetched_at,omitempty"`
	ExpiresAt         *time.Time           `json:"expires_at,omitempty"`
}

type ProviderOverview struct {
	RuntimeProviderID string                  `json:"runtime_provider_id,omitempty"`
	Title             string                  `json:"title,omitempty"`
	Number            int                     `json:"number,omitempty"`
	State             string                  `json:"state,omitempty"`
	ExternalURL       string                  `json:"external_url,omitempty"`
	Author            string                  `json:"author,omitempty"`
	Body              string                  `json:"body,omitempty"`
	BaseRef           string                  `json:"base_ref,omitempty"`
	HeadRef           string                  `json:"head_ref,omitempty"`
	UpdatedAt         *time.Time              `json:"updated_at,omitempty"`
	Comments          []ProviderIssueComment  `json:"comments,omitempty"`
	Reviews           []ProviderReviewSummary `json:"reviews,omitempty"`
}

type ProviderIssueComment struct {
	ExternalID  string    `json:"external_id"`
	Author      string    `json:"author,omitempty"`
	Body        string    `json:"body"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ExternalURL string    `json:"external_url,omitempty"`
}

type ProviderReviewSummary struct {
	ExternalID  string    `json:"external_id"`
	Author      string    `json:"author,omitempty"`
	State       string    `json:"state,omitempty"`
	Body        string    `json:"body,omitempty"`
	SubmittedAt time.Time `json:"submitted_at"`
	ExternalURL string    `json:"external_url,omitempty"`
}

// ---- publish_review ----

// PublishReviewParams holds the payload for a publish request.
type PublishReviewParams struct {
	Payload ReviewPublishPayload `json:"payload"`
}

// PublishReviewResultData wraps the publish outcome returned by the plugin.
type PublishReviewResultData struct {
	Result ReviewPublishResult `json:"result"`
}

// ReviewPublishPayload is the wire payload sent to publish a review.
type ReviewPublishPayload struct {
	ProviderID string              `json:"provider_id"`
	Context    ReviewContext       `json:"context"`
	Draft      ReviewDraftSnapshot `json:"draft"`
}

// ReviewPublishResult captures the provider's response to a publish attempt.
type ReviewPublishResult struct {
	ProviderID       string                      `json:"provider_id"`
	ExternalReviewID string                      `json:"external_review_id,omitempty"`
	ExternalURL      string                      `json:"external_url,omitempty"`
	PublishedRefs    []PublishedReviewCommentRef `json:"published_refs"`
	Ambiguous        bool                        `json:"ambiguous"`
}

// PublishedReviewCommentRef maps a local comment to its remote identifier.
type PublishedReviewCommentRef struct {
	LocalCommentID string `json:"local_comment_id"`
	ExternalID     string `json:"external_id"`
	ExternalURL    string `json:"external_url,omitempty"`
}

// ---- review context metadata (wire duplicates of internal/core) ----

// ReviewContext captures the full metadata about a review session.
type ReviewContext struct {
	Repository RepositoryMetadata    `json:"repository"`
	Target     ReviewTargetMetadata  `json:"target"`
	Diff       DiffMetadata          `json:"diff"`
	Files      []ReviewFileMetadata  `json:"files"`
	Session    ReviewSessionMetadata `json:"session"`
}

// RepositoryMetadata describes the repository and its remotes.
type RepositoryMetadata struct {
	RepoPath      string      `json:"repo_path"`
	WorktreeRoot  string      `json:"worktree_root"`
	Remotes       []GitRemote `json:"remotes"`
	DefaultBranch string      `json:"default_branch,omitempty"`
	CurrentBranch string      `json:"current_branch,omitempty"`
	HeadSHA       string      `json:"head_sha,omitempty"`
}

// GitRemote is a named remote URL.
type GitRemote struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// ReviewTargetMetadata describes the revision range under review.
type ReviewTargetMetadata struct {
	Mode         string `json:"mode"`
	BaseRef      string `json:"base_ref,omitempty"`
	HeadRef      string `json:"head_ref,omitempty"`
	BaseSHA      string `json:"base_sha,omitempty"`
	HeadSHA      string `json:"head_sha,omitempty"`
	MergeBaseSHA string `json:"merge_base_sha,omitempty"`
}

// DiffMetadata holds aggregate diff statistics.
type DiffMetadata struct {
	FilesChanged int `json:"files_changed"`
	Additions    int `json:"additions"`
	Deletions    int `json:"deletions"`
}

// ReviewFileMetadata captures per-file change details with hunk and line anchors.
type ReviewFileMetadata struct {
	Path        string             `json:"path"`
	OldPath     string             `json:"old_path,omitempty"`
	Status      string             `json:"status"`
	Language    string             `json:"language,omitempty"`
	Hunks       []ReviewHunkAnchor `json:"hunks"`
	LineAnchors []ReviewLineAnchor `json:"line_anchors"`
}

// ReviewHunkAnchor references a unified section (hunk) in a review file.
type ReviewHunkAnchor struct {
	SectionID    string `json:"section_id"`
	OldStartLine int    `json:"old_start_line,omitempty"`
	NewStartLine int    `json:"new_start_line,omitempty"`
}

// ReviewLineAnchor is a stable reference to a line in a diff file.
type ReviewLineAnchor struct {
	FilePath      string `json:"file_path"`
	OldLineNumber int    `json:"old_line_number,omitempty"`
	NewLineNumber int    `json:"new_line_number,omitempty"`
	Side          string `json:"side"`
}

// ReviewSessionMetadata holds review-session identifiers and timestamps.
type ReviewSessionMetadata struct {
	EroVersion      string    `json:"ero_version"`
	ProtocolVersion string    `json:"protocol_version"`
	LocalReviewID   string    `json:"local_review_id"`
	CreatedAt       time.Time `json:"created_at"`
	IdempotencyKey  string    `json:"idempotency_key"`
}

// ---- remote review threads ----

// RemoteReviewThread represents an imported review thread from a remote provider.
type RemoteReviewThread struct {
	ProviderID  string                `json:"provider_id"`
	ExternalID  string                `json:"external_id"`
	FilePath    string                `json:"file_path,omitempty"`
	Range       ReviewLineRange       `json:"range"`
	Comments    []RemoteReviewComment `json:"comments"`
	Unmapped    bool                  `json:"unmapped"`
	ExternalURL string                `json:"external_url,omitempty"`
}

// RemoteReviewComment is a single comment inside a remote thread.
type RemoteReviewComment struct {
	ExternalID string    `json:"external_id"`
	Author     string    `json:"author,omitempty"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

// ---- draft snapshot ----

// ReviewDraftSnapshot is an immutable copy of a review draft at a point in time.
type ReviewDraftSnapshot struct {
	ID             string          `json:"id"`
	Comments       []ReviewComment `json:"comments"`
	Decision       ReviewDecision  `json:"decision,omitempty"`
	Summary        string          `json:"summary,omitempty"`
	IdempotencyKey string          `json:"idempotency_key"`
}

// ReviewComment is a single review comment in a draft.
type ReviewComment struct {
	ID           string               `json:"id"`
	FilePath     string               `json:"file"`
	Range        ReviewLineRange      `json:"range"`
	Body         string               `json:"body"`
	State        string               `json:"state,omitempty"`
	ProviderRefs []ProviderCommentRef `json:"provider_refs,omitempty"`
}

// ReviewLineRange marks a range of lines in a diff.
type ReviewLineRange struct {
	Start ReviewLineRef `json:"start"`
	End   ReviewLineRef `json:"end"`
}

// ReviewLineRef references a single line position in a diff.
type ReviewLineRef struct {
	OldLineNumber int    `json:"old"`
	NewLineNumber int    `json:"new"`
	Kind          string `json:"kind"`
}

// ProviderCommentRef maps a local comment to its remote published identifier.
type ProviderCommentRef struct {
	ProviderID  string `json:"provider_id"`
	ExternalID  string `json:"external_id"`
	ExternalURL string `json:"external_url,omitempty"`
}
