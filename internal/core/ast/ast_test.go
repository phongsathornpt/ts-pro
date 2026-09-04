package ast

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/core/token"
	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

func TestAllASTNodes(t *testing.T) {
	span := source.Span{Start: 1, End: 10}

	exprs := []Expr{
		&IdentExpr{SourceSpan: span, Name: "x"},
		&NumberLit{SourceSpan: span, Value: 42, Raw: "42"},
		&StringLit{SourceSpan: span, Value: "hello"},
		&RegexLit{SourceSpan: span, Pattern: "abc", Flags: "g"},
		&BoolLit{SourceSpan: span, Value: true},
		&NullLit{SourceSpan: span},
		&UndefinedLit{SourceSpan: span},
		&ThisExpr{SourceSpan: span},
		&SuperExpr{SourceSpan: span},
		&NewExpr{SourceSpan: span, ClassName: "Foo"},
		&BinaryExpr{SourceSpan: span, Left: &IdentExpr{SourceSpan: span, Name: "a"}, Op: token.Plus, Right: &NumberLit{SourceSpan: span, Value: 1}},
		&UnaryExpr{SourceSpan: span, Op: token.Minus, Target: &NumberLit{SourceSpan: span, Value: 1}},
		&AwaitExpr{SourceSpan: span, Target: &IdentExpr{SourceSpan: span, Name: "p"}},
		&CallExpr{SourceSpan: span, Callee: &IdentExpr{SourceSpan: span, Name: "fn"}},
		&MemberExpr{SourceSpan: span, Object: &IdentExpr{SourceSpan: span, Name: "obj"}, Property: "prop"},
		&IndexExpr{SourceSpan: span, Target: &IdentExpr{SourceSpan: span, Name: "arr"}, Index: &NumberLit{SourceSpan: span, Value: 0}},
		&ArrayLit{SourceSpan: span},
		&SpreadExpr{SourceSpan: span, Value: &IdentExpr{SourceSpan: span, Name: "rest"}},
		&ObjectLit{SourceSpan: span},
		&ArrowFuncExpr{SourceSpan: span, Body: &BlockStmt{SourceSpan: span}},
		&FunctionExpr{SourceSpan: span, Body: &BlockStmt{SourceSpan: span}},
		&AssignExpr{SourceSpan: span, Left: &IdentExpr{SourceSpan: span, Name: "x"}, Right: &NumberLit{SourceSpan: span, Value: 2}},
		&TernaryExpr{SourceSpan: span, Cond: &BoolLit{SourceSpan: span, Value: true}, Then: &NumberLit{SourceSpan: span, Value: 1}, Else: &NumberLit{SourceSpan: span, Value: 2}},
	}

	for i, e := range exprs {
		if e.Span() != span {
			t.Errorf("expr %d (%T) Span() = %v, want %v", i, e, e.Span(), span)
		}
		e.exprNode()
	}

	stmts := []Stmt{
		&VarDeclStmt{SourceSpan: span, Kind: token.KwLet},
		&BlockStmt{SourceSpan: span},
		&ExprStmt{SourceSpan: span, Expr: &NumberLit{SourceSpan: span, Value: 1}},
		&IfStmt{SourceSpan: span, Cond: &BoolLit{SourceSpan: span, Value: true}, Then: &BlockStmt{SourceSpan: span}},
		&WhileStmt{SourceSpan: span, Cond: &BoolLit{SourceSpan: span, Value: true}, Body: &BlockStmt{SourceSpan: span}},
		&DoWhileStmt{SourceSpan: span, Body: &BlockStmt{SourceSpan: span}, Cond: &BoolLit{SourceSpan: span, Value: true}},
		&ForStmt{SourceSpan: span, Body: &BlockStmt{SourceSpan: span}},
		&ForOfStmt{SourceSpan: span, Name: "item", Iterable: &IdentExpr{SourceSpan: span, Name: "items"}, Body: &BlockStmt{SourceSpan: span}},
		&SwitchStmt{SourceSpan: span, Expr: &IdentExpr{SourceSpan: span, Name: "x"}},
		&ReturnStmt{SourceSpan: span},
		&ThrowStmt{SourceSpan: span, Value: &StringLit{SourceSpan: span, Value: "err"}},
		&TryStmt{SourceSpan: span, Try: &BlockStmt{SourceSpan: span}},
		&BreakStmt{SourceSpan: span},
		&ContinueStmt{SourceSpan: span},
	}

	for i, s := range stmts {
		if s.Span() != span {
			t.Errorf("stmt %d (%T) Span() = %v, want %v", i, s, s.Span(), span)
		}
		s.stmtNode()
	}

	decls := []Decl{
		&ImportDecl{SourceSpan: span, Module: "./mod"},
		&FunctionDecl{SourceSpan: span, Name: "foo"},
		&ClassDecl{SourceSpan: span, Name: "Bar"},
		&EnumDecl{SourceSpan: span, Name: "Color"},
		&InterfaceDecl{SourceSpan: span, Name: "IFoo"},
		&TypeAliasDecl{SourceSpan: span, Name: "Alias"},
	}

	for i, d := range decls {
		if d.Span() != span {
			t.Errorf("decl %d (%T) Span() = %v, want %v", i, d, d.Span(), span)
		}
		d.stmtNode()
		d.declNode()
	}

	types := []TypeNode{
		&PrimitiveTypeNode{SourceSpan: span, Kind: "number"},
		&TypeRefNode{SourceSpan: span, Name: "MyType"},
		&UnionTypeNode{SourceSpan: span},
		&ArrayTypeNode{SourceSpan: span, ElemType: &PrimitiveTypeNode{SourceSpan: span, Kind: "string"}},
		&TupleTypeNode{SourceSpan: span},
		&ObjectTypeNode{SourceSpan: span},
		&FunctionTypeNode{SourceSpan: span, ReturnType: &PrimitiveTypeNode{SourceSpan: span, Kind: "void"}},
	}

	for i, tn := range types {
		if tn.Span() != span {
			t.Errorf("type %d (%T) Span() = %v, want %v", i, tn, tn.Span(), span)
		}
		tn.typeNode()
	}

	prog := &Program{SourceSpan: span}
	if prog.Span() != span {
		t.Errorf("prog.Span() = %v, want %v", prog.Span(), span)
	}
}
