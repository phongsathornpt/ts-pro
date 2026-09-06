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

func TestLinuxAMD64WinterTCUTF8Validator(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("requires linux/amd64 execution")
	}
	prog := &ir.Program{}
	fn := ir.NewFunction("@main", types.TypeVoid)
	bb := fn.NewBlock("entry")
	bufType := types.NewObject("$ByteBuffer")

	sequences := [][]float64{
		{0x68, 0xc3, 0xa9, 0xe2, 0x9c, 0x93},
		{0xc0, 0xaf},
		{0xed, 0xa0, 0x80},
		{0xf4, 0x90, 0x80, 0x80},
		{0xe2, 0x82},
	}
	for caseIndex, bytes := range sequences {
		buf := fn.NewValue("utf8_buf", bufType)
		bb.Instructions = append(bb.Instructions, &ir.CallInst{
			Res: buf, Callee: "ts_byte_buffer_new",
			Args:       []ir.Operand{ir.ConstNumber{Value: float64(len(bytes))}},
			ParamTypes: []types.Type{types.TypeNumber},
		})
		for i, b := range bytes {
			bb.Instructions = append(bb.Instructions, &ir.CallInst{
				Callee:     "ts_byte_buffer_set",
				Args:       []ir.Operand{buf, ir.ConstNumber{Value: float64(i)}, ir.ConstNumber{Value: b}},
				ParamTypes: []types.Type{bufType, types.TypeNumber, types.TypeNumber},
			})
		}

		valid := fn.NewValue("utf8_valid", types.TypeBoolean)
		bb.Instructions = append(bb.Instructions,
			&ir.CallInst{
				Res: valid, Callee: "ts_utf8_validate",
				Args:       []ir.Operand{buf, ir.ConstNumber{Value: 0}, ir.ConstNumber{Value: float64(len(bytes))}},
				ParamTypes: []types.Type{bufType, types.TypeNumber, types.TypeNumber},
			},
			&ir.CallInst{Callee: "ts_print_bool", Args: []ir.Operand{valid}, ParamTypes: []types.Type{types.TypeBoolean}},
		)
		_ = caseIndex
	}
	bb.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, fn)

	code, err := lower.Lower(prog, lower.ArchAMD64)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	bin, err := elf.CreateExecutable(code, false)
	if err != nil {
		t.Fatalf("elf: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "utf8-validator")
	if err := os.WriteFile(path, bin, 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, err := exec.Command(path).CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if got, want := string(out), "true\nfalse\nfalse\nfalse\nfalse\n"; got != want {
		t.Fatalf("validator output = %q, want %q", got, want)
	}
}
