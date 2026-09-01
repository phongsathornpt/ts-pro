package main

import (
	"testing"
	"unsafe"
)

func nativeHeapContains(raw unsafe.Pointer) bool {
	nativeHeap.Lock()
	_, ok := nativeHeap.blocks[uintptr(raw)]
	nativeHeap.Unlock()
	return ok
}

func TestNativeJSValueReferenceBoxesKeepPayloadAlive(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	object := tsnative_heap_alloc(16)
	boxedObjectRaw := tsnative_jsvalue_box_object(object)
	boxedObject := (*nativeJSValue)(boxedObjectRaw)
	if boxedObject.tag != nativeJSTagObject || nativeJSRef(boxedObject) != object {
		t.Fatal("object JSValue tag/payload mismatch")
	}

	rootSlot := boxedObjectRaw
	root := tsnative_gc_root_register(unsafe.Pointer(&rootSlot))
	if root == nil {
		t.Fatal("object JSValue root registration failed")
	}
	tsnative_gc_collect()
	if !nativeHeapContains(object) {
		t.Fatal("boxed object payload was collected while JSValue was rooted")
	}
	tsnative_gc_root_unregister(root)
	tsnative_gc_collect()
	if nativeHeapContains(object) {
		t.Fatal("object payload survived after JSValue root release")
	}

	function := tsnative_heap_alloc(16)
	boxedFunctionRaw := tsnative_jsvalue_box_function(function)
	boxedFunction := (*nativeJSValue)(boxedFunctionRaw)
	if boxedFunction.tag != nativeJSTagFunction || nativeJSRef(boxedFunction) != function {
		t.Fatal("function JSValue tag/payload mismatch")
	}
	rootSlot = boxedFunctionRaw
	root = tsnative_gc_root_register(unsafe.Pointer(&rootSlot))
	tsnative_gc_collect()
	if !nativeHeapContains(function) {
		t.Fatal("boxed function payload was collected while JSValue was rooted")
	}
	tsnative_gc_root_unregister(root)
	tsnative_gc_collect()
	if nativeHeapContains(function) {
		t.Fatal("function payload survived after JSValue root release")
	}
}
