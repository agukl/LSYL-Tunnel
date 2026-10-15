//go:build windows && !386

package tunnel

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	winDivertPriority     = 123
	winDivertMTUMax       = 40 + 0xffff
	winDivertFlagOutbound = uint32(1 << 17)
	winDivertFlagLoopback = uint32(1 << 18)
	winDivertDLLSHA256    = "c1e060ee19444a259b2162f8af0f3fe8c4428a1c6f694dce20de194ac8d7d9a2"
	winDivertDriverSHA256 = "8da085332782708d8767bcace5327a6ec7283c17cfb85e40b03cd2323a90ddc2"
)

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
