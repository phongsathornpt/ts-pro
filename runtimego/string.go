package runtimego

import (
	"os"
	"unsafe"
)

const nativeStringHeaderSize = uintptr(8)

func nativeStringLen(raw unsafe.Pointer) uint64 {
	if raw == nil {
		return 0
	}
	return *(*uint64)(raw)
}

func nativeStringBytes(raw unsafe.Pointer) []byte {
	length := nativeStringLen(raw)
	if length == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Add(raw, nativeStringHeaderSize)), int(length))
}

func tsnative_string_new(data unsafe.Pointer, length uint64) unsafe.Pointer {
	n := uintptr(length)
	raw := tsnative_heap_alloc_atomic(nativeStringHeaderSize + n)
	*(*uint64)(raw) = length
	if n != 0 && data != nil {
		dst := unsafe.Slice((*byte)(unsafe.Add(raw, nativeStringHeaderSize)), int(n))
		src := unsafe.Slice((*byte)(data), int(n))
		copy(dst, src)
	}
	return raw
}

func tsnative_string_concat(left, right unsafe.Pointer) unsafe.Pointer {
	leftBytes := nativeStringBytes(left)
	rightBytes := nativeStringBytes(right)
	raw := tsnative_string_new(nil, uint64(len(leftBytes)+len(rightBytes)))
	dst := nativeStringBytes(raw)
	copy(dst, leftBytes)
	copy(dst[len(leftBytes):], rightBytes)
	return raw
}

func tsnative_console_log_string(raw unsafe.Pointer) {
	if data := nativeStringBytes(raw); len(data) != 0 {
		_, _ = os.Stdout.Write(data)
	}
	_, _ = os.Stdout.Write([]byte{'\n'})
}
