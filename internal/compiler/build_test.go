package compiler

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildFibNativeExecutable(t *testing.T) {
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "fib")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/fib.ts", Output: output, Optimization: "-O2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output || result.Functions != 2 {
		t.Fatalf("build result = %+v", result)
	}
	command := exec.CommandContext(ctx, output)
	nativeOutput, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run native binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "6765" {
		t.Fatalf("native output = %q", got)
	}
}

func TestNormalizeRejectsUnknownOptimization(t *testing.T) {
	_, err := normalizeOptions(BuildOptions{Root: ".", Input: "x.ts", Optimization: "-Ofast"})
	if err == nil {
		t.Fatal("expected optimization validation error")
	}
}

func TestBuildStopsOnTypeScriptErrors(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(root, "examples", "__tsnative_invalid_build_test.ts")
	source := "const value: number = \"not-a-number\";\nconsole.log(1);\n"
	if err := os.WriteFile(input, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(input)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err = Build(ctx, BuildOptions{Root: root, Input: input, Output: filepath.Join(t.TempDir(), "invalid")})
	if err == nil {
		t.Fatal("expected TypeScript diagnostic failure")
	}
	message := err.Error()
	if !strings.Contains(message, "TS2322") || !strings.Contains(message, ":1:7:") {
		t.Fatalf("diagnostic = %q", message)
	}
}
