package llvm_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	repranalysis "github.com/projectthorn/tsv7-bin/internal/analysis/repr"
	llvmcodegen "github.com/projectthorn/tsv7-bin/internal/codegen/llvm"
	"github.com/projectthorn/tsv7-bin/internal/frontend"
	"github.com/projectthorn/tsv7-bin/internal/lowering"
	"github.com/projectthorn/tsv7-bin/internal/mir"
	"github.com/projectthorn/tsv7-bin/internal/toolchain"
	"github.com/projectthorn/tsv7-bin/internal/tsls"
)

func TestEmitFibLLVMAndCompileObject(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := tsls.StartAPI("../../..")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	if _, err := client.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	snapshot, err := client.UpdateSnapshot(ctx, "tsconfig.json")
	if err != nil {
		t.Fatal(err)
	}
	project := snapshot.Projects[0]
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	semantic, err := frontend.ExtractFile(ctx, client, snapshot.Snapshot, project.ID, filepath.Join(root, "examples", "fib.ts"))
	if err != nil {
		t.Fatal(err)
	}
	hirModule, err := lowering.LowerHIR(semantic, "fib")
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics := repranalysis.Analyze(&hirModule); len(diagnostics) != 0 {
		t.Fatalf("representation diagnostics = %+v", diagnostics)
	}
	mirModule, err := lowering.LowerMIR(hirModule)
	if err != nil {
		t.Fatal(err)
	}
	text, err := llvmcodegen.Emit(mirModule)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"declare void @tsnative_console_log_f64(double)",
		"define double @tsnative_f0(double %v0)",
		"fcmp ole double",
		"call double @tsnative_f0",
		"fadd double",
		"call void @tsnative_console_log_f64",
		"define i32 @main()",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLVM IR missing %q:\n%s", want, text)
		}
	}

	tc, err := toolchain.DiscoverClang()
	if err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	ll := filepath.Join(dir, "fib.ll")
	obj := filepath.Join(dir, "fib.o")
	runtimeObj := filepath.Join(dir, "runtime.o")
	heapObj := filepath.Join(dir, "heap.o")
	schedulerObj := filepath.Join(dir, "scheduler.o")
	bin := filepath.Join(dir, "fib")
	if err := os.WriteFile(ll, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := tc.CompileLLVM(ctx, ll, obj, "-O2"); err != nil {
		t.Fatalf("compile LLVM: %v\nIR:\n%s", err, text)
	}
	if err := tc.CompileC(ctx, filepath.Join(root, "runtime", "core", "console.c"), runtimeObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := tc.CompileC(ctx, filepath.Join(root, "runtime", "core", "heap.c"), heapObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := tc.CompileC(ctx, filepath.Join(root, "runtime", "concurrency", "scheduler.c"), schedulerObj, "-O2"); err != nil {
		t.Fatal(err)
	}
	if err := tc.Link(ctx, []string{obj, runtimeObj, heapObj, schedulerObj}, bin); err != nil {
		t.Fatal(err)
	}
	output, err := exec.CommandContext(ctx, bin).CombinedOutput()
	if err != nil {
		t.Fatalf("run native fib: %v: %s", err, output)
	}
	if got := strings.TrimSpace(string(output)); got != "6765" {
		t.Fatalf("native output = %q", got)
	}

}

func TestEmitClosedObjectUsesFixedShapeOffsets(t *testing.T) {
	ret := mir.ValueID(4)
	module := mir.Module{
		Name: "object",
		Shapes: []mir.Shape{{ID: 0, Name: "Point", Fields: []mir.ShapeField{
			{Name: "x", Repr: mir.ReprF64}, {Name: "y", Repr: mir.ReprF64},
		}}},
		Functions: []mir.Function{{ID: 0, Name: "pointX", ReturnRepr: mir.ReprF64, Entry: 0,
			Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
				{Result: 0, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 3}},
				{Result: 1, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 4}},
				{Result: 2, Repr: mir.ReprObjectRef, Op: mir.ObjectNew{Shape: 0, Fields: []mir.ValueID{0, 1}}},
				{Result: 4, Repr: mir.ReprF64, Op: mir.FieldGet{Object: 2, Shape: 0, Field: 0}},
			}, Terminator: mir.Return{Value: &ret}}},
		}},
	}
	text, err := llvmcodegen.Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"%tsnative_shape_s0 = type { double, double }", "@tsnative_object_alloc", "getelementptr %tsnative_shape_s0", "load double"} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLVM IR missing %q:\n%s", want, text)
		}
	}
}

func TestEmitProvenIntegerFastArithmetic(t *testing.T) {
	result := mir.ValueID(2)
	module := mir.Module{Name: "intfast", Functions: []mir.Function{{
		ID: 0, Name: "intfast", ReturnRepr: mir.ReprF64, Entry: 0,
		Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			{Result: 0, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 20}},
			{Result: 1, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 22}},
			{Result: 2, Repr: mir.ReprF64, Op: mir.ProvenIntBinary{Width: mir.IntWidth32, Operator: mir.FloatAdd, Left: 0, Right: 1}},
		}, Terminator: mir.Return{Value: &result}}},
	}}}
	text, err := llvmcodegen.Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"fptosi double", "add i32", "sitofp i32"} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLVM IR missing %q:\n%s", want, text)
		}
	}
}

func TestEmitNativeTaskIntrinsics(t *testing.T) {
	module := mir.Module{Name: "tasks", Functions: []mir.Function{
		{ID: 0, Name: "worker", ReturnRepr: mir.ReprVoid, Entry: 0,
			Blocks: []mir.Block{{ID: 0, Terminator: mir.Return{}}}},
		{ID: 1, Name: "entry", ReturnRepr: mir.ReprVoid, Entry: 0,
			Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
				{Result: 0, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 0}},
				{Result: 1, Repr: mir.ReprVoid, Op: mir.TaskJoin{Task: 0}},
				{Result: 2, Repr: mir.ReprVoid, Op: mir.TaskYield{}},
			}, Terminator: mir.Return{}}},
		},
	}}
	entry := mir.FunctionID(1)
	module.Entry = &entry
	text, err := llvmcodegen.Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"define void @tsnative_task_entry_f0(ptr %state, ptr %result_slot)",
		"call ptr @tsnative_task_spawn_or_abort(ptr @tsnative_task_entry_f0, ptr null)",
		"call void @tsnative_task_join_release(ptr %v0)",
		"call void @tsnative_task_yield()",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLVM IR missing %q:\n%s", want, text)
		}
	}
}

func TestEmitCapturedTaskState(t *testing.T) {
	module := mir.Module{Name: "captured-task", Functions: []mir.Function{
		{ID: 0, Name: "worker", Params: []mir.Param{{Value: 0, Name: "message", Repr: mir.ReprStringRef}}, ReturnRepr: mir.ReprVoid, Entry: 0,
			Blocks: []mir.Block{{ID: 0, Terminator: mir.Return{}}}},
		{ID: 1, Name: "entry", ReturnRepr: mir.ReprVoid, Entry: 0,
			Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
				{Result: 0, Repr: mir.ReprStringRef, Op: mir.ConstString{Value: "captured"}},
				{Result: 1, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 0, Captures: []mir.ValueID{0}}},
				{Result: 2, Repr: mir.ReprVoid, Op: mir.TaskJoin{Task: 1}},
			}, Terminator: mir.Return{}}},
		},
	}}
	entry := mir.FunctionID(1)
	module.Entry = &entry
	text, err := llvmcodegen.Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"%tsnative_task_env_f0 = type { ptr }",
		"call ptr @tsnative_object_alloc",
		"store ptr %v0",
		"call ptr @tsnative_task_spawn_or_abort(ptr @tsnative_task_entry_f0, ptr %v1.state)",
		"load ptr, ptr %capture0.ptr",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLVM IR missing %q:\n%s", want, text)
		}
	}
}

func TestEmitF64TaskResult(t *testing.T) {
	result := mir.ValueID(0)
	module := mir.Module{Name: "task-f64", Functions: []mir.Function{
		{ID: 0, Name: "worker", ReturnRepr: mir.ReprF64, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{{Result: 0, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 42}}}, Terminator: mir.Return{Value: &result}}}},
		{ID: 1, Name: "entry", ReturnRepr: mir.ReprVoid, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			{Result: 0, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 0}},
			{Result: 1, Repr: mir.ReprF64, Op: mir.TaskJoin{Task: 0}},
		}, Terminator: mir.Return{}}}},
	}}
	entry := mir.FunctionID(1)
	module.Entry = &entry
	text, err := llvmcodegen.Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"call ptr @tsnative_task_spawn_f64_or_abort",
		"store double %result, ptr %result_slot",
		"call double @tsnative_task_join_f64_release",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLVM IR missing %q:\n%s", want, text)
		}
	}
}

func TestEmitStringTaskResult(t *testing.T) {
	result := mir.ValueID(0)
	module := mir.Module{Name: "task-string", Functions: []mir.Function{
		{ID: 0, Name: "worker", ReturnRepr: mir.ReprStringRef, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{{Result: 0, Repr: mir.ReprStringRef, Op: mir.ConstString{Value: "task-string"}}}, Terminator: mir.Return{Value: &result}}}},
		{ID: 1, Name: "entry", ReturnRepr: mir.ReprVoid, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			{Result: 0, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 0}},
			{Result: 1, Repr: mir.ReprStringRef, Op: mir.TaskJoin{Task: 0}},
		}, Terminator: mir.Return{}}}},
	}}
	entry := mir.FunctionID(1)
	module.Entry = &entry
	text, err := llvmcodegen.Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"call ptr @tsnative_task_spawn_ref_or_abort",
		"store ptr %result, ptr %result_slot",
		"call ptr @tsnative_task_join_ref_release",
		"call void @tsnative_gc_handoff_begin()",
		"call void @tsnative_gc_handoff_end()",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLVM IR missing %q:\n%s", want, text)
		}
	}
}

func TestEmitBoolTaskResult(t *testing.T) {
	result := mir.ValueID(0)
	module := mir.Module{Name: "task-bool", Functions: []mir.Function{
		{ID: 0, Name: "worker", ReturnRepr: mir.ReprBool, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{{Result: 0, Repr: mir.ReprBool, Op: mir.ConstBool{Value: true}}}, Terminator: mir.Return{Value: &result}}}},
		{ID: 1, Name: "entry", ReturnRepr: mir.ReprVoid, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			{Result: 0, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 0}},
			{Result: 1, Repr: mir.ReprBool, Op: mir.TaskJoin{Task: 0}},
		}, Terminator: mir.Return{}}}},
	}}
	entry := mir.FunctionID(1)
	module.Entry = &entry
	text, err := llvmcodegen.Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"@tsnative_task_spawn_bool_or_abort", "store i1 %result, ptr %result_slot", "@tsnative_task_join_bool_release", "trunc i8"} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLVM IR missing %q:\n%s", want, text)
		}
	}
}

func TestEmitNativeF64ChannelTryOps(t *testing.T) {
	module := mir.Module{Name: "channel-try", Functions: []mir.Function{{ID: 0, Name: "entry", ReturnRepr: mir.ReprVoid, Entry: 0,
		Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			{Result: 0, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 1}},
			{Result: 1, Repr: mir.ReprChannelRef, Op: mir.ChannelNewF64{Capacity: 0}},
			{Result: 2, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 42}},
			{Result: 3, Repr: mir.ReprBool, Op: mir.ChannelTrySendF64{Channel: 1, Value: 2}},
			{Result: 4, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 7}},
			{Result: 5, Repr: mir.ReprF64, Op: mir.ChannelTryRecvOrF64{Channel: 1, Fallback: 4}},
		}, Terminator: mir.Return{}}},
	}}}
	entry := mir.FunctionID(0)
	module.Entry = &entry
	text, err := llvmcodegen.Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"call ptr @tsnative_channel_f64_new_checked(double",
		"call i32 @tsnative_channel_f64_try_send(ptr",
		"icmp eq i32",
		"call double @tsnative_channel_f64_try_recv_or(ptr",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLVM IR missing %q:\n%s", want, text)
		}
	}
}

func TestEmitNativeF64ChannelBlockingOps(t *testing.T) {
	module := mir.Module{Name: "channel-blocking", Functions: []mir.Function{{ID: 0, Name: "entry", ReturnRepr: mir.ReprVoid, Entry: 0,
		Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			{Result: 0, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 1}},
			{Result: 1, Repr: mir.ReprChannelRef, Op: mir.ChannelNewF64{Capacity: 0}},
			{Result: 2, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 42}},
			{Result: 3, Repr: mir.ReprVoid, Op: mir.ChannelSendF64{Channel: 1, Value: 2}},
			{Result: 4, Repr: mir.ReprF64, Op: mir.ChannelRecvF64{Channel: 1}},
		}, Terminator: mir.Return{}}},
	}}}
	entry := mir.FunctionID(0)
	module.Entry = &entry
	text, err := llvmcodegen.Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"call void @tsnative_channel_f64_send_cooperative(ptr",
		"call double @tsnative_channel_f64_recv_cooperative(ptr",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLVM IR missing %q:\n%s", want, text)
		}
	}
}

func TestEmitStacklessChannelTaskContinuation(t *testing.T) {
	module := mir.Module{Name: "stackless-channel", Functions: []mir.Function{
		{ID: 0, Name: "sender", Params: []mir.Param{{Value: 0, Name: "ch", Repr: mir.ReprChannelRef}}, ReturnRepr: mir.ReprVoid, Entry: 0,
			Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
				{Result: 1, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 42}},
				{Result: 2, Repr: mir.ReprVoid, Op: mir.ChannelSendF64{Channel: 0, Value: 1}},
			}, Terminator: mir.Return{}}}},
		{ID: 1, Name: "receiver", Params: []mir.Param{{Value: 0, Name: "ch", Repr: mir.ReprChannelRef}}, ReturnRepr: mir.ReprF64, Entry: 0,
			Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
				{Result: 1, Repr: mir.ReprF64, Op: mir.ChannelRecvF64{Channel: 0}},
			}, Terminator: mir.Return{Value: valueIDPtr(1)}}}},
		{ID: 2, Name: "launcher", Params: []mir.Param{{Value: 0, Name: "ch", Repr: mir.ReprChannelRef}}, ReturnRepr: mir.ReprVoid, Entry: 0,
			Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
				{Result: 1, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 0, Captures: []mir.ValueID{0}}},
				{Result: 2, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 1, Captures: []mir.ValueID{0}}},
			}, Terminator: mir.Return{}}}},
	}}
	text, err := llvmcodegen.Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"%tsnative_task_env_f0 = type { ptr, i32 }",
		"%tsnative_task_env_f1 = type { ptr, i32, double }",
		"call i32 @tsnative_channel_f64_send_task(ptr %capture0, double 4.200000e+01)",
		"call i32 @tsnative_channel_f64_recv_task(ptr %capture0, ptr %recv.ptr)",
		"switch i32 %pc",
		"store i32 1, ptr %pc.ptr",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLVM IR missing %q:\n%s", want, text)
		}
	}
}

func valueIDPtr(value mir.ValueID) *mir.ValueID { return &value }
