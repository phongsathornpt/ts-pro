package regalloc

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func TestLinearScanAllocation(t *testing.T) {
	fn := ir.NewFunction("testAlloc", types.TypeNumber)
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

	// 2 physical registers available
	ra := New(2)
	locs := ra.Allocate(fn)

	if len(locs) != 3 {
		t.Fatalf("expected 3 allocated locations, got %d", len(locs))
	}

	for id, loc := range locs {
		if !loc.IsReg && loc.StackSlot < 0 {
			t.Errorf("value %d unallocated", id)
		}
	}
}

func TestSpillWhenPressureHigh(t *testing.T) {
	fn := ir.NewFunction("testSpill", types.TypeNumber)
	b := fn.NewBlock("entry")

	v0 := fn.NewValue("v0", types.TypeNumber)
	v1 := fn.NewValue("v1", types.TypeNumber)
	v2 := fn.NewValue("v2", types.TypeNumber)

	b.Instructions = append(b.Instructions,
		&ir.BinaryInst{Res: v0, Op: ir.OpAdd, LHS: ir.ConstNumber{Value: 1}, RHS: ir.ConstNumber{Value: 2}},
		&ir.BinaryInst{Res: v1, Op: ir.OpAdd, LHS: ir.ConstNumber{Value: 3}, RHS: ir.ConstNumber{Value: 4}},
		&ir.BinaryInst{Res: v2, Op: ir.OpAdd, LHS: v0, RHS: v1},
	)
	b.Terminator = &ir.ReturnTerm{Val: v2}

	// Only 1 register available -> must spill
	ra := New(1)
	locs := ra.Allocate(fn)

	spillFound := false
	for _, loc := range locs {
		if !loc.IsReg {
			spillFound = true
			break
		}
	}

	if !spillFound {
		t.Errorf("expected at least one spill with only 1 physical register")
	}
}
