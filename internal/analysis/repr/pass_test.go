package repr

import (
	"testing"

	"github.com/projectthorn/tsv7-bin/internal/hir"
)

func TestAnalyzeProvesScalarRepresentations(t *testing.T) {
	v0 := hir.NewValueID(0)
	module := hir.Module{
		Types: []hir.SemanticType{{Kind: hir.TypeNumber}, {Kind: hir.TypeBoolean}},
		Functions: []hir.Function{{
			ID: hir.NewFunctionID(0), Name: "f", ReturnType: hir.NewTypeID(0), Entry: hir.NewBlockID(0),
			Params: []hir.Param{{Value: v0, Name: "x", SemanticType: hir.NewTypeID(0)}},
			Blocks: []hir.Block{{ID: hir.NewBlockID(0), Instructions: []hir.Instruction{
				{Result: hir.NewValueID(1), SemanticType: hir.NewTypeID(0), Op: hir.ConstOp{Literal: hir.Literal{Kind: hir.LiteralNumber, Number: 1}}},
				{Result: hir.NewValueID(2), SemanticType: hir.NewTypeID(1), Op: hir.BinaryExpr{Operator: hir.BinaryLessEqual, Left: v0, Right: hir.NewValueID(1)}},
			}, Terminator: hir.ReturnTerm{Value: &v0}}},
		}},
	}
	diagnostics := Analyze(&module)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
	fn := module.Functions[0]
	if fn.ReturnRepr.Kind != hir.ReprF64 || fn.Params[0].Repr.Kind != hir.ReprF64 {
		t.Fatalf("function repr = %+v params=%+v", fn.ReturnRepr, fn.Params)
	}
	if fn.Blocks[0].Instructions[1].Repr.Kind != hir.ReprBool {
		t.Fatalf("condition repr = %+v", fn.Blocks[0].Instructions[1].Repr)
	}
}

func TestAnalyzeRejectsUnsupportedTaggedUnionMember(t *testing.T) {
	union := hir.NewTypeID(0)
	task := hir.NewTypeID(1)
	module := hir.Module{
		Types: []hir.SemanticType{
			{Kind: hir.TypeUnion, Members: []hir.TypeID{hir.NewTypeID(2), task}},
			{Kind: hir.TypeTask},
			{Kind: hir.TypeNumber},
		},
		Functions: []hir.Function{{
			ID: hir.NewFunctionID(0), Name: "f", ReturnType: union, Entry: hir.NewBlockID(0),
			Params: []hir.Param{{Value: hir.NewValueID(0), Name: "x", SemanticType: union}},
			Blocks: []hir.Block{{ID: hir.NewBlockID(0), Terminator: hir.ReturnTerm{Value: func() *hir.ValueID { v := hir.NewValueID(0); return &v }()}}},
		}},
	}
	diagnostics := Analyze(&module)
	if len(diagnostics) == 0 {
		t.Fatal("expected unsupported union member diagnostic")
	}
}
