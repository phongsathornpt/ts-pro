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
	if err := tc.Link(ctx, []string{obj, runtimeObj}, bin); err != nil {
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
