package tspro

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
)

func BenchmarkArrayGrowthCopies(b *testing.B) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		b.Skip("native Linux AMD64 execution required")
	}
	src := []byte(`
let total: number = 0;
for (let round = 0; round < 200; round++) {
  let values: number[] = [];
  for (let i = 0; i < 8192; i++) values.push(i);
  total = total + values.length;
}
console.log(total);
`)
	c := New(Options{TargetOS: "linux", TargetArch: "amd64", OptLevel: 2})
	bin, diags, err := c.CompileSource("array-growth-bench.ts", src)
	if err != nil {
		b.Fatalf("compile: %v\n%s", err, diags.Format(c.FileSet()))
	}
	path := b.TempDir() + "/array-growth-bench"
	if err := os.WriteFile(path, bin, 0o755); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if out, err := exec.Command(path).CombinedOutput(); err != nil {
			b.Fatalf("execute: %v: %s", err, out)
		}
	}
	b.ReportMetric(1638400, "array-pushes/op")
}
