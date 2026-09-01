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

type nativeTask struct {
	handle uintptr
	entry  uintptr
	state  uintptr
	id     uint64
	kind   int32

	stateRoot   unsafe.Pointer
	resultRoot  unsafe.Pointer
	contextRoot unsafe.Pointer
	failureRoot unsafe.Pointer

	context    uintptr
	failureRef uintptr
	resultF64  float64
	resultBool uint8
	resultRef  uintptr

	status           atomic.Int32
	parkRequested    atomic.Int32
	wakeRequested    atomic.Int32
	cancelRequested  atomic.Int32
	failureRequested atomic.Int32
	budgetRemaining  atomic.Uint32

	completionMu         sync.Mutex
	completionWaiter     uintptr
	completionOut        uintptr
	completionConsume    bool
	completionStatusOnly bool
	completionHandoff    bool
}

var nativeTasks = struct {
	sync.RWMutex
	next atomic.Uint64
	byID map[uintptr]*nativeTask
}{byID: map[uintptr]*nativeTask{}}

func newNativeTaskHandle() uintptr {
	return uintptr(nativeTasks.next.Add(1)<<4 | 1)
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

func registerNativeTask(task *nativeTask) {
	nativeTasks.Lock()
	nativeTasks.byID[task.handle] = task
	nativeTasks.Unlock()
}

func unregisterNativeTask(handle uintptr) {
	nativeTasks.Lock()
	delete(nativeTasks.byID, handle)
	nativeTasks.Unlock()
}

func nativeTaskResultSlot(task *nativeTask) uintptr {
	switch task.kind {
	case nativeTaskResultF64:
		return uintptr(unsafe.Pointer(&task.resultF64))
	case nativeTaskResultBool:
		return uintptr(unsafe.Pointer(&task.resultBool))
	case nativeTaskResultRef:
		return uintptr(unsafe.Pointer(&task.resultRef))
	default:
		return uintptr(unsafe.Pointer(&task.resultRef))
	}
}

func createNativeTask(entry, state uintptr, kind int32) *nativeTask {
	if entry == 0 {
		return nil
	}
	task := &nativeTask{handle: newNativeTaskHandle(), entry: entry, state: state, kind: kind}
	task.status.Store(nativeTaskRunnable)
	task.budgetRemaining.Store(nativeTaskInitialBudget)
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
	if state != 0 {
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

func destroyNativeTaskStorage(task *nativeTask) {
	if task == nil {
		return
	}
	unregisterNativeTask(task.handle)
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
	if task.completionHandoff {
		task.completionHandoff = false
		tsnative_gc_handoff_end()
	}
}

func spawnNativeTask(kind int32, entry, state uintptr) uintptr {
	if tsnative_scheduler_init() != 0 {
		return 0
	}
	task := createNativeTask(entry, state, kind)
	if task == nil {
		return 0
	}
	if tsnative_scheduler_submit(unsafe.Pointer(task.handle)) != 0 {
		destroyNativeTaskStorage(task)
		return 0
	}
	return task.handle
}

func spawnNativeTaskOrAbort(kind int32, entry, state uintptr) uintptr {
	task := spawnNativeTask(kind, entry, state)
	if task == 0 {
		nativeAbort("task spawn failed")
	}
	return task
}

//export tsnative_task_spawn
func tsnative_task_spawn(entry, state unsafe.Pointer) unsafe.Pointer {
	return unsafe.Pointer(spawnNativeTask(nativeTaskResultVoid, uintptr(entry), uintptr(state)))
}

//export tsnative_task_spawn_f64
func tsnative_task_spawn_f64(entry, state unsafe.Pointer) unsafe.Pointer {
	return unsafe.Pointer(spawnNativeTask(nativeTaskResultF64, uintptr(entry), uintptr(state)))
}

//export tsnative_task_spawn_bool
func tsnative_task_spawn_bool(entry, state unsafe.Pointer) unsafe.Pointer {
	return unsafe.Pointer(spawnNativeTask(nativeTaskResultBool, uintptr(entry), uintptr(state)))
}

//export tsnative_task_spawn_ref
func tsnative_task_spawn_ref(entry, state unsafe.Pointer) unsafe.Pointer {
	return unsafe.Pointer(spawnNativeTask(nativeTaskResultRef, uintptr(entry), uintptr(state)))
}

//export tsnative_task_spawn_or_abort
func tsnative_task_spawn_or_abort(entry, state unsafe.Pointer) unsafe.Pointer {
	return unsafe.Pointer(spawnNativeTaskOrAbort(nativeTaskResultVoid, uintptr(entry), uintptr(state)))
}

//export tsnative_task_spawn_f64_or_abort
func tsnative_task_spawn_f64_or_abort(entry, state unsafe.Pointer) unsafe.Pointer {
	return unsafe.Pointer(spawnNativeTaskOrAbort(nativeTaskResultF64, uintptr(entry), uintptr(state)))
}

//export tsnative_task_spawn_bool_or_abort
func tsnative_task_spawn_bool_or_abort(entry, state unsafe.Pointer) unsafe.Pointer {
	return unsafe.Pointer(spawnNativeTaskOrAbort(nativeTaskResultBool, uintptr(entry), uintptr(state)))
}

//export tsnative_task_spawn_ref_or_abort
func tsnative_task_spawn_ref_or_abort(entry, state unsafe.Pointer) unsafe.Pointer {
	return unsafe.Pointer(spawnNativeTaskOrAbort(nativeTaskResultRef, uintptr(entry), uintptr(state)))
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
	kind              int
	completionWaiter  uintptr
	completionConsume bool
}

const (
	nativeTaskExecWaiting = iota
	nativeTaskExecRequeue
	nativeTaskExecTerminal
)

func transferNativeTaskResult(task *nativeTask, out uintptr) {
	if task == nil || out == 0 {
		return
	}
	switch task.kind {
	case nativeTaskResultF64:
		*(*float64)(unsafe.Pointer(out)) = task.resultF64
	case nativeTaskResultBool:
		*(*uint8)(unsafe.Pointer(out)) = task.resultBool
	case nativeTaskResultRef:
		tsnative_gc_handoff_begin()
		*(*uintptr)(unsafe.Pointer(out)) = task.resultRef
		task.completionHandoff = true
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
		C.tsnative_go_call_task(C.uintptr_t(task.entry), unsafe.Pointer(task.state), unsafe.Pointer(nativeTaskResultSlot(task)))
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
	execution.completionWaiter = task.completionWaiter
	execution.completionConsume = task.completionConsume
	completionOut := task.completionOut
	statusOnly := task.completionStatusOnly
	task.completionWaiter = 0
	task.completionOut = 0
	task.completionConsume = false
	task.completionStatusOnly = false
	if waiter := lookupNativeTask(execution.completionWaiter); waiter != nil {
		if statusOnly && completionOut != 0 {
			if failed {
				*(*uint8)(unsafe.Pointer(completionOut)) = 0
			} else {
				*(*uint8)(unsafe.Pointer(completionOut)) = 1
			}
		} else if failed {
			waiter.failureRef = task.failureRef
			waiter.failureRequested.Store(1)
		} else if completionOut != 0 {
			transferNativeTaskResult(task, completionOut)
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
	*(*unsafe.Pointer)(out) = nil
	_ = tsnative_scheduler_wait(raw)
	task.completionMu.Lock()
	defer task.completionMu.Unlock()
	status := task.status.Load()
	if status == nativeTaskFailed && task.failureRef != 0 {
		*(*unsafe.Pointer)(out) = unsafe.Pointer(task.failureRef)
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
	if task.status.Load() == nativeTaskFailed && task.failureRef != 0 {
		return unsafe.Pointer(task.failureRef)
	}
	return nil
}

func awaitNativeTask(raw unsafe.Pointer, kind int32, out uintptr, statusOnly bool, consume bool) int32 {
	task := lookupNativeTask(uintptr(raw))
	waiter := lookupNativeTask(schedulerCurrentTaskPtr())
	if task == nil || waiter == nil || waiter == task {
		return -1
	}
	if !statusOnly && kind >= 0 && (out == 0 || task.kind != kind) {
		return -1
	}
	task.completionMu.Lock()
	status := task.status.Load()
	if nativeTaskIsTerminal(task) {
		if statusOnly && out != 0 {
			if status == nativeTaskDone {
				*(*uint8)(unsafe.Pointer(out)) = 1
			} else {
				*(*uint8)(unsafe.Pointer(out)) = 0
			}
		} else if status == nativeTaskDone && out != 0 {
			transferNativeTaskResult(task, out)
		} else if status == nativeTaskFailed {
			waiter.failureRef = task.failureRef
			waiter.failureRequested.Store(1)
		}
		task.completionMu.Unlock()
		if consume {
			destroyNativeTaskStorage(task)
		}
		if status == nativeTaskDone || statusOnly {
			return 1
		}
		return -1
	}
	if task.completionWaiter != 0 || tsnative_scheduler_prepare_park() != 0 {
		task.completionMu.Unlock()
		return -1
	}
	task.completionWaiter = waiter.handle
	task.completionOut = out
	task.completionConsume = consume
	task.completionStatusOnly = statusOnly
	task.completionMu.Unlock()
	return 0
}

//export tsnative_task_await_task
func tsnative_task_await_task(raw unsafe.Pointer) int32 {
	return awaitNativeTask(raw, -1, 0, false, false)
}

//export tsnative_task_await_status_task
func tsnative_task_await_status_task(raw, out unsafe.Pointer) int32 {
	return awaitNativeTask(raw, -1, uintptr(out), true, false)
}

//export tsnative_task_await_f64_task
func tsnative_task_await_f64_task(raw, out unsafe.Pointer) int32 {
	return awaitNativeTask(raw, nativeTaskResultF64, uintptr(out), false, true)
}

//export tsnative_task_await_bool_task
func tsnative_task_await_bool_task(raw, out unsafe.Pointer) int32 {
	return awaitNativeTask(raw, nativeTaskResultBool, uintptr(out), false, true)
}

//export tsnative_task_await_ref_task
func tsnative_task_await_ref_task(raw, out unsafe.Pointer) int32 {
	return awaitNativeTask(raw, nativeTaskResultRef, uintptr(out), false, true)
}

//export tsnative_task_release
func tsnative_task_release(raw unsafe.Pointer) {
	task := lookupNativeTask(uintptr(raw))
	if task == nil {
		return
	}
	_ = tsnative_scheduler_wait(raw)
	destroyNativeTaskStorage(task)
}

func abortNativeTaskFailure(task *nativeTask) {
	if task != nil && task.failureRef != 0 {
		tsnative_console_log_jsvalue(unsafe.Pointer(task.failureRef))
	}
	nativeAbortSignal()
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
	destroyNativeTaskStorage(task)
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
	destroyNativeTaskStorage(task)
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
	destroyNativeTaskStorage(task)
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
	destroyNativeTaskStorage(task)
	return unsafe.Pointer(result)
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
	task.failureRef = uintptr(errorRef)
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
	if current == 0 || tsnative_scheduler_wake(unsafe.Pointer(current)) != 0 {
		tsnative_scheduler_cancel_park()
		return -1
	}
	return 0
}

//export tsnative_task_yield
func tsnative_task_yield() {
	runtime.Gosched()
}
