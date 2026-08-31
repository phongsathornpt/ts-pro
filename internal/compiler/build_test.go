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

func TestBuildLoopSSAExecutable(t *testing.T) {
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "loops")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := Build(ctx, BuildOptions{Root: root, Input: "examples/loops.ts", Output: output, Optimization: "-O2"}); err != nil {
		t.Fatal(err)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run native binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "45\n45" {
		t.Fatalf("native output = %q", got)
	}
}

func TestBuildTypedNumberArrayExecutable(t *testing.T) {
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "arrays")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := Build(ctx, BuildOptions{Root: root, Input: "examples/arrays.ts", Output: output, Optimization: "-O2"}); err != nil {
		t.Fatal(err)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run native binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "15" {
		t.Fatalf("native output = %q", got)
	}
}

func TestBuildNativeStringExecutable(t *testing.T) {
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "strings")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := Build(ctx, BuildOptions{Root: root, Input: "examples/strings.ts", Output: output, Optimization: "-O2"}); err != nil {
		t.Fatal(err)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run native binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "Hello, TypeScript 7!" {
		t.Fatalf("native output = %q", got)
	}
}

func TestBuildClosedObjectNativeExecutable(t *testing.T) {
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "objects")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/objects.ts", Output: output, Optimization: "-O2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Functions != 2 {
		t.Fatalf("functions = %d", result.Functions)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run native object binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "7" {
		t.Fatalf("native object output = %q", got)
	}
}

func TestBuildClassMethodNativeExecutable(t *testing.T) {
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "classes")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/classes.ts", Output: output, Optimization: "-O2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Functions != 3 {
		t.Fatalf("functions = %d", result.Functions)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run native class binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "7" {
		t.Fatalf("native class output = %q", got)
	}
}

func TestBuildExplicitClassFieldsNativeExecutable(t *testing.T) {
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "class-fields")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/class_fields.ts", Output: output, Optimization: "-O2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Functions != 3 {
		t.Fatalf("functions = %d", result.Functions)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run explicit class fields binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "11" {
		t.Fatalf("native explicit class fields output = %q", got)
	}
}
