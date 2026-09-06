package tspro

import (
	"os"
	"os/exec"
	"runtime"
	"testing"
)

func BenchmarkStringConcatChainFusion(b *testing.B) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		b.Skip("native Linux AMD64 execution required")
	}
	src := []byte(`
let result: string = "";
for (let i = 0; i < 100000; i++) {
  result = "abcdefghijklmnop" + "qrstuvwx" + "yz012345" + "6789";
}
console.log(result);
`)
	c := New(Options{TargetOS: "linux", TargetArch: "amd64", OptLevel: 2})
	bin, diags, err := c.CompileSource("concat-chain-bench.ts", src)
	if err != nil {
		b.Fatalf("compile: %v\n%s", err, diags.Format(c.FileSet()))
	}
	path := b.TempDir() + "/concat-chain-bench"
	if err := os.WriteFile(path, bin, 0o755); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if out, err := exec.Command(path).CombinedOutput(); err != nil {
			b.Fatalf("execute: %v: %s", err, out)
		}
	}
	b.ReportMetric(100000, "concat-chains/op")
}
