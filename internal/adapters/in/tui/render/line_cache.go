package render

import (
	"fmt"
	"strings"

	"ero/internal/adapters/in/tui/presenter"
	"ero/internal/core"
)

type ReviewLineCache struct {
	render  func(core.ReviewLine, int) string
	entries map[reviewLineCacheKey]string
}

type reviewLineCacheKey struct {
	FileIndex     int
	SectionIndex  int
	LineIndex     int
	OldLineNumber int
	NewLineNumber int
	Kind          core.LineKind
	Content       string
	Tokens        string
	NumberWidth   int
}

func NewReviewLineCache() *ReviewLineCache {
	return NewReviewLineCacheWithRenderer(ReviewLine)
}

func NewReviewLineCacheWithRenderer(render func(core.ReviewLine, int) string) *ReviewLineCache {
	if render == nil {
		render = ReviewLine
	}
	return &ReviewLineCache{render: render, entries: map[reviewLineCacheKey]string{}}
}

func (c *ReviewLineCache) Render(row presenter.ReviewRow, numberWidth int) string {
	if c == nil {
		return ReviewLine(row.Line, numberWidth)
	}
	if c.entries == nil {
		c.entries = map[reviewLineCacheKey]string{}
	}
	key := newReviewLineCacheKey(row, numberWidth)
	if rendered, ok := c.entries[key]; ok {
		return rendered
	}
	rendered := c.render(row.Line, numberWidth)
	c.entries[key] = rendered
	return rendered
}

func (c *ReviewLineCache) Clear() {
	if c == nil {
		return
	}
	c.entries = map[reviewLineCacheKey]string{}
}

func newReviewLineCacheKey(row presenter.ReviewRow, numberWidth int) reviewLineCacheKey {
	line := row.Line
	return reviewLineCacheKey{
		FileIndex:     row.FileIndex,
		SectionIndex:  row.SectionIndex,
		LineIndex:     row.LineIndex,
		OldLineNumber: line.OldLineNumber,
		NewLineNumber: line.NewLineNumber,
		Kind:          line.Kind,
		Content:       line.Content,
		Tokens:        syntaxTokensKey(line.SyntaxTokens),
		NumberWidth:   numberWidth,
	}
}

func syntaxTokensKey(tokens []core.SyntaxToken) string {
	if len(tokens) == 0 {
		return ""
	}
	var b strings.Builder
	for _, token := range tokens {
		_, _ = fmt.Fprintf(&b, "%d:%d:%s:%s|", token.Start, token.End, token.Type, token.SourceType)
	}
	return b.String()
}
