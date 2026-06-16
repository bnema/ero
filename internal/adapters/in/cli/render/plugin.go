package render

import (
	"strings"

	"charm.land/lipgloss/v2"

	"ero/internal/ports"
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	badgeStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Padding(0, 1)
	keyStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
	mutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	okStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	warnStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
)

// PluginList renders available plugins for human CLI output.
func PluginList(plugins []ports.InstalledPlugin, width int) string {
	if len(plugins) == 0 {
		return mutedStyle.Render("No plugins available.")
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("Available plugins"))
	for _, plugin := range plugins {
		b.WriteString("\n\n")
		label := plugin.Name + " v" + plugin.Version
		b.WriteString(badgeStyle.Render(truncateRunes(label, max(width, 20))))
		if len(plugin.Contributions) > 0 {
			b.WriteString("\n")
			b.WriteString("  ")
			b.WriteString(keyStyle.Render("provides"))
			b.WriteString("  ")
			b.WriteString(truncateRunes(strings.Join(plugin.Contributions, ", "), max(width-12, 10)))
		}
		if source := pluginListSource(plugin); source != "" {
			b.WriteString("\n")
			b.WriteString("  ")
			b.WriteString(mutedStyle.Render("source"))
			b.WriteString("    ")
			b.WriteString(mutedStyle.Render(truncateRunes(source, max(width-12, 10))))
		}
		if plugin.Bundled {
			b.WriteString("\n")
			b.WriteString("  ")
			b.WriteString(mutedStyle.Render("managed"))
			b.WriteString("   ")
			b.WriteString(mutedStyle.Render("shipped"))
		}
	}
	return b.String()
}

func pluginListSource(plugin ports.InstalledPlugin) string {
	if plugin.Bundled && plugin.Path != "" {
		return plugin.Path
	}
	return plugin.Source
}

// PluginInstall renders a successful plugin install.
func PluginInstall(result ports.PluginInstallResult) string {
	return strings.Join([]string{
		okStyle.Render("✓ Installed plugin") + " " + titleStyle.Render(result.Name) + mutedStyle.Render(" v"+result.Version),
		field("source", result.Source),
		field("path", result.Path),
	}, "\n")
}

// PluginUpdates renders plugin update results.
func PluginUpdates(results []ports.PluginUpdateResult) string {
	if len(results) == 0 {
		return mutedStyle.Render("No plugins to update.")
	}
	lines := []string{titleStyle.Render("Plugin updates")}
	for _, result := range results {
		if result.Message != "" {
			lines = append(lines, warnStyle.Render("• ")+titleStyle.Render(result.Name)+mutedStyle.Render(" — "+result.Message))
			continue
		}
		lines = append(lines, okStyle.Render("• ")+titleStyle.Render(result.Name)+" "+mutedStyle.Render(shortSHA(result.PreviousRef)+" → "+shortSHA(result.UpdatedRef)))
	}
	return strings.Join(lines, "\n")
}

// PluginRemove renders a successful plugin removal.
func PluginRemove(result ports.PluginRemoveResult) string {
	action := "Removed plugin reference"
	if result.RemovedRepo {
		action = "Removed plugin"
	}
	return okStyle.Render("✓ "+action) + " " + titleStyle.Render(result.Name)
}

func field(label, value string) string {
	return "  " + mutedStyle.Render(label) + "  " + value
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func truncateRunes(value string, width int) string {
	if lipgloss.Width(value) <= width {
		return value
	}
	if width <= 1 {
		return "…"
	}
	runes := []rune(value)
	if len(runes) <= width-1 {
		return value
	}
	return string(runes[:width-1]) + "…"
}
