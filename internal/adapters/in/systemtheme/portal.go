package systemtheme

import (
	"context"
	"errors"
	"fmt"

	"github.com/godbus/dbus/v5"

	"ero/internal/core"
)

const (
	portalBusName   = "org.freedesktop.portal.Desktop"
	portalObject    = "/org/freedesktop/portal/desktop"
	portalInterface = "org.freedesktop.portal.Settings"
	portalNamespace = "org.freedesktop.appearance"
	portalKey       = "color-scheme"
)

// PortalReader reads the desktop light/dark preference from the XDG Desktop
// Portal settings API.
type PortalReader struct {
	Connect func() (*dbus.Conn, error)
}

func (r PortalReader) CurrentPreference(ctx context.Context) (core.SystemThemePreference, error) {
	conn, err := r.connect()
	if err != nil {
		return core.SystemThemeUnknown, err
	}
	defer func() { _ = conn.Close() }()
	value, err := readPortalColorScheme(ctx, conn.Object(portalBusName, dbus.ObjectPath(portalObject)))
	if err != nil {
		return core.SystemThemeUnknown, err
	}
	return core.ParseSystemThemePreference(value), nil
}

func (r PortalReader) connect() (*dbus.Conn, error) {
	if r.Connect != nil {
		return r.Connect()
	}
	return dbus.ConnectSessionBus()
}

type portalCaller interface {
	CallWithContext(ctx context.Context, method string, flags dbus.Flags, args ...any) *dbus.Call
}

func readPortalColorScheme(ctx context.Context, obj portalCaller) (uint32, error) {
	var value dbus.Variant
	err := obj.CallWithContext(ctx, portalInterface+".ReadOne", 0, portalNamespace, portalKey).Store(&value)
	if err == nil {
		return variantUint32(value)
	}
	if isUnknownMethodError(err) {
		if err := obj.CallWithContext(ctx, portalInterface+".Read", 0, portalNamespace, portalKey).Store(&value); err != nil {
			return 0, err
		}
		return variantUint32(value)
	}
	return 0, err
}

func isUnknownMethodError(err error) bool {
	var dbusErr dbus.Error
	if !errors.As(err, &dbusErr) {
		return false
	}
	return dbusErr.Name == "org.freedesktop.DBus.Error.UnknownMethod"
}

func variantUint32(variant dbus.Variant) (uint32, error) {
	value := variant.Value()
	for {
		switch typed := value.(type) {
		case uint32:
			return typed, nil
		case dbus.Variant:
			value = typed.Value()
		case uint:
			return uint32(typed), nil
		case int32:
			if typed < 0 {
				return 0, fmt.Errorf("portal color-scheme variant was negative: %d", typed)
			}
			return uint32(typed), nil
		case int:
			if typed < 0 {
				return 0, fmt.Errorf("portal color-scheme variant was negative: %d", typed)
			}
			return uint32(typed), nil
		default:
			return 0, fmt.Errorf("portal color-scheme variant carried %T, want uint32", value)
		}
	}
}
