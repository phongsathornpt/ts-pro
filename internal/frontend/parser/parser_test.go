package parser

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/token"
	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

func TestParseFunctionAndFib(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("fib.ts", []byte(`
function fib(n: number): number {
    if (n <= 1) {
        return n;
    }
    return fib(n - 1) + fib(n - 2);
}
`))

	p := New(file)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser diagnostics: %s", diags.Format(fs))
	}

	if len(prog.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Statements))
	}

	fn, ok := prog.Statements[0].(*ast.FunctionDecl)
	if !ok {
		t.Fatalf("expected *ast.FunctionDecl, got %T", prog.Statements[0])
	}
	if fn.Name != "fib" {
		t.Errorf("function name = %q, want 'fib'", fn.Name)
	}
	if len(fn.Params) != 1 || fn.Params[0].Name != "n" {
		t.Errorf("expected param 'n', got %+v", fn.Params)
	}
	if len(fn.Body.Statements) != 2 {
		t.Errorf("expected 2 statements in body, got %d", len(fn.Body.Statements))
	}

	ifStmt, ok := fn.Body.Statements[0].(*ast.IfStmt)
	if !ok {
		t.Fatalf("expected *ast.IfStmt, got %T", fn.Body.Statements[0])
	}
	binCond, ok := ifStmt.Cond.(*ast.BinaryExpr)
	if !ok || binCond.Op != token.LtEq {
		t.Errorf("expected binary <= cond, got %+v", ifStmt.Cond)
	}
}

func TestParseClassesAndTypes(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("types.ts", []byte(`
interface Point {
    x: number;
    y: number;
}

class Vector {
    x: number;
    y: number;
    length(): number {
        return 0;
    }
}
`))

	p := New(file)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser diagnostics: %s", diags.Format(fs))
	}

	if len(prog.Statements) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(prog.Statements))
	}

	iface, ok := prog.Statements[0].(*ast.InterfaceDecl)
	if !ok || iface.Name != "Point" || len(iface.Fields) != 2 {
		t.Errorf("unexpected interface node: %+v", iface)
	}

	cls, ok := prog.Statements[1].(*ast.ClassDecl)
	if !ok || cls.Name != "Vector" || len(cls.Fields) != 2 || len(cls.Methods) != 1 {
		t.Errorf("unexpected class node: %+v", cls)
	}
}
