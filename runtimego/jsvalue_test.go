package runtimego

import (
	"math"
	"testing"
	"unsafe"
)

func nativeHeapContains(raw unsafe.Pointer) bool {
	return nativeBlocks.get(uintptr(raw)) != nil
}

func TestNativeJSValueReferenceBoxesKeepPayloadAlive(t *testing.T) {
	heapShutdown()
	defer heapShutdown()

	object := heapAlloc(16)
	boxedObjectRaw := jsValueBoxObject(object)
	boxedObject := (*nativeJSValue)(boxedObjectRaw)
	if boxedObject.tag != nativeJSTagObject || nativeJSRef(boxedObject) != object {
		t.Fatal("object JSValue tag/payload mismatch")
	}

	rootSlot := boxedObjectRaw
	root := gcRootRegister(unsafe.Pointer(&rootSlot))
	if root == nil {
		t.Fatal("object JSValue root registration failed")
	}
	gcCollect()
	if !nativeHeapContains(object) {
		t.Fatal("boxed object payload was collected while JSValue was rooted")
	}
	gcRootUnregister(root)
	gcCollect()
	if nativeHeapContains(object) {
		t.Fatal("object payload survived after JSValue root release")
	}

	function := heapAlloc(16)
	boxedFunctionRaw := jsValueBoxFunction(function)
	boxedFunction := (*nativeJSValue)(boxedFunctionRaw)
	if boxedFunction.tag != nativeJSTagFunction || nativeJSRef(boxedFunction) != function {
		t.Fatal("function JSValue tag/payload mismatch")
	}
	rootSlot = boxedFunctionRaw
	root = gcRootRegister(unsafe.Pointer(&rootSlot))
	gcCollect()
	if !nativeHeapContains(function) {
		t.Fatal("boxed function payload was collected while JSValue was rooted")
	}
	gcRootUnregister(root)
	gcCollect()
	if nativeHeapContains(function) {
		t.Fatal("function payload survived after JSValue root release")
	}
}

func TestNativeJSValueDynamicPrimitiveOperators(t *testing.T) {
	heapShutdown()
	defer heapShutdown()

	six := jsValueBoxString(nativeJSStringLiteral("6"))
	seven := jsValueBoxF64(7)
	if got := float64(jsValueSub(six, jsValueBoxF64(1))); got != 5 {
		t.Fatalf("sub = %v", got)
	}
	if got := float64(jsValueMul(six, seven)); got != 42 {
		t.Fatalf("mul = %v", got)
	}
	if got := float64(jsValueDiv(jsValueBoxF64(84), seven)); got != 12 {
		t.Fatalf("div = %v", got)
	}
	if jsValueLt(six, seven) == 0 {
		t.Fatal("6 < 7 should be true")
	}
	ten := jsValueBoxString(nativeJSStringLiteral("10"))
	two := jsValueBoxString(nativeJSStringLiteral("2"))
	if jsValueLt(ten, two) == 0 {
		t.Fatal("string 10 < 2 should be true")
	}
	if jsValueEq(six, jsValueBoxF64(6)) == 0 {
		t.Fatal("loose equality should coerce string")
	}
	if jsValueStrictEq(six, jsValueBoxF64(6)) != 0 {
		t.Fatal("strict equality should not coerce string")
	}
	if jsValueEq(jsValueNull(), jsValueUndefined()) == 0 {
		t.Fatal("null == undefined should be true")
	}
	if jsValueStrictEq(jsValueNull(), jsValueUndefined()) != 0 {
		t.Fatal("null === undefined should be false")
	}
	object := heapAlloc(8)
	left := jsValueBoxObject(object)
	right := jsValueBoxObject(object)
	if jsValueStrictEq(left, right) == 0 {
		t.Fatal("same object payload should be strictly equal")
	}
}

func TestNativeJSValueCheckedUnboxing(t *testing.T) {
	heapShutdown()
	defer heapShutdown()

	number := jsValueBoxF64(42)
	if got := float64(jsValueUnboxF64(number)); got != 42 {
		t.Fatalf("number unbox = %v", got)
	}
	stringRef := nativeJSStringLiteral("checked")
	stringValue := jsValueBoxString(stringRef)
	if got := jsValueUnboxString(stringValue); got != stringRef {
		t.Fatal("string unbox did not preserve native reference")
	}
	truth := jsValueBoxBool(1)
	if jsValueUnboxBool(truth) == 0 {
		t.Fatal("boolean unbox = false; want true")
	}
	array := heapAlloc(24)
	arrayValue := jsValueBoxArray(array)
	if (*nativeJSValue)(arrayValue).tag != nativeJSTagArray {
		t.Fatal("array JSValue tag mismatch")
	}
	if got := jsValueUnboxArray(arrayValue); got != array {
		t.Fatal("array unbox did not preserve native reference")
	}
}

func TestJSValueOrdinaryObjectAndArrayToPrimitive(t *testing.T) {
	heapShutdown()
	defer heapShutdown()

	objectRaw := heapAllocAtomic(8)
	object := jsValueBoxObject(objectRaw)
	number := jsValueBoxF64(1)
	added := (*nativeJSValue)(jsValueAdd(object, number))
	if added.tag != nativeJSTagString || string(nativeStringBytes(nativeJSRef(added))) != "[object Object]1" {
		t.Fatalf("object + 1 = tag=%d value=%q", added.tag, nativeStringBytes(nativeJSRef(added)))
	}
	objectText := jsValueBoxString(nativeJSStringLiteral("[object Object]"))
	if jsValueEq(object, objectText) == 0 {
		t.Fatal("ordinary object did not loose-equal its default primitive string")
	}

	arrayRaw := arrayF64New(2)
	arrayF64Set(arrayRaw, 0, 1)
	arrayF64Set(arrayRaw, 1, 2)
	array := jsValueBoxArray(arrayRaw)
	prefix := jsValueBoxString(nativeJSStringLiteral("values="))
	arrayText := (*nativeJSValue)(jsValueAdd(prefix, array))
	if arrayText.tag != nativeJSTagString || string(nativeStringBytes(nativeJSRef(arrayText))) != "values=1,2" {
		t.Fatalf("prefix + array = tag=%d value=%q", arrayText.tag, nativeStringBytes(nativeJSRef(arrayText)))
	}
}

func TestJSValueObjectArrayFunctionNumericCoercion(t *testing.T) {
	heapShutdown()
	defer heapShutdown()

	one := arrayF64New(1)
	arrayF64Set(one, 0, 5)
	array := jsValueBoxArray(one)
	if got := float64(jsValueSub(array, jsValueBoxF64(2))); got != 3 {
		t.Fatalf("[5] - 2 = %v, want 3", got)
	}

	object := jsValueBoxObject(heapAllocAtomic(8))
	if got := float64(jsValueSub(object, jsValueBoxF64(1))); !math.IsNaN(got) {
		t.Fatalf("object - 1 = %v, want NaN", got)
	}
	function := jsValueBoxFunction(heapAllocAtomic(8))
	if got := float64(jsValueMul(function, jsValueBoxF64(2))); !math.IsNaN(got) {
		t.Fatalf("function * 2 = %v, want NaN", got)
	}
}

func TestNativeJSValueObjectShapeMetadataSurvivesGC(t *testing.T) {
	heapShutdown()
	defer heapShutdown()

	object := heapAllocAtomic(16)
	boxed := jsValueBoxObjectShape(object, 7)
	if got := uint32(jsValueObjectShape(boxed)); got != 7 {
		t.Fatalf("boxed object shape = %d, want 7", got)
	}
	rootSlot := boxed
	root := gcRootRegister(unsafe.Pointer(&rootSlot))
	if root == nil {
		t.Fatal("object shape JSValue root registration failed")
	}
	gcCollect()
	if got := uint32(jsValueObjectShape(boxed)); got != 7 {
		t.Fatalf("boxed object shape after GC = %d, want 7", got)
	}
	if !nativeHeapContains(object) {
		t.Fatal("shape-tagged boxed object lost payload during GC")
	}
	gcRootUnregister(root)
}
