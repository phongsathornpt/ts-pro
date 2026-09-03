package rangeanalysis

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/hir"
)

func TestAnalyzeProvesIntegerArithmeticAndPhi(t *testing.T) {
	t0 := hir.NewTypeID(0)
	v0, v1, v2 := hir.NewValueID(0), hir.NewValueID(1), hir.NewValueID(2)
	v3, v4 := hir.NewValueID(3), hir.NewValueID(4)

	block0 := hir.Block{ID: 0, Instructions: []hir.Instruction{
		{Result: v0, SemanticType: t0, Op: numberConst(10)},
		{Result: v1, SemanticType: t0, Op: numberConst(20)},
		{Result: v2, SemanticType: t0, Op: hir.BinaryExpr{Operator: hir.BinaryAdd, Left: v0, Right: v1}},
	}}
	block1 := hir.Block{ID: 1, Instructions: []hir.Instruction{
		{Result: v3, SemanticType: t0, Op: numberConst(-5)},
		{Result: v4, SemanticType: t0, Op: hir.PhiOp{Incoming: []hir.PhiIncoming{{Block: 0, Value: v2}, {Block: 1, Value: v3}}}},
	}}
	module := hir.Module{
		Types:     []hir.SemanticType{{Kind: hir.TypeNumber}},
		Functions: []hir.Function{{ID: 0, Name: "ranges", ReturnType: t0, Entry: 0, Blocks: []hir.Block{block0, block1}}},
	}
	result := Analyze(module)[0]
	if got := result[v2]; !got.Known || got.Min != 30 || got.Max != 30 || !got.FitsI32() {
		t.Fatalf("add interval = %+v", got)
	}
	if got := result[v4]; !got.Known || got.Min != -5 || got.Max != 30 || !got.FitsI32() {
		t.Fatalf("phi interval = %+v", got)
	}
}

func TestAnalyzeRejectsFractionalDivisionAndUnsafeOverflow(t *testing.T) {
	left := Interval{Min: 5, Max: 5, Known: true}
	right := Interval{Min: 2, Max: 2, Known: true}
	if got, ok := inferBinary(hir.BinaryDiv, left, right); ok || got.Known {
		t.Fatalf("fractional division unexpectedly proven: %+v", got)
	}
	unsafe := Interval{Min: MaxSafeInteger, Max: MaxSafeInteger, Known: true}
	one := Interval{Min: 1, Max: 1, Known: true}
	if got, ok := inferBinary(hir.BinaryAdd, unsafe, one); ok || got.Known {
		t.Fatalf("unsafe integer overflow unexpectedly proven: %+v", got)
	}
}

func TestIntervalWidthClassification(t *testing.T) {
	if !((Interval{Min: -10, Max: 10, Known: true}).FitsI32()) {
		t.Fatal("small integer range should fit i32")
	}
	wide := Interval{Min: int64(1) << 40, Max: int64(1) << 40, Known: true}
	if wide.FitsI32() || !wide.FitsI64() {
		t.Fatalf("wide safe integer classification = %+v", wide)
	}
}

func TestAnalyzeCrossCallPropagation(t *testing.T) {
	t0 := hir.NewTypeID(0)
	f0, f1 := hir.NewFunctionID(0), hir.NewFunctionID(1)

	// Function 1: add5(x: number) -> x + 5
	// param v10, const v11 (5), add v12 (v10 + v11), return v12
	v10, v11, v12 := hir.NewValueID(10), hir.NewValueID(11), hir.NewValueID(12)
	blockCallee := hir.Block{ID: 0, Instructions: []hir.Instruction{
		{Result: v11, SemanticType: t0, Op: numberConst(5)},
		{Result: v12, SemanticType: t0, Op: hir.BinaryExpr{Operator: hir.BinaryAdd, Left: v10, Right: v11}},
	}, Terminator: hir.ReturnTerm{Value: &v12}}
	calleeFn := hir.Function{
		ID: f1, Name: "add5", ReturnType: t0, Entry: 0,
		Params: []hir.Param{{Value: v10, Name: "x", SemanticType: t0}},
		Blocks: []hir.Block{blockCallee},
	}

	// Function 0: caller() -> add5(10) + 1
	// const v0 (10), call v1 = add5(v0), const v2 (1), add v3 = v1 + v2
	v0, v1, v2, v3 := hir.NewValueID(0), hir.NewValueID(1), hir.NewValueID(2), hir.NewValueID(3)
	blockCaller := hir.Block{ID: 0, Instructions: []hir.Instruction{
		{Result: v0, SemanticType: t0, Op: numberConst(10)},
		{Result: v1, SemanticType: t0, Op: hir.CallOp{Callee: f1, Args: []hir.ValueID{v0}}},
		{Result: v2, SemanticType: t0, Op: numberConst(1)},
		{Result: v3, SemanticType: t0, Op: hir.BinaryExpr{Operator: hir.BinaryAdd, Left: v1, Right: v2}},
	}, Terminator: hir.ReturnTerm{Value: &v3}}
	callerFn := hir.Function{
		ID: f0, Name: "caller", ReturnType: t0, Entry: 0,
		Blocks: []hir.Block{blockCaller},
	}

	module := hir.Module{
		Types:     []hir.SemanticType{{Kind: hir.TypeNumber}},
		Functions: []hir.Function{callerFn, calleeFn},
	}

	res := Analyze(module)

	// Callee param v10 should be [10, 10]
	calleeRes := res[f1]
	if got := calleeRes[v10]; !got.Known || got.Min != 10 || got.Max != 10 {
		t.Fatalf("callee param range = %+v, want [10, 10]", got)
	}
	if got := calleeRes[v12]; !got.Known || got.Min != 15 || got.Max != 15 {
		t.Fatalf("callee return value range = %+v, want [15, 15]", got)
	}

	// Caller call result v1 should be [15, 15], v3 should be [16, 16]
	callerRes := res[f0]
	if got := callerRes[v1]; !got.Known || got.Min != 15 || got.Max != 15 {
		t.Fatalf("call site range = %+v, want [15, 15]", got)
	}
	if got := callerRes[v3]; !got.Known || got.Min != 16 || got.Max != 16 || !got.FitsI32() {
		t.Fatalf("caller final add range = %+v, want [16, 16] (FitsI32)", got)
	}
}

func TestAnalyzeLoopCarriedState(t *testing.T) {
	t0 := hir.NewTypeID(0)
	// Loop: i = 0; while (i < 10) { i = i + 1; }
	// Block 0 (preheader): v0 = 0, Jump to 1
	// Block 1 (header): v1 = Phi [0: v0, 2: v4], v2 = 10, v3 = (v1 < v2), Branch v3 ? 2 : 3
	// Block 2 (body): v5 = 1, v4 = v1 + v5, Jump to 1
	// Block 3 (exit): return v1
	v0, v1, v2, v3 := hir.NewValueID(0), hir.NewValueID(1), hir.NewValueID(2), hir.NewValueID(3)
	v4, v5 := hir.NewValueID(4), hir.NewValueID(5)

	b0 := hir.Block{ID: 0, Instructions: []hir.Instruction{
		{Result: v0, SemanticType: t0, Op: numberConst(0)},
	}, Terminator: hir.JumpTerm{Target: 1}}

	b1 := hir.Block{ID: 1, Instructions: []hir.Instruction{
		{Result: v1, SemanticType: t0, Op: hir.PhiOp{Incoming: []hir.PhiIncoming{{Block: 0, Value: v0}, {Block: 2, Value: v4}}}},
		{Result: v2, SemanticType: t0, Op: numberConst(10)},
		{Result: v3, SemanticType: t0, Op: hir.BinaryExpr{Operator: hir.BinaryLessThan, Left: v1, Right: v2}},
	}, Terminator: hir.BranchTerm{Condition: v3, Then: 2, Else: 3}}

	b2 := hir.Block{ID: 2, Instructions: []hir.Instruction{
		{Result: v5, SemanticType: t0, Op: numberConst(1)},
		{Result: v4, SemanticType: t0, Op: hir.BinaryExpr{Operator: hir.BinaryAdd, Left: v1, Right: v5}},
	}, Terminator: hir.JumpTerm{Target: 1}}

	b3 := hir.Block{ID: 3, Terminator: hir.ReturnTerm{Value: &v1}}

	fn := hir.Function{
		ID: 0, Name: "loop", ReturnType: t0, Entry: 0,
		Blocks: []hir.Block{b0, b1, b2, b3},
	}
	module := hir.Module{
		Types:     []hir.SemanticType{{Kind: hir.TypeNumber}},
		Functions: []hir.Function{fn},
	}

	res := Analyze(module)[0]
	// Loop phi v1 should be proven with Min: 0, Max: 9
	if got := res[v1]; !got.Known || got.Min != 0 || got.Max != 9 || !got.FitsI32() {
		t.Fatalf("loop phi interval = %+v, want [0, 9] (FitsI32)", got)
	}
	// Loop-carried increment v4 should be proven with Min: 1, Max: 10
	if got := res[v4]; !got.Known || got.Min != 1 || got.Max != 10 || !got.FitsI32() {
		t.Fatalf("loop increment interval = %+v, want [1, 10] (FitsI32)", got)
	}
}

func numberConst(value float64) hir.ConstOp {
	return hir.ConstOp{Literal: hir.Literal{Kind: hir.LiteralNumber, Number: value}}
}

