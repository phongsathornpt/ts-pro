package lower

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
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
