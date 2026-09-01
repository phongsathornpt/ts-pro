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
