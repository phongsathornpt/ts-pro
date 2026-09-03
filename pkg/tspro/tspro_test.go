package tspro

import (
	"os"
	"path/filepath"
	"strings"
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
		TargetArch: "arm64",
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

func TestCompileFileRejectsCyclicRelativeImports(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.ts")
	b := filepath.Join(dir, "b.ts")
	if err := os.WriteFile(a, []byte(`import { b } from "./b"; export function a(): number { return b(); }`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte(`import { a } from "./a"; export function b(): number { return a(); }`), 0o644); err != nil {
		t.Fatal(err)
	}
	c := New(Options{TargetOS: "linux", TargetArch: "amd64", OptLevel: 2})
	_, err := c.CompileFile(a, filepath.Join(dir, "out"))
	if err == nil || !strings.Contains(err.Error(), "cyclic module import") {
		t.Fatalf("CompileFile cycle error = %v", err)
	}
}
