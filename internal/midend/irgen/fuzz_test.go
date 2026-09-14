package irgen

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/frontend/parser"
	"github.com/phongsathornpt/ts-pro/internal/frontend/sema"
	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

func FuzzIRGen(f *testing.F) {
	seeds := []string{
		"let x: number = 1 + 2;",
		"function add(a: number, b: number): number { return a + b; } console.log(add(1, 2));",
		"const values: number[] = [1, 2, 3]; let total = 0; for (const value of values) { total = total + value; }",
		"class Box { constructor(public value: number) {} get(): number { return this.value; } } const b = new Box(1); console.log(b.get());",
		"async function main(): Promise<void> { const value = await Promise.resolve(1); console.log(value); } main();",
		"const obj: any = { a: 1 }; obj.a = obj.a + 1; console.log(obj.a);",
		"const key = new Uint8Array(3); crypto.subtle.importKey(\"raw\", key, { name: \"HMAC\", hash: \"SHA-256\", length: 17 }, false, [\"sign\"]);",
		"console.log();",
		"console.log(() => 1);",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, src string) {
		if len(src) > 64<<10 {
			t.Skip()
		}
		fs := source.NewFileSet()
		file := fs.AddFile("fuzz.ts", []byte(src))
		prog, parseDiags := parser.New(file).Parse()
		if parseDiags.HasErrors() || prog == nil {
			return
		}
		semaResult := sema.Check(prog)
		if semaResult == nil || semaResult.Diagnostics.HasErrors() {
			return
		}
		irProg, err := Generate(prog, semaResult)
		if err != nil {
			return
		}
		if irProg == nil {
			t.Fatal("IR generator succeeded with nil program")
		}
		_ = irProg.Dump()
	})
}
