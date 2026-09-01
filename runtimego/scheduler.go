package main

/*
#cgo CFLAGS: -I../runtime/concurrency
#include <stdint.h>
#include "../runtime/concurrency/task_internal.h"

static _Thread_local long long tsnative_go_worker_tls = -1;
static _Thread_local tsnative_task *tsnative_go_task_tls = NULL;

static void tsnative_go_set_worker(long long index) { tsnative_go_worker_tls = index; }
static long long tsnative_go_get_worker(void) { return tsnative_go_worker_tls; }
static void tsnative_go_set_current_task(tsnative_task *task) { tsnative_go_task_tls = task; }
static tsnative_task *tsnative_go_get_current_task(void) { return tsnative_go_task_tls; }

extern void *tsnative_scheduler_current_task(void);
extern int tsnative_scheduler_prepare_park(void);
extern void tsnative_scheduler_cancel_park(void);
extern int tsnative_scheduler_wake(void *task);
extern int tsnative_scheduler_help_once(void);

static uintptr_t tsnative_go_addr_current(void) { return (uintptr_t)&tsnative_scheduler_current_task; }
static uintptr_t tsnative_go_addr_prepare(void) { return (uintptr_t)&tsnative_scheduler_prepare_park; }
static uintptr_t tsnative_go_addr_cancel(void) { return (uintptr_t)&tsnative_scheduler_cancel_park; }
static uintptr_t tsnative_go_addr_wake(void) { return (uintptr_t)&tsnative_scheduler_wake; }
static uintptr_t tsnative_go_addr_help(void) { return (uintptr_t)&tsnative_scheduler_help_once; }
*/
import "C"

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
	workers  []*nativeSchedulerWorker
	inject   []uintptr
	started  bool
	stopping bool
	maxTasks int64
	nextID   uint64
	wg       sync.WaitGroup

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
	return s
}()

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

func schedulerWorkerIndex() int {
	return int(C.tsnative_go_get_worker())
}

func schedulerCurrentTaskPtr() uintptr {
	return uintptr(unsafe.Pointer(C.tsnative_go_get_current_task()))
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

func schedulerWakeTask(task uintptr) int {
	if task == 0 {
		return -1
	}
	goScheduler.mu.Lock()
	wake := int(C.tsnative_task_wake_internal((*C.tsnative_task)(unsafe.Pointer(task))))
	if wake == 1 {
		goScheduler.inject = append(goScheduler.inject, task)
		goScheduler.runnable.Add(1)
		goScheduler.wake.Signal()
	}
	goScheduler.mu.Unlock()
	if wake < 0 {
		return -1
	}
	return 0
}

func schedulerExecuteTask(task uintptr) {
	ptr := (*C.tsnative_task)(unsafe.Pointer(task))
	previous := C.tsnative_go_get_current_task()
	C.tsnative_go_set_current_task(ptr)
	execution := C.tsnative_task_execute_once_internal(ptr)
	C.tsnative_go_set_current_task(previous)

	kind := int(execution.kind)
	goScheduler.mu.Lock()
	if kind == int(C.TSNATIVE_TASK_EXEC_REQUEUE) {
		goScheduler.inject = append(goScheduler.inject, task)
		goScheduler.runnable.Add(1)
		goScheduler.wake.Signal()
	} else if kind == int(C.TSNATIVE_TASK_EXEC_TERMINAL) {
		goScheduler.active.Add(-1)
		goScheduler.completed.Add(1)
	}
	goScheduler.done.Broadcast()
	goScheduler.mu.Unlock()

	if kind == int(C.TSNATIVE_TASK_EXEC_TERMINAL) {
		C.tsnative_task_notify_completed_internal(ptr)
		if execution.completion_waiter != nil {
			_ = schedulerWakeTask(uintptr(unsafe.Pointer(execution.completion_waiter)))
		}
		C.tsnative_task_destroy_completed_internal(ptr, execution.completion_consume)
	}
}

func schedulerWorkerLoop(worker *nativeSchedulerWorker) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	C.tsnative_go_set_worker(C.longlong(worker.index))
	defer C.tsnative_go_set_worker(-1)
	defer C.tsnative_go_set_current_task(nil)

	for {
		if task := schedulerTakeWork(worker); task != 0 {
			schedulerExecuteTask(task)
			continue
		}
		goScheduler.mu.Lock()
		if goScheduler.stopping && goScheduler.runnable.Load() == 0 {
			goScheduler.mu.Unlock()
			return
		}
		for !goScheduler.stopping && len(goScheduler.inject) == 0 && goScheduler.runnable.Load() == 0 {
			goScheduler.parks.Add(1)
			goScheduler.wake.Wait()
			goScheduler.wakeups.Add(1)
		}
		goScheduler.mu.Unlock()
	}
}

func bindGoSchedulerHooks() {
	current := C.uintptr_t(C.tsnative_go_addr_current())
	prepare := C.uintptr_t(C.tsnative_go_addr_prepare())
	cancel := C.uintptr_t(C.tsnative_go_addr_cancel())
	wake := C.uintptr_t(C.tsnative_go_addr_wake())
	help := C.uintptr_t(C.tsnative_go_addr_help())
	tsnative_timer_bind_scheduler(current, prepare, cancel, wake, help)
	tsnative_blocking_bind_scheduler(current, prepare, cancel, wake, help)
	tsnative_channel_bind_scheduler(current, prepare, cancel, wake, help)
}

//export tsnative_scheduler_init
func tsnative_scheduler_init() C.int {
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
	resetSchedulerMetrics()
	workers := append([]*nativeSchedulerWorker(nil), goScheduler.workers...)
	goScheduler.mu.Unlock()

	bindGoSchedulerHooks()
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
func tsnative_scheduler_submit(raw unsafe.Pointer) C.int {
	if raw == nil {
		return -1
	}
	task := uintptr(raw)
	goScheduler.mu.Lock()
	active := goScheduler.active.Load()
	if !goScheduler.started || goScheduler.stopping || active >= goScheduler.maxTasks {
		goScheduler.mu.Unlock()
		return -1
	}
	goScheduler.nextID++
	C.tsnative_task_mark_submitted_internal((*C.tsnative_task)(raw), C.uint64_t(goScheduler.nextID))
	active = goScheduler.active.Add(1)
	goScheduler.runnable.Add(1)
	goScheduler.spawned.Add(1)
	updateSchedulerPeak(active)
	workerIndex := schedulerWorkerIndex()
	if workerIndex >= 0 && workerIndex < len(goScheduler.workers) {
		goScheduler.workers[workerIndex].pushTail(task)
	} else {
		goScheduler.inject = append(goScheduler.inject, task)
	}
	goScheduler.wake.Signal()
	goScheduler.mu.Unlock()
	return 0
}

//export tsnative_scheduler_wait
func tsnative_scheduler_wait(raw unsafe.Pointer) C.int {
	if raw == nil {
		return -1
	}
	task := (*C.tsnative_task)(raw)
	for C.tsnative_task_is_terminal_internal(task) == 0 {
		workerIndex := schedulerWorkerIndex()
		if workerIndex >= 0 && workerIndex < len(goScheduler.workers) {
			if tsnative_scheduler_help_once() != 0 {
				continue
			}
		}
		goScheduler.mu.Lock()
		if C.tsnative_task_is_terminal_internal(task) == 0 {
			goScheduler.done.Wait()
		}
		goScheduler.mu.Unlock()
	}
	if C.tsnative_task_status_internal(task) == C.TSNATIVE_TASK_DONE {
		return 0
	}
	return -1
}

//export tsnative_scheduler_task_status
func tsnative_scheduler_task_status(raw unsafe.Pointer) C.int {
	if raw == nil {
		return C.int(C.TSNATIVE_TASK_FAILED)
	}
	return C.int(C.tsnative_task_status_internal((*C.tsnative_task)(raw)))
}

//export tsnative_scheduler_help_once
func tsnative_scheduler_help_once() C.int {
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
	goScheduler.mu.Unlock()
}

//export tsnative_scheduler_worker_count
func tsnative_scheduler_worker_count() C.size_t {
	goScheduler.mu.Lock()
	count := len(goScheduler.workers)
	started := goScheduler.started
	goScheduler.mu.Unlock()
	if !started {
		count = configuredSchedulerWorkers()
	}
	return C.size_t(count)
}

//export tsnative_scheduler_is_running
func tsnative_scheduler_is_running() C.int {
	goScheduler.mu.Lock()
	running := goScheduler.started && !goScheduler.stopping
	goScheduler.mu.Unlock()
	if running {
		return 1
	}
	return 0
}

//export tsnative_scheduler_active_tasks
func tsnative_scheduler_active_tasks() C.size_t { return C.size_t(goScheduler.active.Load()) }

//export tsnative_scheduler_peak_active_tasks
func tsnative_scheduler_peak_active_tasks() C.size_t { return C.size_t(goScheduler.peak.Load()) }

//export tsnative_scheduler_spawned_tasks
func tsnative_scheduler_spawned_tasks() C.uint64_t { return C.uint64_t(goScheduler.spawned.Load()) }

//export tsnative_scheduler_completed_tasks
func tsnative_scheduler_completed_tasks() C.uint64_t { return C.uint64_t(goScheduler.completed.Load()) }

//export tsnative_scheduler_steal_attempts
func tsnative_scheduler_steal_attempts() C.uint64_t { return C.uint64_t(goScheduler.stealTry.Load()) }

//export tsnative_scheduler_successful_steals
func tsnative_scheduler_successful_steals() C.uint64_t { return C.uint64_t(goScheduler.stealOK.Load()) }

//export tsnative_scheduler_worker_parks
func tsnative_scheduler_worker_parks() C.uint64_t { return C.uint64_t(goScheduler.parks.Load()) }

//export tsnative_scheduler_worker_wakeups
func tsnative_scheduler_worker_wakeups() C.uint64_t { return C.uint64_t(goScheduler.wakeups.Load()) }

//export tsnative_scheduler_current_task
func tsnative_scheduler_current_task() unsafe.Pointer {
	return unsafe.Pointer(schedulerCurrentTaskPtr())
}

//export tsnative_scheduler_prepare_park
func tsnative_scheduler_prepare_park() C.int {
	current := schedulerCurrentTaskPtr()
	if current == 0 {
		return -1
	}
	return C.tsnative_task_prepare_park_internal((*C.tsnative_task)(unsafe.Pointer(current)))
}

//export tsnative_scheduler_cancel_park
func tsnative_scheduler_cancel_park() {
	current := schedulerCurrentTaskPtr()
	if current != 0 {
		C.tsnative_task_cancel_park_internal((*C.tsnative_task)(unsafe.Pointer(current)))
	}
}

//export tsnative_scheduler_wake
func tsnative_scheduler_wake(raw unsafe.Pointer) C.int {
	return C.int(schedulerWakeTask(uintptr(raw)))
}
