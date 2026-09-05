package tspro

import (
	"fmt"
	"strings"
	"testing"
)

func benchmarkSource(statements int) []byte {
	var b strings.Builder
	b.WriteString("function add(a: number, b: number): number { return a + b; }\n")
	b.WriteString("let total: number = 0;\n")
	for i := 0; i < statements; i++ {
		fmt.Fprintf(&b, "total = add(total, %d);\n", i)
	}
	b.WriteString("console.log(total);\n")
	return []byte(b.String())
}

func benchmarkCompileSource(b *testing.B, statements int) {
	src := benchmarkSource(statements)
	opts := DefaultOptions()
	opts.TargetOS = "linux"
	opts.TargetArch = "amd64"
	opts.OptLevel = 2

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		compiler := New(opts)
		if _, diags, err := compiler.CompileSource("bench.ts", src); err != nil {
			b.Fatalf("compile failed: %v (%v)", err, diags)
		}
	}
}

func BenchmarkCompileSourceSmall(b *testing.B)  { benchmarkCompileSource(b, 32) }
func BenchmarkCompileSourceMedium(b *testing.B) { benchmarkCompileSource(b, 512) }
