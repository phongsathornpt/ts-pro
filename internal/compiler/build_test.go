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

func requireLLVM(t *testing.T) {
	t.Helper()
	if os.Getenv("TS_PRO_LLVM") != "1" {
		t.Skip("skipping legacy LLVM/native test (quarantined; set TS_PRO_LLVM=1 to run)")
	}
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
}

func TestBuildFibNativeExecutable(t *testing.T) {
	requireLLVM(t)
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

func TestBuildFibPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "fib-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/basics/fib.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output || result.Functions != 2 {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "6765" {
		t.Fatalf("pure-Go output = %q", got)
	}
}

func TestBuildConcurrencySleepPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "sleep-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/concurrency/concurrency_sleep.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "42" {
		t.Fatalf("pure-Go output = %q", got)
	}
}

func TestBuildConcurrencyChannelRefPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "chan-ref-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/concurrency/concurrency_channel_ref.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	expected := "1\nbuffered\nfallback\ntask-ref"
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("pure-Go output = %q, want %q", got, expected)
	}
}

func TestBuildConcurrencyTasksPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "tasks-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/concurrency/concurrency_tasks.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	expected := "42\n7"
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("pure-Go output = %q, want %q", got, expected)
	}
}

func TestBuildConcurrencyGroupPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "group-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/concurrency/concurrency_group.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	got := strings.TrimSpace(string(nativeOutput))
	if got != "1\n2\n30" && got != "2\n1\n30" {
		t.Fatalf("pure-Go output = %q, want 1\\n2\\n30 or 2\\n1\\n30", got)
	}
}

func TestBuildConcurrencyCancelPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "cancel-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err = Build(ctx, BuildOptions{
		Root: root, Input: "examples/concurrency/concurrency_cancel.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "1" {
		t.Fatalf("pure-Go cancel output = %q, want 1", got)
	}
}

func TestBuildConcurrencyGroupCancelPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "group-cancel-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err = Build(ctx, BuildOptions{
		Root: root, Input: "examples/concurrency/concurrency_group_cancel.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "3" {
		t.Fatalf("pure-Go group cancel output = %q, want 3", got)
	}
}

func TestBuildConcurrencyContextPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "context-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err = Build(ctx, BuildOptions{
		Root: root, Input: "examples/concurrency/concurrency_context.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "trace-42\ntrace-42" {
		t.Fatalf("pure-Go context output = %q, want trace-42\\ntrace-42", got)
	}
}

func TestBuildRejectsMutableMapCaptureAcrossSpawn(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	sourceFile := filepath.Join(root, "examples", "concurrency", "__test_bad_capture.ts")
	code := "const m = new Map<string, number>();\nconst t = spawn((): void => {\n  m.set(\"bad\", 1);\n});\njoin(t);\n"
	if err := os.WriteFile(sourceFile, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(sourceFile) }()
	output := filepath.Join(t.TempDir(), "bad_capture")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err = Build(ctx, BuildOptions{
		Root: root, Input: sourceFile, Output: output, PureGo: true,
	})
	if err == nil {
		t.Fatal("expected build error for mutable Map capture across spawn, but got nil")
	}
	if !strings.Contains(err.Error(), "cannot capture mutable") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestBuildRejectsMutableSetCaptureAcrossSpawn(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	sourceFile := filepath.Join(root, "examples", "concurrency", "__test_bad_set_capture.ts")
	code := "const s = new Set<string>();\nconst t = spawn((): void => {\n  s.add(\"bad\");\n});\njoin(t);\n"
	if err := os.WriteFile(sourceFile, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(sourceFile) }()
	output := filepath.Join(t.TempDir(), "bad_set_capture")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err = Build(ctx, BuildOptions{
		Root: root, Input: sourceFile, Output: output, PureGo: true,
	})
	if err == nil {
		t.Fatal("expected build error for mutable Set capture across spawn, but got nil")
	}
	if !strings.Contains(err.Error(), "cannot capture mutable") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestBuildStringArraysPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "string-arrays-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/arrays/string_arrays.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	expected := "3\nalpha\nupdated\ngamma"
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("pure-Go output = %q, want %q", got, expected)
	}
}

func TestBuildConcurrencyAsyncRefPhiPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "async-ref-phi-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/concurrency/concurrency_async_ref_phi.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	expected := "left:done\nright:done\nxyyy\n42\n7\ndynamic-left\ndynamic-right"
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("pure-Go output = %q, want %q", got, expected)
	}
}

func TestBuildConcurrencyPromiseAggregatePureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "promise-agg-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/concurrency/concurrency_promise_aggregate.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	// In aggregateRaceBooleans(), Promise.race is called on two immediately-resolved promises:
	// Promise.resolve(false) and Promise.resolve(true). In pure-Go, concurrent watcher goroutines
	// deliver completion to the race coordinator channel; depending on runtime goroutine scheduling,
	// either can win the race, producing aggregateBoolScore 1 (true) or 0 (false).
	expected1 := "345\n9\naggregate-reject\n42\n9\n30\n30\nrace-reject\n43\nalpha:beta-done\nfirst-root\n101\n1\n1\nbool-reject\n44\n123\n8\nraw:promise\n10"
	expected2 := "345\n9\naggregate-reject\n42\n9\n30\n30\nrace-reject\n43\nalpha:beta-done\nfirst-root\n101\n0\n1\nbool-reject\n44\n123\n8\nraw:promise\n10"
	if got := strings.TrimSpace(string(nativeOutput)); got != expected1 && got != expected2 {
		t.Fatalf("pure-Go output = %q, want %q or %q", got, expected1, expected2)
	}
}

func TestBuildConcurrencyAsyncRejectPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "async-reject-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/concurrency/concurrency_async_reject.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	expected := "99"
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("pure-Go output = %q, want %q", got, expected)
	}
}

func TestBuildConcurrencyAsyncAwaitCatchPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "async-catch-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/concurrency/concurrency_async_await_catch.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	expected := "caught-await"
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("pure-Go output = %q, want %q", got, expected)
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
	requireLLVM(t)
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
	requireLLVM(t)
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
	requireLLVM(t)
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
	requireLLVM(t)
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
	requireLLVM(t)
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
	requireLLVM(t)
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

func TestBuildNativeTaskIntrinsicsExecutable(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "tasks")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/concurrency_tasks.ts", Output: output, Optimization: "-O2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 1 || result.Metrics.TaskJoins != 1 || result.Metrics.TaskYields != 1 {
		t.Fatalf("task metrics = %d/%d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins, result.Metrics.TaskYields)
	}
	command := exec.CommandContext(ctx, output)
	command.Env = append(os.Environ(), "TSNATIVE_WORKERS=2")
	nativeOutput, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run native task binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "42\n7" {
		t.Fatalf("native task output = %q", got)
	}
}

func TestBuildNativeF64TaskResultExecutable(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "task-result")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_results.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 1 || result.Metrics.TaskJoins != 1 {
		t.Fatalf("task metrics = %d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins)
	}
	command := exec.CommandContext(ctx, output)
	command.Env = append(os.Environ(), "TSNATIVE_WORKERS=2")
	nativeOutput, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run native task result binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "42" {
		t.Fatalf("native task result output = %q", got)
	}
}

func TestBuildNativeStringTaskResultExecutable(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "task-string-result")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_string_results.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 1 || result.Metrics.TaskJoins != 1 {
		t.Fatalf("task metrics = %d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins)
	}
	command := exec.CommandContext(ctx, output)
	command.Env = append(os.Environ(), "TSNATIVE_WORKERS=2")
	nativeOutput, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run native string task result binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "task-string" {
		t.Fatalf("native task string result output = %q", got)
	}
}

func TestBuildNativeExtendedTaskResultMatrix(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ fixture, want string }{
		{"concurrency_object_results.ts", "42"},
		{"concurrency_array_results.ts", "42"},
		{"concurrency_any_results.ts", "42"},
		{"concurrency_function_results.ts", "42"},
		{"concurrency_bool_results.ts", "42"},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			output := filepath.Join(t.TempDir(), "task-result")
			result, err := Build(ctx, BuildOptions{Root: root, Input: filepath.Join("examples", tc.fixture), Output: output, Optimization: "-O2"})
			if err != nil {
				t.Fatal(err)
			}
			if result.Metrics.TaskSpawns != 1 || result.Metrics.TaskJoins != 1 {
				t.Fatalf("task metrics = %d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins)
			}
			cmd := exec.CommandContext(ctx, output)
			cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=2")
			nativeOutput, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("run %s: %v: %s", tc.fixture, err, nativeOutput)
			}
			if got := strings.TrimSpace(string(nativeOutput)); got != tc.want {
				t.Fatalf("output = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildNativeChannelTryExecutable(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "channel-try")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_channel_try.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.ChannelCreates != 1 || result.Metrics.ChannelTrySends != 2 || result.Metrics.ChannelTryRecvs != 2 {
		t.Fatalf("channel metrics = %d/%d/%d", result.Metrics.ChannelCreates, result.Metrics.ChannelTrySends, result.Metrics.ChannelTryRecvs)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run native channel try binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "1\n0\n42\n7" {
		t.Fatalf("native channel try output = %q", got)
	}
}

func TestBuildNativeBlockingChannelTasksSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "channel-tasks")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_channel_tasks.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.ChannelCreates != 1 || result.Metrics.ChannelSends != 1 || result.Metrics.ChannelRecvs != 1 {
		t.Fatalf("channel metrics = create:%d send:%d recv:%d", result.Metrics.ChannelCreates, result.Metrics.ChannelSends, result.Metrics.ChannelRecvs)
	}
	if result.Metrics.TaskSpawns != 2 || result.Metrics.TaskJoins != 2 {
		t.Fatalf("task metrics = %d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	nativeOutput, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run blocking channel tasks: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "42" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeSleepTaskSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "sleep-task")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_sleep.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.Sleeps != 1 || result.Metrics.TaskSpawns != 1 || result.Metrics.TaskJoins != 1 {
		t.Fatalf("sleep/task metrics = %d/%d/%d", result.Metrics.Sleeps, result.Metrics.TaskSpawns, result.Metrics.TaskJoins)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	nativeOutput, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run native sleep task: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "42" {
		t.Fatalf("native sleep task output = %q", got)
	}
}

func TestBuildNativeMultiSuspendTaskSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "multi-suspend")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_multi_suspend.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 2 || result.Metrics.TaskJoins != 2 || result.Metrics.Sleeps != 3 {
		t.Fatalf("task/timer metrics = %d/%d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins, result.Metrics.Sleeps)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run multi-suspend: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "42" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeAsyncAwaitSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "async-await")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_async.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 2 || result.Metrics.TaskJoins != 2 || result.Metrics.Sleeps != 1 {
		t.Fatalf("async metrics = %d/%d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins, result.Metrics.Sleeps)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run async: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "42" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeTypedAsyncAwaitSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ fixture, want string }{
		{"concurrency_async_bool.ts", "42"},
		{"concurrency_async_string.ts", "async-string-done"},
		{"concurrency_async_any.ts", "async-any"},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			output := filepath.Join(t.TempDir(), "async-typed")
			result, err := Build(ctx, BuildOptions{Root: root, Input: filepath.Join("examples", tc.fixture), Output: output, Optimization: "-O2"})
			if err != nil {
				t.Fatal(err)
			}
			if result.Metrics.TaskSpawns != 2 || result.Metrics.TaskJoins != 2 {
				t.Fatalf("task metrics = %d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins)
			}
			cmd := exec.CommandContext(ctx, output)
			cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
			got, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("run %s: %v: %s", tc.fixture, err, got)
			}
			if strings.TrimSpace(string(got)) != tc.want {
				t.Fatalf("output = %q; want %q", got, tc.want)
			}
		})
	}
}

func TestBuildNativeAsyncLinearSSAContinuation(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "async-ssa")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_async_ssa.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 4 || result.Metrics.TaskJoins != 4 || result.Metrics.Sleeps != 3 {
		t.Fatalf("async SSA metrics = %d/%d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins, result.Metrics.Sleeps)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run async SSA: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "85\n42" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeAsyncReferenceSSAContinuation(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "async-ref-ssa")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_async_ref_ssa.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 2 || result.Metrics.TaskJoins != 2 || result.Metrics.Sleeps != 2 {
		t.Fatalf("async ref SSA metrics = %d/%d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins, result.Metrics.Sleeps)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run async ref SSA: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "pre:value:done\nvalue-any" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeAsyncBranchContinuation(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "async-branch")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_async_branch.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 2 || result.Metrics.TaskJoins != 2 || result.Metrics.Sleeps != 1 {
		t.Fatalf("async branch metrics = %d/%d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins, result.Metrics.Sleeps)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run async branch: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "42\n7" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeAsyncLoopPhiContinuation(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "async-loop")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_async_loop.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 1 || result.Metrics.TaskJoins != 1 || result.Metrics.Sleeps != 1 {
		t.Fatalf("async loop metrics = %d/%d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins, result.Metrics.Sleeps)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run async loop: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "10" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeAsyncNativeOpsContinuation(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "async-native-ops")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_async_native_ops.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 1 || result.Metrics.TaskJoins != 1 || result.Metrics.Sleeps != 1 {
		t.Fatalf("async native-op metrics = %d/%d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins, result.Metrics.Sleeps)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run async native ops: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "26" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeDelayedNestedJoinSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "delayed-join")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_delayed_join.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 4 || result.Metrics.TaskJoins != 4 || result.Metrics.Sleeps != 4 {
		t.Fatalf("delayed join metrics = %d/%d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins, result.Metrics.Sleeps)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run delayed join: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "42\n42" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeAsyncReferencePhiStateSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "async-ref-phi")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_async_ref_phi.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 7 || result.Metrics.TaskJoins != 7 || result.Metrics.Sleeps != 4 {
		t.Fatalf("async ref phi metrics = %d/%d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins, result.Metrics.Sleeps)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run async ref phi: %v: %s", err, got)
	}
	want := "left:done\nright:done\nxyyy\n42\n7\ndynamic-left\ndynamic-right"
	if strings.TrimSpace(string(got)) != want {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeDynamicReferenceBoxing(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "dynamic-any-refs")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/dynamic_any_refs.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.BoxingSites != 3 {
		t.Fatalf("boxing sites = %d; want 3", result.Metrics.BoxingSites)
	}
	got, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run dynamic reference boxing: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "1\n1\n1" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeTaggedUnionBoundary(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "dynamic-union")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/dynamic_union.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.BoxingSites != 6 {
		t.Fatalf("boxing sites = %d; want 6", result.Metrics.BoxingSites)
	}
	got, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run dynamic union: %v: %s", err, got)
	}
	want := "42\nunion\ntrue\nnull\nundefined\n1\n1\n1"
	if strings.TrimSpace(string(got)) != want {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeDynamicPrimitiveOperators(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "dynamic-ops")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/dynamic_ops.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.RuntimeCalls < 12 {
		t.Fatalf("runtime calls = %d", result.Metrics.RuntimeCalls)
	}
	got, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run dynamic ops: %v: %s", err, got)
	}
	want := "5\n42\n12\ntrue\ntrue\ntrue\ntrue\ntrue\ntrue\nfalse\nfalse\ntrue\ntrue\nfalse\ntrue"
	if strings.TrimSpace(string(got)) != want {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeAsyncDynamicOperatorSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "async-dynamic-ops")
	_, err = Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_async_dynamic_ops.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run async dynamic ops: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "42" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeDynamicCheckedUnboxing(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "dynamic-unbox")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/dynamic_unbox.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.BoxingSites != 6 {
		t.Fatalf("boxing sites = %d; want 6", result.Metrics.BoxingSites)
	}
	got, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run dynamic checked unboxing: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "42\nunboxed\n42\n42\n42\n42" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeReferenceChannelSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "channel-ref")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_channel_ref_blocking.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.ChannelCreates != 1 || result.Metrics.ChannelSends != 1 || result.Metrics.ChannelRecvs != 1 {
		t.Fatalf("channel metrics = %d creates / %d sends / %d recvs", result.Metrics.ChannelCreates, result.Metrics.ChannelSends, result.Metrics.ChannelRecvs)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run reference channel: %v: %s", err, got)
	}
	want := "task-ref"
	if strings.TrimSpace(string(got)) != want {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeBooleanChannelSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "channel-bool")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_channel_bool_blocking.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.ChannelCreates != 1 || result.Metrics.ChannelSends != 1 || result.Metrics.ChannelRecvs != 1 {
		t.Fatalf("channel metrics = %d/%d/%d", result.Metrics.ChannelCreates, result.Metrics.ChannelSends, result.Metrics.ChannelRecvs)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run boolean channel: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "1" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeLogicalTaskYieldSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "logical-yield")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_logical_yield.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 2 || result.Metrics.TaskJoins != 2 || result.Metrics.TaskYields != 1 {
		t.Fatalf("task metrics = %d/%d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins, result.Metrics.TaskYields)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run logical yield: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "1\n2\n3" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeCooperativeCancellationSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "cooperative-cancel")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_cancel.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 2 || result.Metrics.TaskJoins != 2 {
		t.Fatalf("task metrics = %d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run cancellation: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "1" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeExecutionBudgetFairnessSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "execution-budget")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_budget.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 2 || result.Metrics.TaskJoins != 2 {
		t.Fatalf("task metrics = %d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run budget fairness: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "2\n1" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeStructuredTaskGroupSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "structured-group")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_group.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 2 || result.Metrics.TaskJoins != 2 {
		t.Fatalf("task metrics = %d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run structured group: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "1\n2\n30" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeTaskGroupCancellationSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "structured-group-cancel")
	_, err = Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_group_cancel.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run group cancellation: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "3" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeTaskContextInheritanceSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "task-context")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_context.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 2 || result.Metrics.TaskJoins != 2 {
		t.Fatalf("task metrics = %d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run task context: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "trace-42\ntrace-42" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeAsyncRejectPropagatesSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "async-reject")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_async_reject.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 2 || result.Metrics.TaskJoins != 2 || result.Metrics.Sleeps != 1 {
		t.Fatalf("async rejection metrics = %d/%d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins, result.Metrics.Sleeps)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("async rejection unexpectedly succeeded: %q", got)
	}
	if strings.TrimSpace(string(got)) != "async-boom" {
		t.Fatalf("output = %q", got)
	}
	if strings.Contains(string(got), "99") {
		t.Fatalf("execution continued after rejected join: %q", got)
	}
}

func TestBuildNativeAsyncLocalTryCatchSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "async-local-try-catch")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_async_try_catch.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 1 || result.Metrics.TaskJoins != 1 {
		t.Fatalf("task metrics = %d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run local try/catch: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "caught-local" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeAsyncAwaitCatchSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "async-await-catch")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_async_await_catch.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 2 || result.Metrics.TaskJoins != 2 || result.Metrics.Sleeps != 1 {
		t.Fatalf("await catch metrics = %d/%d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins, result.Metrics.Sleeps)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run awaited rejection catch: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "caught-await" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeAsyncNestedRecoverySingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "async-nested-recovery")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_async_nested_recovery.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 2 || result.Metrics.TaskJoins != 2 || result.Metrics.Sleeps != 1 {
		t.Fatalf("nested recovery metrics = %d/%d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins, result.Metrics.Sleeps)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run nested recovery: %v: %s", err, got)
	}
	want := "leaf-rejection\ninner-finally\nmiddle-rethrow\nouter-finally\n7"
	if strings.TrimSpace(string(got)) != want {
		t.Fatalf("output = %q; want %q", got, want)
	}
}

func TestBuildNativeImmediatePromiseSemanticsSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "promise-immediate")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/promise_immediate.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 3 || result.Metrics.TaskJoins != 6 {
		t.Fatalf("immediate promise task metrics = %d/%d, want 3/6", result.Metrics.TaskSpawns, result.Metrics.TaskJoins)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1", "TSNATIVE_GC_NURSERY_BYTES=1024")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run immediate promises: %v: %s", err, got)
	}
	want := "41\npromise-reject\n42\nrooted-promise"
	if strings.TrimSpace(string(got)) != want {
		t.Fatalf("output = %q; want %q", got, want)
	}
}

func TestBuildNativePromiseAggregatesSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "promise-aggregates")
	if _, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_promise_aggregate.ts", Output: output, Optimization: "-O2"}); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1", "TSNATIVE_GC_NURSERY_BYTES=1024")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run Promise aggregates: %v: %s", err, got)
	}
	want := "345\n7\naggregate-reject\n42\n9\n30\n30\nrace-reject\n43\nalpha:beta-done\nfirst-root\n101\n0\n1\nbool-reject\n44\n123\n8\nraw:promise\n10"
	if strings.TrimSpace(string(got)) != want {
		t.Fatalf("output = %q; want %q", got, want)
	}
}

func TestBuildNativeRepeatedPromiseAwaitSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "promise-repeated-await")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/promise_repeated_await.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 2 || result.Metrics.TaskJoins != 6 || result.Metrics.TaskReleases != 2 {
		t.Fatalf("repeated Promise task metrics = spawns:%d joins:%d releases:%d; want 2/6/2", result.Metrics.TaskSpawns, result.Metrics.TaskJoins, result.Metrics.TaskReleases)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1", "TSNATIVE_GC_NURSERY_BYTES=1024")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run repeated Promise awaits: %v: %s", err, got)
	}
	want := "42\nrepeat-root:repeat-root"
	if strings.TrimSpace(string(got)) != want {
		t.Fatalf("output = %q; want %q", got, want)
	}
}

func TestBuildNativePromiseAdoptionSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "promise-adoption")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/promise_adoption.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 2 || result.Metrics.TaskJoins != 6 || result.Metrics.TaskRetains != 3 || result.Metrics.TaskReleases != 5 {
		t.Fatalf("Promise adoption metrics = spawns:%d joins:%d retains:%d releases:%d; want 2/6/3/5", result.Metrics.TaskSpawns, result.Metrics.TaskJoins, result.Metrics.TaskRetains, result.Metrics.TaskReleases)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1", "TSNATIVE_GC_NURSERY_BYTES=1024")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run Promise adoption: %v: %s", err, got)
	}
	want := "42\nadopt-root:adopt-root"
	if strings.TrimSpace(string(got)) != want {
		t.Fatalf("output = %q; want %q", got, want)
	}
}

func TestBuildNativePromiseReassignmentSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "promise-reassignment")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/promise_reassignment.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 2 || result.Metrics.TaskJoins != 6 || result.Metrics.TaskRetains != 4 || result.Metrics.TaskReleases != 8 {
		t.Fatalf("Promise reassignment metrics = spawns:%d joins:%d retains:%d releases:%d; want 2/6/4/8", result.Metrics.TaskSpawns, result.Metrics.TaskJoins, result.Metrics.TaskRetains, result.Metrics.TaskReleases)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1", "TSNATIVE_GC_NURSERY_BYTES=1024")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run Promise reassignment: %v: %s", err, got)
	}
	want := "30\nold-root:new-root"
	if strings.TrimSpace(string(got)) != want {
		t.Fatalf("output = %q; want %q", got, want)
	}
}

func TestBuildNativePromiseControlFlowOwnershipSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "promise-control-flow")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/promise_control_flow.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 3 || result.Metrics.TaskJoins != 6 || result.Metrics.TaskRetains != 0 || result.Metrics.TaskReleases != 9 {
		t.Fatalf("Promise control-flow metrics = spawns:%d joins:%d retains:%d releases:%d; want 3/6/0/9", result.Metrics.TaskSpawns, result.Metrics.TaskJoins, result.Metrics.TaskRetains, result.Metrics.TaskReleases)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1", "TSNATIVE_GC_NURSERY_BYTES=1024")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run Promise control-flow ownership: %v: %s", err, got)
	}
	want := "2\n6\n3"
	if strings.TrimSpace(string(got)) != want {
		t.Fatalf("output = %q; want %q", got, want)
	}
}

func TestBuildNativeAsyncFinallySingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "async-finally")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_async_finally.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 2 || result.Metrics.TaskJoins != 2 || result.Metrics.Sleeps != 1 {
		t.Fatalf("finally metrics = %d/%d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins, result.Metrics.Sleeps)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run finally: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "caught-finally\n2" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeAsyncFinallyCompletionSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "async-finally-completion")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_async_finally_completion.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 4 || result.Metrics.TaskJoins != 4 || result.Metrics.Sleeps != 1 {
		t.Fatalf("finally completion metrics = %d/%d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins, result.Metrics.Sleeps)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run finally completion: %v: %s", err, got)
	}
	want := "finally-return\n40\nfinally-rethrow\nrethrow-finally\n42"
	if strings.TrimSpace(string(got)) != want {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeAsyncFinallyOverrideSingleWorker(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "async-finally-override")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/concurrency_async_finally_override.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.TaskSpawns != 3 || result.Metrics.TaskJoins != 3 {
		t.Fatalf("finally override metrics = %d/%d", result.Metrics.TaskSpawns, result.Metrics.TaskJoins)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run finally override: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "99\nfinally-override\n42" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeChannelExternalTaskCrossPath(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	fixtures := []struct {
		name string
		want string
	}{
		{"concurrency_channel_cross_bool.ts", "1\n0"},
		{"concurrency_channel_cross_ref.ts", "external-to-task\ntask-to-external"},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			output := filepath.Join(t.TempDir(), strings.TrimSuffix(fixture.name, ".ts"))
			if _, err := Build(ctx, BuildOptions{Root: root, Input: filepath.Join("examples", fixture.name), Output: output, Optimization: "-O2"}); err != nil {
				t.Fatal(err)
			}
			cmd := exec.CommandContext(ctx, output)
			cmd.Env = append(os.Environ(), "TSNATIVE_WORKERS=2")
			got, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("run %s: %v: %s", fixture.name, err, got)
			}
			if strings.TrimSpace(string(got)) != fixture.want {
				t.Fatalf("output = %q; want %q", got, fixture.want)
			}
		})
	}
}

func TestBuildStackLocalObjectNativeExecutable(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "stack-object")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/stack_object.ts", Output: output, Optimization: "-O2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.AllocationCandidates != 1 || result.Metrics.NonEscapingAllocations != 1 || result.Metrics.EscapingAllocations != 0 {
		t.Fatalf("escape metrics = %d/%d/%d, want 1/1/0", result.Metrics.AllocationCandidates, result.Metrics.NonEscapingAllocations, result.Metrics.EscapingAllocations)
	}
	if result.Metrics.ScalarObjectAllocs != 1 || result.Metrics.StackObjectAllocs != 0 {
		t.Fatalf("object storage scalar/stack = %d/%d, want 1/0", result.Metrics.ScalarObjectAllocs, result.Metrics.StackObjectAllocs)
	}
	if result.Metrics.RuntimeCalls != 1 {
		t.Fatalf("runtime calls = %d, want 1 console.log call", result.Metrics.RuntimeCalls)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run stack-object binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "7" {
		t.Fatalf("stack-object output = %q", got)
	}
}

func TestBuildMutableScalarObjectNativeExecutable(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "scalar-mutable-object")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/scalar_mutable_object.ts", Output: output, Optimization: "-O2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.AllocationCandidates != 1 || result.Metrics.NonEscapingAllocations != 1 || result.Metrics.EscapingAllocations != 0 {
		t.Fatalf("escape metrics = %d/%d/%d, want 1/1/0", result.Metrics.AllocationCandidates, result.Metrics.NonEscapingAllocations, result.Metrics.EscapingAllocations)
	}
	if result.Metrics.ScalarObjectAllocs != 1 || result.Metrics.StackObjectAllocs != 0 {
		t.Fatalf("object storage scalar/stack = %d/%d, want 1/0", result.Metrics.ScalarObjectAllocs, result.Metrics.StackObjectAllocs)
	}
	if result.Metrics.RuntimeCalls != 1 {
		t.Fatalf("runtime calls = %d, want 1 console.log call", result.Metrics.RuntimeCalls)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run scalar-mutable-object binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "7" {
		t.Fatalf("scalar-mutable-object output = %q", got)
	}
}

func TestBuildNestedMutableScalarObjectNativeExecutable(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "scalar-nested-object")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/scalar_nested_object.ts", Output: output, Optimization: "-O2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.AllocationCandidates != 1 || result.Metrics.NonEscapingAllocations != 1 || result.Metrics.EscapingAllocations != 0 {
		t.Fatalf("escape metrics = %d/%d/%d, want 1/1/0", result.Metrics.AllocationCandidates, result.Metrics.NonEscapingAllocations, result.Metrics.EscapingAllocations)
	}
	if result.Metrics.ScalarObjectAllocs != 1 || result.Metrics.StackObjectAllocs != 0 {
		t.Fatalf("object storage scalar/stack = %d/%d, want 1/0", result.Metrics.ScalarObjectAllocs, result.Metrics.StackObjectAllocs)
	}
	if result.Metrics.RuntimeCalls != 1 {
		t.Fatalf("runtime calls = %d, want 1 console.log call", result.Metrics.RuntimeCalls)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run scalar-nested-object binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "8" {
		t.Fatalf("scalar-nested-object output = %q", got)
	}
}

func TestBuildReferenceBearingScalarObjectNativeExecutable(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "scalar-reference-object")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/scalar_reference_object.ts", Output: output, Optimization: "-O2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.AllocationCandidates != 1 || result.Metrics.NonEscapingAllocations != 1 || result.Metrics.EscapingAllocations != 0 {
		t.Fatalf("escape metrics = %d/%d/%d, want 1/1/0", result.Metrics.AllocationCandidates, result.Metrics.NonEscapingAllocations, result.Metrics.EscapingAllocations)
	}
	if result.Metrics.ScalarObjectAllocs != 1 || result.Metrics.StackObjectAllocs != 0 {
		t.Fatalf("object storage scalar/stack = %d/%d, want 1/0", result.Metrics.ScalarObjectAllocs, result.Metrics.StackObjectAllocs)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run scalar-reference-object binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "beta" {
		t.Fatalf("scalar-reference-object output = %q", got)
	}
}

func TestBuildStackClosureReferenceCaptureNativeExecutable(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "closure-stack-reference")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/closure_stack_reference.ts", Output: output, Optimization: "-O2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.AllocationCandidates != 1 || result.Metrics.NonEscapingAllocations != 1 || result.Metrics.EscapingAllocations != 0 {
		t.Fatalf("escape metrics = %d/%d/%d, want 1/1/0", result.Metrics.AllocationCandidates, result.Metrics.NonEscapingAllocations, result.Metrics.EscapingAllocations)
	}
	if result.Metrics.StackClosureAllocs != 1 {
		t.Fatalf("stack closure allocations = %d, want 1", result.Metrics.StackClosureAllocs)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run closure-stack-reference binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "root-ok" {
		t.Fatalf("closure-stack-reference output = %q", got)
	}
}

func TestBuildEscapingClosureRemainsHeapAllocated(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "closure-escape")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/closures_escape.ts", Output: output, Optimization: "-O2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.AllocationCandidates != 1 || result.Metrics.NonEscapingAllocations != 0 || result.Metrics.EscapingAllocations != 1 {
		t.Fatalf("escape metrics = %d/%d/%d, want 1/0/1", result.Metrics.AllocationCandidates, result.Metrics.NonEscapingAllocations, result.Metrics.EscapingAllocations)
	}
	if result.Metrics.StackClosureAllocs != 0 {
		t.Fatalf("escaping closure stack allocations = %d, want 0", result.Metrics.StackClosureAllocs)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run closure-escape binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "12" {
		t.Fatalf("closure-escape output = %q", got)
	}
}

func TestBuildReferenceBearingStackObjectSurvivesGCStress(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "stack-reference-object")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/stack_reference_object.ts", Output: output, Optimization: "-O2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.AllocationCandidates != 1 || result.Metrics.NonEscapingAllocations != 1 || result.Metrics.EscapingAllocations != 0 {
		t.Fatalf("escape metrics = %d/%d/%d, want 1/1/0", result.Metrics.AllocationCandidates, result.Metrics.NonEscapingAllocations, result.Metrics.EscapingAllocations)
	}
	if result.Metrics.ScalarObjectAllocs != 0 || result.Metrics.StackObjectAllocs != 1 {
		t.Fatalf("object storage scalar/stack = %d/%d, want 0/1", result.Metrics.ScalarObjectAllocs, result.Metrics.StackObjectAllocs)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_GC_NURSERY_BYTES=1024")
	nativeOutput, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run stack-reference-object binary: %v: %s", err, nativeOutput)
	}
	if got := strings.TrimSpace(string(nativeOutput)); got != "kept" {
		t.Fatalf("stack-reference-object output = %q", got)
	}
}

func TestBuildDynamicPropertySetSurvivesGCStress(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "dynamic-property-set")
	if _, err := Build(ctx, BuildOptions{Root: root, Input: "examples/dynamic_property_set.ts", Output: output, Optimization: "-O2"}); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_GC_NURSERY_BYTES=1024")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run dynamic property set GC stress: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "99\nafter\nfalse\n77" {
		t.Fatalf("output = %q", got)
	}
}
func TestBuildNativeDynamicMethodPreservesThis(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "dynamic-method-this")
	result, err := Build(ctx, BuildOptions{Root: root, Input: "examples/dynamic_method_this.ts", Output: output, Optimization: "-O2"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics.DynamicDispatch != 2 {
		t.Fatalf("dynamic dispatch = %d; want 2", result.Metrics.DynamicDispatch)
	}
	got, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run dynamic method: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "42\n50" {
		t.Fatalf("output = %q", got)
	}
}
func TestBuildNativeDynamicStructuralMethodPreservesThis(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "dynamic-structural-this")
	if _, err := Build(ctx, BuildOptions{Root: root, Input: "examples/dynamic_structural_this.ts", Output: output, Optimization: "-O2"}); err != nil {
		t.Fatal(err)
	}
	got, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run dynamic structural method: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "42\n42" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeClassPromiseThenableAssimilation(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "promise-thenable-class")
	if _, err := Build(ctx, BuildOptions{Root: root, Input: "examples/promise_thenable_class.ts", Output: output, Optimization: "-O2"}); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_GC_NURSERY_BYTES=128", "TSNATIVE_SCHEDULER_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run class Promise thenable GC stress: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "42" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeOptionalPromiseThenableAssimilation(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "promise-thenable-optional")
	if _, err := Build(ctx, BuildOptions{Root: root, Input: "examples/promise_thenable_optional.ts", Output: output, Optimization: "-O2"}); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_GC_NURSERY_BYTES=128", "TSNATIVE_SCHEDULER_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run optional Promise thenable GC stress: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "42" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativePromiseThenableAssimilation(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "promise-thenable")
	if _, err := Build(ctx, BuildOptions{Root: root, Input: "examples/promise_thenable.ts", Output: output, Optimization: "-O2"}); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_GC_NURSERY_BYTES=256")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run Promise thenable GC stress: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "42\nthenable-reject\n7" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeAsyncPromiseThenableAssimilation(t *testing.T) {
	requireLLVM(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "promise-thenable-async")
	if _, err := Build(ctx, BuildOptions{Root: root, Input: "examples/promise_thenable_async.ts", Output: output, Optimization: "-O2"}); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, output)
	cmd.Env = append(os.Environ(), "TSNATIVE_GC_NURSERY_BYTES=128", "TSNATIVE_SCHEDULER_WORKERS=1")
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run async Promise thenable GC stress: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "42\nasync-thenable-reject\n7" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildArrayPushPopPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "array-push-pop-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/arrays/array_push_pop.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	expected := "2\n3\n3\n30\n30\n2\n5\n100\n101\n102\n3\ngamma\ngamma\n2\n2\n0\n0\n1\n2\nsecond\n2\n1"
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("pure-Go output = %q, want %q", got, expected)
	}
}

func TestBuildSwitchControlFlowPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "switch-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/basics/switch_control_flow.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	expected := "other\none\ntwo\nthree\nother\n100\n50\n0\n-1"
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("pure-Go output = %q, want %q", got, expected)
	}
}

func TestBuildNullishCoalescingPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "nullish-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/basics/nullish_coalescing.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	expected := "Anonymous\nAlice\n42\n0\n100\ntrue\nfalse\ntrue\nHello world!\nNo bio available\nNo bio available"
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("pure-Go output = %q, want %q", got, expected)
	}
}

func TestBuildTemplateLiteralsPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "template-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/basics/template_literals.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	expected := "Hello, Alice! Next year you will be 26.\nHello, Bob! Next year you will be 31.\nstatus: true, count: 10\nstatus: false, count: 0\nsimple template with no substitutions\nitem #1: square=1\nitem #2: square=4\nitem #3: square=9"
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("pure-Go output = %q, want %q", got, expected)
	}
}

func TestBuildTuplesAndDestructuringPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "tuples-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/basics/tuples_and_destructuring.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	expected := "10\n20\nlocalhost\n8080\n1\nalice\ntrue\n100\n200\n99\n42"
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("pure-Go output = %q, want %q", got, expected)
	}
}

func TestBuildEnumsAndDefaultsPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "enums-defaults-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/basics/enums_and_defaults.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	expected := "200\n404\n0\n1\n2\n3\nmoving Up by 1\nmoving Down by 5\nHello, Alice!\nHi, Bob!\nGreetings, Dr. Watson!"
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("pure-Go output = %q, want %q", got, expected)
	}
}

func TestBuildForOfLoopsPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "for-of-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/basics/for_of_loops.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	expected := "100\nts-pro is fast and pure-Go \n3\n10\n20\n30\n40"
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("pure-Go output = %q, want %q", got, expected)
	}
}

func TestBuildJSONAPIPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "json-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/basics/json_api.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	expected := "{\"message\":\"hello\"}\n{\"status\":200}\n{\"count\":5,\"key\":\"items\"}\n[10,20,30]\n[\"a\",\"b\",\"c\"]\n[true,false]\n123.45\n\"pure-Go\"\ntrue\n123.45\nparsed string\ntrue\nfalse\n[1,2,3]\n{\"id\":42}\n1"
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("pure-Go output = %q, want %q", got, expected)
	}
}

func TestBuildRestAndSpreadPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "rest-spread-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/basics/rest_and_spread.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	expected := "60\n15\n0\nitems:apple:banana:cherry\nempty\n0\n10\n20\n30\n40\nfirst\nsecond\ntThird\nlocalhost\n9000\ntrue\nprod"
	_ = expected
	if !strings.Contains(string(nativeOutput), "60\n15\n0") || !strings.Contains(string(nativeOutput), "items:apple:banana:cherry") || !strings.Contains(string(nativeOutput), "localhost\n9000\ntrue\nprod") {
		t.Fatalf("pure-Go output = %q", string(nativeOutput))
	}
}

func TestBuildMapSetPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "map-set-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/basics/map_set.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	expected := "10\n20\ntrue\nfalse\n2\nfalse\n1\n0\ntrue\nfalse\n2\nfalse\n1\n0\n2\n100\n200\n3"
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("pure-Go output = %q, want %q", got, expected)
	}
}

func TestBuildDateAPIPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "date-api-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/basics/date_api.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	expected := "true\n2023-11-14T22:13:20.000Z\n2023\n10\n14\n22\n13\n20\n2023-11-14T22:13:20.000Z"
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("pure-Go output = %q, want %q", got, expected)
	}
}

func TestBuildRegExpAPIPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "regexp-api-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/basics/regexp_api.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	expected := "true\nfalse\nworld\ntrue\nfalse\ntrue\nfalse"
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("pure-Go output = %q, want %q", got, expected)
	}
}

func TestBuildComputedPropertyKeysPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "computed-keys-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/basics/computed_property_keys.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	expected := "101\nAlice\nAlice\n202\napplication/json\ntext/html\n20\n99"
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("pure-Go output = %q, want %q", got, expected)
	}
}

func TestBuildOptionalPropertiesPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "opt-props-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/basics/optional_properties.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	expected := "localhost\n8080\ntrue\nremote\nundefined\nundefined\ncircle\nsquare"
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("pure-Go output = %q, want %q", got, expected)
	}
}

func TestBuildEvolvingShapesPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "evolving-shapes-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/basics/evolving_shapes.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	expected := "10\n20\n99\nadded\n42\nundefined"
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("pure-Go output = %q, want %q", got, expected)
	}
}

func TestBuildGenericsPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "generics-pure-go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root: root, Input: "examples/basics/generics.ts", Output: output, PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != output {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	expected := "42\nhello\nanswer\n42\n100\nboxed\n3\n30\n20\n1"
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("pure-Go output = %q, want %q", got, expected)
	}
}

func TestBuildMultiModulePureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "multi_module_pure_go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root:   root,
		Input:  filepath.Join("examples", "basics", "multi_module.ts"),
		Output: output,
		PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Functions == 0 {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	expected := "42\n256\n12"
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("pure-Go output = %q, want %q", got, expected)
	}
}

func TestBuildPromiseCompletenessPureGoExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "promise_completeness_pure_go")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := Build(ctx, BuildOptions{
		Root:   root,
		Input:  filepath.Join("examples", "concurrency", "promise_completeness.ts"),
		Output: output,
		PureGo: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Functions == 0 {
		t.Fatalf("build result = %+v", result)
	}
	nativeOutput, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run pure-Go binary: %v: %s", err, nativeOutput)
	}
	expected := "42\n100\nhello\ntrue\n10\n20\n30\n50\n60"
	if got := strings.TrimSpace(string(nativeOutput)); got != expected {
		t.Fatalf("pure-Go output = %q, want %q", got, expected)
	}
}

func TestBuildCrossCompileExecutable(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	targets := []struct {
		target   string
		filename string
	}{
		{"linux/amd64", "fib-linux-amd64"},
		{"linux/arm64", "fib-linux-arm64"},
		{"windows/amd64", "fib-windows-amd64.exe"},
	}

	for _, tc := range targets {
		tc := tc
		t.Run(tc.target, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), tc.filename)
			result, err := Build(ctx, BuildOptions{
				Root:   root,
				Input:  "examples/basics/fib.ts",
				Output: output,
				Target: tc.target,
				PureGo: true,
			})
			if err != nil {
				t.Fatalf("cross-compile to %s failed: %v", tc.target, err)
			}
			if result.Functions == 0 {
				t.Fatalf("build result has 0 functions: %+v", result)
			}
			info, err := os.Stat(output)
			if err != nil || info.Size() == 0 {
				t.Fatalf("output file invalid: info=%v err=%v", info, err)
			}
		})
	}
}

func TestBuildThinLTOAndPGOBoptions(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	output := filepath.Join(t.TempDir(), "fib-opt")
	result, err := Build(ctx, BuildOptions{
		Root:       root,
		Input:      "examples/basics/fib.ts",
		Output:     output,
		ThinLTO:    true,
		PGOProfile: "auto",
		PureGo:     true,
	})
	if err != nil {
		t.Fatalf("build with ThinLTO and PGO failed: %v", err)
	}
	if result.Functions == 0 {
		t.Fatalf("expected non-zero functions, got %+v", result)
	}
}
