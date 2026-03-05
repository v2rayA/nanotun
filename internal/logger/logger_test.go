package logger

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupWithDirCreatesLogInCustomDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "custom-logs")
	log := SetupWithDir("info", dir)
	log.Info("test custom dir")

	pattern := filepath.Join(dir, "nanotun-*.log")
	files, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob logs: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("expected log file in %s", dir)
	}

	global = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
}
