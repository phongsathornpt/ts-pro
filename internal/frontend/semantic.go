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
)

type IntrinsicKind uint8

const (
	IntrinsicNone IntrinsicKind = iota
	IntrinsicConsoleLogF64
	IntrinsicConsoleLogString
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

type ObjectFieldExpr struct {
	Name  string
	Index uint32
	Value *Expr
}

type Expr struct {
	Kind       ExprKind
	Type       TypeID
	Symbol     SymbolID
	Name       string
	Number     float64
	String     string
	Operator   BinaryOperator
	Left       *Expr
	Right      *Expr
	Callee     *Expr
	Args       []*Expr
	CallTarget *FunctionID
	Intrinsic  IntrinsicKind
	Elements   []*Expr
	Object     *Expr
	Index      *Expr
	Fields     []ObjectFieldExpr
	Field      string
	FieldIndex uint32
	Span       Span
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
)

type Statement struct {
	Kind   StmtKind
	Span   Span
	Expr   *Expr
	Then   []Statement
	Else   []Statement
	Return *Expr

	Symbol SymbolID
	Name   string
	Type   TypeID
	Value  *Expr
	Init   []Statement
	Update []Statement
}
