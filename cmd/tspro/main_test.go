package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var repoRoot string

func TestMain(m *testing.M) {
	var err error
	repoRoot, err = filepath.Abs("../..")
	if err == nil {
		_ = os.Chdir(repoRoot)
	}
	os.Exit(m.Run())
}

func TestParseBuildArgs(t *testing.T) {
	options, err := parseBuildArgs([]string{
		"examples/basics/fib.ts",
		"-o", "build/fib",
		"-O3",
		"-p", "tsconfig.json",
		"--report-performance",
	})
	if err != nil {
		t.Fatal(err)
	}
	if options.Input != "examples/basics/fib.ts" || options.Output != "build/fib" {
		t.Fatalf("unexpected paths in options: %+v", options)
	}
	if options.Optimization != "-O3" || options.Config != "tsconfig.json" || !options.ReportPerformance {
		t.Fatalf("unexpected flags in options: %+v", options)
	}
	if !options.PureGo {
		t.Fatalf("expected PureGo to be true by default, got false")
	}
}

func TestParseBuildArgsLLVM(t *testing.T) {
	options, err := parseBuildArgs([]string{"examples/basics/fib.ts", "--llvm"})
	if err != nil {
		t.Fatal(err)
	}
	if options.PureGo || !options.DisablePureGo {
		t.Fatalf("expected pure-go disabled for --llvm: %+v", options)
	}
}

func TestParseBuildArgsErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"empty", []string{}},
		{"missing output path", []string{"fib.ts", "-o"}},
		{"missing project path", []string{"fib.ts", "-p"}},
		{"unknown option", []string{"fib.ts", "--invalid-flag"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseBuildArgs(tc.args); err == nil {
				t.Fatalf("expected error for args %v", tc.args)
			}
		})
	}
}

func TestRunVersion(t *testing.T) {
	for _, flag := range []string{"version", "--version", "-version"} {
		if err := run([]string{flag}); err != nil {
			t.Fatalf("run %q: %v", flag, err)
		}
	}
}

func TestRunEmptyUsage(t *testing.T) {
	if err := run([]string{}); err != nil {
		t.Fatalf("run empty: %v", err)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	if err := run([]string{"bogus-command"}); err == nil {
		t.Fatal("expected error for unknown command")
	}
}

func TestBuildFibE2E(t *testing.T) {
	input := filepath.Join(repoRoot, "examples", "basics", "fib.ts")
	output := filepath.Join(t.TempDir(), "fib-e2e")

	if err := run([]string{"build", input, "-o", output}); err != nil {
		t.Fatalf("run build: %v", err)
	}

	nativeOutput, err := exec.Command(output).CombinedOutput()
	if err != nil {
		t.Fatalf("run generated binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "6765" {
		t.Fatalf("output = %q, want 6765", got)
	}
}

func TestBuildConcurrencySleepE2E(t *testing.T) {
	input := filepath.Join(repoRoot, "examples", "concurrency", "concurrency_sleep.ts")
	output := filepath.Join(t.TempDir(), "sleep-e2e")

	if err := run([]string{"build", input, "-o", output}); err != nil {
		t.Fatalf("run build: %v", err)
	}

	nativeOutput, err := exec.Command(output).CombinedOutput()
	if err != nil {
		t.Fatalf("run generated binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "42" {
		t.Fatalf("output = %q, want 42", got)
	}
}

func TestSubprocessCLIE2E(t *testing.T) {
	binaryPath := filepath.Join(repoRoot, "bin", "tspro")
	if _, err := os.Stat(binaryPath); err != nil {
		buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
		buildCmd.Dir = filepath.Join(repoRoot, "cmd", "tspro")
		buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := buildCmd.CombinedOutput(); err != nil {
			t.Fatalf("compile tspro CLI: %v: %s", err, out)
		}
	}

	input := filepath.Join(repoRoot, "examples", "basics", "fib.ts")
	output := filepath.Join(t.TempDir(), "fib-cli-e2e")

	cmd := exec.Command(binaryPath, "build", input, "-o", output, "--report-performance")
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tspro CLI execution failed: %v: %s", err, out)
	}

	outStr := string(out)
	if !strings.Contains(outStr, "Built") {
		t.Fatalf("expected 'Built' in output: %s", outStr)
	}
	if !strings.Contains(strings.ToLower(outStr), "performance report") {
		t.Fatalf("expected performance report in output: %s", outStr)
	}

	nativeOutput, err := exec.Command(output).CombinedOutput()
	if err != nil {
		t.Fatalf("run generated binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "6765" {
		t.Fatalf("output = %q, want 6765", got)
	}
}
