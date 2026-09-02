package lowering

import (
	"testing"

	rangeanalysis "github.com/phongsathornpt/ts-pro/internal/analysis/range"
	"github.com/phongsathornpt/ts-pro/internal/hir"
	"github.com/phongsathornpt/ts-pro/internal/mir"
)

func TestLowerMIRUsesProvenI32Arithmetic(t *testing.T) {
	t0 := hir.NewTypeID(0)
	v0, v1, v2 := hir.NewValueID(0), hir.NewValueID(1), hir.NewValueID(2)
	module := hir.Module{Types: []hir.SemanticType{{Kind: hir.TypeNumber}}, Functions: []hir.Function{{
		ID: 0, Name: "intfast", ReturnType: t0, ReturnRepr: hir.Repr{Kind: hir.ReprF64}, Entry: 0,
		Blocks: []hir.Block{{ID: 0, Instructions: []hir.Instruction{
			{Result: v0, SemanticType: t0, Repr: hir.Repr{Kind: hir.ReprF64}, Op: hir.ConstOp{Literal: hir.Literal{Kind: hir.LiteralNumber, Number: 20}}},
			{Result: v1, SemanticType: t0, Repr: hir.Repr{Kind: hir.ReprF64}, Op: hir.ConstOp{Literal: hir.Literal{Kind: hir.LiteralNumber, Number: 22}}},
			{Result: v2, SemanticType: t0, Repr: hir.Repr{Kind: hir.ReprF64}, Op: hir.BinaryExpr{Operator: hir.BinaryAdd, Left: v0, Right: v1}},
		}, Terminator: hir.ReturnTerm{Value: &v2}}},
	}}}

	ranges := rangeanalysis.Analyze(module)
	lowered, err := LowerMIRWithRanges(module, ranges)
	if err != nil {
		t.Fatal(err)
	}
	op, ok := lowered.Functions[0].Blocks[0].Instructions[2].Op.(mir.ProvenIntBinary)
	if !ok {
		t.Fatalf("operation = %T", lowered.Functions[0].Blocks[0].Instructions[2].Op)
	}
	if op.Width != mir.IntWidth32 || op.Operator != mir.FloatAdd {
		t.Fatalf("proven integer operation = %+v", op)
	}
}
