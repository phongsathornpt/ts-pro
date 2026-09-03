package e2e_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2E_Bad_CLI(t *testing.T) {
	buildDir := t.TempDir()
	cliBin := filepath.Join(buildDir, "ts-pro")
	buildCmd := exec.Command("go", "build", "-o", cliBin, "../../cmd/ts-pro")
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
			args:        []string{"build", "../../examples/basics/scalars.ts", "--target=linux-potato"},
			expectedSub: "unsupported target: linux/potato",
		},
		{
			name:        "unwritable_output_directory",
			args:        []string{"build", "../../examples/basics/fib.ts", "-o", "/non_existent_dir_9999/bin"},
			expectedSub: "write output binary",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(cliBin, tc.args...)
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
