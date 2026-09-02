package runtime

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

func arrayF64New(length uint64) unsafe.Pointer {
	n := length
	maxUintptr := ^uintptr(0)
	if n > uint64((maxUintptr-nativeF64ArrayHeaderSize)/unsafe.Sizeof(float64(0))) {
		nativeAbort("f64 array allocation overflow")
	}
	size := nativeF64ArrayHeaderSize + uintptr(n)*unsafe.Sizeof(float64(0))
	raw := heapAllocAtomic(size)
	*(*uint64)(raw) = n
	return raw
}

func arrayF64Set(raw unsafe.Pointer, index uint64, value float64) {
	i := index
	if raw == nil || i >= nativeF64ArrayLen(raw) {
		nativeAbort("f64 array index out of bounds")
	}
	*nativeF64ArrayElement(raw, i) = value
}

func arrayF64Len(raw unsafe.Pointer) float64 {
	return float64(nativeF64ArrayLen(raw))
}

func arrayF64Get(raw unsafe.Pointer, index float64) float64 {
	i, ok := validF64ArrayIndex(raw, index)
	if !ok {
		return math.NaN()
	}
	return *nativeF64ArrayElement(raw, i)
}

func arrayF64SetChecked(raw unsafe.Pointer, index float64, value float64) {
	i, ok := validF64ArrayIndex(raw, index)
	if !ok {
		nativeAbort("f64 array index invalid")
	}
	*nativeF64ArrayElement(raw, i) = value
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

func arrayRefNew(length uint64) unsafe.Pointer {
	n := length
	word := unsafe.Sizeof(uintptr(0))
	maxUintptr := ^uintptr(0)
	if n > uint64((maxUintptr-nativeRefArrayHeaderSize)/word) {
		nativeAbort("reference array allocation overflow")
	}
	size := nativeRefArrayHeaderSize + uintptr(n)*word
	if n == 0 {
		raw := heapAllocAtomic(size)
		*(*uint64)(raw) = 0
		return raw
	}
	offsets := make([]uintptr, n)
	for i := range offsets {
		offsets[i] = nativeRefArrayHeaderSize + uintptr(i)*word
	}
	raw := heapAllocRefs(size, unsafe.Pointer(&offsets[0]), uintptr(n))
	*(*uint64)(raw) = n
	return raw
}

func arrayRefSet(raw unsafe.Pointer, index uint64, value unsafe.Pointer) {
	i := index
	if raw == nil || i >= nativeRefArrayLen(raw) {
		nativeAbort("ref array index out of bounds")
	}
	slot := unsafe.Pointer(nativeRefArrayElement(raw, i))
	gcStoreRef(raw, slot, value)
}

func arrayRefLen(raw unsafe.Pointer) float64 {
	return float64(nativeRefArrayLen(raw))
}

func arrayRefGet(raw unsafe.Pointer, index float64) unsafe.Pointer {
	i, ok := validRefArrayIndex(raw, index)
	if !ok {
		return nil
	}
	return *nativeRefArrayElement(raw, i)
}

func arrayRefSetChecked(raw unsafe.Pointer, index float64, value unsafe.Pointer) {
	i, ok := validRefArrayIndex(raw, index)
	if !ok {
		nativeAbort("ref array index invalid")
	}
	slot := unsafe.Pointer(nativeRefArrayElement(raw, i))
	gcStoreRef(raw, slot, value)
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

func arrayBoolNew(length uint64) unsafe.Pointer {
	n := length
	maxUintptr := ^uintptr(0)
	if n > uint64(maxUintptr-nativeBoolArrayHeaderSize) {
		nativeAbort("boolean array allocation overflow")
	}
	raw := heapAllocAtomic(nativeBoolArrayHeaderSize + uintptr(n))
	*(*uint64)(raw) = n
	return raw
}

func arrayBoolSet(raw unsafe.Pointer, index uint64, value uint8) {
	i := index
	if raw == nil || i >= nativeBoolArrayLen(raw) {
		nativeAbort("bool array index out of bounds")
	}
	if value != 0 {
		*nativeBoolArrayElement(raw, i) = 1
	} else {
		*nativeBoolArrayElement(raw, i) = 0
	}
}

func arrayBoolLen(raw unsafe.Pointer) float64 {
	return float64(nativeBoolArrayLen(raw))
}

func arrayBoolGet(raw unsafe.Pointer, index float64) uint8 {
	i, ok := validBoolArrayIndex(raw, index)
	if !ok {
		return 0
	}
	return *nativeBoolArrayElement(raw, i)
}

func arrayBoolSetChecked(raw unsafe.Pointer, index float64, value uint8) {
	i, ok := validBoolArrayIndex(raw, index)
	if !ok {
		nativeAbort("bool array index invalid")
	}
	if value != 0 {
		*nativeBoolArrayElement(raw, i) = 1
	} else {
		*nativeBoolArrayElement(raw, i) = 0
	}
}
