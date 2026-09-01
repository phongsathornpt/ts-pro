package main

/*
#cgo CFLAGS: -I../runtime/concurrency
#include <stdint.h>
#include <stdlib.h>
#define TSNATIVE_CGO_TASKGROUP_EXPORTS 1
#include "../runtime/concurrency/task_internal.h"

extern tsnative_task *tsnative_task_create_unsubmitted_internal(tsnative_task_entry, void *, tsnative_task_result_kind) __attribute__((weak));
extern void tsnative_task_destroy_unsubmitted_internal(tsnative_task *) __attribute__((weak));

static tsnative_task *tsnative_go_task_create(uintptr_t entry, void *state, int kind) {
    if (!tsnative_task_create_unsubmitted_internal) return NULL;
    return tsnative_task_create_unsubmitted_internal((tsnative_task_entry)entry, state, (tsnative_task_result_kind)kind);
}
static void tsnative_go_task_destroy(tsnative_task *task) {
    if (tsnative_task_destroy_unsubmitted_internal) tsnative_task_destroy_unsubmitted_internal(task);
}
static void tsnative_go_task_cancel(tsnative_task *task) {
    tsnative_task_request_cancel_internal(task);
}
*/
import "C"

import (
	"sync"
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
	raw := unsafe.Pointer(value.(uintptr))
	group, ok := taskGroupState(raw)
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

func taskGroupSpawn(raw unsafe.Pointer, entry unsafe.Pointer, state unsafe.Pointer, kind C.int) unsafe.Pointer {
	group, ok := taskGroupState(raw)
	if !ok || entry == nil {
		return nil
	}
	if tsnative_scheduler_init() != 0 {
		return nil
	}
	task := C.tsnative_go_task_create(C.uintptr_t(uintptr(entry)), state, kind)
	if task == nil {
		return nil
	}
	taskPtr := uintptr(unsafe.Pointer(task))
	if !taskGroupAttach(raw, taskPtr) {
		C.tsnative_go_task_destroy(task)
		return nil
	}
	if tsnative_scheduler_submit(unsafe.Pointer(task)) != 0 {
		taskGroupDetach(taskPtr)
		C.tsnative_go_task_destroy(task)
		return nil
	}
	_ = group
	return unsafe.Pointer(task)
}

//export tsnative_task_group_new
func tsnative_task_group_new() unsafe.Pointer {
	token := C.malloc(1)
	if token == nil {
		return nil
	}
	group := &nativeTaskGroup{tasks: make(map[uintptr]struct{})}
	group.done = sync.NewCond(&group.mu)
	nativeTaskGroups.Store(uintptr(token), group)
	return token
}

func taskGroupSpawnOrAbort(group, entry, state unsafe.Pointer, kind C.int) unsafe.Pointer {
	task := taskGroupSpawn(group, entry, state, kind)
	if task == nil {
		C.abort()
	}
	return task
}

//export tsnative_task_group_spawn_or_abort
func tsnative_task_group_spawn_or_abort(group, entry, state unsafe.Pointer) unsafe.Pointer {
	return taskGroupSpawnOrAbort(group, entry, state, C.TSNATIVE_TASK_RESULT_VOID)
}

//export tsnative_task_group_spawn_f64_or_abort
func tsnative_task_group_spawn_f64_or_abort(group, entry, state unsafe.Pointer) unsafe.Pointer {
	return taskGroupSpawnOrAbort(group, entry, state, C.TSNATIVE_TASK_RESULT_F64)
}

//export tsnative_task_group_spawn_bool_or_abort
func tsnative_task_group_spawn_bool_or_abort(group, entry, state unsafe.Pointer) unsafe.Pointer {
	return taskGroupSpawnOrAbort(group, entry, state, C.TSNATIVE_TASK_RESULT_BOOL)
}

//export tsnative_task_group_spawn_ref_or_abort
func tsnative_task_group_spawn_ref_or_abort(group, entry, state unsafe.Pointer) unsafe.Pointer {
	return taskGroupSpawnOrAbort(group, entry, state, C.TSNATIVE_TASK_RESULT_REF)
}

//export tsnative_task_group_cancel
func tsnative_task_group_cancel(raw unsafe.Pointer) C.int {
	group, ok := taskGroupState(raw)
	if !ok {
		return -1
	}
	group.mu.Lock()
	for task := range group.tasks {
		C.tsnative_go_task_cancel((*C.tsnative_task)(unsafe.Pointer(task)))
	}
	group.mu.Unlock()
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
	C.free(raw)
	return 0
}
