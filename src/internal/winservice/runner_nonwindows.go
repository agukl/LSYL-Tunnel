//go:build !windows

package winservice

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type RunFunc func(ctx context.Context, logf func(string, ...any)) error

type Options struct {
	Name    string
	LogFile string
	Run     RunFunc
}

func Run(opts Options) error {
	if opts.Run == nil {
		return fmt.Errorf("service run function is required")
	}
	logger, closeLog, err := newLogger(opts.LogFile)
	if err != nil {
		return err
	}
	defer closeLog()
	return opts.Run(context.Background(), func(format string, args ...any) { logger.Printf(format, args...) })
}

func newLogger(path string) (*log.Logger, func(), error) {
	if path == "" {
		return log.New(os.Stdout, "", log.LstdFlags), func() {}, nil
	}
	writer := &dailyLogWriter{basePath: path}
	if err := writer.rotateIfNeeded(time.Now()); err != nil {
		return nil, nil, err
	}
	return log.New(writer, "", log.LstdFlags), func() { _ = writer.Close() }, nil
}

type dailyLogWriter struct {
	mu          sync.Mutex
	basePath    string
	currentDate string
	file        *os.File
}

func (w *dailyLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.rotateIfNeeded(time.Now()); err != nil {
		return 0, err
	}
	return w.file.Write(p)
}

func (w *dailyLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *dailyLogWriter) rotateIfNeeded(now time.Time) error {
	date := now.Format("2006-01-02")
	if w.file != nil && w.currentDate == date {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(w.basePath), 0o755); err != nil {
		return err
	}
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	file, err := os.OpenFile(dailyLogPath(w.basePath, date), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	w.currentDate = date
	w.file = file
	return nil
}

func dailyLogPath(basePath, date string) string {
	dir := filepath.Dir(basePath)
	ext := filepath.Ext(basePath)
	name := strings.TrimSuffix(filepath.Base(basePath), ext)
	return filepath.Join(dir, name+"-"+date+ext)
}
