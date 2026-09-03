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

func TestParseParenthesizedUnionArrayType(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("union-array.ts", []byte(`const values: (number | string)[] = [1, "two"];`))
	p := New(file)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser diagnostics: %s", diags.Format(fs))
	}
	decl, ok := prog.Statements[0].(*ast.VarDeclStmt)
	if !ok || len(decl.Declarations) != 1 {
		t.Fatalf("unexpected declaration: %T", prog.Statements[0])
	}
	arr, ok := decl.Declarations[0].Type.(*ast.ArrayTypeNode)
	if !ok {
		t.Fatalf("expected array type, got %T", decl.Declarations[0].Type)
	}
	if _, ok := arr.ElemType.(*ast.UnionTypeNode); !ok {
		t.Fatalf("expected union element type, got %T", arr.ElemType)
	}
}

func TestParseObjectTypeAlias(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("object-type.ts", []byte(`type Point = { x: number; y?: string };`))
	p := New(file)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser diagnostics: %s", diags.Format(fs))
	}
	alias, ok := prog.Statements[0].(*ast.TypeAliasDecl)
	if !ok {
		t.Fatalf("expected type alias, got %T", prog.Statements[0])
	}
	obj, ok := alias.Type.(*ast.ObjectTypeNode)
	if !ok || len(obj.Fields) != 2 {
		t.Fatalf("unexpected object type: %#v", alias.Type)
	}
	if !obj.Fields[1].Optional {
		t.Fatal("expected optional field")
	}
}

func TestParserUnsupportedSyntaxAlwaysMakesProgress(t *testing.T) {
	cases := map[string]string{
		"generic_function":   `function identity<T>(x: T): T { return x; }`,
		"parameter_property": `class Box { constructor(public value: number) {} }`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			fs := source.NewFileSet()
			file := fs.AddFile(name+".ts", []byte(src))
			p := New(file)
			prog, diags := p.Parse()
			if prog == nil {
				t.Fatal("parser returned nil program")
			}
			if !diags.HasErrors() {
				t.Fatal("unsupported syntax must produce diagnostics until lowering support is added")
			}
			if p.current().Kind != token.EOF {
				t.Fatalf("parser stopped before EOF at %s", p.current().Kind)
			}
		})
	}
}
