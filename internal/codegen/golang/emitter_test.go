package golang

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/phongsathornpt/ts-pro/internal/mir"
)

func TestEmitPureGoProgramCompilesAndRuns(t *testing.T) {
	entry := mir.FunctionID(1)
	module := mir.Module{
		Name:  "pure-go-smoke",
		Entry: &entry,
		Functions: []mir.Function{
			{
				ID: 0, Name: "add", ReturnRepr: mir.ReprF64, Entry: 0,
				Params: []mir.Param{
					{Value: 0, Name: "left", Repr: mir.ReprF64},
					{Value: 1, Name: "right", Repr: mir.ReprF64},
				},
				Blocks: []mir.Block{{
					ID: 0,
					Instructions: []mir.Instruction{{
						Result: 2, Repr: mir.ReprF64,
						Op: mir.FloatBinary{Operator: mir.FloatAdd, Left: 0, Right: 1},
					}},
					Terminator: mir.Return{Value: valuePtr(2)},
				}},
			},
			{
				ID: 1, Name: "entry", ReturnRepr: mir.ReprVoid, Entry: 0,
				Blocks: []mir.Block{{
					ID: 0,
					Instructions: []mir.Instruction{
						{Result: 0, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 8}},
						{Result: 1, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 2}},
						{Result: 2, Repr: mir.ReprF64, Op: mir.Call{Callee: 0, Args: []mir.ValueID{0, 1}}},
						{Result: 3, Repr: mir.ReprVoid, Op: mir.IntrinsicCall{Intrinsic: mir.IntrinsicConsoleLogF64, Args: []mir.ValueID{2}}},
					},
					Terminator: mir.Return{},
				}},
			},
		},
	}

	source, err := Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	// Keep the assertion explicit: generated source must not import C or
	// reference the legacy native ABI.
	if strings.Contains(source, `import "C"`) || strings.Contains(source, "tsnative_") {
		t.Fatalf("generated pure-Go source contains native ABI references:\n%s", source)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/puregosmoke\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainPath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, "go", "run", ".")
	command.Dir = dir
	command.Env = append(os.Environ(), "CGO_ENABLED=0", "GOCACHE=/tmp/ts-pro-go-build")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run generated pure-Go program: %v: %s\nsource:\n%s", err, output, source)
	}
	if got := strings.TrimSpace(string(output)); got != "10" {
		t.Fatalf("generated output = %q, want %q", got, "10")
	}
}

func TestEmitRejectsLegacyRepresentations(t *testing.T) {
	entry := mir.FunctionID(0)
	module := mir.Module{
		Name:   "unsupported",
		Entry:  &entry,
		Shapes: []mir.Shape{{ID: 0, Name: "Object"}},
		Functions: []mir.Function{{
			ID: 0, Name: "entry", ReturnRepr: mir.ReprVoid, Entry: 0,
			Blocks: []mir.Block{{ID: 0, Instructions: []mir.Instruction{{
				Result: 0, Repr: mir.Repr(255), Op: mir.ObjectAlloc{Shape: 0},
			}}, Terminator: mir.Return{}}},
		}},
	}
	if _, err := Emit(module); err == nil || !strings.Contains(err.Error(), "not supported by the pure-Go backend") {
		t.Fatalf("Emit error = %v", err)
	}
}

func TestEmitPureGoArrays(t *testing.T) {
	entry := mir.FunctionID(0)
	module := mir.Module{
		Name:  "pure-go-arrays",
		Entry: &entry,
		Functions: []mir.Function{{
			ID: 0, Name: "entry", ReturnRepr: mir.ReprVoid, Entry: 0,
			Blocks: []mir.Block{{
				ID: 0,
				Instructions: []mir.Instruction{
					{Result: 0, Repr: mir.ReprBool, Op: mir.ConstBool{Value: true}},
					{Result: 1, Repr: mir.ReprBool, Op: mir.ConstBool{Value: false}},
					{Result: 2, Repr: mir.ReprArrayRef, Op: mir.ArrayNewBool{Elements: []mir.ValueID{0, 1}}},
					{Result: 3, Repr: mir.ReprF64, Op: mir.ArrayLengthBool{Array: 2}},
					{Result: 4, Repr: mir.ReprVoid, Op: mir.IntrinsicCall{Intrinsic: mir.IntrinsicConsoleLogF64, Args: []mir.ValueID{3}}},
				},
				Terminator: mir.Return{},
			}},
		}},
	}
	output := runEmittedModule(t, module)
	if got := strings.TrimSpace(output); got != "2" {
		t.Fatalf("output = %q, want %q", got, "2")
	}
}

func TestEmitPureGoInteriorPointer(t *testing.T) {
	entry := mir.FunctionID(0)
	module := mir.Module{
		Name:   "pure-go-interior-ptr",
		Entry:  &entry,
		Shapes: []mir.Shape{{ID: 0, Name: "Counter", Fields: []mir.ShapeField{{Name: "val", Repr: mir.ReprF64}}}},
		Functions: []mir.Function{{
			ID: 0, Name: "main", ReturnRepr: mir.ReprVoid, Entry: 0,
			Blocks: []mir.Block{{
				ID: 0,
				Instructions: []mir.Instruction{
					{Result: 0, Repr: mir.ReprObjectRef, Op: mir.ObjectAlloc{Shape: 0}},
					{Result: 1, Repr: mir.ReprRawPtr, Op: mir.FieldAddr{Object: 0, Shape: 0, Field: 0}},
					{Result: 2, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 123}},
					{Result: 3, Repr: mir.ReprVoid, Op: mir.PtrStore{Ptr: 1, Value: 2}},
					{Result: 4, Repr: mir.ReprF64, Op: mir.PtrLoad{Ptr: 1, Repr: mir.ReprF64}},
					{Result: 5, Repr: mir.ReprVoid, Op: mir.IntrinsicCall{Intrinsic: mir.IntrinsicConsoleLogF64, Args: []mir.ValueID{4}}},
				},
				Terminator: mir.Return{},
			}},
		}},
	}
	output := runEmittedModule(t, module)
	if got := strings.TrimSpace(output); got != "123" {
		t.Fatalf("output = %q, want %q", got, "123")
	}
}

func TestEmitPureGoDynamicJSValues(t *testing.T) {
	entry := mir.FunctionID(0)
	module := mir.Module{
		Name:  "pure-go-dynamic",
		Entry: &entry,
		Functions: []mir.Function{{
			ID: 0, Name: "entry", ReturnRepr: mir.ReprVoid, Entry: 0,
			Blocks: []mir.Block{{
				ID: 0,
				Instructions: []mir.Instruction{
					{Result: 0, Repr: mir.ReprStringRef, Op: mir.ConstString{Value: "hello "}},
					{Result: 1, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 42}},
					{Result: 2, Repr: mir.ReprJSValue, Op: mir.BoxJSValue{Kind: mir.BoxJSString, Value: 0}},
					{Result: 3, Repr: mir.ReprJSValue, Op: mir.BoxJSValue{Kind: mir.BoxJSNumber, Value: 1}},
					{Result: 4, Repr: mir.ReprJSValue, Op: mir.DynamicAddJSValue{Left: 2, Right: 3}},
					{Result: 5, Repr: mir.ReprVoid, Op: mir.IntrinsicCall{Intrinsic: mir.IntrinsicConsoleLogJSValue, Args: []mir.ValueID{4}}},
				},
				Terminator: mir.Return{},
			}},
		}},
	}
	output := runEmittedModule(t, module)
	if got := strings.TrimSpace(output); got != "hello 42" {
		t.Fatalf("output = %q, want %q", got, "hello 42")
	}
}

func TestEmitPureGoTasksAndChannels(t *testing.T) {
	entry := mir.FunctionID(1)
	module := mir.Module{
		Name:  "pure-go-concurrency",
		Entry: &entry,
		Functions: []mir.Function{
			{
				ID: 0, Name: "worker", ReturnRepr: mir.ReprF64, Entry: 0,
				Blocks: []mir.Block{{
					ID: 0,
					Instructions: []mir.Instruction{
						{Result: 0, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 99}},
					},
					Terminator: mir.Return{Value: valuePtr(0)},
				}},
			},
			{
				ID: 1, Name: "entry", ReturnRepr: mir.ReprVoid, Entry: 0,
				Blocks: []mir.Block{{
					ID: 0,
					Instructions: []mir.Instruction{
						{Result: 0, Repr: mir.ReprTaskRef, Op: mir.TaskSpawn{Callee: 0}},
						{Result: 1, Repr: mir.ReprF64, Op: mir.TaskJoin{Task: 0}},
						{Result: 2, Repr: mir.ReprVoid, Op: mir.IntrinsicCall{Intrinsic: mir.IntrinsicConsoleLogF64, Args: []mir.ValueID{1}}},
					},
					Terminator: mir.Return{},
				}},
			},
		},
	}
	output := runEmittedModule(t, module)
	if got := strings.TrimSpace(output); got != "99" {
		t.Fatalf("output = %q, want %q", got, "99")
	}
}

func TestEmitPureGoClosures(t *testing.T) {
	entry := mir.FunctionID(1)
	module := mir.Module{
		Name:  "pure-go-closures",
		Entry: &entry,
		Functions: []mir.Function{
			{
				ID: 0, Name: "adder", ReturnRepr: mir.ReprF64, Entry: 0,
				Params: []mir.Param{
					{Value: 0, Name: "captured", Repr: mir.ReprF64},
					{Value: 1, Name: "arg", Repr: mir.ReprF64},
				},
				Blocks: []mir.Block{{
					ID: 0,
					Instructions: []mir.Instruction{
						{Result: 2, Repr: mir.ReprF64, Op: mir.FloatBinary{Operator: mir.FloatAdd, Left: 0, Right: 1}},
					},
					Terminator: mir.Return{Value: valuePtr(2)},
				}},
			},
			{
				ID: 1, Name: "entry", ReturnRepr: mir.ReprVoid, Entry: 0,
				Blocks: []mir.Block{{
					ID: 0,
					Instructions: []mir.Instruction{
						{Result: 0, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 100}},
						{Result: 1, Repr: mir.ReprFunctionRef, Op: mir.ClosureNew{Callee: 0, Captures: []mir.ValueID{0}}},
						{Result: 2, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 42}},
						{Result: 3, Repr: mir.ReprF64, Op: mir.ClosureCall{Closure: 1, Args: []mir.ValueID{2}}},
						{Result: 4, Repr: mir.ReprVoid, Op: mir.IntrinsicCall{Intrinsic: mir.IntrinsicConsoleLogF64, Args: []mir.ValueID{3}}},
					},
					Terminator: mir.Return{},
				}},
			},
		},
	}
	output := runEmittedModule(t, module)
	if got := strings.TrimSpace(output); got != "142" {
		t.Fatalf("output = %q, want %q", got, "142")
	}
}

func TestEmitPureGoChannelCapacityValidation(t *testing.T) {
	entry := mir.FunctionID(0)
	module := mir.Module{
		Name:  "channel-cap-test",
		Entry: &entry,
		Functions: []mir.Function{
			{
				ID: 0, Name: "main", ReturnRepr: mir.ReprVoid, Entry: 0,
				Blocks: []mir.Block{{
					ID: 0,
					Instructions: []mir.Instruction{
						// Negative capacity -> clamped to 0
						{Result: 0, Repr: mir.ReprF64, Op: mir.ConstF64{Value: -1}},
						{Result: 1, Repr: mir.ReprChannelRef, Op: mir.ChannelNewF64{Capacity: 0}},
						// Huge capacity (1e18) -> clamped to 65536
						{Result: 2, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 1e18}},
						{Result: 3, Repr: mir.ReprChannelRef, Op: mir.ChannelNewF64{Capacity: 2}},
						// NaN capacity -> clamped to 0
						{Result: 4, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 0}},
						{Result: 5, Repr: mir.ReprF64, Op: mir.FloatBinary{Operator: mir.FloatDiv, Left: 4, Right: 4}},
						{Result: 6, Repr: mir.ReprChannelRef, Op: mir.ChannelNewF64{Capacity: 5}},
						// Send value 42 on channel 3 (buffered, cap clamped to 65536)
						{Result: 7, Repr: mir.ReprF64, Op: mir.ConstF64{Value: 42}},
						{Result: 8, Repr: mir.ReprVoid, Op: mir.ChannelSendF64{Channel: 3, Value: 7}},
						{Result: 9, Repr: mir.ReprF64, Op: mir.ChannelRecvF64{Channel: 3}},
						{Result: 10, Repr: mir.ReprVoid, Op: mir.IntrinsicCall{Intrinsic: mir.IntrinsicConsoleLogF64, Args: []mir.ValueID{9}}},
					},
					Terminator: mir.Return{},
				}},
			},
		},
	}
	output := runEmittedModule(t, module)
	if got := strings.TrimSpace(output); got != "42" {
		t.Fatalf("output = %q, want %q", got, "42")
	}
}

func runEmittedModule(t *testing.T, module mir.Module) string {
	t.Helper()
	source, err := Emit(module)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(source, `import "C"`) || strings.Contains(source, "tsnative_") {
		t.Fatalf("generated pure-Go source contains native ABI references:\n%s", source)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/testrun\n\ngo 1.27\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, "go", "run", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOCACHE=/tmp/ts-pro-go-build")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run generated code: %v\nsource:\n%s\noutput:\n%s", err, source, out)
	}
	return string(out)
}

func valuePtr(value mir.ValueID) *mir.ValueID { return &value }
