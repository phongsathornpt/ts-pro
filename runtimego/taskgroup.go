package main

/*
#include <stdint.h>
*/
import "C"

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

type nativeTaskGroup struct {
	mu     sync.Mutex
	done   *sync.Cond
	tasks  map[uintptr]struct{}
	closed bool
}

var nativeTaskGroups sync.Map
var nativeTaskGroupsByTask sync.Map
var nativeTaskGroupToken atomic.Uint64

func taskGroupState(raw unsafe.Pointer) (*nativeTaskGroup, bool) {
	if raw == nil {
		return nil, false
	}
	value, ok := nativeTaskGroups.Load(uintptr(raw))
	if !ok {
		return nil, false
	}
	group, ok := value.(*nativeTaskGroup)
	return group, ok
}

func taskGroupAttach(raw unsafe.Pointer, task uintptr) bool {
	group, ok := taskGroupState(raw)
	if !ok || task == 0 {
		return false
	}
	group.mu.Lock()
	defer group.mu.Unlock()
	if group.closed {
		return false
	}
	group.tasks[task] = struct{}{}
	nativeTaskGroupsByTask.Store(task, uintptr(raw))
	return true
}

func taskGroupDetach(task uintptr) {
	value, ok := nativeTaskGroupsByTask.LoadAndDelete(task)
	if !ok {
		return
	}
	group, ok := taskGroupState(unsafe.Pointer(value.(uintptr)))
	if !ok {
		return
	}
	group.mu.Lock()
	delete(group.tasks, task)
	if len(group.tasks) == 0 {
		group.done.Broadcast()
	}
	group.mu.Unlock()
}

func taskGroupSpawn(raw, entry, state unsafe.Pointer, kind int32) unsafe.Pointer {
	if _, ok := taskGroupState(raw); !ok || entry == nil {
		return nil
	}
	if tsnative_scheduler_init() != 0 {
		return nil
	}
	task := createNativeTask(uintptr(entry), uintptr(state), kind)
	if task == nil {
		return nil
	}
	if !taskGroupAttach(raw, task.handle) {
		destroyNativeTaskStorage(task)
		return nil
	}
	if tsnative_scheduler_submit(unsafe.Pointer(task.handle)) != 0 {
		taskGroupDetach(task.handle)
		destroyNativeTaskStorage(task)
		return nil
	}
	return unsafe.Pointer(task.handle)
}

//export tsnative_task_group_new
func tsnative_task_group_new() unsafe.Pointer {
	handle := uintptr(nativeTaskGroupToken.Add(1)<<4 | 3)
	group := &nativeTaskGroup{tasks: make(map[uintptr]struct{})}
	group.done = sync.NewCond(&group.mu)
	nativeTaskGroups.Store(handle, group)
	return unsafe.Pointer(handle)
}

func taskGroupSpawnOrAbort(group, entry, state unsafe.Pointer, kind int32) unsafe.Pointer {
	task := taskGroupSpawn(group, entry, state, kind)
	if task == nil {
		nativeAbort("task group spawn failed")
	}
	return task
}

//export tsnative_task_group_spawn_or_abort
func tsnative_task_group_spawn_or_abort(group, entry, state unsafe.Pointer) unsafe.Pointer {
	return taskGroupSpawnOrAbort(group, entry, state, nativeTaskResultVoid)
}

//export tsnative_task_group_spawn_f64_or_abort
func tsnative_task_group_spawn_f64_or_abort(group, entry, state unsafe.Pointer) unsafe.Pointer {
	return taskGroupSpawnOrAbort(group, entry, state, nativeTaskResultF64)
}

//export tsnative_task_group_spawn_bool_or_abort
func tsnative_task_group_spawn_bool_or_abort(group, entry, state unsafe.Pointer) unsafe.Pointer {
	return taskGroupSpawnOrAbort(group, entry, state, nativeTaskResultBool)
}

//export tsnative_task_group_spawn_ref_or_abort
func tsnative_task_group_spawn_ref_or_abort(group, entry, state unsafe.Pointer) unsafe.Pointer {
	return taskGroupSpawnOrAbort(group, entry, state, nativeTaskResultRef)
}

//export tsnative_task_group_cancel
func tsnative_task_group_cancel(raw unsafe.Pointer) C.int {
	group, ok := taskGroupState(raw)
	if !ok {
		return -1
	}
	group.mu.Lock()
	tasks := make([]uintptr, 0, len(group.tasks))
	for handle := range group.tasks {
		tasks = append(tasks, handle)
	}
	group.mu.Unlock()
	for _, handle := range tasks {
		if task := lookupNativeTask(handle); task != nil {
			task.cancelRequested.Store(1)
		}
	}
	return 0
}

//export tsnative_task_group_join_release
func tsnative_task_group_join_release(raw unsafe.Pointer) C.int {
	group, ok := taskGroupState(raw)
	if !ok {
		return -1
	}
	group.mu.Lock()
	group.closed = true
	for len(group.tasks) != 0 {
		group.done.Wait()
	}
	group.mu.Unlock()
	nativeTaskGroups.Delete(uintptr(raw))
	return 0
}
