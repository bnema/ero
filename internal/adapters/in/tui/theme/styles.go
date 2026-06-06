package theme

import "charm.land/lipgloss/v2"

const (
	ColorText       = "#c9d1d9"
	ColorMutedText  = "#8b949e"
	ColorAccent     = "#58a6ff"
	ColorWarning    = "#ffa657"
	ColorKeyword    = "#ff7b72"
	ColorFunction   = "#d2a8ff"
	ColorString     = "#a5d6ff"
	ColorNumber     = "#79c0ff"
	ColorStatusBase = "236"
	ColorStatusInfo = "248"
	ColorCodeBg     = "#1f2a44"
)

var (
	FileHeaderStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	FileRuleStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	PanelTitleStyle      = lipgloss.NewStyle().Bold(true).Underline(true)
	MutedStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	AddedLineStyle       = lipgloss.NewStyle().Background(lipgloss.Color("#011209")).Foreground(lipgloss.Color("#c9d1d9"))
	DeletedLineStyle     = lipgloss.NewStyle().Background(lipgloss.Color("#1f0101")).Foreground(lipgloss.Color("#c9d1d9"))
	AddedMarkerStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#3fb950")).Bold(true)
	DeletedMarkerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff7b72")).Bold(true)
	LineNumberStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#8b949e"))
	SelectedExpander     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#58a6ff"))
	CursorRowStyle       = lipgloss.NewStyle().Background(lipgloss.Color("#1f2a44"))
	SelectedRowStyle     = lipgloss.NewStyle().Background(lipgloss.Color("#25351f"))
	CommentRangeRowStyle = lipgloss.NewStyle().Background(lipgloss.Color("#201a35"))
	KeywordStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff7b72"))
	FunctionStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#d2a8ff"))
	TypeStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffa657"))
	NameStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("#c9d1d9"))
	StringStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("#a5d6ff"))
	NumberStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("#79c0ff"))
	CommentStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#8b949e")).Italic(true)
	OperatorStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff7b72"))
	PunctuationStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#c9d1d9"))
	StatusBaseStyle      = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("252"))
	StatusAppStyle       = StatusBaseStyle.Bold(true).Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230")).Padding(0, 1)
	StatusModeStyle      = StatusBaseStyle.Foreground(lipgloss.Color("229")).Padding(0, 1)
	StatusInfoStyle      = StatusBaseStyle.Foreground(lipgloss.Color("248")).Padding(0, 1)
	StatusKeyStyle       = StatusBaseStyle.Foreground(lipgloss.Color("81")).Bold(true)
	StatusHintTextStyle  = StatusBaseStyle.Foreground(lipgloss.Color("244"))

	SearchPaneStyle        = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).Padding(0, 1)
	SearchPaneTitleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	SearchSelectedRowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62"))

	HelpPaneStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).Padding(0, 2)
	HelpPaneTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	HelpSectionStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("229"))
	HelpKeyStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
	HelpLabelStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
)
