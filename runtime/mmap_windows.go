//go:build windows

package runtime

import (
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

func nativeAbortSignal() {
	p, err := os.FindProcess(os.Getpid())
	if err == nil {
		_ = p.Kill()
	}
	for {
		runtime.Gosched()
	}
}

func nativeMap(size uintptr) (unsafe.Pointer, []byte) {
	if size == 0 {
		size = 1
	}
	page := uintptr(os.Getpagesize())
	mapped := (size + page - 1) &^ (page - 1)
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	virtualAlloc := kernel32.NewProc("VirtualAlloc")
	const (
		memCommit     = 0x1000
		memReserve    = 0x2000
		pageReadWrite = 0x04
	)
	r1, _, _ := syscall.Syscall6(virtualAlloc.Addr(), 4, 0, mapped, memCommit|memReserve, pageReadWrite, 0, 0)
	if r1 == 0 {
		nativeAbort("native VirtualAlloc failed")
	}
	raw := *(*unsafe.Pointer)(unsafe.Pointer(&r1))
	data := unsafe.Slice((*byte)(raw), int(mapped))
	return raw, data
}

func nativeUnmap(data []byte) {
	if len(data) == 0 {
		return
	}
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	virtualFree := kernel32.NewProc("VirtualFree")
	const memRelease = 0x8000
	_, _, _ = syscall.Syscall(virtualFree.Addr(), 3, uintptr(unsafe.Pointer(&data[0])), 0, memRelease)
}
