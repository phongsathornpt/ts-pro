package sema

import (
	"testing"

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
