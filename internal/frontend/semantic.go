package frontend

type ExprKind uint8

const (
	ExprInvalid ExprKind = iota
	ExprIdentifier
	ExprBoolean
	ExprNumber
	ExprString
	ExprNull
	ExprUndefined
	ExprBinary
	ExprCall
	ExprDynamicCall
	ExprDynamicMethodCall
	ExprArray
	ExprIndex
	ExprArrayLength
	ExprArrayPush
	ExprArrayPop
	ExprObject
	ExprFieldGet
	ExprDynamicFieldGet
	ExprDynamicIndexGet
	ExprClosure
	ExprNewClass
	ExprTaskSpawn
	ExprPromiseResolve
	ExprPromiseAdopt
	ExprPromiseThenable
	ExprPromiseReject
	ExprPromiseAll
	ExprPromiseRace
	ExprTaskJoin
	ExprTaskYield
	ExprTaskCancel
	ExprTaskCancelled
	ExprTaskGroupNew
	ExprTaskGroupJoin
	ExprTaskGroupCancel
	ExprTaskContextSet
	ExprTaskContextGet
	ExprChannelNew
	ExprChannelTrySend
	ExprChannelTryRecvOr
	ExprChannelSend
	ExprChannelRecv
	ExprSleep
	ExprTemplateLiteral
	ExprJSONStringify
	ExprJSONParse
	ExprArrayConcat
	ExprMapNew
	ExprMapGet
	ExprMapSet
	ExprMapHas
	ExprMapDelete
	ExprMapClear
	ExprMapSize
	ExprSetNew
	ExprSetAdd
	ExprSetHas
	ExprSetDelete
	ExprSetClear
	ExprSetSize
	ExprDateNow
	ExprDateNew
	ExprDateGetTime
	ExprDateToISOString
	ExprDateGetFullYear
	ExprDateGetMonth
	ExprDateGetDate
	ExprDateGetHours
	ExprDateGetMinutes
	ExprDateGetSeconds
	ExprRegExpNew
	ExprRegExpTest
	ExprRegExpSource
)

type IntrinsicKind uint8

const (
	IntrinsicNone IntrinsicKind = iota
	IntrinsicConsoleLogF64
	IntrinsicConsoleLogString
	IntrinsicConsoleLogJSValue
)

type BinaryOperator uint8

const (
	BinaryInvalid BinaryOperator = iota
	BinaryAdd
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
	BinaryNullishCoalesce
	BinaryLogicalOr
	BinaryLogicalAnd
)

type DispatchTarget struct {
	ClassTag uint32
	Function FunctionID
}

type ObjectFieldExpr struct {
	Name  string
	Index uint32
	Value *Expr
}

type Expr struct {
	Kind               ExprKind
	Type               TypeID
	Symbol             SymbolID
	Name               string
	Boolean            bool
	Number             float64
	String             string
	Operator           BinaryOperator
	Left               *Expr
	Right              *Expr
	Callee             *Expr
	Args               []*Expr
	CallTarget         *FunctionID
	Intrinsic          IntrinsicKind
	Elements           []*Expr
	Object             *Expr
	Index              *Expr
	Fields             []ObjectFieldExpr
	Captures           []*Expr
	Field              string
	FieldIndex         uint32
	Constructor        *FunctionID
	Dispatch           []DispatchTarget
	ThenResolveJSValue bool
	ThenRejectJSValue  bool
	ThenResolveReturn  TypeKind
	ThenRejectReturn   TypeKind
	ConcreteType       TypeID
	ConcreteKnown      bool
	TaskShared         bool
	Span               Span
}

type StmtKind uint8

const (
	StmtInvalid StmtKind = iota
	StmtBlock
	StmtIf
	StmtReturn
	StmtThrow
	StmtTry
	StmtExpr
	StmtVar
	StmtAssign
	StmtWhile
	StmtDoWhile
	StmtFor
	StmtClosureBind
	StmtFieldAssign
	StmtDynamicFieldAssign
	StmtDynamicIndexSet
	StmtArrayAssign
)

type Statement struct {
	Kind    StmtKind
	Span    Span
	Expr    *Expr
	Then    []Statement
	Else    []Statement
	Catch   []Statement
	Finally []Statement
	Return  *Expr

	Symbol      SymbolID
	CatchSymbol SymbolID
	CatchType   TypeID
	Name        string
	Type        TypeID
	Value       *Expr
	Object      *Expr
	Index       *Expr
	Field       string
	FieldIndex  uint32
	Init        []Statement
	Update      []Statement
}
