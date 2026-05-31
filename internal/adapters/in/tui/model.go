package tui

import (
	"context"
	"fmt"

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
	defaultWidth  = 100
	defaultHeight = 24
	contextStep   = 10
)

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

type reviewProvidersLoadedMsg struct {
	infos   []core.ReviewProviderInfo
	threads []core.RemoteReviewThread
	clients map[ports.ReviewProviderClient]core.ReviewProviderInfo
	errs    []string
}

type Model struct {
	title                string
	files                []core.ReviewFile
	loader               reviewLoader
	request              core.ReviewRequest
	loading              bool
	loadError            string
	selectedFile         int
	selectedContext      int
	width                int
	height               int
	reviewViewport       ReviewPane
	reviewAnchors        ReviewAnchors
	activeFilePath       string
	cursorRow            int
	selectionAnchorRow   *int
	reviewRows           []ReviewRow
	reviewExpanderRows   map[presenter.ReviewExpanderAnchor]int
	selectableRows       []int
	clipboardWriter      ports.ClipboardWriter
	lastCopiedText       string
	copyFeedback         string
	copyFeedbackID       int
	diffMode             core.DiffMode
	nerdFont             bool
	helpActive           bool
	search               searchState
	reviewDraft          *core.ReviewDraft
	commentEditor        *InlineCommentEditor
	reviewContext        core.ReviewContext
	reviewProviders      []ports.ReviewProviderClient
	remoteThreads        []core.RemoteReviewThread
	providerInfos        []core.ReviewProviderInfo
	providerInfoByClient map[ports.ReviewProviderClient]core.ReviewProviderInfo
	publish              publishState
	ctx                  context.Context
	reviewLineCache      *render.ReviewLineCache
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
	if ctx == nil {
		ctx = context.Background()
	}
	if request.DiffMode == "" {
		request.DiffMode = core.DiffModeBranch
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
		providerInfos:        nil,
		providerInfoByClient: map[ports.ReviewProviderClient]core.ReviewProviderInfo{},
		remoteThreads:        nil,
		ctx:                  ctx,
		reviewLineCache:      render.NewReviewLineCache(),
	}
	if terminal != nil {
		m.nerdFont = terminal.SupportsNerdFont()
	}
	m.resetContextSelection()
	m.syncReviewViewport()
	return m
}

func (m Model) Init() tea.Cmd {
	if len(m.reviewProviders) == 0 {
		return nil
	}
	return m.loadReviewProvidersCmd()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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
		m.resetContextSelection()
		m.reviewViewport.GotoTop()
		m.syncReviewViewport()
		if len(m.reviewProviders) > 0 {
			return m, m.loadReviewProvidersCmd()
		}
		return m, nil
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
		if m.publish.active {
			return m.updatePublishReview(msg)
		}
		return m.updateReviewAction(keymap.ReviewAction(msg.String()), msg)
	default:
		return m, nil
	}
}

func (m Model) updateReviewAction(action keymap.Action, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
		m.moveFile(-1)
	case keymap.ActionNextFile:
		m.moveFile(1)
	case keymap.ActionExpandAllContext:
		m.showAllContext()
	case keymap.ActionExpandMoreContext:
		m.showMoreContext(contextStep)
	case keymap.ActionOpenHelp:
		m.helpActive = true
	case keymap.ActionNone:
		return m, nil
	}
	m.syncReviewVisualState()
	return m, nil
}

func (m Model) View() tea.View {
	review := m.reviewViewport.View(m.reviewVisualState())
	if m.loading {
		review = theme.MutedStyle.Render("Loading diff…") + "\n" + review
	} else if m.loadError != "" {
		review = theme.MutedStyle.Render("Failed to load diff: "+m.loadError) + "\n" + review
	}
	content := lipgloss.JoinVertical(lipgloss.Left,
		review,
		NewStatusBar(m.width).Render(StatusModel{
			AppName:       m.title,
			Mode:          diffModeLabel(m.diffMode, m.nerdFont),
			FileCount:     len(m.files),
			ProviderCount: len(m.providerInfos),
			CurrentFile:   m.activeLocation(),
			Message:       m.copyFeedback,
			ScrollPercent: m.reviewViewport.ScrollPercent(),
		}),
	)
	if m.search.active() {
		content = m.renderSearchOverlay(content)
	}
	if m.publish.active {
		content = m.renderPublishOverlay(content)
	}
	if m.helpActive {
		content = m.renderHelpOverlay(content)
	}
	view := tea.NewView(content)
	view.AltScreen = true
	if m.commentEditor != nil {
		view.KeyboardEnhancements.ReportAllKeysAsEscapeCodes = true
		view.KeyboardEnhancements.ReportAssociatedText = true
	}
	return view
}
