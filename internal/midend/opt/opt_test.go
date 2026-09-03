package opt

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func TestConstantFolding(t *testing.T) {
	prog := &ir.Program{}
	fn := ir.NewFunction("testConst", types.TypeNumber)
	b := fn.NewBlock("entry")
	v0 := fn.NewValue("res", types.TypeNumber)

	// res = 10 + 20
	b.Instructions = append(b.Instructions, &ir.BinaryInst{
		Res: v0,
		Op:  ir.OpAdd,
		LHS: ir.ConstNumber{Value: 10},
		RHS: ir.ConstNumber{Value: 20},
	})
	b.Terminator = &ir.ReturnTerm{Val: v0}
	prog.Functions = append(prog.Functions, fn)

	Optimize(prog, Options{Level: 2})

	// After constant folding + DCE, res = 10 + 20 should be folded and instruction eliminated
	if len(b.Instructions) != 0 {
		t.Errorf("expected instruction to be folded away, got %d instructions", len(b.Instructions))
	}
}

func TestDeadCodeElimination(t *testing.T) {
	prog := &ir.Program{}
	fn := ir.NewFunction("testDCE", types.TypeNumber)
	b := fn.NewBlock("entry")
	deadVal := fn.NewValue("dead", types.TypeNumber)
	usedVal := fn.NewValue("used", types.TypeNumber)

	// deadVal = param + 1 (never used)
	b.Instructions = append(b.Instructions, &ir.BinaryInst{
		Res: deadVal,
		Op:  ir.OpAdd,
		LHS: ir.ConstNumber{Value: 1},
		RHS: ir.ConstNumber{Value: 2},
	})
	// usedVal = 42
	b.Instructions = append(b.Instructions, &ir.BinaryInst{
		Res: usedVal,
		Op:  ir.OpAdd,
		LHS: ir.ConstNumber{Value: 40},
		RHS: ir.ConstNumber{Value: 2},
	})
	b.Terminator = &ir.ReturnTerm{Val: usedVal}
	prog.Functions = append(prog.Functions, fn)

	Optimize(prog, Options{Level: 1})

	for _, inst := range b.Instructions {
		if inst.Result() == deadVal {
			t.Errorf("expected deadVal instruction to be eliminated")
		}
	}
}
