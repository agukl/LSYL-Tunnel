//go:build windows && !386

package tunnel

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	virtualRedirectHelperFlag      = "virtual-redirect-helper"
	virtualRedirectHelperSession   = "session"
	virtualRedirectHelperAnchor    = "anchor"
	virtualRedirectHelperTimeout   = 60 * time.Second
	virtualRedirectShutdownTimeout = 8 * time.Second
	virtualRedirectResultDirectory = "virtual-redirect-helper-results"
	winDivertPriority              = 123
	winDivertMTUMax                = 40 + 0xffff
	seeMaskNoCloseProcess          = 0x00000040
)

type virtualRedirectHelperOptions struct {
	Action      string
	Rules       []virtualRedirectRule
	ResultToken string
	AnchorPID   uint32
}

type virtualRedirectHelperResult struct {
	Ready   bool   `json:"ready"`
	Stopped bool   `json:"stopped"`
	Error   string `json:"error,omitempty"`
}

type windowsVirtualRedirectSession struct {
	anchorStdin   io.WriteCloser
	anchorCmd     *exec.Cmd
	helperProcess windows.Handle
	resultPath    string
	done          chan struct{}
	closing       atomic.Bool
	closeOnce     sync.Once
	mu            sync.Mutex
	err           error
}

type winDivertAddress struct {
	Timestamp int64
	Flags     uint32
	Reserved2 uint32
	Data      [64]byte
}

type winDivertAPI struct {
	dll           *windows.DLL
	open          *windows.Proc
	recv          *windows.Proc
	send          *windows.Proc
	close         *windows.Proc
	calcChecksums *windows.Proc
}

type winDivertRedirect struct {
	api         *winDivertAPI
	handle      windows.Handle
	table       virtualRedirectTable
	closing     atomic.Bool
	closeOnce   sync.Once
	releaseOnce sync.Once
}

type shellExecuteInfo struct {
	Size       uint32
	Mask       uint32
	Window     windows.Handle
	Verb       *uint16
	File       *uint16
	Parameters *uint16
	Directory  *uint16
	Show       int32
	Instance   windows.Handle
	IDList     uintptr
	Class      *uint16
	ClassKey   windows.Handle
	HotKey     uint32
	Icon       windows.Handle
	Process    windows.Handle
}

var shellExecuteExW = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

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
	resultPath, err := virtualRedirectResultPath(token)
	if err != nil {
		return nil, err
	}
	if err := prepareVirtualRedirectResultPath(resultPath); err != nil {
		return nil, err
	}

	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	anchorCmd := exec.Command(exe, "-"+virtualRedirectHelperFlag, virtualRedirectHelperAnchor)
	anchorCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	anchorStdin, err := anchorCmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("create virtual redirect session anchor: %w", err)
	}
	if err := anchorCmd.Start(); err != nil {
		_ = anchorStdin.Close()
		return nil, fmt.Errorf("start virtual redirect session anchor: %w", err)
	}

	failAnchor := func(cause error) (virtualRedirectSession, error) {
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
		"-virtual-redirect-result", token,
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
		resultPath:    resultPath,
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
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		result, ready, err := readVirtualRedirectHelperResult(s.resultPath)
		if err != nil {
			return err
		}
		if ready {
			if result.Error != "" {
				return errors.New(result.Error)
			}
			if result.Stopped {
				return fmt.Errorf("virtual redirect helper stopped before setup completed")
			}
			if result.Ready {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.done:
			if err := s.Err(); err != nil {
				return err
			}
			return fmt.Errorf("virtual redirect helper stopped before setup completed")
		case <-timer.C:
			return fmt.Errorf("virtual redirect administrator helper timed out")
		case <-ticker.C:
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
	result, ready, resultErr := readVirtualRedirectHelperResult(s.resultPath)
	if resultErr != nil {
		sessionErr = errors.Join(sessionErr, resultErr)
	} else if ready && result.Error != "" {
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
		if s.helperProcess != 0 {
			_ = windows.CloseHandle(s.helperProcess)
			s.helperProcess = 0
		}
		_ = os.Remove(s.resultPath)
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

func HandleVirtualRedirectHelperArgs(args []string) (bool, error) {
	opts, handled, err := parseVirtualRedirectHelperArgs(args)
	if !handled || err != nil {
		return handled, err
	}
	if opts.Action == virtualRedirectHelperAnchor {
		_, err := io.Copy(io.Discard, os.Stdin)
		return true, err
	}
	return true, runVirtualRedirectSessionHelper(opts)
}

func parseVirtualRedirectHelperArgs(args []string) (virtualRedirectHelperOptions, bool, error) {
	var opts virtualRedirectHelperOptions
	handled := false
	for _, arg := range args {
		name := strings.TrimLeft(strings.TrimSpace(arg), "-")
		if name == virtualRedirectHelperFlag || strings.HasPrefix(name, virtualRedirectHelperFlag+"=") {
			handled = true
			break
		}
	}
	if !handled {
		return opts, false, nil
	}
	fs := flag.NewFlagSet("virtual-redirect-helper", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	action := fs.String(virtualRedirectHelperFlag, "", "")
	rulesText := fs.String("virtual-redirect-rules", "", "")
	resultToken := fs.String("virtual-redirect-result", "", "")
	anchorPID := fs.Uint("virtual-redirect-anchor-pid", 0, "")
	if err := fs.Parse(args); err != nil {
		return opts, true, err
	}
	if fs.NArg() != 0 {
		return opts, true, fmt.Errorf("unexpected virtual redirect helper arguments")
	}
	opts.Action = strings.ToLower(strings.TrimSpace(*action))
	if opts.Action != virtualRedirectHelperSession && opts.Action != virtualRedirectHelperAnchor {
		return opts, true, fmt.Errorf("virtual redirect helper action must be session or anchor")
	}
	if opts.Action == virtualRedirectHelperSession {
		var err error
		opts.Rules, err = decodeVirtualRedirectRules(*rulesText)
		if err != nil {
			return opts, true, err
		}
		opts.ResultToken = strings.ToLower(strings.TrimSpace(*resultToken))
		if !validVirtualRedirectToken(opts.ResultToken) {
			return opts, true, fmt.Errorf("invalid virtual redirect helper result token")
		}
		if *anchorPID == 0 || *anchorPID > uint(^uint32(0)) {
			return opts, true, fmt.Errorf("virtual redirect helper requires a valid anchor PID")
		}
		opts.AnchorPID = uint32(*anchorPID)
	}
	return opts, true, nil
}

func runVirtualRedirectSessionHelper(opts virtualRedirectHelperOptions) error {
	anchor, err := windows.OpenProcess(windows.SYNCHRONIZE, false, opts.AnchorPID)
	if err != nil {
		return finishVirtualRedirectHelper(opts.ResultToken, false, fmt.Errorf("open virtual redirect session anchor: %w", err))
	}
	defer windows.CloseHandle(anchor)
	if status, err := windows.WaitForSingleObject(anchor, 0); err != nil || status != uint32(windows.WAIT_TIMEOUT) {
		if err == nil {
			err = fmt.Errorf("virtual redirect session ended before setup completed")
		}
		return finishVirtualRedirectHelper(opts.ResultToken, false, err)
	}

	redirect, err := openWinDivertRedirect(opts.Rules)
	if err != nil {
		return finishVirtualRedirectHelper(opts.ResultToken, false, err)
	}
	if err := writeVirtualRedirectHelperResult(opts.ResultToken, virtualRedirectHelperResult{Ready: true}); err != nil {
		redirect.Close()
		redirect.Release()
		return err
	}
	runErr := redirect.Run(anchor)
	return finishVirtualRedirectHelper(opts.ResultToken, true, runErr)
}

func finishVirtualRedirectHelper(token string, ready bool, actionErr error) error {
	result := virtualRedirectHelperResult{Ready: ready, Stopped: true}
	if actionErr != nil {
		result.Error = actionErr.Error()
	}
	writeErr := writeVirtualRedirectHelperResult(token, result)
	return errors.Join(actionErr, writeErr)
}

func openWinDivertRedirect(rules []virtualRedirectRule) (*winDivertRedirect, error) {
	table, err := newVirtualRedirectTable(rules)
	if err != nil {
		return nil, err
	}
	filter, err := buildVirtualRedirectFilter(rules)
	if err != nil {
		return nil, err
	}
	api, err := loadWinDivertAPI()
	if err != nil {
		return nil, err
	}
	handle, err := api.Open(filter)
	if err != nil {
		api.Release()
		return nil, err
	}
	return &winDivertRedirect{api: api, handle: handle, table: table}, nil
}

func (r *winDivertRedirect) Run(anchor windows.Handle) error {
	anchorDone := make(chan struct{})
	go func() {
		_, _ = windows.WaitForSingleObject(anchor, windows.INFINITE)
		close(anchorDone)
		r.Close()
	}()
	err := r.packetLoop()
	r.Close()
	r.Release()
	select {
	case <-anchorDone:
		return nil
	default:
		return err
	}
}

func (r *winDivertRedirect) Close() {
	if r == nil {
		return
	}
	r.closeOnce.Do(func() {
		r.closing.Store(true)
		if r.api != nil && r.handle != 0 && r.handle != windows.InvalidHandle {
			_ = r.api.Close(r.handle)
		}
	})
}

func (r *winDivertRedirect) Release() {
	if r == nil {
		return
	}
	r.releaseOnce.Do(func() {
		if r.api != nil {
			r.api.Release()
		}
	})
}

func (r *winDivertRedirect) packetLoop() error {
	packet := make([]byte, winDivertMTUMax)
	for {
		var address winDivertAddress
		packetLen, err := r.api.Recv(r.handle, packet, &address)
		if err != nil {
			if r.closing.Load() {
				return nil
			}
			return err
		}
		outbound := address.Flags&(1<<17) != 0
		newOutbound, action := rewriteVirtualRedirectPacket(packet[:packetLen], outbound, r.table)
		if action == virtualRedirectDrop {
			continue
		}
		if action == virtualRedirectInject {
			if newOutbound {
				address.Flags |= 1 << 17
			} else {
				address.Flags &^= 1 << 17
			}
			if err := r.api.CalcChecksums(packet[:packetLen], &address); err != nil {
				return err
			}
		}
		if err := r.api.Send(r.handle, packet[:packetLen], &address); err != nil {
			if r.closing.Load() {
				return nil
			}
			return err
		}
	}
}

func loadWinDivertAPI() (*winDivertAPI, error) {
	if unsafe.Sizeof(winDivertAddress{}) != 80 {
		return nil, fmt.Errorf("unsupported WinDivert address layout")
	}
	dllPath, err := resolveWinDivertDLL()
	if err != nil {
		return nil, err
	}
	dll, err := windows.LoadDLL(dllPath)
	if err != nil {
		return nil, fmt.Errorf("load WinDivert.dll: %w", err)
	}
	loadProc := func(name string) (*windows.Proc, error) {
		proc, err := dll.FindProc(name)
		if err != nil {
			return nil, fmt.Errorf("load %s from WinDivert.dll: %w", name, err)
		}
		return proc, nil
	}
	api := &winDivertAPI{dll: dll}
	if api.open, err = loadProc("WinDivertOpen"); err != nil {
		api.Release()
		return nil, err
	}
	if api.recv, err = loadProc("WinDivertRecv"); err != nil {
		api.Release()
		return nil, err
	}
	if api.send, err = loadProc("WinDivertSend"); err != nil {
		api.Release()
		return nil, err
	}
	if api.close, err = loadProc("WinDivertClose"); err != nil {
		api.Release()
		return nil, err
	}
	if api.calcChecksums, err = loadProc("WinDivertHelperCalcChecksums"); err != nil {
		api.Release()
		return nil, err
	}
	return api, nil
}

func (a *winDivertAPI) Open(filter string) (windows.Handle, error) {
	filterPtr, err := windows.BytePtrFromString(filter)
	if err != nil {
		return 0, err
	}
	value, _, callErr := a.open.Call(uintptr(unsafe.Pointer(filterPtr)), 0, winDivertPriority, 0)
	handle := windows.Handle(value)
	if handle == windows.InvalidHandle {
		return 0, fmt.Errorf("WinDivertOpen failed: %w", normalizeWindowsCallError(callErr))
	}
	return handle, nil
}

func (a *winDivertAPI) Recv(handle windows.Handle, packet []byte, address *winDivertAddress) (int, error) {
	var packetLen uint32
	value, _, callErr := a.recv.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&packet[0])),
		uintptr(len(packet)),
		uintptr(unsafe.Pointer(&packetLen)),
		uintptr(unsafe.Pointer(address)),
	)
	if value == 0 {
		return 0, fmt.Errorf("WinDivertRecv failed: %w", normalizeWindowsCallError(callErr))
	}
	return int(packetLen), nil
}

func (a *winDivertAPI) Send(handle windows.Handle, packet []byte, address *winDivertAddress) error {
	value, _, callErr := a.send.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&packet[0])),
		uintptr(len(packet)),
		0,
		uintptr(unsafe.Pointer(address)),
	)
	if value == 0 {
		return fmt.Errorf("WinDivertSend failed: %w", normalizeWindowsCallError(callErr))
	}
	return nil
}

func (a *winDivertAPI) CalcChecksums(packet []byte, address *winDivertAddress) error {
	value, _, callErr := a.calcChecksums.Call(
		uintptr(unsafe.Pointer(&packet[0])),
		uintptr(len(packet)),
		uintptr(unsafe.Pointer(address)),
		0,
	)
	if value == 0 {
		return fmt.Errorf("WinDivert checksum update failed: %w", normalizeWindowsCallError(callErr))
	}
	return nil
}

func (a *winDivertAPI) Close(handle windows.Handle) error {
	value, _, callErr := a.close.Call(uintptr(handle))
	if value == 0 {
		return normalizeWindowsCallError(callErr)
	}
	return nil
}

func (a *winDivertAPI) Release() {
	if a != nil && a.dll != nil {
		_ = a.dll.Release()
		a.dll = nil
	}
}

func resolveWinDivertDLL() (string, error) {
	candidates := make([]string, 0, 4)
	if dir := strings.TrimSpace(os.Getenv("LSYL_WINDIVERT_DIR")); dir != "" {
		candidates = append(candidates, filepath.Join(dir, "WinDivert.dll"))
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "WinDivert.dll"))
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, "third_party", "windivert", "2.2.2", "x64", "WinDivert.dll"),
			filepath.Join(cwd, "tool", "windivert", "x64", "WinDivert.dll"),
		)
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("WinDivert.dll is missing; repair the standard 64-bit Windows client installation")
}

func normalizeWindowsCallError(err error) error {
	if err == nil || errors.Is(err, syscall.Errno(0)) {
		return windows.ERROR_GEN_FAILURE
	}
	return err
}

func shellExecuteElevated(exe, parameters, directory string) (windows.Handle, error) {
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return 0, err
	}
	file, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return 0, err
	}
	params, err := windows.UTF16PtrFromString(parameters)
	if err != nil {
		return 0, err
	}
	dir, err := windows.UTF16PtrFromString(directory)
	if err != nil {
		return 0, err
	}
	info := shellExecuteInfo{
		Size:       uint32(unsafe.Sizeof(shellExecuteInfo{})),
		Mask:       seeMaskNoCloseProcess,
		Verb:       verb,
		File:       file,
		Parameters: params,
		Directory:  dir,
		Show:       windows.SW_HIDE,
	}
	value, _, callErr := shellExecuteExW.Call(uintptr(unsafe.Pointer(&info)))
	if value == 0 {
		return 0, normalizeWindowsCallError(callErr)
	}
	if info.Process == 0 {
		return 0, fmt.Errorf("elevated virtual redirect helper did not return a process handle")
	}
	return info.Process, nil
}

func encodeVirtualRedirectRules(rules []virtualRedirectRule) (string, error) {
	rules, err := normalizeVirtualRedirectRules(rules)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(rules)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func decodeVirtualRedirectRules(value string) ([]virtualRedirectRule, error) {
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return nil, fmt.Errorf("decode virtual redirect rules: %w", err)
	}
	var rules []virtualRedirectRule
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("parse virtual redirect rules: %w", err)
	}
	return normalizeVirtualRedirectRules(rules)
}

func writeVirtualRedirectHelperResult(token string, result virtualRedirectHelperResult) error {
	path, err := virtualRedirectResultPath(token)
	if err != nil {
		return err
	}
	cleanupOldVirtualRedirectResults(filepath.Dir(path))
	return writeVirtualRedirectJSONAtomic(path, result)
}

func readVirtualRedirectHelperResult(path string) (virtualRedirectHelperResult, bool, error) {
	var result virtualRedirectHelperResult
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return result, false, nil
	}
	if err != nil {
		return result, false, err
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, false, fmt.Errorf("read virtual redirect helper result: %w", err)
	}
	return result, true, nil
}

func writeVirtualRedirectJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	from, _ := windows.UTF16PtrFromString(tmpPath)
	to, _ := windows.UTF16PtrFromString(path)
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

func prepareVirtualRedirectResultPath(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale virtual redirect helper result: %w", err)
	}
	cleanupOldVirtualRedirectResults(filepath.Dir(path))
	return nil
}

func virtualRedirectResultPath(token string) (string, error) {
	if !validVirtualRedirectToken(token) {
		return "", fmt.Errorf("invalid virtual redirect helper result token")
	}
	root, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(root) == "" {
		root = os.TempDir()
	}
	return filepath.Join(root, "LSYL Tunnel", virtualRedirectResultDirectory, token+".json"), nil
}

func newVirtualRedirectToken() (string, error) {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func validVirtualRedirectToken(token string) bool {
	if len(token) != 32 {
		return false
	}
	_, err := hex.DecodeString(token)
	return err == nil
}

func cleanupOldVirtualRedirectResults(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		info, err := entry.Info()
		if err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}

func virtualRedirectCommandLine(args ...string) string {
	escaped := make([]string, 0, len(args))
	for _, arg := range args {
		escaped = append(escaped, syscall.EscapeArg(arg))
	}
	return strings.Join(escaped, " ")
}
