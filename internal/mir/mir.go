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
	ReprTaskRef
	ReprChannelRef
	ReprTaskGroupRef
	ReprTagged
	ReprJSValue
)

type ShapeField struct {
	Name           string
	Repr           Repr
	ObjectShape    ShapeID
	HasObjectShape bool
}

type Shape struct {
	ID       ShapeID
	Name     string
	ClassTag uint32
	Fields   []ShapeField
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

type ConstBool struct{ Value bool }
type ConstF64 struct{ Value float64 }
type ConstString struct{ Value string }

type ConstJSKind uint8

const (
	ConstJSNull ConstJSKind = iota + 1
	ConstJSUndefined
)

type ConstJSValue struct{ Kind ConstJSKind }
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

type IntWidth uint8

const (
	IntWidth32 IntWidth = 32
	IntWidth64 IntWidth = 64
)

type ProvenIntBinary struct {
	Width    IntWidth
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

type BoxJSKind uint8

const (
	BoxJSInvalid BoxJSKind = iota
	BoxJSNumber
	BoxJSString
	BoxJSBoolean
	BoxJSObject
	BoxJSArray
	BoxJSFunction
)

type BoxJSValue struct {
	Kind  BoxJSKind
	Value ValueID
	Shape ShapeID
}

type UnboxJSKind uint8

const (
	UnboxJSInvalid UnboxJSKind = iota
	UnboxJSNumber
	UnboxJSString
	UnboxJSBoolean
	UnboxJSArray
)

type UnboxJSValue struct {
	Kind  UnboxJSKind
	Value ValueID
}

type DynamicAddJSValue struct {
	Left  ValueID
	Right ValueID
}

type DynamicJSBinaryOp uint8

const (
	DynamicJSInvalid DynamicJSBinaryOp = iota
	DynamicJSSub
	DynamicJSMul
	DynamicJSDiv
	DynamicJSLessThan
	DynamicJSLessEqual
	DynamicJSGreaterThan
	DynamicJSGreaterEqual
	DynamicJSEqual
	DynamicJSNotEqual
	DynamicJSStrictEqual
	DynamicJSStrictNotEqual
)

type DynamicBinaryJSValue struct {
	Operator DynamicJSBinaryOp
	Left     ValueID
	Right    ValueID
}

type Call struct {
	Callee FunctionID
	Args   []ValueID
}

type DispatchCase struct {
	ClassTag uint32
	Callee   FunctionID
}

type DispatchCall struct {
	Args  []ValueID
	Cases []DispatchCase
}

type Intrinsic uint8

const (
	IntrinsicInvalid Intrinsic = iota
	IntrinsicConsoleLogF64
	IntrinsicConsoleLogString
	IntrinsicConsoleLogJSValue
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
type ArraySetF64 struct{ Array, Index, Value ValueID }
type ObjectNew struct {
	Shape  ShapeID
	Fields []ValueID
}
type ObjectAlloc struct{ Shape ShapeID }
type FieldSet struct {
	Object ValueID
	Shape  ShapeID
	Field  uint32
	Value  ValueID
}
type FieldGet struct {
	Object ValueID
	Shape  ShapeID
	Field  uint32
}
type DynamicFieldGet struct {
	Object ValueID
	Field  string
}
type DynamicFieldSet struct {
	Object ValueID
	Field  string
	Value  ValueID
}
type ClosureNew struct {
	Callee   FunctionID
	Captures []ValueID
}
type ClosureCall struct {
	Closure ValueID
	Args    []ValueID
}
type TaskSpawn struct {
	Callee   FunctionID
	Captures []ValueID
	Group    *ValueID
}
type PromiseResolve struct {
	Value  ValueID
	Result Repr
}
type PromiseReject struct {
	Reason ValueID
	Result Repr
}
type TaskJoin struct{ Task ValueID }
type TaskWait struct{ Task ValueID }
type TaskFailure struct{ Task ValueID }
type TaskRelease struct{ Task ValueID }
type TaskYield struct{}
type TaskCancel struct{ Task ValueID }
type TaskCancelled struct{}
type TaskGroupNew struct{}
type TaskGroupJoin struct{ Group ValueID }
type TaskGroupCancel struct{ Group ValueID }
type TaskContextSet struct{ Value ValueID }
type TaskContextGet struct{}
type ChannelNewF64 struct{ Capacity ValueID }
type ChannelTrySendF64 struct{ Channel, Value ValueID }
type ChannelTryRecvOrF64 struct{ Channel, Fallback ValueID }
type ChannelSendF64 struct{ Channel, Value ValueID }
type ChannelRecvF64 struct{ Channel ValueID }
type ChannelNewBool struct{ Capacity ValueID }
type ChannelTrySendBool struct{ Channel, Value ValueID }
type ChannelTryRecvOrBool struct{ Channel, Fallback ValueID }
type ChannelSendBool struct{ Channel, Value ValueID }
type ChannelRecvBool struct{ Channel ValueID }
type ChannelNewRef struct{ Capacity ValueID }
type ChannelTrySendRef struct{ Channel, Value ValueID }
type ChannelTryRecvOrRef struct{ Channel, Fallback ValueID }
type ChannelSendRef struct{ Channel, Value ValueID }
type ChannelRecvRef struct{ Channel ValueID }
type Sleep struct{ Duration ValueID }

func (ConstJSValue) isOperation()         {}
func (ConstBool) isOperation()            {}
func (ConstF64) isOperation()             {}
func (ConstString) isOperation()          {}
func (StringConcat) isOperation()         {}
func (FloatBinary) isOperation()          {}
func (ProvenIntBinary) isOperation()      {}
func (FloatCompare) isOperation()         {}
func (Call) isOperation()                 {}
func (DispatchCall) isOperation()         {}
func (IntrinsicCall) isOperation()        {}
func (Phi) isOperation()                  {}
func (ArrayNewF64) isOperation()          {}
func (ArrayLengthF64) isOperation()       {}
func (ArrayGetF64) isOperation()          {}
func (ArraySetF64) isOperation()          {}
func (ObjectNew) isOperation()            {}
func (ObjectAlloc) isOperation()          {}
func (FieldSet) isOperation()             {}
func (FieldGet) isOperation()             {}
func (DynamicFieldGet) isOperation()      {}
func (DynamicFieldSet) isOperation()      {}
func (ClosureNew) isOperation()           {}
func (ClosureCall) isOperation()          {}
func (TaskSpawn) isOperation()            {}
func (PromiseResolve) isOperation()       {}
func (PromiseReject) isOperation()        {}
func (TaskJoin) isOperation()             {}
func (TaskWait) isOperation()             {}
func (TaskFailure) isOperation()          {}
func (TaskRelease) isOperation()          {}
func (TaskYield) isOperation()            {}
func (TaskCancel) isOperation()           {}
func (TaskCancelled) isOperation()        {}
func (TaskGroupNew) isOperation()         {}
func (TaskGroupJoin) isOperation()        {}
func (TaskGroupCancel) isOperation()      {}
func (TaskContextSet) isOperation()       {}
func (TaskContextGet) isOperation()       {}
func (ChannelNewF64) isOperation()        {}
func (ChannelTrySendF64) isOperation()    {}
func (ChannelTryRecvOrF64) isOperation()  {}
func (ChannelSendF64) isOperation()       {}
func (ChannelRecvF64) isOperation()       {}
func (ChannelNewBool) isOperation()       {}
func (ChannelTrySendBool) isOperation()   {}
func (ChannelTryRecvOrBool) isOperation() {}
func (ChannelSendBool) isOperation()      {}
func (ChannelRecvBool) isOperation()      {}
func (ChannelNewRef) isOperation()        {}
func (ChannelTrySendRef) isOperation()    {}
func (ChannelTryRecvOrRef) isOperation()  {}
func (ChannelSendRef) isOperation()       {}
func (ChannelRecvRef) isOperation()       {}
func (Sleep) isOperation()                {}
func (BoxJSValue) isOperation()           {}
func (UnboxJSValue) isOperation()         {}
func (DynamicAddJSValue) isOperation()    {}
func (DynamicBinaryJSValue) isOperation() {}

type Terminator interface{ isTerminator() }

type Return struct{ Value *ValueID }
type Throw struct{ Value ValueID }
type Jump struct{ Target BlockID }
type Branch struct {
	Condition ValueID
	Then      BlockID
	Else      BlockID
}

func (Return) isTerminator() {}
func (Throw) isTerminator()  {}
func (Jump) isTerminator()   {}
func (Branch) isTerminator() {}
