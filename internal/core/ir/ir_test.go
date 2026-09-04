package ir

import (
	"strings"
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func TestIRDump(t *testing.T) {
	prog := &Program{}
	fn := NewFunction("add", types.TypeNumber)
	v0 := fn.NewValue("a", types.TypeNumber)
	v1 := fn.NewValue("b", types.TypeNumber)
	fn.Params = []*Value{v0, v1}

	entry := fn.NewBlock("entry")
	v2 := fn.NewValue("sum", types.TypeNumber)
	v0.operandNode()
	entry.Phis = append(entry.Phis, &PhiInst{
		Res:      v0,
		Incoming: []PhiIncoming{{Block: entry, Value: v0}},
	})
	entry.Instructions = append(entry.Instructions, &BinaryInst{
		Res: v2,
		Op:  OpAdd,
		LHS: v0,
		RHS: v1,
	})
	entry.Terminator = &ReturnTerm{Val: v2}

	prog.Functions = append(prog.Functions, fn)

	dump := prog.Dump()
	if !strings.Contains(dump, "define @add(%a: number, %b: number): number") {
		t.Errorf("expected signature in dump, got:\n%s", dump)
	}
	if !strings.Contains(dump, "%sum = add %a, %b") {
		t.Errorf("expected add inst in dump, got:\n%s", dump)
	}
	if !strings.Contains(dump, "ret %sum") {
		t.Errorf("expected ret in dump, got:\n%s", dump)
	}
}

func TestIROperandsAndInstructions(t *testing.T) {
	cn := ConstNumber{Value: 3.14}
	cs := ConstString{Value: "foo"}
	cb := ConstBool{Value: true}
	cbf := ConstBool{Value: false}
	cnull := ConstNull{}
	cundef := ConstUndefined{}

	ops := []Operand{cn, cs, cb, cbf, cnull, cundef}
	for _, op := range ops {
		op.operandNode()
		if op.Type() == nil {
			t.Errorf("operand %T returned nil type", op)
		}
		if op.String() == "" {
			t.Errorf("operand %T returned empty string", op)
		}
	}

	fn := NewFunction("test", types.TypeVoid)
	bb := fn.NewBlock("bb")
	targetBB := fn.NewBlock("target")
	elseBB := fn.NewBlock("else")

	v := fn.NewValue("v", types.TypeNumber)
	vAnon := fn.NewValue("", types.TypeNumber)
	_ = vAnon.String()

	insts := []Instruction{
		&BinaryInst{Res: v, Op: OpSub, LHS: v, RHS: cn},
		&BinaryInst{Res: v, Op: OpMul, LHS: v, RHS: cn},
		&BinaryInst{Res: v, Op: OpDiv, LHS: v, RHS: cn},
		&BinaryInst{Res: v, Op: OpMod, LHS: v, RHS: cn},
		&BinaryInst{Res: v, Op: OpEq, LHS: v, RHS: cn},
		&BinaryInst{Res: v, Op: OpNe, LHS: v, RHS: cn},
		&BinaryInst{Res: v, Op: OpLt, LHS: v, RHS: cn},
		&BinaryInst{Res: v, Op: OpLe, LHS: v, RHS: cn},
		&BinaryInst{Res: v, Op: OpGt, LHS: v, RHS: cn},
		&BinaryInst{Res: v, Op: OpGe, LHS: v, RHS: cn},
		&BinaryInst{Res: v, Op: OpAnd, LHS: v, RHS: cn},
		&BinaryInst{Res: v, Op: OpOr, LHS: v, RHS: cn},
		&BinaryInst{Res: v, Op: BinaryOp(99), LHS: v, RHS: cn},
		&UnaryInst{Res: v, Op: "-", Val: v},
		&CallInst{Res: v, Callee: "callee", Args: []Operand{v, cn}},
		&CallInst{Res: nil, Callee: "voidCallee", Args: []Operand{}},
		&MakeClosureInst{Res: v, Function: "foo", Captures: []Operand{v}, RefMask: 0x1},
		&ClosureGetInst{Res: v, Closure: v, Index: 0},
		&IndirectCallInst{Res: v, Closure: v, ThisArg: v, Args: []Operand{cn}},
		&IndirectCallInst{Res: nil, Closure: v, ThisArg: nil, Args: []Operand{}},
		&AllocObjectInst{Res: v, Shape: "Point", FieldCount: 2, RefMask: 0},
		&GetFieldInst{Res: v, Obj: v, Field: "x", Offset: 16},
		&SetFieldInst{Obj: v, Field: "x", Offset: 16, Val: cn},
		&AllocArrayInst{Res: v, ElemType: types.TypeNumber, Length: cn},
		&GetElementInst{Res: v, Array: v, Index: cn},
		&SetElementInst{Array: v, Index: cn, Val: v},
		&ArrayLengthInst{Res: v, Array: v},
		&ArrayPushInst{Res: v, Array: v, Val: cn},
		&ArrayPopInst{Res: v, Array: v},
		&PhiInst{Res: v, Incoming: []PhiIncoming{{Block: bb, Value: cn}}},
	}

	for _, inst := range insts {
		inst.instructionNode()
		_ = inst.Result()
		if inst.String() == "" {
			t.Errorf("instruction %T returned empty string", inst)
		}
	}

	terms := []Terminator{
		&ReturnTerm{Val: nil},
		&ReturnTerm{Val: v},
		&BranchTerm{Cond: cb, Then: targetBB, Else: elseBB},
		&JumpTerm{Target: targetBB},
	}

	for _, term := range terms {
		term.terminatorNode()
		_ = term.Successors()
		if term.String() == "" {
			t.Errorf("terminator %T returned empty string", term)
		}
	}
}
