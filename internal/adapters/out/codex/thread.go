package codex

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// ThreadStatus mirrors the Codex app-server thread status type discriminator.
type ThreadStatus string

const (
	ThreadStatusNotLoaded ThreadStatus = "notLoaded"
	ThreadStatusIdle      ThreadStatus = "idle"
	ThreadStatusActive    ThreadStatus = "active"
	ThreadStatusError     ThreadStatus = "systemError"
)

// ThreadCandidate is a pared-down projection of a Codex thread relevant for
// session selection matching. This is the intersection of what thread/loaded/list,
// thread/list, and thread/read return.
type ThreadCandidate struct {
	// ID is the stable Codex thread identifier (e.g. "thr_abc123").
	ID string
	// CWD is the thread's working directory at session start.
	CWD string
	// SessionKey identifies the session this thread belongs to.
	// Used by explicit session override matching.
	SessionKey string
	// Preview is a short human-readable description of the thread's purpose.
	Preview string
	// IsLoaded is true when this thread is currently loaded in memory.
	IsLoaded bool
	// Status is the thread's current status.
	Status ThreadStatus
	// CreatedAt is when the thread was created.
	CreatedAt time.Time
	// UpdatedAt is when the thread was last active.
	UpdatedAt time.Time
}

// PageToken is an opaque continuation token for paginated stored-thread listing.
type PageToken string

// ThreadPage is a page of stored thread candidates returned by the
// stored-thread listing interface.
type ThreadPage struct {
	// Items are the thread candidates in this page.
	Items []ThreadCandidate
	// NextPage is the token for the next page, or empty if this is the last page.
	NextPage PageToken
}

// StoredThreadLister provides paginated access to stored (not loaded) Codex
// threads for session selection and resume decisions.
type StoredThreadLister interface {
	// ListStoredThreads returns the next page of stored thread candidates.
	// The caller provides a context for cancellation/timeout and a page token
	// (empty for the first page). Implementations must return a non-nil error
	// for I/O failures. Pure implementations (e.g. SliceLister) return nil.
	ListStoredThreads(ctx context.Context, page PageToken) (ThreadPage, error)
}

// ThreadDecision represents the outcome of session selection.
type ThreadDecision string

const (
	// ThreadDecisionResume means a matching thread was found and the session
	// should resume it.
	ThreadDecisionResume ThreadDecision = "resume"
	// ThreadDecisionCreateNew means no suitable thread was found; the caller
	// should start a fresh thread.
	ThreadDecisionCreateNew ThreadDecision = "create_new"
	// ThreadDecisionAmbiguous means multiple candidates matched and the caller
	// must disambiguate (via user prompt or additional criteria).
	ThreadDecisionAmbiguous ThreadDecision = "ambiguous"
	// ThreadDecisionInvalidOverride means an explicit thread/session override
	// was specified but could not be resolved to an existing thread. The caller
	// should NOT fall back to creating a new thread; instead, it should report
	// the invalid override to the user.
	ThreadDecisionInvalidOverride ThreadDecision = "invalid_override"

	// ThreadDecisionIOError means an I/O error occurred during thread listing
	// or discovery that prevents reliable selection. The caller should surface
	// an actionable error rather than silently falling back to CreateNew,
	// because doing so risks creating duplicate Codex threads for the same
	// review context.
	ThreadDecisionIOError ThreadDecision = "io_error"
)

// ThreadSelectionResult is the outcome of running the session selector.
type ThreadSelectionResult struct {
	// Decision is the selection outcome.
	Decision ThreadDecision `json:"decision"`
	// Candidate is the single matched thread when Decision is Resume, or nil
	// for CreateNew / Ambiguous / InvalidOverride.
	Candidate *ThreadCandidate `json:"candidate,omitempty"`
	// Matches is non-empty when Decision is Ambiguous; it lists every
	// candidate that matched.
	Matches []ThreadCandidate `json:"matches,omitempty"`
	// Reason provides a human-readable explanation of the selection decision.
	Reason string `json:"reason"`
}

// ExplicitOverride identifies the thread or session to resume. When set,
// selection skips all other matching rules. If the override cannot be
// resolved to an existing thread, the result is InvalidOverride.
type ExplicitOverride struct {
	// ThreadID is a stable Codex thread identifier (e.g. "thr_abc123").
	// When non-empty, selection looks for this exact thread ID.
	ThreadID string

	// SessionKey is an opaque session identifier used to match threads
	// by their SessionKey field. When set (and ThreadID is empty),
	// selection resumes the unique thread whose SessionKey matches.
	// Zero matches yields InvalidOverride; multiple matches yields
	// Ambiguous.
	SessionKey string
}

// ThreadSelectionCriteria captures the inputs needed to select a Codex thread
// for a review session.
type ThreadSelectionCriteria struct {
	// Explicit takes highest precedence. When non-nil, selection attempts to
	// resume the identified thread or session. If it cannot be found, the
	// result is InvalidOverride.
	Explicit *ExplicitOverride

	// CWD is the review repository's worktree root. Used to match threads by
	// working directory.
	CWD string

	// CurrentBranch is the git branch at the time of review. Used to inform
	// decisions but not as a strict match criterion.
	CurrentBranch string

	// RepoPath is the repository root path (may differ from CWD in
	// worktree setups).
	RepoPath string
}

// NormalizePath returns a cleaned, absolute-form path for candidate matching.
// It resolves relative paths and removes trailing slashes. If path is empty
// it returns empty.
func NormalizePath(path string) string {
	if path == "" {
		return ""
	}
	cleaned := filepath.Clean(path)
	// filepath.Clean does not convert to absolute; if the path is relative
	// we can't resolve it without a base. For matching purposes we store the
	// cleaned form; callers should pass absolutes.
	return cleaned
}

// exactCWDMatch returns true when two paths are equal after normalization.
func exactCWDMatch(a, b string) bool {
	return NormalizePath(a) == NormalizePath(b)
}

const maxStoredThreadPages = 100

// SelectThread applies the session selection precedence rules:
//
//  1. Explicit override: when criteria.Explicit is non-nil, attempt to resume
//     the identified thread (by ThreadID) or session (by SessionKey).
//     ThreadID takes precedence over SessionKey when both are set.
//     If the override cannot be resolved to exactly one existing thread, the
//     result is InvalidOverride (zero matches) or Ambiguous (multiple matches
//     via SessionKey) — never CreateNew.
//
//  2. Unique loaded-thread match: when exactly one loaded thread has a CWD
//     matching criteria.CWD (or criteria.RepoPath), return Resume with that
//     thread.
//
//  3. Unique stored-thread match (paginated): when exactly one stored (not
//     loaded) thread has a CWD matching criteria.CWD, return Resume with that
//     thread.
//
//  4. When loaded matches exist but are not unique (>1), return Ambiguous.
//     Same for stored matches.
//
//  5. Otherwise (zero candidates), return CreateNew.
//
// loaded provides the loaded-thread candidates. lister provides paginated
// access to stored-thread candidates. When lister is nil, stored is treated
// as empty.
//
// The ctx parameter is passed through to the lister for cancellation/timeout.
// When the lister returns an I/O error, stored threads are treated as empty
// and the error is recorded in the result Reason.
func SelectThread(ctx context.Context, criteria ThreadSelectionCriteria, loaded []ThreadCandidate, lister StoredThreadLister) ThreadSelectionResult {
	// Step 1: explicit override
	if criteria.Explicit != nil {
		return selectExplicitThread(ctx, criteria.Explicit, loaded, lister)
	}

	// Determine the matching path set: primary is CWD, fallback is RepoPath.
	matchPaths := buildMatchPaths(criteria)

	// Step 2: unique loaded-thread match
	if result := uniqueMatchByCWD(loaded, matchPaths); result != nil {
		return *result
	}

	// Step 3: unique stored-thread match (paginated)
	stored, err := collectStoredThreads(ctx, lister)
	if err != nil {
		return ThreadSelectionResult{
			Decision: ThreadDecisionIOError,
			Reason: fmt.Sprintf(
				"listing stored threads failed: %v; cannot safely auto-select thread",
				err,
			),
		}
	}
	if result := uniqueMatchByCWD(stored, matchPaths); result != nil {
		return *result
	}

	// Step 5: no matches
	return ThreadSelectionResult{
		Decision: ThreadDecisionCreateNew,
		Candidate: &ThreadCandidate{
			CWD: criteria.CWD,
		},
		Reason: fmt.Sprintf(
			"no Codex thread found for cwd %q; will create a new thread on the next turn",
			criteria.CWD,
		),
	}
}

// buildMatchPaths produces the list of paths to check for CWD matching.
// Prefers CWD when non-empty, falls back to RepoPath.
func buildMatchPaths(criteria ThreadSelectionCriteria) []string {
	paths := make([]string, 0, 2)
	if criteria.CWD != "" {
		paths = append(paths, criteria.CWD)
	}
	if criteria.RepoPath != "" && criteria.RepoPath != criteria.CWD {
		paths = append(paths, criteria.RepoPath)
	}
	if len(paths) == 0 {
		paths = append(paths, "")
	}
	return paths
}

// selectExplicitThread handles the explicit override case.
func selectExplicitThread(ctx context.Context, override *ExplicitOverride, loaded []ThreadCandidate, lister StoredThreadLister) ThreadSelectionResult {
	threadID := strings.TrimSpace(override.ThreadID)
	sessionKey := strings.TrimSpace(override.SessionKey)

	if threadID == "" && sessionKey == "" {
		return ThreadSelectionResult{
			Decision: ThreadDecisionInvalidOverride,
			Reason:   "explicit override is set but both ThreadID and SessionKey are empty",
		}
	}

	if threadID != "" {
		// Check loaded first (fast path)
		for i := range loaded {
			if loaded[i].ID == threadID {
				return ThreadSelectionResult{
					Decision:  ThreadDecisionResume,
					Candidate: &loaded[i],
					Reason:    fmt.Sprintf("resuming explicitly requested thread %s (loaded)", threadID),
				}
			}
		}
		// Check stored threads (paginated)
		stored, err := collectStoredThreads(ctx, lister)
		if err != nil {
			return ThreadSelectionResult{
				Decision: ThreadDecisionInvalidOverride,
				Reason:   fmt.Sprintf("listing stored threads failed while searching for explicit thread %q: %v", threadID, err),
			}
		}
		for i := range stored {
			if stored[i].ID == threadID {
				return ThreadSelectionResult{
					Decision:  ThreadDecisionResume,
					Candidate: &stored[i],
					Reason:    fmt.Sprintf("resuming explicitly requested thread %s (stored)", threadID),
				}
			}
		}
		return ThreadSelectionResult{
			Decision: ThreadDecisionInvalidOverride,
			Reason:   fmt.Sprintf("explicit thread %q not found in loaded or stored threads", threadID),
		}
	}

	// SessionKey matching — checked after ThreadID, before cwd/worktree matching.
	// Deduplicate by thread ID: loaded takes precedence over stored for the
	// same thread ID so that a thread that is both loaded and stored does
	// not produce a false ambiguous match.
	stored, err := collectStoredThreads(ctx, lister)
	if err != nil {
		return ThreadSelectionResult{
			Decision: ThreadDecisionInvalidOverride,
			Reason:   fmt.Sprintf("listing stored threads failed while searching for session key %q: %v", sessionKey, err),
		}
	}

	var matched []ThreadCandidate
	seen := make(map[string]bool, len(loaded)+len(stored))
	for _, c := range loaded {
		if c.SessionKey == sessionKey && !seen[c.ID] {
			seen[c.ID] = true
			matched = append(matched, c)
		}
	}
	for _, c := range stored {
		if c.SessionKey == sessionKey && !seen[c.ID] {
			seen[c.ID] = true
			matched = append(matched, c)
		}
	}

	switch len(matched) {
	case 0:
		return ThreadSelectionResult{
			Decision: ThreadDecisionInvalidOverride,
			Reason:   fmt.Sprintf("no thread found for session key %q", sessionKey),
		}
	case 1:
		return ThreadSelectionResult{
			Decision:  ThreadDecisionResume,
			Candidate: &matched[0],
			Reason:    fmt.Sprintf("resuming thread %s for session key %q", matched[0].ID, sessionKey),
		}
	default:
		ids := make([]string, len(matched))
		for i, m := range matched {
			ids[i] = m.ID
		}
		return ThreadSelectionResult{
			Decision: ThreadDecisionAmbiguous,
			Matches:  matched,
			Reason:   fmt.Sprintf("multiple threads match session key %q: %s", sessionKey, strings.Join(ids, ", ")),
		}
	}
}

// uniqueMatchByCWD examines candidates for a unique CWD match. Returns nil
// when the result is not decidable (zero or multiple matches).
func uniqueMatchByCWD(candidates []ThreadCandidate, matchPaths []string) *ThreadSelectionResult {
	if len(candidates) == 0 {
		return nil
	}

	var matched []ThreadCandidate
	for _, c := range candidates {
		if c.CWD == "" {
			continue
		}
		for _, mp := range matchPaths {
			if mp == "" {
				continue
			}
			if exactCWDMatch(c.CWD, mp) {
				matched = append(matched, c)
				break
			}
		}
	}

	switch len(matched) {
	case 0:
		return nil
	case 1:
		return &ThreadSelectionResult{
			Decision:  ThreadDecisionResume,
			Candidate: &matched[0],
			Reason: fmt.Sprintf(
				"unique thread %s matched by cwd %q",
				matched[0].ID, matched[0].CWD,
			),
		}
	default:
		ids := make([]string, len(matched))
		for i, m := range matched {
			ids[i] = m.ID
		}
		return &ThreadSelectionResult{
			Decision: ThreadDecisionAmbiguous,
			Matches:  matched,
			Reason: fmt.Sprintf(
				"multiple threads match cwd %q: %s",
				matchPaths[0], strings.Join(ids, ", "),
			),
		}
	}
}

// FilterLoaded returns the subset of candidates whose IsLoaded field is true.
func FilterLoaded(candidates []ThreadCandidate) []ThreadCandidate {
	result := make([]ThreadCandidate, 0, len(candidates))
	for _, c := range candidates {
		if c.IsLoaded {
			result = append(result, c)
		}
	}
	return result
}

// FilterStored returns the subset of candidates whose IsLoaded field is false.
func FilterStored(candidates []ThreadCandidate) []ThreadCandidate {
	result := make([]ThreadCandidate, 0, len(candidates))
	for _, c := range candidates {
		if !c.IsLoaded {
			result = append(result, c)
		}
	}
	return result
}

// ThreadStatusFromString parses a thread status string from the Codex
// app-server wire format. Returns the zero value for unrecognized strings.
func ThreadStatusFromString(s string) ThreadStatus {
	switch s {
	case "notLoaded":
		return ThreadStatusNotLoaded
	case "idle":
		return ThreadStatusIdle
	case "active":
		return ThreadStatusActive
	case "systemError":
		return ThreadStatusError
	default:
		return ThreadStatus("")
	}
}

// collectStoredThreads paginates through a StoredThreadLister and returns
// all thread candidates as a flat slice. Returns nil when lister is nil.
//
// Bounded pagination: at most maxStoredThreadPages are fetched.
// Repeated-token protection: if the same page token is returned twice,
// the function returns an error to prevent infinite loops.
func collectStoredThreads(ctx context.Context, lister StoredThreadLister) ([]ThreadCandidate, error) {
	if lister == nil {
		return nil, nil
	}
	var result []ThreadCandidate
	var page PageToken
	seen := make(map[PageToken]bool, maxStoredThreadPages)
	for pageCount := 0; pageCount < maxStoredThreadPages; pageCount++ {
		if seen[page] {
			return nil, fmt.Errorf("codex: stored thread lister returned repeated page token %q", page)
		}
		seen[page] = true

		p, err := lister.ListStoredThreads(ctx, page)
		if err != nil {
			return nil, fmt.Errorf("codex: listing stored threads at token %q: %w", page, err)
		}
		result = append(result, p.Items...)
		if p.NextPage == "" {
			return result, nil
		}
		page = p.NextPage
	}
	return nil, fmt.Errorf("codex: stored thread listing exceeded max pages (%d)", maxStoredThreadPages)
}

// SliceLister wraps a []ThreadCandidate into a StoredThreadLister that
// returns all items in a single page. Useful for tests and callers that
// already have a flattened list.
func SliceLister(items []ThreadCandidate) StoredThreadLister {
	return &sliceLister{items: items}
}

type sliceLister struct {
	items []ThreadCandidate
}

func (s *sliceLister) ListStoredThreads(_ context.Context, page PageToken) (ThreadPage, error) {
	if page != "" {
		return ThreadPage{}, nil
	}
	return ThreadPage{
		Items:    s.items,
		NextPage: "",
	}, nil
}
