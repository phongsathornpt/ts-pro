package main

/*
#include <stdint.h>
*/
import "C"

import "unsafe"

type nativePromiseObservation struct {
	index int
	task  *nativeTask
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
				releaseNativePromiseInputWhenTerminal(held)
			}
			nativeAbort("invalid Promise aggregate task")
		}
		tasks = append(tasks, task)
	}
	return tasks
}

func observeNativePromise(index int, task *nativeTask, out chan<- nativePromiseObservation) {
	_ = tsnative_scheduler_wait(task.handle)
	out <- nativePromiseObservation{index: index, task: task}
}

func releaseNativePromiseInputWhenTerminal(task *nativeTask) {
	if task == nil {
		return
	}
	if nativeTaskIsTerminal(task) {
		releaseNativeTaskRef(task)
		return
	}
	go func() {
		_ = tsnative_scheduler_wait(task.handle)
		releaseNativeTaskRef(task)
	}()
}

func settleNativeAggregate(task *nativeTask, failed bool, failure unsafe.Pointer, f64 float64, ref unsafe.Pointer) {
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
		case nativeTaskResultRef:
			nativeGCStoreRefSlot(unsafe.Pointer(&task.resultRef), ref)
		default:
			task.completionMu.Unlock()
			nativeAbort("invalid Promise aggregate result kind")
		}
		task.status.Store(nativeTaskDone)
	}

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
	for i := 0; i < consumes; i++ {
		releaseNativeTaskRef(task)
	}
}

func settledNativePromiseF64(task *nativeTask) (status int32, failure unsafe.Pointer, value float64, terminal bool) {
	task.completionMu.Lock()
	defer task.completionMu.Unlock()
	if !nativeTaskIsTerminal(task) {
		return 0, nil, 0, false
	}
	return task.status.Load(), task.failureRef, task.resultF64, true
}

func finishNativePromiseAllF64(aggregate *nativeTask, results []float64) {
	array := tsnative_array_f64_new(C.uint64_t(len(results)))
	for index, value := range results {
		tsnative_array_f64_set(array, C.uint64_t(index), C.double(value))
	}
	settleNativeAggregate(aggregate, false, nil, 0, array)
}

func startNativePromiseAllF64(tasks []*nativeTask, aggregate *nativeTask) {
	if len(tasks) == 0 {
		finishNativePromiseAllF64(aggregate, nil)
		return
	}
	results := make([]float64, len(tasks))
	pending := make([]nativePromiseObservation, 0, len(tasks))
	for index, child := range tasks {
		status, failure, value, terminal := settledNativePromiseF64(child)
		if !terminal {
			pending = append(pending, nativePromiseObservation{index: index, task: child})
			continue
		}
		if status == nativeTaskFailed {
			settleNativeAggregate(aggregate, true, failure, 0, nil)
			for _, held := range tasks {
				releaseNativePromiseInputWhenTerminal(held)
			}
			return
		}
		results[index] = value
		releaseNativeTaskRef(child)
		tasks[index] = nil
	}
	if len(pending) == 0 {
		finishNativePromiseAllF64(aggregate, results)
		return
	}

	observations := make(chan nativePromiseObservation, len(pending))
	for _, item := range pending {
		go observeNativePromise(item.index, item.task, observations)
	}
	go func() {
		failed := false
		for range pending {
			observation := <-observations
			child := observation.task
			status, failure, value, _ := settledNativePromiseF64(child)
			if status == nativeTaskFailed && !failed {
				failed = true
				settleNativeAggregate(aggregate, true, failure, 0, nil)
			} else if status == nativeTaskDone {
				results[observation.index] = value
			}
			releaseNativeTaskRef(child)
		}
		if !failed {
			finishNativePromiseAllF64(aggregate, results)
		}
	}()
}

func startNativePromiseRaceF64(tasks []*nativeTask, aggregate *nativeTask) {
	for _, child := range tasks {
		status, failure, value, terminal := settledNativePromiseF64(child)
		if !terminal {
			continue
		}
		if status == nativeTaskFailed {
			settleNativeAggregate(aggregate, true, failure, 0, nil)
		} else {
			settleNativeAggregate(aggregate, false, nil, value, nil)
		}
		for _, held := range tasks {
			releaseNativePromiseInputWhenTerminal(held)
		}
		return
	}

	observations := make(chan nativePromiseObservation, len(tasks))
	for index, task := range tasks {
		go observeNativePromise(index, task, observations)
	}
	go func() {
		settled := false
		for range tasks {
			observation := <-observations
			child := observation.task
			status, failure, value, _ := settledNativePromiseF64(child)
			if !settled {
				settled = true
				if status == nativeTaskFailed {
					settleNativeAggregate(aggregate, true, failure, 0, nil)
				} else {
					settleNativeAggregate(aggregate, false, nil, value, nil)
				}
			}
			releaseNativeTaskRef(child)
		}
	}()
}

//export tsnative_promise_all_f64
func tsnative_promise_all_f64(raw unsafe.Pointer, count C.uint64_t) unsafe.Pointer {
	tasks := retainNativePromiseInputs(raw, uint64(count), nativeTaskResultF64)
	aggregate := allocateNativeTask(nil, nativeTaskResultRef)
	if aggregate == nil {
		for _, task := range tasks {
			releaseNativePromiseInputWhenTerminal(task)
		}
		nativeAbort("Promise.all allocation failed")
	}
	aggregate.status.Store(nativeTaskWaiting)
	startNativePromiseAllF64(tasks, aggregate)
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
			releaseNativePromiseInputWhenTerminal(task)
		}
		nativeAbort("Promise.race allocation failed")
	}
	aggregate.status.Store(nativeTaskWaiting)
	startNativePromiseRaceF64(tasks, aggregate)
	return aggregate.handle
}
