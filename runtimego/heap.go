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
	raw       unsafe.Pointer
	size      uintptr
	data      []byte
	span      *nativeHeapSpan
	slot      uint32
	marked    bool
	finalizer func()
}

type nativeRootFrame struct {
	token      unsafe.Pointer
	slots      unsafe.Pointer
	count      uintptr
	persistent bool
	tid        int
}

var nativeRoots = struct {
	nativeMeasuredMutex
	roots        map[uintptr]*nativeRootFrame
	threadStacks map[int][]uintptr
	tokenPages   [][]byte
	tokenFree    []unsafe.Pointer
	handoffs     uintptr
}{
	roots:        map[uintptr]*nativeRootFrame{},
	threadStacks: map[int][]uintptr{},
}

var nativeHeap = struct {
	nativeMeasuredMutex
	spans                 map[*nativeHeapSpan]struct{}
	spanPages             map[uintptr]*nativeHeapSpan
	freeSpans             [nativeSizeClassCount][]*nativeHeapSpan
	remoteFrees           uint64
	spanTransfers         uint64
	markWork              uint64
	markPages             uint64
	markQueueSwitches     uint64
	markAssistWorkers     uint64
	markAssistPages       uint64
	markIdleAssistWorkers uint64
	markIdleAssistPages   uint64
}{
	spans:     map[*nativeHeapSpan]struct{}{},
	spanPages: map[uintptr]*nativeHeapSpan{},
}

var nativeHeapWorld sync.RWMutex
var nativeHeapBytes atomic.Uint64
var nativeHeapAllocations atomic.Uint64
var nativeHeapCollections atomic.Uint64
var nativeHeapThreshold = func() *atomic.Uint64 {
	value := &atomic.Uint64{}
	value.Store(uint64(initialGCThreshold))
	return value
}()
var nativeGCRequested atomic.Bool

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

func nativeMap(size uintptr) (unsafe.Pointer, []byte) {
	if size == 0 {
		size = 1
	}
	page := uintptr(os.Getpagesize())
	mapped := (size + page - 1) &^ (page - 1)
	data, err := syscall.Mmap(-1, 0, int(mapped), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_PRIVATE|syscall.MAP_ANON)
	if err != nil || len(data) == 0 {
		nativeAbort("native mmap allocation failed")
	}
	return unsafe.Pointer(&data[0]), data
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
	nativeHeapWorld.RLock()
	ok := nativeBlocks.update(uintptr(raw), func(block *nativeHeapBlock) {
		block.finalizer = finalizer
	})
	nativeHeapWorld.RUnlock()
	if !ok {
		nativeAbort("native heap finalizer target is not allocated")
	}
}

const nativeRootTokenSize = uintptr(unsafe.Sizeof(uintptr(0)))

func allocRootTokenLocked() unsafe.Pointer {
	if count := len(nativeRoots.tokenFree); count != 0 {
		token := nativeRoots.tokenFree[count-1]
		nativeRoots.tokenFree[count-1] = nil
		nativeRoots.tokenFree = nativeRoots.tokenFree[:count-1]
		return token
	}
	_, page := nativeMap(uintptr(os.Getpagesize()))
	base := unsafe.Pointer(&page[0])
	for offset := nativeRootTokenSize; offset+nativeRootTokenSize <= uintptr(len(page)); offset += nativeRootTokenSize {
		nativeRoots.tokenFree = append(nativeRoots.tokenFree, unsafe.Add(base, offset))
	}
	nativeRoots.tokenPages = append(nativeRoots.tokenPages, page)
	return base
}

func freeRootTokenLocked(token unsafe.Pointer) {
	if token != nil {
		nativeRoots.tokenFree = append(nativeRoots.tokenFree, token)
	}
}

//export tsnative_heap_alloc
func tsnative_heap_alloc(size uintptr) unsafe.Pointer {
	nativeHeapWorld.RLock()
	raw, data, span, slot := allocateNativeHeapStorage(size)
	block := &nativeHeapBlock{ptr: uintptr(raw), raw: raw, size: size, data: data, span: span, slot: slot}
	nativeBlocks.set(block.ptr, block)
	liveBytes := nativeHeapBytes.Add(uint64(size))
	nativeHeapAllocations.Add(1)
	if liveBytes >= nativeHeapThreshold.Load() {
		nativeGCRequested.Store(true)
	}
	nativeHeapWorld.RUnlock()
	return block.raw
}

//export tsnative_gc_enter
func tsnative_gc_enter(slots unsafe.Pointer, count uintptr) unsafe.Pointer {
	tid := syscall.Gettid()
	nativeRoots.Lock()
	token := allocRootTokenLocked()
	key := uintptr(token)
	frame := &nativeRootFrame{token: token, slots: slots, count: count, tid: tid}
	nativeRoots.roots[key] = frame
	nativeRoots.threadStacks[tid] = append(nativeRoots.threadStacks[tid], key)
	nativeRoots.Unlock()
	return token
}

//export tsnative_gc_leave
func tsnative_gc_leave(raw unsafe.Pointer) {
	token := uintptr(raw)
	tid := syscall.Gettid()
	nativeRoots.Lock()
	frame := nativeRoots.roots[token]
	stack := nativeRoots.threadStacks[tid]
	if frame == nil || frame.persistent || len(stack) == 0 || stack[len(stack)-1] != token {
		nativeRoots.Unlock()
		nativeAbort("invalid GC root frame discipline")
		return
	}
	delete(nativeRoots.roots, token)
	freeRootTokenLocked(frame.token)
	stack = stack[:len(stack)-1]
	if len(stack) == 0 {
		delete(nativeRoots.threadStacks, tid)
	} else {
		nativeRoots.threadStacks[tid] = stack
	}
	nativeRoots.Unlock()
}

//export tsnative_gc_root_register
func tsnative_gc_root_register(slot unsafe.Pointer) unsafe.Pointer {
	if slot == nil {
		return nil
	}
	nativeRoots.Lock()
	token := allocRootTokenLocked()
	nativeRoots.roots[uintptr(token)] = &nativeRootFrame{token: token, slots: slot, count: 1, persistent: true}
	nativeRoots.Unlock()
	return token
}

//export tsnative_gc_root_unregister
func tsnative_gc_root_unregister(raw unsafe.Pointer) {
	if raw == nil {
		return
	}
	token := uintptr(raw)
	nativeRoots.Lock()
	frame := nativeRoots.roots[token]
	if frame == nil || !frame.persistent {
		nativeRoots.Unlock()
		nativeAbort("invalid persistent GC root")
		return
	}
	delete(nativeRoots.roots, token)
	freeRootTokenLocked(frame.token)
	nativeRoots.Unlock()
}

//export tsnative_gc_handoff_begin
func tsnative_gc_handoff_begin() {
	nativeRoots.Lock()
	nativeRoots.handoffs++
	nativeRoots.Unlock()
}

//export tsnative_gc_handoff_end
func tsnative_gc_handoff_end() {
	nativeRoots.Lock()
	if nativeRoots.handoffs == 0 {
		nativeRoots.Unlock()
		nativeAbort("invalid GC handoff discipline")
		return
	}
	nativeRoots.handoffs--
	nativeRoots.Unlock()
}

func gcCanCollectLocked(tid int) bool {
	if nativeRoots.handoffs != 0 {
		return false
	}
	if len(nativeRoots.threadStacks) == 0 {
		return true
	}
	if len(nativeRoots.threadStacks) != 1 {
		return false
	}
	_, ownsActiveStack := nativeRoots.threadStacks[tid]
	return ownsActiveStack
}

func collectIfSafeLocked(tid int, force bool) []*nativeHeapBlock {
	if !force && !nativeGCRequested.Load() {
		return nil
	}
	if !gcCanCollectLocked(tid) {
		return nil
	}
	blocks := collectLocked()
	nativeGCRequested.Store(false)
	return blocks
}

func collectLocked() []*nativeHeapBlock {
	nativeBlocks.rangeBlocks(func(_ uintptr, block *nativeHeapBlock) {
		block.marked = false
	})
	markNativeHeapRootsLocked()
	blocks := make([]*nativeHeapBlock, 0)
	var reclaimedBytes uint64
	var reclaimedAllocations uint64
	nativeBlocks.sweep(func(_ uintptr, block *nativeHeapBlock) bool {
		if block.marked {
			return false
		}
		reclaimedBytes += uint64(block.size)
		reclaimedAllocations++
		if block.finalizer != nil {
			blocks = append(blocks, block)
			return true
		}
		if data := releaseNativeHeapBlockStorageLocked(block); len(data) != 0 {
			block.data = data
			blocks = append(blocks, block)
		}
		return true
	})
	if reclaimedBytes != 0 {
		nativeHeapBytes.Add(^uint64(reclaimedBytes - 1))
	}
	if reclaimedAllocations != 0 {
		nativeHeapAllocations.Add(^uint64(reclaimedAllocations - 1))
	}
	nativeHeapCollections.Add(1)
	next := nativeHeapBytes.Load() * 2
	if next < initialGCThreshold {
		next = initialGCThreshold
	}
	nativeHeapThreshold.Store(next)
	return blocks
}

func finalizeCollectedNativeHeapBlocks(blocks []*nativeHeapBlock) {
	for _, block := range blocks {
		if block.finalizer != nil {
			block.finalizer()
			if data := releaseNativeHeapBlockStorage(block); len(data) != 0 {
				nativeUnmap(data)
			}
			continue
		}
		nativeUnmap(block.data)
	}
}

func finalizeShutdownNativeHeapBlocks(blocks []*nativeHeapBlock) {
	for _, block := range blocks {
		if block.finalizer != nil {
			block.finalizer()
		}
		if block.span == nil {
			nativeUnmap(block.data)
		}
	}
}

//export tsnative_gc_collect
func tsnative_gc_collect() {
	nativeGCRequested.Store(true)
	tid := syscall.Gettid()
	nativeHeapWorld.Lock()
	nativeHeap.Lock()
	nativeRoots.Lock()
	blocks := collectIfSafeLocked(tid, true)
	nativeRoots.Unlock()
	nativeHeap.Unlock()
	nativeHeapWorld.Unlock()
	finalizeCollectedNativeHeapBlocks(blocks)
}

//export tsnative_gc_safepoint
func tsnative_gc_safepoint() {
	if !nativeGCRequested.Load() {
		return
	}
	tid := syscall.Gettid()
	nativeHeapWorld.Lock()
	nativeHeap.Lock()
	nativeRoots.Lock()
	blocks := collectIfSafeLocked(tid, false)
	nativeRoots.Unlock()
	nativeHeap.Unlock()
	nativeHeapWorld.Unlock()
	finalizeCollectedNativeHeapBlocks(blocks)
}

//export tsnative_heap_shutdown
func tsnative_heap_shutdown() {
	nativeHeapWorld.Lock()
	nativeHeap.Lock()
	blocks := nativeBlocks.drain()
	nativeHeapBytes.Store(0)
	nativeHeapAllocations.Store(0)
	nativeHeapThreshold.Store(uint64(initialGCThreshold))
	nativeGCRequested.Store(false)
	nativeHeap.Unlock()
	nativeHeapWorld.Unlock()

	// Finalizers may own persistent GC roots (for example buffered reference
	// channels), so keep root/token tables alive until finalization finishes.
	finalizeShutdownNativeHeapBlocks(blocks)

	nativeHeapWorld.Lock()
	nativeRoots.Lock()
	nativeRoots.roots = map[uintptr]*nativeRootFrame{}
	nativeRoots.threadStacks = map[int][]uintptr{}
	nativeRoots.handoffs = 0
	tokenPages := nativeRoots.tokenPages
	nativeRoots.tokenPages = nil
	nativeRoots.tokenFree = nil
	nativeRoots.Unlock()

	nativeHeap.Lock()
	resetNativeWorkerAllocatorsLocked()
	nativeHeap.remoteFrees = 0
	nativeHeap.spanTransfers = 0
	nativeHeap.markWork = 0
	nativeHeap.markPages = 0
	nativeHeap.markQueueSwitches = 0
	nativeHeap.markAssistWorkers = 0
	nativeHeap.markAssistPages = 0
	nativeHeap.markIdleAssistWorkers = 0
	nativeHeap.markIdleAssistPages = 0
	spans := make([]*nativeHeapSpan, 0, len(nativeHeap.spans))
	for span := range nativeHeap.spans {
		spans = append(spans, span)
	}
	nativeHeap.spans = map[*nativeHeapSpan]struct{}{}
	nativeHeap.spanPages = map[uintptr]*nativeHeapSpan{}
	nativeHeap.freeSpans = [nativeSizeClassCount][]*nativeHeapSpan{}
	nativeHeap.Unlock()
	nativeHeapWorld.Unlock()

	for _, span := range spans {
		nativeUnmap(span.data)
	}
	for _, page := range tokenPages {
		nativeUnmap(page)
	}
	nativeHeap.resetMetrics()
	nativeRoots.resetMetrics()
}

//export tsnative_heap_live_bytes
func tsnative_heap_live_bytes() uintptr {
	return uintptr(nativeHeapBytes.Load())
}

//export tsnative_heap_live_allocations
func tsnative_heap_live_allocations() uintptr {
	return uintptr(nativeHeapAllocations.Load())
}

//export tsnative_gc_collections
func tsnative_gc_collections() uintptr {
	return uintptr(nativeHeapCollections.Load())
}
