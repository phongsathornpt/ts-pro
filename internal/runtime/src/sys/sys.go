package sys

// Syscall numbers / constants for supported platforms.
type Syscall interface {
	Write(fd int, b []byte) (int, error)
	Exit(code int)
}
