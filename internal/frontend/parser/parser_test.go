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

func TestParseRestParameter(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("rest.ts", []byte(`function sum(prefix: string, ...values: number[]): number { return 0; }`))
	p := New(file)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser diagnostics: %s", diags.Format(fs))
	}
	fn, ok := prog.Statements[0].(*ast.FunctionDecl)
	if !ok || len(fn.Params) != 2 {
		t.Fatalf("unexpected function: %#v", prog.Statements[0])
	}
	if fn.Params[0].Rest || !fn.Params[1].Rest {
		t.Fatalf("rest flags = [%v %v], want [false true]", fn.Params[0].Rest, fn.Params[1].Rest)
	}
	if p.current().Kind != token.EOF {
		t.Fatalf("parser stopped before EOF at %s", p.current().Kind)
	}
}

func TestParseGenericSyntaxAndTupleTypes(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("generic-syntax.ts", []byte(`
function pair<A, B>(first: A, second: B): [A, B] { return [first, second]; }
class Box<T> { value: T; }
interface Result<T> { value: T; }
type Maybe<T> = T | undefined;
const p = pair<string, number>("answer", 42);
const cmp = 1 < 2;
`))
	p := New(file)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser diagnostics: %s", diags.Format(fs))
	}
	fn, ok := prog.Statements[0].(*ast.FunctionDecl)
	if !ok || len(fn.TypeParams) != 2 || fn.TypeParams[0] != "A" || fn.TypeParams[1] != "B" {
		t.Fatalf("unexpected generic function: %#v", prog.Statements[0])
	}
	if tuple, ok := fn.ReturnType.(*ast.TupleTypeNode); !ok || len(tuple.Elements) != 2 {
		t.Fatalf("expected two-element tuple return type, got %T %#v", fn.ReturnType, fn.ReturnType)
	}
	cls := prog.Statements[1].(*ast.ClassDecl)
	if len(cls.TypeParams) != 1 || cls.TypeParams[0] != "T" {
		t.Fatalf("unexpected class type params: %#v", cls.TypeParams)
	}
	iface := prog.Statements[2].(*ast.InterfaceDecl)
	if len(iface.TypeParams) != 1 || iface.TypeParams[0] != "T" {
		t.Fatalf("unexpected interface type params: %#v", iface.TypeParams)
	}
	alias := prog.Statements[3].(*ast.TypeAliasDecl)
	if len(alias.TypeParams) != 1 || alias.TypeParams[0] != "T" {
		t.Fatalf("unexpected alias type params: %#v", alias.TypeParams)
	}
	callDecl := prog.Statements[4].(*ast.VarDeclStmt)
	call, ok := callDecl.Declarations[0].Init.(*ast.CallExpr)
	if !ok || len(call.TypeArgs) != 2 {
		t.Fatalf("expected generic call with two type args, got %T %#v", callDecl.Declarations[0].Init, callDecl.Declarations[0].Init)
	}
	cmpDecl := prog.Statements[5].(*ast.VarDeclStmt)
	cmp, ok := cmpDecl.Declarations[0].Init.(*ast.BinaryExpr)
	if !ok || cmp.Op != token.Lt {
		t.Fatalf("comparison must remain binary <, got %T %#v", cmpDecl.Declarations[0].Init, cmpDecl.Declarations[0].Init)
	}
}

func TestParseTypedArrowExpressionAndFunctionType(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("arrow.ts", []byte(`
const offset = 5;
const add = (x: number): number => x + offset;
const funcs: Array<(x: number) => number> = [add];
`))
	p := New(file)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser diagnostics: %s", diags.Format(fs))
	}
	arrowDecl := prog.Statements[1].(*ast.VarDeclStmt)
	arrow, ok := arrowDecl.Declarations[0].Init.(*ast.ArrowFuncExpr)
	if !ok || len(arrow.Params) != 1 || arrow.ReturnType == nil || !arrow.IsExprBody {
		t.Fatalf("unexpected arrow expression: %T %#v", arrowDecl.Declarations[0].Init, arrowDecl.Declarations[0].Init)
	}
	funcsDecl := prog.Statements[2].(*ast.VarDeclStmt)
	arr, ok := funcsDecl.Declarations[0].Type.(*ast.TypeRefNode)
	if !ok || arr.Name != "Array" || len(arr.TypeArgs) != 1 {
		t.Fatalf("unexpected Array function type: %T %#v", funcsDecl.Declarations[0].Type, funcsDecl.Declarations[0].Type)
	}
	if _, ok := arr.TypeArgs[0].(*ast.FunctionTypeNode); !ok {
		t.Fatalf("expected function type argument, got %T", arr.TypeArgs[0])
	}
}

func TestParseSwitchCasesAndBreak(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("switch.ts", []byte(`
switch (value) {
  case 1:
    console.log("one");
    break;
  default:
    console.log("other");
}
`))
	p := New(file)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser diagnostics: %s", diags.Format(fs))
	}
	sw, ok := prog.Statements[0].(*ast.SwitchStmt)
	if !ok || len(sw.Cases) != 2 {
		t.Fatalf("unexpected switch AST: %T %#v", prog.Statements[0], prog.Statements[0])
	}
	if _, ok := sw.Cases[0].Statements[len(sw.Cases[0].Statements)-1].(*ast.BreakStmt); !ok {
		t.Fatalf("expected trailing break in first case")
	}
	if sw.Cases[1].Test != nil {
		t.Fatal("default clause must have nil test")
	}
}

func TestParseArrayForOfLoop(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("forof.ts", []byte(`
for (const value: number of values) {
  console.log(value);
}
`))
	p := New(file)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser diagnostics: %s", diags.Format(fs))
	}
	loop, ok := prog.Statements[0].(*ast.ForOfStmt)
	if !ok || loop.Name != "value" || loop.Type == nil {
		t.Fatalf("unexpected for-of AST: %T %#v", prog.Statements[0], prog.Statements[0])
	}
}

func TestParseClassThisNewParameterPropertiesAndOverride(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("classes-native.ts", []byte(`
class Base<T> {
  constructor(public value: T) {}
  score(): T { return this.value; }
}
class Derived extends Base {
  constructor(public x: number, protected readonly label: string) {
    super(x);
    this.x = this.x + 1;
  }
  override score(): number { return this.x; }
}
const item = new Derived(40, "ok");
`))
	p := New(file)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser diagnostics: %s", diags.Format(fs))
	}
	if len(prog.Statements) != 3 {
		t.Fatalf("statements = %d, want 3", len(prog.Statements))
	}
	base := prog.Statements[0].(*ast.ClassDecl)
	if len(base.TypeParams) != 1 || len(base.Methods) != 2 {
		t.Fatalf("unexpected base class: %#v", base)
	}
	ctor := base.Methods[0]
	if ctor.Name != "constructor" || len(ctor.Params) != 1 || !ctor.Params[0].IsParameterProperty || ctor.Params[0].Visibility != "public" {
		t.Fatalf("unexpected constructor parameter property: %#v", ctor)
	}
	derived := prog.Statements[1].(*ast.ClassDecl)
	if derived.Extends != "Base" || len(derived.Methods) != 2 || !derived.Methods[1].IsOverride {
		t.Fatalf("unexpected derived class: %#v", derived)
	}
	param := derived.Methods[0].Params[1]
	if param.Visibility != "protected" || !param.Readonly || !param.IsParameterProperty {
		t.Fatalf("unexpected protected readonly parameter property: %#v", param)
	}
	decl := prog.Statements[2].(*ast.VarDeclStmt)
	if _, ok := decl.Declarations[0].Init.(*ast.NewExpr); !ok {
		t.Fatalf("expected new expression, got %T", decl.Declarations[0].Init)
	}
}

func TestParseNamedImportAliases(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("main.ts", []byte(`import { multiply, power as pow, Counter } from "./helper";`))
	p := New(file)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser diagnostics: %s", diags.Format(fs))
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("expected one import, got %d statements", len(prog.Statements))
	}
	imp, ok := prog.Statements[0].(*ast.ImportDecl)
	if !ok {
		t.Fatalf("expected *ast.ImportDecl, got %T", prog.Statements[0])
	}
	if imp.Module != "./helper" || len(imp.Specifiers) != 3 {
		t.Fatalf("unexpected import: %#v", imp)
	}
	if imp.Specifiers[1].Imported != "power" || imp.Specifiers[1].Local != "pow" {
		t.Fatalf("alias specifier = %#v", imp.Specifiers[1])
	}
}

func TestParseRegexLiteralWithoutBreakingDivision(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("regex.ts", []byte(`
const r = /abc\d+/i;
const n = 8 / 2;
`))
	p := New(file)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser diagnostics: %s", diags.Format(fs))
	}
	if len(prog.Statements) != 2 {
		t.Fatalf("statements = %d", len(prog.Statements))
	}
	first := prog.Statements[0].(*ast.VarDeclStmt).Declarations[0].Init
	r, ok := first.(*ast.RegexLit)
	if !ok {
		t.Fatalf("regex init = %T", first)
	}
	if r.Pattern != `abc\d+` || r.Flags != "i" {
		t.Fatalf("regex = /%s/%s", r.Pattern, r.Flags)
	}
	second := prog.Statements[1].(*ast.VarDeclStmt).Declarations[0].Init
	bin, ok := second.(*ast.BinaryExpr)
	if !ok || bin.Op != token.Slash {
		t.Fatalf("division init = %#v", second)
	}
}
