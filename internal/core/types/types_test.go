package types

import (
	"testing"
)

func TestPrimitiveSubtyping(t *testing.T) {
	if !TypeNumber.AssignableTo(TypeNumber) {
		t.Errorf("number should be assignable to number")
	}
	if TypeNumber.AssignableTo(TypeString) {
		t.Errorf("number should NOT be assignable to string")
	}
	if !TypeNumber.AssignableTo(TypeAny) {
		t.Errorf("number should be assignable to any")
	}
	if !TypeNumber.AssignableTo(TypeUnknown) {
		t.Errorf("number should be assignable to unknown")
	}
	if !TypeNever.AssignableTo(TypeNumber) {
		t.Errorf("never should be assignable to number")
	}
	if !TypeAny.AssignableTo(TypeNumber) {
		t.Errorf("any should be assignable to number")
	}
	if TypeNumber.AssignableTo(nil) {
		t.Errorf("number should NOT be assignable to nil")
	}
	if TypeNumber.Equals(nil) {
		t.Errorf("number should NOT equal nil")
	}
	if !TypeNumber.Equals(TypeNumber) {
		t.Errorf("number should equal number")
	}
	if TypeNumber.Equals(TypeString) {
		t.Errorf("number should not equal string")
	}
}

func TestUnionTypes(t *testing.T) {
	u := NewUnion(TypeNumber, TypeString)
	if u.String() != "number | string" {
		t.Errorf("u.String() = %q, want 'number | string'", u.String())
	}
	if !TypeNumber.AssignableTo(u) {
		t.Errorf("number should be assignable to number | string")
	}
	if !TypeString.AssignableTo(u) {
		t.Errorf("string should be assignable to number | string")
	}
	if TypeBoolean.AssignableTo(u) {
		t.Errorf("boolean should NOT be assignable to number | string")
	}
	if u.AssignableTo(nil) {
		t.Errorf("union should not be assignable to nil")
	}
	if !u.AssignableTo(TypeAny) {
		t.Errorf("union should be assignable to any")
	}
	if !u.AssignableTo(NewUnion(TypeNumber, TypeString, TypeBoolean)) {
		t.Errorf("number|string should be assignable to number|string|boolean")
	}
	if u.AssignableTo(TypeNumber) {
		t.Errorf("number|string should not be assignable to number")
	}
	if !u.Equals(NewUnion(TypeString, TypeNumber)) {
		t.Errorf("union equality should be order-independent")
	}
	if u.Equals(TypeNumber) {
		t.Errorf("union should not equal primitive")
	}

	emptyU := NewUnion()
	if !emptyU.Equals(TypeNever) {
		t.Errorf("empty union should be never")
	}
	singleU := NewUnion(TypeNumber)
	if !singleU.Equals(TypeNumber) {
		t.Errorf("single union should be unwrapped")
	}
	nestedU := NewUnion(u, TypeBoolean, TypeNever)
	if nestedU.String() != "number | string | boolean" {
		t.Errorf("nested union string = %q", nestedU.String())
	}
}

func TestStructuralSubtyping(t *testing.T) {
	point2D := NewObject("Point2D")
	point2D.AddField("x", TypeNumber, false)
	point2D.AddField("y", TypeNumber, false)

	point3D := NewObject("Point3D")
	point3D.AddField("x", TypeNumber, false)
	point3D.AddField("y", TypeNumber, false)
	point3D.AddField("z", TypeNumber, false)

	if !point3D.AssignableTo(point2D) {
		t.Errorf("Point3D should be assignable to Point2D")
	}
	if point2D.AssignableTo(point3D) {
		t.Errorf("Point2D should NOT be assignable to Point3D")
	}
	if point2D.AssignableTo(nil) {
		t.Errorf("Point2D should not be assignable to nil")
	}
	if !point2D.AssignableTo(TypeAny) {
		t.Errorf("Point2D should be assignable to any")
	}
	if !point2D.Equals(point2D) {
		t.Errorf("Point2D should equal itself")
	}
	if point2D.Equals(point3D) {
		t.Errorf("Point2D should not equal Point3D")
	}
	if point2D.Equals(TypeNumber) {
		t.Errorf("Point2D should not equal number")
	}

	optObj := NewObject("Opt")
	optObj.AddField("a", TypeString, true)
	emptyObj := NewObject("Empty")
	if !emptyObj.AssignableTo(optObj) {
		t.Errorf("empty object should be assignable to object with optional fields")
	}

	// Anonymous object string
	anonObj := NewObject("")
	anonObj.AddField("foo", TypeNumber, false)
	anonObj.AddField("bar", TypeString, true)
	if anonStr := anonObj.String(); anonStr == "" {
		t.Errorf("anonymous object string should not be empty")
	}
}

func TestArrayAndTupleTypes(t *testing.T) {
	arrNum := NewArray(TypeNumber)
	arrStr := NewArray(TypeString)

	if arrNum.Kind() != KindArray {
		t.Errorf("arrNum.Kind() should be KindArray")
	}
	if !arrNum.Equals(arrNum) {
		t.Errorf("arrNum should equal itself")
	}
	if arrNum.Equals(arrStr) {
		t.Errorf("arrNum should not equal arrStr")
	}
	if arrNum.Equals(TypeNumber) {
		t.Errorf("arrNum should not equal number")
	}
	if arrNum.AssignableTo(nil) {
		t.Errorf("arrNum should not be assignable to nil")
	}
	if !arrNum.AssignableTo(TypeAny) {
		t.Errorf("arrNum should be assignable to any")
	}
	if !arrNum.AssignableTo(NewArray(TypeNumber)) {
		t.Errorf("arrNum should be assignable to Array(number)")
	}
	if arrNum.AssignableTo(arrStr) {
		t.Errorf("arrNum should not be assignable to arrStr")
	}
	if arrNum.String() != "number[]" {
		t.Errorf("arrNum.String() = %q, want 'number[]'", arrNum.String())
	}

	tuple := NewTuple(TypeNumber, TypeString)
	if tuple.Kind() != KindTuple {
		t.Errorf("tuple.Kind() should be KindTuple")
	}
	if !tuple.Equals(tuple) {
		t.Errorf("tuple should equal itself")
	}
	if tuple.Equals(NewTuple(TypeNumber)) {
		t.Errorf("tuples of different lengths should not be equal")
	}
	if tuple.Equals(arrNum) {
		t.Errorf("tuple should not equal array")
	}
	if tuple.AssignableTo(nil) {
		t.Errorf("tuple should not be assignable to nil")
	}
	if !tuple.AssignableTo(TypeAny) {
		t.Errorf("tuple should be assignable to any")
	}
	if !tuple.AssignableTo(NewTuple(TypeNumber, TypeString)) {
		t.Errorf("tuple should be assignable to identical tuple")
	}
	if tuple.AssignableTo(NewTuple(TypeString, TypeNumber)) {
		t.Errorf("tuple should not be assignable to mismatched tuple")
	}
	if tuple.AssignableTo(NewTuple(TypeNumber)) {
		t.Errorf("tuple should not be assignable to shorter tuple")
	}
}

func TestFunctionTypes(t *testing.T) {
	fn1 := NewFunction([]Param{{Name: "x", Type: TypeNumber}}, TypeString)
	fn2 := NewFunction([]Param{{Name: "a", Type: TypeNumber}}, TypeString)
	fn3 := NewFunction([]Param{{Name: "x", Type: TypeString}}, TypeString)

	if fn1.Kind() != KindFunction {
		t.Errorf("fn1.Kind() should be KindFunction")
	}
	if !fn1.Equals(fn2) {
		t.Errorf("fn1 should equal fn2")
	}
	if fn1.Equals(fn3) {
		t.Errorf("fn1 should not equal fn3")
	}
	if fn1.Equals(TypeNumber) {
		t.Errorf("fn1 should not equal number")
	}
	if fn1.AssignableTo(nil) {
		t.Errorf("fn1 should not be assignable to nil")
	}
	if !fn1.AssignableTo(TypeAny) {
		t.Errorf("fn1 should be assignable to any")
	}
	if !fn1.AssignableTo(fn2) {
		t.Errorf("fn1 should be assignable to fn2")
	}
	if fn1.AssignableTo(fn3) {
		t.Errorf("fn1 should not be assignable to fn3")
	}

	thisFn := &FunctionType{This: TypeString, Params: []Param{}, Return: TypeVoid}
	_ = thisFn.String()
	thisFn2 := &FunctionType{This: TypeNumber, Params: []Param{}, Return: TypeVoid}
	if thisFn.Equals(thisFn2) {
		t.Errorf("different this types should not be equal")
	}

	optParamFn := NewFunction([]Param{{Name: "x", Type: TypeNumber, Optional: true, Rest: false}}, TypeVoid)
	restParamFn := NewFunction([]Param{{Name: "x", Type: NewArray(TypeNumber), Optional: false, Rest: true}}, TypeVoid)
	_ = optParamFn.String()
	_ = restParamFn.String()
}

func TestTypeVarsUseDeclarationIdentity(t *testing.T) {
	a := NewTypeVar("T", nil)
	b := NewTypeVar("T", nil)
	if !a.Equals(a) {
		t.Fatal("type variable must equal itself")
	}
	if a.Equals(b) {
		t.Fatal("distinct declarations named T must not be equal")
	}
	constrained := NewTypeVar("N", TypeNumber)
	if !constrained.AssignableTo(TypeNumber) {
		t.Fatal("number-constrained type variable should be assignable to number")
	}
	if constrained.AssignableTo(TypeString) {
		t.Fatal("number-constrained type variable should not be assignable to string")
	}
	if constrained.AssignableTo(nil) {
		t.Fatal("type variable should not be assignable to nil")
	}
	if !constrained.AssignableTo(TypeAny) {
		t.Fatal("type variable should be assignable to any")
	}
	if constrained.Equals(nil) {
		t.Fatal("type variable should not equal nil")
	}
	if constrained.Equals(TypeNumber) {
		t.Fatal("type variable should not equal number")
	}
}

func TestTupleTypeAndGenericSubstitution(t *testing.T) {
	tvT := NewTypeVar("T", nil)
	tvU := NewTypeVar("U", nil)
	fn := NewGenericFunction(
		[]*TypeVar{tvT, tvU},
		[]Param{{Name: "first", Type: tvT}, {Name: "rest", Type: NewArray(tvU)}},
		NewTuple(tvT, tvU),
	)
	inst, err := InstantiateFunction(fn, []Type{TypeString, TypeNumber})
	if err != nil {
		t.Fatalf("InstantiateFunction failed: %v", err)
	}
	if len(inst.TypeParams) != 0 {
		t.Fatalf("instantiated function retained type params: %v", inst.TypeParams)
	}
	if !inst.Params[0].Type.Equals(TypeString) || !inst.Params[1].Type.Equals(NewArray(TypeNumber)) {
		t.Fatalf("unexpected instantiated params: %s", inst)
	}
	want := NewTuple(TypeString, TypeNumber)
	if !inst.Return.Equals(want) {
		t.Fatalf("return = %s, want %s", inst.Return, want)
	}
	if got := want.String(); got != "[string, number]" {
		t.Fatalf("tuple string = %q", got)
	}
	if !want.AssignableTo(NewArray(NewUnion(TypeString, TypeNumber))) {
		t.Fatal("heterogeneous tuple should be assignable to compatible union array")
	}

	// Arity mismatch
	if _, err := InstantiateFunction(fn, []Type{TypeString}); err == nil {
		t.Fatal("expected arity error in InstantiateFunction")
	}
	// Constraint violation
	tvConstrained := NewTypeVar("C", TypeNumber)
	cFn := NewGenericFunction([]*TypeVar{tvConstrained}, []Param{}, TypeVoid)
	if _, err := InstantiateFunction(cFn, []Type{TypeString}); err == nil {
		t.Fatal("expected constraint violation error")
	}
}

func TestInferGenericFunction(t *testing.T) {
	tv := NewTypeVar("T", nil)
	fn := NewGenericFunction([]*TypeVar{tv}, []Param{{Name: "value", Type: tv}}, tv)
	inst, err := InferFunction(fn, []Type{TypeNumber})
	if err != nil {
		t.Fatalf("InferFunction failed: %v", err)
	}
	if !inst.Params[0].Type.Equals(TypeNumber) || !inst.Return.Equals(TypeNumber) {
		t.Fatalf("unexpected inferred function: %s", inst)
	}

	tvElem := NewTypeVar("E", nil)
	arrayFn := NewGenericFunction([]*TypeVar{tvElem}, []Param{{Name: "items", Type: NewArray(tvElem)}}, tvElem)
	arrayInst, err := InferFunction(arrayFn, []Type{NewArray(TypeString)})
	if err != nil {
		t.Fatalf("array inference failed: %v", err)
	}
	if !arrayInst.Return.Equals(TypeString) {
		t.Fatalf("array inference return = %s", arrayInst.Return)
	}

	// Tuple inference
	tvA := NewTypeVar("A", nil)
	tupleFn := NewGenericFunction([]*TypeVar{tvA}, []Param{{Name: "t", Type: NewTuple(tvA, TypeNumber)}}, tvA)
	tupleInst, err := InferFunction(tupleFn, []Type{NewTuple(TypeString, TypeNumber)})
	if err != nil {
		t.Fatalf("tuple inference failed: %v", err)
	}
	if !tupleInst.Return.Equals(TypeString) {
		t.Fatalf("tuple inference return = %s", tupleInst.Return)
	}

	// Object inference
	objPattern := NewObject("Pattern")
	objPattern.AddField("val", tvA, false)
	objFn := NewGenericFunction([]*TypeVar{tvA}, []Param{{Name: "o", Type: objPattern}}, tvA)
	argObj := NewObject("Arg")
	argObj.AddField("val", TypeBoolean, false)
	objInst, err := InferFunction(objFn, []Type{argObj})
	if err != nil {
		t.Fatalf("object inference failed: %v", err)
	}
	if !objInst.Return.Equals(TypeBoolean) {
		t.Fatalf("object inference return = %s", objInst.Return)
	}

	// Function inference
	fnPattern := NewGenericFunction([]*TypeVar{tvA}, []Param{{Name: "cb", Type: NewFunction([]Param{{Name: "x", Type: tvA}}, TypeVoid)}}, tvA)
	fnArg := NewFunction([]Param{{Name: "x", Type: TypeNumber}}, TypeVoid)
	fnInst, err := InferFunction(fnPattern, []Type{fnArg})
	if err != nil {
		t.Fatalf("function inference failed: %v", err)
	}
	if !fnInst.Return.Equals(TypeNumber) {
		t.Fatalf("function inference return = %s", fnInst.Return)
	}
}

func TestInferGenericFunctionRejectsConflictingBindings(t *testing.T) {
	tv := NewTypeVar("T", nil)
	fn := NewGenericFunction([]*TypeVar{tv}, []Param{{Name: "a", Type: tv}, {Name: "b", Type: tv}}, tv)
	if _, err := InferFunction(fn, []Type{TypeNumber, TypeString}); err == nil {
		t.Fatal("expected conflicting generic inference to fail")
	}

	// Uninferred parameter error
	if _, err := InferFunction(fn, []Type{}); err == nil {
		t.Fatal("expected inference error for empty actual args")
	}
}

func TestFunctionBindings(t *testing.T) {
	tvT := NewTypeVar("T", nil)
	tvU := NewTypeVar("U", nil)
	generic := NewGenericFunction(
		[]*TypeVar{tvT, tvU},
		[]Param{{Name: "a", Type: tvT}, {Name: "b", Type: NewArray(tvU)}},
		NewTuple(tvT, tvU),
	)
	concrete, err := InstantiateFunction(generic, []Type{TypeNumber, TypeString})
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := FunctionBindings(generic, concrete)
	if err != nil {
		t.Fatalf("FunctionBindings failed: %v", err)
	}
	if !bindings[tvT].Equals(TypeNumber) || !bindings[tvU].Equals(TypeString) {
		t.Fatalf("unexpected bindings: T=%v U=%v", bindings[tvT], bindings[tvU])
	}

	// Non-generic function bindings
	nonGen := NewFunction([]Param{}, TypeVoid)
	b2, err := FunctionBindings(nonGen, nonGen)
	if err != nil || len(b2) != 0 {
		t.Fatalf("expected empty bindings for non-generic function, got %v, err %v", b2, err)
	}
}

func TestTypesComprehensive(t *testing.T) {
	tv := NewTypeVar("T", nil)
	if tv.Kind() != KindTypeVar {
		t.Errorf("tv.Kind() = %v, want KindTypeVar", tv.Kind())
	}
	if !tv.Equals(tv) {
		t.Errorf("tv should equal itself")
	}

	tuple1 := NewTuple(TypeNumber, TypeString)
	tuple2 := NewTuple(TypeNumber, TypeString)
	if !tuple1.AssignableTo(tuple2) {
		t.Errorf("tuple1 should be assignable to tuple2")
	}

	fn := NewFunction([]Param{{Name: "a", Type: TypeNumber}}, TypeString)
	if fn.Kind() != KindFunction {
		t.Errorf("fn.Kind() = %v, want KindFunction", fn.Kind())
	}
	fnThis := &FunctionType{This: TypeNumber, Params: []Param{{Name: "b", Type: TypeNumber}}, Return: TypeString}
	if fn.Equals(fnThis) {
		t.Errorf("fn without this should not equal fn with this")
	}

	obj1 := NewObject("Obj1")
	obj1.AddField("x", TypeNumber, false)
	obj2 := NewObject("Obj2")
	obj2.AddField("x", TypeNumber, false)
	obj2.AddField("y", TypeString, true)
	if !obj1.AssignableTo(obj2) {
		t.Errorf("obj1 should be assignable to obj2 with optional field")
	}

	u := NewUnion(TypeNumber, TypeString)
	if !u.AssignableTo(u) {
		t.Errorf("union should be assignable to itself")
	}
	if !TypeNumber.AssignableTo(u) {
		t.Errorf("number should be assignable to number | string")
	}

	bound := Substitute(tv, map[*TypeVar]Type{tv: TypeNumber})
	if !bound.Equals(TypeNumber) {
		t.Errorf("Substitute failed: got %v, want number", bound)
	}

	objWithTV := NewObject("WithTV")
	objWithTV.AddField("item", tv, false)
	subObj := Substitute(objWithTV, map[*TypeVar]Type{tv: TypeString}).(*ObjectType)
	if !subObj.Fields["item"].Type.Equals(TypeString) {
		t.Errorf("Substitute on object failed: got %v", subObj)
	}
}
