package main

import "testing"

func TestGoTaskContextOwnsPersistentGCRoot(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	task := createNativeTask(1, 0, nativeTaskResultVoid)
	if task == nil {
		t.Fatal("task allocation failed")
	}
	defer destroyNativeTaskStorage(task)

	payload := tsnative_heap_alloc(32)
	if payload == nil {
		t.Fatal("payload allocation failed")
	}
	task.context = uintptr(payload)
	tsnative_gc_collect()
	if !nativeHeapContains(payload) {
		t.Fatal("task context payload was collected while rooted")
	}

	task.context = 0
	tsnative_gc_collect()
	if nativeHeapContains(payload) {
		t.Fatal("task context payload survived after context cleared")
	}
}
