package escape

import (
	"testing"

	"github.com/projectthorn/tsv7-bin/internal/mir"
)

func TestScalarObjectsSelectsImmutableObjectNew(t *testing.T) {
	module := mir.Module{
		Shapes: []mir.Shape{{ID: 0, Fields: []mir.ShapeField{{Name: "n", Repr: mir.ReprF64}}}},
		Functions: []mir.Function{{ID: 0, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			{Result: 0, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 7}},
			{Result: 1, Repr: mir.ReprObjectRef, Op: mir.ObjectNew{Shape: 0, Fields: []mir.ValueID{0}}},
			{Result: 2, Repr: mir.ReprF64, Op: mir.FieldGet{Object: 1, Shape: 0, Field: 0}},
		}, Terminator: mir.Return{}}}}},
	}
	stack := StackObjects(module, Analyze(module))
	object, ok := ScalarObjects(module, stack).Get(0, 1)
	if !ok || object.Shape != 0 || len(object.Fields) != 1 || object.Fields[0] != 0 {
		t.Fatalf("scalar object = %+v, ok=%v", object, ok)
	}
}
func TestScalarObjectsRejectsMutableObject(t *testing.T) {
	module := mir.Module{
		Shapes: []mir.Shape{{ID: 0, Fields: []mir.ShapeField{{Name: "n", Repr: mir.ReprF64}}}},
		Functions: []mir.Function{{ID: 0, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			{Result: 0, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 7}},
			{Result: 1, Repr: mir.ReprObjectRef, Op: mir.ObjectNew{Shape: 0, Fields: []mir.ValueID{0}}},
			{Result: 2, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 8}},
			{Result: 3, Repr: mir.ReprF64, Op: mir.FieldSet{Object: 1, Shape: 0, Field: 0, Value: 2}},
		}, Terminator: mir.Return{}}}}},
	}
	stack := StackObjects(module, Analyze(module))
	if _, ok := ScalarObjects(module, stack).Get(0, 1); ok {
		t.Fatal("mutable object was selected for scalar replacement")
	}
}

func TestScalarObjectsRejectsAliasedObject(t *testing.T) {
	module := mir.Module{
		Shapes: []mir.Shape{{ID: 0, Fields: []mir.ShapeField{{Name: "n", Repr: mir.ReprF64}}}},
		Functions: []mir.Function{{ID: 0, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			{Result: 0, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 7}},
			{Result: 1, Repr: mir.ReprObjectRef, Op: mir.ObjectNew{Shape: 0, Fields: []mir.ValueID{0}}},
			{Result: 2, Repr: mir.ReprObjectRef, Op: mir.Phi{Incoming: []mir.PhiIncoming{{Block: 0, Value: 1}}}},
		}, Terminator: mir.Return{}}}}},
	}
	stack := StackObjects(module, Analyze(module))
	if _, ok := ScalarObjects(module, stack).Get(0, 1); ok {
		t.Fatal("aliased object was selected for scalar replacement")
	}
}
