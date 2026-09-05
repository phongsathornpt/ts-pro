//go:build windows

package sys

import (
	"syscall"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procExitProcess  = kernel32.NewProc("ExitProcess")
	procVirtualAlloc = kernel32.NewProc("VirtualAlloc")
	procVirtualFree  = kernel32.NewProc("VirtualFree")
)

const (
	memCommit     = 0x1000
	memReserve    = 0x2000
	memRelease    = 0x8000
	pageReadWrite = 0x04
)

// Write writes bytes directly to the file descriptor via raw syscall.
func Write(fd int, p []byte) (int, error) {
	var done uint32
	err := syscall.WriteFile(syscall.Handle(fd), p, &done, nil)
	return int(done), err
}

// SysExitHook allows intercepting Exit in tests.
var SysExitHook = func(code int) {
	procExitProcess.Call(uintptr(code))
}

// Exit exits the process.
func Exit(code int) {
	SysExitHook(code)
}

// Mmap allocates virtual memory pages via VirtualAlloc.
func Mmap(length uintptr) ([]byte, error) {
	addr, _, err := procVirtualAlloc.Call(0, length, memCommit|memReserve, pageReadWrite)
	if addr == 0 {
		return nil, err
	}
	var sl = struct {
		addr uintptr
		len  int
		cap  int
	}{addr, int(length), int(length)}
	return *(*[]byte)(unsafe.Pointer(&sl)), nil
}

// Munmap releases memory allocated via Mmap.
func Munmap(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	addr := uintptr(unsafe.Pointer(&b[0]))
	r, _, err := procVirtualFree.Call(addr, 0, memRelease)
	if r == 0 {
		return err
	}
	return nil
}
