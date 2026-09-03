//go:build unix

package sys

import (
	"syscall"
)

// Write writes bytes directly to the file descriptor via raw syscall.
func Write(fd int, p []byte) (int, error) {
	return syscall.Write(fd, p)
}

// Exit exits the process directly via syscall.
func Exit(code int) {
	syscall.Exit(code)
}

// Mmap allocates virtual memory pages via raw mmap.
func Mmap(length uintptr) ([]byte, error) {
	return syscall.Mmap(
		-1,
		0,
		int(length),
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE,
	)
}

// Munmap releases memory allocated via Mmap.
func Munmap(b []byte) error {
	return syscall.Munmap(b)
}
