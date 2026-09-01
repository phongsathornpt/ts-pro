package main

/*
#include <stdint.h>
*/
import "C"

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

//export tsnative_string_new
func tsnative_string_new(data unsafe.Pointer, length C.uint64_t) unsafe.Pointer {
	n := uintptr(length)
	raw := tsnative_heap_alloc(C.size_t(nativeStringHeaderSize + n))
	*(*uint64)(raw) = uint64(length)
	if n != 0 && data != nil {
		dst := unsafe.Slice((*byte)(unsafe.Add(raw, nativeStringHeaderSize)), int(n))
		src := unsafe.Slice((*byte)(data), int(n))
		copy(dst, src)
	}
	return raw
}

//export tsnative_string_concat
func tsnative_string_concat(left, right unsafe.Pointer) unsafe.Pointer {
	leftBytes := nativeStringBytes(left)
	rightBytes := nativeStringBytes(right)
	raw := tsnative_string_new(nil, C.uint64_t(len(leftBytes)+len(rightBytes)))
	dst := nativeStringBytes(raw)
	copy(dst, leftBytes)
	copy(dst[len(leftBytes):], rightBytes)
	return raw
}

//export tsnative_console_log_string
func tsnative_console_log_string(raw unsafe.Pointer) {
	if data := nativeStringBytes(raw); len(data) != 0 {
		_, _ = os.Stdout.Write(data)
	}
	_, _ = os.Stdout.Write([]byte{'\n'})
}
