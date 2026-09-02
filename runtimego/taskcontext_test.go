package runtimego

import (
	"testing"
	"unsafe"
)

func TestGoTaskContextOwnsPersistentGCRoot(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	var entry byte
	task := createNativeTask(unsafe.Pointer(&entry), nil, nativeTaskResultVoid)
	if task == nil {
		t.Fatal("task allocation failed")
	}
	defer destroyNativeTaskStorage(task)

	payload := tsnative_heap_alloc(32)
	if payload == nil {
		t.Fatal("payload allocation failed")
	}
	task.context = payload
	tsnative_gc_collect()
	if !nativeHeapContains(payload) {
		t.Fatal("task context payload was collected while rooted")
	}

	task.context = nil
	tsnative_gc_collect()
	if nativeHeapContains(payload) {
		t.Fatal("task context payload survived after context cleared")
	}
}
