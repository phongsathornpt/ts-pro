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
		"fib.ts", "scalars.ts", "loops.ts", "do_while.ts", "arrays.ts", "array_writes.ts", "boolean_arrays.ts", "string_arrays.ts", "object_arrays.ts", "function_arrays.ts", "nested_arrays.ts", "any_arrays.ts", "union_arrays.ts", "dynamic_any.ts", "dynamic_any_call.ts", "dynamic_any_bool.ts", "dynamic_any_nullish.ts", "dynamic_any_refs.ts", "dynamic_union.ts", "dynamic_ops.ts", "dynamic_unbox.ts", "dynamic_property_get.ts", "dynamic_property_set.ts", "dynamic_call.ts", "dynamic_method_this.ts", "dynamic_object_coercion.ts",
		"strings.ts", "objects.ts", "classes.ts", "class_fields.ts",
		"closures.ts", "closures_escape.ts", "generics.ts", "gc_churn.ts", "top_level.ts", "top_level_loops.ts", "class_initializers.ts", "class_constructor_effects.ts", "class_mutation.ts", "inheritance.ts", "override_dispatch.ts", "virtual_dispatch.ts", "integer_fast.ts", "concurrency_tasks.ts", "concurrency_captures.ts", "concurrency_results.ts", "concurrency_string_results.ts", "concurrency_object_results.ts", "concurrency_array_results.ts", "concurrency_any_results.ts", "concurrency_function_results.ts", "concurrency_bool_results.ts", "concurrency_sleep.ts", "concurrency_channel_try.ts", "concurrency_channel_blocking.ts", "concurrency_channel_bool.ts", "concurrency_channel_ref.ts", "concurrency_multi_suspend.ts", "concurrency_delayed_join.ts",
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
	if strings.HasPrefix(fixture, "concurrency_") {
		body, err := os.ReadFile(js)
		if err != nil {
			t.Fatal(err)
		}
		shim := []byte("const spawn = fn => ({ result: fn() });\nconst join = task => task.result;\nconst yieldNow = () => {};\nconst sleep = _milliseconds => {};\nconst channel = capacity => ({ capacity, items: [] });\nconst channelTrySend = (ch, value) => { if (ch.items.length >= ch.capacity) return false; ch.items.push(value); return true; };\nconst channelTryRecvOr = (ch, fallback) => ch.items.length ? ch.items.shift() : fallback;\nconst channelSend = (ch, value) => { if (ch.items.length >= ch.capacity) throw new Error('channel full in reference shim'); ch.items.push(value); };\nconst channelRecv = ch => { if (!ch.items.length) throw new Error('channel empty in reference shim'); return ch.items.shift(); };\n")
		if err := os.WriteFile(js, append(shim, body...), 0o644); err != nil {
			t.Fatal(err)
		}
	}
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
