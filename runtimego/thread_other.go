//go:build !linux && !darwin

package runtimego

func nativeCurrentThreadID() int {
	return 1
}
