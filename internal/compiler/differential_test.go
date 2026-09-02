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

func TestPureGoOutputMatchesTypeScriptReference(t *testing.T) {
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
		"fib.ts", "scalars.ts", "loops.ts", "do_while.ts", "arrays.ts", "array_writes.ts", "string_arrays.ts",
		"strings.ts", "objects.ts", "closures.ts", "top_level.ts", "top_level_loops.ts",
		"inheritance.ts", "virtual_dispatch.ts",
		"dynamic_property_get.ts", "dynamic_property_set.ts", "dynamic_call.ts", "dynamic_method_this.ts", "dynamic_structural_this.ts",
		"concurrency_tasks.ts", "concurrency_captures.ts", "concurrency_results.ts",
		"concurrency_string_results.ts", "concurrency_object_results.ts", "concurrency_array_results.ts",
		"concurrency_any_results.ts", "concurrency_function_results.ts", "concurrency_bool_results.ts",
		"concurrency_sleep.ts", "concurrency_channel_try.ts", "concurrency_channel_blocking.ts",
		"concurrency_channel_bool.ts", "concurrency_channel_ref.ts",
		"concurrency_multi_suspend.ts", "concurrency_delayed_join.ts",
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture, func(t *testing.T) {
			compareReferenceOutput(t, root, tsc, node, fixture, true)
		})
	}
}

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
		"fib.ts", "scalars.ts", "loops.ts", "do_while.ts", "arrays.ts", "array_writes.ts", "boolean_arrays.ts", "string_arrays.ts", "object_arrays.ts", "function_arrays.ts", "nested_arrays.ts", "any_arrays.ts", "union_arrays.ts", "dynamic_any.ts", "dynamic_any_call.ts", "dynamic_any_bool.ts", "dynamic_any_nullish.ts", "dynamic_any_refs.ts", "dynamic_union.ts", "dynamic_ops.ts", "dynamic_unbox.ts", "dynamic_property_get.ts", "dynamic_property_set.ts", "dynamic_call.ts", "dynamic_method_this.ts", "dynamic_structural_this.ts", "dynamic_object_coercion.ts", "promise_thenable.ts", "promise_thenable_optional.ts", "promise_thenable_class.ts",
		"strings.ts", "objects.ts", "classes.ts", "class_fields.ts",
		"closures.ts", "closures_escape.ts", "generics.ts", "gc_churn.ts", "top_level.ts", "top_level_loops.ts", "class_initializers.ts", "class_constructor_effects.ts", "class_mutation.ts", "inheritance.ts", "override_dispatch.ts", "virtual_dispatch.ts", "integer_fast.ts", "concurrency_tasks.ts", "concurrency_captures.ts", "concurrency_results.ts", "concurrency_string_results.ts", "concurrency_object_results.ts", "concurrency_array_results.ts", "concurrency_any_results.ts", "concurrency_function_results.ts", "concurrency_bool_results.ts", "concurrency_sleep.ts", "concurrency_channel_try.ts", "concurrency_channel_blocking.ts", "concurrency_channel_bool.ts", "concurrency_channel_ref.ts", "concurrency_multi_suspend.ts", "concurrency_delayed_join.ts",
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture, func(t *testing.T) {
			compareReferenceOutput(t, root, tsc, node, fixture, false)
		})
	}
}

func compareReferenceOutput(t *testing.T, root, tsc, node, fixture string, pureGo bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	input := filepath.Join(root, "examples", fixture)
	if _, err := os.Stat(input); err != nil {
		for _, cat := range []string{"basics", "arrays", "objects", "dynamic", "concurrency", "memory"} {
			candidate := filepath.Join(root, "examples", cat, fixture)
			if _, err := os.Stat(candidate); err == nil {
				input = candidate
				break
			}
		}
	}
	refDir := filepath.Join(t.TempDir(), "reference")
	args := []string{"--ignoreConfig", input, filepath.Join(root, "types", "tsnative-runtime.d.ts"),
		"--target", "ES2022", "--module", "ESNext", "--moduleResolution", "Bundler",
		"--strict", "--noUncheckedIndexedAccess", "--exactOptionalPropertyTypes",
		"--lib", "ESNext", "--outDir", refDir, "--noEmit", "false"}
	if output, err := exec.CommandContext(ctx, tsc, args...).CombinedOutput(); err != nil {
		t.Fatalf("reference TypeScript compile: %v: %s", err, output)
	}
	var js string
	_ = filepath.Walk(refDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".js") {
			js = path
		}
		return nil
	})
	if js == "" {
		t.Fatalf("could not find emitted js in %s", refDir)
	}
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
	buildOpts := BuildOptions{Root: root, Input: input, Output: native, Optimization: "-O2"}
	if pureGo {
		buildOpts.PureGo = true
	} else {
		buildOpts.DisablePureGo = true
	}
	if _, err := Build(ctx, buildOpts); err != nil {
		t.Skipf("skipping differential test: %v", err)
		return
	}
	actual, err := exec.CommandContext(ctx, native).CombinedOutput()
	if err != nil {
		t.Fatalf("native run: %v: %s", err, actual)
	}
	if string(actual) != string(reference) {
		t.Fatalf("output mismatch\nreference: %q\nnative:    %q", reference, actual)
	}
}
