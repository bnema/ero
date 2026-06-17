package render

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	basechroma "github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"

	"ero/internal/adapters/in/tui/theme"
	"ero/internal/core"
)

func ReviewLine(line core.ReviewLine, lineNumberWidth int) string {
	oldNum := theme.LineNumberStyle.Render(formatLineNumber(line.OldLineNumber, lineNumberWidth))
	newNum := theme.LineNumberStyle.Render(formatLineNumber(line.NewLineNumber, lineNumberWidth))
	marker := " "
	markerStyle := lipgloss.NewStyle()
	lineStyle := lipgloss.NewStyle()

	switch line.Kind {
	case core.LineKindAdded:
		marker = "+"
		markerStyle = theme.AddedMarkerStyle
		lineStyle = theme.AddedLineStyle
	case core.LineKindDeleted:
		marker = "-"
		markerStyle = theme.DeletedMarkerStyle
		lineStyle = theme.DeletedLineStyle
	}

	content := applySyntaxHighlighting(line.Content, line.SyntaxTokens, lineStyle)
	return fmt.Sprintf("%s %s %s %s", oldNum, newNum, markerStyle.Inherit(lineStyle).Render(marker), content)
}

func ApplySyntaxHighlighting(content string, tokens []core.SyntaxToken, baseStyle lipgloss.Style) string {
	return applySyntaxHighlighting(content, tokens, baseStyle)
}

func applySyntaxHighlighting(content string, tokens []core.SyntaxToken, baseStyle lipgloss.Style) string {
	if len(tokens) == 0 {
		return baseStyle.Render(content)
	}

	runes := []rune(content)
	var result strings.Builder
	lastEnd := 0
	background := baseStyle.GetBackground()
	chroma := chromaStyle()

	for _, token := range tokens {
		start := min(max(token.Start, 0), len(runes))
		end := min(max(token.End, 0), len(runes))
		start = max(start, lastEnd)
		if end <= lastEnd || start >= end {
			continue
		}
		if start > lastEnd {
			result.WriteString(baseStyle.Render(string(runes[lastEnd:start])))
		}
		tokenStyle := styleForSyntaxToken(token, chroma)
		if background != nil {
			tokenStyle = tokenStyle.Background(background)
		}
		result.WriteString(tokenStyle.Render(string(runes[start:end])))
		lastEnd = end
	}

	if lastEnd < len(runes) {
		result.WriteString(baseStyle.Render(string(runes[lastEnd:])))
	}

	return result.String()
}

func StyleForSyntaxToken(token core.SyntaxToken) lipgloss.Style {
	return styleForSyntaxToken(token, chromaStyle())
}

func styleForSyntaxToken(token core.SyntaxToken, chroma *basechroma.Style) lipgloss.Style {
	if token.SourceType != "" {
		if tokenType, err := basechroma.TokenTypeString(token.SourceType); err == nil {
			entry := chroma.Get(tokenType)
			style := lipgloss.NewStyle()
			if entry.Colour.IsSet() {
				style = style.Foreground(lipgloss.Color(entry.Colour.String()))
			}
			if entry.Bold == basechroma.Yes {
				style = style.Bold(true)
			}
			if entry.Italic == basechroma.Yes {
				style = style.Italic(true)
			}
			if entry.Underline == basechroma.Yes {
				style = style.Underline(true)
			}
			return style
		}
	}
	return StyleForToken(token.Type)
}

func chromaStyle() *basechroma.Style {
	style := styles.Get(theme.CurrentPalette().ChromaStyle)
	if style == nil {
		return styles.Fallback
	}
	return style
}

func StyleForToken(tokenType core.SemanticTokenType) lipgloss.Style {
	switch tokenType {
	case core.SemanticTokenKeyword:
		return theme.KeywordStyle
	case core.SemanticTokenFunction:
		return theme.FunctionStyle
	case core.SemanticTokenTypeName:
		return theme.TypeStyle
	case core.SemanticTokenName:
		return theme.NameStyle
	case core.SemanticTokenString:
		return theme.StringStyle
	case core.SemanticTokenNumber:
		return theme.NumberStyle
	case core.SemanticTokenComment:
		return theme.CommentStyle
	case core.SemanticTokenOperator:
		return theme.OperatorStyle
	case core.SemanticTokenPunctuation:
		return theme.PunctuationStyle
	default:
		return lipgloss.NewStyle()
	}
}

func LineNumberWidth(file core.ReviewFile) int {
	width := 4
	for _, section := range file.Sections {
		for _, line := range section.Lines {
			if digits := len(fmt.Sprintf("%d", max(line.OldLineNumber, line.NewLineNumber))); digits > width {
				width = digits
			}
		}
	}
	return width
}

func formatLineNumber(number, width int) string {
	if number <= 0 {
		return strings.Repeat(" ", width)
	}
	return fmt.Sprintf("%*d", width, number)
}
