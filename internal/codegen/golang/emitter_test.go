package golang

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/projectthorn/tsv7-bin/internal/mir"
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
	command.Env = append(os.Environ(), "CGO_ENABLED=0", "GOCACHE=/tmp/tsv7-go-build")
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
				Result: 0, Repr: mir.ReprObjectRef, Op: mir.ObjectAlloc{Shape: 0},
			}}, Terminator: mir.Return{}}},
		}},
	}
	if _, err := Emit(module); err == nil || !strings.Contains(err.Error(), "not supported by the pure-Go backend") {
		t.Fatalf("Emit error = %v", err)
	}
}

func valuePtr(value mir.ValueID) *mir.ValueID { return &value }
