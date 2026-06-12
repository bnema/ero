package codex

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// External identifier builders
//
// Stable external IDs are derived from Codex thread/turn identifiers so that
// phase-3 can map PublishReviewResult back to the corresponding Codex thread
// and turn.
// ---------------------------------------------------------------------------

// BuildExternalReviewID creates a stable provider-scoped external review
// identifier from a Codex thread ID. The format is:
//
//	"codex:thread:<escaped-threadID>"
//
// The threadID is URL path-escaped so that IDs containing ":" or other
// special characters are handled robustly.
//
// Example: "codex:thread:thr_abc123" or "codex:thread:thr%3Aabc"
func BuildExternalReviewID(threadID string) string {
	if threadID == "" {
		return ""
	}
	return "codex:thread:" + url.QueryEscape(threadID)
}

// BuildExternalCommentID creates a stable external comment identifier from
// a Codex thread ID, turn ID, and a local comment ordinal. The format is:
//
//	"codex:turn:<escaped-threadID>:<escaped-turnID>:<commentOrdinal>"
//
// The threadID and turnID are URL path-escaped so that IDs containing ":"
// or other special characters are handled robustly. The ordinal is a
// plain integer.
//
// Example: "codex:turn:thr_abc123:turn_456:0" or
// "codex:turn:thr%3Aabc:turn%3Axyz:0"
func BuildExternalCommentID(threadID, turnID string, commentOrdinal int) string {
	if threadID == "" || turnID == "" || commentOrdinal < 0 {
		return ""
	}
	return fmt.Sprintf("codex:turn:%s:%s:%d",
		url.QueryEscape(threadID),
		url.QueryEscape(turnID),
		commentOrdinal,
	)
}

// ParseExternalReviewID extracts the Codex thread ID from an external review
// ID previously built by BuildExternalReviewID. Returns empty when the input
// is not a recognized codex external ID or the encoded form is malformed.
func ParseExternalReviewID(externalID string) string {
	prefix := "codex:thread:"
	if !strings.HasPrefix(externalID, prefix) {
		return ""
	}
	raw := strings.TrimPrefix(externalID, prefix)
	decoded, err := url.QueryUnescape(raw)
	if err != nil {
		return ""
	}
	return decoded
}

// ParseExternalCommentID extracts the Codex thread ID, turn ID, and comment
// ordinal from an external comment ID previously built by
// BuildExternalCommentID. Returns empty strings and -1 when the input is not
// a recognized codex external ID or the encoded form is malformed.
func ParseExternalCommentID(externalID string) (threadID, turnID string, commentOrdinal int) {
	prefix := "codex:turn:"
	if !strings.HasPrefix(externalID, prefix) {
		return "", "", -1
	}
	rest := strings.TrimPrefix(externalID, prefix)
	parts := strings.SplitN(rest, ":", 3)
	if len(parts) < 3 {
		return "", "", -1
	}
	rawThreadID := parts[0]
	rawTurnID := parts[1]
	var err error
	threadID, err = url.QueryUnescape(rawThreadID)
	if err != nil {
		return "", "", -1
	}
	turnID, err = url.QueryUnescape(rawTurnID)
	if err != nil {
		return "", "", -1
	}
	if threadID == "" || turnID == "" {
		return "", "", -1
	}
	if idx, err := strconv.Atoi(parts[2]); err == nil {
		if idx < 0 {
			return "", "", -1
		}
		commentOrdinal = idx
		return
	}
	return "", "", -1
}

// ---------------------------------------------------------------------------
// Publish message formatting
//
// The builtin Codex provider publishes a review by sending a structured user
// message via turn/start. The message includes a summary, the overall
// decision, and inline comments with file paths and ranges.
// ---------------------------------------------------------------------------

// PublishMessage holds the structured review content to be delivered as a
// user message in a Codex turn.
type PublishMessage struct {
	// Summary is the human-readable review overview.
	Summary string
	// Decision is the overall review verdict.
	Decision string
	// Comments are the individual review comments.
	Comments []PublishComment
}

// PublishComment is a single review comment to be included in the publish
// message.
type PublishComment struct {
	// FilePath is the repository-relative path to the file.
	FilePath string
	// OldLineStart is the starting line number on the old (left) side.
	OldLineStart int
	// NewLineStart is the starting line number on the new (right) side.
	NewLineStart int
	// Body is the comment text.
	Body string
}

// FormatPublishMessage renders the review content as a structured text block.
// The format is a Markdown-ish document that the codex agent can interpret:
//
//	## Review Summary
//	<decision>
//
//	<summary>
//
//	## Comments
//
//	### File: path/to/file.go (line 42-55)
//	Comment body
//
//	### File: another/file.js (line 10)
//	Comment body
func FormatPublishMessage(msg PublishMessage) string {
	var b strings.Builder

	// Decision header
	decisionLabel := formatDecisionLabel(msg.Decision)
	b.WriteString("## Review: ")
	b.WriteString(decisionLabel)
	b.WriteString("\n\n")

	// Summary
	if msg.Summary != "" {
		b.WriteString(strings.TrimSpace(msg.Summary))
		b.WriteString("\n\n")
	}

	// Comments
	if len(msg.Comments) > 0 {
		b.WriteString("## Comments\n\n")
		for i, c := range msg.Comments {
			if i > 0 {
				b.WriteString("\n")
			}
			writeCommentBlock(&b, c)
		}
	}

	return strings.TrimSpace(b.String())
}

func formatDecisionLabel(decision string) string {
	switch strings.ToLower(decision) {
	case "approve":
		return "✅ Approve"
	case "request_changes", "request changes":
		return "🔧 Request Changes"
	case "comment":
		return "💬 Comment"
	default:
		if decision == "" {
			return "Review"
		}
		return decision
	}
}

func writeCommentBlock(b *strings.Builder, c PublishComment) {
	lineRef := formatLineRef(c.OldLineStart, c.NewLineStart)
	b.WriteString("### ")
	b.WriteString(c.FilePath)
	if lineRef != "" {
		b.WriteString(" (")
		b.WriteString(lineRef)
		b.WriteString(")")
	}
	b.WriteString("\n\n")
	b.WriteString(strings.TrimSpace(c.Body))
	b.WriteString("\n")
}

func formatLineRef(oldStart, newStart int) string {
	var parts []string
	if oldStart > 0 && newStart > 0 {
		parts = append(parts, fmt.Sprintf("old:%d", oldStart), fmt.Sprintf("new:%d", newStart))
	} else if oldStart > 0 {
		parts = append(parts, fmt.Sprintf("line %d", oldStart))
	} else if newStart > 0 {
		parts = append(parts, fmt.Sprintf("line %d", newStart))
	}
	return strings.Join(parts, ", ")
}

// BuildPublishMessage constructs a PublishMessage from the review components
// that phase-3 will receive via PublishReviewRequest.
//
// Parameters:
//   - summary: the review overview text
//   - decision: the overall verdict ("approve", "request_changes", "comment")
//   - comments: individual review comments with file paths, line numbers, and bodies
//
// Returns an empty message when all inputs are empty (callers should guard
// against trying to publish an empty review).
func BuildPublishMessage(summary, decision string, comments []PublishComment) PublishMessage {
	return PublishMessage{
		Summary:  strings.TrimSpace(summary),
		Decision: decision,
		Comments: comments,
	}
}

// PublishMessageIsEmpty returns true when the message has no substantive
// content to publish.
func PublishMessageIsEmpty(msg PublishMessage) bool {
	return strings.TrimSpace(msg.Summary) == "" &&
		strings.TrimSpace(msg.Decision) == "" &&
		len(msg.Comments) == 0
}
