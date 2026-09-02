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

func numberConst(value float64) hir.ConstOp {
	return hir.ConstOp{Literal: hir.Literal{Kind: hir.LiteralNumber, Number: value}}
}
