package compiler

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeOutputMatchesTypeScriptReference(t *testing.T) {
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	tsc := filepath.Join(root, "node_modules", ".bin", "tsc")
	fixtures := []string{
		"fib.ts", "scalars.ts", "loops.ts", "arrays.ts",
		"strings.ts", "objects.ts", "classes.ts", "class_fields.ts",
		"closures.ts", "closures_escape.ts", "generics.ts", "gc_churn.ts", "top_level.ts", "class_initializers.ts", "class_constructor_effects.ts", "class_mutation.ts",
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture, func(t *testing.T) {
			compareReferenceOutput(t, root, tsc, node, fixture)
		})
	}
}

func compareReferenceOutput(t *testing.T, root, tsc, node, fixture string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	input := filepath.Join(root, "examples", fixture)
	refDir := filepath.Join(t.TempDir(), "reference")
	args := []string{"--ignoreConfig", input, filepath.Join(root, "types", "tsnative-runtime.d.ts"),
		"--target", "ES2022", "--module", "ESNext", "--moduleResolution", "Bundler",
		"--strict", "--noUncheckedIndexedAccess", "--exactOptionalPropertyTypes",
		"--lib", "ESNext", "--outDir", refDir, "--noEmit", "false"}
	if output, err := exec.CommandContext(ctx, tsc, args...).CombinedOutput(); err != nil {
		t.Fatalf("reference TypeScript compile: %v: %s", err, output)
	}
	js := filepath.Join(refDir, strings.TrimSuffix(fixture, filepath.Ext(fixture))+".js")
	reference, err := exec.CommandContext(ctx, node, js).CombinedOutput()
	if err != nil {
		t.Fatalf("reference Node run: %v: %s", err, reference)
	}
	native := filepath.Join(t.TempDir(), "native")
	if _, err := Build(ctx, BuildOptions{Root: root, Input: input, Output: native, Optimization: "-O2"}); err != nil {
		t.Fatalf("native build: %v", err)
	}
	actual, err := exec.CommandContext(ctx, native).CombinedOutput()
	if err != nil {
		t.Fatalf("native run: %v: %s", err, actual)
	}
	if string(actual) != string(reference) {
		t.Fatalf("output mismatch\nreference: %q\nnative:    %q", reference, actual)
	}
}
