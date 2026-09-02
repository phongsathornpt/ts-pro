//go:build linux

package runtime

import "syscall"

func nativeCurrentThreadID() int {
	return syscall.Gettid()
}
