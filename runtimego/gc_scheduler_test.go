package main

import (
	"runtime"
	"sync"
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
	nativeRoots.Lock()
	tokenPtr := allocRootTokenLocked()
	token := uintptr(tokenPtr)
	nativeRoots.roots[token] = &nativeRootFrame{
		token: tokenPtr, slots: unsafe.Pointer(&slot), count: 1, tid: foreignTID,
	}
	nativeRoots.threadStacks[foreignTID] = []uintptr{token}
	before := nativeHeapCollections.Load()
	nativeRoots.Unlock()
	nativeHeap.Unlock()

	tsnative_gc_collect()

	nativeHeap.Lock()
	nativeRoots.Lock()
	after := nativeHeapCollections.Load()
	garbageLive := nativeBlocks.get(uintptr(garbage)) != nil
	frame := nativeRoots.roots[token]
	delete(nativeRoots.roots, token)
	delete(nativeRoots.threadStacks, foreignTID)
	freeRootTokenLocked(frame.token)
	nativeRoots.Unlock()
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

func TestHeapAndRootLockMetricsRecordContention(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	nativeHeap.resetMetrics()
	nativeHeap.Lock()
	heapStarted := make(chan struct{})
	heapDone := make(chan struct{})
	go func() {
		close(heapStarted)
		nativeHeap.Lock()
		nativeHeap.Unlock()
		close(heapDone)
	}()
	<-heapStarted
	heapDeadline := time.Now().Add(250 * time.Millisecond)
	for nativeHeapLockMetrics().contended == 0 && time.Now().Before(heapDeadline) {
		runtime.Gosched()
	}
	if nativeHeapLockMetrics().contended == 0 {
		nativeHeap.Unlock()
		t.Fatal("heap lock contention was not observed")
	}
	nativeHeap.Unlock()
	<-heapDone
	heapMetrics := nativeHeapLockMetrics()
	if heapMetrics.acquisitions < 2 || heapMetrics.waitNanos == 0 {
		t.Fatalf("heap lock metrics = %+v, want acquisitions >= 2 and wait > 0", heapMetrics)
	}

	nativeRoots.resetMetrics()
	nativeRoots.Lock()
	rootStarted := make(chan struct{})
	rootDone := make(chan struct{})
	go func() {
		close(rootStarted)
		nativeRoots.Lock()
		nativeRoots.Unlock()
		close(rootDone)
	}()
	<-rootStarted
	rootDeadline := time.Now().Add(250 * time.Millisecond)
	for nativeRootLockMetrics().contended == 0 && time.Now().Before(rootDeadline) {
		runtime.Gosched()
	}
	if nativeRootLockMetrics().contended == 0 {
		nativeRoots.Unlock()
		t.Fatal("root lock contention was not observed")
	}
	nativeRoots.Unlock()
	<-rootDone
	rootMetrics := nativeRootLockMetrics()
	if rootMetrics.acquisitions < 2 || rootMetrics.waitNanos == 0 {
		t.Fatalf("root lock metrics = %+v, want acquisitions >= 2 and wait > 0", rootMetrics)
	}
}

func TestWorkerAllocationFastPathAvoidsGlobalHeapLock(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	schedulerSetThread(0, 0)
	defer schedulerSetThread(-1, 0)

	if raw := tsnative_heap_alloc(32); raw == nil {
		t.Fatal("warm allocation failed")
	}
	nativeHeap.resetMetrics()
	for i := 0; i < 1000; i++ {
		if raw := tsnative_heap_alloc(32); raw == nil {
			t.Fatalf("fast-path allocation %d failed", i)
		}
	}
	if metrics := nativeHeapLockMetrics(); metrics.acquisitions != 0 {
		t.Fatalf("warm worker fast path used global heap lock: %+v", metrics)
	}
}

func TestConcurrentWorkerAllocationFastPathReducesGlobalLockTraffic(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()
	nativeHeap.resetMetrics()
	nativeRoots.resetMetrics()

	const workers = 8
	const allocationsPerWorker = 5000
	var wg sync.WaitGroup
	wg.Add(workers)
	for owner := 0; owner < workers; owner++ {
		go func(owner int) {
			defer wg.Done()
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			schedulerSetThread(owner, 0)
			defer schedulerSetThread(-1, 0)
			for i := 0; i < allocationsPerWorker; i++ {
				raw := tsnative_heap_alloc(32)
				token := tsnative_gc_root_register(unsafe.Pointer(&raw))
				tsnative_gc_root_unregister(token)
			}
		}(owner)
	}
	wg.Wait()

	heapMetrics := nativeHeapLockMetrics()
	rootMetrics := nativeRootLockMetrics()
	t.Logf("heap lock acquisitions=%d contentions=%d wait_ns=%d", heapMetrics.acquisitions, heapMetrics.contended, heapMetrics.waitNanos)
	t.Logf("root lock acquisitions=%d contentions=%d wait_ns=%d", rootMetrics.acquisitions, rootMetrics.contended, rootMetrics.waitNanos)
	if heapMetrics.acquisitions >= 128 {
		t.Fatalf("global heap lock acquisitions = %d, want < 128 for %d allocations", heapMetrics.acquisitions, workers*allocationsPerWorker)
	}
}

func TestRootMetadataAndHeapAllocationUseIndependentLocks(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	nativeRoots.Lock()
	allocationDone := make(chan unsafe.Pointer, 1)
	go func() {
		allocationDone <- tsnative_heap_alloc(16)
	}()
	var allocated unsafe.Pointer
	select {
	case allocated = <-allocationDone:
	case <-time.After(250 * time.Millisecond):
		nativeRoots.Unlock()
		t.Fatal("heap allocation blocked on root metadata lock")
	}
	nativeRoots.Unlock()
	if allocated == nil {
		t.Fatal("heap allocation failed while root lock was held")
	}

	slot := new(unsafe.Pointer)
	nativeHeap.Lock()
	rootDone := make(chan unsafe.Pointer, 1)
	go func() {
		rootDone <- tsnative_gc_root_register(unsafe.Pointer(slot))
	}()
	var token unsafe.Pointer
	select {
	case token = <-rootDone:
	case <-time.After(250 * time.Millisecond):
		nativeHeap.Unlock()
		t.Fatal("root registration blocked on heap allocation lock")
	}
	nativeHeap.Unlock()
	if token == nil {
		t.Fatal("root registration failed while heap lock was held")
	}
	tsnative_gc_root_unregister(token)
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
	var entry byte
	task := createNativeTask(unsafe.Pointer(&entry), state, nativeTaskResultVoid)
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
	var entry byte
	task := createNativeTask(unsafe.Pointer(&entry), state, nativeTaskResultVoid)
	if task == nil {
		t.Fatal("task allocation failed")
	}
	task.status.Store(nativeTaskWaiting)
	if waiter := scheduleNativeTimer(time.Hour, nativeTaskKey(task), false); waiter == nil {
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

	block := nativeBlocks.get(uintptr(second))
	if block == nil || block.span == nil || block.span.owner != 0 {
		t.Fatalf("reused block span = %+v, want worker 0", block)
	}
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
	allocator := nativeAllocatorForOwner(0)
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
	allocator := nativeAllocatorForOwner(0)
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
	worker0 := nativeAllocatorForOwner(0)
	worker1 := nativeAllocatorForOwner(1)
	span0, span1 := worker0.active[1], worker1.active[1]
	nativeHeap.Unlock()
	if span0 == nil || span1 == nil || span0 == span1 || span0.owner != 0 || span1.owner != 1 {
		t.Fatalf("worker spans not isolated: worker0=%+v worker1=%+v", span0, span1)
	}
}

func TestHeapShutdownFinalizesBufferedReferenceChannelRoots(t *testing.T) {
	tsnative_heap_shutdown()

	channel := newNativeRefChannel(1)
	payload := tsnative_heap_alloc(16)
	if tsnative_channel_ref_try_send(channel, payload) != 1 {
		t.Fatal("reference channel send failed")
	}

	tsnative_heap_shutdown()
	if lookupNativeRefChannel(channel) != nil {
		t.Fatal("reference channel state survived heap shutdown")
	}
	nativeRoots.Lock()
	rootCount := len(nativeRoots.roots)
	nativeRoots.Unlock()
	nativeHeap.Lock()
	spanCount := len(nativeHeap.spans)
	nativeHeap.Unlock()
	if rootCount != 0 || spanCount != 0 {
		t.Fatalf("heap shutdown leaked roots/spans: roots=%d spans=%d", rootCount, spanCount)
	}
}

func TestAllocatorPageMetadataAndRemoteSpanReuse(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	schedulerSetThread(0, 0)
	for index := uint32(0); index <= uint32(nativeSpanSize/2048); index++ {
		if raw := tsnative_heap_alloc(2048); raw == nil {
			t.Fatalf("allocation %d failed", index)
		}
	}

	nativeHeap.Lock()
	var pageSpan *nativeHeapSpan
	for span := range nativeHeap.spans {
		if span.classIndex == nativeSizeClassCount-1 && span.owner == 0 && span.next == span.capacity {
			pageSpan = span
			break
		}
	}
	if pageSpan == nil {
		nativeHeap.Unlock()
		t.Fatal("worker 0 did not create a 2 KiB span")
	}
	if got := nativeHeapSpanForPointerLocked(uintptr(pageSpan.base)); got != pageSpan {
		nativeHeap.Unlock()
		t.Fatal("span page metadata did not resolve its base pointer")
	}
	transfersBefore := nativeHeap.spanTransfers
	remoteFreesBefore := nativeHeap.remoteFrees
	nativeHeap.Unlock()

	schedulerSetThread(1, 0)
	tsnative_gc_collect()
	nativeHeap.Lock()
	if pageSpan.owner != 0 || nativeHeap.spanTransfers != transfersBefore {
		nativeHeap.Unlock()
		t.Fatal("span transferred before owner drained remote frees")
	}
	nativeHeap.Unlock()
	schedulerSetThread(0, 0)
	_ = tsnative_heap_alloc(2048)
	schedulerSetThread(1, 0)
	_ = tsnative_heap_alloc(2048)

	nativeHeap.Lock()
	transfersAfter := nativeHeap.spanTransfers
	remoteFreesAfter := nativeHeap.remoteFrees
	owner := pageSpan.owner
	nativeHeap.Unlock()
	if remoteFreesAfter <= remoteFreesBefore {
		t.Fatalf("remote span reclamation was not counted: before=%d after=%d", remoteFreesBefore, remoteFreesAfter)
	}
	if transfersAfter <= transfersBefore || owner != 1 {
		t.Fatalf("span reuse was not transferred to worker 1: before=%d after=%d owner=%d", transfersBefore, transfersAfter, owner)
	}
}

func TestHeapPageMetadataKeepsInteriorPointerAlive(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	raw := tsnative_heap_alloc(32)
	if raw == nil {
		t.Fatal("allocation failed")
	}
	interior := unsafe.Add(raw, 7)
	root := tsnative_gc_root_register(unsafe.Pointer(&interior))
	if root == nil {
		t.Fatal("interior root registration failed")
	}
	tsnative_gc_collect()
	if !nativeHeapContains(raw) {
		t.Fatal("page metadata failed to resolve interior pointer")
	}
	tsnative_gc_root_unregister(root)
	tsnative_gc_collect()
	if nativeHeapContains(raw) {
		t.Fatal("allocation survived after interior root release")
	}
}

func TestAllocatorAccountsRemoteFreeAndSpanTransfer(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	schedulerSetThread(0, 0)
	classIndex := nativeSizeClassIndex(32)
	capacity := int(nativeSpanSize / nativeSizeClasses[classIndex])
	for i := 0; i <= capacity; i++ {
		if tsnative_heap_alloc(32) == nil {
			t.Fatalf("allocation %d failed", i)
		}
	}

	schedulerSetThread(1, 0)
	tsnative_gc_collect()
	if got := nativeAllocatorRemoteFrees(); got == 0 {
		t.Fatal("cross-worker collection recorded no remote frees")
	}
	before := nativeAllocatorSpanTransfers()
	schedulerSetThread(0, 0)
	if tsnative_heap_alloc(32) == nil {
		t.Fatal("worker 0 remote-free drain allocation failed")
	}
	schedulerSetThread(1, 0)
	if tsnative_heap_alloc(32) == nil {
		t.Fatal("worker 1 transfer allocation failed")
	}
	if got := nativeAllocatorSpanTransfers(); got <= before {
		t.Fatalf("span transfer count = %d, want > %d", got, before)
	}
	schedulerSetThread(-1, 0)
}

func TestGCIterativeMarkHandlesDeepHeapGraph(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	const nodes = 20000
	var head unsafe.Pointer
	for i := 0; i < nodes; i++ {
		node := tsnative_heap_alloc(16)
		if node == nil {
			t.Fatalf("node allocation %d failed", i)
		}
		*(*unsafe.Pointer)(node) = head
		head = node
	}
	root := tsnative_gc_root_register(unsafe.Pointer(&head))
	if root == nil {
		t.Fatal("deep graph root registration failed")
	}
	tsnative_gc_collect()
	if got := tsnative_heap_live_allocations(); got != nodes {
		t.Fatalf("live allocations = %d, want %d", got, nodes)
	}
	work, _ := nativeGCMarkWork()
	if work < nodes {
		t.Fatalf("mark work = %d, want at least %d", work, nodes)
	}
	tsnative_gc_root_unregister(root)
}

func TestGCMarkQueueSwitchesAcrossSpanOwners(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	schedulerSetThread(0, 0)
	parent := tsnative_heap_alloc(16)
	schedulerSetThread(1, 0)
	child := tsnative_heap_alloc(16)
	if parent == nil || child == nil {
		t.Fatal("cross-owner graph allocation failed")
	}
	*(*unsafe.Pointer)(parent) = child
	rooted := parent
	root := tsnative_gc_root_register(unsafe.Pointer(&rooted))
	if root == nil {
		t.Fatal("cross-owner root registration failed")
	}
	_, before := nativeGCMarkWork()
	schedulerSetThread(0, 0)
	tsnative_gc_collect()
	_, after := nativeGCMarkWork()
	if after <= before {
		t.Fatalf("mark queue switches = %d, want > %d", after, before)
	}
	if !nativeHeapContains(parent) || !nativeHeapContains(child) {
		t.Fatal("cross-owner graph was not preserved")
	}
	tsnative_gc_root_unregister(root)
	schedulerSetThread(-1, 0)
}

func TestGCParallelMarkUsesBoundedAssistWorkers(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	previousProcs := runtime.GOMAXPROCS(4)
	defer runtime.GOMAXPROCS(previousProcs)
	t.Setenv("TSNATIVE_WORKERS", "4")
	t.Setenv("TSNATIVE_GC_MARK_WORKERS", "4")

	const rootsCount = 1024
	roots := make([]unsafe.Pointer, rootsCount)
	for i := range roots {
		roots[i] = tsnative_heap_alloc(2048)
		if roots[i] == nil {
			t.Fatalf("root allocation %d failed", i)
		}
	}
	token := tsnative_gc_enter(unsafe.Pointer(&roots[0]), uintptr(len(roots)))
	if token == nil {
		t.Fatal("root frame allocation failed")
	}
	tsnative_gc_collect()
	workers, pages := nativeGCMarkAssist()
	if workers != 3 {
		t.Fatalf("GC assist workers = %d, want 3", workers)
	}
	if pages == 0 {
		t.Fatal("parallel GC helpers processed no mark pages")
	}
	if got := tsnative_heap_live_allocations(); got != rootsCount {
		t.Fatalf("live allocations = %d, want %d", got, rootsCount)
	}
	tsnative_gc_leave(token)
	tsnative_gc_collect()
	if got := tsnative_heap_live_allocations(); got != 0 {
		t.Fatalf("live allocations after root release = %d, want 0", got)
	}
	runtime.KeepAlive(roots)
}

func TestGCParallelMarkReusesIdleSchedulerWorkers(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	tsnative_scheduler_shutdown()
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()
	defer tsnative_scheduler_shutdown()
	previousProcs := runtime.GOMAXPROCS(4)
	defer runtime.GOMAXPROCS(previousProcs)
	t.Setenv("TSNATIVE_WORKERS", "4")
	t.Setenv("TSNATIVE_GC_MARK_WORKERS", "4")

	if tsnative_scheduler_init() != 0 {
		t.Fatal("scheduler init failed")
	}
	deadline := time.Now().Add(time.Second)
	for schedulerIdleWorkerCount() != 4 && time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(time.Millisecond)
	}
	if idle := schedulerIdleWorkerCount(); idle != 4 {
		t.Fatalf("idle scheduler workers = %d, want 4", idle)
	}

	const rootsCount = 1024
	roots := make([]unsafe.Pointer, rootsCount)
	for i := range roots {
		roots[i] = tsnative_heap_alloc(2048)
		if roots[i] == nil {
			t.Fatalf("root allocation %d failed", i)
		}
	}
	token := tsnative_gc_enter(unsafe.Pointer(&roots[0]), uintptr(len(roots)))
	if token == nil {
		t.Fatal("root frame allocation failed")
	}
	tsnative_gc_collect()
	donors, pages := nativeGCMarkIdleAssist()
	if donors != 3 {
		t.Fatalf("idle GC donor workers = %d, want 3", donors)
	}
	if pages == 0 {
		t.Fatal("idle scheduler GC donors processed no mark pages")
	}
	if got := tsnative_heap_live_allocations(); got != rootsCount {
		t.Fatalf("live allocations = %d, want %d", got, rootsCount)
	}
	tsnative_gc_leave(token)
	tsnative_gc_collect()
	if got := tsnative_heap_live_allocations(); got != 0 {
		t.Fatalf("live allocations after root release = %d, want 0", got)
	}
	runtime.KeepAlive(roots)
}

func TestGCMarkBatchesBlocksByPage(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	schedulerSetThread(0, 0)
	defer schedulerSetThread(-1, 0)
	container := tsnative_heap_alloc(128)
	if container == nil {
		t.Fatal("container allocation failed")
	}
	for i := 0; i < 10; i++ {
		child := tsnative_heap_alloc(16)
		if child == nil {
			t.Fatalf("child allocation %d failed", i)
		}
		*(*unsafe.Pointer)(unsafe.Add(container, uintptr(i)*unsafe.Sizeof(uintptr(0)))) = child
	}
	rooted := container
	root := tsnative_gc_root_register(unsafe.Pointer(&rooted))
	if root == nil {
		t.Fatal("page batch root registration failed")
	}
	tsnative_gc_collect()
	work, _ := nativeGCMarkWork()
	pages := nativeGCMarkPages()
	if work < 11 || pages == 0 || pages >= work {
		t.Fatalf("mark batching work=%d pages=%d", work, pages)
	}
	tsnative_gc_root_unregister(root)
}
