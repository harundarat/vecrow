package logger

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
)

// Redacted replaces the value of any attribute whose key is sensitive.
const Redacted = "[REDACTED]"

// sensitiveKeys are attribute keys whose values must never reach the logs
// (spec.md §8: session tokens, guest tokens and OTP codes are never logged).
// This is a safety net; code should not log these values in the first place.
var sensitiveKeys = map[string]struct{}{
	"authorization": {},
	"cookie":        {},
	"set-cookie":    {},
	"token":         {},
	"access_token":  {},
	"refresh_token": {},
	"session_token": {},
	"guest_token":   {},
	"x-guest-token": {},
	"otp":           {},
	"password":      {},
	"secret":        {},
	"signature":     {},
	"private_key":   {},
	"api_key":       {},
}

// replaceAttr redacts sensitive keys and shortens source locations to
// "package/file.go:line".
func replaceAttr(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 && a.Key == slog.SourceKey {
		if src, ok := a.Value.Any().(*slog.Source); ok && src != nil {
			dir := filepath.Base(filepath.Dir(src.File))
			return slog.String(slog.SourceKey, fmt.Sprintf("%s/%s:%d", dir, filepath.Base(src.File), src.Line))
		}
	}
	if _, ok := sensitiveKeys[strings.ToLower(a.Key)]; ok {
		return slog.String(a.Key, Redacted)
	}
	return a
}
