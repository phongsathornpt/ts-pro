package sema

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/frontend/parser"
	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

func checkSource(t *testing.T, src string) *Result {
	t.Helper()
	fs := source.NewFileSet()
	f := fs.AddFile("test.ts", []byte(src))
	p := parser.New(f)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser errors: %v", diags)
	}
	return Check(prog)
}

func expectDiagnostic(t *testing.T, src, wantCode string) {
	t.Helper()
	res := checkSource(t, src)
	if !res.Diagnostics.HasErrors() {
		t.Fatalf("expected diagnostic %s, got none", wantCode)
	}
	for _, d := range res.Diagnostics {
		if d.Code == wantCode {
			return
		}
	}
	t.Fatalf("expected diagnostic code %s, got: %v", wantCode, res.Diagnostics)
}

func TestSemaDiagnostics(t *testing.T) {
	// TS2335: 'this' outside class or function with this parameter
	expectDiagnostic(t, `const x = this;`, "TS2335")

	// TS2335: 'super' outside derived class
	expectDiagnostic(t, `class A { foo() { super.foo(); } }`, "TS2335")

	// TS2554: RegExp bad arity
	expectDiagnostic(t, `const r = new RegExp("a", "b", "c");`, "TS2554")

	// TS2345: RegExp non-string arg
	expectDiagnostic(t, `const r = new RegExp(123);`, "TS2345")

	// TS2554: Date bad arity
	expectDiagnostic(t, `const d = new Date(1, 2);`, "TS2554")

	// TS2345: Date non-number non-string arg
	expectDiagnostic(t, `const d = new Date(true);`, "TS2345")

	// TS2558: Map/Set type args arity
	expectDiagnostic(t, `const m = new Map<number>();`, "TS2558")
	expectDiagnostic(t, `const s = new Set<number, string>();`, "TS2558")

	// TS2304: Unknown class
	expectDiagnostic(t, `const x = new NonExistentClass();`, "TS2304")

	// TS2558: Generic class type args count mismatch
	expectDiagnostic(t, `class Box<T> { x: T; } const b = new Box();`, "TS2558")

	// TS2506: Class inheritance cycle
	expectDiagnostic(t, `class A extends B {} class B extends A {}`, "TS2506")

	// TS2300: Duplicate symbol declaration
	expectDiagnostic(t, `let x = 1; let x = 2;`, "TS2300")

	// TS2322: Assignability mismatch
	expectDiagnostic(t, `let x: number = "hello";`, "TS2322")

	// TS2349: Calling non-callable
	expectDiagnostic(t, `let x = 1; x();`, "TS2349")

	// TS2339: Property does not exist
	expectDiagnostic(t, `let o = { a: 1 }; o.b;`, "TS2339")
}
