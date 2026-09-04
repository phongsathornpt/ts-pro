package lower

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
	"github.com/phongsathornpt/ts-pro/internal/target"
)

func TestLowerAMD64(t *testing.T) {
	prog := &ir.Program{}
	fn := ir.NewFunction("add", types.TypeNumber)
	v0 := fn.NewValue("a", types.TypeNumber)
	v1 := fn.NewValue("b", types.TypeNumber)
	fn.Params = []*ir.Value{v0, v1}

	b := fn.NewBlock("entry")
	v2 := fn.NewValue("sum", types.TypeNumber)
	b.Instructions = append(b.Instructions, &ir.BinaryInst{
		Res: v2,
		Op:  ir.OpAdd,
		LHS: v0,
		RHS: v1,
	})
	b.Terminator = &ir.ReturnTerm{Val: v2}
	prog.Functions = append(prog.Functions, fn)

	code, err := Lower(prog, ArchAMD64)
	if err != nil {
		t.Fatalf("Lower AMD64 failed: %v", err)
	}

	if len(code) == 0 {
		t.Errorf("expected non-empty machine code")
	}
	// Ends with RET (0xC3)
	if code[len(code)-1] != 0xC3 {
		t.Errorf("expected RET (0xC3) at end of AMD64 code, got %x", code[len(code)-1])
	}
}

func TestLowerARM64(t *testing.T) {
	prog := &ir.Program{}
	fn := ir.NewFunction("add", types.TypeNumber)
	b := fn.NewBlock("entry")
	v2 := fn.NewValue("sum", types.TypeNumber)
	b.Instructions = append(b.Instructions, &ir.BinaryInst{
		Res: v2,
		Op:  ir.OpAdd,
	})
	b.Terminator = &ir.ReturnTerm{Val: v2}
	prog.Functions = append(prog.Functions, fn)

	code, err := Lower(prog, ArchARM64)
	if err != nil {
		t.Fatalf("Lower ARM64 failed: %v", err)
	}

	if len(code) == 0 {
		t.Errorf("expected non-empty machine code")
	}
}

func TestLowerScalars(t *testing.T) {
	prog := &ir.Program{}
	fn := ir.NewFunction("@main", types.TypeVoid)
	b := fn.NewBlock("entry")
	v0 := fn.NewValue("val", types.TypeNumber)
	b.Instructions = append(b.Instructions, &ir.CallInst{
		Res:    v0,
		Callee: "ts_print_val",
		Args:   []ir.Operand{ir.ConstNumber{Value: 42}},
	})
	b.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, fn)

	code, err := Lower(prog, ArchARM64)
	if err != nil {
		t.Fatalf("Lower failed: %v", err)
	}
	t.Logf("Generated ARM64 code length: %d bytes", len(code))
	for i := 0; i < len(code); i += 4 {
		t.Logf("%04x: %02x %02x %02x %02x", i, code[i], code[i+1], code[i+2], code[i+3])
	}
}

func TestCallArgPassing(t *testing.T) {
	prog := &ir.Program{}

	mathFn := ir.NewFunction("math", types.TypeNumber)
	a := mathFn.NewValue("a", types.TypeNumber)
	bArg := mathFn.NewValue("b", types.TypeNumber)
	mathFn.Params = []*ir.Value{a, bArg}
	mathBB := mathFn.NewBlock("entry")
	mathBB.Terminator = &ir.ReturnTerm{Val: a}
	prog.Functions = append(prog.Functions, mathFn)

	fn := ir.NewFunction("@main", types.TypeVoid)
	b := fn.NewBlock("entry")
	v0 := fn.NewValue("ret", types.TypeNumber)
	b.Instructions = append(b.Instructions, &ir.CallInst{
		Res:    v0,
		Callee: "math",
		Args:   []ir.Operand{ir.ConstNumber{Value: 8}, ir.ConstNumber{Value: 2}},
	})
	v1 := fn.NewValue("printRet", types.TypeNumber)
	b.Instructions = append(b.Instructions, &ir.CallInst{
		Res:    v1,
		Callee: "ts_print_val",
		Args:   []ir.Operand{v0},
	})
	b.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, fn)

	code, err := Lower(prog, ArchARM64)
	if err != nil {
		t.Fatalf("Lower failed: %v", err)
	}
	for i := 0; i < 40 && i < len(code); i += 4 {
		t.Logf("%04x: %02x %02x %02x %02x", i, code[i], code[i+1], code[i+2], code[i+3])
	}
}

func TestLowerRejectsUnresolvedCall(t *testing.T) {
	prog := &ir.Program{}
	fn := ir.NewFunction("@main", types.TypeVoid)
	b := fn.NewBlock("entry")
	b.Instructions = append(b.Instructions, &ir.CallInst{Callee: "missing"})
	b.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, fn)

	if _, err := Lower(prog, ArchAMD64); err == nil {
		t.Fatal("expected unresolved call target to fail lowering")
	}
}

func TestLowerTargetAndArchErrors(t *testing.T) {
	prog := &ir.Program{}
	if _, err := Lower(prog, Arch("unsupported")); err == nil {
		t.Errorf("expected unsupported arch error")
	}

	tgtLinux := target.Target{OS: target.OSLinux, Arch: target.ArchAMD64}
	if _, err := LowerTarget(prog, tgtLinux); err != nil {
		t.Errorf("expected LowerTarget linux/amd64 to succeed, got %v", err)
	}

	tgtDarwin := target.Target{OS: target.OSDarwin, Arch: target.ArchARM64}
	if _, err := LowerTarget(prog, tgtDarwin); err != nil {
		t.Errorf("expected LowerTarget darwin/arm64 to succeed, got %v", err)
	}

	tgtUnsupported := target.Target{OS: target.OSLinux, Arch: target.ArchARM64}
	if _, err := LowerTarget(prog, tgtUnsupported); err == nil {
		t.Errorf("expected error for unsupported target linux/arm64")
	}
}

func TestIsNumberTypeCoverage(t *testing.T) {
	if isNumberType(nil) {
		t.Errorf("isNumberType(nil) should be false")
	}
	if !isNumberType(types.TypeNumber) {
		t.Errorf("isNumberType(TypeNumber) should be true")
	}
	uNum := types.NewUnion(types.TypeNumber, types.TypeNull)
	if !isNumberType(uNum) {
		t.Errorf("isNumberType(number | null) should be true")
	}
	uOther := types.NewUnion(types.TypeString, types.TypeBoolean)
	if isNumberType(uOther) {
		t.Errorf("isNumberType(string | boolean) should be false")
	}
}
