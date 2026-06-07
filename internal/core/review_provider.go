package core

import (
	"slices"
	"time"
)

// ReviewDecision is the top-level verdict on a review.
type ReviewDecision string

const (
	ReviewDecisionComment        ReviewDecision = "comment"
	ReviewDecisionRequestChanges ReviewDecision = "request_changes"
	ReviewDecisionApprove        ReviewDecision = "approve"
)

// ReviewCommentState tracks where a comment exists in the publication lifecycle.
type ReviewCommentState string

const (
	ReviewCommentStateLocal     ReviewCommentState = "local"
	ReviewCommentStateImported  ReviewCommentState = "imported"
	ReviewCommentStatePublished ReviewCommentState = "published"
	ReviewCommentStateFailed    ReviewCommentState = "failed"
)

// ReviewProviderCapabilities declares what a provider can do.
type ReviewProviderCapabilities struct {
	LoadRemoteComments bool             `json:"load_remote_comments"`
	LoadRemoteSnapshot bool             `json:"load_remote_snapshot,omitempty"`
	PublishReview      bool             `json:"publish_review"`
	Decisions          []ReviewDecision `json:"decisions"`
	IdempotentPublish  bool             `json:"idempotent_publish"`
}

// SupportsDecision reports whether the given decision is in the capability set.
// An empty decision is treated as supported (no decision required).
func (c ReviewProviderCapabilities) SupportsDecision(decision ReviewDecision) bool {
	if decision == "" {
		return true
	}
	return slices.Contains(c.Decisions, decision)
}

// ReviewProviderInfo is the static metadata about a provider instance.
type ReviewProviderInfo struct {
	ID           string                     `json:"id"`
	Label        string                     `json:"label"`
	Name         string                     `json:"name"`
	Capabilities ReviewProviderCapabilities `json:"capabilities"`
}

// DetectionResult reports whether a provider considers a review context applicable.
type DetectionResult struct {
	Applicable bool   `json:"applicable"`
	Reason     string `json:"reason,omitempty"`
}

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

// PublishReviewRequest carries the review context and draft snapshot to a provider.
type PublishReviewRequest struct {
	ProviderID string              `json:"provider_id"`
	Context    ReviewContext       `json:"context"`
	Draft      ReviewDraftSnapshot `json:"draft"`
}

// PublishReviewResult captures the provider's response to a publish attempt.
type PublishReviewResult struct {
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
