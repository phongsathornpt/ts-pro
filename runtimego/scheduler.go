package main

/*
#include <stdint.h>
*/
import "C"

import (
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
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
	tid := syscall.Gettid()
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
	tid := syscall.Gettid()
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
	tid := syscall.Gettid()
	schedulerThreads.RLock()
	worker, ok := schedulerThreads.workers[tid]
	schedulerThreads.RUnlock()
	if !ok {
		return -1
	}
	return worker
}

func schedulerCurrentTaskPtr() uintptr {
	tid := syscall.Gettid()
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
	tsnative_gc_safepoint()

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

//export tsnative_scheduler_init
func tsnative_scheduler_init() int32 {
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

//export tsnative_scheduler_submit
func tsnative_scheduler_submit(raw unsafe.Pointer) int32 {
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

//export tsnative_scheduler_wait
func tsnative_scheduler_wait(raw unsafe.Pointer) int32 {
	task := lookupNativeTask(uintptr(raw))
	if task == nil {
		return -1
	}
	for !nativeTaskIsTerminal(task) {
		workerIndex := schedulerWorkerIndex()
		if workerIndex >= 0 && workerIndex < len(goScheduler.workers) && tsnative_scheduler_help_once() != 0 {
			continue
		}
		goScheduler.mu.Lock()
		if !nativeTaskIsTerminal(task) {
			goScheduler.done.Wait()
		}
		goScheduler.mu.Unlock()
	}
	tsnative_gc_safepoint()
	if task.status.Load() == nativeTaskDone {
		return 0
	}
	return -1
}

//export tsnative_scheduler_task_status
func tsnative_scheduler_task_status(raw unsafe.Pointer) int32 {
	task := lookupNativeTask(uintptr(raw))
	if task == nil {
		return nativeTaskFailed
	}
	return task.status.Load()
}

//export tsnative_scheduler_help_once
func tsnative_scheduler_help_once() int32 {
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

//export tsnative_scheduler_shutdown
func tsnative_scheduler_shutdown() {
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

//export tsnative_scheduler_worker_count
func tsnative_scheduler_worker_count() uintptr {
	goScheduler.mu.Lock()
	count := len(goScheduler.workers)
	started := goScheduler.started
	goScheduler.mu.Unlock()
	if !started {
		count = configuredSchedulerWorkers()
	}
	return uintptr(count)
}

//export tsnative_scheduler_is_running
func tsnative_scheduler_is_running() int32 {
	goScheduler.mu.Lock()
	running := goScheduler.started && !goScheduler.stopping
	goScheduler.mu.Unlock()
	if running {
		return 1
	}
	return 0
}

//export tsnative_scheduler_active_tasks
func tsnative_scheduler_active_tasks() uintptr { return uintptr(goScheduler.active.Load()) }

//export tsnative_scheduler_peak_active_tasks
func tsnative_scheduler_peak_active_tasks() uintptr { return uintptr(goScheduler.peak.Load()) }

//export tsnative_scheduler_spawned_tasks
func tsnative_scheduler_spawned_tasks() uint64 { return goScheduler.spawned.Load() }

//export tsnative_scheduler_completed_tasks
func tsnative_scheduler_completed_tasks() uint64 { return goScheduler.completed.Load() }

//export tsnative_scheduler_steal_attempts
func tsnative_scheduler_steal_attempts() uint64 { return goScheduler.stealTry.Load() }

//export tsnative_scheduler_successful_steals
func tsnative_scheduler_successful_steals() uint64 { return goScheduler.stealOK.Load() }

//export tsnative_scheduler_worker_parks
func tsnative_scheduler_worker_parks() uint64 { return goScheduler.parks.Load() }

//export tsnative_scheduler_worker_wakeups
func tsnative_scheduler_worker_wakeups() uint64 { return goScheduler.wakeups.Load() }

//export tsnative_scheduler_current_task
func tsnative_scheduler_current_task() unsafe.Pointer {
	task := lookupNativeTask(schedulerCurrentTaskPtr())
	if task == nil {
		return nil
	}
	return task.handle
}

//export tsnative_scheduler_prepare_park
func tsnative_scheduler_prepare_park() int32 {
	tsnative_gc_safepoint()
	task := lookupNativeTask(schedulerCurrentTaskPtr())
	return nativeTaskPreparePark(task)
}

//export tsnative_scheduler_cancel_park
func tsnative_scheduler_cancel_park() {
	nativeTaskCancelPark(lookupNativeTask(schedulerCurrentTaskPtr()))
}

//export tsnative_scheduler_wake
func tsnative_scheduler_wake(raw unsafe.Pointer) int32 {
	return int32(schedulerWakeTask(uintptr(raw)))
}
