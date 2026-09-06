package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/backend/lower"
	"github.com/phongsathornpt/ts-pro/internal/backend/obj/elf"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func TestLinuxAMD64WinterTCInternalByteBuffer(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("requires linux/amd64 execution")
	}
	prog := &ir.Program{}
	fn := ir.NewFunction("@main", types.TypeVoid)
	bb := fn.NewBlock("entry")
	bufType := types.NewObject("$ByteBuffer")
	a := fn.NewValue("a", bufType)
	b := fn.NewValue("b", bufType)
	bb.Instructions = append(bb.Instructions,
		&ir.CallInst{Res: a, Callee: "ts_byte_buffer_new", Args: []ir.Operand{ir.ConstNumber{Value: 4}}, ParamTypes: []types.Type{types.TypeNumber}},
		&ir.CallInst{Res: b, Callee: "ts_byte_buffer_new", Args: []ir.Operand{ir.ConstNumber{Value: 4}}, ParamTypes: []types.Type{types.TypeNumber}},
	)

	for i, value := range []float64{65, 511, 66, 67} {
		bb.Instructions = append(bb.Instructions, &ir.CallInst{
			Callee:     "ts_byte_buffer_set",
			Args:       []ir.Operand{a, ir.ConstNumber{Value: float64(i)}, ir.ConstNumber{Value: value}},
			ParamTypes: []types.Type{bufType, types.TypeNumber, types.TypeNumber},
		})
	}
	bb.Instructions = append(bb.Instructions,
		&ir.CallInst{
			Callee:     "ts_byte_buffer_copy",
			Args:       []ir.Operand{b, a, ir.ConstNumber{Value: 0}, ir.ConstNumber{Value: 0}, ir.ConstNumber{Value: 4}},
			ParamTypes: []types.Type{bufType, bufType, types.TypeNumber, types.TypeNumber, types.TypeNumber},
		},
		&ir.CallInst{Callee: "ts_gc_collect"},
	)
	length := fn.NewValue("length", types.TypeNumber)
	bb.Instructions = append(bb.Instructions,
		&ir.CallInst{Res: length, Callee: "ts_byte_buffer_len", Args: []ir.Operand{b}, ParamTypes: []types.Type{bufType}},
		&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{length}, ParamTypes: []types.Type{types.TypeNumber}},
	)

	for i := 0; i < 4; i++ {
		v := fn.NewValue("byte", types.TypeNumber)
		bb.Instructions = append(bb.Instructions,
			&ir.CallInst{Res: v, Callee: "ts_byte_buffer_get", Args: []ir.Operand{b, ir.ConstNumber{Value: float64(i)}}, ParamTypes: []types.Type{bufType, types.TypeNumber}},
			&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{v}, ParamTypes: []types.Type{types.TypeNumber}},
		)
	}
	sliced := fn.NewValue("sliced", bufType)
	bb.Instructions = append(bb.Instructions,
		&ir.CallInst{Res: sliced, Callee: "ts_byte_buffer_slice", Args: []ir.Operand{b, ir.ConstNumber{Value: 1}, ir.ConstNumber{Value: 3}}, ParamTypes: []types.Type{bufType, types.TypeNumber, types.TypeNumber}},
		&ir.CallInst{Callee: "ts_gc_collect"},
	)
	sliceLen := fn.NewValue("slice_len", types.TypeNumber)
	bb.Instructions = append(bb.Instructions,
		&ir.CallInst{Res: sliceLen, Callee: "ts_byte_buffer_len", Args: []ir.Operand{sliced}, ParamTypes: []types.Type{bufType}},
		&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{sliceLen}, ParamTypes: []types.Type{types.TypeNumber}},
	)
	for i := 0; i < 2; i++ {
		v := fn.NewValue("slice_byte", types.TypeNumber)
		bb.Instructions = append(bb.Instructions,
			&ir.CallInst{Res: v, Callee: "ts_byte_buffer_get", Args: []ir.Operand{sliced, ir.ConstNumber{Value: float64(i)}}, ParamTypes: []types.Type{bufType, types.TypeNumber}},
			&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{v}, ParamTypes: []types.Type{types.TypeNumber}},
		)
	}
	utf8 := fn.NewValue("utf8", bufType)
	roundTrip := fn.NewValue("round_trip", types.TypeString)
	bb.Instructions = append(bb.Instructions,
		&ir.CallInst{Res: utf8, Callee: "ts_byte_buffer_from_utf8_string", Args: []ir.Operand{ir.ConstString{Value: "hé✓"}}, ParamTypes: []types.Type{types.TypeString}},
		&ir.CallInst{Callee: "ts_gc_collect"},
		&ir.CallInst{Res: roundTrip, Callee: "ts_byte_buffer_to_utf8_string", Args: []ir.Operand{utf8}, ParamTypes: []types.Type{bufType}},
		&ir.CallInst{Callee: "ts_print_str", Args: []ir.Operand{roundTrip}, ParamTypes: []types.Type{types.TypeString}},
	)
	bb.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, fn)

	code, err := lower.Lower(prog, lower.ArchAMD64)
	if err != nil {
		t.Fatalf("lower byte buffer IR: %v", err)
	}
	bin, err := elf.CreateExecutable(code, false)
	if err != nil {
		t.Fatalf("create byte buffer ELF: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "byte-buffer")
	if err := os.WriteFile(path, bin, 0o755); err != nil {
		t.Fatalf("write byte buffer ELF: %v", err)
	}

	out, err := exec.Command(path).CombinedOutput()
	if err != nil {
		t.Fatalf("execute byte buffer ELF: %v\nOutput:\n%s", err, out)
	}
	if got, want := string(out), "4\n65\n255\n66\n67\n2\n255\n66\nhé✓\n"; got != want {
		t.Fatalf("byte buffer output = %q, want %q", got, want)
	}
}
