package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCLIVersionAndDoctor(t *testing.T) {
	if err := run([]string{"version"}); err != nil {
		t.Errorf("run(version) failed: %v", err)
	}

	if err := run([]string{"doctor"}); err != nil {
		t.Errorf("run(doctor) failed: %v", err)
	}
}

func TestCLIBldAndCheck(t *testing.T) {
	dir := t.TempDir()
	tsFile := filepath.Join(dir, "fib.ts")
	outFile := filepath.Join(dir, "fib_bin")

	src := []byte(`
function fib(n: number): number {
    return n <= 1 ? n : fib(n - 1) + fib(n - 2);
}
`)
	if err := os.WriteFile(tsFile, src, 0o644); err != nil {
		t.Fatal(err)
	}

	// Check
	if err := run([]string{"check", tsFile}); err != nil {
		t.Errorf("run(check) failed: %v", err)
	}

	// Build
	if err := run([]string{"build", "-o", outFile, tsFile}); err != nil {
		t.Errorf("run(build) failed: %v", err)
	}

	info, err := os.Stat(outFile)
	if err != nil || info.Size() == 0 {
		t.Errorf("expected generated output binary at %s, info: %v, err: %v", outFile, info, err)
	}
}
