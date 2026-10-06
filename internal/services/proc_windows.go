//go:build windows

package services

import (
	"os/exec"
	"syscall"
	"unsafe"
)

const (
	stillActive         = 259
	processTerminate    = 0x0001
	processQueryLimited = 0x1000
)

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess        = kernel32.NewProc("OpenProcess")
	procGetExitCodeProcess = kernel32.NewProc("GetExitCodeProcess")
	procTerminateProcess   = kernel32.NewProc("TerminateProcess")
	procCloseHandle        = kernel32.NewProc("CloseHandle")
)

func detachAttr() *syscall.SysProcAttr {
	const createNewProcessGroup = 0x00000200
	return &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup,
	}
}

func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, _, _ := procOpenProcess.Call(processQueryLimited, 0, uintptr(pid))
	if handle == 0 {
		return false
	}
	defer procCloseHandle.Call(handle)
	var code uint32
	if r, _, _ := procGetExitCodeProcess.Call(handle, uintptr(unsafe.Pointer(&code))); r == 0 {
		return false
	}
	return code == stillActive
}

func kill(pid int) error {
	if pid <= 0 {
		return nil
	}
	handle, _, _ := procOpenProcess.Call(processTerminate, 0, uintptr(pid))
	if handle == 0 {
		return exec.ErrNotFound
	}
	defer procCloseHandle.Call(handle)
	if r, _, _ := procTerminateProcess.Call(handle, 1); r == 0 {
		return exec.ErrNotFound
	}
	return nil
}
