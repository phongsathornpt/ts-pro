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
func TestScalarObjectsSelectsSingleBlockMutableObject(t *testing.T) {
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
	object, ok := ScalarObjects(module, stack).Get(0, 1)
	if !ok || !object.Mutable {
		t.Fatalf("single-block mutable scalar object = %+v, ok=%v", object, ok)
	}
}

func TestScalarObjectsSelectsLinearCrossBlockMutableObject(t *testing.T) {
	module := mir.Module{
		Shapes: []mir.Shape{{ID: 0, Fields: []mir.ShapeField{{Name: "n", Repr: mir.ReprF64}}}},
		Functions: []mir.Function{{ID: 0, Entry: 0, Blocks: []mir.Block{
			{ID: 0, Instructions: []mir.Instruction{{Result: 1, Repr: mir.ReprObjectRef, Op: mir.ObjectAlloc{Shape: 0}}}, Terminator: mir.Jump{Target: 1}},
			{ID: 1, Instructions: []mir.Instruction{
				{Result: 2, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 8}},
				{Result: 3, Repr: mir.ReprF64, Op: mir.FieldSet{Object: 1, Shape: 0, Field: 0, Value: 2}},
			}, Terminator: mir.Jump{Target: 2}},
			{ID: 2, Instructions: []mir.Instruction{{Result: 4, Repr: mir.ReprF64, Op: mir.FieldGet{Object: 1, Shape: 0, Field: 0}}}, Terminator: mir.Return{}},
		}}},
	}
	stack := StackObjects(module, Analyze(module))
	object, ok := ScalarObjects(module, stack).Get(0, 1)
	if !ok || !object.Mutable {
		t.Fatalf("linear mutable scalar object = %+v, ok=%v", object, ok)
	}
	if read := object.Reads[4]; read.Zero || read.Value != 2 {
		t.Fatalf("cross-block field read = %+v, want v2", read)
	}
}

func TestScalarObjectsPlansMutableDiamondPhi(t *testing.T) {
	module := mir.Module{
		Shapes: []mir.Shape{{ID: 0, Fields: []mir.ShapeField{{Name: "n", Repr: mir.ReprF64}}}},
		Functions: []mir.Function{{ID: 0, Entry: 0, Blocks: []mir.Block{
			{ID: 0, Instructions: []mir.Instruction{
				{Result: 0, Repr: mir.ReprBool, Op: mir.ConstBool{Value: true}},
				{Result: 1, Repr: mir.ReprObjectRef, Op: mir.ObjectAlloc{Shape: 0}},
			}, Terminator: mir.Branch{Condition: 0, Then: 1, Else: 2}},
			{ID: 1, Instructions: []mir.Instruction{
				{Result: 2, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 7}},
				{Result: 3, Repr: mir.ReprF64, Op: mir.FieldSet{Object: 1, Shape: 0, Field: 0, Value: 2}},
			}, Terminator: mir.Jump{Target: 3}},
			{ID: 2, Instructions: []mir.Instruction{
				{Result: 4, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 8}},
				{Result: 5, Repr: mir.ReprF64, Op: mir.FieldSet{Object: 1, Shape: 0, Field: 0, Value: 4}},
			}, Terminator: mir.Jump{Target: 3}},
			{ID: 3, Instructions: []mir.Instruction{{Result: 6, Repr: mir.ReprF64, Op: mir.FieldGet{Object: 1, Shape: 0, Field: 0}}}, Terminator: mir.Return{}},
		}}},
	}
	stack := StackObjects(module, Analyze(module))
	object, ok := ScalarObjects(module, stack).Get(0, 1)
	if !ok || !object.Mutable {
		t.Fatalf("diamond scalar object = %+v, ok=%v", object, ok)
	}
	read := object.Reads[6]
	if len(read.Incoming) != 2 || read.Incoming[0].Block != 1 || read.Incoming[0].Value != 2 || read.Incoming[1].Block != 2 || read.Incoming[1].Value != 4 {
		t.Fatalf("diamond field phi plan = %+v", read)
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

func TestScalarObjectsRejectsNestedMutableBranch(t *testing.T) {
	module := mir.Module{
		Shapes: []mir.Shape{{ID: 0, Fields: []mir.ShapeField{{Name: "n", Repr: mir.ReprF64}}}},
		Functions: []mir.Function{{ID: 0, Entry: 0, Blocks: []mir.Block{
			{ID: 0, Instructions: []mir.Instruction{{Result: 0, Repr: mir.ReprBool, Op: mir.ConstBool{Value: true}}, {Result: 1, Repr: mir.ReprObjectRef, Op: mir.ObjectAlloc{Shape: 0}}}, Terminator: mir.Branch{Condition: 0, Then: 1, Else: 2}},
			{ID: 1, Terminator: mir.Branch{Condition: 0, Then: 3, Else: 4}},
			{ID: 2, Terminator: mir.Jump{Target: 5}},
			{ID: 3, Instructions: []mir.Instruction{{Result: 2, Repr: mir.ReprF64, Op: mir.FieldSet{Object: 1, Shape: 0, Field: 0, Value: 9}}}, Terminator: mir.Jump{Target: 5}},
			{ID: 4, Terminator: mir.Jump{Target: 5}},
			{ID: 5, Instructions: []mir.Instruction{{Result: 3, Repr: mir.ReprF64, Op: mir.FieldGet{Object: 1, Shape: 0, Field: 0}}}, Terminator: mir.Return{}},
		}}},
	}
	stack := StackObjects(module, Analyze(module))
	if _, ok := ScalarObjects(module, stack).Get(0, 1); ok {
		t.Fatal("nested mutable branch was selected for scalar replacement")
	}
}
