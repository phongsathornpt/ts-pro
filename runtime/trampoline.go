package runtime

import "unsafe"

// callNativeEntry1 is a fallback for legacy function-pointer entries; pure-Go tasks use Go function values.
func callNativeEntry1(entry uintptr, arg0 unsafe.Pointer) {
	nativeAbort("SysV raw function-pointer calls are not supported in pure-Go runtime; use Go function values")
}

// callNativeEntry2 is a fallback for legacy function-pointer entries; pure-Go tasks use Go function values.
func callNativeEntry2(entry uintptr, arg0, arg1 unsafe.Pointer) {
	nativeAbort("SysV raw function-pointer calls are not supported in pure-Go runtime; use Go function values")
}
