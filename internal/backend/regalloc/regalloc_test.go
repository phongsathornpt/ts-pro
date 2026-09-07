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
	if ra.StackFrameSlots() < 0 {
		t.Errorf("invalid stack frame slots")
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

func TestParametersLiveAtEntryDoNotShareRegister(t *testing.T) {
	fn := ir.NewFunction("entryParams", types.TypeNumber)
	thisVal := fn.NewValue("this", types.NewObject("Box"))
	used := fn.NewValue("used", types.TypeNumber)
	unused := fn.NewValue("unused", types.NewFunction(nil, types.TypeVoid))
	fn.Params = []*ir.Value{thisVal, used, unused}
	b := fn.NewBlock("entry")
	field := fn.NewValue("field", types.TypeNumber)
	b.Instructions = append(b.Instructions, &ir.GetFieldInst{Res: field, Obj: thisVal, Field: "value", Offset: 16})
	result := fn.NewValue("result", types.TypeNumber)
	b.Instructions = append(b.Instructions, &ir.BinaryInst{Res: result, Op: ir.OpAdd, LHS: field, RHS: used})
	b.Terminator = &ir.ReturnTerm{Val: result}

	ra := New(4)
	locs := ra.Allocate(fn)
	if locs[thisVal.ID].IsReg && locs[unused.ID].IsReg && locs[thisVal.ID].Reg == locs[unused.ID].Reg {
		t.Fatalf("this and unused trailing parameter share entry register %d", locs[thisVal.ID].Reg)
	}
	if locs[used.ID].IsReg && locs[unused.ID].IsReg && locs[used.ID].Reg == locs[unused.ID].Reg {
		t.Fatalf("used and unused trailing parameter share entry register %d", locs[used.ID].Reg)
	}
}

func TestIntervalsAllInstructionKinds(t *testing.T) {
	fn := ir.NewFunction("allInsts", types.TypeNumber)
	vArr := fn.NewValue("arr", types.NewArray(types.TypeNumber))
	vObj := fn.NewValue("obj", types.NewObject("Point"))
	vVal := fn.NewValue("val", types.TypeNumber)
	vIdx := fn.NewValue("idx", types.TypeNumber)
	vLen := fn.NewValue("len", types.TypeNumber)
	vPop := fn.NewValue("pop", types.TypeNumber)
	vPush := fn.NewValue("push", types.TypeNumber)
	vUnary := fn.NewValue("unary", types.TypeNumber)
	vClosure := fn.NewValue("closure", types.NewFunction(nil, types.TypeVoid))
	vIndirect := fn.NewValue("indirect", types.TypeVoid)

	b := fn.NewBlock("entry")
	b.Instructions = append(b.Instructions,
		&ir.UnaryInst{Res: vUnary, Op: "-", Val: vVal},
		&ir.SetFieldInst{Obj: vObj, Field: "x", Offset: 16, Val: vVal},
		&ir.MakeClosureInst{Res: vClosure, Function: "foo", Captures: []ir.Operand{vVal}},
		&ir.ClosureGetInst{Res: vVal, Closure: vClosure, Index: 0},
		&ir.IndirectCallInst{Res: vIndirect, Closure: vClosure, ThisArg: vObj, Args: []ir.Operand{vVal}},
		&ir.AllocArrayInst{Res: vArr, ElemType: types.TypeNumber, Length: vVal},
		&ir.GetElementInst{Res: vVal, Array: vArr, Index: vIdx},
		&ir.SetElementInst{Array: vArr, Index: vIdx, Val: vVal},
		&ir.ArrayLengthInst{Res: vLen, Array: vArr},
		&ir.ArrayPushInst{Res: vPush, Array: vArr, Val: vVal},
		&ir.ArrayPopInst{Res: vPop, Array: vArr},
	)

	loopHeader := fn.NewBlock("loopHeader")
	loopHeader.Phis = append(loopHeader.Phis, &ir.PhiInst{
		Res:      vVal,
		Incoming: []ir.PhiIncoming{{Block: b, Value: vVal}},
	})
	b.Terminator = &ir.JumpTerm{Target: loopHeader}

	loopBody := fn.NewBlock("loopBody")
	loopHeader.Terminator = &ir.BranchTerm{Cond: vVal, Then: loopBody, Else: b}

	// Backedge jump to loopHeader
	loopBody.Terminator = &ir.JumpTerm{Target: loopHeader}

	ra := New(2)
	locs := ra.Allocate(fn)
	if len(locs) == 0 {
		t.Fatalf("expected allocations, got none")
	}
}

func TestSpillActiveEviction(t *testing.T) {
	fn := ir.NewFunction("testEvict", types.TypeNumber)
	b := fn.NewBlock("entry")

	// vLong has a very long lifetime
	vLong := fn.NewValue("vLong", types.TypeNumber)
	// vShort has a shorter lifetime
	vShort := fn.NewValue("vShort", types.TypeNumber)
	vMedium := fn.NewValue("vMedium", types.TypeNumber)

	b.Instructions = append(b.Instructions,
		&ir.CallInst{Res: vLong, Callee: "fn0", Args: []ir.Operand{vLong}},
		&ir.BinaryInst{Res: vShort, Op: ir.OpAdd, LHS: ir.ConstNumber{Value: 1}, RHS: ir.ConstNumber{Value: 2}},
		&ir.BinaryInst{Res: vMedium, Op: ir.OpAdd, LHS: vShort, RHS: ir.ConstNumber{Value: 3}},
	)
	target := fn.NewBlock("target")
	target.Phis = append(target.Phis, &ir.PhiInst{
		Res:      vLong,
		Incoming: []ir.PhiIncoming{{Block: b, Value: vLong}},
	})
	b.Terminator = &ir.BranchTerm{Cond: vShort, Then: target, Else: target}
	target.Terminator = &ir.ReturnTerm{Val: vLong}

	ra := New(1)
	_ = ra.Allocate(fn)
}

func TestSortActiveTieBreak(t *testing.T) {
	fn := ir.NewFunction("tiebreak", types.TypeNumber)
	b := fn.NewBlock("entry")

	v1 := fn.NewValue("v1", types.TypeNumber)
	v2 := fn.NewValue("v2", types.TypeNumber)
	v3 := fn.NewValue("v3", types.TypeNumber)
	v4 := fn.NewValue("v4", types.TypeNumber)

	b.Instructions = append(b.Instructions,
		&ir.BinaryInst{Res: v1, Op: ir.OpAdd, LHS: ir.ConstNumber{Value: 1}, RHS: ir.ConstNumber{Value: 2}},
		&ir.BinaryInst{Res: v2, Op: ir.OpAdd, LHS: ir.ConstNumber{Value: 3}, RHS: ir.ConstNumber{Value: 4}},
		&ir.BinaryInst{Res: v3, Op: ir.OpAdd, LHS: ir.ConstNumber{Value: 5}, RHS: ir.ConstNumber{Value: 6}},
		&ir.BinaryInst{Res: v4, Op: ir.OpAdd, LHS: v1, RHS: v2},
	)
	b.Terminator = &ir.ReturnTerm{Val: v3}

	ra := New(2)
	_ = ra.Allocate(fn)
}

func TestSortActiveTieBreak2(t *testing.T) {
	fn := ir.NewFunction("tiebreak2", types.TypeNumber)
	b := fn.NewBlock("entry")

	v1 := fn.NewValue("v1", types.TypeNumber)
	v2 := fn.NewValue("v2", types.TypeNumber)
	v3 := fn.NewValue("v3", types.TypeNumber)
	v4 := fn.NewValue("v4", types.TypeNumber)
	v5 := fn.NewValue("v5", types.TypeNumber)

	b.Instructions = append(b.Instructions,
		&ir.CallInst{Res: v1, Callee: "f1", Args: []ir.Operand{}},
		&ir.CallInst{Res: v2, Callee: "f2", Args: []ir.Operand{}},
		&ir.BinaryInst{Res: v3, Op: ir.OpAdd, LHS: ir.ConstNumber{Value: 1}, RHS: ir.ConstNumber{Value: 2}},
		&ir.BinaryInst{Res: v4, Op: ir.OpAdd, LHS: v3, RHS: v1},
		&ir.BinaryInst{Res: v5, Op: ir.OpAdd, LHS: v4, RHS: v2},
	)
	b.Terminator = &ir.ReturnTerm{Val: v5}

	ra := New(2)
	_ = ra.Allocate(fn)
}

func TestCFGNonLinearBlockOrderKeepsLiveThroughValue(t *testing.T) {
	fn := ir.NewFunction("cfgNonLinear", types.TypeNumber)
	entry := fn.NewBlock("entry")
	def := fn.NewBlock("def")
	use := fn.NewBlock("use") // Intentionally created before the bridge.
	bridge := fn.NewBlock("bridge")

	entry.Terminator = &ir.JumpTerm{Target: def}
	key := fn.NewValue("key", types.TypeNumber)
	def.Instructions = append(def.Instructions, &ir.BinaryInst{Res: key, Op: ir.OpAdd, LHS: ir.ConstNumber{Value: 1}, RHS: ir.ConstNumber{Value: 2}})
	def.Terminator = &ir.JumpTerm{Target: bridge}

	tmp := fn.NewValue("tmp", types.TypeNumber)
	bridge.Instructions = append(bridge.Instructions, &ir.BinaryInst{Res: tmp, Op: ir.OpAdd, LHS: ir.ConstNumber{Value: 4}, RHS: ir.ConstNumber{Value: 5}})
	bridge.Terminator = &ir.JumpTerm{Target: use}
	result := fn.NewValue("result", types.TypeNumber)
	use.Instructions = append(use.Instructions, &ir.BinaryInst{Res: result, Op: ir.OpAdd, LHS: key, RHS: ir.ConstNumber{Value: 1}})
	use.Terminator = &ir.ReturnTerm{Val: result}

	ra := New(1)
	locs := ra.Allocate(fn)
	keyLoc, tmpLoc := locs[key.ID], locs[tmp.ID]
	if keyLoc.IsReg && tmpLoc.IsReg && keyLoc.Reg == tmpLoc.Reg {
		t.Fatalf("live-through key and bridge temporary share register %d", keyLoc.Reg)
	}
	if !keyLoc.IsReg && !tmpLoc.IsReg && keyLoc.StackSlot == tmpLoc.StackSlot {
		t.Fatalf("live-through key and bridge temporary share stack slot %d", keyLoc.StackSlot)
	}
}

func TestPhiResultsAtSameBlockEntryDoNotShareLocation(t *testing.T) {
	fn := ir.NewFunction("parallelPhiResults", types.TypeNumber)
	entry := fn.NewBlock("entry")
	left := fn.NewBlock("left")
	right := fn.NewBlock("right")
	join := fn.NewBlock("join")
	entry.Terminator = &ir.BranchTerm{Cond: ir.ConstBool{Value: true}, Then: left, Else: right}
	left.Terminator = &ir.JumpTerm{Target: join}
	right.Terminator = &ir.JumpTerm{Target: join}

	strPhi := fn.NewValue("strPhi", types.TypeString)
	lenPhi := fn.NewValue("lenPhi", types.TypeNumber)
	join.Phis = append(join.Phis,
		&ir.PhiInst{Res: strPhi, Incoming: []ir.PhiIncoming{{Block: left, Value: ir.ConstString{Value: "a"}}, {Block: right, Value: ir.ConstString{Value: "b"}}}},
		&ir.PhiInst{Res: lenPhi, Incoming: []ir.PhiIncoming{{Block: left, Value: ir.ConstNumber{Value: 1}}, {Block: right, Value: ir.ConstNumber{Value: 2}}}},
	)
	result := fn.NewValue("result", types.TypeNumber)
	join.Instructions = append(join.Instructions, &ir.BinaryInst{Res: result, Op: ir.OpAdd, LHS: lenPhi, RHS: ir.ConstNumber{Value: 1}})
	join.Terminator = &ir.ReturnTerm{Val: result}

	ra := New(2)
	locs := ra.Allocate(fn)
	a, b := locs[strPhi.ID], locs[lenPhi.ID]
	if a.IsReg && b.IsReg && a.Reg == b.Reg {
		t.Fatalf("simultaneous phi results share register %d", a.Reg)
	}
	if !a.IsReg && !b.IsReg && a.StackSlot == b.StackSlot {
		t.Fatalf("simultaneous phi results share stack slot %d", a.StackSlot)
	}
}
