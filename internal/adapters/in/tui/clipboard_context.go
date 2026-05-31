package tui

import (
	"context"
	"time"
)

const clipboardWriteTimeout = 5 * time.Second

func (m Model) clipboardContext() (context.Context, context.CancelFunc) {
	base := m.ctx
	if base == nil {
		base = context.Background()
	}
	return context.WithTimeout(base, clipboardWriteTimeout)
}
