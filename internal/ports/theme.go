package ports

import (
	"context"

	"ero/internal/core"
)

// SystemThemeReader reads the host system light/dark preference.
type SystemThemeReader interface {
	CurrentPreference(context.Context) (core.SystemThemePreference, error)
}
