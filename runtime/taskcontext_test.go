package runtime

import (
	"testing"
	"unsafe"
)

func TestGoTaskContextOwnsPersistentGCRoot(t *testing.T) {
	heapShutdown()
	defer heapShutdown()

	var entry byte
	task := createNativeTask(unsafe.Pointer(&entry), nil, nativeTaskResultVoid)
	if task == nil {
		t.Fatal("task allocation failed")
	}
	defer destroyNativeTaskStorage(task)

	payload := heapAlloc(32)
	if payload == nil {
		t.Fatal("payload allocation failed")
	}
	task.context = payload
	gcCollect()
	if !nativeHeapContains(payload) {
		t.Fatal("task context payload was collected while rooted")
	}

	task.context = nil
	gcCollect()
	if nativeHeapContains(payload) {
		t.Fatal("task context payload survived after context cleared")
	}
}
