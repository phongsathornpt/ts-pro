package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/phongsathornpt/ts-pro/pkg/tspro"
)

type happyCase struct {
	name     string
	source   string
	expected string
}

type badCase struct {
	name         string
	source       string
	expectedCode string
	expectedSub  string
}

func runHappy(t *testing.T, tc happyCase) {
	t.Helper()
	dir := t.TempDir()
	binPath := filepath.Join(dir, "e2e_bin")

	opts := tspro.DefaultOptions()
	opts.OptLevel = 2
	compiler := tspro.New(opts)

	bin, diags, err := compiler.CompileSource(tc.name+".ts", []byte(tc.source))
	if err != nil {
		t.Fatalf("compile failed: %v, diagnostics: %s", err, diags.Format(compiler.FileSet()))
	}

	if err := os.WriteFile(binPath, bin, 0755); err != nil {
		t.Fatalf("write binary failed: %v", err)
	}

	if runtime.GOOS == "darwin" {
		_ = exec.Command("codesign", "-s", "-", binPath).Run()
	}

	cmd := exec.Command(binPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("execution failed: %v\nOutput:\n%s", err, string(out))
	}

	if string(out) != tc.expected {
		t.Errorf("expected stdout:\n%q\ngot:\n%q", tc.expected, string(out))
	}
}

func runBad(t *testing.T, tc badCase) {
	t.Helper()
	compiler := tspro.New(tspro.DefaultOptions())
	bin, diags, err := compiler.CompileSource(tc.name+".ts", []byte(tc.source))

	if err == nil {
		t.Fatalf("expected compile failure for bad case %q, but compilation succeeded with binary size %d", tc.name, len(bin))
	}

	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics to report errors for %q, got none", tc.name)
	}

	diagStr := diags.Format(compiler.FileSet())
	foundCode := false
	for _, d := range diags {
		if d.Code == tc.expectedCode {
			foundCode = true
			break
		}
	}

	if !foundCode {
		t.Errorf("expected diagnostic code %q, but got diagnostics:\n%s", tc.expectedCode, diagStr)
	}

	if tc.expectedSub != "" && !strings.Contains(diagStr, tc.expectedSub) {
		t.Errorf("expected message snippet %q in diagnostics:\n%s", tc.expectedSub, diagStr)
	}
}
