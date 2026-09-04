package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMainEntryPoint(t *testing.T) {
	// Exercise main() with os.Args
	oldArgs := os.Args
	oldExit := exitFunc
	defer func() {
		os.Args = oldArgs
		exitFunc = oldExit
	}()
	os.Args = []string{"ts-pro", "version"}
	main()

	// Exercise main() with error
	var exitCode int
	exitFunc = func(code int) {
		exitCode = code
	}
	os.Args = []string{"ts-pro", "unknown_subcommand"}
	main()
	if exitCode != 1 {
		t.Errorf("expected exitCode 1 on error, got %d", exitCode)
	}
}

func TestCLIVersionAndDoctor(t *testing.T) {
	if err := run([]string{"version"}); err != nil {
		t.Errorf("run(version) failed: %v", err)
	}
	if err := run([]string{"--version"}); err != nil {
		t.Errorf("run(--version) failed: %v", err)
	}
	if err := run([]string{"-v"}); err != nil {
		t.Errorf("run(-v) failed: %v", err)
	}

	if err := run([]string{"doctor"}); err != nil {
		t.Errorf("run(doctor) failed: %v", err)
	}
}

func TestCLIHelpAndNoArgs(t *testing.T) {
	if err := run([]string{}); err != nil {
		t.Errorf("run(empty) failed: %v", err)
	}
	if err := run([]string{"help"}); err != nil {
		t.Errorf("run(help) failed: %v", err)
	}
	if err := run([]string{"--help"}); err != nil {
		t.Errorf("run(--help) failed: %v", err)
	}
	if err := run([]string{"-h"}); err != nil {
		t.Errorf("run(-h) failed: %v", err)
	}
	if err := run([]string{"unknown-command"}); err == nil {
		t.Errorf("expected error on unknown command, got nil")
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
console.log(fib(5));
`)
	if err := os.WriteFile(tsFile, src, 0o644); err != nil {
		t.Fatal(err)
	}

	// Check success
	if err := run([]string{"check", tsFile}); err != nil {
		t.Errorf("run(check) failed: %v", err)
	}

	// Check missing args
	if err := run([]string{"check"}); err == nil {
		t.Errorf("expected error on check without args")
	}

	// Check non-existent file
	if err := run([]string{"check", filepath.Join(dir, "nonexistent.ts")}); err == nil {
		t.Errorf("expected error on check nonexistent file")
	}

	// Check with type errors
	badTs := filepath.Join(dir, "bad.ts")
	if err := os.WriteFile(badTs, []byte("let x: number = 'mismatch';"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"check", badTs}); err == nil {
		t.Errorf("expected type check failure for bad.ts")
	}

	// Build missing input file
	if err := run([]string{"build"}); err == nil {
		t.Errorf("expected error on build without input")
	}

	// Build with different optimization flags and target formats
	for _, optFlag := range []string{"-O0", "-O1", "-O2", "-O3"} {
		out := filepath.Join(dir, "out_"+optFlag)
		if err := run([]string{"build", optFlag, "-o", out, tsFile}); err != nil {
			t.Errorf("run(build %s) failed: %v", optFlag, err)
		}
	}

	// Build with -o= prefix and --target= prefix
	outPrefixed := filepath.Join(dir, "out_prefixed")
	if err := run([]string{"build", "-o=" + outPrefixed, "--target=linux-amd64", tsFile}); err != nil {
		t.Errorf("run(build with prefixes) failed: %v", err)
	}

	// Build with --target two-arg and default output path
	origDir, _ := os.Getwd()
	if err := os.Chdir(dir); err == nil {
		if err := run([]string{"build", "--target", "linux-amd64", "fib.ts"}); err != nil {
			t.Errorf("run(build default out) failed: %v", err)
		}
		_ = os.Chdir(origDir)
	}

	// Build with darwin target to hit codesign branch
	outDarwin := filepath.Join(dir, "out_darwin")
	_ = run([]string{"build", "--target=darwin-arm64", "-o", outDarwin, tsFile})

	// Build with bad target
	if err := run([]string{"build", "--target=unsupported-os", tsFile}); err == nil {
		t.Errorf("expected error on unsupported target")
	}

	// Build with malformed code
	if err := run([]string{"build", badTs}); err == nil {
		t.Errorf("expected build failure for bad.ts")
	}

	// Standard build check
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
		{
			name:       "strings concatenation and printing",
			sourcePath: "../../examples/basics/strings.ts",
			expected:   "Hello, TypeScript 7!\n",
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
