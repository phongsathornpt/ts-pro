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

func TestNativeJSValueDynamicPrimitiveOperators(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	six := tsnative_jsvalue_box_string(nativeJSStringLiteral("6"))
	seven := tsnative_jsvalue_box_f64(7)
	if got := float64(tsnative_jsvalue_sub(six, tsnative_jsvalue_box_f64(1))); got != 5 {
		t.Fatalf("sub = %v", got)
	}
	if got := float64(tsnative_jsvalue_mul(six, seven)); got != 42 {
		t.Fatalf("mul = %v", got)
	}
	if got := float64(tsnative_jsvalue_div(tsnative_jsvalue_box_f64(84), seven)); got != 12 {
		t.Fatalf("div = %v", got)
	}
	if tsnative_jsvalue_lt(six, seven) == 0 {
		t.Fatal("6 < 7 should be true")
	}
	ten := tsnative_jsvalue_box_string(nativeJSStringLiteral("10"))
	two := tsnative_jsvalue_box_string(nativeJSStringLiteral("2"))
	if tsnative_jsvalue_lt(ten, two) == 0 {
		t.Fatal("string 10 < 2 should be true")
	}
	if tsnative_jsvalue_eq(six, tsnative_jsvalue_box_f64(6)) == 0 {
		t.Fatal("loose equality should coerce string")
	}
	if tsnative_jsvalue_strict_eq(six, tsnative_jsvalue_box_f64(6)) != 0 {
		t.Fatal("strict equality should not coerce string")
	}
	if tsnative_jsvalue_eq(tsnative_jsvalue_null(), tsnative_jsvalue_undefined()) == 0 {
		t.Fatal("null == undefined should be true")
	}
	if tsnative_jsvalue_strict_eq(tsnative_jsvalue_null(), tsnative_jsvalue_undefined()) != 0 {
		t.Fatal("null === undefined should be false")
	}
	object := tsnative_heap_alloc(8)
	left := tsnative_jsvalue_box_object(object)
	right := tsnative_jsvalue_box_object(object)
	if tsnative_jsvalue_strict_eq(left, right) == 0 {
		t.Fatal("same object payload should be strictly equal")
	}
}
