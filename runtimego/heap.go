package main

/*
#include <stdlib.h>
#include <stdint.h>
*/
import "C"

import (
	"os"
	"sync"
	"syscall"
	"unsafe"
)

const initialGCThreshold = 64 * 1024

type nativeHeapBlock struct {
	ptr    uintptr
	size   uintptr
	marked bool
}

type nativeRootFrame struct {
	token      uintptr
	slots      uintptr
	count      uintptr
	persistent bool
	tid        int
}

var nativeHeap = struct {
	sync.Mutex
	blocks       map[uintptr]*nativeHeapBlock
	roots        map[uintptr]*nativeRootFrame
	threadStacks map[int][]uintptr
	bytes        uintptr
	allocations  uintptr
	collections  uintptr
	threshold    uintptr
	handoffs     uintptr
}{
	blocks:       map[uintptr]*nativeHeapBlock{},
	roots:        map[uintptr]*nativeRootFrame{},
	threadStacks: map[int][]uintptr{},
	threshold:    initialGCThreshold,
}

func nativeAbort(message string) {
	_, _ = os.Stderr.WriteString("tsnative: " + message + "\n")
	C.abort()
}

func allocToken() uintptr {
	ptr := C.malloc(1)
	if ptr == nil {
		nativeAbort("GC root allocation failed")
	}
	return uintptr(ptr)
}

//export tsnative_heap_alloc
func tsnative_heap_alloc(size C.size_t) unsafe.Pointer {
	if size == 0 {
		size = 1
	}
	ptr := C.malloc(size)
	if ptr == nil {
		nativeAbort("heap allocation failed")
	}
	key := uintptr(ptr)
	nativeHeap.Lock()
	nativeHeap.blocks[key] = &nativeHeapBlock{ptr: key, size: uintptr(size)}
	nativeHeap.bytes += uintptr(size)
	nativeHeap.allocations++
	nativeHeap.Unlock()
	return ptr
}

//export tsnative_gc_enter
func tsnative_gc_enter(slots unsafe.Pointer, count C.size_t) unsafe.Pointer {
	token := allocToken()
	tid := syscall.Gettid()
	frame := &nativeRootFrame{token: token, slots: uintptr(slots), count: uintptr(count), tid: tid}
	nativeHeap.Lock()
	nativeHeap.roots[token] = frame
	nativeHeap.threadStacks[tid] = append(nativeHeap.threadStacks[tid], token)
	nativeHeap.Unlock()
	return unsafe.Pointer(token)
}

//export tsnative_gc_leave
func tsnative_gc_leave(raw unsafe.Pointer) {
	token := uintptr(raw)
	tid := syscall.Gettid()
	nativeHeap.Lock()
	frame := nativeHeap.roots[token]
	stack := nativeHeap.threadStacks[tid]
	if frame == nil || frame.persistent || len(stack) == 0 || stack[len(stack)-1] != token {
		nativeHeap.Unlock()
		nativeAbort("invalid GC root frame discipline")
		return
	}
	delete(nativeHeap.roots, token)
	stack = stack[:len(stack)-1]
	if len(stack) == 0 {
		delete(nativeHeap.threadStacks, tid)
	} else {
		nativeHeap.threadStacks[tid] = stack
	}
	nativeHeap.Unlock()
	C.free(raw)
}

//export tsnative_gc_root_register
func tsnative_gc_root_register(slot unsafe.Pointer) unsafe.Pointer {
	if slot == nil {
		return nil
	}
	token := allocToken()
	frame := &nativeRootFrame{token: token, slots: uintptr(slot), count: 1, persistent: true}
	nativeHeap.Lock()
	nativeHeap.roots[token] = frame
	nativeHeap.Unlock()
	return unsafe.Pointer(token)
}

//export tsnative_gc_root_unregister
func tsnative_gc_root_unregister(raw unsafe.Pointer) {
	if raw == nil {
		return
	}
	token := uintptr(raw)
	nativeHeap.Lock()
	frame := nativeHeap.roots[token]
	if frame == nil || !frame.persistent {
		nativeHeap.Unlock()
		nativeAbort("invalid persistent GC root")
		return
	}
	delete(nativeHeap.roots, token)
	nativeHeap.Unlock()
	C.free(raw)
}

//export tsnative_gc_handoff_begin
func tsnative_gc_handoff_begin() {
	nativeHeap.Lock()
	nativeHeap.handoffs++
	nativeHeap.Unlock()
}

//export tsnative_gc_handoff_end
func tsnative_gc_handoff_end() {
	nativeHeap.Lock()
	if nativeHeap.handoffs == 0 {
		nativeHeap.Unlock()
		nativeAbort("invalid GC handoff discipline")
		return
	}
	nativeHeap.handoffs--
	nativeHeap.Unlock()
}
func markCandidateLocked(candidate uintptr) {
	if candidate == 0 {
		return
	}
	block := nativeHeap.blocks[candidate]
	if block == nil || block.marked {
		return
	}
	block.marked = true
	wordSize := uintptr(unsafe.Sizeof(uintptr(0)))
	count := block.size / wordSize
	for i := uintptr(0); i < count; i++ {
		word := *(*uintptr)(unsafe.Pointer(block.ptr + i*wordSize))
		markCandidateLocked(word)
	}
}

func collectLocked() {
	for _, block := range nativeHeap.blocks {
		block.marked = false
	}
	wordSize := uintptr(unsafe.Sizeof(uintptr(0)))
	for _, frame := range nativeHeap.roots {
		for i := uintptr(0); i < frame.count; i++ {
			slotAddr := frame.slots + i*wordSize
			candidate := *(*uintptr)(unsafe.Pointer(slotAddr))
			markCandidateLocked(candidate)
		}
	}
	for key, block := range nativeHeap.blocks {
		if block.marked {
			continue
		}
		C.free(unsafe.Pointer(block.ptr))
		delete(nativeHeap.blocks, key)
		nativeHeap.bytes -= block.size
		nativeHeap.allocations--
	}
	nativeHeap.collections++
	next := nativeHeap.bytes * 2
	if next < initialGCThreshold {
		next = initialGCThreshold
	}
	nativeHeap.threshold = next
}

//export tsnative_gc_collect
func tsnative_gc_collect() {
	nativeHeap.Lock()
	if len(nativeHeap.threadStacks) <= 1 && nativeHeap.handoffs == 0 {
		collectLocked()
	}
	nativeHeap.Unlock()
}

//export tsnative_gc_safepoint
func tsnative_gc_safepoint() {
	nativeHeap.Lock()
	if nativeHeap.bytes >= nativeHeap.threshold && len(nativeHeap.threadStacks) <= 1 && nativeHeap.handoffs == 0 {
		collectLocked()
	}
	nativeHeap.Unlock()
}

//export tsnative_heap_shutdown
func tsnative_heap_shutdown() {
	nativeHeap.Lock()
	for key, block := range nativeHeap.blocks {
		C.free(unsafe.Pointer(block.ptr))
		delete(nativeHeap.blocks, key)
	}
	for token := range nativeHeap.roots {
		C.free(unsafe.Pointer(token))
		delete(nativeHeap.roots, token)
	}
	nativeHeap.threadStacks = map[int][]uintptr{}
	nativeHeap.bytes = 0
	nativeHeap.allocations = 0
	nativeHeap.threshold = initialGCThreshold
	nativeHeap.handoffs = 0
	nativeHeap.Unlock()
}

//export tsnative_heap_live_bytes
func tsnative_heap_live_bytes() C.size_t {
	nativeHeap.Lock()
	value := nativeHeap.bytes
	nativeHeap.Unlock()
	return C.size_t(value)
}

//export tsnative_heap_live_allocations
func tsnative_heap_live_allocations() C.size_t {
	nativeHeap.Lock()
	value := nativeHeap.allocations
	nativeHeap.Unlock()
	return C.size_t(value)
}

//export tsnative_gc_collections
func tsnative_gc_collections() C.size_t {
	nativeHeap.Lock()
	value := nativeHeap.collections
	nativeHeap.Unlock()
	return C.size_t(value)
}
