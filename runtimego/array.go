package main

/*
#include <stdint.h>
#include <stdlib.h>
*/
import "C"

import (
	"math"
	"unsafe"
)

const nativeF64ArrayHeaderSize = uintptr(8)

func nativeF64ArrayLen(raw unsafe.Pointer) uint64 {
	if raw == nil {
		return 0
	}
	return *(*uint64)(raw)
}

func nativeF64ArrayElement(raw unsafe.Pointer, index uint64) *float64 {
	offset := nativeF64ArrayHeaderSize + uintptr(index)*unsafe.Sizeof(float64(0))
	return (*float64)(unsafe.Add(raw, offset))
}

func validF64ArrayIndex(raw unsafe.Pointer, index float64) (uint64, bool) {
	if raw == nil || math.IsNaN(index) || math.IsInf(index, 0) || index < 0 || math.Trunc(index) != index {
		return 0, false
	}
	if index >= math.Exp2(64) {
		return 0, false
	}
	i := uint64(index)
	return i, i < nativeF64ArrayLen(raw)
}

//export tsnative_array_f64_new
func tsnative_array_f64_new(length C.uint64_t) unsafe.Pointer {
	n := uint64(length)
	maxUintptr := ^uintptr(0)
	if n > uint64((maxUintptr-nativeF64ArrayHeaderSize)/unsafe.Sizeof(float64(0))) {
		nativeAbort("f64 array allocation overflow")
	}
	size := nativeF64ArrayHeaderSize + uintptr(n)*unsafe.Sizeof(float64(0))
	raw := tsnative_heap_alloc(size)
	*(*uint64)(raw) = n
	return raw
}

//export tsnative_array_f64_set
func tsnative_array_f64_set(raw unsafe.Pointer, index C.uint64_t, value C.double) {
	i := uint64(index)
	if raw == nil || i >= nativeF64ArrayLen(raw) {
		C.abort()
	}
	*nativeF64ArrayElement(raw, i) = float64(value)
}

//export tsnative_array_f64_len
func tsnative_array_f64_len(raw unsafe.Pointer) C.double {
	return C.double(float64(nativeF64ArrayLen(raw)))
}

//export tsnative_array_f64_get
func tsnative_array_f64_get(raw unsafe.Pointer, index C.double) C.double {
	i, ok := validF64ArrayIndex(raw, float64(index))
	if !ok {
		return C.double(math.NaN())
	}
	return C.double(*nativeF64ArrayElement(raw, i))
}

//export tsnative_array_f64_set_checked
func tsnative_array_f64_set_checked(raw unsafe.Pointer, index C.double, value C.double) {
	i, ok := validF64ArrayIndex(raw, float64(index))
	if !ok {
		C.abort()
	}
	*nativeF64ArrayElement(raw, i) = float64(value)
}
