package runtime

import (
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"unsafe"
)

const (
	nativeSchedulerMaxWorkers   = 256
	nativeSchedulerDefaultTasks = 1_000_000
	nativeSchedulerHardMaxTasks = 10_000_000
)

type nativeSchedulerWorker struct {
	mu    sync.Mutex
	deque []uintptr
	index int
}

func (w *nativeSchedulerWorker) pushTail(task uintptr) {
	w.mu.Lock()
	w.deque = append(w.deque, task)
	w.mu.Unlock()
}

func (w *nativeSchedulerWorker) popTail() uintptr {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.deque) == 0 {
		return 0
	}
	index := len(w.deque) - 1
	task := w.deque[index]
	w.deque[index] = 0
	w.deque = w.deque[:index]
	return task
}

func (w *nativeSchedulerWorker) stealHead() uintptr {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.deque) == 0 {
		return 0
	}
	task := w.deque[0]
	copy(w.deque, w.deque[1:])
	last := len(w.deque) - 1
	w.deque[last] = 0
	w.deque = w.deque[:last]
	return task
}

type nativeSchedulerState struct {
	mu       sync.Mutex
	wake     *sync.Cond
	done     *sync.Cond
	gcAssist *sync.Cond
	workers  []*nativeSchedulerWorker
	inject   []uintptr
	started  bool
	stopping bool
	maxTasks int64
	nextID   uint64
	idle     int

	gcMarkState  *nativeGCMarkState
	gcMarkSlots  int
	gcMarkClaims int
	gcMarkActive int

	wg sync.WaitGroup

	active    atomic.Int64
	peak      atomic.Int64
	runnable  atomic.Int64
	spawned   atomic.Uint64
	completed atomic.Uint64
	stealTry  atomic.Uint64
	stealOK   atomic.Uint64
	parks     atomic.Uint64
	wakeups   atomic.Uint64
}

var goScheduler = func() *nativeSchedulerState {
	s := &nativeSchedulerState{}
	s.wake = sync.NewCond(&s.mu)
	s.done = sync.NewCond(&s.mu)
	s.gcAssist = sync.NewCond(&s.mu)
	return s
}()

var schedulerThreads = struct {
	sync.RWMutex
	workers map[int]int
	tasks   map[int]uintptr
}{workers: map[int]int{}, tasks: map[int]uintptr{}}

func schedulerLimit(name string, fallback, hardMax int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	if value > hardMax {
		return hardMax
	}
	return value
}

func configuredSchedulerWorkers() int {
	fallback := runtime.NumCPU()
	if fallback < 1 {
		fallback = 1
	}
	return schedulerLimit("TSNATIVE_WORKERS", fallback, nativeSchedulerMaxWorkers)
}

func configuredSchedulerMaxTasks() int {
	return schedulerLimit("TSNATIVE_MAX_TASKS", nativeSchedulerDefaultTasks, nativeSchedulerHardMaxTasks)
}

func resetSchedulerMetrics() {
	goScheduler.active.Store(0)
	goScheduler.peak.Store(0)
	goScheduler.runnable.Store(0)
	goScheduler.spawned.Store(0)
	goScheduler.completed.Store(0)
	goScheduler.stealTry.Store(0)
	goScheduler.stealOK.Store(0)
	goScheduler.parks.Store(0)
	goScheduler.wakeups.Store(0)
}

func updateSchedulerPeak(active int64) {
	for {
		peak := goScheduler.peak.Load()
		if active <= peak || goScheduler.peak.CompareAndSwap(peak, active) {
			return
		}
	}
}

func schedulerSetThread(worker int, task uintptr) {
	tid := nativeCurrentThreadID()
	schedulerThreads.Lock()
	if worker >= 0 {
		schedulerThreads.workers[tid] = worker
	} else {
		delete(schedulerThreads.workers, tid)
	}
	if task != 0 {
		schedulerThreads.tasks[tid] = task
	} else {
		delete(schedulerThreads.tasks, tid)
	}
	schedulerThreads.Unlock()
}

func schedulerSetCurrentTask(task uintptr) uintptr {
	tid := nativeCurrentThreadID()
	schedulerThreads.Lock()
	previous := schedulerThreads.tasks[tid]
	if task == 0 {
		delete(schedulerThreads.tasks, tid)
	} else {
		schedulerThreads.tasks[tid] = task
	}
	schedulerThreads.Unlock()
	return previous
}

func schedulerWorkerIndex() int {
	tid := nativeCurrentThreadID()
	schedulerThreads.RLock()
	worker, ok := schedulerThreads.workers[tid]
	schedulerThreads.RUnlock()
	if !ok {
		return -1
	}
	return worker
}

func schedulerCurrentTaskPtr() uintptr {
	tid := nativeCurrentThreadID()
	schedulerThreads.RLock()
	task := schedulerThreads.tasks[tid]
	schedulerThreads.RUnlock()
	return task
}

func schedulerInjectionPopLocked() uintptr {
	if len(goScheduler.inject) == 0 {
		return 0
	}
	task := goScheduler.inject[0]
	goScheduler.inject[0] = 0
	goScheduler.inject = goScheduler.inject[1:]
	return task
}

func schedulerSteal(worker *nativeSchedulerWorker) uintptr {
	count := len(goScheduler.workers)
	if count < 2 {
		return 0
	}
	for offset := 1; offset < count; offset++ {
		index := (worker.index + offset) % count
		goScheduler.stealTry.Add(1)
		if task := goScheduler.workers[index].stealHead(); task != 0 {
			goScheduler.stealOK.Add(1)
			return task
		}
	}
	return 0
}

func schedulerTakeWork(worker *nativeSchedulerWorker) uintptr {
	if task := worker.popTail(); task != 0 {
		goScheduler.runnable.Add(-1)
		return task
	}
	goScheduler.mu.Lock()
	task := schedulerInjectionPopLocked()
	goScheduler.mu.Unlock()
	if task == 0 {
		task = schedulerSteal(worker)
	}
	if task != 0 {
		goScheduler.runnable.Add(-1)
	}
	return task
}

func schedulerWakeTask(handle uintptr) int {
	task := lookupNativeTask(handle)
	if task == nil {
		return -1
	}
	goScheduler.mu.Lock()
	wake := nativeTaskWake(task)
	if wake == 1 {
		goScheduler.inject = append(goScheduler.inject, handle)
		goScheduler.runnable.Add(1)
		goScheduler.wake.Signal()
	}
	goScheduler.mu.Unlock()
	if wake < 0 {
		return -1
	}
	return 0
}

func schedulerStartGCIdleAssist(state *nativeGCMarkState, limit int) int {
	if state == nil || limit <= 0 {
		return 0
	}
	goScheduler.mu.Lock()
	if !goScheduler.started || goScheduler.stopping || goScheduler.idle == 0 || goScheduler.gcMarkState != nil {
		goScheduler.mu.Unlock()
		return 0
	}
	slots := limit
	if slots > goScheduler.idle {
		slots = goScheduler.idle
	}
	goScheduler.gcMarkState = state
	goScheduler.gcMarkSlots = slots
	goScheduler.gcMarkClaims = 0
	goScheduler.gcMarkActive = 0
	goScheduler.wake.Broadcast()
	for goScheduler.gcMarkClaims < slots {
		goScheduler.gcAssist.Wait()
	}
	donors := goScheduler.gcMarkClaims
	goScheduler.mu.Unlock()
	return donors
}

func schedulerClaimGCIdleAssistLocked() *nativeGCMarkState {
	if goScheduler.gcMarkState == nil || goScheduler.gcMarkClaims >= goScheduler.gcMarkSlots {
		return nil
	}
	state := goScheduler.gcMarkState
	goScheduler.gcMarkClaims++
	goScheduler.gcMarkActive++
	goScheduler.gcAssist.Broadcast()
	return state
}

func schedulerFinishGCIdleAssist(state *nativeGCMarkState) {
	goScheduler.mu.Lock()
	if goScheduler.gcMarkState == state && goScheduler.gcMarkActive > 0 {
		goScheduler.gcMarkActive--
		goScheduler.gcAssist.Broadcast()
	}
	goScheduler.mu.Unlock()
}

func schedulerStopGCIdleAssist(state *nativeGCMarkState) {
	if state == nil {
		return
	}
	goScheduler.mu.Lock()
	for goScheduler.gcMarkState == state && goScheduler.gcMarkActive != 0 {
		goScheduler.gcAssist.Wait()
	}
	if goScheduler.gcMarkState == state {
		goScheduler.gcMarkState = nil
		goScheduler.gcMarkSlots = 0
		goScheduler.gcMarkClaims = 0
		goScheduler.gcMarkActive = 0
	}
	goScheduler.mu.Unlock()
}

func schedulerIdleWorkerCount() int {
	goScheduler.mu.Lock()
	idle := goScheduler.idle
	goScheduler.mu.Unlock()
	return idle
}

func schedulerExecuteTask(handle uintptr) {
	task := lookupNativeTask(handle)
	if task == nil {
		return
	}
	previous := schedulerSetCurrentTask(handle)
	execution := executeNativeTaskOnce(task)
	schedulerSetCurrentTask(previous)
	gcSafepoint()

	goScheduler.mu.Lock()
	switch execution.kind {
	case nativeTaskExecRequeue:
		goScheduler.inject = append(goScheduler.inject, handle)
		goScheduler.runnable.Add(1)
		goScheduler.wake.Signal()
	case nativeTaskExecTerminal:
		goScheduler.active.Add(-1)
		goScheduler.completed.Add(1)
	}
	goScheduler.done.Broadcast()
	goScheduler.mu.Unlock()

	if execution.kind == nativeTaskExecTerminal {
		taskGroupDetach(handle)
		for _, waiter := range execution.completionWaiters {
			_ = schedulerWakeTask(waiter)
		}
		notifyNativePromiseAggregateWatchers(execution.aggregateWatchers, task)
		for i := 0; i < execution.completionConsumes; i++ {
			releaseNativeTaskRef(task)
		}
	}
}

func schedulerWorkerLoop(worker *nativeSchedulerWorker) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	schedulerSetThread(worker.index, 0)
	defer schedulerSetThread(-1, 0)

	for {
		if task := schedulerTakeWork(worker); task != 0 {
			schedulerExecuteTask(task)
			continue
		}

		goScheduler.mu.Lock()
		if assist := schedulerClaimGCIdleAssistLocked(); assist != nil {
			goScheduler.mu.Unlock()
			assist.worker(nativeGCMarkWorkerSchedulerDonor, worker.index)
			schedulerFinishGCIdleAssist(assist)
			continue
		}
		if goScheduler.stopping && goScheduler.runnable.Load() == 0 {
			goScheduler.mu.Unlock()
			return
		}

		goScheduler.idle++
		goScheduler.parks.Add(1)
		goScheduler.wake.Wait()
		goScheduler.wakeups.Add(1)
		goScheduler.idle--
		assist := schedulerClaimGCIdleAssistLocked()
		stopping := goScheduler.stopping && goScheduler.runnable.Load() == 0
		goScheduler.mu.Unlock()

		if assist != nil {
			assist.worker(nativeGCMarkWorkerSchedulerDonor, worker.index)
			schedulerFinishGCIdleAssist(assist)
			continue
		}
		if stopping {
			return
		}
	}
}

func schedulerInit() int32 {
	goScheduler.mu.Lock()
	if goScheduler.started {
		goScheduler.mu.Unlock()
		return 0
	}
	workerCount := configuredSchedulerWorkers()
	goScheduler.workers = make([]*nativeSchedulerWorker, workerCount)
	for i := range goScheduler.workers {
		goScheduler.workers[i] = &nativeSchedulerWorker{index: i}
	}
	goScheduler.inject = nil
	goScheduler.started = true
	goScheduler.stopping = false
	goScheduler.maxTasks = int64(configuredSchedulerMaxTasks())
	goScheduler.nextID = 0
	goScheduler.idle = 0
	goScheduler.gcMarkState = nil
	goScheduler.gcMarkSlots = 0
	goScheduler.gcMarkClaims = 0
	goScheduler.gcMarkActive = 0
	resetSchedulerMetrics()
	workers := append([]*nativeSchedulerWorker(nil), goScheduler.workers...)
	goScheduler.mu.Unlock()

	for _, worker := range workers {
		goScheduler.wg.Add(1)
		go func(w *nativeSchedulerWorker) {
			defer goScheduler.wg.Done()
			schedulerWorkerLoop(w)
		}(worker)
	}
	return 0
}

func schedulerSubmit(raw unsafe.Pointer) int32 {
	if raw == nil {
		return -1
	}
	handle := uintptr(raw)
	task := lookupNativeTask(handle)
	if task == nil {
		return -1
	}
	goScheduler.mu.Lock()
	active := goScheduler.active.Load()
	if !goScheduler.started || goScheduler.stopping || active >= goScheduler.maxTasks {
		goScheduler.mu.Unlock()
		return -1
	}
	goScheduler.nextID++
	task.id = goScheduler.nextID
	task.status.Store(nativeTaskRunnable)
	active = goScheduler.active.Add(1)
	goScheduler.runnable.Add(1)
	goScheduler.spawned.Add(1)
	updateSchedulerPeak(active)
	workerIndex := schedulerWorkerIndex()
	if workerIndex >= 0 && workerIndex < len(goScheduler.workers) {
		goScheduler.workers[workerIndex].pushTail(handle)
	} else {
		goScheduler.inject = append(goScheduler.inject, handle)
	}
	goScheduler.wake.Signal()
	goScheduler.mu.Unlock()
	return 0
}

func schedulerWait(raw unsafe.Pointer) int32 {
	task := lookupNativeTask(uintptr(raw))
	if task == nil {
		return -1
	}
	for !nativeTaskIsTerminal(task) {
		workerIndex := schedulerWorkerIndex()
		if workerIndex >= 0 && workerIndex < len(goScheduler.workers) && schedulerHelpOnce() != 0 {
			continue
		}
		goScheduler.mu.Lock()
		if !nativeTaskIsTerminal(task) {
			goScheduler.done.Wait()
		}
		goScheduler.mu.Unlock()
	}
	gcSafepoint()
	if task.status.Load() == nativeTaskDone {
		return 0
	}
	return -1
}

func schedulerTaskStatus(raw unsafe.Pointer) int32 {
	task := lookupNativeTask(uintptr(raw))
	if task == nil {
		return nativeTaskFailed
	}
	return task.status.Load()
}

func schedulerHelpOnce() int32 {
	workerIndex := schedulerWorkerIndex()
	if workerIndex < 0 || workerIndex >= len(goScheduler.workers) {
		return 0
	}
	task := schedulerTakeWork(goScheduler.workers[workerIndex])
	if task == 0 {
		return 0
	}
	schedulerExecuteTask(task)
	return 1
}

func schedulerShutdown() {
	goScheduler.mu.Lock()
	if !goScheduler.started {
		goScheduler.mu.Unlock()
		return
	}
	goScheduler.stopping = true
	goScheduler.wake.Broadcast()
	goScheduler.mu.Unlock()
	goScheduler.wg.Wait()

	goScheduler.mu.Lock()
	goScheduler.workers = nil
	goScheduler.inject = nil
	goScheduler.started = false
	goScheduler.stopping = false
	goScheduler.idle = 0
	goScheduler.gcMarkState = nil
	goScheduler.gcMarkSlots = 0
	goScheduler.gcMarkClaims = 0
	goScheduler.gcMarkActive = 0
	goScheduler.mu.Unlock()
}

func schedulerWorkerCount() uintptr {
	goScheduler.mu.Lock()
	count := len(goScheduler.workers)
	started := goScheduler.started
	goScheduler.mu.Unlock()
	if !started {
		count = configuredSchedulerWorkers()
	}
	return uintptr(count)
}

func schedulerIsRunning() int32 {
	goScheduler.mu.Lock()
	running := goScheduler.started && !goScheduler.stopping
	goScheduler.mu.Unlock()
	if running {
		return 1
	}
	return 0
}

func schedulerActiveTasks() uintptr { return uintptr(goScheduler.active.Load()) }

func schedulerPeakActiveTasks() uintptr { return uintptr(goScheduler.peak.Load()) }

func schedulerSpawnedTasks() uint64 { return goScheduler.spawned.Load() }

func schedulerCompletedTasks() uint64 { return goScheduler.completed.Load() }

func schedulerStealAttempts() uint64 { return goScheduler.stealTry.Load() }

func schedulerSuccessfulSteals() uint64 { return goScheduler.stealOK.Load() }

func schedulerWorkerParks() uint64 { return goScheduler.parks.Load() }

func schedulerWorkerWakeups() uint64 { return goScheduler.wakeups.Load() }

func schedulerCurrentTask() unsafe.Pointer {
	task := lookupNativeTask(schedulerCurrentTaskPtr())
	if task == nil {
		return nil
	}
	return task.handle
}

func schedulerPreparePark() int32 {
	gcSafepoint()
	task := lookupNativeTask(schedulerCurrentTaskPtr())
	return nativeTaskPreparePark(task)
}

func schedulerCancelPark() {
	nativeTaskCancelPark(lookupNativeTask(schedulerCurrentTaskPtr()))
}

func schedulerWake(raw unsafe.Pointer) int32 {
	return int32(schedulerWakeTask(uintptr(raw)))
}
