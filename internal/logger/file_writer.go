package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type rotatingFileWriter struct {
	mu      sync.Mutex
	dir     string
	prefix  string
	maxSize int64
	maxAge  time.Duration
	now     func() time.Time

	file        *os.File
	currentDate string
	seq         int
	size        int64
}

func newRotatingFileWriter(dir, prefix string, maxSize int64, maxAge time.Duration, now func() time.Time) (*rotatingFileWriter, *rotatingFileWriter, error) {
	if now == nil {
		now = time.Now
	}
	if maxSize <= 0 {
		maxSize = defaultMaxFileSize
	}
	if maxAge <= 0 {
		maxAge = defaultMaxAge
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, err
	}
	w := &rotatingFileWriter{
		dir:     dir,
		prefix:  prefix,
		maxSize: maxSize,
		maxAge:  maxAge,
		now:     now,
		seq:     -1,
	}
	return w, w, nil
}

func (w *rotatingFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(p) == 0 {
		return 0, nil
	}

	now := w.now()
	date := now.Format("2006-01-02")
	if err := w.ensureFile(date, len(p)); err != nil {
		return 0, err
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *rotatingFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *rotatingFileWriter) ensureFile(date string, incoming int) error {
	if w.file != nil && w.currentDate == date && (w.size+int64(incoming) <= w.maxSize) {
		return nil
	}
	return w.rotate(date)
}

func (w *rotatingFileWriter) rotate(date string) error {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}

	if w.currentDate != date {
		next, err := w.nextSeq(date)
		if err != nil {
			return err
		}
		w.seq = next
	} else {
		w.seq++
	}

	path := filepath.Join(w.dir, w.fileName(date, w.seq))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}

	w.file = f
	w.currentDate = date
	w.size = info.Size()
	w.cleanupOldLocked(w.now())
	return nil
}

func (w *rotatingFileWriter) fileName(date string, seq int) string {
	if seq <= 0 {
		return fmt.Sprintf("%s-%s.log", w.prefix, date)
	}
	return fmt.Sprintf("%s-%s.%d.log", w.prefix, date, seq)
}

func (w *rotatingFileWriter) nextSeq(date string) (int, error) {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return 0, err
	}
	maxSeq := -1
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fileDate, seq, ok := w.parseFileName(entry.Name())
		if !ok || fileDate != date {
			continue
		}
		if seq > maxSeq {
			maxSeq = seq
		}
	}
	return maxSeq + 1, nil
}

func (w *rotatingFileWriter) cleanupOldLocked(now time.Time) {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fileDate, _, ok := w.parseFileName(entry.Name())
		if !ok {
			continue
		}
		parsedDate, err := time.ParseInLocation("2006-01-02", fileDate, now.Location())
		if err != nil {
			continue
		}
		if now.Sub(parsedDate) > w.maxAge {
			_ = os.Remove(filepath.Join(w.dir, entry.Name()))
		}
	}
}

func (w *rotatingFileWriter) parseFileName(name string) (date string, seq int, ok bool) {
	prefix := w.prefix + "-"
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".log") {
		return "", 0, false
	}
	base := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".log")
	parts := strings.Split(base, ".")
	date = parts[0]
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return "", 0, false
	}
	if len(parts) == 1 {
		return date, 0, true
	}
	if len(parts) == 2 {
		n, err := strconv.Atoi(parts[1])
		if err != nil || n < 0 {
			return "", 0, false
		}
		return date, n, true
	}
	return "", 0, false
}
