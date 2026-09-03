package sema

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/core/types"
	"github.com/phongsathornpt/ts-pro/internal/frontend/parser"
	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

func TestSemaFib(t *testing.T) {
	fs := source.NewFileSet()
	f := fs.AddFile("fib.ts", []byte(`
function fib(n: number): number {
    if (n <= 1) {
        return n;
    }
    return fib(n - 1) + fib(n - 2);
}
`))

	p := parser.New(f)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser errors: %v", diags)
	}

	result := Check(prog)
	if result.Diagnostics.HasErrors() {
		t.Fatalf("semantic errors: %s", result.Diagnostics.Format(fs))
	}
}

func TestSemaTypeMismatch(t *testing.T) {
	fs := source.NewFileSet()
	f := fs.AddFile("err.ts", []byte(`
let x: number = "not a number";
`))

	p := parser.New(f)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser errors: %v", diags)
	}

	result := Check(prog)
	if !result.Diagnostics.HasErrors() {
		t.Errorf("expected type mismatch error, got none")
	}
}

func TestSemaUndefinedIdentifier(t *testing.T) {
	fs := source.NewFileSet()
	f := fs.AddFile("err2.ts", []byte(`
function test(): number {
    return missingVar + 1;
}
`))

	p := parser.New(f)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser errors: %v", diags)
	}

	result := Check(prog)
	if !result.Diagnostics.HasErrors() {
		t.Errorf("expected undefined identifier error, got none")
	}
}

func TestObjectTypeAliasResolvesInArray(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("alias.ts", []byte(`
type Point = { x: number; name: string };
const points: Point[] = [{ x: 1, name: "one" }];
console.log(points[0].name);
`))
	p := parser.New(file)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser diagnostics: %s", diags.Format(fs))
	}
	result := Check(prog)
	if result.Diagnostics.HasErrors() {
		t.Fatalf("sema diagnostics: %s", result.Diagnostics.Format(fs))
	}
}

func TestSemaTypedArrowFunction(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("arrow.ts", []byte(`
const offset = 5;
const add: (x: number) => number = (x: number): number => x + offset;
const result: number = add(7);
`))
	p := parser.New(file)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser diagnostics: %s", diags.Format(fs))
	}
	result := Check(prog)
	if result.Diagnostics.HasErrors() {
		t.Fatalf("sema diagnostics: %s", result.Diagnostics.Format(fs))
	}
}

func TestSemaArrowReturnMismatch(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("arrow-bad.ts", []byte(`
const bad = (x: number): string => x + 1;
`))
	p := parser.New(file)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser diagnostics: %s", diags.Format(fs))
	}
	result := Check(prog)
	if !result.Diagnostics.HasErrors() {
		t.Fatal("expected arrow return type mismatch")
	}
}

func TestSemaExplicitGenericFunctionSpecialization(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("generics.ts", []byte(`
function identity<T>(x: T): T { return x; }
function pair<A, B>(first: A, second: B): [A, B] { return [first, second]; }
const n: number = identity<number>(42);
const s: string = identity<string>("hello");
const p: [string, number] = pair<string, number>("answer", 42);
`))
	p := parser.New(file)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser diagnostics: %s", diags.Format(fs))
	}
	result := Check(prog)
	if result.Diagnostics.HasErrors() {
		t.Fatalf("sema diagnostics: %s", result.Diagnostics.Format(fs))
	}
	identity := result.RootScope.Resolve("identity")
	fn, ok := identity.Type.(*types.FunctionType)
	if !ok || len(fn.TypeParams) != 1 {
		t.Fatalf("identity type = %T %v", identity.Type, identity.Type)
	}
	if !fn.Params[0].Type.Equals(fn.TypeParams[0]) || !fn.Return.Equals(fn.TypeParams[0]) {
		t.Fatalf("identity did not preserve declaration type variable: %s", fn)
	}
	pairVar := result.RootScope.Resolve("p")
	if pairVar == nil || !pairVar.Type.Equals(types.NewTuple(types.TypeString, types.TypeNumber)) {
		t.Fatalf("p type = %v, want [string, number]", pairVar)
	}
}

func TestSemaGenericTypeArgumentMismatch(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("generic-bad.ts", []byte(`
function identity<T>(x: T): T { return x; }
const bad: number = identity<string>("wrong");
`))
	p := parser.New(file)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser diagnostics: %s", diags.Format(fs))
	}
	result := Check(prog)
	if !result.Diagnostics.HasErrors() {
		t.Fatal("expected generic specialization assignment mismatch")
	}
}

func TestSemaInfersGenericFunctionCallsAndTupleIndexes(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("generic-infer.ts", []byte(`
function identity<T>(x: T): T { return x; }
function pair<A, B>(first: A, second: B): [A, B] { return [first, second]; }
const n: number = identity(42);
const s: string = identity("hello");
const p: [string, number] = pair("answer", 42);
const first: string = p[0];
const second: number = p[1];
`))
	p := parser.New(file)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser diagnostics: %s", diags.Format(fs))
	}
	result := Check(prog)
	if result.Diagnostics.HasErrors() {
		t.Fatalf("sema diagnostics: %s", result.Diagnostics.Format(fs))
	}
	if len(result.GenericCalls) != 3 {
		t.Fatalf("generic call instantiations = %d, want 3", len(result.GenericCalls))
	}
	if first := result.RootScope.Resolve("first"); first == nil || !first.Type.Equals(types.TypeString) {
		t.Fatalf("first type = %v", first)
	}
	if second := result.RootScope.Resolve("second"); second == nil || !second.Type.Equals(types.TypeNumber) {
		t.Fatalf("second type = %v", second)
	}
}

func TestSemaArrayForOfElementType(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("forof.ts", []byte(`
const values: number[] = [1, 2, 3];
let total = 0;
for (const value of values) {
  total = total + value;
}
`))
	p := parser.New(file)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser diagnostics: %s", diags.Format(fs))
	}
	result := Check(prog)
	if result.Diagnostics.HasErrors() {
		t.Fatalf("sema diagnostics: %s", result.Diagnostics.Format(fs))
	}
}
