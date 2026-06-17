package core

import "testing"

func TestParseThemeMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want ThemeMode
	}{
		{name: "empty defaults auto", raw: "", want: ThemeModeAuto},
		{name: "auto", raw: "auto", want: ThemeModeAuto},
		{name: "light", raw: "light", want: ThemeModeLight},
		{name: "dark", raw: "dark", want: ThemeModeDark},
		{name: "trims and lowercases", raw: " Light ", want: ThemeModeLight},
		{name: "unknown defaults auto", raw: "system", want: ThemeModeAuto},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ParseThemeMode(tt.raw); got != tt.want {
				t.Fatalf("ParseThemeMode(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestResolveThemeAppearance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mode       ThemeMode
		preference SystemThemePreference
		fallback   ThemeAppearance
		want       ThemeAppearance
	}{
		{name: "force light ignores dark preference", mode: ThemeModeLight, preference: SystemThemePreferDark, fallback: ThemeAppearanceDark, want: ThemeAppearanceLight},
		{name: "force dark ignores light preference", mode: ThemeModeDark, preference: SystemThemePreferLight, fallback: ThemeAppearanceLight, want: ThemeAppearanceDark},
		{name: "auto follows dark preference", mode: ThemeModeAuto, preference: SystemThemePreferDark, fallback: ThemeAppearanceLight, want: ThemeAppearanceDark},
		{name: "auto follows light preference", mode: ThemeModeAuto, preference: SystemThemePreferLight, fallback: ThemeAppearanceDark, want: ThemeAppearanceLight},
		{name: "auto keeps light fallback without preference", mode: ThemeModeAuto, preference: SystemThemeUnknown, fallback: ThemeAppearanceLight, want: ThemeAppearanceLight},
		{name: "auto defaults dark without preference", mode: ThemeModeAuto, preference: SystemThemeUnknown, fallback: "", want: ThemeAppearanceDark},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := ResolveThemeAppearance(tt.mode, tt.preference, tt.fallback); got != tt.want {
				t.Fatalf("ResolveThemeAppearance(%q, %v, %q) = %q, want %q", tt.mode, tt.preference, tt.fallback, got, tt.want)
			}
		})
	}
}
