package main

import (
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestGCDefersForForeignActiveNativeRootStack(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	rooted := tsnative_heap_alloc(16)
	garbage := tsnative_heap_alloc(16)
	slot := uintptr(rooted)
	foreignTID := syscall.Gettid() + 1_000_000

	nativeHeap.Lock()
	tokenPtr := allocRootTokenLocked()
	token := uintptr(tokenPtr)
	nativeHeap.roots[token] = &nativeRootFrame{
		token: tokenPtr, slots: unsafe.Pointer(&slot), count: 1, tid: foreignTID,
	}
	nativeHeap.threadStacks[foreignTID] = []uintptr{token}
	before := nativeHeap.collections
	nativeHeap.Unlock()

	tsnative_gc_collect()

	nativeHeap.Lock()
	after := nativeHeap.collections
	_, garbageLive := nativeHeap.blocks[uintptr(garbage)]
	frame := nativeHeap.roots[token]
	delete(nativeHeap.roots, token)
	delete(nativeHeap.threadStacks, foreignTID)
	freeRootTokenLocked(frame.token)
	nativeHeap.Unlock()

	if after != before {
		t.Fatalf("collection ran with foreign active root stack: before=%d after=%d", before, after)
	}
	if !garbageLive {
		t.Fatal("deferred collection reclaimed memory")
	}
	if !nativeGCRequested.Load() {
		t.Fatal("deferred collection did not retain GC request")
	}

	tsnative_gc_safepoint()
	if nativeHeapContains(rooted) || nativeHeapContains(garbage) {
		t.Fatal("deferred collection did not run after root stack became safe")
	}
	runtime.KeepAlive(&slot)
}

func TestHeapThresholdRequestsDeferredGC(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	raw := tsnative_heap_alloc(initialGCThreshold)
	if raw == nil {
		t.Fatal("threshold allocation failed")
	}
	if !nativeGCRequested.Load() {
		t.Fatal("heap threshold did not request GC")
	}
	tsnative_gc_safepoint()
	if nativeGCRequested.Load() {
		t.Fatal("safe collection left request pending")
	}
	if nativeHeapContains(raw) {
		t.Fatal("unrooted threshold allocation survived safepoint")
	}
}

func TestParkedTaskStateRemainsExplicitlyRooted(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	payload := tsnative_heap_alloc(16)
	state := tsnative_heap_alloc(unsafe.Sizeof(uintptr(0)))
	*(*uintptr)(state) = uintptr(payload)
	task := createNativeTask(1, uintptr(state), nativeTaskResultVoid)
	if task == nil {
		t.Fatal("task allocation failed")
	}
	task.status.Store(nativeTaskWaiting)

	tsnative_gc_collect()
	if !nativeHeapContains(state) || !nativeHeapContains(payload) {
		t.Fatal("parked task state graph was not preserved by explicit task root")
	}

	destroyNativeTaskStorage(task)
	tsnative_gc_collect()
	if nativeHeapContains(state) || nativeHeapContains(payload) {
		t.Fatal("task state graph survived after task root destruction")
	}
}

func TestSleepingTaskKeepsStateGraphRooted(t *testing.T) {
	tsnative_heap_shutdown()
	tsnative_timer_shutdown()
	defer tsnative_heap_shutdown()
	defer tsnative_timer_shutdown()

	payload := tsnative_heap_alloc(16)
	state := tsnative_heap_alloc(unsafe.Sizeof(uintptr(0)))
	*(*uintptr)(state) = uintptr(payload)
	task := createNativeTask(1, uintptr(state), nativeTaskResultVoid)
	if task == nil {
		t.Fatal("task allocation failed")
	}
	task.status.Store(nativeTaskWaiting)
	if waiter := scheduleNativeTimer(time.Hour, task.handle, false); waiter == nil {
		t.Fatal("timer scheduling failed")
	}

	tsnative_gc_collect()
	if !nativeHeapContains(state) || !nativeHeapContains(payload) {
		t.Fatal("sleeping task state graph was not preserved")
	}

	tsnative_timer_shutdown()
	destroyNativeTaskStorage(task)
	tsnative_gc_collect()
	if nativeHeapContains(state) || nativeHeapContains(payload) {
		t.Fatal("sleeping task graph survived after timer/task release")
	}
}

func TestHeapReusesWorkerLocalCachedBlocks(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	schedulerSetThread(0, 0)
	defer schedulerSetThread(-1, 0)

	first := tsnative_heap_alloc(32)
	if first == nil {
		t.Fatal("first allocation failed")
	}
	tsnative_gc_collect()
	second := tsnative_heap_alloc(32)
	if second != first {
		t.Fatalf("worker-local cache did not reuse block: first=%p second=%p", first, second)
	}

	nativeHeap.Lock()
	block := nativeHeap.blocks[uintptr(second)]
	if block == nil || block.span == nil || block.span.owner != 0 {
		t.Fatalf("reused block span = %+v, want worker 0", block)
	}
	nativeHeap.Unlock()
}

func TestHeapShutdownReclaimsCachedBlocks(t *testing.T) {
	tsnative_heap_shutdown()

	schedulerSetThread(0, 0)
	first := tsnative_heap_alloc(32)
	if first == nil {
		t.Fatal("allocation failed")
	}
	tsnative_gc_collect()
	nativeHeap.Lock()
	spanCount := len(nativeHeap.spans)
	allocator := nativeHeap.allocators[0]
	span := allocator.active[1]
	nativeHeap.Unlock()
	if spanCount == 0 || span == nil || span.live != 0 || len(span.free) == 0 {
		t.Fatalf("dead block was not returned to its span: spans=%d span=%+v", spanCount, span)
	}

	tsnative_heap_shutdown()
	schedulerSetThread(-1, 0)
	nativeHeap.Lock()
	remaining := len(nativeHeap.spans)
	for _, spans := range nativeHeap.freeSpans {
		remaining += len(spans)
	}
	nativeHeap.Unlock()
	if remaining != 0 {
		t.Fatalf("cached spans survived heap shutdown: %d", remaining)
	}
}

func TestHeapFinalizerPinsSpanSlotUntilCompletion(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	schedulerSetThread(0, 0)
	defer schedulerSetThread(-1, 0)

	raw := tsnative_heap_alloc(32)
	if raw == nil {
		t.Fatal("allocation failed")
	}
	*(*byte)(raw) = 0x5a
	var replacement unsafe.Pointer
	var observed byte
	registerNativeHeapFinalizer(raw, func() {
		observed = *(*byte)(raw)
		replacement = tsnative_heap_alloc(32)
	})
	tsnative_gc_collect()
	if observed != 0x5a {
		t.Fatalf("finalizer observed overwritten slot: %#x", observed)
	}
	if replacement == raw {
		t.Fatal("finalizer-bearing span slot was reused before finalizer completion")
	}
}

func TestSmallAllocationsShareWorkerSpan(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	schedulerSetThread(0, 0)
	defer schedulerSetThread(-1, 0)

	for index := 0; index < 1000; index++ {
		if raw := tsnative_heap_alloc(32); raw == nil {
			t.Fatalf("allocation %d failed", index)
		}
	}
	nativeHeap.Lock()
	spanCount := len(nativeHeap.spans)
	allocator := nativeHeap.allocators[0]
	span := allocator.active[1]
	nativeHeap.Unlock()
	if spanCount != 1 || span == nil || span.live != 1000 {
		t.Fatalf("small allocation density spans=%d span=%+v", spanCount, span)
	}
}

func TestWorkerAllocatorsUseDistinctActiveSpans(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	schedulerSetThread(0, 0)
	_ = tsnative_heap_alloc(32)
	schedulerSetThread(1, 0)
	_ = tsnative_heap_alloc(32)
	schedulerSetThread(-1, 0)

	nativeHeap.Lock()
	worker0 := nativeHeap.allocators[0]
	worker1 := nativeHeap.allocators[1]
	span0, span1 := worker0.active[1], worker1.active[1]
	nativeHeap.Unlock()
	if span0 == nil || span1 == nil || span0 == span1 || span0.owner != 0 || span1.owner != 1 {
		t.Fatalf("worker spans not isolated: worker0=%+v worker1=%+v", span0, span1)
	}
}
