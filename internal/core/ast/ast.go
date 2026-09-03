package ast

import (
	"github.com/phongsathornpt/ts-pro/internal/core/token"
	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

// Node is the common interface for all nodes in the Abstract Syntax Tree.
type Node interface {
	Span() source.Span
}

// Expr represents an expression node.
type Expr interface {
	Node
	exprNode()
}

// Stmt represents a statement node.
type Stmt interface {
	Node
	stmtNode()
}

// Decl represents a declaration node.
type Decl interface {
	Stmt
	declNode()
}

// TypeNode represents a static type annotation node.
type TypeNode interface {
	Node
	typeNode()
}

// --- Program ---

type Program struct {
	SourceSpan source.Span
	Statements []Stmt
}

func (p *Program) Span() source.Span { return p.SourceSpan }

// --- Expressions ---

type (
	IdentExpr struct {
		SourceSpan source.Span
		Name       string
	}

	NumberLit struct {
		SourceSpan source.Span
		Value      float64
		Raw        string
	}

	StringLit struct {
		SourceSpan source.Span
		Value      string
	}

	BoolLit struct {
		SourceSpan source.Span
		Value      bool
	}

	NullLit struct {
		SourceSpan source.Span
	}

	UndefinedLit struct {
		SourceSpan source.Span
	}

	ThisExpr struct {
		SourceSpan source.Span
	}

	SuperExpr struct {
		SourceSpan source.Span
	}

	NewExpr struct {
		SourceSpan source.Span
		ClassName  string
		TypeArgs   []TypeNode
		Args       []Expr
	}

	BinaryExpr struct {
		SourceSpan source.Span
		Left       Expr
		Op         token.Kind
		Right      Expr
	}

	UnaryExpr struct {
		SourceSpan source.Span
		Op         token.Kind
		Target     Expr
		Prefix     bool
	}

	CallExpr struct {
		SourceSpan source.Span
		Callee     Expr
		TypeArgs   []TypeNode
		Args       []Expr
	}

	MemberExpr struct {
		SourceSpan source.Span
		Object     Expr
		Property   string
		Computed   bool
		Optional   bool
	}

	IndexExpr struct {
		SourceSpan source.Span
		Target     Expr
		Index      Expr
	}

	ArrayLit struct {
		SourceSpan source.Span
		Elements   []Expr
	}

	SpreadExpr struct {
		SourceSpan source.Span
		Value      Expr
	}

	PropertyAssignment struct {
		SourceSpan source.Span
		Key        string
		Value      Expr
		Spread     bool
	}

	ObjectLit struct {
		SourceSpan source.Span
		Properties []PropertyAssignment
	}

	ArrowFuncExpr struct {
		SourceSpan source.Span
		Params     []Param
		ReturnType TypeNode
		Body       Node // BlockStmt or Expr
		IsExprBody bool
	}

	AssignExpr struct {
		SourceSpan source.Span
		Left       Expr
		Op         token.Kind // token.Eq, token.PlusEq, etc.
		Right      Expr
	}

	TernaryExpr struct {
		SourceSpan source.Span
		Cond       Expr
		Then       Expr
		Else       Expr
	}
)

func (e *IdentExpr) Span() source.Span     { return e.SourceSpan }
func (e *IdentExpr) exprNode()             {}
func (e *NumberLit) Span() source.Span     { return e.SourceSpan }
func (e *NumberLit) exprNode()             {}
func (e *StringLit) Span() source.Span     { return e.SourceSpan }
func (e *StringLit) exprNode()             {}
func (e *BoolLit) Span() source.Span       { return e.SourceSpan }
func (e *BoolLit) exprNode()               {}
func (e *NullLit) Span() source.Span       { return e.SourceSpan }
func (e *NullLit) exprNode()               {}
func (e *UndefinedLit) Span() source.Span  { return e.SourceSpan }
func (e *UndefinedLit) exprNode()          {}
func (e *ThisExpr) Span() source.Span      { return e.SourceSpan }
func (e *ThisExpr) exprNode()              {}
func (e *SuperExpr) Span() source.Span     { return e.SourceSpan }
func (e *SuperExpr) exprNode()             {}
func (e *NewExpr) Span() source.Span       { return e.SourceSpan }
func (e *NewExpr) exprNode()               {}
func (e *BinaryExpr) Span() source.Span    { return e.SourceSpan }
func (e *BinaryExpr) exprNode()            {}
func (e *UnaryExpr) Span() source.Span     { return e.SourceSpan }
func (e *UnaryExpr) exprNode()             {}
func (e *CallExpr) Span() source.Span      { return e.SourceSpan }
func (e *CallExpr) exprNode()              {}
func (e *MemberExpr) Span() source.Span    { return e.SourceSpan }
func (e *MemberExpr) exprNode()            {}
func (e *IndexExpr) Span() source.Span     { return e.SourceSpan }
func (e *IndexExpr) exprNode()             {}
func (e *ArrayLit) Span() source.Span      { return e.SourceSpan }
func (e *ArrayLit) exprNode()              {}
func (e *SpreadExpr) Span() source.Span    { return e.SourceSpan }
func (e *SpreadExpr) exprNode()            {}
func (e *ObjectLit) Span() source.Span     { return e.SourceSpan }
func (e *ObjectLit) exprNode()             {}
func (e *ArrowFuncExpr) Span() source.Span { return e.SourceSpan }
func (e *ArrowFuncExpr) exprNode()         {}
func (e *AssignExpr) Span() source.Span    { return e.SourceSpan }
func (e *AssignExpr) exprNode()            {}
func (e *TernaryExpr) Span() source.Span   { return e.SourceSpan }
func (e *TernaryExpr) exprNode()           {}

// --- Statements & Declarations ---

type Param struct {
	SourceSpan          source.Span
	Name                string
	Type                TypeNode
	Optional            bool
	Rest                bool
	Default             Expr
	Visibility          string
	Readonly            bool
	IsParameterProperty bool
}

type VarDeclarator struct {
	SourceSpan source.Span
	Name       string
	Type       TypeNode
	Init       Expr
}

type (
	VarDeclStmt struct {
		SourceSpan   source.Span
		Kind         token.Kind // KwLet, KwConst, KwVar
		Declarations []VarDeclarator
	}

	ImportSpecifier struct {
		Imported string
		Local    string
	}

	ImportDecl struct {
		SourceSpan source.Span
		Module     string
		Specifiers []ImportSpecifier
	}

	FunctionDecl struct {
		SourceSpan source.Span
		Name       string
		TypeParams []string
		Params     []Param
		ReturnType TypeNode
		Body       *BlockStmt
	}

	ClassField struct {
		SourceSpan source.Span
		Name       string
		Type       TypeNode
		Init       Expr
		IsStatic   bool
		Visibility string
		Readonly   bool
	}

	ClassMethod struct {
		SourceSpan source.Span
		Name       string
		Params     []Param
		ReturnType TypeNode
		Body       *BlockStmt
		IsStatic   bool
		IsOverride bool
		Visibility string
	}

	ClassDecl struct {
		SourceSpan source.Span
		Name       string
		TypeParams []string
		Extends    string
		Fields     []ClassField
		Methods    []ClassMethod
	}

	EnumMember struct {
		SourceSpan source.Span
		Name       string
		Value      Expr
	}

	EnumDecl struct {
		SourceSpan source.Span
		Name       string
		Members    []EnumMember
	}

	InterfaceField struct {
		SourceSpan source.Span
		Name       string
		Type       TypeNode
		Optional   bool
	}

	InterfaceDecl struct {
		SourceSpan source.Span
		Name       string
		TypeParams []string
		Extends    []string
		Fields     []InterfaceField
	}

	TypeAliasDecl struct {
		SourceSpan source.Span
		Name       string
		TypeParams []string
		Type       TypeNode
	}

	BlockStmt struct {
		SourceSpan source.Span
		Statements []Stmt
	}

	ExprStmt struct {
		SourceSpan source.Span
		Expr       Expr
	}

	IfStmt struct {
		SourceSpan source.Span
		Cond       Expr
		Then       Stmt
		Else       Stmt
	}

	WhileStmt struct {
		SourceSpan source.Span
		Cond       Expr
		Body       Stmt
	}

	DoWhileStmt struct {
		SourceSpan source.Span
		Body       Stmt
		Cond       Expr
	}

	ForStmt struct {
		SourceSpan source.Span
		Init       Stmt
		Cond       Expr
		Post       Expr
		Body       Stmt
	}

	ForOfStmt struct {
		SourceSpan source.Span
		Kind       token.Kind
		Name       string
		Type       TypeNode
		Iterable   Expr
		Body       Stmt
	}

	SwitchCase struct {
		SourceSpan source.Span
		Test       Expr // nil for default
		Statements []Stmt
	}

	SwitchStmt struct {
		SourceSpan source.Span
		Expr       Expr
		Cases      []SwitchCase
	}

	ReturnStmt struct {
		SourceSpan source.Span
		Value      Expr
	}

	BreakStmt struct {
		SourceSpan source.Span
	}

	ContinueStmt struct {
		SourceSpan source.Span
	}
)

func (s *VarDeclStmt) Span() source.Span   { return s.SourceSpan }
func (s *VarDeclStmt) stmtNode()           {}
func (s *VarDeclStmt) declNode()           {}
func (s *ImportDecl) Span() source.Span    { return s.SourceSpan }
func (s *ImportDecl) stmtNode()            {}
func (s *ImportDecl) declNode()            {}
func (s *FunctionDecl) Span() source.Span  { return s.SourceSpan }
func (s *FunctionDecl) stmtNode()          {}
func (s *FunctionDecl) declNode()          {}
func (s *ClassDecl) Span() source.Span     { return s.SourceSpan }
func (s *ClassDecl) stmtNode()             {}
func (s *ClassDecl) declNode()             {}
func (s *EnumDecl) Span() source.Span      { return s.SourceSpan }
func (s *EnumDecl) stmtNode()              {}
func (s *EnumDecl) declNode()              {}
func (s *InterfaceDecl) Span() source.Span { return s.SourceSpan }
func (s *InterfaceDecl) stmtNode()         {}
func (s *InterfaceDecl) declNode()         {}
func (s *TypeAliasDecl) Span() source.Span { return s.SourceSpan }
func (s *TypeAliasDecl) stmtNode()         {}
func (s *TypeAliasDecl) declNode()         {}
func (s *BlockStmt) Span() source.Span     { return s.SourceSpan }
func (s *BlockStmt) stmtNode()             {}
func (s *ExprStmt) Span() source.Span      { return s.SourceSpan }
func (s *ExprStmt) stmtNode()              {}
func (s *IfStmt) Span() source.Span        { return s.SourceSpan }
func (s *IfStmt) stmtNode()                {}
func (s *WhileStmt) Span() source.Span     { return s.SourceSpan }
func (s *WhileStmt) stmtNode()             {}
func (s *DoWhileStmt) Span() source.Span   { return s.SourceSpan }
func (s *DoWhileStmt) stmtNode()           {}
func (s *ForStmt) Span() source.Span       { return s.SourceSpan }
func (s *ForStmt) stmtNode()               {}
func (s *ForOfStmt) Span() source.Span     { return s.SourceSpan }
func (s *ForOfStmt) stmtNode()             {}
func (s *SwitchStmt) Span() source.Span    { return s.SourceSpan }
func (s *SwitchStmt) stmtNode()            {}
func (s *ReturnStmt) Span() source.Span    { return s.SourceSpan }
func (s *ReturnStmt) stmtNode()            {}
func (s *BreakStmt) Span() source.Span     { return s.SourceSpan }
func (s *BreakStmt) stmtNode()             {}
func (s *ContinueStmt) Span() source.Span  { return s.SourceSpan }
func (s *ContinueStmt) stmtNode()          {}

// --- Type Nodes ---

type (
	PrimitiveTypeNode struct {
		SourceSpan source.Span
		Kind       string // "number", "string", "boolean", "void", "any", "never", "unknown"
	}

	TypeRefNode struct {
		SourceSpan source.Span
		Name       string
		TypeArgs   []TypeNode
	}

	UnionTypeNode struct {
		SourceSpan source.Span
		Types      []TypeNode
	}

	ArrayTypeNode struct {
		SourceSpan source.Span
		ElemType   TypeNode
	}

	TupleTypeNode struct {
		SourceSpan source.Span
		Elements   []TypeNode
	}

	ObjectTypeNode struct {
		SourceSpan source.Span
		Fields     []InterfaceField
	}

	FunctionTypeNode struct {
		SourceSpan source.Span
		Params     []Param
		ReturnType TypeNode
	}
)

func (t *PrimitiveTypeNode) Span() source.Span { return t.SourceSpan }
func (t *PrimitiveTypeNode) typeNode()         {}
func (t *TypeRefNode) Span() source.Span       { return t.SourceSpan }
func (t *TypeRefNode) typeNode()               {}
func (t *UnionTypeNode) Span() source.Span     { return t.SourceSpan }
func (t *UnionTypeNode) typeNode()             {}
func (t *ArrayTypeNode) Span() source.Span     { return t.SourceSpan }
func (t *ArrayTypeNode) typeNode()             {}
func (t *TupleTypeNode) Span() source.Span     { return t.SourceSpan }
func (t *TupleTypeNode) typeNode()             {}
func (t *ObjectTypeNode) Span() source.Span    { return t.SourceSpan }
func (t *ObjectTypeNode) typeNode()            {}
func (t *FunctionTypeNode) Span() source.Span  { return t.SourceSpan }
func (t *FunctionTypeNode) typeNode()          {}
