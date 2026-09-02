package runtimego

import (
	"os"
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const (
	initialGCThreshold      = 64 * 1024
	initialMajorGCThreshold = 1024 * 1024
)

type nativeHeapTraceKind uint8

const (
	nativeHeapTraceConservative nativeHeapTraceKind = iota
	nativeHeapTraceAtomic
	nativeHeapTraceOffsets
)

type nativeHeapBlock struct {
	ptr          uintptr
	raw          unsafe.Pointer
	size         uintptr
	data         []byte
	span         *nativeHeapSpan
	slot         uint32
	marked       bool
	generation   nativeHeapGeneration
	nurseryOwner int
	traceKind    nativeHeapTraceKind
	refOffsets   []uintptr
	finalizer    func()
}

var nativeHeap = struct {
	nativeMeasuredMutex
	spans                  map[*nativeHeapSpan]struct{}
	spanPages              map[uintptr]*nativeHeapSpan
	freeSpans              [nativeSizeClassCount][]*nativeHeapSpan
	remoteFrees            uint64
	spanTransfers          uint64
	markWork               uint64
	markPages              uint64
	markQueueSwitches      uint64
	markAssistWorkers      uint64
	markAssistPages        uint64
	markIdleAssistWorkers  uint64
	markIdleAssistPages    uint64
	markTraceWords         uint64
	markAtomicBlocks       uint64
	markPreciseBlocks      uint64
	markConservativeBlocks uint64
}{
	spans:     map[*nativeHeapSpan]struct{}{},
	spanPages: map[uintptr]*nativeHeapSpan{},
}

var nativeHeapWorld nativeMeasuredRWMutex
var nativeHeapBytes atomic.Uint64
var nativeHeapOldBytes atomic.Uint64
var nativeHeapAllocations atomic.Uint64
var nativeHeapCollections atomic.Uint64
var nativeHeapThreshold = func() *atomic.Uint64 {
	value := &atomic.Uint64{}
	value.Store(uint64(initialMajorGCThreshold))
	return value
}()
var nativeGCRequested atomic.Bool
var nativeGCMinorRequested atomic.Bool
var nativeGCMajorRequested atomic.Bool

func nativeAbortSignal() {
	_ = syscall.Kill(syscall.Getpid(), syscall.SIGABRT)
	for {
		runtime.Gosched()
	}
}

func nativeAbort(message string) {
	_, _ = os.Stderr.WriteString("ts-pro: " + message + "\n")
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

func allocateNativeHeapBlock(size uintptr, traceKind nativeHeapTraceKind, refOffsets []uintptr) unsafe.Pointer {
	nativeHeapWorld.RLock()
	owner := nativeAllocatorOwner()
	raw, data, span, slot := allocateNativeHeapStorage(size)
	block := &nativeHeapBlock{
		ptr: uintptr(raw), raw: raw, size: size, data: data, span: span, slot: slot,
		generation: nativeHeapGenerationNursery, nurseryOwner: owner,
		traceKind: traceKind, refOffsets: refOffsets,
	}
	nativeBlocks.set(block.ptr, block)
	nativeNurseryTrack(owner, block)
	nativeHeapBytes.Add(uint64(size))
	nativeHeapAllocations.Add(1)
	if nativeNurseryAdd(owner, size) >= nativeNurseryLimit() {
		nativeGCMinorRequested.Store(true)
		nativeGCRequested.Store(true)
	}
	if nativeHeapOldBytes.Load() >= nativeHeapThreshold.Load() {
		nativeGCMajorRequested.Store(true)
		nativeGCRequested.Store(true)
	}
	nativeHeapWorld.RUnlock()
	return block.raw
}

func heapAlloc(size uintptr) unsafe.Pointer {
	return allocateNativeHeapBlock(size, nativeHeapTraceConservative, nil)
}

func heapAllocAtomic(size uintptr) unsafe.Pointer {
	return allocateNativeHeapBlock(size, nativeHeapTraceAtomic, nil)
}

func heapAllocRefs(size uintptr, offsets unsafe.Pointer, count uintptr) unsafe.Pointer {
	if count == 0 {
		return heapAllocAtomic(size)
	}
	wordSize := uintptr(unsafe.Sizeof(uintptr(0)))
	if offsets == nil || count > size/wordSize {
		nativeAbort("invalid heap reference layout")
	}
	source := unsafe.Slice((*uintptr)(offsets), int(count))
	refs := make([]uintptr, count)
	copy(refs, source)
	for _, offset := range refs {
		if offset%wordSize != 0 || offset > size || size-offset < wordSize {
			nativeAbort("heap reference offset outside allocation")
		}
	}
	return allocateNativeHeapBlock(size, nativeHeapTraceOffsets, refs)
}

func gcCanCollectLocked(tid int) bool {
	return nativeRoots.canCollect(tid)
}

func refreshNativeGCRequested() {
	nativeGCRequested.Store(nativeGCMinorRequested.Load() || nativeGCMajorRequested.Load())
}

func subtractNativeHeapLiveCounters(bytes, allocations uint64) {
	if bytes != 0 {
		nativeHeapBytes.Add(^uint64(bytes - 1))
	}
	if allocations != 0 {
		nativeHeapAllocations.Add(^uint64(allocations - 1))
	}
}

func collectIfSafeLocked(tid int, forceMajor bool) []*nativeHeapBlock {
	if !forceMajor && !nativeGCRequested.Load() {
		return nil
	}
	if !gcCanCollectLocked(tid) {
		return nil
	}
	if forceMajor || nativeGCMajorRequested.Load() {
		blocks := collectMajorLocked()
		nativeGCMinorRequested.Store(false)
		nativeGCMajorRequested.Store(false)
		nativeGCRequested.Store(false)
		return blocks
	}
	if nativeGCMinorRequested.Load() {
		blocks := collectMinorLocked()
		nativeGCMinorRequested.Store(false)
		if nativeHeapOldBytes.Load() >= nativeHeapThreshold.Load() {
			nativeGCMajorRequested.Store(true)
		}
		refreshNativeGCRequested()
		return blocks
	}
	refreshNativeGCRequested()
	return nil
}

func collectMajorLocked() []*nativeHeapBlock {
	nativeBlocks.rangeBlocks(func(_ uintptr, block *nativeHeapBlock) {
		block.marked = false
	})
	markNativeHeapRootsLocked()
	blocks := make([]*nativeHeapBlock, 0)
	var reclaimedBytes uint64
	var reclaimedAllocations uint64
	nativeBlocks.sweep(func(_ uintptr, block *nativeHeapBlock) bool {
		if block.marked {
			block.generation = nativeHeapGenerationOld
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
	subtractNativeHeapLiveCounters(reclaimedBytes, reclaimedAllocations)
	nativeHeapOldBytes.Store(nativeHeapBytes.Load())
	clearNativeNurseryBytes()
	clearNativeNurseryBlocks()
	nativeRemembered.clear()
	nativeHeapCollections.Add(1)
	nativeGCMajorCollections.Add(1)
	next := nativeHeapBytes.Load() * 2
	if next < initialMajorGCThreshold {
		next = initialMajorGCThreshold
	}
	nativeHeapThreshold.Store(next)
	return blocks
}

func collectMinorLocked() []*nativeHeapBlock {
	nursery := takeNativeNurseryBlocks()
	for _, block := range nursery {
		if block != nil && block.generation == nativeHeapGenerationNursery {
			block.marked = false
		}
	}
	nativeGCMinorNurseryScans.Add(uint64(len(nursery)))
	markNativeNurseryRootsLocked(len(nursery))
	blocks := make([]*nativeHeapBlock, 0)
	var reclaimedBytes uint64
	var reclaimedAllocations uint64
	var promoted uint64
	var promotedBytes uint64
	for _, block := range nursery {
		if block == nil || block.generation != nativeHeapGenerationNursery {
			continue
		}
		if block.marked {
			block.generation = nativeHeapGenerationOld
			promoted++
			promotedBytes += uint64(block.size)
			continue
		}
		removed := nativeBlocks.delete(block.ptr)
		if removed != block {
			nativeAbort("nursery block index diverged from live block table")
			continue
		}
		reclaimedBytes += uint64(block.size)
		reclaimedAllocations++
		if block.finalizer != nil {
			blocks = append(blocks, block)
			continue
		}
		if data := releaseNativeHeapBlockStorageLocked(block); len(data) != 0 {
			block.data = data
			blocks = append(blocks, block)
		}
	}
	subtractNativeHeapLiveCounters(reclaimedBytes, reclaimedAllocations)
	clearNativeNurseryBytes()
	nativeRemembered.clear()
	nativeHeapCollections.Add(1)
	nativeGCMinorCollections.Add(1)
	if promoted != 0 {
		nativeGCPromotedBlocks.Add(promoted)
	}
	if promotedBytes != 0 {
		nativeHeapOldBytes.Add(promotedBytes)
		nativeGCPromotedBytes.Add(promotedBytes)
	}
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

func gcCollect() {
	nativeGCMajorRequested.Store(true)
	nativeGCRequested.Store(true)
	tid := nativeCurrentThreadID()
	nativeHeapWorld.Lock()
	nativeHeap.Lock()
	blocks := collectIfSafeLocked(tid, true)
	nativeHeap.Unlock()
	nativeHeapWorld.Unlock()
	finalizeCollectedNativeHeapBlocks(blocks)
}

func gcSafepoint() {
	if !nativeGCRequested.Load() {
		return
	}
	tid := nativeCurrentThreadID()
	nativeHeapWorld.Lock()
	nativeHeap.Lock()
	blocks := collectIfSafeLocked(tid, false)
	nativeHeap.Unlock()
	nativeHeapWorld.Unlock()
	finalizeCollectedNativeHeapBlocks(blocks)
}

func heapShutdown() {
	nativeHeapWorld.Lock()
	nativeHeap.Lock()
	blocks := nativeBlocks.drain()
	nativeHeapBytes.Store(0)
	nativeHeapOldBytes.Store(0)
	nativeHeapAllocations.Store(0)
	nativeHeapThreshold.Store(uint64(initialMajorGCThreshold))
	nativeGCRequested.Store(false)
	nativeGCMinorRequested.Store(false)
	nativeGCMajorRequested.Store(false)
	resetNativeNurseryState()
	nativeHeap.Unlock()
	nativeHeapWorld.Unlock()

	// Finalizers may own persistent GC roots (for example buffered reference
	// channels), so keep root/token tables alive until finalization finishes.
	finalizeShutdownNativeHeapBlocks(blocks)

	nativeHeapWorld.Lock()
	tokenPages := nativeRoots.shutdown()

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
	nativeHeap.markTraceWords = 0
	nativeHeap.markAtomicBlocks = 0
	nativeHeap.markPreciseBlocks = 0
	nativeHeap.markConservativeBlocks = 0
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
	resetNativeGenerationalMetrics()
	resetNativeRememberedState()
	nativeHeap.resetMetrics()
	nativeRoots.resetMetrics()
	nativeHeapWorld.resetMetrics()
	nativeBlocks.resetMetrics()
}

func heapLiveBytes() uintptr {
	return uintptr(nativeHeapBytes.Load())
}

func heapLiveAllocations() uintptr {
	return uintptr(nativeHeapAllocations.Load())
}

func gcCollections() uintptr {
	return uintptr(nativeHeapCollections.Load())
}
