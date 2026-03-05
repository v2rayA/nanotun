package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

var global = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
var (
	fileCloserMu sync.Mutex
	fileCloser   io.Closer
)

const (
	defaultLogDir      = "logs"
	defaultLogPrefix   = "nanotun"
	defaultMaxFileSize = 1 << 20 // 1 MiB
	defaultMaxAge      = 72 * time.Hour
)

// Setup configures the global slog logger according to the requested level.
func Setup(level string) *slog.Logger {
	return SetupWithDir(level, "")
}

// SetupWithDir configures the global slog logger according to the requested
// level and log directory.
func SetupWithDir(level, logDir string) *slog.Logger {
	lvl := parseLevel(level)
	out := io.Writer(os.Stderr)
	if strings.TrimSpace(logDir) == "" {
		logDir = defaultLogDir
	}

	if w, closer, err := newRotatingFileWriter(logDir, defaultLogPrefix, defaultMaxFileSize, defaultMaxAge, time.Now); err == nil {
		fileCloserMu.Lock()
		if fileCloser != nil {
			_ = fileCloser.Close()
		}
		fileCloser = closer
		fileCloserMu.Unlock()
		out = io.MultiWriter(os.Stderr, w)
	}

	handler := slog.NewTextHandler(out, &slog.HandlerOptions{Level: lvl})
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
