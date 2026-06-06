package tui

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"charm.land/glamour/v2"
	glamouransi "charm.land/glamour/v2/ansi"

	"ero/internal/adapters/in/tui/theme"
)

type MarkdownTheme string

const (
	MarkdownThemeDark  MarkdownTheme = "dark"
	MarkdownThemeLight MarkdownTheme = "light"
)

type markdownTermRenderer interface {
	Render(markdown string) (string, error)
}

type markdownRendererFactory func(width int, theme MarkdownTheme) (markdownTermRenderer, error)

type MarkdownRenderer struct {
	factory   markdownRendererFactory
	renderers map[markdownRendererConfig]markdownTermRenderer
	entries   map[markdownRendererCacheKey]string
}

type markdownRendererConfig struct {
	Width int
	Theme MarkdownTheme
}

type markdownRendererCacheKey struct {
	InputHash string
	Width     int
	Theme     MarkdownTheme
}

func NewMarkdownRenderer() *MarkdownRenderer {
	return NewMarkdownRendererWithFactory(newGlamourTermRenderer)
}

func NewMarkdownRendererWithFactory(factory markdownRendererFactory) *MarkdownRenderer {
	if factory == nil {
		factory = newGlamourTermRenderer
	}
	return &MarkdownRenderer{
		factory:   factory,
		renderers: map[markdownRendererConfig]markdownTermRenderer{},
		entries:   map[markdownRendererCacheKey]string{},
	}
}

func (r *MarkdownRenderer) Render(markdown string, width int, theme MarkdownTheme) string {
	if r == nil {
		return safeMarkdownFallback(markdown)
	}
	if width < 1 {
		width = 1
	}
	if theme != MarkdownThemeLight {
		theme = MarkdownThemeDark
	}
	if r.renderers == nil {
		r.renderers = map[markdownRendererConfig]markdownTermRenderer{}
	}
	if r.entries == nil {
		r.entries = map[markdownRendererCacheKey]string{}
	}

	key := markdownRendererCacheKey{InputHash: hashMarkdownInput(markdown), Width: width, Theme: theme}
	if rendered, ok := r.entries[key]; ok {
		return rendered
	}

	renderer, err := r.renderer(width, theme)
	if err != nil {
		return safeMarkdownFallback(markdown)
	}
	rendered, err := renderer.Render(markdown)
	if err != nil {
		return safeMarkdownFallback(markdown)
	}
	rendered = sanitizeRenderedMarkdown(rendered)
	r.entries[key] = rendered
	return rendered
}

func (r *MarkdownRenderer) renderer(width int, theme MarkdownTheme) (markdownTermRenderer, error) {
	config := markdownRendererConfig{Width: width, Theme: theme}
	if renderer, ok := r.renderers[config]; ok {
		return renderer, nil
	}
	renderer, err := r.factory(width, theme)
	if err != nil {
		return nil, err
	}
	r.renderers[config] = renderer
	return renderer, nil
}

func newGlamourTermRenderer(width int, theme MarkdownTheme) (markdownTermRenderer, error) {
	return glamour.NewTermRenderer(
		glamour.WithStyles(eroMarkdownStyle(theme)),
		glamour.WithWordWrap(width),
	)
}

func eroMarkdownStyle(markdownTheme MarkdownTheme) glamouransi.StyleConfig {
	text := theme.ColorStatusInfo
	muted := theme.ColorMutedText
	heading := theme.ColorAccent
	section := theme.ColorWarning
	codeBackground := theme.ColorCodeBg
	if markdownTheme == MarkdownThemeLight {
		text = theme.ColorStatusBase
		muted = "244"
		codeBackground = "#f6f8fa"
	}
	bold := true
	italic := true
	underline := true
	zeroIndent := uint(0)
	quoteIndent := uint(1)
	quoteToken := "│ "
	return glamouransi.StyleConfig{
		Document:    glamouransi.StyleBlock{StylePrimitive: glamouransi.StylePrimitive{Color: &text}},
		Text:        glamouransi.StylePrimitive{Color: &text},
		Paragraph:   glamouransi.StyleBlock{StylePrimitive: glamouransi.StylePrimitive{Color: &text}},
		Heading:     glamouransi.StyleBlock{StylePrimitive: glamouransi.StylePrimitive{Color: &heading, Bold: &bold}},
		H1:          glamouransi.StyleBlock{StylePrimitive: glamouransi.StylePrimitive{Color: &heading, Bold: &bold}},
		H2:          glamouransi.StyleBlock{StylePrimitive: glamouransi.StylePrimitive{Color: &section, Bold: &bold}},
		H3:          glamouransi.StyleBlock{StylePrimitive: glamouransi.StylePrimitive{Color: &section, Bold: &bold}},
		Strong:      glamouransi.StylePrimitive{Color: &section, Bold: &bold},
		Emph:        glamouransi.StylePrimitive{Color: &text, Italic: &italic},
		Item:        glamouransi.StylePrimitive{Color: &text},
		Enumeration: glamouransi.StylePrimitive{Color: &muted},
		Link:        glamouransi.StylePrimitive{Color: &heading, Underline: &underline},
		LinkText:    glamouransi.StylePrimitive{Color: &heading, Underline: &underline},
		Code:        glamouransi.StyleBlock{StylePrimitive: glamouransi.StylePrimitive{Color: &section, BackgroundColor: &codeBackground}},
		CodeBlock:   glamouransi.StyleCodeBlock{StyleBlock: glamouransi.StyleBlock{StylePrimitive: glamouransi.StylePrimitive{Color: &text, BackgroundColor: &codeBackground}, Margin: &zeroIndent}, Theme: "github-dark"},
		BlockQuote:  glamouransi.StyleBlock{StylePrimitive: glamouransi.StylePrimitive{Color: &muted}, Indent: &quoteIndent, IndentToken: &quoteToken},
	}
}

func hashMarkdownInput(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}

var (
	ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	oscEscapePattern  = regexp.MustCompile(`\x1b\][^\x1b\x07]*(?:\x07|\x1b\\)`)
)

func sanitizeRenderedMarkdown(input string) string {
	return oscEscapePattern.ReplaceAllString(input, "")
}

func safeMarkdownFallback(input string) string {
	withoutEscapes := sanitizeRenderedMarkdown(input)
	withoutEscapes = ansiEscapePattern.ReplaceAllString(withoutEscapes, "")
	withoutEscapes = strings.ReplaceAll(withoutEscapes, "\x1b", "")
	return strings.TrimSpace(withoutEscapes)
}
