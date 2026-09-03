package irgen

import (
	"strings"
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/frontend/parser"
	"github.com/phongsathornpt/ts-pro/internal/frontend/sema"
	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

func TestIRGenFib(t *testing.T) {
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
		t.Fatalf("parser diags: %v", diags)
	}

	semaResult := sema.Check(prog)
	if semaResult.Diagnostics.HasErrors() {
		t.Fatalf("sema diags: %v", semaResult.Diagnostics)
	}

	irProg, err := Generate(prog, semaResult)
	if err != nil {
		t.Fatalf("irgen failed: %v", err)
	}

	dump := irProg.Dump()
	if !strings.Contains(dump, "define @fib(%n: number): number") {
		t.Errorf("expected @fib in dump, got:\n%s", dump)
	}
	if !strings.Contains(dump, "call @fib") {
		t.Errorf("expected call @fib in dump, got:\n%s", dump)
	}
}

func TestIRGenSSAIfPhi(t *testing.T) {
	fs := source.NewFileSet()
	f := fs.AddFile("test_phi.ts", []byte(`
function testPhi(cond: number): number {
    let x = 10;
    if (cond) {
        x = 20;
    } else {
        x = 30;
    }
    return x;
}
`))

	p := parser.New(f)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser diags: %v", diags)
	}

	semaResult := sema.Check(prog)
	irProg, err := Generate(prog, semaResult)
	if err != nil {
		t.Fatalf("irgen failed: %v", err)
	}

	dump := irProg.Dump()
	if !strings.Contains(dump, "phi") {
		t.Errorf("expected phi instruction in dump, got:\n%s", dump)
	}
}
