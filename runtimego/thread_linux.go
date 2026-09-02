//go:build linux

package runtimego

import "syscall"

func nativeCurrentThreadID() int {
	return syscall.Gettid()
}
