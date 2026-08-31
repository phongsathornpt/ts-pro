package mir

type FunctionID uint32
type BlockID uint32
type ValueID uint32

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

type Module struct {
	Name      string
	Functions []Function
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
	FloatLessEqual FloatCompareOp = iota + 1
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

func (ConstF64) isOperation()     {}
func (FloatBinary) isOperation()  {}
func (FloatCompare) isOperation() {}
func (Call) isOperation()         {}

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
