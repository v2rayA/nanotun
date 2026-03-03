package logger

import (
	"log/slog"
	"os"
	"strings"
)

var global = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

// Setup configures the global slog logger according to the requested level.
func Setup(level string) *slog.Logger {
	lvl := parseLevel(level)
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	global = slog.New(handler)
	return global
}

// L returns the globally configured logger.
func L() *slog.Logger {
	return global
}

func parseLevel(v string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "none", "silent":
		return slog.LevelError + 10
	default:
		return slog.LevelInfo
	}
}
