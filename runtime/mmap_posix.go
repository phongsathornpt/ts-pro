//go:build !windows

package runtime

import (
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

func nativeAbortSignal() {
	_ = syscall.Kill(syscall.Getpid(), syscall.SIGABRT)
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
	data, err := syscall.Mmap(-1, 0, int(mapped), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_PRIVATE|syscall.MAP_ANON)
	if err != nil || len(data) == 0 {
		nativeAbort("native mmap allocation failed")
	}
	return unsafe.Pointer(&data[0]), data
}

func nativeUnmap(data []byte) {
	if len(data) == 0 {
		return
	}
	if err := syscall.Munmap(data); err != nil {
		nativeAbort("native munmap failed")
	}
}
