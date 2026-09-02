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

type BoxKind uint8

const (
	BoxInvalid BoxKind = iota
	BoxNumber
	BoxString
	BoxBoolean
	BoxObject
	BoxArray
	BoxFunction
)

type BoxOp struct {
	Kind        BoxKind
	Value       ValueID
	Shape       ShapeID
	Function    FunctionID
	HasFunction bool
}

type UnboxKind uint8

const (
	UnboxInvalid UnboxKind = iota
	UnboxNumber
	UnboxString
	UnboxBoolean
	UnboxArray
	UnboxObject
	UnboxFunction
)

type UnboxOp struct {
	Kind  UnboxKind
	Value ValueID
	Shape ShapeID
}

type DynamicBinaryOp struct {
	Operator BinaryOperator
	Left     ValueID
	Right    ValueID
}
type CallOp struct {
	Callee FunctionID
	Args   []ValueID
}
type DynamicCallOp struct {
	Callee ValueID
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
	IntrinsicConsoleLogJSValue
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

type ArrayElementKind uint8

const (
	ArrayElementInvalid ArrayElementKind = iota
	ArrayElementF64
	ArrayElementBool
	ArrayElementRef
)

type ArrayNewOp struct {
	Elements []ValueID
	Element  ArrayElementKind
}
type ArrayLengthOp struct {
	Array   ValueID
	Element ArrayElementKind
}
type ArrayGetOp struct {
	Array, Index ValueID
	Element      ArrayElementKind
}
type ArraySetOp struct {
	Array, Index, Value ValueID
	Element             ArrayElementKind
}
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
type DynamicFieldGetOp struct {
	Object ValueID
	Field  string
}
type DynamicFieldSetOp struct {
	Object ValueID
	Field  string
	Value  ValueID
}
type ClosureNewOp struct {
	Callee   FunctionID
	Captures []ValueID
}
type ClosureCallOp struct {
	Closure ValueID
	Args    []ValueID
}
type TaskSpawnOp struct {
	Callee   FunctionID
	Captures []ValueID
	Group    *ValueID
}
type TaskResultKind uint8

const (
	TaskResultInvalid TaskResultKind = iota
	TaskResultF64
	TaskResultBool
	TaskResultRef
)

type PromiseResolveOp struct {
	Value  ValueID
	Result TaskResultKind
}
type PromiseAdoptOp struct{ Promise ValueID }
type PromiseRejectOp struct {
	Reason ValueID
	Result TaskResultKind
}
type PromiseAllF64Op struct{ Promises []ValueID }
type PromiseRaceF64Op struct{ Promises []ValueID }
type PromiseAllBoolOp struct{ Promises []ValueID }
type PromiseRaceBoolOp struct{ Promises []ValueID }
type PromiseAllRefOp struct{ Promises []ValueID }
type PromiseRaceRefOp struct{ Promises []ValueID }
type TaskJoinOp struct {
	Task   ValueID
	Shared bool
}
type TaskWaitOp struct{ Task ValueID }
type TaskFailureOp struct{ Task ValueID }
type TaskRetainOp struct{ Task ValueID }
type TaskReleaseOp struct{ Task ValueID }
type TaskYieldOp struct{}
type TaskCancelOp struct{ Task ValueID }
type TaskCancelledOp struct{}
type TaskGroupNewOp struct{}
type TaskGroupJoinOp struct{ Group ValueID }
type TaskGroupCancelOp struct{ Group ValueID }
type TaskContextSetOp struct{ Value ValueID }
type TaskContextGetOp struct{}
type ChannelElementKind uint8

const (
	ChannelElementInvalid ChannelElementKind = iota
	ChannelElementF64
	ChannelElementBool
	ChannelElementRef
)

type ChannelNewOp struct {
	Capacity ValueID
	Element  ChannelElementKind
}
type ChannelTrySendOp struct {
	Channel, Value ValueID
	Element        ChannelElementKind
}
type ChannelTryRecvOrOp struct {
	Channel, Fallback ValueID
	Element           ChannelElementKind
}
type ChannelSendOp struct {
	Channel, Value ValueID
	Element        ChannelElementKind
}
type ChannelRecvOp struct {
	Channel ValueID
	Element ChannelElementKind
}
type SleepOp struct{ Duration ValueID }

func (ConstOp) isOperation()            {}
func (UnaryExpr) isOperation()          {}
func (BinaryExpr) isOperation()         {}
func (BoxOp) isOperation()              {}
func (UnboxOp) isOperation()            {}
func (DynamicBinaryOp) isOperation()    {}
func (CallOp) isOperation()             {}
func (DynamicCallOp) isOperation()      {}
func (DispatchCallOp) isOperation()     {}
func (IntrinsicCallOp) isOperation()    {}
func (PhiOp) isOperation()              {}
func (ArrayNewOp) isOperation()         {}
func (ArrayLengthOp) isOperation()      {}
func (ArrayGetOp) isOperation()         {}
func (ArraySetOp) isOperation()         {}
func (ObjectNewOp) isOperation()        {}
func (ObjectAllocOp) isOperation()      {}
func (FieldSetOp) isOperation()         {}
func (FieldGetOp) isOperation()         {}
func (DynamicFieldGetOp) isOperation()  {}
func (DynamicFieldSetOp) isOperation()  {}
func (ClosureNewOp) isOperation()       {}
func (ClosureCallOp) isOperation()      {}
func (TaskSpawnOp) isOperation()        {}
func (PromiseResolveOp) isOperation()   {}
func (PromiseAdoptOp) isOperation()     {}
func (PromiseRejectOp) isOperation()    {}
func (PromiseAllF64Op) isOperation()    {}
func (PromiseRaceF64Op) isOperation()   {}
func (PromiseAllBoolOp) isOperation()   {}
func (PromiseRaceBoolOp) isOperation()  {}
func (PromiseAllRefOp) isOperation()    {}
func (PromiseRaceRefOp) isOperation()   {}
func (TaskJoinOp) isOperation()         {}
func (TaskWaitOp) isOperation()         {}
func (TaskFailureOp) isOperation()      {}
func (TaskRetainOp) isOperation()       {}
func (TaskReleaseOp) isOperation()      {}
func (TaskYieldOp) isOperation()        {}
func (TaskCancelOp) isOperation()       {}
func (TaskCancelledOp) isOperation()    {}
func (TaskGroupNewOp) isOperation()     {}
func (TaskGroupJoinOp) isOperation()    {}
func (TaskGroupCancelOp) isOperation()  {}
func (TaskContextSetOp) isOperation()   {}
func (TaskContextGetOp) isOperation()   {}
func (ChannelNewOp) isOperation()       {}
func (ChannelTrySendOp) isOperation()   {}
func (ChannelTryRecvOrOp) isOperation() {}
func (ChannelSendOp) isOperation()      {}
func (ChannelRecvOp) isOperation()      {}
func (SleepOp) isOperation()            {}

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
	BinaryStrictEqual
	BinaryStrictNotEqual
)

type Terminator interface {
	isTerminator()
}

type ReturnTerm struct{ Value *ValueID }
type ThrowTerm struct{ Value ValueID }
type JumpTerm struct{ Target BlockID }
type BranchTerm struct {
	Condition ValueID
	Then      BlockID
	Else      BlockID
}

func (ReturnTerm) isTerminator() {}
func (ThrowTerm) isTerminator()  {}
func (JumpTerm) isTerminator()   {}
func (BranchTerm) isTerminator() {}
