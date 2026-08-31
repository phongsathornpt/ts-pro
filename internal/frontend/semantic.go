package frontend

type ExprKind uint8

const (
	ExprInvalid ExprKind = iota
	ExprIdentifier
	ExprNumber
	ExprBinary
	ExprCall
)

type IntrinsicKind uint8

const (
	IntrinsicNone IntrinsicKind = iota
	IntrinsicConsoleLogF64
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

type Expr struct {
	Kind       ExprKind
	Type       TypeID
	Symbol     SymbolID
	Name       string
	Number     float64
	Operator   BinaryOperator
	Left       *Expr
	Right      *Expr
	Callee     *Expr
	Args       []*Expr
	CallTarget *FunctionID
	Intrinsic  IntrinsicKind
	Span       Span
}

type StmtKind uint8

const (
	StmtInvalid StmtKind = iota
	StmtBlock
	StmtIf
	StmtReturn
	StmtExpr
)

type Statement struct {
	Kind   StmtKind
	Span   Span
	Expr   *Expr
	Then   []Statement
	Else   []Statement
	Return *Expr
}
