package tui

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

type fakeMarkdownTermRenderer struct {
	render func(string) (string, error)
}

func (r fakeMarkdownTermRenderer) Render(markdown string) (string, error) {
	return r.render(markdown)
}

func TestMarkdownRendererCachesByInputWidthAndTheme(t *testing.T) {
	calls := 0
	renderer := NewMarkdownRendererWithFactory(func(width int, theme MarkdownTheme) (markdownTermRenderer, error) {
		return fakeMarkdownTermRenderer{render: func(markdown string) (string, error) {
			calls++
			return string(theme) + ":" + markdown + ":" + string(rune(width)), nil
		}}, nil
	})

	if got := renderer.Render("**hello**", 80, MarkdownThemeDark); got == "**hello**" {
		t.Fatalf("expected rendered output, got fallback %q", got)
	}
	if got := renderer.Render("**hello**", 80, MarkdownThemeDark); got == "**hello**" {
		t.Fatalf("expected cached rendered output, got fallback %q", got)
	}
	if calls != 1 {
		t.Fatalf("expected identical input/width/theme to use cache, got %d render calls", calls)
	}

	renderer.Render("**hello**", 100, MarkdownThemeDark)
	renderer.Render("**hello**", 100, MarkdownThemeLight)
	renderer.Render("_hello_", 100, MarkdownThemeLight)

	if calls != 4 {
		t.Fatalf("expected width, theme, and input changes to invalidate cache, got %d render calls", calls)
	}
}

func TestMarkdownRendererColorsHeadingsAndFencedCodeBlocks(t *testing.T) {
	renderer := NewMarkdownRenderer()

	got := renderer.Render("## Validation\n\n```go\nfmt.Println(\"hi\")\n```", 80, MarkdownThemeDark)
	plain := regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`).ReplaceAllString(got, "")

	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("expected rendered markdown to include ANSI color/style escapes, got %q", got)
	}
	if !strings.Contains(plain, "Validation") || !strings.Contains(plain, "fmt.Println") {
		t.Fatalf("expected heading and code block content, got %q", got)
	}
	if strings.Contains(plain, "```") {
		t.Fatalf("expected rendered fenced code block to omit markdown fences, got %q", got)
	}
}

func TestMarkdownRendererRendersFencedCodeBlocks(t *testing.T) {
	renderer := NewMarkdownRenderer()

	got := renderer.Render("```go\nfmt.Println(\"hi\")\n```", 80, MarkdownThemeDark)
	plain := regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`).ReplaceAllString(got, "")

	if !strings.Contains(plain, "fmt.Println") {
		t.Fatalf("expected rendered fenced code block to include code, got %q", got)
	}
	if strings.Contains(plain, "```") {
		t.Fatalf("expected rendered fenced code block to omit markdown fences, got %q", got)
	}
}

func TestMarkdownRendererStripsOSC8Hyperlinks(t *testing.T) {
	renderer := NewMarkdownRendererWithFactory(func(width int, theme MarkdownTheme) (markdownTermRenderer, error) {
		return fakeMarkdownTermRenderer{render: func(markdown string) (string, error) {
			return "\x1b]8;id=abc;https://github.com/bnema/ero\x1b\\GitHub\x1b]8;;\x1b\\", nil
		}}, nil
	})

	got := renderer.Render("[GitHub](https://github.com/bnema/ero)", 80, MarkdownThemeDark)

	if strings.Contains(got, "]8;") || strings.Contains(got, "id=abc") || strings.Contains(got, "\x1b]") {
		t.Fatalf("expected OSC-8 hyperlink sequences to be stripped, got %q", got)
	}
	if !strings.Contains(got, "GitHub") {
		t.Fatalf("expected link text to remain, got %q", got)
	}
}

func TestMarkdownRendererReturnsSafeFallbackOnRenderError(t *testing.T) {
	renderer := NewMarkdownRendererWithFactory(func(width int, theme MarkdownTheme) (markdownTermRenderer, error) {
		return fakeMarkdownTermRenderer{render: func(markdown string) (string, error) {
			return "", errors.New("boom")
		}}, nil
	})

	got := renderer.Render("hello\x1b[31m \x1b]8;id=x;https://example.invalid\x1b\\**world**\x1b]8;;\x1b\\", 80, MarkdownThemeDark)

	if strings.Contains(got, "\x1b") || strings.Contains(got, "]8;") {
		t.Fatalf("expected fallback to strip escape characters, got %q", got)
	}
	if !strings.Contains(got, "hello") || !strings.Contains(got, "world") {
		t.Fatalf("expected fallback to preserve readable text, got %q", got)
	}
}
