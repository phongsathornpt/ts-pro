package runtimego

import (
	"runtime"
	"sync"
	"testing"
	"time"
	"unsafe"
)

func TestPreciseHeapTraceMetricsCountOnlyDeclaredReferenceWords(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	conservative := tsnative_heap_alloc(64)
	atomic := tsnative_heap_alloc_atomic(64)
	child := tsnative_heap_alloc_atomic(16)
	offset := uintptr(0)
	precise := tsnative_heap_alloc_refs(64, unsafe.Pointer(&offset), 1)
	*(*unsafe.Pointer)(precise) = child

	conservativeRoot := tsnative_gc_root_register(unsafe.Pointer(&conservative))
	atomicRoot := tsnative_gc_root_register(unsafe.Pointer(&atomic))
	preciseRoot := tsnative_gc_root_register(unsafe.Pointer(&precise))
	tsnative_gc_collect()
	metrics := nativeGCTraceStats()
	if metrics.words != 9 || metrics.conservative != 1 || metrics.precise != 1 || metrics.atomic != 2 {
		t.Fatalf("trace metrics = %+v, want words=9 conservative=1 precise=1 atomic=2", metrics)
	}
	for _, token := range []unsafe.Pointer{preciseRoot, atomicRoot, conservativeRoot} {
		tsnative_gc_root_unregister(token)
	}
}

func TestAtomicHeapLayoutDoesNotTracePointerLikePayload(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	child := tsnative_heap_alloc_atomic(16)
	parent := tsnative_heap_alloc_atomic(unsafe.Sizeof(uintptr(0)))
	*(*uintptr)(parent) = uintptr(child)
	root := tsnative_gc_root_register(unsafe.Pointer(&parent))
	tsnative_gc_collect()
	if !nativeHeapContains(parent) {
		t.Fatal("rooted atomic parent was reclaimed")
	}
	if nativeHeapContains(child) {
		t.Fatal("atomic pointer-like payload incorrectly retained child")
	}
	tsnative_gc_root_unregister(root)
}

func TestReferenceOffsetHeapLayoutTracesDeclaredSlot(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	child := tsnative_heap_alloc_atomic(16)
	offset := uintptr(unsafe.Sizeof(uintptr(0)))
	parent := tsnative_heap_alloc_refs(2*unsafe.Sizeof(uintptr(0)), unsafe.Pointer(&offset), 1)
	*(*uintptr)(parent) = uintptr(0xdeadbeef)
	*(*unsafe.Pointer)(unsafe.Add(parent, offset)) = child
	root := tsnative_gc_root_register(unsafe.Pointer(&parent))
	tsnative_gc_collect()
	if !nativeHeapContains(parent) || !nativeHeapContains(child) {
		t.Fatal("declared reference slot did not retain child")
	}
	tsnative_gc_root_unregister(root)
	tsnative_gc_collect()
	if nativeHeapContains(parent) || nativeHeapContains(child) {
		t.Fatal("reference-layout graph survived after root removal")
	}
}

func TestSettledReferencePromiseRootsResultUntilRelease(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	value := tsnative_heap_alloc_atomic(32)
	promise := tsnative_promise_resolve_ref(value)
	value = nil
	tsnative_gc_collect()
	task := lookupNativeTask(uintptr(promise))
	if task == nil || task.resultRef == nil || !nativeHeapContains(task.resultRef) {
		t.Fatal("settled Promise.resolve reference result was not rooted")
	}
	tsnative_task_release(promise)
	tsnative_gc_collect()
	if tsnative_heap_live_allocations() != 0 {
		t.Fatalf("released Promise.resolve retained %d heap allocations", tsnative_heap_live_allocations())
	}
}

func TestSettledPromiseRetainKeepsReferenceResultAlive(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	value := tsnative_heap_alloc_atomic(32)
	promise := tsnative_promise_resolve_ref(value)
	if tsnative_task_retain(promise) != 0 {
		t.Fatal("retain settled Promise.resolve failed")
	}
	value = nil
	tsnative_task_release(promise)
	tsnative_gc_collect()
	task := lookupNativeTask(uintptr(promise))
	if task == nil || task.refs.Load() != 1 || task.resultRef == nil || !nativeHeapContains(task.resultRef) {
		t.Fatal("retained Promise.resolve did not preserve task/result lifetime")
	}
	tsnative_task_release(promise)
	tsnative_gc_collect()
	if lookupNativeTask(uintptr(promise)) != nil {
		t.Fatal("final Promise.resolve release did not destroy task storage")
	}
	if tsnative_heap_live_allocations() != 0 {
		t.Fatalf("final retained Promise.resolve release left %d heap allocations", tsnative_heap_live_allocations())
	}
}

func TestSettledRejectedPromiseRootsFailureUntilRelease(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	reason := tsnative_heap_alloc_atomic(32)
	promise := tsnative_promise_reject(reason, 1)
	reason = nil
	tsnative_gc_collect()
	task := lookupNativeTask(uintptr(promise))
	if task == nil || task.failureRef == nil || !nativeHeapContains(task.failureRef) {
		t.Fatal("settled Promise.reject failure was not rooted")
	}
	tsnative_task_release(promise)
	tsnative_gc_collect()
	if tsnative_heap_live_allocations() != 0 {
		t.Fatalf("released Promise.reject retained %d heap allocations", tsnative_heap_live_allocations())
	}
}

func TestGCDefersForForeignActiveNativeRootStack(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	rooted := tsnative_heap_alloc(16)
	garbage := tsnative_heap_alloc(16)
	slot := uintptr(rooted)
	foreignTID := nativeCurrentThreadID() + 1_000_000

	nativeHeapWorld.RLock()
	index := nativeRootShardIndexForThread(foreignTID)
	shard, tokenPtr := nativeRoots.lockShardWithToken(index)
	token := uintptr(tokenPtr)
	shard.roots[token] = &nativeRootFrame{
		token: tokenPtr, slots: unsafe.Pointer(&slot), count: 1, tid: foreignTID,
	}
	shard.threadStacks[foreignTID] = []uintptr{token}
	before := nativeHeapCollections.Load()
	shard.Unlock()
	nativeHeapWorld.RUnlock()

	tsnative_gc_collect()

	after := nativeHeapCollections.Load()
	garbageLive := nativeBlocks.get(uintptr(garbage)) != nil
	nativeHeapWorld.RLock()
	shard = nativeRoots.shardForToken(tokenPtr)
	shard.Lock()
	frame := shard.roots[token]
	delete(shard.roots, token)
	delete(shard.threadStacks, foreignTID)
	nativeRoots.freeTokenLocked(shard, frame.token)
	shard.Unlock()
	nativeHeapWorld.RUnlock()

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
	rootShard := &nativeRoots.shards[0]
	rootShard.Lock()
	rootStarted := make(chan struct{})
	rootDone := make(chan struct{})
	go func() {
		close(rootStarted)
		rootShard.Lock()
		rootShard.Unlock()
		close(rootDone)
	}()
	<-rootStarted
	rootDeadline := time.Now().Add(250 * time.Millisecond)
	for nativeRootLockMetrics().contended == 0 && time.Now().Before(rootDeadline) {
		runtime.Gosched()
	}
	if nativeRootLockMetrics().contended == 0 {
		rootShard.Unlock()
		t.Fatal("root lock contention was not observed")
	}
	rootShard.Unlock()
	<-rootDone
	rootMetrics := nativeRootLockMetrics()
	if rootMetrics.acquisitions < 2 || rootMetrics.waitNanos == 0 {
		t.Fatalf("root lock metrics = %+v, want acquisitions >= 2 and wait > 0", rootMetrics)
	}
}

func TestIndependentRootShardsDoNotSerialize(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	first := &nativeRoots.shards[0]
	second := &nativeRoots.shards[1]
	first.Lock()
	done := make(chan struct{})
	go func() {
		second.Lock()
		second.Unlock()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		first.Unlock()
		t.Fatal("independent root shard blocked on another shard")
	}
	first.Unlock()
}

func TestPersistentRootCanUnregisterAcrossOSThreads(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	slot := tsnative_heap_alloc(16)
	registeredTID := nativeCurrentThreadID()
	token := tsnative_gc_root_register(unsafe.Pointer(&slot))
	if token == nil {
		t.Fatal("persistent root registration failed")
	}
	unregisteredTID := make(chan int, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		tid := nativeCurrentThreadID()
		tsnative_gc_root_unregister(token)
		unregisteredTID <- tid
	}()
	if tid := <-unregisteredTID; tid == registeredTID {
		t.Fatalf("persistent root test reused registration OS thread %d", tid)
	}
	if count := nativeRoots.rootCount(); count != 0 {
		t.Fatalf("persistent roots after cross-thread unregister = %d, want 0", count)
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
	nativeHeapWorld.resetMetrics()
	nativeBlocks.resetMetrics()

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
	worldMetrics := nativeWorldLockMetrics()
	blockMetrics := nativeBlockLockMetrics()
	t.Logf("heap lock acquisitions=%d contentions=%d wait_ns=%d", heapMetrics.acquisitions, heapMetrics.contended, heapMetrics.waitNanos)
	t.Logf("root lock acquisitions=%d contentions=%d wait_ns=%d", rootMetrics.acquisitions, rootMetrics.contended, rootMetrics.waitNanos)
	t.Logf("world read acquisitions=%d contentions=%d wait_ns=%d", worldMetrics.read.acquisitions, worldMetrics.read.contended, worldMetrics.read.waitNanos)
	t.Logf("block write acquisitions=%d contentions=%d wait_ns=%d", blockMetrics.write.acquisitions, blockMetrics.write.contended, blockMetrics.write.waitNanos)
	if worldMetrics.read.contended != 0 {
		t.Fatalf("steady-state world read contention = %d, want 0 without GC writers", worldMetrics.read.contended)
	}
	if heapMetrics.acquisitions >= 128 {
		t.Fatalf("global heap lock acquisitions = %d, want < 128 for %d allocations", heapMetrics.acquisitions, workers*allocationsPerWorker)
	}
}

func TestConcurrentGCWorldBarrierProfile(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()
	nativeHeap.resetMetrics()
	nativeRoots.resetMetrics()
	nativeHeapWorld.resetMetrics()
	nativeBlocks.resetMetrics()

	const workers = 8
	const allocationsPerWorker = 2000
	const collections = 32
	start := make(chan struct{})
	var workersDone sync.WaitGroup
	workersDone.Add(workers)
	for owner := 0; owner < workers; owner++ {
		go func(owner int) {
			defer workersDone.Done()
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			schedulerSetThread(owner, 0)
			defer schedulerSetThread(-1, 0)
			<-start
			for i := 0; i < allocationsPerWorker; i++ {
				_ = tsnative_heap_alloc(32)
			}
		}(owner)
	}
	collectorDone := make(chan struct{})
	go func() {
		<-start
		for i := 0; i < collections; i++ {
			tsnative_gc_collect()
			runtime.Gosched()
		}
		close(collectorDone)
	}()
	close(start)
	workersDone.Wait()
	<-collectorDone

	worldMetrics := nativeWorldLockMetrics()
	blockMetrics := nativeBlockLockMetrics()
	heapMetrics := nativeHeapLockMetrics()
	t.Logf("GC profile world read=%+v write=%+v", worldMetrics.read, worldMetrics.write)
	t.Logf("GC profile block read=%+v write=%+v", blockMetrics.read, blockMetrics.write)
	t.Logf("GC profile heap=%+v collections=%d", heapMetrics, nativeHeapCollections.Load())
	if worldMetrics.write.acquisitions < collections {
		t.Fatalf("world write acquisitions = %d, want >= %d", worldMetrics.write.acquisitions, collections)
	}
	if blockMetrics.write.acquisitions == 0 {
		t.Fatal("GC/allocation workload recorded no block-table writes")
	}
}

func TestRootMetadataAndHeapAllocationUseIndependentLocks(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	rootShard := &nativeRoots.shards[0]
	rootShard.Lock()
	allocationDone := make(chan unsafe.Pointer, 1)
	go func() {
		allocationDone <- tsnative_heap_alloc(16)
	}()
	var allocated unsafe.Pointer
	select {
	case allocated = <-allocationDone:
	case <-time.After(250 * time.Millisecond):
		rootShard.Unlock()
		t.Fatal("heap allocation blocked on root metadata lock")
	}
	rootShard.Unlock()
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

func TestMinorGCReclaimsUnrootedNursery(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()
	t.Setenv("TSNATIVE_GC_NURSERY_BYTES", "1024")

	raw := tsnative_heap_alloc(1024)
	if raw == nil || !nativeGCMinorRequested.Load() {
		t.Fatal("nursery threshold did not request minor GC")
	}
	tsnative_gc_safepoint()
	if nativeHeapContains(raw) {
		t.Fatal("unrooted nursery allocation survived minor GC")
	}
	if nativeGCMinorCollections.Load() != 1 || nativeGCMajorCollections.Load() != 0 {
		t.Fatalf("collections minor=%d major=%d, want 1/0", nativeGCMinorCollections.Load(), nativeGCMajorCollections.Load())
	}
	if got := nativeNurseryLiveBytes(); got != 0 {
		t.Fatalf("nursery bytes = %d, want 0 after minor GC", got)
	}
}

func TestMinorGCPromotesRootedNurseryObject(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()
	t.Setenv("TSNATIVE_GC_NURSERY_BYTES", "1024")

	raw := tsnative_heap_alloc(1024)
	root := tsnative_gc_root_register(unsafe.Pointer(&raw))
	tsnative_gc_safepoint()
	block := nativeBlocks.get(uintptr(raw))
	if block == nil || block.generation != nativeHeapGenerationOld {
		t.Fatalf("rooted nursery block = %+v, want promoted old block", block)
	}
	if nativeGCPromotedBlocks.Load() != 1 {
		t.Fatalf("promoted blocks = %d, want 1", nativeGCPromotedBlocks.Load())
	}
	if nativeGCPromotedBytes.Load() != 1024 || nativeHeapOldBytes.Load() != 1024 {
		t.Fatalf("promoted bytes=%d old bytes=%d, want 1024/1024", nativeGCPromotedBytes.Load(), nativeHeapOldBytes.Load())
	}
	tsnative_gc_root_unregister(root)
	tsnative_gc_collect()
	if nativeHeapContains(raw) {
		t.Fatal("promoted object survived major GC after root release")
	}
}

func TestMinorGCRequiresBarrierForOldToYoungReferences(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()
	t.Setenv("TSNATIVE_GC_NURSERY_BYTES", "1024")

	parent := tsnative_heap_alloc(16)
	root := tsnative_gc_root_register(unsafe.Pointer(&parent))
	tsnative_gc_collect()
	parentBlock := nativeBlocks.get(uintptr(parent))
	if parentBlock == nil || parentBlock.generation != nativeHeapGenerationOld {
		t.Fatal("parent was not old after major GC")
	}

	child := tsnative_heap_alloc(1024)
	*(*unsafe.Pointer)(parent) = child
	tsnative_gc_safepoint()
	if nativeBlocks.get(uintptr(child)) != nil {
		t.Fatal("raw old-to-young store survived minor GC without a remembered-set barrier")
	}
	if got := nativeGCMinorOldScans.Load(); got != 0 {
		t.Fatalf("minor old scans = %d, want 0 without remembered parents", got)
	}
	if !nativeHeapContains(parent) {
		t.Fatal("rooted old parent was reclaimed by minor GC")
	}

	tsnative_gc_root_unregister(root)
	tsnative_gc_collect()
	if nativeHeapContains(parent) {
		t.Fatal("old parent survived major GC after root release")
	}
}

func TestGCStoreRefRecordsOldToYoungParent(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()
	t.Setenv("TSNATIVE_GC_NURSERY_BYTES", "1024")

	parent := tsnative_heap_alloc(16)
	root := tsnative_gc_root_register(unsafe.Pointer(&parent))
	tsnative_gc_collect()
	child := tsnative_heap_alloc(1024)
	nativeGCStoreRef(parent, parent, child)
	if got := nativeRemembered.count(); got != 1 {
		t.Fatalf("remembered parents = %d, want 1", got)
	}
	if nativeRemembered.stores.Load() != 1 || nativeRemembered.records.Load() != 1 {
		t.Fatalf("barrier stores=%d records=%d, want 1/1", nativeRemembered.stores.Load(), nativeRemembered.records.Load())
	}
	tsnative_gc_safepoint()
	childBlock := nativeBlocks.get(uintptr(child))
	if childBlock == nil || childBlock.generation != nativeHeapGenerationOld {
		t.Fatalf("remembered child = %+v, want promoted old block", childBlock)
	}
	if got := nativeRemembered.count(); got != 0 {
		t.Fatalf("remembered parents after minor GC = %d, want 0", got)
	}
	tsnative_gc_root_unregister(root)
}

func TestGCStoreRefSlotResolvesOldParentInterior(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()
	t.Setenv("TSNATIVE_GC_NURSERY_BYTES", "1024")

	parent := tsnative_heap_alloc(32)
	root := tsnative_gc_root_register(unsafe.Pointer(&parent))
	tsnative_gc_collect()
	child := tsnative_heap_alloc(1024)
	slot := unsafe.Add(parent, unsafe.Sizeof(uintptr(0)))
	nativeGCStoreRefSlot(slot, child)
	if got := nativeRemembered.count(); got != 1 {
		t.Fatalf("remembered parents = %d, want 1 for interior slot", got)
	}
	if stored := *(*unsafe.Pointer)(slot); stored != child {
		t.Fatalf("interior slot child = %p, want %p", stored, child)
	}
	tsnative_gc_safepoint()
	if childBlock := nativeBlocks.get(uintptr(child)); childBlock == nil || childBlock.generation != nativeHeapGenerationOld {
		t.Fatalf("interior-slot child = %+v, want promoted old block", childBlock)
	}
	tsnative_gc_root_unregister(root)
}

func TestGCStoreRefResolvesInteriorYoungChild(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()
	t.Setenv("TSNATIVE_GC_NURSERY_BYTES", "1024")

	parent := tsnative_heap_alloc(16)
	root := tsnative_gc_root_register(unsafe.Pointer(&parent))
	tsnative_gc_collect()
	child := tsnative_heap_alloc(1024)
	interior := unsafe.Add(child, unsafe.Sizeof(uintptr(0)))
	nativeGCStoreRef(parent, parent, interior)
	if got := nativeRemembered.count(); got != 1 {
		t.Fatalf("remembered parents = %d, want 1 for interior young child", got)
	}
	tsnative_gc_safepoint()
	if block := nativeBlocks.get(uintptr(child)); block == nil || block.generation != nativeHeapGenerationOld {
		t.Fatalf("interior-referenced child = %+v, want promoted old block", block)
	}
	tsnative_gc_root_unregister(root)
}

func TestGCStoreRefSkipsYoungParent(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	parent := tsnative_heap_alloc(16)
	child := tsnative_heap_alloc(16)
	nativeGCStoreRef(parent, parent, child)
	if got := nativeRemembered.count(); got != 0 {
		t.Fatalf("young parent entered remembered set: %d", got)
	}
	if stored := *(*unsafe.Pointer)(parent); stored != child {
		t.Fatalf("stored child = %p, want %p", stored, child)
	}
}

func TestPromotedOldBytesTriggerMajorCadence(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()
	t.Setenv("TSNATIVE_GC_NURSERY_BYTES", "524288")

	first := tsnative_heap_alloc(524288)
	firstRoot := tsnative_gc_root_register(unsafe.Pointer(&first))
	tsnative_gc_safepoint()
	if nativeHeapOldBytes.Load() != 524288 || nativeGCMajorRequested.Load() {
		t.Fatalf("after first promotion old=%d majorRequested=%v, want 524288/false", nativeHeapOldBytes.Load(), nativeGCMajorRequested.Load())
	}

	second := tsnative_heap_alloc(524288)
	secondRoot := tsnative_gc_root_register(unsafe.Pointer(&second))
	tsnative_gc_safepoint()
	if nativeGCMinorCollections.Load() != 2 || nativeGCMajorCollections.Load() != 0 {
		t.Fatalf("after second promotion minor=%d major=%d, want 2/0", nativeGCMinorCollections.Load(), nativeGCMajorCollections.Load())
	}
	if nativeHeapOldBytes.Load() != initialMajorGCThreshold || !nativeGCMajorRequested.Load() {
		t.Fatalf("old=%d majorRequested=%v, want %d/true", nativeHeapOldBytes.Load(), nativeGCMajorRequested.Load(), initialMajorGCThreshold)
	}

	tsnative_gc_safepoint()
	if nativeGCMajorCollections.Load() != 1 || nativeGCMajorRequested.Load() {
		t.Fatalf("major collections=%d requested=%v, want 1/false", nativeGCMajorCollections.Load(), nativeGCMajorRequested.Load())
	}
	if nativeHeapOldBytes.Load() != initialMajorGCThreshold {
		t.Fatalf("old bytes after rooted major = %d, want %d", nativeHeapOldBytes.Load(), initialMajorGCThreshold)
	}
	tsnative_gc_root_unregister(secondRoot)
	tsnative_gc_root_unregister(firstRoot)
}

func TestAggregateYoungPressureDoesNotTriggerMajor(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Setenv("TSNATIVE_GC_NURSERY_BYTES", "262144")

	for owner := 0; owner < 8; owner++ {
		schedulerSetThread(owner, 0)
		_ = tsnative_heap_alloc(131072)
	}
	if got := nativeHeapBytes.Load(); got != 8*131072 {
		t.Fatalf("young live bytes = %d, want %d", got, 8*131072)
	}
	if got := nativeHeapOldBytes.Load(); got != 0 {
		t.Fatalf("old bytes under young-only pressure = %d, want 0", got)
	}
	if nativeGCMajorRequested.Load() || nativeGCMinorRequested.Load() {
		t.Fatalf("premature GC request minor=%v major=%v", nativeGCMinorRequested.Load(), nativeGCMajorRequested.Load())
	}

	schedulerSetThread(0, 0)
	_ = tsnative_heap_alloc(131072)
	if !nativeGCMinorRequested.Load() {
		t.Fatal("worker-local nursery bound did not request minor GC")
	}
	if nativeGCMajorRequested.Load() {
		t.Fatal("aggregate young pressure incorrectly requested major GC")
	}
	tsnative_gc_safepoint()
	schedulerSetThread(-1, 0)
	if nativeGCMinorCollections.Load() != 1 || nativeGCMajorCollections.Load() != 0 {
		t.Fatalf("collections minor=%d major=%d, want 1/0", nativeGCMinorCollections.Load(), nativeGCMajorCollections.Load())
	}
	if nativeHeapBytes.Load() != 0 || nativeHeapOldBytes.Load() != 0 {
		t.Fatalf("post-minor bytes live=%d old=%d, want 0/0", nativeHeapBytes.Load(), nativeHeapOldBytes.Load())
	}
}

func TestMinorGCScansOnlyNurseryMembership(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()
	t.Setenv("TSNATIVE_GC_NURSERY_BYTES", "65536")

	const oldChildren = 1024
	wordSize := unsafe.Sizeof(uintptr(0))
	parent := tsnative_heap_alloc(uintptr(oldChildren) * wordSize)
	root := tsnative_gc_root_register(unsafe.Pointer(&parent))
	for i := 0; i < oldChildren; i++ {
		child := tsnative_heap_alloc(16)
		*(*unsafe.Pointer)(unsafe.Add(parent, uintptr(i)*wordSize)) = child
	}
	tsnative_gc_collect()
	if got := nativeNurseryBlockCount(); got != 0 {
		t.Fatalf("nursery blocks after major = %d, want 0", got)
	}
	nativeGCMinorNurseryScans.Store(0)

	young := tsnative_heap_alloc(65536)
	if got := nativeNurseryBlockCount(); got != 1 {
		t.Fatalf("nursery blocks before minor = %d, want 1", got)
	}
	tsnative_gc_safepoint()
	if nativeHeapContains(young) {
		t.Fatal("unrooted indexed nursery block survived minor GC")
	}
	if got := nativeGCMinorNurseryScans.Load(); got != 1 {
		t.Fatalf("minor nursery scans = %d, want 1 independent of %d old children", got, oldChildren)
	}
	if got := nativeNurseryBlockCount(); got != 0 {
		t.Fatalf("nursery blocks after minor = %d, want 0", got)
	}
	if got := tsnative_heap_live_allocations(); got != oldChildren+1 {
		t.Fatalf("live allocations after minor = %d, want %d old blocks", got, oldChildren+1)
	}
	tsnative_gc_root_unregister(root)
}

func TestMinorGCDrainsAllWorkerNurseryBuckets(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Setenv("TSNATIVE_GC_NURSERY_BYTES", "65536")

	schedulerSetThread(0, 0)
	_ = tsnative_heap_alloc(32768)
	schedulerSetThread(1, 0)
	_ = tsnative_heap_alloc(32768)
	schedulerSetThread(0, 0)
	_ = tsnative_heap_alloc(32768)
	if got := nativeNurseryBlockCount(); got != 3 {
		t.Fatalf("nursery blocks across workers = %d, want 3", got)
	}
	nativeGCMinorNurseryScans.Store(0)
	tsnative_gc_safepoint()
	schedulerSetThread(-1, 0)
	if got := nativeGCMinorNurseryScans.Load(); got != 3 {
		t.Fatalf("minor nursery scans = %d, want all 3 worker-local blocks", got)
	}
	if got := nativeNurseryBlockCount(); got != 0 {
		t.Fatalf("nursery blocks after cross-worker minor = %d, want 0", got)
	}
	if got := tsnative_heap_live_allocations(); got != 0 {
		t.Fatalf("live allocations after cross-worker minor = %d, want 0", got)
	}
}

func TestNurseryThresholdIsBoundedPerWorker(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Setenv("TSNATIVE_GC_NURSERY_BYTES", "65536")

	schedulerSetThread(0, 0)
	_ = tsnative_heap_alloc(32768)
	schedulerSetThread(1, 0)
	_ = tsnative_heap_alloc(32768)
	if nativeGCMinorRequested.Load() {
		t.Fatal("separate worker nurseries incorrectly shared their threshold")
	}
	schedulerSetThread(0, 0)
	_ = tsnative_heap_alloc(32768)
	if !nativeGCMinorRequested.Load() {
		t.Fatal("worker-local nursery did not request minor GC at its bound")
	}
	schedulerSetThread(-1, 0)
}

func TestAutomaticNurseryCollectionsDelayMajorGC(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	t.Setenv("TSNATIVE_GC_NURSERY_BYTES", "65536")

	roots := make([]unsafe.Pointer, 8)
	frame := tsnative_gc_enter(unsafe.Pointer(&roots[0]), uintptr(len(roots)))
	for i := range roots {
		roots[i] = tsnative_heap_alloc(65536)
		if !nativeGCMinorRequested.Load() {
			t.Fatalf("cycle %d did not request minor GC", i)
		}
		tsnative_gc_safepoint()
		if got := nativeGCMinorCollections.Load(); got != uint64(i+1) {
			t.Fatalf("minor collections after cycle %d = %d, want %d", i, got, i+1)
		}
		if got := nativeGCMajorCollections.Load(); got != 0 {
			t.Fatalf("major collections before 1 MiB threshold = %d, want 0", got)
		}
	}
	if got := nativeHeapBytes.Load(); got != 8*65536 {
		t.Fatalf("live bytes after nursery cycles = %d, want %d", got, 8*65536)
	}
	tsnative_gc_leave(frame)
	tsnative_gc_collect()
	if nativeGCMajorCollections.Load() != 1 || nativeHeapAllocations.Load() != 0 {
		t.Fatalf("final major collections=%d live=%d, want 1/0", nativeGCMajorCollections.Load(), nativeHeapAllocations.Load())
	}
	runtime.KeepAlive(roots)
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
	rootCount := nativeRoots.rootCount()
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

func TestUnresolvedThenablePromiseLastReleaseDoesNotBlock(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()
	promise := tsnative_promise_thenable_new(1)
	if promise == nil {
		t.Fatal("thenable Promise allocation failed")
	}
	if task := lookupNativeTask(uintptr(promise)); task == nil || task.status.Load() != nativeTaskWaiting {
		t.Fatal("thenable Promise was not created pending")
	}
	tsnative_task_release(promise)
	if task := lookupNativeTask(uintptr(promise)); task != nil {
		t.Fatal("unresolved thenable Promise survived final release")
	}
}
