package theme

import (
	"sync"

	"charm.land/lipgloss/v2"

	"ero/internal/core"
)

// Palette contains every color Ero uses to render the TUI. The dark palette is
// the original hard-coded Ero palette kept byte-for-byte where existing styles
// depended on exact colors.
type Palette struct {
	Appearance core.ThemeAppearance

	ColorBackground string
	ColorText       string
	ColorMutedText  string
	ColorAccent     string
	ColorWarning    string
	ColorKeyword    string
	ColorFunction   string
	ColorType       string
	ColorString     string
	ColorNumber     string
	ColorStatusBase string
	ColorStatusInfo string
	ColorCodeBg     string

	FileHeaderFg      string
	FileRuleFg        string
	AddedLineBg       string
	DeletedLineBg     string
	AddedMarkerFg     string
	DeletedMarkerFg   string
	CursorRowBg       string
	SelectedRowBg     string
	CommentRangeRowBg string
	StatusBaseFg      string
	StatusAppBg       string
	StatusAppFg       string
	StatusModeFg      string
	StatusKeyFg       string
	StatusHintTextFg  string
	SearchBorderFg    string
	SearchTitleFg     string
	SearchSelectedFg  string
	SearchSelectedBg  string
	HelpBorderFg      string
	HelpTitleFg       string
	HelpSectionFg     string
	HelpKeyFg         string
	HelpLabelFg       string
	ChromaStyle       string
	MarkdownCodeTheme string
}

type Styles struct {
	Palette Palette

	FileHeaderStyle      lipgloss.Style
	FileRuleStyle        lipgloss.Style
	PanelTitleStyle      lipgloss.Style
	MutedStyle           lipgloss.Style
	AddedLineStyle       lipgloss.Style
	DeletedLineStyle     lipgloss.Style
	AddedMarkerStyle     lipgloss.Style
	DeletedMarkerStyle   lipgloss.Style
	LineNumberStyle      lipgloss.Style
	SelectedExpander     lipgloss.Style
	CursorRowStyle       lipgloss.Style
	SelectedRowStyle     lipgloss.Style
	CommentRangeRowStyle lipgloss.Style
	KeywordStyle         lipgloss.Style
	FunctionStyle        lipgloss.Style
	TypeStyle            lipgloss.Style
	NameStyle            lipgloss.Style
	StringStyle          lipgloss.Style
	NumberStyle          lipgloss.Style
	CommentStyle         lipgloss.Style
	OperatorStyle        lipgloss.Style
	PunctuationStyle     lipgloss.Style
	StatusBaseStyle      lipgloss.Style
	StatusAppStyle       lipgloss.Style
	StatusModeStyle      lipgloss.Style
	StatusInfoStyle      lipgloss.Style
	StatusKeyStyle       lipgloss.Style
	StatusHintTextStyle  lipgloss.Style

	SearchPaneStyle        lipgloss.Style
	SearchPaneTitleStyle   lipgloss.Style
	SearchSelectedRowStyle lipgloss.Style

	HelpPaneStyle      lipgloss.Style
	HelpPaneTitleStyle lipgloss.Style
	HelpSectionStyle   lipgloss.Style
	HelpKeyStyle       lipgloss.Style
	HelpLabelStyle     lipgloss.Style
}

var darkPalette = Palette{
	Appearance: core.ThemeAppearanceDark,

	ColorBackground: "#000000",
	ColorText:       "#c9d1d9",
	ColorMutedText:  "#8b949e",
	ColorAccent:     "#58a6ff",
	ColorWarning:    "#ffa657",
	ColorKeyword:    "#ff7b72",
	ColorFunction:   "#d2a8ff",
	ColorType:       "#ffa657",
	ColorString:     "#a5d6ff",
	ColorNumber:     "#79c0ff",
	ColorStatusBase: "236",
	ColorStatusInfo: "248",
	ColorCodeBg:     "#1f2a44",

	FileHeaderFg:      "15",
	FileRuleFg:        "8",
	AddedLineBg:       "#011209",
	DeletedLineBg:     "#1f0101",
	AddedMarkerFg:     "#3fb950",
	DeletedMarkerFg:   "#ff7b72",
	CursorRowBg:       "#1f2a44",
	SelectedRowBg:     "#25351f",
	CommentRangeRowBg: "#201a35",
	StatusBaseFg:      "252",
	StatusAppBg:       "62",
	StatusAppFg:       "230",
	StatusModeFg:      "229",
	StatusKeyFg:       "81",
	StatusHintTextFg:  "244",
	SearchBorderFg:    "62",
	SearchTitleFg:     "81",
	SearchSelectedFg:  "230",
	SearchSelectedBg:  "62",
	HelpBorderFg:      "62",
	HelpTitleFg:       "81",
	HelpSectionFg:     "229",
	HelpKeyFg:         "81",
	HelpLabelFg:       "252",
	ChromaStyle:       "github-dark",
	MarkdownCodeTheme: "github-dark",
}

var lightPalette = Palette{
	Appearance: core.ThemeAppearanceLight,

	ColorBackground: "#ffffff",
	ColorText:       "#24292f",
	ColorMutedText:  "#57606a",
	ColorAccent:     "#0969da",
	ColorWarning:    "#9a6700",
	ColorKeyword:    "#cf222e",
	ColorFunction:   "#8250df",
	ColorType:       "#953800",
	ColorString:     "#0a3069",
	ColorNumber:     "#0550ae",
	ColorStatusBase: "252",
	ColorStatusInfo: "239",
	ColorCodeBg:     "#f6f8fa",

	FileHeaderFg:      "#24292f",
	FileRuleFg:        "#d0d7de",
	AddedLineBg:       "#dafbe1",
	DeletedLineBg:     "#ffebe9",
	AddedMarkerFg:     "#1a7f37",
	DeletedMarkerFg:   "#cf222e",
	CursorRowBg:       "#eaeef2",
	SelectedRowBg:     "#ddf4ff",
	CommentRangeRowBg: "#fff8c5",
	StatusBaseFg:      "#24292f",
	StatusAppBg:       "#0969da",
	StatusAppFg:       "#ffffff",
	StatusModeFg:      "#24292f",
	StatusKeyFg:       "#0969da",
	StatusHintTextFg:  "#57606a",
	SearchBorderFg:    "#0969da",
	SearchTitleFg:     "#0969da",
	SearchSelectedFg:  "#ffffff",
	SearchSelectedBg:  "#0969da",
	HelpBorderFg:      "#0969da",
	HelpTitleFg:       "#0969da",
	HelpSectionFg:     "#9a6700",
	HelpKeyFg:         "#0969da",
	HelpLabelFg:       "#24292f",
	ChromaStyle:       "github",
	MarkdownCodeTheme: "github",
}

var (
	mu                sync.RWMutex
	currentAppearance = core.ThemeAppearanceDark
	currentPalette    = darkPalette
)

var (
	ColorText       = darkPalette.ColorText
	ColorMutedText  = darkPalette.ColorMutedText
	ColorAccent     = darkPalette.ColorAccent
	ColorWarning    = darkPalette.ColorWarning
	ColorKeyword    = darkPalette.ColorKeyword
	ColorFunction   = darkPalette.ColorFunction
	ColorType       = darkPalette.ColorType
	ColorString     = darkPalette.ColorString
	ColorNumber     = darkPalette.ColorNumber
	ColorStatusBase = darkPalette.ColorStatusBase
	ColorStatusInfo = darkPalette.ColorStatusInfo
	ColorCodeBg     = darkPalette.ColorCodeBg
)

var (
	FileHeaderStyle      lipgloss.Style
	FileRuleStyle        lipgloss.Style
	PanelTitleStyle      lipgloss.Style
	MutedStyle           lipgloss.Style
	AddedLineStyle       lipgloss.Style
	DeletedLineStyle     lipgloss.Style
	AddedMarkerStyle     lipgloss.Style
	DeletedMarkerStyle   lipgloss.Style
	LineNumberStyle      lipgloss.Style
	SelectedExpander     lipgloss.Style
	CursorRowStyle       lipgloss.Style
	SelectedRowStyle     lipgloss.Style
	CommentRangeRowStyle lipgloss.Style
	KeywordStyle         lipgloss.Style
	FunctionStyle        lipgloss.Style
	TypeStyle            lipgloss.Style
	NameStyle            lipgloss.Style
	StringStyle          lipgloss.Style
	NumberStyle          lipgloss.Style
	CommentStyle         lipgloss.Style
	OperatorStyle        lipgloss.Style
	PunctuationStyle     lipgloss.Style
	StatusBaseStyle      lipgloss.Style
	StatusAppStyle       lipgloss.Style
	StatusModeStyle      lipgloss.Style
	StatusInfoStyle      lipgloss.Style
	StatusKeyStyle       lipgloss.Style
	StatusHintTextStyle  lipgloss.Style

	SearchPaneStyle        lipgloss.Style
	SearchPaneTitleStyle   lipgloss.Style
	SearchSelectedRowStyle lipgloss.Style

	HelpPaneStyle      lipgloss.Style
	HelpPaneTitleStyle lipgloss.Style
	HelpSectionStyle   lipgloss.Style
	HelpKeyStyle       lipgloss.Style
	HelpLabelStyle     lipgloss.Style
)

func init() {
	applyPalette(darkPalette)
}

func CurrentAppearance() core.ThemeAppearance {
	mu.RLock()
	defer mu.RUnlock()
	return currentAppearance
}

func CurrentPalette() Palette {
	mu.RLock()
	defer mu.RUnlock()
	return currentPalette
}

// ApplyAppearance updates the package-level style variables for the selected
// appearance. Call it from the main Bubble Tea event loop before rendering so
// style reads remain serialized with theme switches.
func ApplyAppearance(appearance core.ThemeAppearance) bool {
	mu.Lock()
	defer mu.Unlock()
	palette := paletteForAppearance(appearance)
	if currentAppearance == palette.Appearance {
		return false
	}
	applyPalette(palette)
	return true
}

func PaletteForAppearance(appearance core.ThemeAppearance) Palette {
	if appearance == core.ThemeAppearanceLight {
		return lightPalette
	}
	return darkPalette
}

func StylesForAppearance(appearance core.ThemeAppearance) Styles {
	return stylesForPalette(PaletteForAppearance(appearance))
}

func paletteForAppearance(appearance core.ThemeAppearance) Palette {
	return PaletteForAppearance(appearance)
}

func applyPalette(p Palette) {
	currentAppearance = p.Appearance
	currentPalette = p

	ColorText = p.ColorText
	ColorMutedText = p.ColorMutedText
	ColorAccent = p.ColorAccent
	ColorWarning = p.ColorWarning
	ColorKeyword = p.ColorKeyword
	ColorFunction = p.ColorFunction
	ColorType = p.ColorType
	ColorString = p.ColorString
	ColorNumber = p.ColorNumber
	ColorStatusBase = p.ColorStatusBase
	ColorStatusInfo = p.ColorStatusInfo
	ColorCodeBg = p.ColorCodeBg

	styles := stylesForPalette(p)
	FileHeaderStyle = styles.FileHeaderStyle
	FileRuleStyle = styles.FileRuleStyle
	PanelTitleStyle = styles.PanelTitleStyle
	MutedStyle = styles.MutedStyle
	AddedLineStyle = styles.AddedLineStyle
	DeletedLineStyle = styles.DeletedLineStyle
	AddedMarkerStyle = styles.AddedMarkerStyle
	DeletedMarkerStyle = styles.DeletedMarkerStyle
	LineNumberStyle = styles.LineNumberStyle
	SelectedExpander = styles.SelectedExpander
	CursorRowStyle = styles.CursorRowStyle
	SelectedRowStyle = styles.SelectedRowStyle
	CommentRangeRowStyle = styles.CommentRangeRowStyle
	KeywordStyle = styles.KeywordStyle
	FunctionStyle = styles.FunctionStyle
	TypeStyle = styles.TypeStyle
	NameStyle = styles.NameStyle
	StringStyle = styles.StringStyle
	NumberStyle = styles.NumberStyle
	CommentStyle = styles.CommentStyle
	OperatorStyle = styles.OperatorStyle
	PunctuationStyle = styles.PunctuationStyle
	StatusBaseStyle = styles.StatusBaseStyle
	StatusAppStyle = styles.StatusAppStyle
	StatusModeStyle = styles.StatusModeStyle
	StatusInfoStyle = styles.StatusInfoStyle
	StatusKeyStyle = styles.StatusKeyStyle
	StatusHintTextStyle = styles.StatusHintTextStyle
	SearchPaneStyle = styles.SearchPaneStyle
	SearchPaneTitleStyle = styles.SearchPaneTitleStyle
	SearchSelectedRowStyle = styles.SearchSelectedRowStyle
	HelpPaneStyle = styles.HelpPaneStyle
	HelpPaneTitleStyle = styles.HelpPaneTitleStyle
	HelpSectionStyle = styles.HelpSectionStyle
	HelpKeyStyle = styles.HelpKeyStyle
	HelpLabelStyle = styles.HelpLabelStyle
}

func stylesForPalette(p Palette) Styles {
	statusBaseStyle := lipgloss.NewStyle().Background(lipgloss.Color(p.ColorStatusBase)).Foreground(lipgloss.Color(p.StatusBaseFg))
	return Styles{
		Palette:                p,
		FileHeaderStyle:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(p.FileHeaderFg)),
		FileRuleStyle:          lipgloss.NewStyle().Foreground(lipgloss.Color(p.FileRuleFg)),
		PanelTitleStyle:        lipgloss.NewStyle().Bold(true).Underline(true),
		MutedStyle:             lipgloss.NewStyle().Foreground(lipgloss.Color(p.FileRuleFg)),
		AddedLineStyle:         lipgloss.NewStyle().Background(lipgloss.Color(p.AddedLineBg)).Foreground(lipgloss.Color(p.ColorText)),
		DeletedLineStyle:       lipgloss.NewStyle().Background(lipgloss.Color(p.DeletedLineBg)).Foreground(lipgloss.Color(p.ColorText)),
		AddedMarkerStyle:       lipgloss.NewStyle().Foreground(lipgloss.Color(p.AddedMarkerFg)).Bold(true),
		DeletedMarkerStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color(p.DeletedMarkerFg)).Bold(true),
		LineNumberStyle:        lipgloss.NewStyle().Foreground(lipgloss.Color(p.ColorMutedText)),
		SelectedExpander:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(p.ColorAccent)),
		CursorRowStyle:         lipgloss.NewStyle().Background(lipgloss.Color(p.CursorRowBg)),
		SelectedRowStyle:       lipgloss.NewStyle().Background(lipgloss.Color(p.SelectedRowBg)),
		CommentRangeRowStyle:   lipgloss.NewStyle().Background(lipgloss.Color(p.CommentRangeRowBg)),
		KeywordStyle:           lipgloss.NewStyle().Foreground(lipgloss.Color(p.ColorKeyword)),
		FunctionStyle:          lipgloss.NewStyle().Foreground(lipgloss.Color(p.ColorFunction)),
		TypeStyle:              lipgloss.NewStyle().Foreground(lipgloss.Color(p.ColorType)),
		NameStyle:              lipgloss.NewStyle().Foreground(lipgloss.Color(p.ColorText)),
		StringStyle:            lipgloss.NewStyle().Foreground(lipgloss.Color(p.ColorString)),
		NumberStyle:            lipgloss.NewStyle().Foreground(lipgloss.Color(p.ColorNumber)),
		CommentStyle:           lipgloss.NewStyle().Foreground(lipgloss.Color(p.ColorMutedText)).Italic(true),
		OperatorStyle:          lipgloss.NewStyle().Foreground(lipgloss.Color(p.ColorKeyword)),
		PunctuationStyle:       lipgloss.NewStyle().Foreground(lipgloss.Color(p.ColorText)),
		StatusBaseStyle:        statusBaseStyle,
		StatusAppStyle:         statusBaseStyle.Bold(true).Background(lipgloss.Color(p.StatusAppBg)).Foreground(lipgloss.Color(p.StatusAppFg)).Padding(0, 1),
		StatusModeStyle:        statusBaseStyle.Foreground(lipgloss.Color(p.StatusModeFg)).Padding(0, 1),
		StatusInfoStyle:        statusBaseStyle.Foreground(lipgloss.Color(p.ColorStatusInfo)).Padding(0, 1),
		StatusKeyStyle:         statusBaseStyle.Foreground(lipgloss.Color(p.StatusKeyFg)).Bold(true),
		StatusHintTextStyle:    statusBaseStyle.Foreground(lipgloss.Color(p.StatusHintTextFg)),
		SearchPaneStyle:        lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(p.SearchBorderFg)).Padding(0, 1),
		SearchPaneTitleStyle:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(p.SearchTitleFg)),
		SearchSelectedRowStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(p.SearchSelectedFg)).Background(lipgloss.Color(p.SearchSelectedBg)),
		HelpPaneStyle:          lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(p.HelpBorderFg)).Padding(0, 2),
		HelpPaneTitleStyle:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(p.HelpTitleFg)),
		HelpSectionStyle:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(p.HelpSectionFg)),
		HelpKeyStyle:           lipgloss.NewStyle().Foreground(lipgloss.Color(p.HelpKeyFg)).Bold(true),
		HelpLabelStyle:         lipgloss.NewStyle().Foreground(lipgloss.Color(p.HelpLabelFg)),
	}
}
