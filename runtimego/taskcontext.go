package runtimego

import "unsafe"

func tsnative_task_context_go_enabled() int32 { return 1 }

func tsnative_task_context_release_go(raw unsafe.Pointer) {
	// Task context storage and GC rooting are owned by nativeTask.
}

func tsnative_task_set_context(value unsafe.Pointer) {
	task := lookupNativeTask(schedulerCurrentTaskPtr())
	if task == nil {
		nativeAbort("task context set outside task")
	}
	task.context = value
}

func tsnative_task_get_context() unsafe.Pointer {
	task := lookupNativeTask(schedulerCurrentTaskPtr())
	if task == nil {
		nativeAbort("task context get outside task")
	}
	return task.context
}
