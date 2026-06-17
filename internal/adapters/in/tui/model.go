package tui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"ero/internal/adapters/in/tui/keymap"
	"ero/internal/adapters/in/tui/presenter"
	"ero/internal/adapters/in/tui/render"
	"ero/internal/adapters/in/tui/theme"
	"ero/internal/core"
	"ero/internal/ports"
)

const (
	defaultWidth           = 100
	defaultHeight          = 24
	contextStep            = 10
	themeDetectionInterval = 2 * time.Second
)

type ModelConfig struct {
	ThemeMode        core.ThemeMode
	ThemeModeChanges <-chan core.ThemeMode
}

type reviewLoader interface {
	LoadReview(request core.ReviewRequest) ([]core.ReviewFile, error)
}

type reviewLoadedMsg struct {
	mode  core.DiffMode
	files []core.ReviewFile
}

type reviewLoadFailedMsg struct {
	previousMode core.DiffMode
	err          error
}

type copyFeedbackExpiredMsg struct {
	id int
}

type themeConfigChangedMsg struct {
	mode core.ThemeMode
	ok   bool
}

type themeDetectionTickMsg struct{ generation uint64 }

type clipboardCopiedMsg struct {
	text         string
	lineCount    int
	withMetadata bool
	reviewJSON   bool
	commentCount int
}

type clipboardCopyFailedMsg struct {
	err error
}

type activeProviderController interface {
	Catalog(context.Context) ([]ports.ReviewProviderDescriptor, error)
	Start(context.Context, core.ReviewContext) (ActiveProviderState, error)
	Refresh(context.Context, core.ReviewContext, bool) (ActiveProviderState, error)
	Switch(context.Context, core.ReviewContext, string) (ActiveProviderState, error)
	PublishReview(context.Context, core.PublishReviewRequest) (core.PublishReviewResult, error)
	Generation() int64
	CompleteTimer(context.Context, core.ReviewContext, int64) (ActiveProviderState, error)
	Close() error
}

// ActiveProviderState is the TUI-facing active provider snapshot.
type ActiveProviderState struct {
	StableProviderKey string
	RuntimeProviderID string
	RuntimeInfo       core.ReviewProviderInfo
	Snapshot          core.ProviderSnapshot
	FromCache         bool
	Syncing           bool
	LastError         error
}

type activeProviderStartedMsg struct {
	catalog []ports.ReviewProviderDescriptor
	state   ActiveProviderState
	err     error
}

type activeProviderRefreshedMsg struct {
	state ActiveProviderState
	err   error
}

type activeProviderSwitchedMsg struct {
	stableKey string
	state     ActiveProviderState
	err       error
}

type activeProviderPollDueMsg struct{ generation int64 }

type Model struct {
	title                    string
	files                    []core.ReviewFile
	loader                   reviewLoader
	request                  core.ReviewRequest
	loading                  bool
	loadError                string
	selectedFile             int
	selectedContext          int
	width                    int
	height                   int
	reviewViewport           ReviewPane
	reviewAnchors            ReviewAnchors
	activeFilePath           string
	cursorRow                int
	selectionAnchorRow       *int
	reviewRows               []ReviewRow
	reviewExpanderRows       map[presenter.ReviewExpanderAnchor]int
	selectableRows           []int
	clipboardWriter          ports.ClipboardWriter
	lastCopiedText           string
	copyFeedback             string
	copyFeedbackID           int
	diffMode                 core.DiffMode
	nerdFont                 bool
	helpActive               bool
	search                   searchState
	reviewDraft              *core.ReviewDraft
	commentEditor            *InlineCommentEditor
	reviewContext            core.ReviewContext
	reviewProviders          []ports.ReviewProviderClient
	activeProvider           activeProviderController
	providerCatalog          []ports.ReviewProviderDescriptor
	activeProviderKey        string
	activeRuntimeID          string
	activeRuntimeInfo        core.ReviewProviderInfo
	providerSyncState        core.ProviderSyncState
	providerOverview         *core.ProviderOverview
	remoteThreads            []core.RemoteReviewThread
	providerInfos            []core.ReviewProviderInfo
	providerInfoByClient     map[ports.ReviewProviderClient]core.ReviewProviderInfo
	providerPicker           providerPickerState
	publish                  publishState
	prSheet                  prSheetState
	markdownRenderer         *MarkdownRenderer
	ctx                      context.Context
	themeMode                core.ThemeMode
	themeAppearance          core.ThemeAppearance
	systemTheme              core.SystemThemePreference
	themeDetectionGeneration uint64
	themeModeChanges         <-chan core.ThemeMode
	reviewLineCache          *render.ReviewLineCache
	cachedEditorWidth        int
	cachedEditorLines        []string
}

func NewModel(files []core.ReviewFile) Model {
	return NewModelWithTerminal(files, nil)
}

func NewModelWithTerminal(files []core.ReviewFile, terminal ports.Terminal) Model {
	return NewModelWithLoader(files, terminal, nil, core.ReviewRequest{DiffMode: core.DiffModeBranch})
}

func NewModelWithLoader(files []core.ReviewFile, terminal ports.Terminal, loader reviewLoader, request core.ReviewRequest) Model {
	return NewModelWithClipboardWriter(files, terminal, loader, request, nil)
}

func NewModelWithClipboardWriter(files []core.ReviewFile, terminal ports.Terminal, loader reviewLoader, request core.ReviewRequest, clipboardWriter ports.ClipboardWriter) Model {
	return NewModelWithReviewProviders(files, terminal, loader, request, clipboardWriter, core.ReviewContext{}, nil)
}

func NewModelWithReviewProviders(files []core.ReviewFile, terminal ports.Terminal, loader reviewLoader, request core.ReviewRequest, clipboardWriter ports.ClipboardWriter, reviewContext core.ReviewContext, providers []ports.ReviewProviderClient) Model {
	return NewModelWithReviewProvidersContext(context.Background(), files, terminal, loader, request, clipboardWriter, reviewContext, providers)
}

func NewModelWithReviewProvidersContext(ctx context.Context, files []core.ReviewFile, terminal ports.Terminal, loader reviewLoader, request core.ReviewRequest, clipboardWriter ports.ClipboardWriter, reviewContext core.ReviewContext, providers []ports.ReviewProviderClient) Model {
	return NewModelWithActiveProviderContext(ctx, files, terminal, loader, request, clipboardWriter, reviewContext, nil, providers)
}

func NewModelWithActiveProviderContext(ctx context.Context, files []core.ReviewFile, terminal ports.Terminal, loader reviewLoader, request core.ReviewRequest, clipboardWriter ports.ClipboardWriter, reviewContext core.ReviewContext, activeProvider activeProviderController, providers []ports.ReviewProviderClient) Model {
	return NewModelWithActiveProviderContextConfig(ctx, files, terminal, loader, request, clipboardWriter, reviewContext, activeProvider, providers, ModelConfig{})
}

func NewModelWithActiveProviderContextConfig(ctx context.Context, files []core.ReviewFile, terminal ports.Terminal, loader reviewLoader, request core.ReviewRequest, clipboardWriter ports.ClipboardWriter, reviewContext core.ReviewContext, activeProvider activeProviderController, providers []ports.ReviewProviderClient, config ModelConfig) Model {
	if ctx == nil {
		ctx = context.Background()
	}
	if request.DiffMode == "" {
		request.DiffMode = core.DiffModeBranch
	}
	themeMode := core.ThemeModeDark
	if config.ThemeMode != "" {
		themeMode = core.ParseThemeMode(string(config.ThemeMode))
	}
	m := Model{
		title:                "ero",
		files:                sortedReviewFiles(files),
		loader:               loader,
		request:              request,
		selectedContext:      -1,
		width:                defaultWidth,
		height:               defaultHeight,
		reviewViewport:       NewReviewPane(ReviewPaneConfig{Width: defaultWidth, Height: max(defaultHeight-1, 1)}),
		clipboardWriter:      clipboardWriter,
		diffMode:             request.DiffMode,
		nerdFont:             true,
		search:               newSearchState(),
		reviewDraft:          core.NewReviewDraft(),
		reviewContext:        reviewContext,
		reviewProviders:      append([]ports.ReviewProviderClient(nil), providers...),
		activeProvider:       activeProvider,
		providerInfos:        nil,
		providerInfoByClient: map[ports.ReviewProviderClient]core.ReviewProviderInfo{},
		remoteThreads:        nil,
		markdownRenderer:     NewMarkdownRenderer(),
		ctx:                  ctx,
		themeMode:            themeMode,
		themeAppearance:      core.ResolveThemeAppearance(themeMode, core.SystemThemeUnknown, core.ThemeAppearanceDark),
		systemTheme:          core.SystemThemeUnknown,
		themeModeChanges:     config.ThemeModeChanges,
		reviewLineCache:      render.NewReviewLineCache(),
	}
	if terminal != nil {
		m.nerdFont = terminal.SupportsNerdFont()
	}
	m.resetContextSelection()
	m.syncReviewViewport()
	return m
}

func (m Model) ThemeMode() core.ThemeMode {
	return m.themeMode
}

func (m Model) ThemeAppearance() core.ThemeAppearance {
	return m.themeAppearance
}

func (m Model) watchThemeConfigCmd() tea.Cmd {
	if m.themeModeChanges == nil {
		return nil
	}
	return func() tea.Msg {
		mode, ok := <-m.themeModeChanges
		return themeConfigChangedMsg{mode: mode, ok: ok}
	}
}

func (m Model) scheduleThemeDetectionCmd() tea.Cmd {
	generation := m.themeDetectionGeneration
	return tea.Tick(themeDetectionInterval, func(time.Time) tea.Msg {
		return themeDetectionTickMsg{generation: generation}
	})
}

func (m *Model) applyThemeMode(mode core.ThemeMode) tea.Cmd {
	previousMode := m.themeMode
	m.themeMode = mode
	if previousMode == core.ThemeModeAuto || m.themeMode == core.ThemeModeAuto {
		m.themeDetectionGeneration++
	}
	appearance := core.ResolveThemeAppearance(m.themeMode, m.systemTheme, m.themeAppearance)
	if m.applyThemeAppearance(appearance) {
		m.syncReviewViewport()
	}
	cmds := []tea.Cmd{m.watchThemeConfigCmd()}
	if m.themeMode == core.ThemeModeAuto {
		cmds = append(cmds, tea.RequestBackgroundColor)
		if previousMode != core.ThemeModeAuto {
			cmds = append(cmds, m.scheduleThemeDetectionCmd())
		}
	}
	return tea.Batch(cmds...)
}

func (m *Model) applySystemThemePreference(preference core.SystemThemePreference) bool {
	if m.themeMode != core.ThemeModeAuto {
		return false
	}
	m.systemTheme = preference
	appearance := core.ResolveThemeAppearance(m.themeMode, m.systemTheme, m.themeAppearance)
	changed := m.applyThemeAppearance(appearance)
	if changed {
		m.syncReviewViewport()
	}
	return changed
}

func (m *Model) applyThemeAppearance(appearance core.ThemeAppearance) bool {
	if appearance != core.ThemeAppearanceLight {
		appearance = core.ThemeAppearanceDark
	}
	modelChanged := m.themeAppearance != appearance
	themeChanged := theme.ApplyAppearance(appearance)
	m.themeAppearance = appearance
	if !modelChanged && !themeChanged {
		return false
	}
	if m.reviewLineCache != nil {
		m.reviewLineCache.Clear()
	}
	if m.markdownRenderer != nil {
		m.markdownRenderer.Clear()
	}
	return true
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.watchThemeConfigCmd()}
	if m.themeMode == core.ThemeModeAuto {
		cmds = append(cmds, tea.RequestBackgroundColor, m.scheduleThemeDetectionCmd())
	}
	if m.activeProvider != nil {
		cmds = append(cmds, m.startActiveProviderCmd())
	} else if len(m.reviewProviders) > 0 {
		cmds = append(cmds, m.loadReviewProvidersCmd())
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case themeConfigChangedMsg:
		if !msg.ok {
			m.themeModeChanges = nil
			return m, nil
		}
		return m, m.applyThemeMode(msg.mode)
	case tea.BackgroundColorMsg:
		m.applySystemThemePreference(core.SystemThemePreferenceFromDarkBackground(msg.IsDark()))
		return m, nil
	case themeDetectionTickMsg:
		if m.themeMode != core.ThemeModeAuto || msg.generation != m.themeDetectionGeneration {
			return m, nil
		}
		return m, tea.Batch(tea.RequestBackgroundColor, m.scheduleThemeDetectionCmd())
	case reviewLoadedMsg:
		m.loading = false
		m.loadError = ""
		m.diffMode = msg.mode
		m.request.DiffMode = msg.mode
		m.files = sortedReviewFiles(msg.files)
		m.selectedFile = 0
		m.cursorRow = 0
		m.clearSelection()
		m.commentEditor = nil
		m.reviewDraft = core.NewReviewDraft()
		m.providerInfos = nil
		m.remoteThreads = nil
		m.providerInfoByClient = map[ports.ReviewProviderClient]core.ReviewProviderInfo{}
		m.publish = publishState{}
		m.reviewContext = m.reviewContextForLoadedReview(msg.mode)
		m.clearActiveProviderRemoteData()
		m.resetContextSelection()
		m.reviewViewport.GotoTop()
		m.syncReviewViewport()
		cmds := []tea.Cmd{}
		if m.activeProvider != nil {
			if m.activeProviderSyncEnabled() {
				cmds = append(cmds, m.startActiveProviderCmd())
			} else {
				cmds = append(cmds, m.closeReviewProvidersCmd())
			}
		}
		if len(m.reviewProviders) > 0 {
			cmds = append(cmds, m.loadReviewProvidersCmd())
		}
		return m, tea.Batch(cmds...)
	case reviewLoadFailedMsg:
		m.loading = false
		m.loadError = msg.err.Error()
		m.diffMode = msg.previousMode
		m.request.DiffMode = msg.previousMode
		return m, nil
	case copyFeedbackExpiredMsg:
		if msg.id == m.copyFeedbackID {
			m.copyFeedback = ""
		}
		return m, nil
	case clipboardCopiedMsg:
		m.lastCopiedText = msg.text
		if msg.reviewJSON {
			m.setCopyFeedback(fmt.Sprintf("Review JSON copied (%d %s)", msg.commentCount, pluralize("comment", msg.commentCount)))
			return m, m.expireCopyFeedbackCmd()
		}
		feedback := fmt.Sprintf("Copied %d %s", msg.lineCount, pluralize("line", msg.lineCount))
		if msg.withMetadata {
			feedback += " with metadata"
		}
		m.setCopyFeedback(feedback)
		return m, m.expireCopyFeedbackCmd()
	case clipboardCopyFailedMsg:
		m.setCopyFeedback("Copy failed: " + msg.err.Error())
		return m, m.expireCopyFeedbackCmd()
	case prSheetToggledMsg:
		return m.TogglePRSheet(), nil
	case prSheetScrolledMsg:
		return m.ScrollPRSheet(msg.delta), nil
	case activeProviderStartedMsg:
		m.providerCatalog = msg.catalog
		m.applyActiveProviderState(msg.state)
		if msg.err != nil {
			m.setCopyFeedback("Provider unavailable: " + msg.err.Error())
			m.syncReviewViewport()
			return m, m.expireCopyFeedbackCmd()
		}
		m.syncReviewViewport()
		return m, m.refreshActiveProviderCmd(false)
	case activeProviderRefreshedMsg:
		m.applyActiveProviderState(msg.state)
		if msg.err != nil {
			m.setCopyFeedback("Provider refresh failed: " + msg.err.Error())
			m.syncReviewViewport()
			return m, tea.Batch(m.expireCopyFeedbackCmd(), m.scheduleActiveProviderPollCmd())
		}
		m.syncReviewViewport()
		return m, m.scheduleActiveProviderPollCmd()
	case activeProviderSwitchedMsg:
		if msg.err != nil {
			m.clearActiveProviderRemoteData()
			m.activeProviderKey = msg.stableKey
			m.setCopyFeedback("Provider switch failed: " + msg.err.Error())
			m.syncReviewViewport()
			return m, m.expireCopyFeedbackCmd()
		}
		m.applyActiveProviderState(msg.state)
		m.syncReviewViewport()
		return m, m.refreshActiveProviderCmd(false)
	case activeProviderPollDueMsg:
		return m, m.completeActiveProviderTimerCmd(msg.generation)
	case reviewProvidersLoadedMsg:
		m.providerInfos = msg.infos
		m.remoteThreads = msg.threads
		m.providerInfoByClient = msg.clients
		if len(msg.errs) > 0 {
			m.setCopyFeedback(msg.errs[0])
			m.syncReviewViewport()
			return m, m.expireCopyFeedbackCmd()
		}
		m.syncReviewViewport()
		return m, nil
	case publishReviewCompletedMsg:
		return m.handlePublishReviewCompleted(msg)
	case tea.WindowSizeMsg:
		m.width = max(msg.Width, 0)
		m.height = max(msg.Height, 0)
		m.syncReviewViewport()
		return m, nil
	case tea.MouseWheelMsg:
		return m.updateMouseWheel(msg)
	case tea.KeyPressMsg:
		if m.helpActive {
			switch msg.String() {
			case "?", "esc":
				m.helpActive = false
				return m, nil
			case "q", "ctrl+c":
				return m, tea.Batch(m.closeReviewProvidersCmd(), tea.Quit)
			default:
				return m, nil
			}
		}
		if m.commentEditor != nil {
			return m.updateCommentEditor(msg)
		}
		if m.search.active() {
			return m.updateSearch(msg)
		}
		if m.providerPicker.open {
			return m.updateProviderPicker(msg)
		}
		if m.publish.active {
			return m.updatePublishReview(msg)
		}
		if m.prSheet.open {
			return m.updatePRSheetAction(msg)
		}
		return m.updateReviewAction(keymap.ReviewAction(msg.Keystroke()))
	default:
		return m, nil
	}
}

func (m Model) updateMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.helpActive || m.commentEditor != nil || m.search.active() || m.providerPicker.open || m.publish.active {
		return m, nil
	}
	switch msg.Mouse().Button {
	case tea.MouseWheelUp:
		if m.prSheet.open {
			return m.ScrollPRSheet(-1), nil
		}
		m.moveCursor(-1)
	case tea.MouseWheelDown:
		if m.prSheet.open {
			return m.ScrollPRSheet(1), nil
		}
		m.moveCursor(1)
	default:
		return m, nil
	}
	return m, nil
}

func (m Model) updatePRSheetAction(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch keymap.ReviewAction(msg.Keystroke()) {
	case keymap.ActionMoveUp:
		return m.ScrollPRSheet(-1), nil
	case keymap.ActionMoveDown:
		return m.ScrollPRSheet(1), nil
	case keymap.ActionPageUp:
		return m.ScrollPRSheet(-max(m.height-1, 1)), nil
	case keymap.ActionPageDown:
		return m.ScrollPRSheet(max(m.height-1, 1)), nil
	case keymap.ActionPreviousFile:
		m.moveChunk(-1)
		return m, nil
	case keymap.ActionNextFile:
		m.moveChunk(1)
		return m, nil
	case keymap.ActionTogglePRSheet:
		return m.TogglePRSheet(), nil
	case keymap.ActionOpenHelp:
		m.helpActive = true
		return m, nil
	case keymap.ActionQuit:
		return m, tea.Batch(m.closeReviewProvidersCmd(), tea.Quit)
	default:
		return m, nil
	}
}

func (m Model) updateReviewAction(action keymap.Action) (tea.Model, tea.Cmd) {
	switch action {
	case keymap.ActionQuit:
		return m, tea.Batch(m.closeReviewProvidersCmd(), tea.Quit)
	case keymap.ActionMoveUp:
		m.moveCursor(-1)
	case keymap.ActionMoveDown:
		m.moveCursor(1)
	case keymap.ActionPageUp:
		m.pageCursor(-1)
	case keymap.ActionPageDown:
		m.pageCursor(1)
	case keymap.ActionMoveStart:
		m.moveCursorToStart()
	case keymap.ActionMoveEnd:
		m.moveCursorToEnd()
	case keymap.ActionToggleSelection:
		m.toggleSelection()
	case keymap.ActionClearSelection:
		m.clearSelection()
	case keymap.ActionOpenComment:
		return m.openCommentEditor()
	case keymap.ActionClearReview:
		m.clearReviewDraft()
	case keymap.ActionPublishReview:
		return m.openPublishReview()
	case keymap.ActionCopyReviewJSON:
		return m.copyReviewJSONToClipboard()
	case keymap.ActionCopyPlain:
		return m.copyToClipboard(false)
	case keymap.ActionCopyWithMetadata:
		return m.copyToClipboard(true)
	case keymap.ActionOpenFileSearch:
		return m.openSearch(searchModeFiles)
	case keymap.ActionOpenGrepSearch:
		return m.openSearch(searchModeGrep)
	case keymap.ActionOpenDiffMode:
		return m.openSearch(searchModeDiff)
	case keymap.ActionPreviousFile:
		m.moveChunk(-1)
	case keymap.ActionNextFile:
		m.moveChunk(1)
	case keymap.ActionExpandAllContext:
		m.showAllContext()
	case keymap.ActionExpandMoreContext:
		m.showMoreContext(contextStep)
	case keymap.ActionCycleProvider:
		if !m.canSwitchProvider() {
			return m, nil
		}
		return m, m.cycleProviderCmd()
	case keymap.ActionOpenProviderPicker:
		if !m.canSwitchProvider() {
			return m, nil
		}
		m = m.openProviderPicker()
	case keymap.ActionRefreshProvider:
		return m, m.refreshActiveProviderCmd(true)
	case keymap.ActionTogglePRSheet:
		m = m.TogglePRSheet()
	case keymap.ActionOpenHelp:
		m.helpActive = true
	case keymap.ActionNone:
		return m, nil
	}
	m.syncReviewVisualState()
	return m, nil
}

func (m Model) unpublishedDraftCommentCount() int {
	if m.reviewDraft == nil {
		return 0
	}
	count := 0
	for _, comment := range m.reviewDraft.Comments() {
		if comment.State != core.ReviewCommentStatePublished {
			count++
		}
	}
	return count
}

func (m Model) View() tea.View {
	m.applyThemeAppearance(m.themeAppearance)
	review := m.reviewViewport.View(m.reviewVisualState())
	if m.loading {
		review = theme.MutedStyle.Render("Loading diff…") + "\n" + review
	} else if m.loadError != "" {
		review = theme.MutedStyle.Render("Failed to load diff: "+m.loadError) + "\n" + review
	}
	content := lipgloss.JoinVertical(lipgloss.Left,
		review,
		NewStatusBar(m.width).Render(StatusModel{
			AppName:             m.title,
			Mode:                diffModeLabel(m.diffMode, m.nerdFont),
			FileCount:           len(m.files),
			ProviderCount:       m.statusProviderCount(),
			ProviderSwitch:      m.canSwitchProvider(),
			CurrentFile:         m.activeLocation(),
			Message:             m.copyFeedback,
			ScrollPercent:       m.reviewViewport.ScrollPercent(),
			ActiveProviderLabel: m.activeRuntimeInfo.Label,
			ActiveRuntimeName:   m.activeRuntimeID,
			ProviderSync:        m.providerSyncState,
			DraftCommentCount:   m.unpublishedDraftCommentCount(),
			ShowNoProvider:      m.activeProvider != nil && m.activeProviderKey == "" && m.activeRuntimeInfo.ID == "",
			NerdFont:            m.nerdFont,
		}),
	)
	if m.search.active() {
		content = m.renderSearchOverlay(content)
	}
	if m.publish.active {
		content = m.renderPublishOverlay(content)
	}
	if m.providerPicker.open {
		content = m.renderProviderPickerOverlay(content)
	}
	if m.prSheet.open {
		content = m.renderPRSheetOverlay(content)
	}
	if m.helpActive {
		content = m.renderHelpOverlay(content)
	}
	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	if m.commentEditor != nil {
		view.KeyboardEnhancements.ReportAllKeysAsEscapeCodes = true
		view.KeyboardEnhancements.ReportAssociatedText = true
	}
	return view
}
