//go:build windows

package winservice

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows/svc"
)

type RunFunc func(ctx context.Context, logf func(string, ...any)) error

type Options struct {
	Name    string
	LogFile string
	Run     RunFunc
}

func Run(opts Options) error {
	if opts.Name == "" {
		return fmt.Errorf("service name is required")
	}
	if opts.Run == nil {
		return fmt.Errorf("service run function is required")
	}
	logger, closeLog, err := newLogger(opts.LogFile)
	if err != nil {
		return err
	}
	defer closeLog()
	logf := func(format string, args ...any) { logger.Printf(format, args...) }
	isService, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if !isService {
		logf("running in console mode")
		ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		return opts.Run(ctx, logf)
	}
	return svc.Run(opts.Name, &handler{name: opts.Name, run: opts.Run, logf: logf})
}

type handler struct {
	name string
	run  RunFunc
	logf func(string, ...any)
}

func (h *handler) Execute(args []string, requests <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const accepts = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	var runErr error
	go func() {
		defer wg.Done()
		runErr = h.run(ctx, h.logf)
	}()
	changes <- svc.Status{State: svc.Running, Accepts: accepts}
	for req := range requests {
		switch req.Cmd {
		case svc.Interrogate:
			changes <- req.CurrentStatus
		case svc.Stop, svc.Shutdown:
			h.logf("service stopping")
			changes <- svc.Status{State: svc.StopPending}
			cancel()
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				h.logf("service stop timed out")
			}
			if runErr != nil && ctx.Err() == nil {
				h.logf("service stopped with error: %v", runErr)
			}
			changes <- svc.Status{State: svc.Stopped}
			return false, 0
		default:
			h.logf("unsupported service command: %v", req.Cmd)
		}
	}
	cancel()
	wg.Wait()
	return false, 0
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
