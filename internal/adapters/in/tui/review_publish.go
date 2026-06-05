package tui

import (
	"context"
	"fmt"
	"maps"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/bnema/zerowrap"

	"ero/internal/adapters/in/tui/theme"

	"ero/internal/core"
)

type publishState struct {
	active                 bool
	selected               map[string]bool
	focused                int
	publishing             bool
	message                string
	unsupportedDecision    map[string]core.ReviewDecision
	confirmWithoutDecision map[string]bool
}

type publishReviewCompletedMsg struct {
	results []core.PublishReviewResult
	errors  map[string]error
}

func (m Model) openPublishReview() (Model, tea.Cmd) {
	log := zerowrap.FromCtx(m.ctx)
	if len(m.providerInfos) == 0 {
		log.Warn().Msg("publish requested with no review providers available")
		m.setCopyFeedback("No review providers available")
		return m, nil
	}
	focused := firstPublishableProviderIndex(m.providerInfos)
	selected := make(map[string]bool, len(m.providerInfos))
	if focused >= 0 && focused < len(m.providerInfos) && m.providerInfos[focused].Capabilities.PublishReview {
		selected[m.providerInfos[focused].ID] = true
	}
	log.Info().Int("provider_count", len(m.providerInfos)).Msg("publish overlay opened")
	m.publish = publishState{
		active:                 true,
		selected:               selected,
		focused:                focused,
		unsupportedDecision:    map[string]core.ReviewDecision{},
		confirmWithoutDecision: map[string]bool{},
		message:                "Select a destination, then press enter to publish.",
	}
	return m, nil
}

func (m Model) updatePublishReview(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.publish = publishState{}
		return m, nil
	case "up", "k":
		m.movePublishFocus(-1)
		return m, nil
	case "down", "j":
		m.movePublishFocus(1)
		return m, nil
	case "space":
		m.toggleFocusedPublishProvider()
		return m, nil
	case "enter":
		return m.publishSelectedProviders()
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		idx := int(msg.String()[0] - '1')
		if idx >= 0 && idx < len(m.providerInfos) {
			m.publish.focused = idx
			m.togglePublishProvider(idx)
		}
	}
	return m, nil
}

func (m Model) publishSelectedProviders() (Model, tea.Cmd) {
	if m.publish.publishing {
		return m, nil
	}
	if m.reviewDraft == nil {
		m.reviewDraft = core.NewReviewDraft()
	}
	selectedInfos := m.selectedPublishProviderInfos()
	log := zerowrap.FromCtx(m.ctx)
	if len(selectedInfos) == 0 {
		log.Warn().Msg("publish attempted with no selected providers")
		m.publish.message = "Select at least one provider"
		return m, nil
	}
	decision := m.reviewDraft.Decision()
	m.publish.unsupportedDecision = map[string]core.ReviewDecision{}
	for _, info := range selectedInfos {
		if !info.Capabilities.PublishReview {
			log.Warn().Str("provider_id", info.ID).Msg("publish attempted for provider without publish capability")
			m.publish.message = fmt.Sprintf("%s does not support publishing", providerDisplayLabel(info))
			return m, nil
		}
		if !info.Capabilities.SupportsDecision(decision) && !m.publish.confirmWithoutDecision[info.ID] {
			m.publish.unsupportedDecision[info.ID] = decision
		}
	}
	if len(m.publish.unsupportedDecision) > 0 {
		log.Warn().Str("decision", string(decision)).Any("provider_ids", mapKeys(m.publish.unsupportedDecision)).Msg("publish decision unsupported by selected providers")
		m.publish.message = "Decision unsupported by selected provider; press enter again to publish without decision"
		if m.publish.confirmWithoutDecision == nil {
			m.publish.confirmWithoutDecision = map[string]bool{}
		}
		for id := range m.publish.unsupportedDecision {
			m.publish.confirmWithoutDecision[id] = true
		}
		return m, nil
	}
	providers := append([]core.ReviewProviderInfo(nil), selectedInfos...)
	clients := append([]providerClientWithInfo(nil), m.providerClientsFor(providers)...)
	reviewCtx := m.reviewContext
	cmdCtx := m.ctx
	draft := m.reviewDraft.Snapshot()
	confirmWithoutDecision := copyBoolMap(m.publish.confirmWithoutDecision)
	m.publish.publishing = true
	m.publish.message = "Publishing review…"
	providerIDs := make([]string, 0, len(providers))
	for _, info := range providers {
		providerIDs = append(providerIDs, info.ID)
	}
	log.Info().Any("provider_ids", providerIDs).Int("comment_count", len(draft.Comments)).Str("decision", string(draft.Decision)).Msg("publishing review")
	return m, func() tea.Msg {
		cmdLog := zerowrap.FromCtx(cmdCtx)
		results := make([]core.PublishReviewResult, 0, len(clients))
		errs := map[string]error{}
		for _, item := range clients {
			payload := draft
			if confirmWithoutDecision[item.info.ID] {
				payload.Decision = ""
			}
			result, err := item.client.PublishReview(cmdCtx, core.PublishReviewRequest{ProviderID: item.info.ID, Context: reviewCtx, Draft: payload})
			if err != nil {
				cmdLog.Error().Err(err).Str("provider_id", item.info.ID).Msg("publish provider failed")
				errs[item.info.ID] = err
				continue
			}
			cmdLog.Info().Str("provider_id", item.info.ID).Str("external_review_id", result.ExternalReviewID).Bool("ambiguous", result.Ambiguous).Msg("publish provider succeeded")
			results = append(results, result)
		}
		return publishReviewCompletedMsg{results: results, errors: errs}
	}
}

func (m Model) handlePublishReviewCompleted(msg publishReviewCompletedMsg) (Model, tea.Cmd) {
	m.publish.publishing = false
	if len(msg.results) > 0 {
		m.applyPublishedRefs(msg.results)
	}
	ambiguous := ambiguousProviderIDs(msg.results)
	log := zerowrap.FromCtx(m.ctx)
	if len(msg.errors) > 0 || len(ambiguous) > 0 {
		parts := make([]string, 0, len(msg.errors)+len(ambiguous)+1)
		if len(msg.results) > 0 {
			parts = append(parts, fmt.Sprintf("published to %d %s", len(msg.results), pluralize("provider", len(msg.results))))
		}
		for id, err := range msg.errors {
			parts = append(parts, id+": "+err.Error())
		}
		if len(ambiguous) > 0 {
			parts = append(parts, "ambiguous: "+strings.Join(ambiguous, ", "))
		}
		log.Warn().Int("result_count", len(msg.results)).Int("error_count", len(msg.errors)).Any("ambiguous_provider_ids", ambiguous).Msg("publish completed with errors")
		m.publish.message = "Partial publish: " + strings.Join(parts, "; ")
		m.setCopyFeedback(m.publish.message)
		m.syncReviewViewport()
		return m, m.expireCopyFeedbackCmd()
	}
	log.Info().Int("result_count", len(msg.results)).Msg("publish completed successfully")
	m.publish = publishState{}
	m.setCopyFeedback(fmt.Sprintf("Published review to %d %s", len(msg.results), pluralize("provider", len(msg.results))))
	m.syncReviewViewport()
	return m, m.expireCopyFeedbackCmd()
}

func ambiguousProviderIDs(results []core.PublishReviewResult) []string {
	ids := make([]string, 0)
	for _, result := range results {
		if result.Ambiguous {
			ids = append(ids, result.ProviderID)
		}
	}
	return ids
}

func (m *Model) movePublishFocus(delta int) {
	if len(m.providerInfos) == 0 || delta == 0 || m.publish.publishing {
		return
	}
	m.publish.focused = (m.publish.focused + delta + len(m.providerInfos)) % len(m.providerInfos)
}

func (m *Model) toggleFocusedPublishProvider() {
	m.togglePublishProvider(m.publish.focused)
}

func (m *Model) togglePublishProvider(idx int) {
	if m.publish.publishing || idx < 0 || idx >= len(m.providerInfos) {
		return
	}
	if m.publish.selected == nil {
		m.publish.selected = map[string]bool{}
	}
	id := m.providerInfos[idx].ID
	if m.publish.selected[id] {
		delete(m.publish.selected, id)
		return
	}
	m.publish.selected[id] = true
}

func firstPublishableProviderIndex(infos []core.ReviewProviderInfo) int {
	for i, info := range infos {
		if info.Capabilities.PublishReview {
			return i
		}
	}
	return 0
}

func (m Model) selectedPublishProviderInfos() []core.ReviewProviderInfo {
	infos := make([]core.ReviewProviderInfo, 0)
	for _, info := range m.providerInfos {
		if m.publish.selected[info.ID] {
			infos = append(infos, info)
		}
	}
	return infos
}

type providerClientWithInfo struct {
	info   core.ReviewProviderInfo
	client reviewProviderClient
}

type reviewProviderClient interface {
	PublishReview(context.Context, core.PublishReviewRequest) (core.PublishReviewResult, error)
}

func (m Model) providerClientsFor(infos []core.ReviewProviderInfo) []providerClientWithInfo {
	selected := make(map[string]core.ReviewProviderInfo, len(infos))
	for _, info := range infos {
		selected[info.ID] = info
	}
	result := make([]providerClientWithInfo, 0, len(infos))
	if m.activeProvider != nil && m.activeRuntimeInfo.ID != "" {
		if info, ok := selected[m.activeRuntimeInfo.ID]; ok {
			return append(result, providerClientWithInfo{info: info, client: m.activeProvider})
		}
	}
	for _, client := range m.reviewProviders {
		providerInfo, ok := m.providerInfoByClient[client]
		if !ok {
			continue
		}
		if info, ok := selected[providerInfo.ID]; ok {
			result = append(result, providerClientWithInfo{info: info, client: client})
		}
	}
	return result
}

func (m *Model) applyPublishedRefs(results []core.PublishReviewResult) {
	if m.reviewDraft == nil {
		return
	}
	for _, result := range results {
		m.reviewDraft.ApplyPublishedRefs(result.ProviderID, result.PublishedRefs)
	}
}

func copyBoolMap(input map[string]bool) map[string]bool {
	out := make(map[string]bool, len(input))
	maps.Copy(out, input)
	return out
}

func mapKeys[K comparable, V any](input map[K]V) []K {
	keys := make([]K, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	return keys
}

func (m Model) renderPublishOverlay(content string) string {
	width := max(m.width, 1)
	height := max(m.height, 1)
	pane := m.renderPublishPane(width)
	return renderCenteredOverlay(content, pane, width, height, max((height-lipgloss.Height(pane))/2, 0))
}

func (m Model) renderPublishPane(width int) string {
	var b strings.Builder
	b.WriteString("Publish review\n")
	if m.publish.message != "" {
		b.WriteString(theme.HelpLabelStyle.Render(m.publish.message))
		b.WriteString("\n\n")
	}
	for i, info := range m.providerInfos {
		b.WriteString(m.renderPublishProviderRow(i, info, width))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	if m.publish.publishing {
		b.WriteString(theme.StatusInfoStyle.Render("Publishing… waiting for provider response"))
		b.WriteString("\n")
	}
	b.WriteString(theme.MutedStyle.Render("↑↓/j/k move · space select · enter publish · esc cancel"))
	return theme.SearchPaneStyle.Width(min(max(width-8, 32), 76)).Render(b.String())
}

func (m Model) renderPublishProviderRow(index int, info core.ReviewProviderInfo, width int) string {
	selected := m.publish.selected[info.ID]
	focused := index == m.publish.focused
	mark := "○"
	if selected {
		mark = "●"
	}
	label := providerDisplayLabel(info)
	capability := "publish"
	if !info.Capabilities.PublishReview {
		capability = "read-only"
	}
	rowWidth := min(max(width-12, 24), 72)
	row := fmt.Sprintf("%d  %s  %s", index+1, mark, label)
	if decision, ok := m.publish.unsupportedDecision[info.ID]; ok {
		row += "  does not support " + string(decision)
	} else {
		row += "  " + capability
	}
	row = truncatePlainRow(row, rowWidth)
	if focused {
		return theme.SearchSelectedRowStyle.Width(rowWidth).Render(row)
	}
	if selected {
		return theme.HelpLabelStyle.Render(row)
	}
	return theme.MutedStyle.Render(row)
}

func truncatePlainRow(row string, width int) string {
	if lipgloss.Width(row) <= width {
		return row
	}
	return string([]rune(row)[:max(width-1, 0)]) + "…"
}

func providerDisplayLabel(info core.ReviewProviderInfo) string {
	if info.Label != "" {
		return info.Label
	}
	if info.Name != "" {
		return info.Name
	}
	return info.ID
}
