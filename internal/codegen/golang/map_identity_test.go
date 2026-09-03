package golang

import (
	"strings"
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/mir"
)

func TestEmitPureGoMapArrayKeyIdentity(t *testing.T) {
	entry := mir.FunctionID(0)
	module := mir.Module{
		Name:  "map-array-key-test",
		Entry: &entry,
		Functions: []mir.Function{
			{
				ID: 0, Name: "main", ReturnRepr: mir.ReprVoid, Entry: 0,
				Blocks: []mir.Block{{
					ID: 0,
					Instructions: []mir.Instruction{
						// arr1 = [1, 2, 3]
						{Result: 0, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 1}},
						{Result: 1, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 2}},
						{Result: 2, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 3}},
						{Result: 3, Repr: mir.ReprArrayRef, Op: mir.ArrayNewF64{Elements: []mir.ValueID{0, 1, 2}}},

						// arr2 = [1, 2, 3] (distinct allocation)
						{Result: 4, Repr: mir.ReprArrayRef, Op: mir.ArrayNewF64{Elements: []mir.ValueID{0, 1, 2}}},

						// m = new Map()
						{Result: 5, Repr: mir.ReprObjectRef, Op: mir.MapOp{Kind: mir.MapOpNew}},

						// val = 42
						{Result: 6, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 42}},

						// m.set(arr1, 42)
						{Result: 7, Repr: mir.ReprObjectRef, Op: mir.MapOp{Kind: mir.MapOpSet, Map: 5, Key: 3, Value: 6}},

						// m.has(arr1) -> bool
						{Result: 8, Repr: mir.ReprBool, Op: mir.MapOp{Kind: mir.MapOpHas, Map: 5, Key: 3}},
						// m.has(arr2) -> bool
						{Result: 9, Repr: mir.ReprBool, Op: mir.MapOp{Kind: mir.MapOpHas, Map: 5, Key: 4}},

						// m.get(arr1) -> 42
						{Result: 10, Repr: mir.ReprF64, Op: mir.MapOp{Kind: mir.MapOpGet, Map: 5, Key: 3}},
						{Result: 11, Repr: mir.ReprVoid, Op: mir.IntrinsicCall{Intrinsic: mir.IntrinsicConsoleLogF64, Args: []mir.ValueID{10}}},

						// m.size -> 1
						{Result: 12, Repr: mir.ReprF64, Op: mir.MapOp{Kind: mir.MapOpSize, Map: 5}},
						{Result: 13, Repr: mir.ReprVoid, Op: mir.IntrinsicCall{Intrinsic: mir.IntrinsicConsoleLogF64, Args: []mir.ValueID{12}}},

						// s = new Set()
						{Result: 14, Repr: mir.ReprObjectRef, Op: mir.SetOp{Kind: mir.SetOpNew}},
						// s.add(arr1)
						{Result: 15, Repr: mir.ReprObjectRef, Op: mir.SetOp{Kind: mir.SetOpAdd, Set: 14, Item: 3}},
						// s.has(arr1) -> bool
						{Result: 16, Repr: mir.ReprBool, Op: mir.SetOp{Kind: mir.SetOpHas, Set: 14, Item: 3}},
						// s.has(arr2) -> bool
						{Result: 17, Repr: mir.ReprBool, Op: mir.SetOp{Kind: mir.SetOpHas, Set: 14, Item: 4}},

						// s.size -> 1
						{Result: 18, Repr: mir.ReprF64, Op: mir.SetOp{Kind: mir.SetOpSize, Set: 14}},
						{Result: 19, Repr: mir.ReprVoid, Op: mir.IntrinsicCall{Intrinsic: mir.IntrinsicConsoleLogF64, Args: []mir.ValueID{18}}},
					},
					Terminator: mir.Return{},
				}},
			},
		},
	}

	out := runEmittedModule(t, module)
	got := strings.TrimSpace(out)
	want := "42\n1\n1"
	if got != want {
		t.Fatalf("got output:\n%s\nwant:\n%s", got, want)
	}
}

func TestEmitPureGoMapNaNAndSignedZero(t *testing.T) {
	entry := mir.FunctionID(0)
	module := mir.Module{
		Name:  "map-nan-zero-test",
		Entry: &entry,
		Functions: []mir.Function{
			{
				ID: 0, Name: "main", ReturnRepr: mir.ReprVoid, Entry: 0,
				Blocks: []mir.Block{{
					ID: 0,
					Instructions: []mir.Instruction{
						// m = new Map()
						{Result: 0, Repr: mir.ReprObjectRef, Op: mir.MapOp{Kind: mir.MapOpNew}},

						// val = 99
						{Result: 1, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 99}},

						// nan1 = 0 / 0
						{Result: 2, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 0}},
						{Result: 3, Repr: mir.ReprF64, Op: mir.FloatBinary{Operator: mir.FloatDiv, Left: 2, Right: 2}},

						// m.set(nan1, 99)
						{Result: 4, Repr: mir.ReprObjectRef, Op: mir.MapOp{Kind: mir.MapOpSet, Map: 0, Key: 3, Value: 1}},

						// nan2 = 0 / 0
						{Result: 5, Repr: mir.ReprF64, Op: mir.FloatBinary{Operator: mir.FloatDiv, Left: 2, Right: 2}},

						// m.get(nan2) -> 99
						{Result: 6, Repr: mir.ReprF64, Op: mir.MapOp{Kind: mir.MapOpGet, Map: 0, Key: 5}},
						{Result: 7, Repr: mir.ReprVoid, Op: mir.IntrinsicCall{Intrinsic: mir.IntrinsicConsoleLogF64, Args: []mir.ValueID{6}}},

						// m.size -> 1
						{Result: 8, Repr: mir.ReprF64, Op: mir.MapOp{Kind: mir.MapOpSize, Map: 0}},
						{Result: 9, Repr: mir.ReprVoid, Op: mir.IntrinsicCall{Intrinsic: mir.IntrinsicConsoleLogF64, Args: []mir.ValueID{8}}},

						// zeroPlus = +0.0, zeroMinus = -0.0
						{Result: 10, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 0}},
						{Result: 11, Repr: mir.ReprF64, Op: mir.ConstF64{Value: -1}},
						{Result: 12, Repr: mir.ReprF64, Op: mir.FloatBinary{Operator: mir.FloatMul, Left: 10, Right: 11}},

						// m.set(zeroPlus, 100)
						{Result: 13, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 100}},
						{Result: 14, Repr: mir.ReprObjectRef, Op: mir.MapOp{Kind: mir.MapOpSet, Map: 0, Key: 10, Value: 13}},

						// m.get(zeroMinus) -> 100
						{Result: 15, Repr: mir.ReprF64, Op: mir.MapOp{Kind: mir.MapOpGet, Map: 0, Key: 12}},
						{Result: 16, Repr: mir.ReprVoid, Op: mir.IntrinsicCall{Intrinsic: mir.IntrinsicConsoleLogF64, Args: []mir.ValueID{15}}},
					},
					Terminator: mir.Return{},
				}},
			},
		},
	}

	out := runEmittedModule(t, module)
	got := strings.TrimSpace(out)
	want := "99\n1\n100"
	if got != want {
		t.Fatalf("got output:\n%s\nwant:\n%s", got, want)
	}
}

func TestEmitPureGoMapEmptyArrayKeyIdentity(t *testing.T) {
	entry := mir.FunctionID(0)
	module := mir.Module{
		Name:  "map-empty-array-test",
		Entry: &entry,
		Functions: []mir.Function{
			{
				ID: 0, Name: "main", ReturnRepr: mir.ReprVoid, Entry: 0,
				Blocks: []mir.Block{{
					ID: 0,
					Instructions: []mir.Instruction{
						// arr1 = []
						{Result: 0, Repr: mir.ReprArrayRef, Op: mir.ArrayNewF64{Elements: nil}},
						// arr2 = []
						{Result: 1, Repr: mir.ReprArrayRef, Op: mir.ArrayNewF64{Elements: nil}},
						// m = new Map()
						{Result: 2, Repr: mir.ReprObjectRef, Op: mir.MapOp{Kind: mir.MapOpNew}},
						// val1 = 10
						{Result: 3, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 10}},
						// val2 = 20
						{Result: 4, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 20}},
						// m.set(arr1, 10)
						{Result: 5, Repr: mir.ReprObjectRef, Op: mir.MapOp{Kind: mir.MapOpSet, Map: 2, Key: 0, Value: 3}},
						// m.set(arr2, 20)
						{Result: 6, Repr: mir.ReprObjectRef, Op: mir.MapOp{Kind: mir.MapOpSet, Map: 2, Key: 1, Value: 4}},
						// m.size -> 2
						{Result: 7, Repr: mir.ReprF64, Op: mir.MapOp{Kind: mir.MapOpSize, Map: 2}},
						{Result: 8, Repr: mir.ReprVoid, Op: mir.IntrinsicCall{Intrinsic: mir.IntrinsicConsoleLogF64, Args: []mir.ValueID{7}}},
						// m.get(arr1) -> 10
						{Result: 9, Repr: mir.ReprF64, Op: mir.MapOp{Kind: mir.MapOpGet, Map: 2, Key: 0}},
						{Result: 10, Repr: mir.ReprVoid, Op: mir.IntrinsicCall{Intrinsic: mir.IntrinsicConsoleLogF64, Args: []mir.ValueID{9}}},
						// m.get(arr2) -> 20
						{Result: 11, Repr: mir.ReprF64, Op: mir.MapOp{Kind: mir.MapOpGet, Map: 2, Key: 1}},
						{Result: 12, Repr: mir.ReprVoid, Op: mir.IntrinsicCall{Intrinsic: mir.IntrinsicConsoleLogF64, Args: []mir.ValueID{11}}},
					},
					Terminator: mir.Return{},
				}},
			},
		},
	}

	out := runEmittedModule(t, module)
	got := strings.TrimSpace(out)
	want := "2\n10\n20"
	if got != want {
		t.Fatalf("got output:\n%s\nwant:\n%s", got, want)
	}
}

func TestEmitPureGoMapEmptyConcatKeyIdentity(t *testing.T) {
	entry := mir.FunctionID(0)
	module := mir.Module{
		Name:  "map-empty-concat-test",
		Entry: &entry,
		Functions: []mir.Function{
			{
				ID: 0, Name: "main", ReturnRepr: mir.ReprVoid, Entry: 0,
				Blocks: []mir.Block{{
					ID: 0,
					Instructions: []mir.Instruction{
						// arr1 = concat() (F64)
						{Result: 0, Repr: mir.ReprArrayRef, Op: mir.ArrayConcatF64{Arrays: nil}},
						// arr2 = concat() (F64)
						{Result: 1, Repr: mir.ReprArrayRef, Op: mir.ArrayConcatF64{Arrays: nil}},
						// m = new Map()
						{Result: 2, Repr: mir.ReprObjectRef, Op: mir.MapOp{Kind: mir.MapOpNew}},
						// val1 = 100
						{Result: 3, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 100}},
						// val2 = 200
						{Result: 4, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 200}},
						// m.set(arr1, 100)
						{Result: 5, Repr: mir.ReprObjectRef, Op: mir.MapOp{Kind: mir.MapOpSet, Map: 2, Key: 0, Value: 3}},
						// m.set(arr2, 200)
						{Result: 6, Repr: mir.ReprObjectRef, Op: mir.MapOp{Kind: mir.MapOpSet, Map: 2, Key: 1, Value: 4}},
						// m.size -> 2
						{Result: 7, Repr: mir.ReprF64, Op: mir.MapOp{Kind: mir.MapOpSize, Map: 2}},
						{Result: 8, Repr: mir.ReprVoid, Op: mir.IntrinsicCall{Intrinsic: mir.IntrinsicConsoleLogF64, Args: []mir.ValueID{7}}},
						// m.get(arr1) -> 100
						{Result: 9, Repr: mir.ReprF64, Op: mir.MapOp{Kind: mir.MapOpGet, Map: 2, Key: 0}},
						{Result: 10, Repr: mir.ReprVoid, Op: mir.IntrinsicCall{Intrinsic: mir.IntrinsicConsoleLogF64, Args: []mir.ValueID{9}}},
						// m.get(arr2) -> 200
						{Result: 11, Repr: mir.ReprF64, Op: mir.MapOp{Kind: mir.MapOpGet, Map: 2, Key: 1}},
						{Result: 12, Repr: mir.ReprVoid, Op: mir.IntrinsicCall{Intrinsic: mir.IntrinsicConsoleLogF64, Args: []mir.ValueID{11}}},
					},
					Terminator: mir.Return{},
				}},
			},
		},
	}

	out := runEmittedModule(t, module)
	got := strings.TrimSpace(out)
	want := "2\n100\n200"
	if got != want {
		t.Fatalf("got output:\n%s\nwant:\n%s", got, want)
	}
}
