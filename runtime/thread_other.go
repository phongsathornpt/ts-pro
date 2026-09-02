//go:build !linux && !darwin

package runtime

func nativeCurrentThreadID() int {
	return 1
}
