package main

import (
	"os"
	"os/exec"
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

func TestEndToEndExecution(t *testing.T) {
	tests := []struct {
		name       string
		sourcePath string
		expected   string
	}{
		{
			name:       "fibonacci recursive",
			sourcePath: "../../examples/basics/fib.ts",
			expected:   "6765\n",
		},
		{
			name:       "scalars arithmetic and comparisons",
			sourcePath: "../../examples/basics/scalars.ts",
			expected:   "20\n1\n",
		},
		{
			name:       "loops while and for",
			sourcePath: "../../examples/basics/loops.ts",
			expected:   "45\n45\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			binPath := filepath.Join(dir, "prog_bin")

			if err := run([]string{"build", "-o", binPath, tc.sourcePath}); err != nil {
				t.Fatalf("build failed: %v", err)
			}

			cmd := exec.Command(binPath)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("execution failed: %v\nOutput:\n%s", err, string(out))
			}

			if string(out) != tc.expected {
				t.Errorf("expected output %q, got %q", tc.expected, string(out))
			}
		})
	}
}
