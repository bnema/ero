package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"ero/internal/adapters/in/tui/theme"
	"ero/internal/ports"
)

type providerPickerState struct {
	open     bool
	selected int
	rows     []providerPickerRow
}

type providerPickerRow struct {
	Key          string
	Label        string
	PluginName   string
	PluginSource string
	Reason       string
	Active       bool
}

func (m Model) openProviderPicker() Model {
	m.providerPicker.open = true
	m.providerPicker.rows = m.providerPickerRows()
	m.providerPicker.selected = clampProviderPickerSelection(m.providerPicker.selected, len(m.providerPicker.rows))
	for i, row := range m.providerPicker.rows {
		if row.Active {
			m.providerPicker.selected = i
			break
		}
	}
	return m
}

func (m Model) closeProviderPicker() Model {
	m.providerPicker.open = false
	return m
}

func (m Model) updateProviderPicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.Keystroke() {
	case "esc":
		return m.closeProviderPicker(), nil
	case "up", "k":
		m.providerPicker.selected = clampProviderPickerSelection(m.providerPicker.selected-1, len(m.providerPicker.rows))
		return m, nil
	case "down", "j":
		m.providerPicker.selected = clampProviderPickerSelection(m.providerPicker.selected+1, len(m.providerPicker.rows))
		return m, nil
	case "enter":
		if len(m.providerPicker.rows) == 0 {
			return m.closeProviderPicker(), nil
		}
		key := m.providerPicker.rows[m.providerPicker.selected].Key
		m = m.closeProviderPicker()
		return m, m.switchActiveProviderCmd(key)
	case "alt+p":
		m = m.closeProviderPicker()
		return m, m.cycleProviderCmd()
	default:
		return m, nil
	}
}

func (m Model) cycleProviderCmd() tea.Cmd {
	rows := m.providerPickerRows()
	if len(rows) == 0 {
		return nil
	}
	next := 0
	for i, row := range rows {
		if row.Active {
			next = (i + 1) % len(rows)
			break
		}
	}
	return m.switchActiveProviderCmd(rows[next].Key)
}

func (m Model) providerPickerRows() []providerPickerRow {
	rows := make([]providerPickerRow, 0, len(m.providerCatalog))
	for _, descriptor := range m.providerCatalog {
		rows = append(rows, m.providerPickerRow(descriptor))
	}
	return rows
}

func (m Model) providerPickerRow(descriptor ports.ReviewProviderDescriptor) providerPickerRow {
	label := descriptor.Label
	if label == "" {
		label = descriptor.ContributionID
	}
	if label == "" {
		label = descriptor.Key
	}
	reason := ""
	if descriptor.Key == m.activeProviderKey && m.providerSyncState.LastError != "" {
		reason = m.providerSyncState.LastError
	}
	return providerPickerRow{
		Key:          descriptor.Key,
		Label:        label,
		PluginName:   descriptor.PluginName,
		PluginSource: descriptor.PluginSource,
		Reason:       reason,
		Active:       descriptor.Key == m.activeProviderKey,
	}
}

func clampProviderPickerSelection(selected, rowCount int) int {
	if rowCount <= 0 || selected < 0 {
		return 0
	}
	if selected >= rowCount {
		return rowCount - 1
	}
	return selected
}

func (m Model) renderProviderPickerOverlay(content string) string {
	width := max(m.width, 1)
	height := max(m.height, 1)
	pane := m.renderProviderPicker(width, height)
	return renderCenteredOverlay(content, pane, width, height, max((height-lipgloss.Height(pane))/2, 0))
}

func (m Model) renderProviderPicker(width, height int) string {
	// Keep an eight-column outer margin, a readable 36-column minimum, and a 76-column maximum.
	paneWidth := min(max(width-8, 36), 76)
	// Account for pane padding/chrome while preserving at least one content column.
	contentWidth := max(paneWidth-6, 1)
	lines := []string{theme.HelpPaneTitleStyle.Render("Provider"), theme.MutedStyle.Render("Active publish destination"), ""}
	rows := m.providerPicker.rows
	if len(rows) == 0 {
		lines = append(lines, theme.MutedStyle.Render("No providers discovered"))
	} else {
		for i, row := range rows {
			cursor := "  "
			if i == m.providerPicker.selected {
				cursor = "› "
			}
			state := "available"
			marker := "○"
			if row.Active {
				state = "active"
				marker = "●"
			}
			meta := strings.TrimSpace(strings.Join([]string{row.PluginName, row.PluginSource}, " "))
			line := fmt.Sprintf("%s%s %-12s %s", cursor, marker, row.Label, state)
			if meta != "" {
				line += " — " + meta
			}
			if row.Reason != "" {
				line += " (" + row.Reason + ")"
			}
			lines = append(lines, theme.HelpLabelStyle.Render(componentTruncate(line, contentWidth)))
		}
	}
	lines = append(lines, "", theme.HelpLabelStyle.Render("enter switch • alt+p cycle • esc close"))
	lines = fitProviderPickerLines(lines, max(height-2, 1))
	return theme.HelpPaneStyle.Width(paneWidth).Render(strings.Join(lines, "\n"))
}

func fitProviderPickerLines(lines []string, maxLines int) []string {
	if len(lines) <= maxLines {
		return lines
	}
	if maxLines <= 1 {
		return lines[:maxLines]
	}
	result := append([]string(nil), lines[:maxLines-1]...)
	result = append(result, lines[len(lines)-1])
	return result
}

func componentTruncate(s string, width int) string {
	trimmed := strings.TrimRight(s, " ")
	runes := []rune(trimmed)
	if width < 0 {
		width = 0
	}
	if len(runes) <= width {
		return trimmed
	}
	return string(runes[:width])
}
