package codex

import (
	"strings"
	"testing"
)

func TestBuildExternalReviewID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"thr_abc123", "codex:thread:thr_abc123"},
		{"thr_", "codex:thread:thr_"},
		{"", ""},
	}
	for _, tt := range tests {
		got := BuildExternalReviewID(tt.input)
		if got != tt.want {
			t.Errorf("BuildExternalReviewID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBuildExternalCommentID(t *testing.T) {
	tests := []struct {
		threadID string
		turnID   string
		ordinal  int
		want     string
	}{
		{"thr_abc", "turn_123", 0, "codex:turn:thr_abc:turn_123:0"},
		{"thr_abc", "turn_123", 5, "codex:turn:thr_abc:turn_123:5"},
		{"thr_abc", "", 0, ""},
		{"", "turn_123", 0, ""},
		{"thr_abc", "turn_", 1, "codex:turn:thr_abc:turn_:1"},
		{"thr_abc", "turn_123", -1, ""},
	}
	for _, tt := range tests {
		got := BuildExternalCommentID(tt.threadID, tt.turnID, tt.ordinal)
		if got != tt.want {
			t.Errorf("BuildExternalCommentID(%q, %q, %d) = %q, want %q", tt.threadID, tt.turnID, tt.ordinal, got, tt.want)
		}
	}
}

func TestParseExternalReviewID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"codex:thread:thr_abc123", "thr_abc123"},
		{"codex:thread:thr_", "thr_"},
		{"unknown:thread:thr_abc", ""},
		{"codex:turn:thr_abc:turn_123", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := ParseExternalReviewID(tt.input)
		if got != tt.want {
			t.Errorf("ParseExternalReviewID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseExternalCommentID(t *testing.T) {
	tests := []struct {
		input        string
		wantThreadID string
		wantTurnID   string
		wantOrdinal  int
	}{
		{"codex:turn:thr_abc:turn_123:0", "thr_abc", "turn_123", 0},
		{"codex:turn:thr_abc:turn_123:5", "thr_abc", "turn_123", 5},
		{"codex:turn:thr_:turn_:1", "thr_", "turn_", 1},
		{"codex:thread:thr_abc", "", "", -1},
		{"malformed:turn:thr_abc:turn_123", "", "", -1},
		{"", "", "", -1},
		{"codex:turn:onlyone", "", "", -1},
		{"codex:turn:thr_abc:turn_123", "", "", -1},    // old two-part format, now rejected
		{"codex:turn:thr:turn:notint", "", "", -1},     // malformed ordinal
		{"codex:turn::turn_123:0", "", "", -1},         // empty thread ID
		{"codex:turn:thr_abc::0", "", "", -1},          // empty turn ID
		{"codex:turn:thr_abc:turn_123:-1", "", "", -1}, // negative ordinal
	}
	for _, tt := range tests {
		threadID, turnID, commentOrdinal := ParseExternalCommentID(tt.input)
		if threadID != tt.wantThreadID || turnID != tt.wantTurnID || commentOrdinal != tt.wantOrdinal {
			t.Errorf("ParseExternalCommentID(%q) = (%q, %q, %d), want (%q, %q, %d)",
				tt.input, threadID, turnID, commentOrdinal, tt.wantThreadID, tt.wantTurnID, tt.wantOrdinal)
		}
	}
}

func TestBuildPublishMessage(t *testing.T) {
	msg := BuildPublishMessage("Looks good overall.", "approve", []PublishComment{
		{FilePath: "main.go", NewLineStart: 42, Body: "Consider renaming this variable."},
		{FilePath: "lib.go", OldLineStart: 10, NewLineStart: 15, Body: "This function is unused."},
	})
	if msg.Summary != "Looks good overall." {
		t.Fatalf("unexpected summary: %q", msg.Summary)
	}
	if msg.Decision != "approve" {
		t.Fatalf("unexpected decision: %q", msg.Decision)
	}
	if len(msg.Comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(msg.Comments))
	}
}

func TestBuildPublishMessageWithExtraWhitespace(t *testing.T) {
	msg := BuildPublishMessage("  Summary with space  ", "request_changes", nil)
	if msg.Summary != "Summary with space" {
		t.Fatalf("expected trimmed summary, got %q", msg.Summary)
	}
}

func TestFormatPublishMessageWithComments(t *testing.T) {
	msg := PublishMessage{
		Summary:  "The code looks clean overall. A few minor suggestions.",
		Decision: "comment",
		Comments: []PublishComment{
			{FilePath: "src/main.go", NewLineStart: 42, Body: "Add a null check before dereferencing."},
			{FilePath: "src/lib.go", OldLineStart: 10, NewLineStart: 15, Body: "Consider using a constant here."},
		},
	}
	formatted := FormatPublishMessage(msg)

	if !strings.Contains(formatted, "💬 Comment") {
		t.Fatal("expected decision label")
	}
	if !strings.Contains(formatted, "The code looks clean overall") {
		t.Fatal("expected summary in output")
	}
	if !strings.Contains(formatted, "### src/main.go") {
		t.Fatal("expected file path in output")
	}
	if !strings.Contains(formatted, "Add a null check") {
		t.Fatal("expected comment body in output")
	}
	if !strings.Contains(formatted, "line 42") || !strings.Contains(formatted, "old:10, new:15") {
		t.Fatal("expected line refs in output")
	}
}

func TestFormatPublishMessageNoComments(t *testing.T) {
	msg := PublishMessage{
		Summary:  "Approved with no comments.",
		Decision: "approve",
		Comments: nil,
	}
	formatted := FormatPublishMessage(msg)

	if !strings.Contains(formatted, "✅ Approve") {
		t.Fatal("expected approve label")
	}
	if strings.Contains(formatted, "## Comments") {
		t.Fatal("expected no Comments section when no comments")
	}
}

func TestFormatPublishMessageEmpty(t *testing.T) {
	formatted := FormatPublishMessage(PublishMessage{})
	// When both summary and decision are empty-with-fallback, the output
	// still contains "## Review: Review" from the default label. This is
	// expected because the caller should check PublishMessageIsEmpty before
	// formatting and sending.
	if formatted == "" {
		t.Fatal("expected non-empty output even for empty input (decision fallback)")
	}
	if !strings.Contains(formatted, "## Review: Review") {
		t.Fatal("expected default review header")
	}
}

func TestFormatPublishMessageDecisionLabels(t *testing.T) {
	tests := []struct {
		decision string
		contains string
	}{
		{"approve", "✅"},
		{"Approve", "✅"},
		{"request_changes", "🔧"},
		{"request changes", "🔧"},
		{"Request Changes", "🔧"},
		{"comment", "💬"},
		{"Comment", "💬"},
		{"", "Review"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.decision, func(t *testing.T) {
			msg := PublishMessage{Summary: "test", Decision: tt.decision}
			formatted := FormatPublishMessage(msg)
			if tt.contains != "" && !strings.Contains(formatted, tt.contains) {
				t.Errorf("expected decision %q to contain %q", tt.decision, tt.contains)
			}
		})
	}
}

func TestFormatPublishMessageLineRefs(t *testing.T) {
	tests := []struct {
		name    string
		comment PublishComment
		ref     string
	}{
		{"both sides", PublishComment{FilePath: "f.go", OldLineStart: 10, NewLineStart: 20}, "old:10, new:20"},
		{"old only", PublishComment{FilePath: "f.go", OldLineStart: 10, NewLineStart: 0}, "line 10"},
		{"new only", PublishComment{FilePath: "f.go", OldLineStart: 0, NewLineStart: 42}, "line 42"},
		{"neither", PublishComment{FilePath: "f.go", OldLineStart: 0, NewLineStart: 0}, "f.go"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := PublishMessage{
				Summary:  "Review",
				Decision: "comment",
				Comments: []PublishComment{tt.comment},
			}
			formatted := FormatPublishMessage(msg)
			if !strings.Contains(formatted, tt.ref) {
				t.Errorf("expected ref %q in formatted output, got: %s", tt.ref, formatted)
			}
		})
	}
}

func TestPublishMessageIsEmpty(t *testing.T) {
	if !PublishMessageIsEmpty(PublishMessage{}) {
		t.Fatal("expected empty message to be empty")
	}
	if PublishMessageIsEmpty(PublishMessage{Summary: "test"}) {
		t.Fatal("expected message with summary to not be empty")
	}
	if PublishMessageIsEmpty(PublishMessage{Decision: "approve"}) {
		t.Fatal("expected message with decision only to not be empty")
	}
	if PublishMessageIsEmpty(PublishMessage{Comments: []PublishComment{{FilePath: "x", Body: "y"}}}) {
		t.Fatal("expected message with comments to not be empty")
	}
	if !PublishMessageIsEmpty(PublishMessage{Summary: "  ", Comments: nil}) {
		t.Fatal("expected whitespace-only summary to be treated as empty")
	}
	if !PublishMessageIsEmpty(PublishMessage{Decision: "  "}) {
		t.Fatal("expected whitespace-only decision to be treated as empty")
	}
}

func TestFormatPublishMessageMultipleComments(t *testing.T) {
	msg := PublishMessage{
		Summary:  "Review of the PR.",
		Decision: "approve",
		Comments: []PublishComment{
			{FilePath: "a.go", NewLineStart: 10, Body: "First comment."},
			{FilePath: "b.go", NewLineStart: 20, Body: "Second comment."},
		},
	}
	formatted := FormatPublishMessage(msg)
	if !strings.HasPrefix(formatted, "## Review: ✅ Approve") {
		t.Fatalf("unexpected header: %q", formatted[:30])
	}
	// Both comments should appear
	if !strings.Contains(formatted, "First comment.") || !strings.Contains(formatted, "Second comment.") {
		t.Fatal("both comments should be in formatted output")
	}
	// Comments section heading
	if !strings.Contains(formatted, "## Comments") {
		t.Fatal("expected Comments section heading")
	}
}

func TestExternalIDRoundTrip(t *testing.T) {
	tests := []struct {
		threadID string
		turnID   string
		ordinal  int
	}{
		{"thr_roundtrip", "turn_42", 0},
		{"thr_roundtrip", "turn_42", 7},
		{"thr:abc", "turn:123", 0},
		{"thr:abc:def", "turn:xyz", 1},
		{"thr_normal", "turn_special:with:colons", 5},
		{"thr_😊", "turn_ok", 0}, // unicode is fine too
	}
	for _, tt := range tests {
		// Review ID round-trip
		reviewID := BuildExternalReviewID(tt.threadID)
		parsed := ParseExternalReviewID(reviewID)
		if parsed != tt.threadID {
			t.Errorf("review round-trip failed for threadID=%q: got %q", tt.threadID, parsed)
		}

		// Comment ID round-trip
		commentID := BuildExternalCommentID(tt.threadID, tt.turnID, tt.ordinal)
		threadID, turnID, commentOrdinal := ParseExternalCommentID(commentID)
		if threadID != tt.threadID || turnID != tt.turnID || commentOrdinal != tt.ordinal {
			t.Errorf("comment round-trip failed for (%q, %q, %d): got (%q, %q, %d)",
				tt.threadID, tt.turnID, tt.ordinal, threadID, turnID, commentOrdinal)
		}

		// Parsing non-prefixed IDs should fail gracefully
		if got := ParseExternalReviewID("unknown:" + tt.threadID); got != "" {
			t.Errorf("expected empty for unknown prefix, got %q", got)
		}
		if _, _, ord := ParseExternalCommentID("unknown:" + tt.threadID); ord != -1 {
			t.Errorf("expected -1 for unknown prefix, got %d", ord)
		}
	}

	// Malformed escape sequences produce empty results
	if got := ParseExternalReviewID("codex:thread:%ZZ"); got != "" {
		t.Errorf("expected empty for malformed escape, got %q", got)
	}
	if _, _, ord := ParseExternalCommentID("codex:turn:%ZZ:turn_1:0"); ord != -1 {
		t.Errorf("expected -1 for malformed escape in threadID, got %d", ord)
	}
	if _, _, ord := ParseExternalCommentID("codex:turn:thr_1:%ZZ:0"); ord != -1 {
		t.Errorf("expected -1 for malformed escape in turnID, got %d", ord)
	}
}

func TestFormatPublishMessageTrailingNewline(t *testing.T) {
	msg := PublishMessage{
		Summary:  "Looks good.",
		Decision: "approve",
		Comments: []PublishComment{
			{FilePath: "f.go", NewLineStart: 1, Body: "OK"},
		},
	}
	formatted := FormatPublishMessage(msg)
	// Should not have trailing newline
	if strings.HasSuffix(formatted, "\n") {
		t.Fatal("formatted message should not have trailing newline")
	}
}
