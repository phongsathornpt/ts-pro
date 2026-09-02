package main

/*
#include <stdint.h>
typedef void (*tsnative_task_entry_fn)(void *, void *);
static void tsnative_go_call_task(uintptr_t entry, void *state, void *result) {
    ((tsnative_task_entry_fn)entry)(state, result);
}
*/
import "C"

import (
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
)

const (
	nativeTaskRunnable int32 = 1 + iota
	nativeTaskRunning
	nativeTaskWaiting
	nativeTaskDone
	nativeTaskCancelled
	nativeTaskFailed
)

const (
	nativeTaskResultVoid int32 = iota
	nativeTaskResultF64
	nativeTaskResultBool
	nativeTaskResultRef
)

const nativeTaskInitialBudget = 256

type nativeTaskCompletionWaiter struct {
	task       uintptr
	out        unsafe.Pointer
	consume    bool
	statusOnly bool
}

type nativeTask struct {
	handle unsafe.Pointer
	entry  unsafe.Pointer
	state  unsafe.Pointer
	id     uint64
	kind   int32

	stateRoot   unsafe.Pointer
	resultRoot  unsafe.Pointer
	contextRoot unsafe.Pointer
	failureRoot unsafe.Pointer

	context    unsafe.Pointer
	failureRef unsafe.Pointer
	resultF64  float64
	resultBool uint8
	resultRef  unsafe.Pointer

	status           atomic.Int32
	parkRequested    atomic.Int32
	wakeRequested    atomic.Int32
	cancelRequested  atomic.Int32
	failureRequested atomic.Int32
	budgetRemaining  atomic.Uint32
	refs             atomic.Int32

	completionMu sync.Mutex
}

var nativeTasks = struct {
	sync.RWMutex
	byID map[uintptr]*nativeTask
}{byID: map[uintptr]*nativeTask{}}

var nativeTaskCompletions = struct {
	sync.Mutex
	byTask map[uintptr][]nativeTaskCompletionWaiter
}{byTask: map[uintptr][]nativeTaskCompletionWaiter{}}

func appendNativeTaskCompletion(task uintptr, completion nativeTaskCompletionWaiter) bool {
	nativeTaskCompletions.Lock()
	defer nativeTaskCompletions.Unlock()
	for _, existing := range nativeTaskCompletions.byTask[task] {
		if existing.task == completion.task {
			return false
		}
	}
	nativeTaskCompletions.byTask[task] = append(nativeTaskCompletions.byTask[task], completion)
	return true
}

func takeNativeTaskCompletions(task uintptr) []nativeTaskCompletionWaiter {
	nativeTaskCompletions.Lock()
	defer nativeTaskCompletions.Unlock()
	items := nativeTaskCompletions.byTask[task]
	delete(nativeTaskCompletions.byTask, task)
	return items
}

func clearNativeTaskCompletions(task uintptr) {
	nativeTaskCompletions.Lock()
	delete(nativeTaskCompletions.byTask, task)
	nativeTaskCompletions.Unlock()
}

func newNativeTaskHandle() unsafe.Pointer {
	return allocNativeHandle()
}

func lookupNativeTask(handle uintptr) *nativeTask {
	if handle == 0 {
		return nil
	}
	nativeTasks.RLock()
	task := nativeTasks.byID[handle]
	nativeTasks.RUnlock()
	return task
}

func nativeTaskKey(task *nativeTask) uintptr {
	if task == nil || task.handle == nil {
		return 0
	}
	return uintptr(task.handle)
}

func registerNativeTask(task *nativeTask) {
	nativeTasks.Lock()
	nativeTasks.byID[nativeTaskKey(task)] = task
	nativeTasks.Unlock()
}

func unregisterNativeTask(handle unsafe.Pointer) {
	if handle == nil {
		return
	}
	nativeTasks.Lock()
	delete(nativeTasks.byID, uintptr(handle))
	nativeTasks.Unlock()
	freeNativeHandle(handle)
}

func nativeTaskResultSlot(task *nativeTask) unsafe.Pointer {
	switch task.kind {
	case nativeTaskResultF64:
		return unsafe.Pointer(&task.resultF64)
	case nativeTaskResultBool:
		return unsafe.Pointer(&task.resultBool)
	default:
		return unsafe.Pointer(&task.resultRef)
	}
}

func allocateNativeTask(state unsafe.Pointer, kind int32) *nativeTask {
	task := &nativeTask{handle: newNativeTaskHandle(), state: state, kind: kind}
	if task.handle == nil {
		return nil
	}
	task.status.Store(nativeTaskRunnable)
	task.budgetRemaining.Store(nativeTaskInitialBudget)
	task.refs.Store(1)
	if parent := lookupNativeTask(schedulerCurrentTaskPtr()); parent != nil {
		task.context = parent.context
	}
	registerNativeTask(task)

	task.contextRoot = tsnative_gc_root_register(unsafe.Pointer(&task.context))
	if task.contextRoot == nil {
		destroyNativeTaskStorage(task)
		return nil
	}
	if kind == nativeTaskResultRef {
		task.resultRoot = tsnative_gc_root_register(unsafe.Pointer(&task.resultRef))
		if task.resultRoot == nil {
			destroyNativeTaskStorage(task)
			return nil
		}
	}
	if state != nil {
		task.stateRoot = tsnative_gc_root_register(unsafe.Pointer(&task.state))
		if task.stateRoot == nil {
			destroyNativeTaskStorage(task)
			return nil
		}
	}
	task.failureRoot = tsnative_gc_root_register(unsafe.Pointer(&task.failureRef))
	if task.failureRoot == nil {
		destroyNativeTaskStorage(task)
		return nil
	}
	return task
}

func retainNativeTaskRef(task *nativeTask) bool {
	if task == nil {
		return false
	}
	for {
		refs := task.refs.Load()
		if refs <= 0 {
			return false
		}
		if task.refs.CompareAndSwap(refs, refs+1) {
			return true
		}
	}
}

func releaseNativeTaskRef(task *nativeTask) {
	if task == nil {
		return
	}
	refs := task.refs.Add(-1)
	if refs < 0 {
		nativeAbort("task handle reference count underflow")
	}
	if refs == 0 {
		destroyNativeTaskStorage(task)
	}
}

func createNativeTask(entry, state unsafe.Pointer, kind int32) *nativeTask {
	if entry == nil {
		return nil
	}
	task := allocateNativeTask(state, kind)
	if task != nil {
		task.entry = entry
	}
	return task
}

func destroyNativeTaskStorage(task *nativeTask) {
	if task == nil {
		return
	}
	clearNativeTaskCompletions(nativeTaskKey(task))
	unregisterNativeTask(task.handle)
	task.handle = nil
	if task.stateRoot != nil {
		tsnative_gc_root_unregister(task.stateRoot)
		task.stateRoot = nil
	}
	if task.resultRoot != nil {
		tsnative_gc_root_unregister(task.resultRoot)
		task.resultRoot = nil
	}
	if task.contextRoot != nil {
		tsnative_gc_root_unregister(task.contextRoot)
		task.contextRoot = nil
	}
	if task.failureRoot != nil {
		tsnative_gc_root_unregister(task.failureRoot)
		task.failureRoot = nil
	}
}

func settledNativePromise(kind int32, status int32, value unsafe.Pointer, f64 float64, boolean uint8) unsafe.Pointer {
	task := allocateNativeTask(nil, kind)
	if task == nil {
		nativeAbort("promise allocation failed")
	}
	switch kind {
	case nativeTaskResultF64:
		task.resultF64 = f64
	case nativeTaskResultBool:
		task.resultBool = boolean
	case nativeTaskResultRef:
		task.resultRef = value
	}
	if status == nativeTaskFailed {
		task.failureRef = value
		task.failureRequested.Store(1)
	}
	task.status.Store(status)
	return task.handle
}

//export tsnative_promise_resolve_f64
func tsnative_promise_resolve_f64(value C.double) unsafe.Pointer {
	return settledNativePromise(nativeTaskResultF64, nativeTaskDone, nil, float64(value), 0)
}

//export tsnative_promise_resolve_bool
func tsnative_promise_resolve_bool(value C.uint8_t) unsafe.Pointer {
	return settledNativePromise(nativeTaskResultBool, nativeTaskDone, nil, 0, uint8(value))
}

//export tsnative_promise_resolve_ref
func tsnative_promise_resolve_ref(value unsafe.Pointer) unsafe.Pointer {
	return settledNativePromise(nativeTaskResultRef, nativeTaskDone, value, 0, 0)
}

//export tsnative_promise_thenable_require_settled
func tsnative_promise_thenable_require_settled(raw unsafe.Pointer) unsafe.Pointer {
	if raw == nil {
		nativeAbort("asynchronous or unresolved thenable is not supported yet")
	}
	return raw
}

//export tsnative_promise_reject
func tsnative_promise_reject(reason unsafe.Pointer, kind C.int) unsafe.Pointer {
	if int32(kind) < nativeTaskResultF64 || int32(kind) > nativeTaskResultRef {
		nativeAbort("invalid Promise.reject result kind")
	}
	return settledNativePromise(int32(kind), nativeTaskFailed, reason, 0, 0)
}

func spawnNativeTask(kind int32, entry, state unsafe.Pointer) unsafe.Pointer {
	if tsnative_scheduler_init() != 0 {
		return nil
	}
	task := createNativeTask(entry, state, kind)
	if task == nil {
		return nil
	}
	if tsnative_scheduler_submit(task.handle) != 0 {
		destroyNativeTaskStorage(task)
		return nil
	}
	return task.handle
}

func spawnNativeTaskOrAbort(kind int32, entry, state unsafe.Pointer) unsafe.Pointer {
	task := spawnNativeTask(kind, entry, state)
	if task == nil {
		nativeAbort("task spawn failed")
	}
	return task
}

//export tsnative_task_spawn
func tsnative_task_spawn(entry, state unsafe.Pointer) unsafe.Pointer {
	return spawnNativeTask(nativeTaskResultVoid, entry, state)
}

//export tsnative_task_spawn_f64
func tsnative_task_spawn_f64(entry, state unsafe.Pointer) unsafe.Pointer {
	return spawnNativeTask(nativeTaskResultF64, entry, state)
}

//export tsnative_task_spawn_bool
func tsnative_task_spawn_bool(entry, state unsafe.Pointer) unsafe.Pointer {
	return spawnNativeTask(nativeTaskResultBool, entry, state)
}

//export tsnative_task_spawn_ref
func tsnative_task_spawn_ref(entry, state unsafe.Pointer) unsafe.Pointer {
	return spawnNativeTask(nativeTaskResultRef, entry, state)
}

//export tsnative_task_spawn_or_abort
func tsnative_task_spawn_or_abort(entry, state unsafe.Pointer) unsafe.Pointer {
	return spawnNativeTaskOrAbort(nativeTaskResultVoid, entry, state)
}

//export tsnative_task_spawn_f64_or_abort
func tsnative_task_spawn_f64_or_abort(entry, state unsafe.Pointer) unsafe.Pointer {
	return spawnNativeTaskOrAbort(nativeTaskResultF64, entry, state)
}

//export tsnative_task_spawn_bool_or_abort
func tsnative_task_spawn_bool_or_abort(entry, state unsafe.Pointer) unsafe.Pointer {
	return spawnNativeTaskOrAbort(nativeTaskResultBool, entry, state)
}

//export tsnative_task_spawn_ref_or_abort
func tsnative_task_spawn_ref_or_abort(entry, state unsafe.Pointer) unsafe.Pointer {
	return spawnNativeTaskOrAbort(nativeTaskResultRef, entry, state)
}

func nativeTaskIsTerminal(task *nativeTask) bool {
	if task == nil {
		return true
	}
	status := task.status.Load()
	return status == nativeTaskDone || status == nativeTaskCancelled || status == nativeTaskFailed
}

func nativeTaskPreparePark(task *nativeTask) int32 {
	if task == nil {
		return -1
	}
	task.parkRequested.Store(1)
	return 0
}

func nativeTaskCancelPark(task *nativeTask) {
	if task == nil {
		return
	}
	task.parkRequested.Store(0)
	task.wakeRequested.Store(0)
}

func nativeTaskWake(task *nativeTask) int {
	if task == nil {
		return -1
	}
	status := task.status.Load()
	if status == nativeTaskWaiting {
		task.status.Store(nativeTaskRunnable)
		return 1
	}
	if status == nativeTaskRunning && task.parkRequested.Load() != 0 {
		task.wakeRequested.Store(1)
		return 0
	}
	if status == nativeTaskRunnable {
		return 0
	}
	return -1
}

type nativeTaskExecution struct {
	kind               int
	completionWaiters  []uintptr
	completionConsumes int
	aggregateWatchers  []nativePromiseAggregateWatcher
}

const (
	nativeTaskExecWaiting = iota
	nativeTaskExecRequeue
	nativeTaskExecTerminal
)

func transferNativeTaskResult(task *nativeTask, out unsafe.Pointer) {
	if task == nil || out == nil {
		return
	}
	switch task.kind {
	case nativeTaskResultF64:
		*(*float64)(out) = task.resultF64
	case nativeTaskResultBool:
		*(*uint8)(out) = task.resultBool
	case nativeTaskResultRef:
		tsnative_gc_handoff_begin()
		nativeGCStoreRefSlot(out, task.resultRef)
		tsnative_gc_handoff_end()
	}
}

func executeNativeTaskOnce(task *nativeTask) nativeTaskExecution {
	execution := nativeTaskExecution{kind: nativeTaskExecTerminal}
	if task == nil {
		return execution
	}
	task.status.Store(nativeTaskRunning)
	task.parkRequested.Store(0)
	task.wakeRequested.Store(0)
	if task.failureRequested.Load() == 0 {
		C.tsnative_go_call_task(C.uintptr_t(uintptr(task.entry)), task.state, nativeTaskResultSlot(task))
	}
	if task.parkRequested.Load() != 0 {
		if task.wakeRequested.Swap(0) != 0 {
			task.status.Store(nativeTaskRunnable)
			execution.kind = nativeTaskExecRequeue
		} else {
			task.status.Store(nativeTaskWaiting)
			execution.kind = nativeTaskExecWaiting
		}
		return execution
	}

	task.completionMu.Lock()
	failed := task.failureRequested.Load() != 0
	if failed {
		task.status.Store(nativeTaskFailed)
	} else {
		task.status.Store(nativeTaskDone)
	}
	execution.aggregateWatchers = takeNativePromiseAggregateWatchers(nativeTaskKey(task))
	completions := takeNativeTaskCompletions(nativeTaskKey(task))
	for _, completion := range completions {
		waiter := lookupNativeTask(completion.task)
		if waiter == nil {
			continue
		}
		execution.completionWaiters = append(execution.completionWaiters, completion.task)
		if completion.consume {
			execution.completionConsumes++
		}
		if completion.statusOnly && completion.out != nil {
			if failed {
				*(*uint8)(completion.out) = 0
			} else {
				*(*uint8)(completion.out) = 1
			}
		} else if failed {
			waiter.failureRef = task.failureRef
			waiter.failureRequested.Store(1)
		} else if completion.out != nil {
			transferNativeTaskResult(task, completion.out)
		}
	}
	task.completionMu.Unlock()
	return execution
}

//export tsnative_task_join
func tsnative_task_join(raw unsafe.Pointer) int32 {
	if raw == nil {
		return -1
	}
	return tsnative_scheduler_wait(raw)
}

//export tsnative_task_get_failure
func tsnative_task_get_failure(raw, out unsafe.Pointer) int32 {
	task := lookupNativeTask(uintptr(raw))
	if task == nil || out == nil {
		return -1
	}
	nativeGCStoreRefSlot(out, nil)
	_ = tsnative_scheduler_wait(raw)
	task.completionMu.Lock()
	defer task.completionMu.Unlock()
	status := task.status.Load()
	if status == nativeTaskFailed && task.failureRef != nil {
		nativeGCStoreRefSlot(out, task.failureRef)
		return 1
	}
	if status == nativeTaskDone {
		return 0
	}
	return -1
}

//export tsnative_task_get_status
func tsnative_task_get_status(raw unsafe.Pointer) int32 {
	return tsnative_scheduler_task_status(raw)
}

//export tsnative_task_wait_status
func tsnative_task_wait_status(raw unsafe.Pointer) uint8 {
	if raw == nil {
		return 0
	}
	if tsnative_scheduler_wait(raw) == 0 {
		return 1
	}
	return 0
}

//export tsnative_task_failure_ref
func tsnative_task_failure_ref(raw unsafe.Pointer) unsafe.Pointer {
	task := lookupNativeTask(uintptr(raw))
	if task == nil {
		return nil
	}
	_ = tsnative_scheduler_wait(raw)
	if task.status.Load() == nativeTaskFailed && task.failureRef != nil {
		return task.failureRef
	}
	return nil
}

func awaitNativeTask(raw unsafe.Pointer, kind int32, out unsafe.Pointer, statusOnly bool, consume bool) int32 {
	task := lookupNativeTask(uintptr(raw))
	waiter := lookupNativeTask(schedulerCurrentTaskPtr())
	if task == nil || waiter == nil || waiter == task {
		return -1
	}
	if !statusOnly && kind >= 0 && (out == nil || task.kind != kind) {
		return -1
	}
	task.completionMu.Lock()
	status := task.status.Load()
	if nativeTaskIsTerminal(task) {
		if statusOnly && out != nil {
			if status == nativeTaskDone {
				*(*uint8)(out) = 1
			} else {
				*(*uint8)(out) = 0
			}
		} else if status == nativeTaskDone && out != nil {
			transferNativeTaskResult(task, out)
		} else if status == nativeTaskFailed {
			waiter.failureRef = task.failureRef
			waiter.failureRequested.Store(1)
		}
		task.completionMu.Unlock()
		if consume {
			releaseNativeTaskRef(task)
		}
		if status == nativeTaskDone || statusOnly {
			return 1
		}
		return -1
	}
	waiterKey := nativeTaskKey(waiter)
	if tsnative_scheduler_prepare_park() != 0 {
		task.completionMu.Unlock()
		return -1
	}
	if !appendNativeTaskCompletion(nativeTaskKey(task), nativeTaskCompletionWaiter{
		task: waiterKey, out: out, consume: consume, statusOnly: statusOnly,
	}) {
		tsnative_scheduler_cancel_park()
		task.completionMu.Unlock()
		return -1
	}
	task.completionMu.Unlock()
	return 0
}

//export tsnative_task_await_task
func tsnative_task_await_task(raw unsafe.Pointer) int32 {
	return awaitNativeTask(raw, -1, nil, false, false)
}

//export tsnative_task_await_task_consume
func tsnative_task_await_task_consume(raw unsafe.Pointer) int32 {
	return awaitNativeTask(raw, -1, nil, false, true)
}

//export tsnative_task_await_status_task
func tsnative_task_await_status_task(raw, out unsafe.Pointer) int32 {
	return awaitNativeTask(raw, -1, out, true, false)
}

//export tsnative_task_await_f64_task
func tsnative_task_await_f64_task(raw, out unsafe.Pointer) int32 {
	return awaitNativeTask(raw, nativeTaskResultF64, out, false, true)
}

//export tsnative_task_await_f64_shared
func tsnative_task_await_f64_shared(raw, out unsafe.Pointer) int32 {
	return awaitNativeTask(raw, nativeTaskResultF64, out, false, false)
}

//export tsnative_task_await_bool_task
func tsnative_task_await_bool_task(raw, out unsafe.Pointer) int32 {
	return awaitNativeTask(raw, nativeTaskResultBool, out, false, true)
}

//export tsnative_task_await_bool_shared
func tsnative_task_await_bool_shared(raw, out unsafe.Pointer) int32 {
	return awaitNativeTask(raw, nativeTaskResultBool, out, false, false)
}

//export tsnative_task_await_ref_task
func tsnative_task_await_ref_task(raw, out unsafe.Pointer) int32 {
	return awaitNativeTask(raw, nativeTaskResultRef, out, false, true)
}

//export tsnative_task_await_ref_shared
func tsnative_task_await_ref_shared(raw, out unsafe.Pointer) int32 {
	return awaitNativeTask(raw, nativeTaskResultRef, out, false, false)
}

//export tsnative_task_retain
func tsnative_task_retain(raw unsafe.Pointer) int32 {
	task := lookupNativeTask(uintptr(raw))
	if !retainNativeTaskRef(task) {
		return -1
	}
	return 0
}

//export tsnative_task_release
func tsnative_task_release(raw unsafe.Pointer) {
	task := lookupNativeTask(uintptr(raw))
	if task == nil {
		return
	}
	for {
		refs := task.refs.Load()
		if refs <= 0 {
			return
		}
		if refs > 1 {
			if task.refs.CompareAndSwap(refs, refs-1) {
				return
			}
			continue
		}
		_ = tsnative_scheduler_wait(raw)
		releaseNativeTaskRef(task)
		return
	}
}

func abortNativeTaskFailure(task *nativeTask) {
	if task != nil && task.failureRef != nil {
		tsnative_console_log_jsvalue(task.failureRef)
	}
	nativeAbortSignal()
}

//export tsnative_task_join_f64
func tsnative_task_join_f64(raw unsafe.Pointer) float64 {
	task := lookupNativeTask(uintptr(raw))
	if task == nil || task.kind != nativeTaskResultF64 {
		nativeAbort("invalid f64 task join")
	}
	if tsnative_scheduler_wait(raw) != 0 {
		abortNativeTaskFailure(task)
	}
	return task.resultF64
}

//export tsnative_task_join_bool
func tsnative_task_join_bool(raw unsafe.Pointer) uint8 {
	task := lookupNativeTask(uintptr(raw))
	if task == nil || task.kind != nativeTaskResultBool {
		nativeAbort("invalid bool task join")
	}
	if tsnative_scheduler_wait(raw) != 0 {
		abortNativeTaskFailure(task)
	}
	return task.resultBool
}

//export tsnative_task_join_ref
func tsnative_task_join_ref(raw unsafe.Pointer) unsafe.Pointer {
	task := lookupNativeTask(uintptr(raw))
	if task == nil || task.kind != nativeTaskResultRef {
		nativeAbort("invalid ref task join")
	}
	if tsnative_scheduler_wait(raw) != 0 {
		abortNativeTaskFailure(task)
	}
	return task.resultRef
}

//export tsnative_task_join_release
func tsnative_task_join_release(raw unsafe.Pointer) {
	task := lookupNativeTask(uintptr(raw))
	if task == nil || task.kind != nativeTaskResultVoid {
		nativeAbort("invalid void task join")
	}
	if tsnative_scheduler_wait(raw) != 0 {
		abortNativeTaskFailure(task)
	}
	releaseNativeTaskRef(task)
}

//export tsnative_task_join_f64_release
func tsnative_task_join_f64_release(raw unsafe.Pointer) float64 {
	task := lookupNativeTask(uintptr(raw))
	if task == nil || task.kind != nativeTaskResultF64 {
		nativeAbort("invalid f64 task join")
	}
	if tsnative_scheduler_wait(raw) != 0 {
		abortNativeTaskFailure(task)
	}
	result := task.resultF64
	releaseNativeTaskRef(task)
	return result
}

//export tsnative_task_join_bool_release
func tsnative_task_join_bool_release(raw unsafe.Pointer) uint8 {
	task := lookupNativeTask(uintptr(raw))
	if task == nil || task.kind != nativeTaskResultBool {
		nativeAbort("invalid bool task join")
	}
	if tsnative_scheduler_wait(raw) != 0 {
		abortNativeTaskFailure(task)
	}
	result := task.resultBool
	releaseNativeTaskRef(task)
	return result
}

//export tsnative_task_join_ref_release
func tsnative_task_join_ref_release(raw unsafe.Pointer) unsafe.Pointer {
	task := lookupNativeTask(uintptr(raw))
	if task == nil || task.kind != nativeTaskResultRef {
		nativeAbort("invalid ref task join")
	}
	if tsnative_scheduler_wait(raw) != 0 {
		abortNativeTaskFailure(task)
	}
	result := task.resultRef
	releaseNativeTaskRef(task)
	return result
}

//export tsnative_task_cancel
func tsnative_task_cancel(raw unsafe.Pointer) int32 {
	task := lookupNativeTask(uintptr(raw))
	if task == nil {
		return -1
	}
	task.cancelRequested.Store(1)
	return 0
}

//export tsnative_task_is_cancelled
func tsnative_task_is_cancelled() int32 {
	task := lookupNativeTask(schedulerCurrentTaskPtr())
	if task != nil && task.cancelRequested.Load() != 0 {
		return 1
	}
	return 0
}

//export tsnative_task_fail_current
func tsnative_task_fail_current(errorRef unsafe.Pointer) {
	task := lookupNativeTask(schedulerCurrentTaskPtr())
	if task == nil {
		nativeAbort("task failure outside task")
	}
	task.failureRef = errorRef
	task.failureRequested.Store(1)
}

//export tsnative_task_budget_poll_task
func tsnative_task_budget_poll_task() int32 {
	tsnative_gc_safepoint()
	task := lookupNativeTask(schedulerCurrentTaskPtr())
	if task == nil {
		return 1
	}
	previous := task.budgetRemaining.Add(^uint32(0)) + 1
	if previous > 1 {
		return 1
	}
	task.budgetRemaining.Store(nativeTaskInitialBudget)
	if tsnative_task_yield_task() == 0 {
		return 0
	}
	return -1
}

//export tsnative_task_yield_task
func tsnative_task_yield_task() int32 {
	if tsnative_scheduler_prepare_park() != 0 {
		return -1
	}
	current := schedulerCurrentTaskPtr()
	if current == 0 || schedulerWakeTask(current) != 0 {
		tsnative_scheduler_cancel_park()
		return -1
	}
	return 0
}

//export tsnative_task_yield
func tsnative_task_yield() {
	runtime.Gosched()
}
