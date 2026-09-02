package escape

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/mir"
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
	if read.Phi == "" {
		t.Fatalf("diamond field read has no phi source: %+v", read)
	}
	phi, ok := object.Phis[read.Phi]
	if !ok || phi.Block != 3 || len(phi.Incoming) != 2 || phi.Incoming[0].Block != 1 || phi.Incoming[0].Source.Value != 2 || phi.Incoming[1].Block != 2 || phi.Incoming[1].Source.Value != 4 {
		t.Fatalf("diamond field phi plan = %+v, ok=%v", phi, ok)
	}
}

func TestScalarObjectsSelectsReferenceBearingStackCandidate(t *testing.T) {
	module := mir.Module{
		Shapes: []mir.Shape{{ID: 0, Fields: []mir.ShapeField{{Name: "value", Repr: mir.ReprStringRef}}}},
		Functions: []mir.Function{{ID: 0, ReturnRepr: mir.ReprStringRef, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			{Result: 0, Repr: mir.ReprStringRef, Op: mir.ConstString{Value: "value"}},
			{Result: 1, Repr: mir.ReprObjectRef, Op: mir.ObjectNew{Shape: 0, Fields: []mir.ValueID{0}}},
			{Result: 2, Repr: mir.ReprStringRef, Op: mir.FieldGet{Object: 1, Shape: 0, Field: 0}},
		}, Terminator: mir.Return{}}}}},
	}
	escapes := Analyze(module)
	stack := StackObjects(module, escapes)
	if !stack.Contains(0, 1) {
		t.Fatal("reference-bearing object was not selected as a stack candidate")
	}
	object, ok := ScalarObjectsWithEscapeAnalysis(module, stack, escapes).Get(0, 1)
	if !ok || object.Mutable || len(object.Fields) != 1 || object.Fields[0] != 0 {
		t.Fatalf("reference-bearing scalar object = %+v, ok=%v", object, ok)
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

func TestScalarObjectsPlansNestedMutableBranch(t *testing.T) {
	module := mir.Module{
		Shapes: []mir.Shape{{ID: 0, Fields: []mir.ShapeField{{Name: "n", Repr: mir.ReprF64}}}},
		Functions: []mir.Function{{ID: 0, Entry: 0, Blocks: []mir.Block{
			{ID: 0, Instructions: []mir.Instruction{{Result: 0, Repr: mir.ReprBool, Op: mir.ConstBool{Value: true}}, {Result: 1, Repr: mir.ReprObjectRef, Op: mir.ObjectAlloc{Shape: 0}}}, Terminator: mir.Branch{Condition: 0, Then: 1, Else: 2}},
			{ID: 1, Terminator: mir.Branch{Condition: 0, Then: 3, Else: 4}},
			{ID: 2, Terminator: mir.Jump{Target: 5}},
			{ID: 3, Instructions: []mir.Instruction{{Result: 2, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 9}}, {Result: 3, Repr: mir.ReprF64, Op: mir.FieldSet{Object: 1, Shape: 0, Field: 0, Value: 2}}}, Terminator: mir.Jump{Target: 5}},
			{ID: 4, Terminator: mir.Jump{Target: 5}},
			{ID: 5, Instructions: []mir.Instruction{{Result: 4, Repr: mir.ReprF64, Op: mir.FieldGet{Object: 1, Shape: 0, Field: 0}}}, Terminator: mir.Return{}},
		}}},
	}
	stack := StackObjects(module, Analyze(module))
	object, ok := ScalarObjects(module, stack).Get(0, 1)
	if !ok || !object.Mutable {
		t.Fatalf("nested mutable scalar object = %+v, ok=%v", object, ok)
	}
	read := object.Reads[4]
	if read.Phi == "" {
		t.Fatalf("nested field read has no phi source: %+v", read)
	}
	phi := object.Phis[read.Phi]
	if phi.Block != 5 || len(phi.Incoming) != 3 {
		t.Fatalf("nested merge phi = %+v", phi)
	}
}

func TestScalarObjectsRejectsLoopCarriedMutableState(t *testing.T) {
	module := mir.Module{
		Shapes: []mir.Shape{{ID: 0, Fields: []mir.ShapeField{{Name: "n", Repr: mir.ReprF64}}}},
		Functions: []mir.Function{{ID: 0, Entry: 0, Blocks: []mir.Block{
			{ID: 0, Instructions: []mir.Instruction{{Result: 0, Repr: mir.ReprObjectRef, Op: mir.ObjectAlloc{Shape: 0}}}, Terminator: mir.Jump{Target: 1}},
			{ID: 1, Instructions: []mir.Instruction{{Result: 1, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 1}}, {Result: 2, Repr: mir.ReprF64, Op: mir.FieldSet{Object: 0, Shape: 0, Field: 0, Value: 1}}, {Result: 3, Repr: mir.ReprF64, Op: mir.FieldGet{Object: 0, Shape: 0, Field: 0}}}, Terminator: mir.Jump{Target: 1}},
		}}},
	}
	stack := StackObjects(module, Analyze(module))
	if _, ok := ScalarObjects(module, stack).Get(0, 0); ok {
		t.Fatal("loop-carried mutable object was selected for scalar replacement")
	}
}

func TestScalarObjectsPlansNestedPhiChain(t *testing.T) {
	module := mir.Module{
		Shapes: []mir.Shape{{ID: 0, Fields: []mir.ShapeField{{Name: "n", Repr: mir.ReprF64}}}},
		Functions: []mir.Function{{ID: 0, Entry: 0, Blocks: []mir.Block{
			{ID: 0, Instructions: []mir.Instruction{{Result: 0, Repr: mir.ReprObjectRef, Op: mir.ObjectAlloc{Shape: 0}}, {Result: 1, Repr: mir.ReprBool, Op: mir.ConstBool{Value: true}}}, Terminator: mir.Branch{Condition: 1, Then: 1, Else: 2}},
			{ID: 1, Terminator: mir.Branch{Condition: 1, Then: 3, Else: 4}},
			{ID: 2, Instructions: []mir.Instruction{{Result: 2, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 9}}, {Result: 3, Repr: mir.ReprF64, Op: mir.FieldSet{Object: 0, Shape: 0, Field: 0, Value: 2}}}, Terminator: mir.Jump{Target: 6}},
			{ID: 3, Instructions: []mir.Instruction{{Result: 4, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 7}}, {Result: 5, Repr: mir.ReprF64, Op: mir.FieldSet{Object: 0, Shape: 0, Field: 0, Value: 4}}}, Terminator: mir.Jump{Target: 5}},
			{ID: 4, Instructions: []mir.Instruction{{Result: 6, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 8}}, {Result: 7, Repr: mir.ReprF64, Op: mir.FieldSet{Object: 0, Shape: 0, Field: 0, Value: 6}}}, Terminator: mir.Jump{Target: 5}},
			{ID: 5, Terminator: mir.Jump{Target: 6}},
			{ID: 6, Instructions: []mir.Instruction{{Result: 8, Repr: mir.ReprF64, Op: mir.FieldGet{Object: 0, Shape: 0, Field: 0}}}, Terminator: mir.Return{}},
		}}},
	}
	stack := StackObjects(module, Analyze(module))
	object, ok := ScalarObjects(module, stack).Get(0, 0)
	if !ok || !object.Mutable {
		t.Fatalf("nested phi scalar object = %+v, ok=%v", object, ok)
	}
	inner, innerOK := object.Phis["scalar.phi.v0.f0.b5"]
	outer, outerOK := object.Phis["scalar.phi.v0.f0.b6"]
	if !innerOK || len(inner.Incoming) != 2 || !outerOK || len(outer.Incoming) != 2 {
		t.Fatalf("nested phi plans inner=%+v/%v outer=%+v/%v", inner, innerOK, outer, outerOK)
	}
	if outer.Incoming[1].Source.Phi != inner.Name || object.Reads[8].Phi != outer.Name {
		t.Fatalf("nested phi chain outer=%+v read=%+v", outer, object.Reads[8])
	}
}
