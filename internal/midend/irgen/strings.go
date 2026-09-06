package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/token"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) canFuseStringConcat(expr ast.Expr) bool {
	bin, ok := expr.(*ast.BinaryExpr)
	if !ok || bin.Op != token.Plus || g.semanticType(bin) != types.TypeString {
		return false
	}
	// Dynamic JSValue addition owns ToPrimitive/addition semantics and must stay
	// on ts_js_add rather than being flattened into native string concatenation.
	return !irJSValueType(g.semanticType(bin.Left)) && !irJSValueType(g.semanticType(bin.Right))
}

func (g *generator) collectStringConcatParts(expr ast.Expr, parts *[]ast.Expr) {
	if g.canFuseStringConcat(expr) {
		bin := expr.(*ast.BinaryExpr)
		g.collectStringConcatParts(bin.Left, parts)
		g.collectStringConcatParts(bin.Right, parts)
		return
	}
	*parts = append(*parts, expr)
}

func (g *generator) lowerStringConcatChain(expr *ast.BinaryExpr) (ir.Operand, bool) {
	var parts []ast.Expr
	g.collectStringConcatParts(expr, &parts)
	if len(parts) < 3 {
		return nil, false
	}
	ops := make([]ir.Operand, 0, len(parts))
	for _, part := range parts {
		op := g.lowerExpr(part)
		ops = append(ops, g.coerceStringOperand(part, op))
	}
	emit := func(args []ir.Operand) ir.Operand {
		callee := "ts_string_concat"
		switch len(args) {
		case 3:
			callee = "ts_string_concat3"
		case 4:
			callee = "ts_string_concat4"
		}
		res := g.currentFn.NewValue("str", types.TypeString)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: callee, Args: args})
		return res
	}
	if len(ops) <= 4 {
		return emit(ops), true
	}
	current := emit(ops[:4])
	for i := 4; i < len(ops); {
		n := len(ops) - i
		if n > 3 {
			n = 3
		}
		args := make([]ir.Operand, 1, n+1)
		args[0] = current
		args = append(args, ops[i:i+n]...)
		current = emit(args)
		i += n
	}
	return current, true
}

func (g *generator) ownedStringAppendCandidate(s *ast.ForStmt) (string, ast.Expr, bool) {
	body, ok := s.Body.(*ast.BlockStmt)
	if !ok || len(body.Statements) != 1 {
		return "", nil, false
	}
	exprStmt, ok := body.Statements[0].(*ast.ExprStmt)
	if !ok {
		return "", nil, false
	}
	assign, ok := exprStmt.Expr.(*ast.AssignExpr)
	if !ok || assign.Op != token.Eq {
		return "", nil, false
	}
	left, ok := assign.Left.(*ast.IdentExpr)
	if !ok {
		return "", nil, false
	}
	bin, ok := assign.Right.(*ast.BinaryExpr)
	if !ok || bin.Op != token.Plus || g.semanticType(bin) != types.TypeString {
		return "", nil, false
	}
	base, ok := bin.Left.(*ast.IdentExpr)
	if !ok || base.Name != left.Name {
		return "", nil, false
	}
	if _, ok := g.locals[left.Name].(ir.ConstString); !ok {
		return "", nil, false
	}
	// Keep the first ownership proof intentionally narrow. Pure literals and a
	// different local cannot observe or alias the accumulator during append.
	switch suffix := bin.Right.(type) {
	case *ast.StringLit, *ast.NumberLit, *ast.BoolLit:
		return left.Name, suffix, true
	case *ast.IdentExpr:
		if suffix.Name != left.Name {
			return left.Name, suffix, true
		}
	}
	return "", nil, false
}
