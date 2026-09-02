//go:build darwin

package runtimego

import "syscall"

func nativeCurrentThreadID() int {
	r1, _, err := syscall.Syscall(372, 0, 0, 0)
	if err != 0 || r1 == 0 {
		return syscall.Getpid()
	}
	return int(r1)
}
