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

func TestBuildNativeTaskIntrinsicsExecutable(t *testing.T) {
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if result.Metrics.BoxingSites != 4 {
		t.Fatalf("boxing sites = %d; want 4", result.Metrics.BoxingSites)
	}
	got, err := exec.CommandContext(ctx, output).CombinedOutput()
	if err != nil {
		t.Fatalf("run dynamic checked unboxing: %v: %s", err, got)
	}
	if strings.TrimSpace(string(got)) != "42\nunboxed\n42\n42" {
		t.Fatalf("output = %q", got)
	}
}

func TestBuildNativeReferenceChannelSingleWorker(t *testing.T) {
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	want := "345\n7\naggregate-reject\n42\n9\n30\n30\nrace-reject\n43"
	if strings.TrimSpace(string(got)) != want {
		t.Fatalf("output = %q; want %q", got, want)
	}
}

func TestBuildNativeRepeatedPromiseAwaitSingleWorker(t *testing.T) {
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not installed")
	}
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
