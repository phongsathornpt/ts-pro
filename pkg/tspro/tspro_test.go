package tspro

import (
	"testing"
)

func TestCompilerEndToEnd(t *testing.T) {
	src := []byte(`
function add(a: number, b: number): number {
    return a + b;
}
`)

	c := New(Options{
		TargetOS:   "darwin",
		TargetArch: "amd64",
		OptLevel:   2,
	})

	bin, diags, err := c.CompileSource("add.ts", src)
	if err != nil {
		t.Fatalf("CompileSource failed: %v (diags: %s)", err, diags.Format(c.FileSet()))
	}

	if len(bin) < 64 {
		t.Fatalf("generated binary too small: %d bytes", len(bin))
	}

	// Test Linux ELF target
	cLinux := New(Options{
		TargetOS:   "linux",
		TargetArch: "amd64",
		OptLevel:   2,
	})
	binLinux, _, err := cLinux.CompileSource("add.ts", src)
	if err != nil {
		t.Fatalf("Linux CompileSource failed: %v", err)
	}
	if len(binLinux) < 64 {
		t.Fatalf("Linux binary too small: %d bytes", len(binLinux))
	}
}

func TestCompilerCheck(t *testing.T) {
	src := []byte(`
let x: number = "mismatch";
`)
	c := New(DefaultOptions())
	diags := c.Check("test.ts", src)
	if !diags.HasErrors() {
		t.Errorf("expected type error, got none")
	}
}
