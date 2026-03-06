package logger

import (
	"path/filepath"
	"testing"
)

func TestSetupWithDirCreatesLogInCustomDir(t *testing.T) {
	oldGlobal := global
	fileCloserMu.Lock()
	oldCloser := fileCloser
	fileCloserMu.Unlock()
	defer func() {
		global = oldGlobal
		fileCloserMu.Lock()
		if fileCloser != nil {
			_ = fileCloser.Close()
		}
		fileCloser = oldCloser
		fileCloserMu.Unlock()
	}()

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
}
