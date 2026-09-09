package cases_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("could not find repo root with go.mod")
	return ""
}

func TestE2E_Bad_CLI(t *testing.T) {
	root := repoRoot(t)
	buildDir := t.TempDir()
	cliBin := filepath.Join(buildDir, "ts-pro")
	buildCmd := exec.Command("go", "build", "-o", cliBin, "./cmd/ts-pro")
	buildCmd.Dir = root
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build ts-pro CLI: %v\nOutput:\n%s", err, string(out))
	}

	tests := []struct {
		name        string
		args        []string
		expectedSub string
	}{
		{
			name:        "non_existent_input_file",
			args:        []string{"build", "non_existent_source_99999.ts"},
			expectedSub: "no such file",
		},
		{
			name:        "missing_arguments_build",
			args:        []string{"build"},
			expectedSub: "usage: ts-pro build",
		},
		{
			name:        "missing_arguments_check",
			args:        []string{"check"},
			expectedSub: "usage: ts-pro check",
		},
		{
			name:        "unknown_subcommand",
			args:        []string{"completely_unknown_command"},
			expectedSub: "unknown command",
		},
		{
			name:        "unsupported_linux_arch",
			args:        []string{"build", filepath.Join(root, "examples/basics/scalars.ts"), "--target=linux-potato"},
			expectedSub: "unsupported target: linux/potato",
		},
		{
			name:        "unwritable_output_directory",
			args:        []string{"build", filepath.Join(root, "examples/basics/fib.ts"), "-o", "/non_existent_dir_9999/bin"},
			expectedSub: "write output binary",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(cliBin, tc.args...)
			cmd.Dir = root
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("expected CLI command to fail for %q, but succeeded with output:\n%s", tc.name, string(out))
			}
			outStr := string(out)
			if !strings.Contains(outStr, tc.expectedSub) {
				t.Errorf("expected output to contain %q, got:\n%s", tc.expectedSub, outStr)
			}
		})
	}
}
