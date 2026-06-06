package component

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"ero/internal/adapters/in/tui/theme"
	"ero/internal/core"
)

type StatusModel struct {
	AppName             string
	Mode                string
	FileCount           int
	ProviderCount       int
	ProviderSwitch      bool
	CurrentFile         string
	Message             string
	ScrollPercent       float64
	ActiveProviderLabel string
	ActiveRuntimeName   string
	ProviderSync        core.ProviderSyncState
	DraftCommentCount   int
	ShowNoProvider      bool
	NerdFont            bool
}

type StatusBar struct {
	width int
}

func NewStatusBar(width int) StatusBar {
	return StatusBar{width: width}
}

func (c StatusBar) Render(model StatusModel) string {
	width := max(c.width, 1)
	right := renderStatusHint(width, model.ProviderCount, model.ProviderSwitch)
	leftWidth := max(width-lipgloss.Width(right)-1, 0)

	segments := []statusSegment{
		{style: theme.StatusAppStyle, label: model.AppName},
		{style: theme.StatusModeStyle, label: model.Mode},
		{style: theme.StatusInfoStyle, label: fileCountLabel(model.FileCount)},
	}
	if model.ProviderCount > 0 && (strings.TrimSpace(model.ActiveProviderLabel) == "" || !model.NerdFont) {
		segments = append(segments, statusSegment{style: theme.StatusInfoStyle, label: providerCountLabel(model.ProviderCount)})
	}
	if syncSegment := providerSyncSegment(model); syncSegment.label != "" || syncSegment.rendered != "" {
		segments = append(segments, syncSegment)
	}
	if model.DraftCommentCount > 0 {
		segments = append(segments, statusSegment{style: theme.StatusInfoStyle, label: draftCommentCountLabel(model.DraftCommentCount)})
	}
	prefix := renderStatusSegments(leftWidth, segments...)
	percent := renderStatusSegments(leftWidth-lipgloss.Width(prefix), statusSegment{style: theme.StatusInfoStyle, label: fmt.Sprintf("%3.0f%%", model.ScrollPercent*100)})

	middleLabel := model.CurrentFile
	if model.Message != "" {
		middleLabel = " " + model.Message
	}
	middleWidth := leftWidth - lipgloss.Width(prefix) - lipgloss.Width(percent)
	middle := ""
	if middleLabel != "" && middleWidth > 0 {
		middle = renderStatusSegments(middleWidth, statusSegment{style: theme.StatusInfoStyle, label: middleLabel})
	}
	left := prefix + middle + percent
	gap := max(width-lipgloss.Width(left)-lipgloss.Width(right), 0)
	bar := left + theme.StatusBaseStyle.Render(strings.Repeat(" ", gap)) + right
	return theme.StatusBaseStyle.Width(width).Render(bar)
}

type statusSegment struct {
	style    lipgloss.Style
	label    string
	rendered string
}

type KeyHint struct {
	Key   string
	Label string
}

func renderStatusHint(width, providerCount int, providerSwitch bool) string {
	hints := []KeyHint{{Key: "?", Label: "help"}}
	if providerCount > 0 {
		hints = []KeyHint{{Key: "P", Label: "publish"}, {Key: "?", Label: "help"}}
		if providerSwitch {
			hints = []KeyHint{{Key: "p", Label: "provider"}, {Key: "P", Label: "publish"}, {Key: "?", Label: "help"}}
		}
	}
	full := RenderKeyHints(hints)
	if lipgloss.Width(full) <= width {
		return full
	}
	fallback := "? help"
	if providerCount > 0 {
		fallback = "P publish"
		if providerSwitch {
			fallback = "p provider"
		}
	}
	return theme.StatusInfoStyle.Render(TruncateRunes(fallback, max(width-theme.StatusInfoStyle.GetHorizontalPadding(), 0)))
}

func renderStatusSegments(width int, segments ...statusSegment) string {
	var rendered strings.Builder
	for _, segment := range segments {
		used := lipgloss.Width(rendered.String())
		remaining := width - used
		if remaining <= 0 {
			break
		}
		if segment.rendered != "" {
			rendered.WriteString(ansi.Truncate(segment.rendered, remaining, "…"))
			continue
		}
		padding := segment.style.GetHorizontalPadding()
		labelWidth := remaining - padding
		if labelWidth <= 0 {
			continue
		}
		rendered.WriteString(segment.style.Render(TruncateRunes(segment.label, labelWidth)))
	}
	return rendered.String()
}

func RenderKeyHints(hints []KeyHint) string {
	parts := make([]string, 0, len(hints))
	for _, hint := range hints {
		parts = append(parts, theme.StatusKeyStyle.Render(hint.Key)+theme.StatusHintTextStyle.Render(" "+hint.Label))
	}
	return theme.StatusBaseStyle.Render(strings.Join(parts, theme.StatusHintTextStyle.Render("  ")))
}

func fileCountLabel(count int) string {
	if count == 1 {
		return "1 file"
	}
	return fmt.Sprintf("%d files", count)
}

func providerCountLabel(count int) string {
	if count == 1 {
		return "1 provider"
	}
	return fmt.Sprintf("%d providers", count)
}

const (
	nerdFontGitHubLarge = "\uf113" // nf-fa-github_alt
	nerdFontSyncDot     = "\u25cf"
)

func providerSyncSegment(model StatusModel) statusSegment {
	if model.NerdFont && strings.TrimSpace(model.ActiveProviderLabel) != "" {
		return statusSegment{rendered: renderNerdFontProviderSync(model)}
	}
	label := providerSyncLabel(model)
	if label == "" {
		return statusSegment{}
	}
	return statusSegment{style: theme.StatusInfoStyle, label: label}
}

func providerSyncLabel(model StatusModel) string {
	provider := strings.TrimSpace(model.ActiveProviderLabel)
	if provider == "" {
		if model.ShowNoProvider {
			return "no provider"
		}
		return ""
	}
	if runtimeName := strings.TrimSpace(model.ActiveRuntimeName); runtimeName != "" && runtimeName != provider {
		provider += "/" + runtimeName
	}

	parts := []string{provider}
	status := providerSyncStatusLabel(model.ProviderSync.Status)
	if status != "" {
		parts = append(parts, status)
	}
	if model.ProviderSync.LastError != "" {
		parts = append(parts, TruncateRunes(model.ProviderSync.LastError, 24))
	}
	if model.ProviderSync.LastSyncAt != nil {
		parts = append(parts, "last "+formatStatusTime(*model.ProviderSync.LastSyncAt))
	}
	if model.ProviderSync.NextSyncAt != nil {
		parts = append(parts, "next "+formatStatusTime(*model.ProviderSync.NextSyncAt))
	}
	return strings.Join(parts, " ")
}

func draftCommentCountLabel(count int) string {
	if count == 1 {
		return "1 draft comment"
	}
	return fmt.Sprintf("%d draft comments", count)
}

func renderNerdFontProviderSync(model StatusModel) string {
	provider := strings.TrimSpace(model.ActiveProviderLabel)
	if runtimeName := strings.TrimSpace(model.ActiveRuntimeName); runtimeName != "" && runtimeName != provider {
		provider += "/" + runtimeName
	}
	var b strings.Builder
	b.WriteString(theme.StatusBaseStyle.Render(" "))
	b.WriteString(theme.StatusBaseStyle.Foreground(lipgloss.Color("248")).Render(providerGlyph(provider)))
	b.WriteString(theme.StatusBaseStyle.Render(" "))
	b.WriteString(theme.StatusBaseStyle.Foreground(providerStatusDotColor(model.ProviderSync.Status)).Render(nerdFontSyncDot))
	for _, part := range nerdFontProviderSyncTextParts(model) {
		b.WriteString(theme.StatusBaseStyle.Render(" "))
		b.WriteString(theme.StatusBaseStyle.Foreground(lipgloss.Color("248")).Render(part))
	}
	b.WriteString(theme.StatusBaseStyle.Render(" "))
	return b.String()
}

func nerdFontProviderSyncTextParts(model StatusModel) []string {
	parts := []string{}
	if model.ProviderCount > 1 {
		parts = append(parts, fmt.Sprintf("+%d", model.ProviderCount-1))
	}
	if model.ProviderSync.LastError != "" {
		parts = append(parts, TruncateRunes(model.ProviderSync.LastError, 24))
	}
	if model.ProviderSync.LastSyncAt != nil {
		parts = append(parts, "last "+formatStatusTime(*model.ProviderSync.LastSyncAt))
	}
	if model.ProviderSync.NextSyncAt != nil {
		parts = append(parts, "next "+formatStatusTime(*model.ProviderSync.NextSyncAt))
	}
	return parts
}

func providerGlyph(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if strings.Contains(provider, "github") {
		return nerdFontGitHubLarge
	}
	return providerAbbreviation(provider)
}

func providerStatusDotColor(status core.ProviderSyncStatus) color.Color {
	switch status {
	case core.ProviderSyncStatusSynced:
		return lipgloss.Color("#3fb950")
	case core.ProviderSyncStatusFailed:
		return lipgloss.Color("#ff7b72")
	case core.ProviderSyncStatusBackingOff:
		return lipgloss.Color("#ffa657")
	case core.ProviderSyncStatusLoadingCache, core.ProviderSyncStatusSyncing:
		return lipgloss.Color("#58a6ff")
	default:
		return lipgloss.Color("81")
	}
}

func providerAbbreviation(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return "?"
	}
	switch {
	case strings.Contains(provider, "github"):
		return "gh"
	case strings.Contains(provider, "gitlab"):
		return "gl"
	case strings.Contains(provider, "bitbucket"):
		return "bb"
	case strings.Contains(provider, "forgejo"):
		return "fj"
	case strings.Contains(provider, "gitea"):
		return "gt"
	}
	runes := []rune(provider)
	if len(runes) > 2 {
		runes = runes[:2]
	}
	return string(runes)
}

func providerSyncStatusLabel(status core.ProviderSyncStatus) string {
	switch status {
	case core.ProviderSyncStatusLoadingCache:
		return "cache"
	case core.ProviderSyncStatusSyncing:
		return "syncing"
	case core.ProviderSyncStatusSynced:
		return "synced"
	case core.ProviderSyncStatusFailed:
		return "failed"
	case core.ProviderSyncStatusBackingOff:
		return "backoff"
	default:
		return ""
	}
}

func formatStatusTime(value time.Time) string {
	return value.UTC().Format("15:04")
}

func TruncateRunes(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(value) <= width {
		return value
	}
	return ansi.Truncate(value, width, "…")
}
