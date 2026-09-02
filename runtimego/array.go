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
	raw := tsnative_heap_alloc_atomic(size)
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

const nativeRefArrayHeaderSize = uintptr(8)

func nativeRefArrayLen(raw unsafe.Pointer) uint64 {
	if raw == nil {
		return 0
	}
	return *(*uint64)(raw)
}

func nativeRefArrayElement(raw unsafe.Pointer, index uint64) *unsafe.Pointer {
	offset := nativeRefArrayHeaderSize + uintptr(index)*unsafe.Sizeof(uintptr(0))
	return (*unsafe.Pointer)(unsafe.Add(raw, offset))
}

func validRefArrayIndex(raw unsafe.Pointer, index float64) (uint64, bool) {
	if raw == nil || math.IsNaN(index) || math.IsInf(index, 0) || index < 0 || math.Trunc(index) != index || index >= math.Exp2(64) {
		return 0, false
	}
	i := uint64(index)
	return i, i < nativeRefArrayLen(raw)
}

//export tsnative_array_ref_new
func tsnative_array_ref_new(length C.uint64_t) unsafe.Pointer {
	n := uint64(length)
	word := unsafe.Sizeof(uintptr(0))
	maxUintptr := ^uintptr(0)
	if n > uint64((maxUintptr-nativeRefArrayHeaderSize)/word) {
		nativeAbort("reference array allocation overflow")
	}
	size := nativeRefArrayHeaderSize + uintptr(n)*word
	if n == 0 {
		raw := tsnative_heap_alloc_atomic(size)
		*(*uint64)(raw) = 0
		return raw
	}
	offsets := make([]uintptr, n)
	for i := range offsets {
		offsets[i] = nativeRefArrayHeaderSize + uintptr(i)*word
	}
	raw := tsnative_heap_alloc_refs(size, unsafe.Pointer(&offsets[0]), uintptr(n))
	*(*uint64)(raw) = n
	return raw
}

//export tsnative_array_ref_set
func tsnative_array_ref_set(raw unsafe.Pointer, index C.uint64_t, value unsafe.Pointer) {
	i := uint64(index)
	if raw == nil || i >= nativeRefArrayLen(raw) {
		C.abort()
	}
	slot := unsafe.Pointer(nativeRefArrayElement(raw, i))
	tsnative_gc_store_ref(raw, slot, value)
}

//export tsnative_array_ref_len
func tsnative_array_ref_len(raw unsafe.Pointer) C.double {
	return C.double(float64(nativeRefArrayLen(raw)))
}

//export tsnative_array_ref_get
func tsnative_array_ref_get(raw unsafe.Pointer, index C.double) unsafe.Pointer {
	i, ok := validRefArrayIndex(raw, float64(index))
	if !ok {
		return nil
	}
	return *nativeRefArrayElement(raw, i)
}

//export tsnative_array_ref_set_checked
func tsnative_array_ref_set_checked(raw unsafe.Pointer, index C.double, value unsafe.Pointer) {
	i, ok := validRefArrayIndex(raw, float64(index))
	if !ok {
		C.abort()
	}
	slot := unsafe.Pointer(nativeRefArrayElement(raw, i))
	tsnative_gc_store_ref(raw, slot, value)
}

const nativeBoolArrayHeaderSize = uintptr(8)

func nativeBoolArrayLen(raw unsafe.Pointer) uint64 {
	if raw == nil {
		return 0
	}
	return *(*uint64)(raw)
}

func nativeBoolArrayElement(raw unsafe.Pointer, index uint64) *uint8 {
	return (*uint8)(unsafe.Add(raw, nativeBoolArrayHeaderSize+uintptr(index)))
}

func validBoolArrayIndex(raw unsafe.Pointer, index float64) (uint64, bool) {
	if raw == nil || math.IsNaN(index) || math.IsInf(index, 0) || index < 0 || math.Trunc(index) != index || index >= math.Exp2(64) {
		return 0, false
	}
	i := uint64(index)
	return i, i < nativeBoolArrayLen(raw)
}

//export tsnative_array_bool_new
func tsnative_array_bool_new(length C.uint64_t) unsafe.Pointer {
	n := uint64(length)
	maxUintptr := ^uintptr(0)
	if n > uint64(maxUintptr-nativeBoolArrayHeaderSize) {
		nativeAbort("boolean array allocation overflow")
	}
	raw := tsnative_heap_alloc_atomic(nativeBoolArrayHeaderSize + uintptr(n))
	*(*uint64)(raw) = n
	return raw
}

//export tsnative_array_bool_set
func tsnative_array_bool_set(raw unsafe.Pointer, index C.uint64_t, value C.uint8_t) {
	i := uint64(index)
	if raw == nil || i >= nativeBoolArrayLen(raw) {
		C.abort()
	}
	if value != 0 {
		*nativeBoolArrayElement(raw, i) = 1
	} else {
		*nativeBoolArrayElement(raw, i) = 0
	}
}

//export tsnative_array_bool_len
func tsnative_array_bool_len(raw unsafe.Pointer) C.double {
	return C.double(float64(nativeBoolArrayLen(raw)))
}

//export tsnative_array_bool_get
func tsnative_array_bool_get(raw unsafe.Pointer, index C.double) C.uint8_t {
	i, ok := validBoolArrayIndex(raw, float64(index))
	if !ok {
		return 0
	}
	return C.uint8_t(*nativeBoolArrayElement(raw, i))
}

//export tsnative_array_bool_set_checked
func tsnative_array_bool_set_checked(raw unsafe.Pointer, index C.double, value C.uint8_t) {
	i, ok := validBoolArrayIndex(raw, float64(index))
	if !ok {
		C.abort()
	}
	if value != 0 {
		*nativeBoolArrayElement(raw, i) = 1
	} else {
		*nativeBoolArrayElement(raw, i) = 0
	}
}
