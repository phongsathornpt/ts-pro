package ir

import (
	"fmt"
	"strings"

	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

// Value represents a unique SSA register value.
type Value struct {
	ID      int
	Name    string
	ValType types.Type
}

func (v *Value) Type() types.Type {
	return v.ValType
}

func (v *Value) String() string {
	if v.Name != "" {
		return fmt.Sprintf("%%%s", v.Name)
	}
	return fmt.Sprintf("%%v%d", v.ID)
}

// Operand can be either a *Value or a constant literal.
type Operand interface {
	operandNode()
	Type() types.Type
	String() string
}

func (v *Value) operandNode() {}

type ConstNumber struct {
	Value float64
}

func (c ConstNumber) operandNode()     {}
func (c ConstNumber) Type() types.Type { return types.TypeNumber }
func (c ConstNumber) String() string   { return fmt.Sprintf("%.17g", c.Value) }

type ConstString struct {
	Value string
}

func (c ConstString) operandNode()     {}
func (c ConstString) Type() types.Type { return types.TypeString }
func (c ConstString) String() string   { return fmt.Sprintf("%q", c.Value) }

type ConstBool struct {
	Value bool
}

func (c ConstBool) operandNode()     {}
func (c ConstBool) Type() types.Type { return types.TypeBoolean }
func (c ConstBool) String() string {
	if c.Value {
		return "true"
	}
	return "false"
}

type ConstNull struct{}

func (ConstNull) operandNode()     {}
func (ConstNull) Type() types.Type { return types.TypeNull }
func (ConstNull) String() string   { return "null" }

type ConstUndefined struct{}

func (ConstUndefined) operandNode()     {}
func (ConstUndefined) Type() types.Type { return types.TypeUndefined }
func (ConstUndefined) String() string   { return "undefined" }

// Instruction is an SSA statement inside a basic block.
type Instruction interface {
	instructionNode()
	Result() *Value
	String() string
}

type (
	BinaryOp uint8

	BinaryInst struct {
		Res *Value
		Op  BinaryOp
		LHS Operand
		RHS Operand
	}

	UnaryInst struct {
		Res *Value
		Op  string // "-", "!"
		Val Operand
	}

	CallInst struct {
		Res        *Value
		Callee     string
		Args       []Operand
		ParamTypes []types.Type
	}

	MakeClosureInst struct {
		Res      *Value
		Function string
		Captures []Operand
		RefMask  uint64
	}

	ClosureGetInst struct {
		Res     *Value
		Closure Operand
		Index   int
	}

	IndirectCallInst struct {
		Res        *Value
		Closure    Operand
		ThisArg    Operand
		Args       []Operand
		ParamTypes []types.Type
	}

	AllocObjectInst struct {
		Res        *Value
		Shape      string
		FieldCount int
		RefMask    uint64
	}

	GetFieldInst struct {
		Res    *Value
		Obj    Operand
		Field  string
		Offset int
	}

	SetFieldInst struct {
		Obj    Operand
		Field  string
		Offset int
		Val    Operand
	}

	AllocArrayInst struct {
		Res      *Value
		ElemType types.Type
		Length   Operand
	}

	GetElementInst struct {
		Res   *Value
		Array Operand
		Index Operand
	}

	SetElementInst struct {
		Array Operand
		Index Operand
		Val   Operand
	}

	ArrayLengthInst struct {
		Res   *Value
		Array Operand
	}

	ArrayPushInst struct {
		Res   *Value
		Array Operand
		Val   Operand
	}

	ArrayPopInst struct {
		Res   *Value
		Array Operand
	}

	PhiIncoming struct {
		Block *BasicBlock
		Value Operand
	}

	PhiInst struct {
		Res      *Value
		Incoming []PhiIncoming
	}
)

const (
	OpAdd BinaryOp = iota
	OpSub
	OpMul
	OpDiv
	OpMod
	OpEq
	OpNe
	OpLt
	OpLe
	OpGt
	OpGe
	OpAnd
	OpOr
)

func (op BinaryOp) String() string {
	switch op {
	case OpAdd:
		return "add"
	case OpSub:
		return "sub"
	case OpMul:
		return "mul"
	case OpDiv:
		return "div"
	case OpMod:
		return "mod"
	case OpEq:
		return "eq"
	case OpNe:
		return "ne"
	case OpLt:
		return "lt"
	case OpLe:
		return "le"
	case OpGt:
		return "gt"
	case OpGe:
		return "ge"
	case OpAnd:
		return "and"
	case OpOr:
		return "or"
	default:
		return "op"
	}
}

func (i *BinaryInst) instructionNode() {}
func (i *BinaryInst) Result() *Value   { return i.Res }
func (i *BinaryInst) String() string   { return fmt.Sprintf("%s = %s %s, %s", i.Res, i.Op, i.LHS, i.RHS) }
func (i *UnaryInst) instructionNode()  {}
func (i *UnaryInst) Result() *Value    { return i.Res }
func (i *UnaryInst) String() string    { return fmt.Sprintf("%s = %s%s", i.Res, i.Op, i.Val) }
func (i *CallInst) instructionNode()   {}
func (i *CallInst) Result() *Value     { return i.Res }
func (i *CallInst) String() string {
	var args []string
	for _, a := range i.Args {
		args = append(args, a.String())
	}
	if i.Res != nil {
		return fmt.Sprintf("%s = call @%s(%s)", i.Res, i.Callee, strings.Join(args, ", "))
	}
	return fmt.Sprintf("call @%s(%s)", i.Callee, strings.Join(args, ", "))
}
func (i *MakeClosureInst) instructionNode() {}
func (i *MakeClosureInst) Result() *Value   { return i.Res }
func (i *MakeClosureInst) String() string {
	var caps []string
	for _, c := range i.Captures {
		caps = append(caps, c.String())
	}
	return fmt.Sprintf("%s = make_closure @%s refs=%#x [%s]", i.Res, i.Function, i.RefMask, strings.Join(caps, ", "))
}
func (i *ClosureGetInst) instructionNode() {}
func (i *ClosureGetInst) Result() *Value   { return i.Res }
func (i *ClosureGetInst) String() string {
	return fmt.Sprintf("%s = closure_get %s[%d]", i.Res, i.Closure, i.Index)
}
func (i *IndirectCallInst) instructionNode() {}
func (i *IndirectCallInst) Result() *Value   { return i.Res }
func (i *IndirectCallInst) String() string {
	var args []string
	for _, a := range i.Args {
		args = append(args, a.String())
	}
	if i.Res != nil {
		return fmt.Sprintf("%s = call_indirect %s(%s)", i.Res, i.Closure, strings.Join(args, ", "))
	}
	return fmt.Sprintf("call_indirect %s(%s)", i.Closure, strings.Join(args, ", "))
}
func (i *AllocObjectInst) instructionNode() {}
func (i *AllocObjectInst) Result() *Value   { return i.Res }
func (i *AllocObjectInst) String() string {
	return fmt.Sprintf("%s = alloc_obj %s fields=%d refs=%#x", i.Res, i.Shape, i.FieldCount, i.RefMask)
}
func (i *GetFieldInst) instructionNode() {}
func (i *GetFieldInst) Result() *Value   { return i.Res }
func (i *GetFieldInst) String() string {
	return fmt.Sprintf("%s = getfield %s.%s@%d", i.Res, i.Obj, i.Field, i.Offset)
}
func (i *SetFieldInst) instructionNode() {}
func (i *SetFieldInst) Result() *Value   { return nil }
func (i *SetFieldInst) String() string {
	return fmt.Sprintf("setfield %s.%s@%d = %s", i.Obj, i.Field, i.Offset, i.Val)
}
func (i *AllocArrayInst) instructionNode() {}
func (i *AllocArrayInst) Result() *Value   { return i.Res }
func (i *AllocArrayInst) String() string {
	return fmt.Sprintf("%s = alloc_array %s[%s]", i.Res, i.ElemType, i.Length)
}
func (i *GetElementInst) instructionNode() {}
func (i *GetElementInst) Result() *Value   { return i.Res }
func (i *GetElementInst) String() string {
	return fmt.Sprintf("%s = getelem %s[%s]", i.Res, i.Array, i.Index)
}
func (i *SetElementInst) instructionNode() {}
func (i *SetElementInst) Result() *Value   { return nil }
func (i *SetElementInst) String() string {
	return fmt.Sprintf("setelem %s[%s] = %s", i.Array, i.Index, i.Val)
}
func (i *ArrayLengthInst) instructionNode() {}
func (i *ArrayLengthInst) Result() *Value   { return i.Res }
func (i *ArrayLengthInst) String() string {
	return fmt.Sprintf("%s = array_len %s", i.Res, i.Array)
}
func (i *ArrayPushInst) instructionNode() {}
func (i *ArrayPushInst) Result() *Value   { return i.Res }
func (i *ArrayPushInst) String() string {
	return fmt.Sprintf("%s = array_push %s, %s", i.Res, i.Array, i.Val)
}
func (i *ArrayPopInst) instructionNode() {}
func (i *ArrayPopInst) Result() *Value   { return i.Res }
func (i *ArrayPopInst) String() string {
	return fmt.Sprintf("%s = array_pop %s", i.Res, i.Array)
}
func (i *PhiInst) instructionNode() {}
func (i *PhiInst) Result() *Value   { return i.Res }
func (i *PhiInst) String() string {
	var parts []string
	for _, inc := range i.Incoming {
		parts = append(parts, fmt.Sprintf("[%s, %%%s]", inc.Value, inc.Block.Name))
	}
	return fmt.Sprintf("%s = phi %s", i.Res, strings.Join(parts, ", "))
}

// Terminator represents control-flow exits from a basic block.
type Terminator interface {
	terminatorNode()
	String() string
	Successors() []*BasicBlock
}

type (
	ReturnTerm struct {
		Val Operand // may be nil for void return
	}

	BranchTerm struct {
		Cond Operand
		Then *BasicBlock
		Else *BasicBlock
	}

	JumpTerm struct {
		Target *BasicBlock
	}
)

func (t *ReturnTerm) terminatorNode()           {}
func (t *ReturnTerm) Successors() []*BasicBlock { return nil }
func (t *ReturnTerm) String() string {
	if t.Val != nil {
		return fmt.Sprintf("ret %s", t.Val)
	}
	return "ret"
}
func (t *BranchTerm) terminatorNode()           {}
func (t *BranchTerm) Successors() []*BasicBlock { return []*BasicBlock{t.Then, t.Else} }
func (t *BranchTerm) String() string {
	return fmt.Sprintf("br %s, label %%%s, label %%%s", t.Cond, t.Then.Name, t.Else.Name)
}
func (t *JumpTerm) terminatorNode()           {}
func (t *JumpTerm) Successors() []*BasicBlock { return []*BasicBlock{t.Target} }
func (t *JumpTerm) String() string            { return fmt.Sprintf("jmp label %%%s", t.Target.Name) }

// BasicBlock is a linear sequence of instructions ending with a terminator.
type BasicBlock struct {
	ID           int
	Name         string
	Phis         []*PhiInst
	Instructions []Instruction
	Terminator   Terminator
}

// Function represents a compiled function in IR.
type Function struct {
	Name        string
	Params      []*Value
	ReturnType  types.Type
	Blocks      []*BasicBlock
	nextValID   int
	nextBlockID int
}

func NewFunction(name string, returnType types.Type) *Function {
	return &Function{
		Name:       name,
		ReturnType: returnType,
	}
}

func (fn *Function) NewValue(name string, t types.Type) *Value {
	v := &Value{
		ID:      fn.nextValID,
		Name:    name,
		ValType: t,
	}
	fn.nextValID++
	return v
}

func (fn *Function) NewBlock(name string) *BasicBlock {
	bb := &BasicBlock{
		ID:   fn.nextBlockID,
		Name: fmt.Sprintf("%s_%d", name, fn.nextBlockID),
	}
	fn.nextBlockID++
	fn.Blocks = append(fn.Blocks, bb)
	return bb
}

// Program is the top-level IR module.
type Program struct {
	Functions []*Function
}

// Dump returns a formatted, human-readable IR textual representation.
func (p *Program) Dump() string {
	var sb strings.Builder
	for _, fn := range p.Functions {
		var paramStrs []string
		for _, param := range fn.Params {
			paramStrs = append(paramStrs, fmt.Sprintf("%s: %s", param, param.Type()))
		}
		sb.WriteString(fmt.Sprintf("define @%s(%s): %s {\n", fn.Name, strings.Join(paramStrs, ", "), fn.ReturnType))
		for _, b := range fn.Blocks {
			sb.WriteString(fmt.Sprintf("%%%s:\n", b.Name))
			for _, phi := range b.Phis {
				sb.WriteString(fmt.Sprintf("    %s\n", phi.String()))
			}
			for _, inst := range b.Instructions {
				sb.WriteString(fmt.Sprintf("    %s\n", inst.String()))
			}
			if b.Terminator != nil {
				sb.WriteString(fmt.Sprintf("    %s\n", b.Terminator.String()))
			}
		}
		sb.WriteString("}\n\n")
	}
	return sb.String()
}
