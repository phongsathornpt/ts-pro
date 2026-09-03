package ast

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/core/token"
	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

func TestASTNodes(t *testing.T) {
	span := source.Span{Start: 1, End: 10}
	ident := &IdentExpr{SourceSpan: span, Name: "fib"}
	if ident.Span() != span {
		t.Errorf("ident.Span() = %v, want %v", ident.Span(), span)
	}

	bin := &BinaryExpr{
		SourceSpan: span,
		Left:       ident,
		Op:         token.Plus,
		Right:      &NumberLit{SourceSpan: span, Value: 1},
	}
	if bin.Op != token.Plus {
		t.Errorf("bin.Op = %v, want token.Plus", bin.Op)
	}

	fn := &FunctionDecl{
		SourceSpan: span,
		Name:       "fib",
		Params: []Param{
			{Name: "n", Type: &PrimitiveTypeNode{SourceSpan: span, Kind: "number"}},
		},
		ReturnType: &PrimitiveTypeNode{SourceSpan: span, Kind: "number"},
		Body: &BlockStmt{
			SourceSpan: span,
			Statements: []Stmt{
				&ReturnStmt{SourceSpan: span, Value: bin},
			},
		},
	}
	if fn.Name != "fib" {
		t.Errorf("fn.Name = %q, want 'fib'", fn.Name)
	}
}
