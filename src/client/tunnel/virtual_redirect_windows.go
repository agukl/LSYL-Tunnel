//go:build windows && !386

package tunnel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const (
	virtualRedirectHelperTimeout   = 60 * time.Second
	virtualRedirectShutdownTimeout = 8 * time.Second
)

type windowsVirtualRedirectSession struct {
	anchorStdin   io.WriteCloser
	anchorCmd     *exec.Cmd
	helperProcess windows.Handle
	control       *virtualRedirectControlServer
	done          chan struct{}
	closing       atomic.Bool
	closeOnce     sync.Once
	mu            sync.Mutex
	err           error
}

func startVirtualRedirectSession(ctx context.Context, rules []virtualRedirectRule) (virtualRedirectSession, error) {
	rules, err := normalizeVirtualRedirectRules(rules)
	if err != nil || len(rules) == 0 {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	token, err := newVirtualRedirectToken()
	if err != nil {
		return nil, err
	}
	control, err := newVirtualRedirectControlServer(token)
	if err != nil {
		return nil, err
	}

	exe, err := os.Executable()
	if err != nil {
		_ = control.Close()
		return nil, err
	}
	anchorCmd := exec.Command(exe, "-"+virtualRedirectHelperFlag, virtualRedirectHelperAnchor)
	anchorCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	anchorStdin, err := anchorCmd.StdinPipe()
	if err != nil {
		_ = control.Close()
		return nil, fmt.Errorf("create virtual redirect session anchor: %w", err)
	}
	if err := anchorCmd.Start(); err != nil {
		_ = anchorStdin.Close()
		_ = control.Close()
		return nil, fmt.Errorf("start virtual redirect session anchor: %w", err)
	}

	failAnchor := func(cause error) (virtualRedirectSession, error) {
		_ = control.Close()
		_ = anchorStdin.Close()
		_ = anchorCmd.Process.Kill()
		_ = anchorCmd.Wait()
		return nil, cause
	}
	rulesText, err := encodeVirtualRedirectRules(rules)
	if err != nil {
		return failAnchor(err)
	}
	args := []string{
		"-" + virtualRedirectHelperFlag, virtualRedirectHelperSession,
		"-virtual-redirect-rules", rulesText,
		"-virtual-redirect-control", control.Address(),
		"-virtual-redirect-token", token,
		"-virtual-redirect-anchor-pid", strconv.Itoa(anchorCmd.Process.Pid),
	}
	helperProcess, err := shellExecuteElevated(exe, virtualRedirectCommandLine(args...), filepath.Dir(exe))
	if err != nil {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return failAnchor(fmt.Errorf("administrator authorization for virtual forwarding was cancelled"))
		}
		return failAnchor(fmt.Errorf("start virtual redirect administrator helper: %w", err))
	}

	session := &windowsVirtualRedirectSession{
		anchorStdin:   anchorStdin,
		anchorCmd:     anchorCmd,
		helperProcess: helperProcess,
		control:       control,
		done:          make(chan struct{}),
	}
	go session.monitorHelper()
	if err := session.waitUntilReady(ctx, virtualRedirectHelperTimeout); err != nil {
		return nil, errors.Join(err, session.Close())
	}
	return session, nil
}

func (s *windowsVirtualRedirectSession) waitUntilReady(ctx context.Context, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case result, ok := <-s.control.Results():
			if !ok {
				if err := s.control.Err(); err != nil {
					return err
				}
				return fmt.Errorf("virtual redirect administrator helper closed its control channel before setup completed")
			}
			if result.Error != "" {
				return errors.New(result.Error)
			}
			if result.Stopped {
				return fmt.Errorf("virtual redirect helper stopped before setup completed")
			}
			if result.Ready {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		case <-s.done:
			if err := s.Err(); err != nil {
				return err
			}
			return fmt.Errorf("virtual redirect helper stopped before setup completed")
		case <-timer.C:
			return fmt.Errorf("virtual redirect administrator helper timed out")
		}
	}
}

func (s *windowsVirtualRedirectSession) monitorHelper() {
	status, waitErr := windows.WaitForSingleObject(s.helperProcess, windows.INFINITE)
	var sessionErr error
	if waitErr != nil {
		sessionErr = fmt.Errorf("wait for virtual redirect helper: %w", waitErr)
	} else if status != windows.WAIT_OBJECT_0 {
		sessionErr = fmt.Errorf("wait for virtual redirect helper returned %d", status)
	}
	select {
	case <-s.control.Done():
	case <-time.After(2 * time.Second):
		sessionErr = errors.Join(sessionErr, fmt.Errorf("virtual redirect helper control channel did not close"))
		_ = s.control.Close()
	}
	if controlErr := s.control.Err(); controlErr != nil {
		sessionErr = errors.Join(sessionErr, controlErr)
	}
	if result := s.control.LastResult(); result.Error != "" {
		sessionErr = errors.Join(sessionErr, errors.New(result.Error))
	}
	var exitCode uint32
	if err := windows.GetExitCodeProcess(s.helperProcess, &exitCode); err != nil {
		sessionErr = errors.Join(sessionErr, fmt.Errorf("read virtual redirect helper exit code: %w", err))
	} else if exitCode != 0 && sessionErr == nil {
		sessionErr = fmt.Errorf("virtual redirect helper exited with code %d", exitCode)
	}
	if !s.closing.Load() && sessionErr == nil {
		sessionErr = fmt.Errorf("virtual redirect helper stopped unexpectedly")
	}
	s.mu.Lock()
	s.err = sessionErr
	s.mu.Unlock()
	close(s.done)
}

func (s *windowsVirtualRedirectSession) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.closing.Store(true)
		var closeErr error
		if s.anchorStdin != nil {
			if err := s.anchorStdin.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
				closeErr = errors.Join(closeErr, err)
			}
		}
		if s.anchorCmd != nil {
			if err := waitForVirtualRedirectAnchor(s.anchorCmd); err != nil {
				closeErr = errors.Join(closeErr, err)
			}
		}
		select {
		case <-s.done:
		case <-time.After(virtualRedirectShutdownTimeout):
			closeErr = errors.Join(closeErr, fmt.Errorf("virtual redirect helper shutdown timed out"))
		}
		if s.control != nil {
			if err := s.control.Close(); err != nil {
				closeErr = errors.Join(closeErr, err)
			}
		}
		if s.helperProcess != 0 {
			_ = windows.CloseHandle(s.helperProcess)
			s.helperProcess = 0
		}
		s.mu.Lock()
		s.err = errors.Join(s.err, closeErr)
		s.mu.Unlock()
	})
	return s.Err()
}

func (s *windowsVirtualRedirectSession) Done() <-chan struct{} {
	if s == nil {
		return nil
	}
	return s.done
}

func (s *windowsVirtualRedirectSession) Err() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func waitForVirtualRedirectAnchor(cmd *exec.Cmd) error {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(3 * time.Second):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			return fmt.Errorf("virtual redirect session anchor did not exit")
		}
		return nil
	}
}
