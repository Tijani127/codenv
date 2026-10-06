//go:build windows

package ui

import (
	"sync"
	"syscall"
	"unsafe"
)

const enableVirtualTerminalProcessing = 0x0004

var (
	vtOnce      sync.Once
	vtSucceeded bool

	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleMode = kernel32.NewProc("SetConsoleMode")
	procGetConsoleMode = kernel32.NewProc("GetConsoleMode")
)

func enableVirtualTerminal() bool {
	vtOnce.Do(func() {
		if out, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE); err == nil {
			vtSucceeded = enableOn(out)
		}
		if !vtSucceeded {
			return
		}
		if errOut, err := syscall.GetStdHandle(syscall.STD_ERROR_HANDLE); err == nil {
			enableOn(errOut)
		}
	})
	return vtSucceeded
}

func enableOn(handle syscall.Handle) bool {
	if handle == 0 || handle == syscall.InvalidHandle {
		return false
	}
	var mode uint32
	if r, _, _ := procGetConsoleMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&mode))); r == 0 {
		return false
	}
	if mode&enableVirtualTerminalProcessing != 0 {
		return true
	}
	r, _, _ := procSetConsoleMode.Call(uintptr(handle), uintptr(mode|enableVirtualTerminalProcessing))
	return r != 0
}
