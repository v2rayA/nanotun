package logger

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRotatingFileWriter_BySizeAndDate(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 3, 5, 10, 0, 0, 0, time.Local)
	w, closer, err := newRotatingFileWriter(dir, "nanotun", 12, 72*time.Hour, func() time.Time { return now })
	if err != nil {
		t.Fatalf("newRotatingFileWriter: %v", err)
	}
	defer closer.Close()

	if _, err := w.Write([]byte("1234567890\n")); err != nil {
		t.Fatalf("write 1: %v", err)
	}
	if _, err := w.Write([]byte("abc\n")); err != nil {
		t.Fatalf("write 2: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "nanotun-2026-03-05.log")); err != nil {
		t.Fatalf("expected first log file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "nanotun-2026-03-05.1.log")); err != nil {
		t.Fatalf("expected rotated log file: %v", err)
	}

	now = now.AddDate(0, 0, 1)
	if _, err := w.Write([]byte("next-day\n")); err != nil {
		t.Fatalf("write next day: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "nanotun-2026-03-06.log")); err != nil {
		t.Fatalf("expected next-day log file: %v", err)
	}
}

func TestRotatingFileWriter_CleanupOldLogs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "nanotun-2026-03-01.log"), []byte("old"), 0o644); err != nil {
		t.Fatalf("seed old log: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nanotun-2026-03-04.log"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("seed keep log: %v", err)
	}

	now := time.Date(2026, 3, 5, 10, 0, 0, 0, time.Local)
	w, closer, err := newRotatingFileWriter(dir, "nanotun", 1024, 72*time.Hour, func() time.Time { return now })
	if err != nil {
		t.Fatalf("newRotatingFileWriter: %v", err)
	}
	defer closer.Close()

	if _, err := w.Write([]byte("trigger-cleanup\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "nanotun-2026-03-01.log")); !os.IsNotExist(err) {
		t.Fatalf("expected old log to be deleted, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "nanotun-2026-03-04.log")); err != nil {
		t.Fatalf("expected recent log to be kept: %v", err)
	}
}
