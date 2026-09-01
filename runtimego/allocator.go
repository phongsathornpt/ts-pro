package main

import (
	"os"
	"unsafe"
)

const (
	nativeSizeClassCount        = 8
	nativeSmallAllocationMax    = uintptr(2048)
	nativeSpanSize              = uintptr(64 * 1024)
	nativeFreeSpanCachePerClass = 8
)

var nativeSizeClasses = [nativeSizeClassCount]uintptr{16, 32, 64, 128, 256, 512, 1024, 2048}
var nativeAllocatorPageSize = uintptr(os.Getpagesize())

type nativeHeapSpan struct {
	data       []byte
	base       unsafe.Pointer
	classSize  uintptr
	classIndex int
	owner      int
	capacity   uint32
	next       uint32
	free       []uint32
	live       uint32
	listed     bool
}

type nativeWorkerAllocator struct {
	active [nativeSizeClassCount]*nativeHeapSpan
}

func nativeSizeClassIndex(size uintptr) int {
	if size == 0 {
		size = 1
	}
	if size > nativeSmallAllocationMax {
		return -1
	}
	for index, classSize := range nativeSizeClasses {
		if size <= classSize {
			return index
		}
	}
	return -1
}

func nativeAllocatorOwner() int {
	if worker := schedulerWorkerIndex(); worker >= 0 {
		return worker
	}
	return -1
}

func nativeWorkerAllocatorLocked(owner int) *nativeWorkerAllocator {
	allocator := nativeHeap.allocators[owner]
	if allocator == nil {
		allocator = &nativeWorkerAllocator{}
		nativeHeap.allocators[owner] = allocator
	}
	return allocator
}

func newNativeHeapSpanLocked(classIndex, owner int) *nativeHeapSpan {
	base, data := nativeMap(nativeSpanSize)
	classSize := nativeSizeClasses[classIndex]
	span := &nativeHeapSpan{
		data:       data,
		base:       base,
		classSize:  classSize,
		classIndex: classIndex,
		owner:      owner,
		capacity:   uint32(uintptr(len(data)) / classSize),
	}
	nativeHeap.spans[span] = struct{}{}
	registerNativeSpanPagesLocked(span)
	return span
}

func nativeAllocatorPageBase(address uintptr) uintptr {
	return address &^ (nativeAllocatorPageSize - 1)
}

func registerNativeSpanPagesLocked(span *nativeHeapSpan) {
	base := uintptr(span.base)
	for offset := uintptr(0); offset < uintptr(len(span.data)); offset += nativeAllocatorPageSize {
		nativeHeap.spanPages[nativeAllocatorPageBase(base+offset)] = span
	}
}

func unregisterNativeSpanPagesLocked(span *nativeHeapSpan) {
	base := uintptr(span.base)
	for offset := uintptr(0); offset < uintptr(len(span.data)); offset += nativeAllocatorPageSize {
		delete(nativeHeap.spanPages, nativeAllocatorPageBase(base+offset))
	}
}

func nativeHeapSpanForPointerLocked(pointer uintptr) *nativeHeapSpan {
	return nativeHeap.spanPages[nativeAllocatorPageBase(pointer)]
}

func nativeSpanHasSpace(span *nativeHeapSpan) bool {
	return span != nil && (len(span.free) != 0 || span.next < span.capacity)
}

func takeReusableNativeSpanLocked(classIndex, owner int) *nativeHeapSpan {
	spans := nativeHeap.freeSpans[classIndex]
	for len(spans) != 0 {
		last := len(spans) - 1
		span := spans[last]
		spans[last] = nil
		spans = spans[:last]
		nativeHeap.freeSpans[classIndex] = spans
		span.listed = false
		if nativeSpanHasSpace(span) {
			if span.owner != owner {
				nativeHeap.spanTransfers++
			}
			span.owner = owner
			return span
		}
	}
	return nil
}

func allocateNativeSpanSlotLocked(span *nativeHeapSpan) (unsafe.Pointer, uint32) {
	var slot uint32
	if count := len(span.free); count != 0 {
		slot = span.free[count-1]
		span.free[count-1] = 0
		span.free = span.free[:count-1]
	} else {
		slot = span.next
		span.next++
	}
	span.live++
	ptr := unsafe.Add(span.base, uintptr(slot)*span.classSize)
	clear(unsafe.Slice((*byte)(ptr), int(span.classSize)))
	return ptr, slot
}

func allocateNativeHeapStorageLocked(size uintptr) (unsafe.Pointer, []byte, *nativeHeapSpan, uint32) {
	classIndex := nativeSizeClassIndex(size)
	if classIndex < 0 {
		ptr, data := nativeMap(size)
		return ptr, data, nil, 0
	}

	owner := nativeAllocatorOwner()
	allocator := nativeWorkerAllocatorLocked(owner)
	span := allocator.active[classIndex]
	if !nativeSpanHasSpace(span) {
		span = takeReusableNativeSpanLocked(classIndex, owner)
		if span == nil {
			span = newNativeHeapSpanLocked(classIndex, owner)
		}
		allocator.active[classIndex] = span
	}
	ptr, slot := allocateNativeSpanSlotLocked(span)
	return ptr, nil, span, slot
}

func nativeSpanIsActiveLocked(span *nativeHeapSpan) bool {
	allocator := nativeHeap.allocators[span.owner]
	return allocator != nil && allocator.active[span.classIndex] == span
}

func removeReusableNativeSpanLocked(span *nativeHeapSpan) {
	if span == nil || !span.listed {
		return
	}
	spans := nativeHeap.freeSpans[span.classIndex]
	for index, candidate := range spans {
		if candidate != span {
			continue
		}
		copy(spans[index:], spans[index+1:])
		spans[len(spans)-1] = nil
		nativeHeap.freeSpans[span.classIndex] = spans[:len(spans)-1]
		span.listed = false
		return
	}
	span.listed = false
}

// releaseNativeHeapBlockStorageLocked returns an mmap region that must be unmapped
// after nativeHeap is unlocked. A nil result means the slot/span remains cached.
func releaseNativeHeapBlockStorageLocked(block *nativeHeapBlock) []byte {
	if block == nil {
		return nil
	}
	if block.span == nil {
		return block.data
	}

	span := block.span
	if span.live == 0 {
		nativeAbort("native span live count underflow")
		return nil
	}
	span.live--
	if owner := nativeAllocatorOwner(); span.owner >= 0 && owner != span.owner {
		nativeHeap.remoteFrees++
	}
	span.free = append(span.free, block.slot)

	if !nativeSpanIsActiveLocked(span) && !span.listed {
		span.listed = true
		nativeHeap.freeSpans[span.classIndex] = append(nativeHeap.freeSpans[span.classIndex], span)
	}

	cache := nativeHeap.freeSpans[span.classIndex]
	if span.live == 0 && span.listed && len(cache) > nativeFreeSpanCachePerClass {
		removeReusableNativeSpanLocked(span)
		delete(nativeHeap.spans, span)
		unregisterNativeSpanPagesLocked(span)
		data := span.data
		span.data = nil
		span.base = nil
		return data
	}
	return nil
}

func releaseNativeHeapBlockStorage(block *nativeHeapBlock) []byte {
	if block == nil {
		return nil
	}
	nativeHeap.Lock()
	data := releaseNativeHeapBlockStorageLocked(block)
	nativeHeap.Unlock()
	return data
}

func nativeAllocatorRemoteFrees() uint64 {
	nativeHeap.Lock()
	value := nativeHeap.remoteFrees
	nativeHeap.Unlock()
	return value
}

func nativeAllocatorSpanTransfers() uint64 {
	nativeHeap.Lock()
	value := nativeHeap.spanTransfers
	nativeHeap.Unlock()
	return value
}
