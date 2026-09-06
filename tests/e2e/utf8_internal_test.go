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

func TestLinuxAMD64WinterTCStringBytePrimitives(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("requires linux/amd64 execution")
	}
	prog := &ir.Program{}
	fn := ir.NewFunction("@main", types.TypeVoid)
	bb := fn.NewBlock("entry")
	text := ir.ConstString{Value: "hé✓"}
	length := fn.NewValue("string_len", types.TypeNumber)
	byte1 := fn.NewValue("string_byte", types.TypeNumber)
	slice := fn.NewValue("string_slice", types.TypeString)
	bb.Instructions = append(bb.Instructions,
		&ir.CallInst{Res: length, Callee: "ts_string_len", Args: []ir.Operand{text}, ParamTypes: []types.Type{types.TypeString}},
		&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{length}, ParamTypes: []types.Type{types.TypeNumber}},
		&ir.CallInst{Res: byte1, Callee: "ts_string_byte_at", Args: []ir.Operand{text, ir.ConstNumber{Value: 1}}, ParamTypes: []types.Type{types.TypeString, types.TypeNumber}},
		&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{byte1}, ParamTypes: []types.Type{types.TypeNumber}},
		&ir.CallInst{Res: slice, Callee: "ts_string_slice_bytes", Args: []ir.Operand{text, ir.ConstNumber{Value: 1}, ir.ConstNumber{Value: 3}}, ParamTypes: []types.Type{types.TypeString, types.TypeNumber, types.TypeNumber}},
		&ir.CallInst{Callee: "ts_print_str", Args: []ir.Operand{slice}, ParamTypes: []types.Type{types.TypeString}},
	)
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
	path := filepath.Join(t.TempDir(), "string-bytes")
	if err := os.WriteFile(path, bin, 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, err := exec.Command(path).CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if got, want := string(out), "6\n195\né\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestLinuxAMD64WinterTCFormURLEncodedCodec(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("requires linux/amd64 execution")
	}
	prog := &ir.Program{}
	fn := ir.NewFunction("@main", types.TypeVoid)
	bb := fn.NewBlock("entry")
	bufType := types.NewObject("$ByteBuffer")
	decodedBytes := fn.NewValue("form_decoded_bytes", bufType)
	decodedLen := fn.NewValue("form_decoded_len", types.TypeNumber)
	decoded := fn.NewValue("form_decoded", types.TypeString)
	encoded := fn.NewValue("form_encoded", types.TypeString)
	bb.Instructions = append(bb.Instructions,
		&ir.CallInst{Res: decodedBytes, Callee: "ts_form_url_decode_bytes", Args: []ir.Operand{ir.ConstString{Value: "a+b%20c%2B%E2%9C%93%ZZ"}}, ParamTypes: []types.Type{types.TypeString}},
		&ir.CallInst{Res: decodedLen, Callee: "ts_byte_buffer_len", Args: []ir.Operand{decodedBytes}, ParamTypes: []types.Type{bufType}},
		&ir.CallInst{Res: decoded, Callee: "ts_utf8_sanitize", Args: []ir.Operand{decodedBytes, ir.ConstNumber{Value: 0}, decodedLen, ir.ConstBool{Value: false}}, ParamTypes: []types.Type{bufType, types.TypeNumber, types.TypeNumber, types.TypeBoolean}},
		&ir.CallInst{Callee: "ts_print_str", Args: []ir.Operand{decoded}, ParamTypes: []types.Type{types.TypeString}},
		&ir.CallInst{Res: encoded, Callee: "ts_form_url_encode", Args: []ir.Operand{ir.ConstString{Value: "a b+c✓*"}}, ParamTypes: []types.Type{types.TypeString}},
		&ir.CallInst{Callee: "ts_print_str", Args: []ir.Operand{encoded}, ParamTypes: []types.Type{types.TypeString}},
	)
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
	path := filepath.Join(t.TempDir(), "form-codec")
	if err := os.WriteFile(path, bin, 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, err := exec.Command(path).CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if got, want := string(out), "a b c+✓%ZZ\na+b%2Bc%E2%9C%93*\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
