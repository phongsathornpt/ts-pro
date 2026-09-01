package main

/*
#include <stddef.h>
*/
import "C"

import (
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const initialGCThreshold = 64 * 1024

type nativeHeapBlock struct {
	ptr       uintptr
	size      uintptr
	data      []byte
	marked    bool
	finalizer func()
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

var nativeRootToken atomic.Uint64

func nativeAbortSignal() {
	_ = syscall.Kill(syscall.Getpid(), syscall.SIGABRT)
	for {
		runtime.Gosched()
	}
}

func nativeAbort(message string) {
	_, _ = os.Stderr.WriteString("tsnative: " + message + "\n")
	nativeAbortSignal()
}

func nativeMap(size uintptr) (uintptr, []byte) {
	if size == 0 {
		size = 1
	}
	page := uintptr(os.Getpagesize())
	mapped := (size + page - 1) &^ (page - 1)
	data, err := syscall.Mmap(-1, 0, int(mapped), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_PRIVATE|syscall.MAP_ANON)
	if err != nil || len(data) == 0 {
		nativeAbort("native mmap allocation failed")
	}
	return uintptr(unsafe.Pointer(&data[0])), data
}

func nativeUnmap(data []byte) {
	if len(data) == 0 {
		return
	}
	if err := syscall.Munmap(data); err != nil {
		nativeAbort("native munmap failed")
	}
}

func registerNativeHeapFinalizer(raw unsafe.Pointer, finalizer func()) {
	if raw == nil || finalizer == nil {
		return
	}
	nativeHeap.Lock()
	block := nativeHeap.blocks[uintptr(raw)]
	if block == nil {
		nativeHeap.Unlock()
		nativeAbort("native heap finalizer target is not allocated")
		return
	}
	block.finalizer = finalizer
	nativeHeap.Unlock()
}

func allocToken() uintptr {
	return uintptr(nativeRootToken.Add(1))
}

//export tsnative_heap_alloc
func tsnative_heap_alloc(size uintptr) unsafe.Pointer {
	ptr, data := nativeMap(size)
	nativeHeap.Lock()
	nativeHeap.blocks[ptr] = &nativeHeapBlock{ptr: ptr, size: size, data: data}
	nativeHeap.bytes += size
	nativeHeap.allocations++
	nativeHeap.Unlock()
	return unsafe.Pointer(ptr)
}

//export tsnative_gc_enter
func tsnative_gc_enter(slots unsafe.Pointer, count uintptr) unsafe.Pointer {
	token := allocToken()
	tid := syscall.Gettid()
	frame := &nativeRootFrame{token: token, slots: uintptr(slots), count: count, tid: tid}
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

func collectLocked() []*nativeHeapBlock {
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
	blocks := make([]*nativeHeapBlock, 0)
	for key, block := range nativeHeap.blocks {
		if block.marked {
			continue
		}
		delete(nativeHeap.blocks, key)
		nativeHeap.bytes -= block.size
		nativeHeap.allocations--
		blocks = append(blocks, block)
	}
	nativeHeap.collections++
	next := nativeHeap.bytes * 2
	if next < initialGCThreshold {
		next = initialGCThreshold
	}
	nativeHeap.threshold = next
	return blocks
}

func finalizeNativeHeapBlocks(blocks []*nativeHeapBlock) {
	for _, block := range blocks {
		if block.finalizer != nil {
			block.finalizer()
		}
		nativeUnmap(block.data)
	}
}

//export tsnative_gc_collect
func tsnative_gc_collect() {
	var blocks []*nativeHeapBlock
	nativeHeap.Lock()
	if len(nativeHeap.threadStacks) <= 1 && nativeHeap.handoffs == 0 {
		blocks = collectLocked()
	}
	nativeHeap.Unlock()
	finalizeNativeHeapBlocks(blocks)
}

//export tsnative_gc_safepoint
func tsnative_gc_safepoint() {
	var blocks []*nativeHeapBlock
	nativeHeap.Lock()
	if nativeHeap.bytes >= nativeHeap.threshold && len(nativeHeap.threadStacks) <= 1 && nativeHeap.handoffs == 0 {
		blocks = collectLocked()
	}
	nativeHeap.Unlock()
	finalizeNativeHeapBlocks(blocks)
}

//export tsnative_heap_shutdown
func tsnative_heap_shutdown() {
	nativeHeap.Lock()
	blocks := make([]*nativeHeapBlock, 0, len(nativeHeap.blocks))
	for key, block := range nativeHeap.blocks {
		blocks = append(blocks, block)
		delete(nativeHeap.blocks, key)
	}
	nativeHeap.bytes = 0
	nativeHeap.allocations = 0
	nativeHeap.threshold = initialGCThreshold
	nativeHeap.handoffs = 0
	nativeHeap.roots = map[uintptr]*nativeRootFrame{}
	nativeHeap.threadStacks = map[int][]uintptr{}
	nativeHeap.Unlock()
	finalizeNativeHeapBlocks(blocks)
}

//export tsnative_heap_live_bytes
func tsnative_heap_live_bytes() uintptr {
	nativeHeap.Lock()
	value := nativeHeap.bytes
	nativeHeap.Unlock()
	return value
}

//export tsnative_heap_live_allocations
func tsnative_heap_live_allocations() uintptr {
	nativeHeap.Lock()
	value := nativeHeap.allocations
	nativeHeap.Unlock()
	return value
}

//export tsnative_gc_collections
func tsnative_gc_collections() uintptr {
	nativeHeap.Lock()
	value := nativeHeap.collections
	nativeHeap.Unlock()
	return value
}
