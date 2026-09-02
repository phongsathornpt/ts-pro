package runtime

import "unsafe"

func taskContextGoEnabled() int32 { return 1 }

func taskContextReleaseGo(raw unsafe.Pointer) {
	// Task context storage and GC rooting are owned by nativeTask.
}

func taskSetContext(value unsafe.Pointer) {
	task := lookupNativeTask(schedulerCurrentTaskPtr())
	if task == nil {
		nativeAbort("task context set outside task")
	}
	task.context = value
}

func taskGetContext() unsafe.Pointer {
	task := lookupNativeTask(schedulerCurrentTaskPtr())
	if task == nil {
		nativeAbort("task context get outside task")
	}
	return task.context
}
