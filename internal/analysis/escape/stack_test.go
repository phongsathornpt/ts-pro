package escape

import (
	"testing"

	"github.com/projectthorn/tsv7-bin/internal/mir"
)

func TestStackObjectsAcceptsLocalAtomicObject(t *testing.T) {
	module := mir.Module{
		Shapes: []mir.Shape{{ID: 0, Fields: []mir.ShapeField{{Name: "n", Repr: mir.ReprF64}}}},
		Functions: []mir.Function{{ID: 0, Entry: 0, Blocks: []mir.Block{{
			ID: 0, Instructions: []mir.Instruction{objectAlloc(0)}, Terminator: mir.Return{},
		}}}},
	}
	escapes := Analyze(module)
	if !StackObjects(module, escapes).Contains(0, 0) {
		t.Fatal("local atomic object was not selected for stack allocation")
	}
}

func TestStackObjectsAcceptsLocalClosure(t *testing.T) {
	module := mir.Module{Functions: []mir.Function{
		{ID: 0, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			{Result: 0, Repr: mir.ReprFunctionRef, Op: mir.ClosureNew{Callee: 1}},
			{Result: 1, Repr: mir.ReprVoid, Op: mir.ClosureCall{Closure: 0}},
		}, Terminator: mir.Return{}}}},
		{ID: 1, Entry: 0, Blocks: []mir.Block{{ID: 0, Terminator: mir.Return{}}}},
	}}
	if !StackObjects(module, Analyze(module)).Contains(0, 0) {
		t.Fatal("local non-escaping closure was not selected for stack allocation")
	}
}

func TestStackObjectsRejectsReturnedClosure(t *testing.T) {
	ret := mir.ValueID(0)
	module := mir.Module{Functions: []mir.Function{
		{ID: 0, ReturnRepr: mir.ReprFunctionRef, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			{Result: 0, Repr: mir.ReprFunctionRef, Op: mir.ClosureNew{Callee: 1}},
		}, Terminator: mir.Return{Value: &ret}}}},
		{ID: 1, Entry: 0, Blocks: []mir.Block{{ID: 0, Terminator: mir.Return{}}}},
	}}
	if StackObjects(module, Analyze(module)).Contains(0, 0) {
		t.Fatal("returned closure was selected for stack allocation")
	}
}

func TestStackObjectsAcceptsReferenceShape(t *testing.T) {
	module := mir.Module{
		Shapes:    []mir.Shape{{ID: 0, Fields: []mir.ShapeField{{Name: "ref", Repr: mir.ReprObjectRef}}}},
		Functions: []mir.Function{{ID: 0, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{objectAlloc(0)}, Terminator: mir.Return{}}}}},
	}
	if !StackObjects(module, Analyze(module)).Contains(0, 0) {
		t.Fatal("reference-bearing object was not selected for stack allocation")
	}
}

func TestStackObjectsAcceptsSingleOriginAliasedReferenceObject(t *testing.T) {
	module := mir.Module{
		Shapes: []mir.Shape{{ID: 0, Fields: []mir.ShapeField{{Name: "ref", Repr: mir.ReprObjectRef}}}},
		Functions: []mir.Function{{ID: 0, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			objectAlloc(0),
			{Result: 1, Repr: mir.ReprObjectRef, Op: mir.Phi{Incoming: []mir.PhiIncoming{{Block: 0, Value: 0}}}},
		}, Terminator: mir.Return{}}}}},
	}
	stack := StackObjects(module, Analyze(module))
	if !stack.Contains(0, 0) {
		t.Fatal("single-origin aliased reference-bearing object was not selected for stack allocation")
	}
	if origin, ok := StackObjectAliases(module, stack)[0][1]; !ok || origin != 0 {
		t.Fatalf("stack alias v1 origin = v%d, ok=%v; want v0", origin, ok)
	}
}

func TestStackObjectsRejectsUnknownMixedAlias(t *testing.T) {
	module := mir.Module{
		Shapes: []mir.Shape{{ID: 0, Fields: []mir.ShapeField{{Name: "ref", Repr: mir.ReprObjectRef}}}},
		Functions: []mir.Function{{ID: 0, Params: []mir.Param{{Value: 9, Repr: mir.ReprObjectRef}}, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			objectAlloc(0),
			{Result: 1, Repr: mir.ReprObjectRef, Op: mir.Phi{Incoming: []mir.PhiIncoming{{Block: 0, Value: 0}, {Block: 0, Value: 9}}}},
		}, Terminator: mir.Return{}}}}},
	}
	if StackObjects(module, Analyze(module)).Contains(0, 0) {
		t.Fatal("object participating in an unknown/mixed alias was selected for stack allocation")
	}
}
func TestStackObjectsRejectsCyclicAllocationBlock(t *testing.T) {
	module := mir.Module{
		Shapes: []mir.Shape{{ID: 0, Fields: []mir.ShapeField{{Name: "n", Repr: mir.ReprF64}}}},
		Functions: []mir.Function{{ID: 0, Entry: 0, Blocks: []mir.Block{
			{ID: 0, Terminator: mir.Jump{Target: 1}},
			{ID: 1, Instructions: []mir.Instruction{objectAlloc(0)}, Terminator: mir.Jump{Target: 1}},
		}}},
	}
	if StackObjects(module, Analyze(module)).Contains(0, 0) {
		t.Fatal("loop allocation was selected for stack allocation")
	}
}

func TestStackObjectsRejectsObjectEmbeddedIntoHeapContainer(t *testing.T) {
	module := mir.Module{
		Shapes: []mir.Shape{
			{ID: 0, Fields: []mir.ShapeField{{Name: "n", Repr: mir.ReprF64}}},
			{ID: 1, Fields: []mir.ShapeField{{Name: "child", Repr: mir.ReprObjectRef}}},
		},
		Functions: []mir.Function{{ID: 0, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			objectAlloc(0),
			{Result: 1, Repr: mir.ReprObjectRef, Op: mir.ObjectNew{Shape: 1, Fields: []mir.ValueID{0}}},
		}, Terminator: mir.Return{}}}}},
	}
	if StackObjects(module, Analyze(module)).Contains(0, 0) {
		t.Fatal("object embedded into heap container was selected for stack allocation")
	}
}
func TestStackObjectsRejectsClosureCapturedObject(t *testing.T) {
	module := mir.Module{
		Shapes: []mir.Shape{{ID: 0, Fields: []mir.ShapeField{{Name: "n", Repr: mir.ReprF64}}}},
		Functions: []mir.Function{{ID: 0, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			objectAlloc(0),
			{Result: 1, Repr: mir.ReprFunctionRef, Op: mir.ClosureNew{Callee: 1, Captures: []mir.ValueID{0}}},
		}, Terminator: mir.Return{}}}}, {ID: 1, Params: []mir.Param{{Value: 0, Repr: mir.ReprObjectRef}}, Entry: 0, Blocks: []mir.Block{{ID: 0, Terminator: mir.Return{}}}}},
	}
	if StackObjects(module, Analyze(module)).Contains(0, 0) {
		t.Fatal("closure-captured object was selected for stack allocation")
	}
}

func TestStackObjectsKeepsAcyclicPreheaderCandidate(t *testing.T) {
	module := mir.Module{
		Shapes: []mir.Shape{{ID: 0, Fields: []mir.ShapeField{{Name: "n", Repr: mir.ReprF64}}}},
		Functions: []mir.Function{{ID: 0, Entry: 0, Blocks: []mir.Block{
			{ID: 0, Instructions: []mir.Instruction{objectAlloc(0)}, Terminator: mir.Jump{Target: 1}},
			{ID: 1, Terminator: mir.Branch{Condition: 9, Then: 1, Else: 2}},
			{ID: 2, Terminator: mir.Return{}},
		}}},
	}
	if !StackObjects(module, Analyze(module)).Contains(0, 0) {
		t.Fatal("acyclic preheader allocation was unnecessarily rejected")
	}
}
