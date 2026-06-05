package core

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ReviewContextKey identifies a provider snapshot for a stable provider and review target.
type ReviewContextKey struct {
	StableProviderKey  string   `json:"stable_provider_key"`
	RepositoryIdentity string   `json:"repository_identity"`
	TargetMode         DiffMode `json:"target_mode"`
	BaseRef            string   `json:"base_ref,omitempty"`
	HeadRef            string   `json:"head_ref,omitempty"`
	BaseSHA            string   `json:"base_sha,omitempty"`
	HeadSHA            string   `json:"head_sha,omitempty"`
	MergeBaseSHA       string   `json:"merge_base_sha,omitempty"`
}

// NewReviewContextKey builds a cache/preference identity from stable review inputs only.
func NewReviewContextKey(stableProviderKey string, ctx ReviewContext) ReviewContextKey {
	return ReviewContextKey{
		StableProviderKey:  stableProviderKey,
		RepositoryIdentity: RepositoryIdentity(ctx.Repository),
		TargetMode:         ctx.Target.Mode,
		BaseRef:            ctx.Target.BaseRef,
		HeadRef:            ctx.Target.HeadRef,
		BaseSHA:            ctx.Target.BaseSHA,
		HeadSHA:            ctx.Target.HeadSHA,
		MergeBaseSHA:       ctx.Target.MergeBaseSHA,
	}
}

// RepositoryIdentity returns a stable repository identity, preferring normalized remotes.
func RepositoryIdentity(repo RepositoryMetadata) string {
	remotes := make([]string, 0, len(repo.Remotes))
	for _, remote := range repo.Remotes {
		if normalized := normalizeRemoteURL(remote.URL); normalized != "" {
			remotes = append(remotes, normalized)
		}
	}
	if len(remotes) > 0 {
		sort.Strings(remotes)
		return "remotes:" + strings.Join(remotes, ",")
	}
	if repo.WorktreeRoot != "" {
		return "path:" + filepath.Clean(repo.WorktreeRoot)
	}
	if repo.RepoPath != "" {
		return "path:" + filepath.Clean(repo.RepoPath)
	}
	return "unknown"
}

// Digest returns a filesystem-safe digest for the context key.
func (k ReviewContextKey) Digest() string {
	parts := []string{k.StableProviderKey, k.RepositoryIdentity, string(k.TargetMode), k.BaseRef, k.HeadRef, k.BaseSHA, k.HeadSHA, k.MergeBaseSHA}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

// ProviderSnapshot is Ero's normalized cached view of remote review data.
type ProviderSnapshot struct {
	StableProviderKey string               `json:"stable_provider_key"`
	RuntimeProviderID string               `json:"runtime_provider_id,omitempty"`
	ContextKey        ReviewContextKey     `json:"context_key"`
	Threads           []RemoteReviewThread `json:"threads"`
	Overview          *ProviderOverview    `json:"overview,omitempty"`
	Metadata          map[string]string    `json:"metadata,omitempty"`
	FetchedAt         time.Time            `json:"fetched_at"`
	ExpiresAt         *time.Time           `json:"expires_at,omitempty"`
	Cached            bool                 `json:"cached"`
	Stale             bool                 `json:"stale"`
	Sync              ProviderSyncState    `json:"sync"`
}

// ProviderOverview is a placeholder for richer provider overview data.
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

type ProviderSyncStatus string

const (
	ProviderSyncStatusIdle         ProviderSyncStatus = "idle"
	ProviderSyncStatusLoadingCache ProviderSyncStatus = "loading_cache"
	ProviderSyncStatusSyncing      ProviderSyncStatus = "syncing"
	ProviderSyncStatusSynced       ProviderSyncStatus = "synced"
	ProviderSyncStatusFailed       ProviderSyncStatus = "failed"
	ProviderSyncStatusBackingOff   ProviderSyncStatus = "backing_off"
)

type ProviderSyncState struct {
	Status     ProviderSyncStatus `json:"status"`
	LastSyncAt *time.Time         `json:"last_sync_at,omitempty"`
	NextSyncAt *time.Time         `json:"next_sync_at,omitempty"`
	LastError  string             `json:"last_error,omitempty"`
}

func normalizeRemoteURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "git@") && strings.Contains(raw, ":") {
		trimmed := strings.TrimPrefix(raw, "git@")
		parts := strings.SplitN(trimmed, ":", 2)
		return strings.ToLower(parts[0]) + "/" + strings.TrimSuffix(parts[1], ".git")
	}
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		path := strings.TrimPrefix(strings.TrimSuffix(u.Path, ".git"), "/")
		return strings.ToLower(u.Host) + "/" + path
	}
	return strings.TrimSuffix(raw, ".git")
}
