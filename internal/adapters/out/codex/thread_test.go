package codex

import (
	"context"
	"errors"
	"testing"
	"time"
)

func sampleLoadedThreads() []ThreadCandidate {
	return []ThreadCandidate{
		{ID: "thr_loaded_1", CWD: "/home/user/project", IsLoaded: true, Preview: "Implement login", Status: ThreadStatusIdle},
		{ID: "thr_loaded_2", CWD: "/home/user/other-project", IsLoaded: true, Preview: "Refactor auth", Status: ThreadStatusIdle},
	}
}

func sampleStoredThreads() []ThreadCandidate {
	return []ThreadCandidate{
		{ID: "thr_stored_1", CWD: "/home/user/project", IsLoaded: false, Preview: "Old review", Status: ThreadStatusNotLoaded},
		{ID: "thr_stored_2", CWD: "/home/user/archive-project", IsLoaded: false, Preview: "Cleanup", Status: ThreadStatusNotLoaded},
		{ID: "thr_stored_3", CWD: "/home/user/project", IsLoaded: false, Preview: "Another old review", Status: ThreadStatusNotLoaded},
	}
}

// TestSelectThreadExplicitOverride verifies that an explicit thread ID that
// matches a loaded candidate returns Resume.
func TestSelectThreadExplicitOverride(t *testing.T) {
	loaded := sampleLoadedThreads()
	stored := sampleStoredThreads()

	criteria := ThreadSelectionCriteria{
		Explicit: &ExplicitOverride{ThreadID: "thr_loaded_1"},
		CWD:      "/home/user/project",
	}
	result := SelectThread(context.Background(), criteria, loaded, SliceLister(stored))
	if result.Decision != ThreadDecisionResume {
		t.Fatalf("expected Resume, got %s", result.Decision)
	}
	if result.Candidate == nil || result.Candidate.ID != "thr_loaded_1" {
		t.Fatalf("expected candidate thr_loaded_1, got %+v", result.Candidate)
	}
}

// TestSelectThreadExplicitOverrideStored verifies that an explicit thread ID
// that matches a stored candidate returns Resume.
func TestSelectThreadExplicitOverrideStored(t *testing.T) {
	loaded := sampleLoadedThreads()
	stored := sampleStoredThreads()

	criteria := ThreadSelectionCriteria{
		Explicit: &ExplicitOverride{ThreadID: "thr_stored_1"},
		CWD:      "/home/user/project",
	}
	result := SelectThread(context.Background(), criteria, loaded, SliceLister(stored))
	if result.Decision != ThreadDecisionResume {
		t.Fatalf("expected Resume, got %s", result.Decision)
	}
	if result.Candidate == nil || result.Candidate.ID != "thr_stored_1" {
		t.Fatalf("expected candidate thr_stored_1, got %+v", result.Candidate)
	}
}

// TestSelectThreadExplicitOverrideNotFound verifies that an explicit thread ID
// that cannot be found returns InvalidOverride — NOT CreateNew.
func TestSelectThreadExplicitOverrideNotFound(t *testing.T) {
	loaded := sampleLoadedThreads()
	stored := sampleStoredThreads()

	criteria := ThreadSelectionCriteria{
		Explicit: &ExplicitOverride{ThreadID: "thr_nonexistent"},
		CWD:      "/home/user/project",
	}
	result := SelectThread(context.Background(), criteria, loaded, SliceLister(stored))
	if result.Decision != ThreadDecisionInvalidOverride {
		t.Fatalf("expected InvalidOverride for nonexistent thread, got %s", result.Decision)
	}
	if result.Candidate != nil {
		t.Fatalf("expected nil candidate for InvalidOverride, got %+v", result.Candidate)
	}
}

// TestSelectThreadExplicitSessionKeyNotFound verifies that a session key
// matching no candidates returns InvalidOverride.
func TestSelectThreadExplicitSessionKeyNotFound(t *testing.T) {
	loaded := sampleLoadedThreads()
	stored := sampleStoredThreads()

	criteria := ThreadSelectionCriteria{
		Explicit: &ExplicitOverride{SessionKey: "nonexistent-session"},
		CWD:      "/home/user/project",
	}
	result := SelectThread(context.Background(), criteria, loaded, SliceLister(stored))
	if result.Decision != ThreadDecisionInvalidOverride {
		t.Fatalf("expected InvalidOverride for unmatched session key, got %s", result.Decision)
	}
	if result.Candidate != nil {
		t.Fatalf("expected nil candidate for InvalidOverride, got %+v", result.Candidate)
	}
}

// TestSelectThreadExplicitSessionKeyResumeLoaded verifies that a session key
// matching exactly one loaded candidate returns Resume.
func TestSelectThreadExplicitSessionKeyResumeLoaded(t *testing.T) {
	loaded := []ThreadCandidate{
		{ID: "thr_loaded_1", CWD: "/home/user/project", SessionKey: "review-jan", IsLoaded: true, Status: ThreadStatusIdle},
		{ID: "thr_loaded_2", CWD: "/home/user/other-project", IsLoaded: true, Status: ThreadStatusIdle},
	}
	stored := sampleStoredThreads()

	criteria := ThreadSelectionCriteria{
		Explicit: &ExplicitOverride{SessionKey: "review-jan"},
		CWD:      "/home/user/project",
	}
	result := SelectThread(context.Background(), criteria, loaded, SliceLister(stored))
	if result.Decision != ThreadDecisionResume {
		t.Fatalf("expected Resume, got %s", result.Decision)
	}
	if result.Candidate == nil || result.Candidate.ID != "thr_loaded_1" {
		t.Fatalf("expected thr_loaded_1, got %+v", result.Candidate)
	}
}

// TestSelectThreadExplicitSessionKeyResumeStored verifies that a session key
// matching exactly one stored candidate returns Resume.
func TestSelectThreadExplicitSessionKeyResumeStored(t *testing.T) {
	loaded := sampleLoadedThreads()
	stored := []ThreadCandidate{
		{ID: "thr_stored_1", CWD: "/home/user/project", SessionKey: "review-feb", IsLoaded: false, Status: ThreadStatusNotLoaded},
		{ID: "thr_stored_2", CWD: "/home/user/archive-project", IsLoaded: false, Status: ThreadStatusNotLoaded},
	}

	criteria := ThreadSelectionCriteria{
		Explicit: &ExplicitOverride{SessionKey: "review-feb"},
		CWD:      "/home/user/project",
	}
	result := SelectThread(context.Background(), criteria, loaded, SliceLister(stored))
	if result.Decision != ThreadDecisionResume {
		t.Fatalf("expected Resume, got %s", result.Decision)
	}
	if result.Candidate == nil || result.Candidate.ID != "thr_stored_1" {
		t.Fatalf("expected thr_stored_1, got %+v", result.Candidate)
	}
}

// TestSelectThreadExplicitSessionKeyAmbiguous verifies that a session key
// matching multiple candidates returns Ambiguous.
func TestSelectThreadExplicitSessionKeyAmbiguous(t *testing.T) {
	loaded := []ThreadCandidate{
		{ID: "thr_a", CWD: "/home/user/project", SessionKey: "shared-session", IsLoaded: true, Status: ThreadStatusIdle},
	}
	stored := []ThreadCandidate{
		{ID: "thr_b", CWD: "/home/user/other-project", SessionKey: "shared-session", IsLoaded: false, Status: ThreadStatusNotLoaded},
	}

	criteria := ThreadSelectionCriteria{
		Explicit: &ExplicitOverride{SessionKey: "shared-session"},
		CWD:      "/home/user/project",
	}
	result := SelectThread(context.Background(), criteria, loaded, SliceLister(stored))
	if result.Decision != ThreadDecisionAmbiguous {
		t.Fatalf("expected Ambiguous, got %s", result.Decision)
	}
	if len(result.Matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(result.Matches))
	}
}

// TestSelectThreadExplicitThreadIDPrecedesSessionKey verifies that when both
// ThreadID and SessionKey are set, ThreadID takes precedence.
func TestSelectThreadExplicitThreadIDPrecedesSessionKey(t *testing.T) {
	loaded := []ThreadCandidate{
		{ID: "thr_tid", CWD: "/home/user/project", SessionKey: "session-x", IsLoaded: true, Status: ThreadStatusIdle},
		{ID: "thr_sk", CWD: "/home/user/other-project", SessionKey: "session-x", IsLoaded: true, Status: ThreadStatusIdle},
	}

	criteria := ThreadSelectionCriteria{
		Explicit: &ExplicitOverride{ThreadID: "thr_tid", SessionKey: "session-x"},
		CWD:      "/home/user/project",
	}
	result := SelectThread(context.Background(), criteria, loaded, nil)
	if result.Decision != ThreadDecisionResume {
		t.Fatalf("expected Resume for ThreadID match, got %s", result.Decision)
	}
	if result.Candidate == nil || result.Candidate.ID != "thr_tid" {
		t.Fatalf("expected thr_tid, got %+v", result.Candidate)
	}
}

// TestSelectThreadExplicitOverrideEmptyFields verifies that an ExplicitOverride
// with both fields empty returns InvalidOverride.
func TestSelectThreadExplicitOverrideEmptyFields(t *testing.T) {
	loaded := sampleLoadedThreads()
	stored := sampleStoredThreads()

	criteria := ThreadSelectionCriteria{
		Explicit: &ExplicitOverride{},
		CWD:      "/home/user/project",
	}
	result := SelectThread(context.Background(), criteria, loaded, SliceLister(stored))
	if result.Decision != ThreadDecisionInvalidOverride {
		t.Fatalf("expected InvalidOverride for empty override, got %s", result.Decision)
	}
}

// TestSelectThreadUniqueLoadedMatch verifies that a single loaded candidate
// matching by CWD returns Resume.
func TestSelectThreadUniqueLoadedMatch(t *testing.T) {
	loaded := []ThreadCandidate{
		{ID: "thr_loaded_1", CWD: "/home/user/project", IsLoaded: true, Status: ThreadStatusIdle},
		{ID: "thr_loaded_2", CWD: "/home/user/other-project", IsLoaded: true, Status: ThreadStatusIdle},
	}
	stored := sampleStoredThreads()

	criteria := ThreadSelectionCriteria{
		CWD: "/home/user/project",
	}
	result := SelectThread(context.Background(), criteria, loaded, SliceLister(stored))
	if result.Decision != ThreadDecisionResume {
		t.Fatalf("expected Resume, got %s", result.Decision)
	}
	if result.Candidate == nil || result.Candidate.ID != "thr_loaded_1" {
		t.Fatalf("expected thr_loaded_1, got %+v", result.Candidate)
	}
}

// TestSelectThreadUniqueStoredMatch verifies that a single stored candidate
// matching by CWD returns Resume (loaded did not match).
func TestSelectThreadUniqueStoredMatch(t *testing.T) {
	loaded := []ThreadCandidate{
		{ID: "thr_loaded_a", CWD: "/home/user/some-other-project", IsLoaded: true, Status: ThreadStatusIdle},
	}
	stored := []ThreadCandidate{
		{ID: "thr_stored_unique", CWD: "/home/user/project", IsLoaded: false, Status: ThreadStatusNotLoaded},
		{ID: "thr_stored_other", CWD: "/home/user/other", IsLoaded: false, Status: ThreadStatusNotLoaded},
	}

	criteria := ThreadSelectionCriteria{
		CWD: "/home/user/project",
	}
	result := SelectThread(context.Background(), criteria, loaded, SliceLister(stored))
	if result.Decision != ThreadDecisionResume {
		t.Fatalf("expected Resume, got %s", result.Decision)
	}
	if result.Candidate == nil || result.Candidate.ID != "thr_stored_unique" {
		t.Fatalf("expected thr_stored_unique, got %+v", result.Candidate)
	}
}

// TestSelectThreadAmbiguousLoadedMatches verifies that multiple loaded
// candidates matching by CWD return Ambiguous.
func TestSelectThreadAmbiguousLoadedMatches(t *testing.T) {
	loaded := []ThreadCandidate{
		{ID: "thr_a", CWD: "/home/user/project", IsLoaded: true, Status: ThreadStatusIdle},
		{ID: "thr_b", CWD: "/home/user/project", IsLoaded: true, Status: ThreadStatusActive},
	}
	stored := sampleStoredThreads()

	criteria := ThreadSelectionCriteria{
		CWD: "/home/user/project",
	}
	result := SelectThread(context.Background(), criteria, loaded, SliceLister(stored))
	if result.Decision != ThreadDecisionAmbiguous {
		t.Fatalf("expected Ambiguous, got %s", result.Decision)
	}
	if len(result.Matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(result.Matches))
	}
}

// TestSelectThreadNoMatchCreateNew verifies that no CWD match at all returns
// CreateNew.
func TestSelectThreadNoMatchCreateNew(t *testing.T) {
	loaded := []ThreadCandidate{
		{ID: "thr_other", CWD: "/home/user/other", IsLoaded: true},
	}
	stored := []ThreadCandidate{
		{ID: "thr_stored_other", CWD: "/tmp/unrelated", IsLoaded: false},
	}

	criteria := ThreadSelectionCriteria{
		CWD: "/home/user/never-seen",
	}
	result := SelectThread(context.Background(), criteria, loaded, SliceLister(stored))
	if result.Decision != ThreadDecisionCreateNew {
		t.Fatalf("expected CreateNew, got %s", result.Decision)
	}
}

// TestSelectThreadEmptyCandidates verifies that nil loaded and nil lister
// returns CreateNew.
func TestSelectThreadEmptyCandidates(t *testing.T) {
	criteria := ThreadSelectionCriteria{
		CWD: "/home/user/project",
	}
	result := SelectThread(context.Background(), criteria, nil, nil)
	if result.Decision != ThreadDecisionCreateNew {
		t.Fatalf("expected CreateNew, got %s", result.Decision)
	}
}

// TestSelectThreadWithRepopathFallback verifies RepoPath matching when CWD
// is empty.
func TestSelectThreadWithRepopathFallback(t *testing.T) {
	loaded := []ThreadCandidate{
		{ID: "thr_loaded_r", CWD: "/home/user/project", IsLoaded: true, Status: ThreadStatusIdle},
	}
	stored := sampleStoredThreads()

	criteria := ThreadSelectionCriteria{
		CWD:      "",
		RepoPath: "/home/user/project",
	}
	result := SelectThread(context.Background(), criteria, loaded, SliceLister(stored))
	if result.Decision != ThreadDecisionResume {
		t.Fatalf("expected Resume via RepoPath fallback, got %s", result.Decision)
	}
	if result.Candidate == nil || result.Candidate.ID != "thr_loaded_r" {
		t.Fatalf("expected thr_loaded_r, got %+v", result.Candidate)
	}
}

// TestSelectThreadAmbiguousStoredMatches verifies that multiple stored
// candidates matching by CWD return Ambiguous (after loaded doesn't match).
func TestSelectThreadAmbiguousStoredMatches(t *testing.T) {
	loaded := []ThreadCandidate{
		{ID: "thr_loaded_other", CWD: "/home/user/different-project", IsLoaded: true},
	}
	stored := []ThreadCandidate{
		{ID: "thr_s1", CWD: "/home/user/project", IsLoaded: false, Status: ThreadStatusNotLoaded},
		{ID: "thr_s2", CWD: "/home/user/project", IsLoaded: false, Status: ThreadStatusNotLoaded},
		{ID: "thr_s3", CWD: "/home/user/other", IsLoaded: false, Status: ThreadStatusNotLoaded},
	}

	criteria := ThreadSelectionCriteria{
		CWD: "/home/user/project",
	}
	result := SelectThread(context.Background(), criteria, loaded, SliceLister(stored))
	if result.Decision != ThreadDecisionAmbiguous {
		t.Fatalf("expected Ambiguous, got %s", result.Decision)
	}
	if len(result.Matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(result.Matches))
	}
}

func TestFilterLoaded(t *testing.T) {
	candidates := []ThreadCandidate{
		{ID: "a", IsLoaded: true},
		{ID: "b", IsLoaded: false},
		{ID: "c", IsLoaded: true},
	}
	loaded := FilterLoaded(candidates)
	if len(loaded) != 2 {
		t.Fatalf("expected 2 loaded, got %d", len(loaded))
	}
	if loaded[0].ID != "a" || loaded[1].ID != "c" {
		t.Fatal("unexpected filtered results")
	}
}

func TestFilterStored(t *testing.T) {
	candidates := []ThreadCandidate{
		{ID: "a", IsLoaded: true},
		{ID: "b", IsLoaded: false},
		{ID: "c", IsLoaded: true},
	}
	stored := FilterStored(candidates)
	if len(stored) != 1 {
		t.Fatalf("expected 1 stored, got %d", len(stored))
	}
	if stored[0].ID != "b" {
		t.Fatal("unexpected filtered results")
	}
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/home/user/project", "/home/user/project"},
		{"/home/user/project/", "/home/user/project"},
		{"/home/user/./project", "/home/user/project"},
		{"/home/user/project/../project", "/home/user/project"},
		{"", ""},
		{"/", "/"},
	}
	for _, tt := range tests {
		got := NormalizePath(tt.input)
		if got != tt.want {
			t.Errorf("NormalizePath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestThreadStatusFromString(t *testing.T) {
	tests := []struct {
		input string
		want  ThreadStatus
	}{
		{"notLoaded", ThreadStatusNotLoaded},
		{"idle", ThreadStatusIdle},
		{"active", ThreadStatusActive},
		{"systemError", ThreadStatusError},
		{"unknown", ThreadStatus("")},
		{"", ThreadStatus("")},
	}
	for _, tt := range tests {
		got := ThreadStatusFromString(tt.input)
		if got != tt.want {
			t.Errorf("ThreadStatusFromString(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSelectThreadWithWorktreePath(t *testing.T) {
	loaded := []ThreadCandidate{
		{ID: "thr_wt", CWD: "/home/user/project-worktree", IsLoaded: true, Status: ThreadStatusIdle},
	}

	criteria := ThreadSelectionCriteria{
		CWD:      "/home/user/project-worktree",
		RepoPath: "/home/user/project",
	}
	result := SelectThread(context.Background(), criteria, loaded, nil)
	if result.Decision != ThreadDecisionResume {
		t.Fatalf("expected Resume for worktree match, got %s", result.Decision)
	}
	if result.Candidate == nil || result.Candidate.ID != "thr_wt" {
		t.Fatalf("expected thr_wt, got %+v", result.Candidate)
	}
}

func TestSelectThreadEmptyExplicitFallsThroughToCwd(t *testing.T) {
	loaded := []ThreadCandidate{
		{ID: "thr_loaded_other", CWD: "/other/path", IsLoaded: true},
	}
	stored := []ThreadCandidate{
		{ID: "thr_s1", CWD: "/home/user/project", IsLoaded: false},
		{ID: "thr_s2", CWD: "/home/user/project", IsLoaded: false},
	}

	// When Explicit is nil, selection falls through to CWD matching.
	criteria := ThreadSelectionCriteria{
		CWD: "/home/user/project",
	}
	result := SelectThread(context.Background(), criteria, loaded, SliceLister(stored))
	if result.Decision != ThreadDecisionAmbiguous {
		t.Fatalf("expected Ambiguous (nil explicit falls through to cwd matching), got %s", result.Decision)
	}
}

func TestSelectThreadWhitespaceExplicitOverride(t *testing.T) {
	loaded := sampleLoadedThreads()
	stored := sampleStoredThreads()

	// A whitespace-only ThreadID is treated as a non-existent thread ID,
	// not as "no override".
	criteria := ThreadSelectionCriteria{
		Explicit: &ExplicitOverride{ThreadID: "  "},
		CWD:      "/home/user/project",
	}
	result := SelectThread(context.Background(), criteria, loaded, SliceLister(stored))
	if result.Decision != ThreadDecisionInvalidOverride {
		t.Fatalf("expected InvalidOverride for whitespace thread id, got %s", result.Decision)
	}
}

func TestBuildMatchPaths(t *testing.T) {
	tests := []struct {
		name     string
		criteria ThreadSelectionCriteria
		wantLen  int
	}{
		{"both set", ThreadSelectionCriteria{CWD: "/a", RepoPath: "/b"}, 2},
		{"cwd only", ThreadSelectionCriteria{CWD: "/a", RepoPath: ""}, 1},
		{"repo only", ThreadSelectionCriteria{CWD: "", RepoPath: "/b"}, 1},
		{"same value", ThreadSelectionCriteria{CWD: "/a", RepoPath: "/a"}, 1},
		{"neither set", ThreadSelectionCriteria{CWD: "", RepoPath: ""}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := buildMatchPaths(tt.criteria)
			if len(paths) != tt.wantLen {
				t.Fatalf("expected %d paths, got %d: %v", tt.wantLen, len(paths), paths)
			}
		})
	}
}

func TestTimeFields(t *testing.T) {
	now := time.Now()
	c := ThreadCandidate{
		ID:        "thr_time",
		CreatedAt: now,
		UpdatedAt: now.Add(time.Hour),
	}
	if !c.CreatedAt.Equal(now) {
		t.Fatal("CreatedAt not preserved")
	}
	if !c.UpdatedAt.Equal(now.Add(time.Hour)) {
		t.Fatal("UpdatedAt not preserved")
	}
}

// --- Pagination tests ---

// mockPagedLister returns pre-defined pages for testing paginated selection.
type mockPagedLister struct {
	pages []ThreadPage
}

func (m *mockPagedLister) ListStoredThreads(_ context.Context, page PageToken) (ThreadPage, error) {
	// First page is served with empty token.
	if page == "" && len(m.pages) > 0 {
		return m.pages[0], nil
	}
	// Subsequent pages are reached via the NextPage token from the prior page.
	for i, p := range m.pages {
		if p.NextPage == page && i+1 < len(m.pages) {
			return m.pages[i+1], nil
		}
	}
	return ThreadPage{}, nil
}

// errorLister returns a fixed error on every call, for testing I/O error paths.
type errorLister struct {
	err error
}

func (e *errorLister) ListStoredThreads(_ context.Context, _ PageToken) (ThreadPage, error) {
	return ThreadPage{}, e.err
}

// TestSelectThreadPaginatedStoredMatch verifies that a stored thread match
// works across multiple pages.
func TestSelectThreadPaginatedStoredMatch(t *testing.T) {
	loaded := []ThreadCandidate{
		{ID: "thr_loaded_other", CWD: "/other/path", IsLoaded: true},
	}
	lister := &mockPagedLister{
		pages: []ThreadPage{
			{Items: []ThreadCandidate{
				{ID: "thr_p1", CWD: "/home/user/other", IsLoaded: false},
			}, NextPage: "p0"},
			{Items: []ThreadCandidate{
				{ID: "thr_p2", CWD: "/home/user/project", IsLoaded: false},
			}, NextPage: ""},
		},
	}

	criteria := ThreadSelectionCriteria{
		CWD: "/home/user/project",
	}
	result := SelectThread(context.Background(), criteria, loaded, lister)
	if result.Decision != ThreadDecisionResume {
		t.Fatalf("expected Resume from paginated match, got %s", result.Decision)
	}
	if result.Candidate == nil || result.Candidate.ID != "thr_p2" {
		t.Fatalf("expected thr_p2, got %+v", result.Candidate)
	}
}

// TestSelectThreadPaginatedAmbiguousStored verifies that ambiguous matches
// across pages are correctly detected.
func TestSelectThreadPaginatedAmbiguousStored(t *testing.T) {
	loaded := []ThreadCandidate{
		{ID: "thr_loaded_other", CWD: "/other/path", IsLoaded: true},
	}
	lister := &mockPagedLister{
		pages: []ThreadPage{
			{Items: []ThreadCandidate{
				{ID: "thr_p1", CWD: "/home/user/project", IsLoaded: false},
			}, NextPage: "p0"},
			{Items: []ThreadCandidate{
				{ID: "thr_p2", CWD: "/home/user/project", IsLoaded: false},
			}, NextPage: ""},
		},
	}

	criteria := ThreadSelectionCriteria{
		CWD: "/home/user/project",
	}
	result := SelectThread(context.Background(), criteria, loaded, lister)
	if result.Decision != ThreadDecisionAmbiguous {
		t.Fatalf("expected Ambiguous across pages, got %s", result.Decision)
	}
	if len(result.Matches) != 2 {
		t.Fatalf("expected 2 matches across pages, got %d", len(result.Matches))
	}
}

// TestSelectThreadPaginatedNoMatch verifies CreateNew when the paginated
// lister contains no matching candidates.
func TestSelectThreadPaginatedNoMatch(t *testing.T) {
	lister := &mockPagedLister{
		pages: []ThreadPage{
			{Items: []ThreadCandidate{
				{ID: "thr_p1", CWD: "/unrelated/path", IsLoaded: false},
			}, NextPage: ""},
		},
	}

	criteria := ThreadSelectionCriteria{
		CWD: "/home/user/project",
	}
	result := SelectThread(context.Background(), criteria, nil, lister)
	if result.Decision != ThreadDecisionCreateNew {
		t.Fatalf("expected CreateNew from paginated lister, got %s", result.Decision)
	}
}

// TestSelectThreadListerError verifies that when the lister returns an I/O
// error, SelectThread returns IOError (not CreateNew), avoiding duplicate
// thread creation.
func TestSelectThreadListerError(t *testing.T) {
	// Loaded threads must NOT match by CWD so selection falls through
	// to the stored-thread lister (step 3).
	loaded := []ThreadCandidate{
		{ID: "thr_other", CWD: "/other/path", IsLoaded: true},
	}
	lister := &errorLister{err: errors.New("connection refused")}

	criteria := ThreadSelectionCriteria{
		CWD: "/home/user/project",
	}
	result := SelectThread(context.Background(), criteria, loaded, lister)
	if result.Decision != ThreadDecisionIOError {
		t.Fatalf("expected IOError on lister error, got %s", result.Decision)
	}
	if result.Candidate != nil {
		t.Fatal("expected nil candidate on IOError")
	}
	if result.Reason == "" {
		t.Fatal("expected reason with error details")
	}
}

// TestSelectThreadExplicitOverrideListerError verifies that when the lister
// errors during explicit override resolution, the result is InvalidOverride.
func TestSelectThreadExplicitOverrideListerError(t *testing.T) {
	loaded := sampleLoadedThreads()
	lister := &errorLister{err: errors.New("timeout")}

	criteria := ThreadSelectionCriteria{
		Explicit: &ExplicitOverride{ThreadID: "thr_stored_1"},
		CWD:      "/home/user/project",
	}
	result := SelectThread(context.Background(), criteria, loaded, lister)
	if result.Decision != ThreadDecisionInvalidOverride {
		t.Fatalf("expected InvalidOverride on lister error during override, got %s", result.Decision)
	}
	if result.Reason == "" {
		t.Fatal("expected reason with error details")
	}
}

// TestSelectThreadExplicitSessionKeyListerError verifies that lister errors
// during session key matching produce InvalidOverride.
func TestSelectThreadExplicitSessionKeyListerError(t *testing.T) {
	loaded := sampleLoadedThreads()
	lister := &errorLister{err: errors.New("broken pipe")}

	criteria := ThreadSelectionCriteria{
		Explicit: &ExplicitOverride{SessionKey: "some-session"},
		CWD:      "/home/user/project",
	}
	result := SelectThread(context.Background(), criteria, loaded, lister)
	if result.Decision != ThreadDecisionInvalidOverride {
		t.Fatalf("expected InvalidOverride on lister error during session key matching, got %s", result.Decision)
	}
}

// TestSliceLister verifies that SliceLister returns all items on the first
// (and only) page.
func TestSliceLister(t *testing.T) {
	items := []ThreadCandidate{
		{ID: "a"}, {ID: "b"}, {ID: "c"},
	}
	lister := SliceLister(items)

	// First page returns all items
	page, err := lister.ListStoredThreads(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Items) != 3 {
		t.Fatalf("expected 3 items on first page, got %d", len(page.Items))
	}
	if page.NextPage != "" {
		t.Fatalf("expected empty NextPage, got %q", page.NextPage)
	}

	// Subsequent pages return empty
	empty, err := lister.ListStoredThreads(context.Background(), "next")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(empty.Items) != 0 {
		t.Fatalf("expected 0 items on second page, got %d", len(empty.Items))
	}
}

// TestCollectStoredThreadsNil verifies collectStoredThreads handles nil.
func TestCollectStoredThreadsNil(t *testing.T) {
	result, err := collectStoredThreads(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil, got %+v", result)
	}
}

// TestRepeatedPageToken verifies that repeated tokens are caught.
func TestRepeatedPageToken(t *testing.T) {
	// A lister that returns the same next token repeatedly.
	lister := &mockPagedLister{
		pages: []ThreadPage{
			{Items: []ThreadCandidate{{ID: "thr_a"}}, NextPage: "loop"},
			{Items: []ThreadCandidate{{ID: "thr_b"}}, NextPage: "loop"},
		},
	}

	_, err := collectStoredThreads(context.Background(), lister)
	if err == nil {
		t.Fatal("expected error for repeated page token")
	}
}

// TestBoundedPagination verifies that the page limit is enforced.
func TestBoundedPagination(t *testing.T) {
	// A lister that returns maxStoredThreadPages pages, all with a next token.
	pages := make([]ThreadPage, maxStoredThreadPages+1)
	for i := 0; i < maxStoredThreadPages; i++ {
		pages[i] = ThreadPage{
			Items:    []ThreadCandidate{{ID: "thr_dummy"}},
			NextPage: PageToken(rune('a' + i%26)),
		}
	}
	pages[maxStoredThreadPages] = ThreadPage{
		Items:    []ThreadCandidate{{ID: "thr_end"}},
		NextPage: "",
	}

	lister := &mockPagedLister{pages: pages}

	_, err := collectStoredThreads(context.Background(), lister)
	if err == nil {
		t.Fatal("expected error for exceeding max pages")
	}
}

// TestThreadDecisionConstants verifies all expected outcome values exist.
func TestThreadDecisionConstants(t *testing.T) {
	expected := []ThreadDecision{
		ThreadDecisionResume,
		ThreadDecisionCreateNew,
		ThreadDecisionAmbiguous,
		ThreadDecisionInvalidOverride,
		ThreadDecisionIOError,
	}
	for _, d := range expected {
		if d == "" {
			t.Fatal("expected non-empty ThreadDecision constant")
		}
	}
}

// TestSelectThreadExplicitSessionKeyDedup verifies that when the same thread
// appears in both loaded and stored results, it is counted once (not twice)
// to avoid false ambiguous matches.
func TestSelectThreadExplicitSessionKeyDedup(t *testing.T) {
	loaded := []ThreadCandidate{
		{ID: "thr_duplicate", CWD: "/home/user/project", SessionKey: "dup-session", IsLoaded: true, Status: ThreadStatusIdle},
	}
	stored := []ThreadCandidate{
		{ID: "thr_duplicate", CWD: "/home/user/project", SessionKey: "dup-session", IsLoaded: false, Status: ThreadStatusNotLoaded},
		{ID: "thr_other", CWD: "/home/user/other", SessionKey: "other-session", IsLoaded: false, Status: ThreadStatusNotLoaded},
	}

	criteria := ThreadSelectionCriteria{
		Explicit: &ExplicitOverride{SessionKey: "dup-session"},
		CWD:      "/home/user/project",
	}
	result := SelectThread(context.Background(), criteria, loaded, SliceLister(stored))
	if result.Decision != ThreadDecisionResume {
		t.Fatalf("expected Resume (not Ambiguous) after dedup, got %s", result.Decision)
	}
	if result.Candidate == nil || result.Candidate.ID != "thr_duplicate" {
		t.Fatalf("expected thr_duplicate, got %+v", result.Candidate)
	}
}
