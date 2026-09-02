package main

/*
#include <stdint.h>
*/
import "C"

import (
	"sync"
	"unsafe"
)

type nativePromiseAggregateKind uint8

const (
	nativePromiseAggregateAllF64 nativePromiseAggregateKind = iota + 1
	nativePromiseAggregateRaceF64
	nativePromiseAggregateAllBool
	nativePromiseAggregateRaceBool
	nativePromiseAggregateAllRef
	nativePromiseAggregateRaceRef
)

type nativePromiseAggregateWatcher struct {
	state *nativePromiseAggregateState
	index int
}

var nativePromiseAggregateWatchers = struct {
	sync.Mutex
	byTask map[uintptr][]nativePromiseAggregateWatcher
}{byTask: map[uintptr][]nativePromiseAggregateWatcher{}}

type nativePromiseAggregateState struct {
	sync.Mutex
	kind      nativePromiseAggregateKind
	aggregate *nativeTask
	tasks     []*nativeTask
	observed  []bool
	remaining int
	settled   bool
	f64       []float64
	bools     []uint8
	refs      []unsafe.Pointer
}

func retainNativePromiseInputs(raw unsafe.Pointer, count uint64, kind int32) []*nativeTask {
	if count == 0 {
		return nil
	}
	if raw == nil || count > uint64(^uint(0)>>1) {
		nativeAbort("invalid Promise aggregate input")
	}
	handles := unsafe.Slice((*unsafe.Pointer)(raw), int(count))
	tasks := make([]*nativeTask, 0, len(handles))
	for _, handle := range handles {
		task := lookupNativeTask(uintptr(handle))
		if task == nil || task.kind != kind || !retainNativeTaskRef(task) {
			for _, held := range tasks {
				releaseNativeTaskRef(held)
			}
			nativeAbort("invalid Promise aggregate task")
		}
		tasks = append(tasks, task)
	}
	return tasks
}

func watchNativePromiseAggregate(task *nativeTask, watcher nativePromiseAggregateWatcher) bool {
	if task == nil {
		return false
	}
	task.completionMu.Lock()
	if nativeTaskIsTerminal(task) {
		task.completionMu.Unlock()
		return false
	}
	nativePromiseAggregateWatchers.Lock()
	key := nativeTaskKey(task)
	nativePromiseAggregateWatchers.byTask[key] = append(nativePromiseAggregateWatchers.byTask[key], watcher)
	nativePromiseAggregateWatchers.Unlock()
	task.completionMu.Unlock()
	return true
}

func takeNativePromiseAggregateWatchers(task uintptr) []nativePromiseAggregateWatcher {
	if task == 0 {
		return nil
	}
	nativePromiseAggregateWatchers.Lock()
	watchers := nativePromiseAggregateWatchers.byTask[task]
	delete(nativePromiseAggregateWatchers.byTask, task)
	nativePromiseAggregateWatchers.Unlock()
	return watchers
}

func notifyNativePromiseAggregateWatchers(watchers []nativePromiseAggregateWatcher, task *nativeTask) {
	for _, watcher := range watchers {
		if watcher.state != nil {
			watcher.state.observe(watcher.index, task)
		}
	}
}

func settleNativeAggregate(task *nativeTask, failed bool, failure unsafe.Pointer, f64 float64, boolean uint8, ref unsafe.Pointer) {
	if task == nil {
		return
	}
	task.completionMu.Lock()
	if nativeTaskIsTerminal(task) {
		task.completionMu.Unlock()
		return
	}
	if failed {
		nativeGCStoreRefSlot(unsafe.Pointer(&task.failureRef), failure)
		task.failureRequested.Store(1)
		task.status.Store(nativeTaskFailed)
	} else {
		switch task.kind {
		case nativeTaskResultF64:
			task.resultF64 = f64
		case nativeTaskResultBool:
			task.resultBool = boolean
		case nativeTaskResultRef:
			nativeGCStoreRefSlot(unsafe.Pointer(&task.resultRef), ref)
		default:
			task.completionMu.Unlock()
			nativeAbort("invalid Promise aggregate result kind")
		}
		task.status.Store(nativeTaskDone)
	}
	aggregateWatchers := takeNativePromiseAggregateWatchers(nativeTaskKey(task))
	completions := takeNativeTaskCompletions(nativeTaskKey(task))
	waiters := make([]uintptr, 0, len(completions))
	consumes := 0
	for _, completion := range completions {
		waiter := lookupNativeTask(completion.task)
		if waiter == nil {
			continue
		}
		waiters = append(waiters, completion.task)
		if completion.consume {
			consumes++
		}
		if completion.statusOnly && completion.out != nil {
			if failed {
				*(*uint8)(completion.out) = 0
			} else {
				*(*uint8)(completion.out) = 1
			}
		} else if failed {
			nativeGCStoreRefSlot(unsafe.Pointer(&waiter.failureRef), failure)
			waiter.failureRequested.Store(1)
		} else if completion.out != nil {
			transferNativeTaskResult(task, completion.out)
		}
	}
	task.completionMu.Unlock()

	goScheduler.mu.Lock()
	goScheduler.done.Broadcast()
	goScheduler.mu.Unlock()
	for _, waiter := range waiters {
		_ = schedulerWakeTask(waiter)
	}
	notifyNativePromiseAggregateWatchers(aggregateWatchers, task)
	for i := 0; i < consumes; i++ {
		releaseNativeTaskRef(task)
	}
}

type nativePromiseAggregateAction struct {
	settle  bool
	failed  bool
	failure unsafe.Pointer
	f64     float64
	boolean uint8
	ref     unsafe.Pointer
	allF64  []float64
	allBool []uint8
	allRef  []unsafe.Pointer
	release bool
}

func (state *nativePromiseAggregateState) observe(index int, child *nativeTask) {
	if state == nil || child == nil {
		return
	}
	child.completionMu.Lock()
	status := child.status.Load()
	failure := child.failureRef
	f64 := child.resultF64
	boolean := child.resultBool
	ref := child.resultRef
	terminal := nativeTaskIsTerminal(child)
	child.completionMu.Unlock()
	if !terminal {
		return
	}

	state.Lock()
	if index < 0 || index >= len(state.tasks) || state.observed[index] {
		state.Unlock()
		return
	}
	state.observed[index] = true
	state.remaining--
	action := nativePromiseAggregateAction{}

	switch state.kind {
	case nativePromiseAggregateAllF64:
		if status == nativeTaskFailed && !state.settled {
			state.settled = true
			action = nativePromiseAggregateAction{settle: true, failed: true, failure: failure}
		} else if status == nativeTaskDone {
			state.f64[index] = f64
		}
		if state.remaining == 0 && !state.settled {
			state.settled = true
			action = nativePromiseAggregateAction{settle: true, allF64: append([]float64(nil), state.f64...)}
		}
	case nativePromiseAggregateRaceF64:
		if !state.settled {
			state.settled = true
			if status == nativeTaskFailed {
				action = nativePromiseAggregateAction{settle: true, failed: true, failure: failure}
			} else {
				action = nativePromiseAggregateAction{settle: true, f64: f64}
			}
		}
	case nativePromiseAggregateAllBool:
		if status == nativeTaskFailed && !state.settled {
			state.settled = true
			action = nativePromiseAggregateAction{settle: true, failed: true, failure: failure}
		} else if status == nativeTaskDone {
			state.bools[index] = boolean
		}
		if state.remaining == 0 && !state.settled {
			state.settled = true
			action = nativePromiseAggregateAction{settle: true, allBool: append([]uint8(nil), state.bools...)}
		}
	case nativePromiseAggregateRaceBool:
		if !state.settled {
			state.settled = true
			if status == nativeTaskFailed {
				action = nativePromiseAggregateAction{settle: true, failed: true, failure: failure}
			} else {
				action = nativePromiseAggregateAction{settle: true, boolean: boolean}
			}
		}
	case nativePromiseAggregateAllRef:
		if status == nativeTaskFailed && !state.settled {
			state.settled = true
			action = nativePromiseAggregateAction{settle: true, failed: true, failure: failure}
		} else if status == nativeTaskDone {
			state.refs[index] = ref
		}
		if state.remaining == 0 && !state.settled {
			state.settled = true
			action = nativePromiseAggregateAction{settle: true, allRef: append([]unsafe.Pointer(nil), state.refs...)}
		}
	case nativePromiseAggregateRaceRef:
		if !state.settled {
			state.settled = true
			if status == nativeTaskFailed {
				action = nativePromiseAggregateAction{settle: true, failed: true, failure: failure}
			} else {
				action = nativePromiseAggregateAction{settle: true, ref: ref}
			}
		}
	}
	if state.remaining == 0 {
		action.release = true
	}
	aggregate := state.aggregate
	tasks := state.tasks
	state.Unlock()

	if action.settle {
		switch {
		case action.failed:
			settleNativeAggregate(aggregate, true, action.failure, 0, 0, nil)
		case action.allF64 != nil:
			array := tsnative_array_f64_new(C.uint64_t(len(action.allF64)))
			for i, value := range action.allF64 {
				tsnative_array_f64_set(array, C.uint64_t(i), C.double(value))
			}
			settleNativeAggregate(aggregate, false, nil, 0, 0, array)
		case action.allBool != nil:
			array := tsnative_array_bool_new(C.uint64_t(len(action.allBool)))
			for i, value := range action.allBool {
				tsnative_array_bool_set(array, C.uint64_t(i), C.uint8_t(value))
			}
			settleNativeAggregate(aggregate, false, nil, 0, 0, array)
		case state.kind == nativePromiseAggregateRaceBool:
			settleNativeAggregate(aggregate, false, nil, 0, action.boolean, nil)
		case action.allRef != nil:
			array := tsnative_array_ref_new(C.uint64_t(len(action.allRef)))
			for i, value := range action.allRef {
				tsnative_array_ref_set(array, C.uint64_t(i), value)
			}
			settleNativeAggregate(aggregate, false, nil, 0, 0, array)
		case state.kind == nativePromiseAggregateRaceRef:
			settleNativeAggregate(aggregate, false, nil, 0, 0, action.ref)
		default:
			settleNativeAggregate(aggregate, false, nil, action.f64, 0, nil)
		}
	}
	if action.release {
		for _, task := range tasks {
			releaseNativeTaskRef(task)
		}
	}
}

func startNativePromiseAggregate(kind nativePromiseAggregateKind, tasks []*nativeTask, aggregate *nativeTask) {
	state := &nativePromiseAggregateState{
		kind: kind, aggregate: aggregate, tasks: tasks, observed: make([]bool, len(tasks)), remaining: len(tasks),
	}
	if kind == nativePromiseAggregateAllF64 {
		state.f64 = make([]float64, len(tasks))
	}
	if kind == nativePromiseAggregateAllBool {
		state.bools = make([]uint8, len(tasks))
	}
	if kind == nativePromiseAggregateAllRef {
		state.refs = make([]unsafe.Pointer, len(tasks))
	}
	if len(tasks) == 0 {
		if kind == nativePromiseAggregateAllF64 {
			array := tsnative_array_f64_new(0)
			settleNativeAggregate(aggregate, false, nil, 0, 0, array)
		} else if kind == nativePromiseAggregateAllBool {
			array := tsnative_array_bool_new(0)
			settleNativeAggregate(aggregate, false, nil, 0, 0, array)
		} else if kind == nativePromiseAggregateAllRef {
			array := tsnative_array_ref_new(0)
			settleNativeAggregate(aggregate, false, nil, 0, 0, array)
		}
		return
	}
	for index, task := range tasks {
		watcher := nativePromiseAggregateWatcher{state: state, index: index}
		if !watchNativePromiseAggregate(task, watcher) {
			state.observe(index, task)
		}
	}
}

//export tsnative_promise_all_f64
func tsnative_promise_all_f64(raw unsafe.Pointer, count C.uint64_t) unsafe.Pointer {
	tasks := retainNativePromiseInputs(raw, uint64(count), nativeTaskResultF64)
	aggregate := allocateNativeTask(nil, nativeTaskResultRef)
	if aggregate == nil {
		for _, task := range tasks {
			releaseNativeTaskRef(task)
		}
		nativeAbort("Promise.all allocation failed")
	}
	aggregate.status.Store(nativeTaskWaiting)
	startNativePromiseAggregate(nativePromiseAggregateAllF64, tasks, aggregate)
	return aggregate.handle
}

//export tsnative_promise_race_f64
func tsnative_promise_race_f64(raw unsafe.Pointer, count C.uint64_t) unsafe.Pointer {
	if count == 0 {
		nativeAbort("empty Promise.race is not supported yet")
	}
	tasks := retainNativePromiseInputs(raw, uint64(count), nativeTaskResultF64)
	aggregate := allocateNativeTask(nil, nativeTaskResultF64)
	if aggregate == nil {
		for _, task := range tasks {
			releaseNativeTaskRef(task)
		}
		nativeAbort("Promise.race allocation failed")
	}
	aggregate.status.Store(nativeTaskWaiting)
	startNativePromiseAggregate(nativePromiseAggregateRaceF64, tasks, aggregate)
	return aggregate.handle
}

//export tsnative_promise_all_ref
func tsnative_promise_all_ref(raw unsafe.Pointer, count C.uint64_t) unsafe.Pointer {
	tasks := retainNativePromiseInputs(raw, uint64(count), nativeTaskResultRef)
	aggregate := allocateNativeTask(nil, nativeTaskResultRef)
	if aggregate == nil {
		for _, task := range tasks {
			releaseNativeTaskRef(task)
		}
		nativeAbort("reference Promise.all allocation failed")
	}
	aggregate.status.Store(nativeTaskWaiting)
	startNativePromiseAggregate(nativePromiseAggregateAllRef, tasks, aggregate)
	return aggregate.handle
}

//export tsnative_promise_race_ref
func tsnative_promise_race_ref(raw unsafe.Pointer, count C.uint64_t) unsafe.Pointer {
	if count == 0 {
		nativeAbort("empty reference Promise.race is not supported")
	}
	tasks := retainNativePromiseInputs(raw, uint64(count), nativeTaskResultRef)
	aggregate := allocateNativeTask(nil, nativeTaskResultRef)
	if aggregate == nil {
		for _, task := range tasks {
			releaseNativeTaskRef(task)
		}
		nativeAbort("reference Promise.race allocation failed")
	}
	aggregate.status.Store(nativeTaskWaiting)
	startNativePromiseAggregate(nativePromiseAggregateRaceRef, tasks, aggregate)
	return aggregate.handle
}

//export tsnative_promise_all_bool
func tsnative_promise_all_bool(raw unsafe.Pointer, count C.uint64_t) unsafe.Pointer {
	tasks := retainNativePromiseInputs(raw, uint64(count), nativeTaskResultBool)
	aggregate := allocateNativeTask(nil, nativeTaskResultRef)
	if aggregate == nil {
		for _, task := range tasks {
			releaseNativeTaskRef(task)
		}
		nativeAbort("boolean Promise.all allocation failed")
	}
	aggregate.status.Store(nativeTaskWaiting)
	startNativePromiseAggregate(nativePromiseAggregateAllBool, tasks, aggregate)
	return aggregate.handle
}

//export tsnative_promise_race_bool
func tsnative_promise_race_bool(raw unsafe.Pointer, count C.uint64_t) unsafe.Pointer {
	if count == 0 {
		nativeAbort("empty boolean Promise.race is not supported")
	}
	tasks := retainNativePromiseInputs(raw, uint64(count), nativeTaskResultBool)
	aggregate := allocateNativeTask(nil, nativeTaskResultBool)
	if aggregate == nil {
		for _, task := range tasks {
			releaseNativeTaskRef(task)
		}
		nativeAbort("boolean Promise.race allocation failed")
	}
	aggregate.status.Store(nativeTaskWaiting)
	startNativePromiseAggregate(nativePromiseAggregateRaceBool, tasks, aggregate)
	return aggregate.handle
}
