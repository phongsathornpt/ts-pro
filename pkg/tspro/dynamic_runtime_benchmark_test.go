package tspro

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

func BenchmarkDynamicPropertyMixedReads(b *testing.B) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		b.Skip("native Linux AMD64 execution required")
	}
	var src strings.Builder
	src.WriteString("const obj: any = {};\n")
	for i := 0; i < 64; i++ {
		fmt.Fprintf(&src, "obj.k%d = %d;\n", i, i)
	}
	src.WriteString("let sum: number = 0;\nfor (let i = 0; i < 100000; i++) {\n")
	for _, i := range []int{0, 8, 16, 24, 32, 40, 48, 56} {
		fmt.Fprintf(&src, "sum = sum + obj.k%d;\n", i)
	}
	src.WriteString("}\nconsole.log(sum);\n")

	c := New(Options{TargetOS: "linux", TargetArch: "amd64", OptLevel: 2})
	bin, diags, err := c.CompileSource("dynamic-bench.ts", []byte(src.String()))
	if err != nil {
		b.Fatalf("compile: %v\n%s", err, diags.Format(c.FileSet()))
	}
	path := b.TempDir() + "/dynamic-bench"
	if err := os.WriteFile(path, bin, 0o755); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if out, err := exec.Command(path).CombinedOutput(); err != nil {
			b.Fatalf("execute: %v: %s", err, out)
		}
	}
	b.ReportMetric(800000, "dynamic-gets/op")
}
