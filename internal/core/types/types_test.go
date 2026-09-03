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
