package tspro

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
)

func BenchmarkStringLoopAppendOwned(b *testing.B) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		b.Skip("native Linux AMD64 execution required")
	}
	src := []byte(`
let s = "";
for (let i = 0; i < 20000; i = i + 1) {
  s = s + "x";
}
console.log(s);
`)
	c := New(Options{TargetOS: "linux", TargetArch: "amd64", OptLevel: 2})
	bin, diags, err := c.CompileSource("string-loop-append-bench.ts", src)
	if err != nil {
		b.Fatalf("compile: %v\n%s", err, diags.Format(c.FileSet()))
	}
	path := b.TempDir() + "/string-loop-append-bench"
	if err := os.WriteFile(path, bin, 0o755); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := exec.Command(path).Run(); err != nil {
			b.Fatalf("execute: %v", err)
		}
	}
	b.ReportMetric(20000, "appends/op")
}
