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

func TestConstantFoldingAllOperators(t *testing.T) {
	ops := []ir.BinaryOp{
		ir.OpSub, ir.OpMul, ir.OpDiv, ir.OpMod, ir.OpAnd, ir.OpOr,
	}
	for _, op := range ops {
		prog := &ir.Program{}
		fn := ir.NewFunction("testOp", types.TypeNumber)
		b := fn.NewBlock("entry")
		v := fn.NewValue("res", types.TypeNumber)
		b.Instructions = append(b.Instructions, &ir.BinaryInst{
			Res: v,
			Op:  op,
			LHS: ir.ConstNumber{Value: 12},
			RHS: ir.ConstNumber{Value: 4},
		})
		b.Terminator = &ir.ReturnTerm{Val: v}
		prog.Functions = append(prog.Functions, fn)
		Optimize(prog, Options{Level: 2})
		if len(b.Instructions) != 0 {
			t.Errorf("expected op %v to be folded away", op)
		}
	}

	// Unary constant folding
	prog := &ir.Program{}
	fn := ir.NewFunction("testUnary", types.TypeNumber)
	b := fn.NewBlock("entry")
	v := fn.NewValue("neg", types.TypeNumber)
	b.Instructions = append(b.Instructions, &ir.UnaryInst{
		Res: v,
		Op:  "-",
		Val: ir.ConstNumber{Value: 42},
	})
	b.Terminator = &ir.ReturnTerm{Val: v}
	prog.Functions = append(prog.Functions, fn)
	Optimize(prog, Options{Level: 2})
	if len(b.Instructions) != 0 {
		t.Errorf("expected unary negation to be folded away")
	}

	// Branch constant folding
	prog2 := &ir.Program{}
	fn2 := ir.NewFunction("testBranch", types.TypeVoid)
	bEntry := fn2.NewBlock("entry")
	bThen := fn2.NewBlock("then")
	bElse := fn2.NewBlock("else")
	bThen.Terminator = &ir.ReturnTerm{}
	bElse.Terminator = &ir.ReturnTerm{}
	bEntry.Terminator = &ir.BranchTerm{
		Cond: ir.ConstBool{Value: true},
		Then: bThen,
		Else: bElse,
	}
	prog2.Functions = append(prog2.Functions, fn2)
	Optimize(prog2, Options{Level: 2})
	if _, ok := bEntry.Terminator.(*ir.JumpTerm); !ok {
		t.Errorf("expected constant true branch to fold into JumpTerm")
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

func TestOptimizeLevel0(t *testing.T) {
	prog := &ir.Program{}
	Optimize(prog, Options{Level: 0})
}
