package parser

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

func FuzzParser(f *testing.F) {
	seeds := []string{
		"",
		"let x: number = 1;",
		"function id<T>(x: T): T { return x; }",
		"class Box { constructor(public value: number) {} get(): number { return this.value; } }",
		"async function main(): Promise<void> { await Promise.resolve(1); }",
		"const x = `hello ${1 + 2}`;",
		"const broken = { a: [1, 2, ;",
		"/* unterminated",
		"const ไทย = \"🙂\";",
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
		prog, _ := New(file).Parse()
		if prog == nil {
			t.Fatal("parser returned nil program")
		}
	})
}
