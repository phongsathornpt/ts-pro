package hir

type Module struct {
	ID        ModuleID
	Name      string
	Types     []SemanticType
	Shapes    []Shape
	Functions []Function
	Entry     *FunctionID
}

type Function struct {
	ID         FunctionID
	Name       string
	Params     []Param
	ReturnType TypeID
	ReturnRepr Repr
	Entry      BlockID
	Blocks     []Block
}

type Param struct {
	Value        ValueID
	Name         string
	SemanticType TypeID
	Repr         Repr
}

type Block struct {
	ID           BlockID
	Instructions []Instruction
	Terminator   Terminator
}

type Instruction struct {
	Result       ValueID
	SemanticType TypeID
	Repr         Repr
	Op           Operation
}
type Operation interface {
	isOperation()
}

type ConstOp struct{ Literal Literal }
type UnaryExpr struct {
	Operator UnaryOperator
	Operand  ValueID
}
type BinaryExpr struct {
	Operator BinaryOperator
	Left     ValueID
	Right    ValueID
}
type CallOp struct {
	Callee FunctionID
	Args   []ValueID
}

type DispatchCase struct {
	ClassTag uint32
	Callee   FunctionID
}

type DispatchCallOp struct {
	Args  []ValueID
	Cases []DispatchCase
}

type IntrinsicKind uint8

const (
	IntrinsicInvalid IntrinsicKind = iota
	IntrinsicConsoleLogF64
	IntrinsicConsoleLogString
)

type IntrinsicCallOp struct {
	Intrinsic IntrinsicKind
	Args      []ValueID
}

type PhiIncoming struct {
	Block BlockID
	Value ValueID
}

type PhiOp struct {
	Incoming []PhiIncoming
}

type ArrayNewOp struct{ Elements []ValueID }
type ArrayLengthOp struct{ Array ValueID }
type ArrayGetOp struct{ Array, Index ValueID }
type ObjectNewOp struct {
	Shape  ShapeID
	Fields []ValueID
}
type ObjectAllocOp struct{ Shape ShapeID }
type FieldSetOp struct {
	Object ValueID
	Shape  ShapeID
	Field  uint32
	Value  ValueID
}
type FieldGetOp struct {
	Object ValueID
	Shape  ShapeID
	Field  uint32
}
type ClosureNewOp struct {
	Callee   FunctionID
	Captures []ValueID
}
type ClosureCallOp struct {
	Closure ValueID
	Args    []ValueID
}

func (ConstOp) isOperation()         {}
func (UnaryExpr) isOperation()       {}
func (BinaryExpr) isOperation()      {}
func (CallOp) isOperation()          {}
func (DispatchCallOp) isOperation()  {}
func (IntrinsicCallOp) isOperation() {}
func (PhiOp) isOperation()           {}
func (ArrayNewOp) isOperation()      {}
func (ArrayLengthOp) isOperation()   {}
func (ArrayGetOp) isOperation()      {}
func (ObjectNewOp) isOperation()     {}
func (ObjectAllocOp) isOperation()   {}
func (FieldSetOp) isOperation()      {}
func (FieldGetOp) isOperation()      {}
func (ClosureNewOp) isOperation()    {}
func (ClosureCallOp) isOperation()   {}

type LiteralKind uint8

const (
	LiteralInvalid LiteralKind = iota
	LiteralBoolean
	LiteralNumber
	LiteralString
	LiteralNull
	LiteralUndefined
)

type Literal struct {
	Kind   LiteralKind
	Bool   bool
	Number float64
	String string
}
type UnaryOperator uint8

const (
	UnaryNegate UnaryOperator = iota + 1
	UnaryNot
)

type BinaryOperator uint8

const (
	BinaryAdd BinaryOperator = iota + 1
	BinarySub
	BinaryMul
	BinaryDiv
	BinaryLessThan
	BinaryLessEqual
	BinaryGreaterThan
	BinaryGreaterEqual
	BinaryEqual
	BinaryNotEqual
)

type Terminator interface {
	isTerminator()
}

type ReturnTerm struct{ Value *ValueID }
type JumpTerm struct{ Target BlockID }
type BranchTerm struct {
	Condition ValueID
	Then      BlockID
	Else      BlockID
}

func (ReturnTerm) isTerminator() {}
func (JumpTerm) isTerminator()   {}
func (BranchTerm) isTerminator() {}
