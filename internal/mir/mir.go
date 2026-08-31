package mir

type FunctionID uint32
type BlockID uint32
type ValueID uint32
type ShapeID uint32

type Repr uint8

const (
	ReprInvalid Repr = iota
	ReprVoid
	ReprBool
	ReprI32
	ReprI64
	ReprF64
	ReprStringRef
	ReprArrayRef
	ReprObjectRef
	ReprFunctionRef
	ReprTagged
	ReprJSValue
)

type ShapeField struct {
	Name string
	Repr Repr
}

type Shape struct {
	ID     ShapeID
	Name   string
	Fields []ShapeField
}

type Module struct {
	Name      string
	Shapes    []Shape
	Functions []Function
	Entry     *FunctionID
}

type Function struct {
	ID         FunctionID
	Name       string
	Params     []Param
	ReturnRepr Repr
	Entry      BlockID
	Blocks     []Block
}

type Param struct {
	Value ValueID
	Name  string
	Repr  Repr
}

type Block struct {
	ID           BlockID
	Instructions []Instruction
	Terminator   Terminator
}

type Instruction struct {
	Result ValueID
	Repr   Repr
	Op     Operation
}

type Operation interface{ isOperation() }

type ConstF64 struct{ Value float64 }
type ConstString struct{ Value string }
type StringConcat struct{ Left, Right ValueID }

type FloatBinaryOp uint8

const (
	FloatAdd FloatBinaryOp = iota + 1
	FloatSub
	FloatMul
	FloatDiv
)

type FloatBinary struct {
	Operator FloatBinaryOp
	Left     ValueID
	Right    ValueID
}

type FloatCompareOp uint8

const (
	FloatLessThan FloatCompareOp = iota + 1
	FloatLessEqual
	FloatGreaterThan
	FloatGreaterEqual
	FloatEqual
	FloatNotEqual
)

type FloatCompare struct {
	Operator FloatCompareOp
	Left     ValueID
	Right    ValueID
}

type Call struct {
	Callee FunctionID
	Args   []ValueID
}

type Intrinsic uint8

const (
	IntrinsicInvalid Intrinsic = iota
	IntrinsicConsoleLogF64
	IntrinsicConsoleLogString
)

type IntrinsicCall struct {
	Intrinsic Intrinsic
	Args      []ValueID
}

type PhiIncoming struct {
	Block BlockID
	Value ValueID
}

type Phi struct {
	Incoming []PhiIncoming
}

type ArrayNewF64 struct{ Elements []ValueID }
type ArrayLengthF64 struct{ Array ValueID }
type ArrayGetF64 struct{ Array, Index ValueID }
type ObjectNew struct {
	Shape  ShapeID
	Fields []ValueID
}
type FieldGet struct {
	Object ValueID
	Shape  ShapeID
	Field  uint32
}

func (ConstF64) isOperation()       {}
func (ConstString) isOperation()    {}
func (StringConcat) isOperation()   {}
func (FloatBinary) isOperation()    {}
func (FloatCompare) isOperation()   {}
func (Call) isOperation()           {}
func (IntrinsicCall) isOperation()  {}
func (Phi) isOperation()            {}
func (ArrayNewF64) isOperation()    {}
func (ArrayLengthF64) isOperation() {}
func (ArrayGetF64) isOperation()    {}
func (ObjectNew) isOperation()      {}
func (FieldGet) isOperation()       {}

type Terminator interface{ isTerminator() }

type Return struct{ Value *ValueID }
type Jump struct{ Target BlockID }
type Branch struct {
	Condition ValueID
	Then      BlockID
	Else      BlockID
}

func (Return) isTerminator() {}
func (Jump) isTerminator()   {}
func (Branch) isTerminator() {}
