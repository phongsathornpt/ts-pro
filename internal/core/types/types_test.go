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
	if !TypeNever.AssignableTo(TypeNumber) {
		t.Errorf("never should be assignable to number")
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
}

func TestStructuralSubtyping(t *testing.T) {
	point2D := NewObject("Point2D")
	point2D.AddField("x", TypeNumber, false)
	point2D.AddField("y", TypeNumber, false)

	point3D := NewObject("Point3D")
	point3D.AddField("x", TypeNumber, false)
	point3D.AddField("y", TypeNumber, false)
	point3D.AddField("z", TypeNumber, false)

	// Point3D has all fields of Point2D (plus z) -> Point3D is assignable to Point2D
	if !point3D.AssignableTo(point2D) {
		t.Errorf("Point3D should be assignable to Point2D")
	}
	// Point2D missing 'z' -> Point2D NOT assignable to Point3D
	if point2D.AssignableTo(point3D) {
		t.Errorf("Point2D should NOT be assignable to Point3D")
	}
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
}
