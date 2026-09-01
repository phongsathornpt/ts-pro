package main

/*
#include <stdint.h>
*/
import "C"

import "unsafe"

//export tsnative_task_context_go_enabled
func tsnative_task_context_go_enabled() C.int { return 1 }

//export tsnative_task_context_release_go
func tsnative_task_context_release_go(raw unsafe.Pointer) {
	// Task context storage and GC rooting are owned by nativeTask.
}

//export tsnative_task_set_context
func tsnative_task_set_context(value unsafe.Pointer) {
	task := lookupNativeTask(schedulerCurrentTaskPtr())
	if task == nil {
		nativeAbort("task context set outside task")
	}
	task.context = uintptr(value)
}

//export tsnative_task_get_context
func tsnative_task_get_context() unsafe.Pointer {
	task := lookupNativeTask(schedulerCurrentTaskPtr())
	if task == nil {
		nativeAbort("task context get outside task")
	}
	return unsafe.Pointer(task.context)
}
