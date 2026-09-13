package sema

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/frontend/parser"
	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

func FuzzSema(f *testing.F) {
	seeds := []string{
		"let x: number = 1;",
		"let x: number = \"wrong\";",
		"function id<T>(x: T): T { return x; } const n: number = id(1);",
		"class A { constructor(public x: number) {} } class B extends A { constructor(x: number) { super(x); } }",
		"const values: number[] = [1, 2, 3]; for (const value of values) { console.log(value); }",
		"async function main(): Promise<void> { const v = await Promise.resolve(1); console.log(v); }",
		"const obj: { a: number; b?: string } = { a: 1 };",
		"crypto.subtle.importKey(\"raw\", new Uint8Array(3), { name: \"HMAC\", hash: \"SHA-256\", length: 17 }, false, [\"sign\"]);",
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
		result := Check(prog)
		if result == nil {
			t.Fatal("semantic checker returned nil result")
		}
		_ = result.Diagnostics.Format(fs)
	})
}
