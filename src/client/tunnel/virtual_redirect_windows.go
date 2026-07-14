//go:build windows && !386

package tunnel

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
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
	virtualRedirectHelperFlag       = "virtual-redirect-helper"
	virtualRedirectHelperSession    = "session"
	virtualRedirectHelperAnchor     = "anchor"
	virtualRedirectHelperTimeout    = 60 * time.Second
	virtualRedirectShutdownTimeout  = 8 * time.Second
	virtualRedirectControlMaxBytes  = 64 * 1024
	virtualRedirectHandshakeTimeout = 3 * time.Second
	winDivertPriority               = 123
	winDivertMTUMax                 = 40 + 0xffff
	winDivertFlagOutbound           = uint32(1 << 17)
	winDivertFlagLoopback           = uint32(1 << 18)
	seeMaskNoCloseProcess           = 0x00000040
	winDivertDLLSHA256              = "c1e060ee19444a259b2162f8af0f3fe8c4428a1c6f694dce20de194ac8d7d9a2"
	winDivertDriverSHA256           = "8da085332782708d8767bcace5327a6ec7283c17cfb85e40b03cd2323a90ddc2"
)

type virtualRedirectHelperOptions struct {
	Action       string
	Rules        []virtualRedirectRule
	ControlAddr  string
	ControlToken string
	AnchorPID    uint32
}

type virtualRedirectHelperResult struct {
	Token     string `json:"token"`
	Connected bool   `json:"connected,omitempty"`
	Ready     bool   `json:"ready,omitempty"`
	Stopped   bool   `json:"stopped,omitempty"`
	Error     string `json:"error,omitempty"`
}

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

type virtualRedirectControlServer struct {
	listener  net.Listener
	token     string
	results   chan virtualRedirectHelperResult
	done      chan struct{}
	closing   atomic.Bool
	closeOnce sync.Once
	mu        sync.Mutex
	conn      net.Conn
	decoder   *json.Decoder
	last      virtualRedirectHelperResult
	err       error
}

type virtualRedirectControlClient struct {
	conn  net.Conn
	token string
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
	controlAddr := fs.String("virtual-redirect-control", "", "")
	controlToken := fs.String("virtual-redirect-token", "", "")
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
		opts.ControlAddr, err = normalizeVirtualRedirectControlAddress(*controlAddr)
		if err != nil {
			return opts, true, err
		}
		opts.ControlToken = strings.ToLower(strings.TrimSpace(*controlToken))
		if !validVirtualRedirectToken(opts.ControlToken) {
			return opts, true, fmt.Errorf("invalid virtual redirect helper control token")
		}
		if *anchorPID == 0 || *anchorPID > uint(^uint32(0)) {
			return opts, true, fmt.Errorf("virtual redirect helper requires a valid anchor PID")
		}
		opts.AnchorPID = uint32(*anchorPID)
	}
	return opts, true, nil
}

func runVirtualRedirectSessionHelper(opts virtualRedirectHelperOptions) error {
	control, err := connectVirtualRedirectControl(opts.ControlAddr, opts.ControlToken)
	if err != nil {
		return err
	}
	defer control.Close()
	anchor, err := windows.OpenProcess(windows.SYNCHRONIZE, false, opts.AnchorPID)
	if err != nil {
		return finishVirtualRedirectHelper(control, false, fmt.Errorf("open virtual redirect session anchor: %w", err))
	}
	defer windows.CloseHandle(anchor)
	if status, err := windows.WaitForSingleObject(anchor, 0); err != nil || status != uint32(windows.WAIT_TIMEOUT) {
		if err == nil {
			err = fmt.Errorf("virtual redirect session ended before setup completed")
		}
		return finishVirtualRedirectHelper(control, false, err)
	}

	redirect, err := openWinDivertRedirect(opts.Rules)
	if err != nil {
		return finishVirtualRedirectHelper(control, false, err)
	}
	if err := control.Send(virtualRedirectHelperResult{Ready: true}); err != nil {
		redirect.Close()
		redirect.Release()
		return err
	}
	runErr := redirect.Run(anchor)
	return finishVirtualRedirectHelper(control, true, runErr)
}

func finishVirtualRedirectHelper(control *virtualRedirectControlClient, ready bool, actionErr error) error {
	result := virtualRedirectHelperResult{Ready: ready, Stopped: true}
	if actionErr != nil {
		result.Error = actionErr.Error()
	}
	sendErr := control.Send(result)
	return errors.Join(actionErr, sendErr)
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
		route := virtualRedirectPacketRoute{
			Outbound: address.Flags&winDivertFlagOutbound != 0,
			Loopback: address.Flags&winDivertFlagLoopback != 0,
			IfIdx:    binary.LittleEndian.Uint32(address.Data[0:4]),
			SubIfIdx: binary.LittleEndian.Uint32(address.Data[4:8]),
		}
		newRoute, action := rewriteVirtualRedirectPacket(packet[:packetLen], route, &r.table)
		if action == virtualRedirectDrop {
			continue
		}
		if action == virtualRedirectInject {
			if newRoute.Outbound {
				address.Flags |= winDivertFlagOutbound
			} else {
				address.Flags &^= winDivertFlagOutbound
			}
			if newRoute.Loopback {
				address.Flags |= winDivertFlagLoopback
			} else {
				address.Flags &^= winDivertFlagLoopback
			}
			binary.LittleEndian.PutUint32(address.Data[0:4], newRoute.IfIdx)
			binary.LittleEndian.PutUint32(address.Data[4:8], newRoute.SubIfIdx)
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
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate the standard Windows client executable: %w", err)
	}
	return resolveWinDivertDLLForExecutable(exe)
}

func resolveWinDivertDLLForExecutable(exe string) (string, error) {
	dir := filepath.Dir(exe)
	dllPath := filepath.Join(dir, "WinDivert.dll")
	driverPath := filepath.Join(dir, "WinDivert64.sys")
	if err := verifyFileSHA256(dllPath, winDivertDLLSHA256); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("WinDivert.dll is missing; repair the standard 64-bit Windows client installation")
		}
		return "", fmt.Errorf("WinDivert.dll integrity check failed: %w", err)
	}
	if err := verifyFileSHA256(driverPath, winDivertDriverSHA256); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("WinDivert64.sys is missing; repair the standard 64-bit Windows client installation")
		}
		return "", fmt.Errorf("WinDivert64.sys integrity check failed: %w", err)
	}
	return dllPath, nil
}

func verifyFileSHA256(path, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("component is not a regular file")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, strings.TrimSpace(expected)) {
		return fmt.Errorf("SHA-256 mismatch")
	}
	return nil
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

func newVirtualRedirectControlServer(token string) (*virtualRedirectControlServer, error) {
	token = strings.ToLower(strings.TrimSpace(token))
	if !validVirtualRedirectToken(token) {
		return nil, fmt.Errorf("invalid virtual redirect helper control token")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for virtual redirect helper control: %w", err)
	}
	server := &virtualRedirectControlServer{
		listener: listener,
		token:    token,
		results:  make(chan virtualRedirectHelperResult, 4),
		done:     make(chan struct{}),
	}
	go server.run()
	return server, nil
}

func (s *virtualRedirectControlServer) run() {
	defer close(s.done)
	defer close(s.results)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if !s.closing.Load() {
				s.setError(fmt.Errorf("accept virtual redirect helper control: %w", err))
			}
			return
		}
		if !s.authenticate(conn) {
			_ = conn.Close()
			continue
		}
		s.mu.Lock()
		s.conn = conn
		s.mu.Unlock()
		_ = s.listener.Close()
		s.readAuthenticated()
		_ = conn.Close()
		return
	}
}

func (s *virtualRedirectControlServer) authenticate(conn net.Conn) bool {
	_ = conn.SetReadDeadline(time.Now().Add(virtualRedirectHandshakeTimeout))
	decoder := json.NewDecoder(io.LimitReader(conn, virtualRedirectControlMaxBytes))
	var result virtualRedirectHelperResult
	if err := decoder.Decode(&result); err != nil || !result.Connected || result.Token != s.token {
		return false
	}
	_ = conn.SetReadDeadline(time.Time{})
	s.recordResult(result)
	s.decoder = decoder
	return true
}

func (s *virtualRedirectControlServer) readAuthenticated() {
	decoder := s.decoder
	for {
		var result virtualRedirectHelperResult
		err := decoder.Decode(&result)
		if err != nil {
			if !s.closing.Load() && !errors.Is(err, io.EOF) {
				s.setError(fmt.Errorf("read virtual redirect helper control: %w", err))
			} else if !s.closing.Load() && !s.LastResult().Stopped {
				s.setError(fmt.Errorf("virtual redirect helper control connection closed unexpectedly"))
			}
			return
		}
		if result.Token != s.token {
			s.setError(fmt.Errorf("virtual redirect helper control token changed during the session"))
			return
		}
		s.recordResult(result)
		if result.Stopped {
			return
		}
	}
}

func (s *virtualRedirectControlServer) recordResult(result virtualRedirectHelperResult) {
	s.mu.Lock()
	s.last = result
	s.mu.Unlock()
	s.results <- result
}

func (s *virtualRedirectControlServer) setError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	s.err = errors.Join(s.err, err)
	s.mu.Unlock()
}

func (s *virtualRedirectControlServer) Address() string {
	if s == nil || s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

func (s *virtualRedirectControlServer) Results() <-chan virtualRedirectHelperResult {
	return s.results
}

func (s *virtualRedirectControlServer) Done() <-chan struct{} {
	return s.done
}

func (s *virtualRedirectControlServer) LastResult() virtualRedirectHelperResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

func (s *virtualRedirectControlServer) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *virtualRedirectControlServer) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.closing.Store(true)
		if s.listener != nil {
			_ = s.listener.Close()
		}
		s.mu.Lock()
		conn := s.conn
		s.mu.Unlock()
		if conn != nil {
			_ = conn.Close()
		}
	})
	return nil
}

func connectVirtualRedirectControl(address, token string) (*virtualRedirectControlClient, error) {
	address, err := normalizeVirtualRedirectControlAddress(address)
	if err != nil {
		return nil, err
	}
	token = strings.ToLower(strings.TrimSpace(token))
	if !validVirtualRedirectToken(token) {
		return nil, fmt.Errorf("invalid virtual redirect helper control token")
	}
	conn, err := net.DialTimeout("tcp4", address, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connect virtual redirect helper control: %w", err)
	}
	client := &virtualRedirectControlClient{conn: conn, token: token}
	if err := client.Send(virtualRedirectHelperResult{Connected: true}); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return client, nil
}

func (c *virtualRedirectControlClient) Send(result virtualRedirectHelperResult) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("virtual redirect helper control is unavailable")
	}
	result.Token = c.token
	if err := json.NewEncoder(c.conn).Encode(result); err != nil {
		return fmt.Errorf("send virtual redirect helper control result: %w", err)
	}
	return nil
}

func (c *virtualRedirectControlClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func normalizeVirtualRedirectControlAddress(value string) (string, error) {
	host, portText, err := net.SplitHostPort(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("virtual redirect helper control must use loopback IPv4:port")
	}
	ip := net.ParseIP(host)
	port, portErr := strconv.Atoi(portText)
	if portErr != nil || port < 1 || port > 65535 || ip == nil || ip.To4() == nil || !ip.IsLoopback() {
		return "", fmt.Errorf("virtual redirect helper control must use loopback IPv4:port")
	}
	return net.JoinHostPort(ip.To4().String(), strconv.Itoa(port)), nil
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

func virtualRedirectCommandLine(args ...string) string {
	escaped := make([]string, 0, len(args))
	for _, arg := range args {
		escaped = append(escaped, syscall.EscapeArg(arg))
	}
	return strings.Join(escaped, " ")
}
