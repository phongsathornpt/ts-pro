package irgen

import (
	"fmt"
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

func TestIRGenTopLevel(t *testing.T) {
	fs := source.NewFileSet()
	f := fs.AddFile("toplevel.ts", []byte(`
function fib(n: number): number {
    return n;
}
console.log(fib(20));
`))
	p := parser.New(f)
	prog, _ := p.Parse()
	semaResult := sema.Check(prog)
	irProg, err := Generate(prog, semaResult)
	if err != nil {
		t.Fatalf("irgen failed: %v", err)
	}
	t.Logf("Dump:\n%s", irProg.Dump())
}

func TestIRGenLoops(t *testing.T) {
	fs := source.NewFileSet()
	f := fs.AddFile("loops.ts", []byte(`
function sumWhile(n: number): number {
  let total: number = 0;
  let i: number = 0;
  while (i < n) {
    total = total + i;
    i++;
  }
  return total;
}
`))
	p := parser.New(f)
	prog, _ := p.Parse()
	semaResult := sema.Check(prog)
	irProg, err := Generate(prog, semaResult)
	if err != nil {
		t.Fatalf("irgen failed: %v", err)
	}
	t.Logf("Loops Dump:\n%s", irProg.Dump())
}

func TestIRGenStrings(t *testing.T) {
	fs := source.NewFileSet()
	f := fs.AddFile("strings.ts", []byte(`
export function greet(name: string): string {
  return "Hello, " + name + "!";
}

console.log(greet("TypeScript 7"));
`))
	p := parser.New(f)
	prog, _ := p.Parse()
	semaResult := sema.Check(prog)
	irProg, err := Generate(prog, semaResult)
	if err != nil {
		t.Fatalf("irgen failed: %v", err)
	}
	t.Logf("Strings Dump:\n%s", irProg.Dump())
}

func TestIRGenArrays(t *testing.T) {
	fs := source.NewFileSet()
	f := fs.AddFile("arrays.ts", []byte(`
function mutate(xs: number[]): number {
  xs[1] = 10;
  xs.push(20);
  return xs.length + xs[0] + xs.pop();
}
console.log(mutate([1, 2, 3]));
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
	for _, want := range []string{"alloc_array", "setelem", "getelem", "array_len", "array_push", "array_pop"} {
		if !strings.Contains(dump, want) {
			t.Fatalf("expected %q in IR:\n%s", want, dump)
		}
	}
}

func TestIRGenObjectFieldsUseCanonicalOffsets(t *testing.T) {
	fs := source.NewFileSet()
	f := fs.AddFile("objects.ts", []byte(`
interface Point { x: number; y: number; }
function readX(p: Point): number { return p.x; }
console.log(readX({ y: 4, x: 3 }));
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
	for _, want := range []string{"alloc_obj", "getfield %p.x@16", "setfield %obj.x@16", "setfield %obj.y@24"} {
		if !strings.Contains(dump, want) {
			t.Fatalf("expected %q in IR:\n%s", want, dump)
		}
	}
}

func TestIRGenRejectsObjectReferenceMaskOverflow(t *testing.T) {
	var src strings.Builder
	src.WriteString("const obj = {\n")
	for i := 0; i < 65; i++ {
		fmt.Fprintf(&src, "  f%02d: \"v%02d\",\n", i, i)
	}
	src.WriteString("};\nconsole.log(obj.f00);\n")

	fs := source.NewFileSet()
	f := fs.AddFile("object-mask-overflow.ts", []byte(src.String()))
	p := parser.New(f)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser diags: %s", diags.Format(fs))
	}
	semaResult := sema.Check(prog)
	if semaResult.Diagnostics.HasErrors() {
		t.Fatalf("sema diags: %s", semaResult.Diagnostics.Format(fs))
	}
	if _, err := Generate(prog, semaResult); err == nil || !strings.Contains(err.Error(), "64-bit GC reference mask") {
		t.Fatalf("expected GC reference-mask overflow error, got %v", err)
	}
}

func TestIRGenLiftsCapturedArrowFunction(t *testing.T) {
	fs := source.NewFileSet()
	f := fs.AddFile("closure.ts", []byte(`
function apply(base: number): number {
  const add = (x: number): number => base + x;
  return add(7);
}
`))
	p := parser.New(f)
	prog, diags := p.Parse()
	if diags.HasErrors() {
		t.Fatalf("parser diags: %s", diags.Format(fs))
	}
	semaResult := sema.Check(prog)
	if semaResult.Diagnostics.HasErrors() {
		t.Fatalf("sema diags: %s", semaResult.Diagnostics.Format(fs))
	}
	irProg, err := Generate(prog, semaResult)
	if err != nil {
		t.Fatalf("irgen failed: %v", err)
	}
	dump := irProg.Dump()
	for _, want := range []string{"make_closure @$arrow0", "closure_get %$env[0]", "call_indirect"} {
		if !strings.Contains(dump, want) {
			t.Fatalf("expected %q in closure IR:\n%s", want, dump)
		}
	}
}
