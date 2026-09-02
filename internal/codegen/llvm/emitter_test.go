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
	bin := filepath.Join(dir, "fib")
	if err := os.WriteFile(ll, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := tc.CompileLLVM(ctx, ll, obj, "-O2"); err != nil {
		t.Fatalf("compile LLVM: %v\nIR:\n%s", err, text)
	}
	cache, err := toolchain.NewObjectCache(root, tc)
	if err != nil {
		t.Fatal(err)
	}
	goRuntime, _, err := cache.BuildGoArchive(ctx, root, "./runtimego")
	if err != nil {
		t.Fatal(err)
	}
	if err := tc.Link(ctx, []string{obj, goRuntime}, bin); err != nil {
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
	for _, want := range []string{"%tsnative_shape_s0 = type { double, double }", "ret double 3.000000e+00"} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLVM IR missing %q:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{"%v2 = alloca %tsnative_shape_s0", "call ptr @tsnative_object_alloc_atomic(i64 %v2.size)", "%v4 = load double"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("scalar-replaced object retained %q:\n%s", forbidden, text)
		}
	}
}

func TestEmitMutableNumericObjectUsesStackStorage(t *testing.T) {
	ret := mir.ValueID(4)
	module := mir.Module{
		Name:   "mutable-stack-object",
		Shapes: []mir.Shape{{ID: 0, Name: "Counter", Fields: []mir.ShapeField{{Name: "value", Repr: mir.ReprF64}}}},
		Functions: []mir.Function{{ID: 0, Name: "counter", ReturnRepr: mir.ReprF64, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			{Result: 0, Repr: mir.ReprObjectRef, Op: mir.ObjectAlloc{Shape: 0}},
			{Result: 1, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 7}},
			{Result: 2, Repr: mir.ReprF64, Op: mir.FieldSet{Object: 0, Shape: 0, Field: 0, Value: 1}},
			{Result: 4, Repr: mir.ReprF64, Op: mir.FieldGet{Object: 0, Shape: 0, Field: 0}},
		}, Terminator: mir.Return{Value: &ret}}}}},
	}
	text, err := llvmcodegen.Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "%v0 = alloca %tsnative_shape_s0") {
		t.Fatalf("mutable local object did not use stack storage:\n%s", text)
	}
	if strings.Contains(text, "call ptr @tsnative_object_alloc_atomic(i64 %v0.size)") {
		t.Fatalf("mutable local object unexpectedly used heap allocation:\n%s", text)
	}
}

func TestEmitReturnedNumericObjectRemainsHeapAllocated(t *testing.T) {
	ret := mir.ValueID(1)
	module := mir.Module{
		Name:   "escaping-object",
		Shapes: []mir.Shape{{ID: 0, Name: "Point", Fields: []mir.ShapeField{{Name: "x", Repr: mir.ReprF64}}}},
		Functions: []mir.Function{{ID: 0, Name: "makePoint", ReturnRepr: mir.ReprObjectRef, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			{Result: 0, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 3}},
			{Result: 1, Repr: mir.ReprObjectRef, Op: mir.ObjectNew{Shape: 0, Fields: []mir.ValueID{0}}},
		}, Terminator: mir.Return{Value: &ret}}}}},
	}
	text, err := llvmcodegen.Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "call ptr @tsnative_object_alloc_atomic(i64 %v1.size)") {
		t.Fatalf("returned object did not remain heap allocated:\n%s", text)
	}
	if strings.Contains(text, "%v1 = alloca %tsnative_shape_s0") {
		t.Fatalf("returned object was stack allocated:\n%s", text)
	}
}

func TestEmitLoopNumericObjectRemainsHeapAllocated(t *testing.T) {
	module := mir.Module{
		Name:   "loop-object",
		Shapes: []mir.Shape{{ID: 0, Name: "Point", Fields: []mir.ShapeField{{Name: "x", Repr: mir.ReprF64}}}},
		Functions: []mir.Function{{ID: 0, Name: "loop", ReturnRepr: mir.ReprVoid, Entry: 0, Blocks: []mir.Block{
			{ID: 0, Terminator: mir.Jump{Target: 1}},
			{ID: 1, Instructions: []mir.Instruction{{Result: 0, Repr: mir.ReprObjectRef, Op: mir.ObjectAlloc{Shape: 0}}}, Terminator: mir.Jump{Target: 1}},
		}}},
	}
	text, err := llvmcodegen.Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "call ptr @tsnative_object_alloc_atomic(i64 %v0.size)") {
		t.Fatalf("loop object did not remain heap allocated:\n%s", text)
	}
}

func TestEmitReferenceFieldStoreUsesGCBarrier(t *testing.T) {
	result := mir.ValueID(2)
	module := mir.Module{
		Name:   "object-ref-store",
		Shapes: []mir.Shape{{ID: 0, Name: "Holder", Fields: []mir.ShapeField{{Name: "value", Repr: mir.ReprStringRef}}}},
		Functions: []mir.Function{{ID: 0, Name: "set", ReturnRepr: mir.ReprStringRef, Entry: 0,
			Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
				{Result: 0, Repr: mir.ReprObjectRef, Op: mir.ObjectAlloc{Shape: 0}},
				{Result: 1, Repr: mir.ReprStringRef, Op: mir.ConstString{Value: "value"}},
				{Result: 2, Repr: mir.ReprStringRef, Op: mir.FieldSet{Object: 0, Shape: 0, Field: 0, Value: 1}},
			}, Terminator: mir.Return{Value: &result}}},
		}},
	}
	text, err := llvmcodegen.Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "call void @tsnative_gc_store_ref(ptr %v0, ptr %v2.ptr, ptr %v1)") {
		t.Fatalf("reference field store missing GC write barrier:\n%s", text)
	}
	if strings.Contains(text, "store ptr %v1, ptr %v2.ptr") {
		t.Fatalf("reference field store bypassed GC write barrier:\n%s", text)
	}
	for _, want := range []string{
		"@tsnative_refs_shape_s0 = private constant [1 x i64]",
		"call ptr @tsnative_object_alloc_refs(i64 %v0.size, ptr @tsnative_refs_shape_s0, i64 1)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("reference shape LLVM IR missing %q:\n%s", want, text)
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

func TestEmitClosureHeapReferenceLayouts(t *testing.T) {
	captured := mir.ValueID(0)
	module := mir.Module{Name: "closure-layout", Functions: []mir.Function{
		{ID: 0, Name: "target", Params: []mir.Param{{Value: 0, Name: "value", Repr: mir.ReprStringRef}}, ReturnRepr: mir.ReprStringRef, Entry: 0,
			Blocks: []mir.Block{{ID: 0, Terminator: mir.Return{Value: &captured}}}},
		{ID: 1, Name: "entry", ReturnRepr: mir.ReprVoid, Entry: 0,
			Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
				{Result: 0, Repr: mir.ReprStringRef, Op: mir.ConstString{Value: "captured"}},
				{Result: 1, Repr: mir.ReprFunctionRef, Op: mir.ClosureNew{Callee: 0, Captures: []mir.ValueID{0}}},
			}, Terminator: mir.Return{}}}},
	}}
	entry := mir.FunctionID(1)
	module.Entry = &entry
	text, err := llvmcodegen.Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"%tsnative_env_f0 = type { ptr }",
		"@tsnative_refs_env_f0 = private constant [1 x i64]",
		"@tsnative_refs_closure = private constant [1 x i64]",
		"call ptr @tsnative_object_alloc_refs(i64 %v1.env.size, ptr @tsnative_refs_env_f0, i64 1)",
		"call ptr @tsnative_object_alloc_refs(i64 %v1.size, ptr @tsnative_refs_closure, i64 1)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("closure LLVM IR missing %q:\n%s", want, text)
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
		"@tsnative_refs_task_f0 = private constant [1 x i64]",
		"call ptr @tsnative_object_alloc_refs(i64 %v1.state.size, ptr @tsnative_refs_task_f0, i64 1)",
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
		"call i32 @tsnative_channel_f64_recv_task(ptr %capture0, ptr %spill0.ptr)",
		"switch i32 %pc",
		"store i32 1, ptr %pc.ptr",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLVM IR missing %q:\n%s", want, text)
		}
	}
}

func valueIDPtr(value mir.ValueID) *mir.ValueID { return &value }

func TestEmitStacklessSleepTaskContinuation(t *testing.T) {
	module := mir.Module{Name: "stackless-sleep", Functions: []mir.Function{
		{ID: 0, Name: "sleeper", ReturnRepr: mir.ReprVoid, Entry: 0,
			Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
				{Result: 0, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 20}},
				{Result: 1, Repr: mir.ReprVoid, Op: mir.Sleep{Duration: 0}},
			}, Terminator: mir.Return{}}}},
		{ID: 1, Name: "launcher", ReturnRepr: mir.ReprVoid, Entry: 0,
			Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
				{Result: 0, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 0}},
			}, Terminator: mir.Return{}}}},
	}}
	text, err := llvmcodegen.Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"%tsnative_task_env_f0 = type { i32 }",
		"call i32 @tsnative_sleep_task(double 2.000000e+01)",
		"switch i32 %pc",
		"store i32 1, ptr %pc.ptr",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLVM IR missing %q:\n%s", want, text)
		}
	}
}

func TestEmitMultiSuspendTaskContinuation(t *testing.T) {
	module := mir.Module{Name: "multi-suspend", Functions: []mir.Function{
		{ID: 0, Name: "receiver", Params: []mir.Param{{Value: 0, Name: "ch", Repr: mir.ReprChannelRef}}, ReturnRepr: mir.ReprF64, Entry: 0,
			Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
				{Result: 1, Repr: mir.ReprF64, Op: mir.ChannelRecvF64{Channel: 0}},
				{Result: 2, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 1}},
				{Result: 3, Repr: mir.ReprVoid, Op: mir.Sleep{Duration: 2}},
			}, Terminator: mir.Return{Value: valueIDPtr(1)}}}},
		{ID: 1, Name: "launcher", Params: []mir.Param{{Value: 0, Name: "ch", Repr: mir.ReprChannelRef}}, ReturnRepr: mir.ReprVoid, Entry: 0,
			Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
				{Result: 1, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 0, Captures: []mir.ValueID{0}}},
			}, Terminator: mir.Return{}}}},
	}}
	text, err := llvmcodegen.Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"%tsnative_task_env_f0 = type { ptr, i32, double }",
		"i32 0, label %step0 i32 1, label %step1 i32 2, label %step2",
		"call i32 @tsnative_channel_f64_recv_task(ptr %capture0, ptr %spill0.ptr)",
		"call i32 @tsnative_sleep_task(double 1.000000e+00)",
		"%spill.0.ret2 = load double, ptr %spill0.ptr",
		"store i32 1, ptr %pc.ptr",
		"store i32 2, ptr %pc.ptr",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLVM IR missing %q:\n%s", want, text)
		}
	}
}

func TestEmitStacklessF64AwaitContinuation(t *testing.T) {
	module := mir.Module{Name: "stackless-await", Functions: []mir.Function{
		{ID: 0, Name: "child", ReturnRepr: mir.ReprF64, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			{Result: 0, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 42}},
		}, Terminator: mir.Return{Value: valueIDPtr(0)}}}},
		{ID: 1, Name: "parent", ReturnRepr: mir.ReprF64, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			{Result: 0, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 0}},
			{Result: 1, Repr: mir.ReprF64, Op: mir.TaskJoin{Task: 0}},
		}, Terminator: mir.Return{Value: valueIDPtr(1)}}}},
		{ID: 2, Name: "launcher", ReturnRepr: mir.ReprVoid, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			{Result: 0, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 1}},
		}, Terminator: mir.Return{}}}},
	}}
	text, err := llvmcodegen.Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"%tsnative_task_env_f1 = type { i32, ptr, double }",
		"call i32 @tsnative_task_await_f64_task(ptr",
		"store i32 2, ptr %pc.ptr",
		"store double %spill.1.ret2, ptr %result_slot",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLVM IR missing %q:\n%s", want, text)
		}
	}
}

func TestEmitStacklessTypedAwaitContinuations(t *testing.T) {
	boolModule := mir.Module{Name: "stackless-await-bool", Functions: []mir.Function{
		{ID: 0, Name: "childBool", ReturnRepr: mir.ReprBool, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{{Result: 0, Repr: mir.ReprBool, Op: mir.ConstBool{Value: true}}}, Terminator: mir.Return{Value: valueIDPtr(0)}}}},
		{ID: 1, Name: "parentBool", ReturnRepr: mir.ReprBool, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{{Result: 0, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 0}}, {Result: 1, Repr: mir.ReprBool, Op: mir.TaskJoin{Task: 0}}}, Terminator: mir.Return{Value: valueIDPtr(1)}}}},
		{ID: 2, Name: "launchBool", ReturnRepr: mir.ReprVoid, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{{Result: 0, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 1}}}, Terminator: mir.Return{}}}},
	}}
	boolIR, err := llvmcodegen.Emit(boolModule)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"%tsnative_task_env_f1 = type { i32, ptr, i1 }", "call i32 @tsnative_task_await_bool_task(ptr", "store i1 %spill.1.ret2, ptr %result_slot"} {
		if !strings.Contains(boolIR, want) {
			t.Fatalf("bool LLVM IR missing %q:\n%s", want, boolIR)
		}
	}

	refModule := mir.Module{Name: "stackless-await-ref", Functions: []mir.Function{
		{ID: 0, Name: "childRef", Params: []mir.Param{{Value: 0, Name: "value", Repr: mir.ReprStringRef}}, ReturnRepr: mir.ReprStringRef, Entry: 0, Blocks: []mir.Block{{ID: 0, Terminator: mir.Return{Value: valueIDPtr(0)}}}},
		{ID: 1, Name: "parentRef", Params: []mir.Param{{Value: 0, Name: "value", Repr: mir.ReprStringRef}}, ReturnRepr: mir.ReprStringRef, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{{Result: 1, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 0, Captures: []mir.ValueID{0}}}, {Result: 2, Repr: mir.ReprStringRef, Op: mir.TaskJoin{Task: 1}}}, Terminator: mir.Return{Value: valueIDPtr(2)}}}},
		{ID: 2, Name: "launchRef", Params: []mir.Param{{Value: 0, Name: "value", Repr: mir.ReprStringRef}}, ReturnRepr: mir.ReprVoid, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{{Result: 1, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 1, Captures: []mir.ValueID{0}}}}, Terminator: mir.Return{}}}},
	}}
	refIR, err := llvmcodegen.Emit(refModule)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"%tsnative_task_env_f1 = type { ptr, i32, ptr, ptr }", "call i32 @tsnative_task_await_ref_task(ptr", "store ptr %spill.1.ret2, ptr %result_slot"} {
		if !strings.Contains(refIR, want) {
			t.Fatalf("ref LLVM IR missing %q:\n%s", want, refIR)
		}
	}
}

func TestEmitStacklessLinearSSAContinuation(t *testing.T) {
	module := mir.Module{Name: "stackless-ssa", Functions: []mir.Function{
		{ID: 0, Name: "child", ReturnRepr: mir.ReprF64, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{{Result: 0, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 40}}}, Terminator: mir.Return{Value: valueIDPtr(0)}}}},
		{ID: 1, Name: "parent", ReturnRepr: mir.ReprBool, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			{Result: 0, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 0}},
			{Result: 1, Repr: mir.ReprF64, Op: mir.TaskJoin{Task: 0}},
			{Result: 2, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 2}},
			{Result: 3, Repr: mir.ReprF64, Op: mir.FloatBinary{Operator: mir.FloatMul, Left: 1, Right: 2}},
			{Result: 4, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 1}},
			{Result: 5, Repr: mir.ReprVoid, Op: mir.Sleep{Duration: 4}},
			{Result: 6, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 41}},
			{Result: 7, Repr: mir.ReprBool, Op: mir.FloatCompare{Operator: mir.FloatGreaterThan, Left: 3, Right: 6}},
		}, Terminator: mir.Return{Value: valueIDPtr(7)}}}},
		{ID: 2, Name: "launcher", ReturnRepr: mir.ReprVoid, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{{Result: 0, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 1}}}, Terminator: mir.Return{}}}},
	}}
	text, err := llvmcodegen.Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"%tsnative_task_env_f1 = type { i32, ptr, double, double, i1 }",
		"fmul double",
		"store double %v3, ptr %spill2.ptr",
		"fcmp ogt double",
		"store i1 %v7, ptr %spill3.ptr",
		"store i1 %spill.3.ret5, ptr %result_slot",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLVM IR missing %q:\n%s", want, text)
		}
	}
}

func TestEmitStacklessReferenceAndJSValueLinearContinuation(t *testing.T) {
	module := mir.Module{Name: "stackless-ref-js", Functions: []mir.Function{
		{ID: 0, Name: "stringWorker", ReturnRepr: mir.ReprStringRef, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			{Result: 0, Repr: mir.ReprStringRef, Op: mir.ConstString{Value: "pre:"}},
			{Result: 1, Repr: mir.ReprStringRef, Op: mir.ConstString{Value: "value"}},
			{Result: 2, Repr: mir.ReprStringRef, Op: mir.StringConcat{Left: 0, Right: 1}},
			{Result: 3, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 1}},
			{Result: 4, Repr: mir.ReprVoid, Op: mir.Sleep{Duration: 3}},
		}, Terminator: mir.Return{Value: valueIDPtr(2)}}}},
		{ID: 1, Name: "dynamicWorker", ReturnRepr: mir.ReprJSValue, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			{Result: 0, Repr: mir.ReprStringRef, Op: mir.ConstString{Value: "value"}},
			{Result: 1, Repr: mir.ReprJSValue, Op: mir.BoxJSValue{Kind: mir.BoxJSString, Value: 0}},
			{Result: 2, Repr: mir.ReprStringRef, Op: mir.ConstString{Value: "-any"}},
			{Result: 3, Repr: mir.ReprJSValue, Op: mir.BoxJSValue{Kind: mir.BoxJSString, Value: 2}},
			{Result: 4, Repr: mir.ReprJSValue, Op: mir.DynamicAddJSValue{Left: 1, Right: 3}},
			{Result: 5, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 1}},
			{Result: 6, Repr: mir.ReprVoid, Op: mir.Sleep{Duration: 5}},
		}, Terminator: mir.Return{Value: valueIDPtr(4)}}}},
		{ID: 2, Name: "launcher", ReturnRepr: mir.ReprVoid, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			{Result: 0, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 0}},
			{Result: 1, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 1}},
		}, Terminator: mir.Return{}}}},
	}}
	text, err := llvmcodegen.Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"%tsnative_task_env_f0 = type { i32, ptr, ptr, ptr }",
		"call ptr @tsnative_string_concat(ptr",
		"call ptr @tsnative_jsvalue_box_string(ptr",
		"call ptr @tsnative_jsvalue_add(ptr",
		"call void @tsnative_gc_safepoint()",
		"call void @tsnative_gc_store_ref(ptr %state, ptr %spill4.ptr, ptr %v4)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLVM IR missing %q:\n%s", want, text)
		}
	}
}

func TestEmitStacklessBranchContinuation(t *testing.T) {
	resultTrue := mir.ValueID(2)
	resultFalse := mir.ValueID(3)
	module := mir.Module{Name: "stackless-branch", Functions: []mir.Function{
		{ID: 0, Name: "choose", Params: []mir.Param{{Value: 0, Name: "flag", Repr: mir.ReprBool}}, ReturnRepr: mir.ReprF64, Entry: 0, Blocks: []mir.Block{
			{ID: 0, Instructions: []mir.Instruction{{Result: 1, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 1}}, {Result: 4, Repr: mir.ReprVoid, Op: mir.Sleep{Duration: 1}}}, Terminator: mir.Branch{Condition: 0, Then: 1, Else: 2}},
			{ID: 1, Instructions: []mir.Instruction{{Result: resultTrue, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 42}}}, Terminator: mir.Return{Value: &resultTrue}},
			{ID: 2, Instructions: []mir.Instruction{{Result: resultFalse, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 7}}}, Terminator: mir.Return{Value: &resultFalse}},
		}},
		{ID: 1, Name: "launcher", Params: []mir.Param{{Value: 0, Name: "flag", Repr: mir.ReprBool}}, ReturnRepr: mir.ReprVoid, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{{Result: 1, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 0, Captures: []mir.ValueID{0}}}}, Terminator: mir.Return{}}}},
	}}
	text, err := llvmcodegen.Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"%tsnative_task_env_f0 = type { i1, i32 }",
		"call i32 @tsnative_sleep_task(double 1.000000e+00)",
		"br i1 %capture0, label %step2, label %step3",
		"store double 4.200000e+01, ptr %result_slot",
		"store double 7.000000e+00, ptr %result_slot",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLVM IR missing %q:\n%s", want, text)
		}
	}
}

func TestEmitStacklessLoopPhiContinuation(t *testing.T) {
	ret := mir.ValueID(3)
	module := mir.Module{Name: "stackless-loop", Functions: []mir.Function{
		{ID: 0, Name: "sum", Params: []mir.Param{{Value: 0, Name: "limit", Repr: mir.ReprF64}}, ReturnRepr: mir.ReprF64, Entry: 0, Blocks: []mir.Block{
			{ID: 0, Instructions: []mir.Instruction{{Result: 1, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 0}}, {Result: 2, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 0}}}, Terminator: mir.Jump{Target: 1}},
			{ID: 1, Instructions: []mir.Instruction{
				{Result: 3, Repr: mir.ReprF64, Op: mir.Phi{Incoming: []mir.PhiIncoming{{Block: 0, Value: 1}, {Block: 2, Value: 7}}}},
				{Result: 4, Repr: mir.ReprF64, Op: mir.Phi{Incoming: []mir.PhiIncoming{{Block: 0, Value: 2}, {Block: 2, Value: 8}}}},
				{Result: 5, Repr: mir.ReprBool, Op: mir.FloatCompare{Operator: mir.FloatLessThan, Left: 4, Right: 0}},
			}, Terminator: mir.Branch{Condition: 5, Then: 2, Else: 3}},
			{ID: 2, Instructions: []mir.Instruction{
				{Result: 6, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 1}},
				{Result: 9, Repr: mir.ReprVoid, Op: mir.Sleep{Duration: 6}},
				{Result: 7, Repr: mir.ReprF64, Op: mir.FloatBinary{Operator: mir.FloatAdd, Left: 3, Right: 4}},
				{Result: 8, Repr: mir.ReprF64, Op: mir.FloatBinary{Operator: mir.FloatAdd, Left: 4, Right: 6}},
			}, Terminator: mir.Jump{Target: 1}},
			{ID: 3, Terminator: mir.Return{Value: &ret}},
		}},
		{ID: 1, Name: "launcher", Params: []mir.Param{{Value: 0, Name: "limit", Repr: mir.ReprF64}}, ReturnRepr: mir.ReprVoid, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{{Result: 1, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 0, Captures: []mir.ValueID{0}}}}, Terminator: mir.Return{}}}},
	}}
	text, err := llvmcodegen.Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"%tsnative_task_env_f0 = type { double, i32, double, double, i1, double, double }",
		"store double 0.000000e+00, ptr %spill0.ptr",
		"store double 0.000000e+00, ptr %spill1.ptr",
		"call i32 @tsnative_sleep_task(double 1.000000e+00)",
		"call i32 @tsnative_task_budget_poll_task()",
		"store double %spill.3.j7.phi0, ptr %spill0.ptr",
		"store double %spill.4.j7.phi1, ptr %spill1.ptr",
		"br label %step1",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLVM IR missing %q:\n%s", want, text)
		}
	}
}

func TestEmitStacklessNativeObjectArrayCallContinuation(t *testing.T) {
	retHelper := mir.ValueID(1)
	retWorker := mir.ValueID(10)
	module := mir.Module{
		Name:   "stackless-native-ops",
		Shapes: []mir.Shape{{ID: 0, Name: "Point", Fields: []mir.ShapeField{{Name: "value", Repr: mir.ReprF64}}}},
		Functions: []mir.Function{
			{ID: 0, Name: "double", Params: []mir.Param{{Value: 0, Name: "value", Repr: mir.ReprF64}}, ReturnRepr: mir.ReprF64, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
				{Result: 2, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 2}},
				{Result: 1, Repr: mir.ReprF64, Op: mir.FloatBinary{Operator: mir.FloatMul, Left: 0, Right: 2}},
			}, Terminator: mir.Return{Value: &retHelper}}}},
			{ID: 1, Name: "worker", Params: []mir.Param{{Value: 0, Name: "base", Repr: mir.ReprF64}}, ReturnRepr: mir.ReprF64, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
				{Result: 1, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 1}},
				{Result: 2, Repr: mir.ReprArrayRef, Op: mir.ArrayNewF64{Elements: []mir.ValueID{0, 1}}},
				{Result: 3, Repr: mir.ReprObjectRef, Op: mir.ObjectNew{Shape: 0, Fields: []mir.ValueID{0}}},
				{Result: 4, Repr: mir.ReprF64, Op: mir.FieldGet{Object: 3, Shape: 0, Field: 0}},
				{Result: 5, Repr: mir.ReprF64, Op: mir.Call{Callee: 0, Args: []mir.ValueID{4}}},
				{Result: 6, Repr: mir.ReprF64, Op: mir.ArraySetF64{Array: 2, Index: 1, Value: 5}},
				{Result: 7, Repr: mir.ReprVoid, Op: mir.Sleep{Duration: 1}},
				{Result: 10, Repr: mir.ReprF64, Op: mir.ArrayGetF64{Array: 2, Index: 1}},
			}, Terminator: mir.Return{Value: &retWorker}}}},
			{ID: 2, Name: "launcher", Params: []mir.Param{{Value: 0, Name: "base", Repr: mir.ReprF64}}, ReturnRepr: mir.ReprVoid, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{{Result: 1, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 1, Captures: []mir.ValueID{0}}}}, Terminator: mir.Return{}}}},
		},
	}
	text, err := llvmcodegen.Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"call ptr @tsnative_array_f64_new(i64 2)",
		"call ptr @tsnative_object_alloc_atomic(i64",
		"call double @tsnative_f0(double",
		"call void @tsnative_array_f64_set_checked(ptr",
		"call i32 @tsnative_sleep_task(double 1.000000e+00)",
		"call void @tsnative_gc_safepoint()",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLVM IR missing %q:\n%s", want, text)
		}
	}
}

func TestEmitStacklessDelayedTaskJoinUsesSpilledHandle(t *testing.T) {
	ret := mir.ValueID(5)
	module := mir.Module{Name: "delayed-task-join", Functions: []mir.Function{
		{ID: 0, Name: "child", ReturnRepr: mir.ReprVoid, Entry: 0, Blocks: []mir.Block{{ID: 0, Terminator: mir.Return{}}}},
		{ID: 1, Name: "parent", ReturnRepr: mir.ReprF64, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			{Result: 0, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 0}},
			{Result: 1, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 1}},
			{Result: 2, Repr: mir.ReprVoid, Op: mir.Sleep{Duration: 1}},
			{Result: 3, Repr: mir.ReprVoid, Op: mir.TaskJoin{Task: 0}},
			{Result: 5, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 42}},
		}, Terminator: mir.Return{Value: &ret}}}},
		{ID: 2, Name: "launcher", ReturnRepr: mir.ReprVoid, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{{Result: 0, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 1}}}, Terminator: mir.Return{}}}},
	}}
	text, err := llvmcodegen.Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"%tsnative_task_env_f1 = type { i32, ptr }",
		"store ptr %v0, ptr %spill0.ptr",
		"call i32 @tsnative_sleep_task(double 1.000000e+00)",
		"call i32 @tsnative_task_await_task(ptr %spill.0.join2)",
		"call void @tsnative_task_release(ptr %spill.0.release3)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLVM IR missing %q:\n%s", want, text)
		}
	}
}

func TestEmitReferenceChannelTaskContinuation(t *testing.T) {
	module := mir.Module{Name: "reference-channel", Functions: []mir.Function{
		{ID: 0, Name: "sender", Params: []mir.Param{{Value: 0, Name: "ch", Repr: mir.ReprChannelRef}, {Value: 1, Name: "value", Repr: mir.ReprStringRef}}, ReturnRepr: mir.ReprVoid, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			{Result: 2, Repr: mir.ReprVoid, Op: mir.ChannelSendRef{Channel: 0, Value: 1}},
		}, Terminator: mir.Return{}}}},
		{ID: 1, Name: "receiver", Params: []mir.Param{{Value: 0, Name: "ch", Repr: mir.ReprChannelRef}}, ReturnRepr: mir.ReprStringRef, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			{Result: 1, Repr: mir.ReprStringRef, Op: mir.ChannelRecvRef{Channel: 0}},
		}, Terminator: mir.Return{Value: valueIDPtr(1)}}}},
		{ID: 2, Name: "launcher", Params: []mir.Param{{Value: 0, Name: "ch", Repr: mir.ReprChannelRef}, {Value: 1, Name: "value", Repr: mir.ReprStringRef}}, ReturnRepr: mir.ReprVoid, Entry: 0, Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{
			{Result: 2, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 0, Captures: []mir.ValueID{0, 1}}},
			{Result: 3, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 1, Captures: []mir.ValueID{0}}},
		}, Terminator: mir.Return{}}}},
	}}
	text, err := llvmcodegen.Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"call i32 @tsnative_channel_ref_send_task(ptr %capture0, ptr %capture1)",
		"call i32 @tsnative_channel_ref_recv_task(ptr %capture0, ptr %spill0.ptr)",
		"store ptr %spill.0.ret1, ptr %result_slot",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLVM IR missing %q:\n%s", want, text)
		}
	}
}
