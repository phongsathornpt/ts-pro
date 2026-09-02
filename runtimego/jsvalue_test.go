package main

import (
	"math"
	"testing"
	"unsafe"
)

func nativeHeapContains(raw unsafe.Pointer) bool {
	return nativeBlocks.get(uintptr(raw)) != nil
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

func TestNativeJSValueCheckedUnboxing(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	number := tsnative_jsvalue_box_f64(42)
	if got := float64(tsnative_jsvalue_unbox_f64(number)); got != 42 {
		t.Fatalf("number unbox = %v", got)
	}
	stringRef := nativeJSStringLiteral("checked")
	stringValue := tsnative_jsvalue_box_string(stringRef)
	if got := tsnative_jsvalue_unbox_string(stringValue); got != stringRef {
		t.Fatal("string unbox did not preserve native reference")
	}
	truth := tsnative_jsvalue_box_bool(1)
	if tsnative_jsvalue_unbox_bool(truth) == 0 {
		t.Fatal("boolean unbox = false; want true")
	}
	array := tsnative_heap_alloc(24)
	arrayValue := tsnative_jsvalue_box_array(array)
	if (*nativeJSValue)(arrayValue).tag != nativeJSTagArray {
		t.Fatal("array JSValue tag mismatch")
	}
	if got := tsnative_jsvalue_unbox_array(arrayValue); got != array {
		t.Fatal("array unbox did not preserve native reference")
	}
}

func TestJSValueOrdinaryObjectAndArrayToPrimitive(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	objectRaw := tsnative_heap_alloc_atomic(8)
	object := tsnative_jsvalue_box_object(objectRaw)
	number := tsnative_jsvalue_box_f64(1)
	added := (*nativeJSValue)(tsnative_jsvalue_add(object, number))
	if added.tag != nativeJSTagString || string(nativeStringBytes(nativeJSRef(added))) != "[object Object]1" {
		t.Fatalf("object + 1 = tag=%d value=%q", added.tag, nativeStringBytes(nativeJSRef(added)))
	}
	objectText := tsnative_jsvalue_box_string(nativeJSStringLiteral("[object Object]"))
	if tsnative_jsvalue_eq(object, objectText) == 0 {
		t.Fatal("ordinary object did not loose-equal its default primitive string")
	}

	arrayRaw := tsnative_array_f64_new(2)
	tsnative_array_f64_set(arrayRaw, 0, 1)
	tsnative_array_f64_set(arrayRaw, 1, 2)
	array := tsnative_jsvalue_box_array(arrayRaw)
	prefix := tsnative_jsvalue_box_string(nativeJSStringLiteral("values="))
	arrayText := (*nativeJSValue)(tsnative_jsvalue_add(prefix, array))
	if arrayText.tag != nativeJSTagString || string(nativeStringBytes(nativeJSRef(arrayText))) != "values=1,2" {
		t.Fatalf("prefix + array = tag=%d value=%q", arrayText.tag, nativeStringBytes(nativeJSRef(arrayText)))
	}
}

func TestJSValueObjectArrayFunctionNumericCoercion(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	one := tsnative_array_f64_new(1)
	tsnative_array_f64_set(one, 0, 5)
	array := tsnative_jsvalue_box_array(one)
	if got := float64(tsnative_jsvalue_sub(array, tsnative_jsvalue_box_f64(2))); got != 3 {
		t.Fatalf("[5] - 2 = %v, want 3", got)
	}

	object := tsnative_jsvalue_box_object(tsnative_heap_alloc_atomic(8))
	if got := float64(tsnative_jsvalue_sub(object, tsnative_jsvalue_box_f64(1))); !math.IsNaN(got) {
		t.Fatalf("object - 1 = %v, want NaN", got)
	}
	function := tsnative_jsvalue_box_function(tsnative_heap_alloc_atomic(8))
	if got := float64(tsnative_jsvalue_mul(function, tsnative_jsvalue_box_f64(2))); !math.IsNaN(got) {
		t.Fatalf("function * 2 = %v, want NaN", got)
	}
}
