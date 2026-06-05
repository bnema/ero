package tui

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"charm.land/glamour/v2"
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
		glamour.WithStandardStyle(string(theme)),
		glamour.WithWordWrap(width),
	)
}

func hashMarkdownInput(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func safeMarkdownFallback(input string) string {
	withoutEscapes := ansiEscapePattern.ReplaceAllString(input, "")
	withoutEscapes = strings.ReplaceAll(withoutEscapes, "\x1b", "")
	return strings.TrimSpace(withoutEscapes)
}
