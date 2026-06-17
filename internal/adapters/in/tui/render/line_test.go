package render

import (
	"regexp"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"

	"ero/internal/adapters/in/tui/theme"
	"ero/internal/core"
)

func TestReviewLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		line        core.ReviewLine
		contains    []string
		notContains []string
		stripped    string
	}{
		{
			name: "applies syntax highlighting and line numbers",
			line: core.ReviewLine{
				OldLineNumber: 4,
				NewLineNumber: 5,
				Content:       "func main() {}",
				Kind:          core.LineKindAdded,
				SyntaxTokens: []core.SyntaxToken{
					{Start: 0, End: 4, Type: core.SemanticTokenKeyword},
					{Start: 5, End: 9, Type: core.SemanticTokenFunction},
				},
			},
			contains: []string{"   4", "   5", "+", "func", "main", "\x1b["},
		},
		{
			name: "preserves diff background with chroma syntax foregrounds",
			line: core.ReviewLine{
				NewLineNumber: 5,
				Content:       "func main() {}",
				Kind:          core.LineKindAdded,
				SyntaxTokens: []core.SyntaxToken{
					{Start: 0, End: 4, Type: core.SemanticTokenText, SourceType: "KeywordDeclaration"},
					{Start: 5, End: 9, Type: core.SemanticTokenText, SourceType: "NameFunction"},
				},
			},
			contains:    []string{"48;2;1;18;9", "38;2;255;123;114", "38;2;210;168;255"},
			notContains: []string{"48;2;3;47;23", "48;2;218;251;225"},
			stripped:    "+ func main() {}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rendered := ReviewLine(tt.line, 4)
			for _, expected := range tt.contains {
				assert.Contains(t, rendered, expected)
			}
			for _, unexpected := range tt.notContains {
				assert.NotContains(t, rendered, unexpected)
			}
			if tt.stripped != "" {
				assert.Contains(t, stripANSI(rendered), tt.stripped)
			}
		})
	}
}

func TestReviewLineUsesLightChromaStyleWhenThemeIsLight(t *testing.T) {
	originalAppearance := theme.CurrentAppearance()
	theme.ApplyAppearance(core.ThemeAppearanceLight)
	t.Cleanup(func() { theme.ApplyAppearance(originalAppearance) })

	rendered := ReviewLine(core.ReviewLine{
		NewLineNumber: 5,
		Content:       "func main() {}",
		Kind:          core.LineKindAdded,
		SyntaxTokens: []core.SyntaxToken{
			{Start: 0, End: 4, Type: core.SemanticTokenText, SourceType: "KeywordDeclaration"},
			{Start: 5, End: 9, Type: core.SemanticTokenText, SourceType: "NameFunction"},
		},
	}, 4)

	assert.Contains(t, rendered, "48;2;218;251;225")
	assert.Contains(t, rendered, "38;2;207;34;46")
	assert.Contains(t, rendered, "38;2;102;57;186")
	assert.NotContains(t, rendered, "48;2;1;18;9")
	assert.Contains(t, stripANSI(rendered), "+ func main() {}")
}

func TestApplySyntaxHighlightingSkipsAlreadyRenderedTokenRanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		tokens []core.SyntaxToken
		want   string
	}{
		{
			name: "overlapping token starts inside previous token",
			tokens: []core.SyntaxToken{
				{Start: 0, End: 4, Type: core.SemanticTokenKeyword},
				{Start: 2, End: 6, Type: core.SemanticTokenFunction},
			},
			want: "abcdef",
		},
		{
			name: "out of order token is skipped after later range rendered",
			tokens: []core.SyntaxToken{
				{Start: 2, End: 4, Type: core.SemanticTokenKeyword},
				{Start: 0, End: 2, Type: core.SemanticTokenFunction},
			},
			want: "abcdef",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rendered := ApplySyntaxHighlighting("abcdef", tt.tokens, lipgloss.NewStyle())

			assert.Equal(t, tt.want, stripANSI(rendered))
		})
	}
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}
