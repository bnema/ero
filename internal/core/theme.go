package core

import "strings"

// ThemeMode describes how Ero chooses its active color palette.
type ThemeMode string

const (
	ThemeModeAuto  ThemeMode = "auto"
	ThemeModeLight ThemeMode = "light"
	ThemeModeDark  ThemeMode = "dark"
)

// SystemThemePreference is the light/dark preference detected from the terminal
// or host system.
type SystemThemePreference int

const (
	SystemThemeUnknown SystemThemePreference = iota
	SystemThemePreferDark
	SystemThemePreferLight
)

// ThemeAppearance is the concrete palette currently used for rendering.
type ThemeAppearance string

const (
	ThemeAppearanceDark  ThemeAppearance = "dark"
	ThemeAppearanceLight ThemeAppearance = "light"
)

// ParseThemeMode normalizes a theme mode string to a valid ThemeMode.
// It trims whitespace, lowercases input, and returns ThemeModeAuto for empty or
// unrecognized values.
func ParseThemeMode(raw string) ThemeMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(ThemeModeLight):
		return ThemeModeLight
	case string(ThemeModeDark):
		return ThemeModeDark
	case "", string(ThemeModeAuto):
		return ThemeModeAuto
	default:
		return ThemeModeAuto
	}
}

// SystemThemePreferenceFromDarkBackground converts terminal background color
// detection into the corresponding SystemThemePreference.
func SystemThemePreferenceFromDarkBackground(isDark bool) SystemThemePreference {
	if isDark {
		return SystemThemePreferDark
	}
	return SystemThemePreferLight
}

// ParseSystemThemePreference normalizes a raw XDG portal color-scheme value.
// The portal uses 0 for no preference, 1 for prefer-dark, and 2 for
// prefer-light.
func ParseSystemThemePreference(raw uint32) SystemThemePreference {
	switch raw {
	case 1:
		return SystemThemePreferDark
	case 2:
		return SystemThemePreferLight
	case 0:
		return SystemThemeUnknown
	default:
		return SystemThemeUnknown
	}
}

// ResolveThemeAppearance chooses the concrete light or dark appearance. Explicit
// light/dark modes win, auto follows the detected system preference, then the
// fallback appearance, and finally dark when nothing else is available.
func ResolveThemeAppearance(mode ThemeMode, preference SystemThemePreference, fallback ThemeAppearance) ThemeAppearance {
	switch mode {
	case ThemeModeLight:
		return ThemeAppearanceLight
	case ThemeModeDark:
		return ThemeAppearanceDark
	default:
		switch preference {
		case SystemThemePreferLight:
			return ThemeAppearanceLight
		case SystemThemePreferDark:
			return ThemeAppearanceDark
		default:
			if fallback == ThemeAppearanceLight {
				return ThemeAppearanceLight
			}
			return ThemeAppearanceDark
		}
	}
}
