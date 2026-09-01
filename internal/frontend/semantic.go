package frontend

type ExprKind uint8

const (
	ExprInvalid ExprKind = iota
	ExprIdentifier
	ExprNumber
	ExprString
	ExprBinary
	ExprCall
	ExprArray
	ExprIndex
	ExprArrayLength
	ExprObject
	ExprFieldGet
	ExprClosure
	ExprNewClass
	ExprTaskSpawn
	ExprTaskJoin
	ExprTaskYield
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
	Kind          ExprKind
	Type          TypeID
	Symbol        SymbolID
	Name          string
	Number        float64
	String        string
	Operator      BinaryOperator
	Left          *Expr
	Right         *Expr
	Callee        *Expr
	Args          []*Expr
	CallTarget    *FunctionID
	Intrinsic     IntrinsicKind
	Elements      []*Expr
	Object        *Expr
	Index         *Expr
	Fields        []ObjectFieldExpr
	Captures      []*Expr
	Field         string
	FieldIndex    uint32
	Constructor   *FunctionID
	Dispatch      []DispatchTarget
	ConcreteType  TypeID
	ConcreteKnown bool
	Span          Span
}

type StmtKind uint8

const (
	StmtInvalid StmtKind = iota
	StmtBlock
	StmtIf
	StmtReturn
	StmtExpr
	StmtVar
	StmtAssign
	StmtWhile
	StmtFor
	StmtClosureBind
	StmtFieldAssign
	StmtArrayAssign
)

type Statement struct {
	Kind   StmtKind
	Span   Span
	Expr   *Expr
	Then   []Statement
	Else   []Statement
	Return *Expr

	Symbol     SymbolID
	Name       string
	Type       TypeID
	Value      *Expr
	Object     *Expr
	Index      *Expr
	Field      string
	FieldIndex uint32
	Init       []Statement
	Update     []Statement
}
