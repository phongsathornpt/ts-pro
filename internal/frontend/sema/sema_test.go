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

func TestSemaNativeClassInstanceConstructorAndMethod(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("class.ts", []byte(`
class NativePoint {
  constructor(public x: number, public y: number) {}
  sum(): number { return this.x + this.y; }
}
const p = new NativePoint(3, 4);
const n: number = p.sum();
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
	info := result.Classes["NativePoint"]
	if info == nil || info.Constructor == nil || len(info.Constructor.Params) != 2 {
		t.Fatalf("unexpected class info: %#v", info)
	}
	if x := info.Instance.Fields["x"]; !x.Type.Equals(types.TypeNumber) {
		t.Fatalf("x field = %#v", x)
	}
	if y := info.Instance.Fields["y"]; !y.Type.Equals(types.TypeNumber) {
		t.Fatalf("y field = %#v", y)
	}
	if sum := info.Methods["sum"]; sum == nil || !sum.Return.Equals(types.TypeNumber) {
		t.Fatalf("sum method = %#v", sum)
	}
	pv := result.RootScope.Resolve("p")
	if pv == nil || !pv.Type.Equals(info.Instance) {
		t.Fatalf("p type = %v, want NativePoint", pv)
	}
}

func TestSemaNativeClassInitializersAndMutation(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("class-effects.ts", []byte(`
class Box {
  value: number = 2;
  constructor(delta: number) { this.value = this.value + delta; }
  get(): number { return this.value; }
}
const box = new Box(3);
const value: number = box.get();
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

func TestSemaRejectsNativeConstructorArgumentMismatch(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("class-bad.ts", []byte(`
class Box { constructor(public value: number) {} }
const bad = new Box("wrong");
`))
	p := parser.New(file)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser diagnostics: %s", diags.Format(fs))
	}
	result := Check(prog)
	if !result.Diagnostics.HasErrors() {
		t.Fatal("expected constructor argument mismatch")
	}
}

func TestSemaClassInheritancePreservesBaseShapeAndMethodOwners(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("inheritance.ts", []byte(`
class Base {
  constructor(public x: number) {}
  value(): number { return this.x; }
}
class Derived extends Base {
  y: number = 2;
  constructor(x: number) { super(x); }
  sum(): number { return this.x + this.y; }
}
const derived: Base = new Derived(40);
const sum: number = new Derived(40).sum();
const value: number = new Derived(40).value();
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
	base := result.Classes["Base"]
	derived := result.Classes["Derived"]
	if base == nil || derived == nil {
		t.Fatalf("missing class metadata: base=%#v derived=%#v", base, derived)
	}
	if len(derived.Instance.FieldOrder) < 2 || derived.Instance.FieldOrder[0] != "x" || derived.Instance.FieldOrder[1] != "y" {
		t.Fatalf("derived field order = %v, want base prefix [x y]", derived.Instance.FieldOrder)
	}
	if owner := derived.MethodOwners["value"]; owner != "Base" {
		t.Fatalf("inherited value owner = %q, want Base", owner)
	}
	if owner := derived.MethodOwners["sum"]; owner != "Derived" {
		t.Fatalf("sum owner = %q, want Derived", owner)
	}
	if !derived.Instance.AssignableTo(base.Instance) {
		t.Fatal("derived instance must be structurally assignable to base instance")
	}
}

func TestSemaClassOverrideReplacesMethodOwner(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("override.ts", []byte(`
class Base { constructor(public value: number) {} score(): number { return this.value; } }
class Derived extends Base { constructor(value: number) { super(value); } override score(): number { return this.value + 2; } }
const item: Base = new Derived(40);
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
	if got := result.Classes["Derived"].MethodOwners["score"]; got != "Derived" {
		t.Fatalf("override owner = %q, want Derived", got)
	}
}

func TestSemaSpecializesGenericClasses(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("generic-class.ts", []byte(`
class Box<T> {
  value: T;
  constructor(v: T) { this.value = v; }
  get(): T { return this.value; }
}
const n = new Box<number>(42);
const s = new Box<string>("hello");
const nv: number = n.get();
const sv: string = s.get();
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
	var specs []*ClassInfo
	for _, info := range result.GenericClasses {
		specs = append(specs, info)
	}
	if len(specs) != 2 {
		t.Fatalf("generic class specializations = %d, want 2", len(specs))
	}
	seenNumber, seenString := false, false
	for _, spec := range specs {
		field := spec.Instance.Fields["value"].Type
		if field == types.TypeNumber {
			seenNumber = true
		}
		if field == types.TypeString {
			seenString = true
		}
		if len(spec.TypeBindings) != 1 {
			t.Fatalf("%s bindings = %d, want 1", spec.Name, len(spec.TypeBindings))
		}
	}
	if !seenNumber || !seenString {
		t.Fatalf("missing number/string specializations: %#v", specs)
	}
}
