package irgen

import (
	"fmt"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/token"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
	"github.com/phongsathornpt/ts-pro/internal/frontend/sema"
)

type generator struct {
	semaResult *sema.Result
	prog       *ir.Program
	currentFn  *ir.Function
	currentBB  *ir.BasicBlock
	locals     map[string]ir.Operand
}

// Generate lowers an AST program and its semantic facts into SSA IR.
func Generate(astProg *ast.Program, semaResult *sema.Result) (*ir.Program, error) {
	g := &generator{
		semaResult: semaResult,
		prog:       &ir.Program{},
	}

	var topStmts []ast.Stmt
	for _, stmt := range astProg.Statements {
		if fnDecl, ok := stmt.(*ast.FunctionDecl); ok {
			fn, err := g.lowerFunction(fnDecl)
			if err != nil {
				return nil, err
			}
			g.prog.Functions = append(g.prog.Functions, fn)
		} else {
			topStmts = append(topStmts, stmt)
		}
	}

	if len(topStmts) > 0 {
		mainFn := g.lowerTopLevel(topStmts)
		// Put @main at the front of Functions so it's the entrypoint
		g.prog.Functions = append([]*ir.Function{mainFn}, g.prog.Functions...)
	}

	return g.prog, nil
}

func (g *generator) lowerTopLevel(stmts []ast.Stmt) *ir.Function {
	irFn := ir.NewFunction("@main", types.TypeVoid)
	g.currentFn = irFn
	g.locals = make(map[string]ir.Operand)

	entryBB := irFn.NewBlock("entry")
	g.currentBB = entryBB

	for _, s := range stmts {
		g.lowerStatement(s)
	}

	if g.currentBB.Terminator == nil {
		g.currentBB.Terminator = &ir.ReturnTerm{}
	}

	return irFn
}

func (g *generator) lowerFunction(fnDecl *ast.FunctionDecl) (*ir.Function, error) {
	fnType, _ := g.semaResult.Types[fnDecl].(*types.FunctionType)
	var retType types.Type = types.TypeVoid
	if fnType != nil {
		retType = fnType.Return
	}

	irFn := ir.NewFunction(fnDecl.Name, retType)
	g.currentFn = irFn
	g.locals = make(map[string]ir.Operand)

	// Entry block
	entryBB := irFn.NewBlock("entry")
	g.currentBB = entryBB

	// Lower parameters as values
	for _, p := range fnDecl.Params {
		pType := types.TypeNumber
		if fnType != nil {
			for _, param := range fnType.Params {
				if param.Name == p.Name {
					pType = param.Type
					break
				}
			}
		}
		val := irFn.NewValue(p.Name, pType)
		irFn.Params = append(irFn.Params, val)
		g.locals[p.Name] = val
	}

	// Lower statements
	if fnDecl.Body != nil {
		for _, stmt := range fnDecl.Body.Statements {
			g.lowerStatement(stmt)
		}
	}

	// Ensure entry or current block has a terminator
	if g.currentBB.Terminator == nil {
		g.currentBB.Terminator = &ir.ReturnTerm{}
	}

	return irFn, nil
}

func (g *generator) lowerStatement(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.BlockStmt:
		for _, child := range s.Statements {
			g.lowerStatement(child)
		}
	case *ast.VarDeclStmt:
		for _, d := range s.Declarations {
			var initOp ir.Operand
			if d.Init != nil {
				initOp = g.lowerExpr(d.Init)
			}
			if initOp == nil {
				initOp = ir.ConstNumber{Value: 0}
			}
			g.locals[d.Name] = initOp
		}
	case *ast.ReturnStmt:
		var val ir.Operand
		if s.Value != nil {
			val = g.lowerExpr(s.Value)
		}
		g.currentBB.Terminator = &ir.ReturnTerm{Val: val}
	case *ast.ExprStmt:
		g.lowerExpr(s.Expr)
	case *ast.IfStmt:
		g.lowerIf(s)
	case *ast.WhileStmt:
		g.lowerWhile(s)
	case *ast.ForStmt:
		g.lowerFor(s)
	}
}

func findModifiedVars(stmt ast.Stmt) map[string]bool {
	res := make(map[string]bool)
	var walk func(n ast.Node)
	walk = func(n ast.Node) {
		if n == nil {
			return
		}
		switch node := n.(type) {
		case *ast.AssignExpr:
			if ident, ok := node.Left.(*ast.IdentExpr); ok {
				res[ident.Name] = true
			}
			walk(node.Right)
		case *ast.UnaryExpr:
			if node.Op == token.PlusPlus || node.Op == token.MinusMinus {
				if ident, ok := node.Target.(*ast.IdentExpr); ok {
					res[ident.Name] = true
				}
			}
			walk(node.Target)
		case *ast.BlockStmt:
			for _, s := range node.Statements {
				walk(s)
			}
		case *ast.ExprStmt:
			walk(node.Expr)
		case *ast.IfStmt:
			walk(node.Cond)
			walk(node.Then)
			walk(node.Else)
		case *ast.WhileStmt:
			walk(node.Cond)
			walk(node.Body)
		case *ast.ForStmt:
			walk(node.Init)
			walk(node.Cond)
			walk(node.Post)
			walk(node.Body)
		}
	}
	walk(stmt)
	return res
}

func (g *generator) lowerFor(s *ast.ForStmt) {
	if s.Init != nil {
		g.lowerStatement(s.Init)
	}
	preBB := g.currentBB
	condBB := g.currentFn.NewBlock("for_cond")
	bodyBB := g.currentFn.NewBlock("for_body")
	postBB := g.currentFn.NewBlock("for_post")
	exitBB := g.currentFn.NewBlock("for_exit")

	if preBB.Terminator == nil {
		preBB.Terminator = &ir.JumpTerm{Target: condBB}
	}

	modVars := findModifiedVars(s.Body)
	if s.Post != nil {
		for k, v := range findModifiedVars(&ast.ExprStmt{Expr: s.Post}) {
			if v {
				modVars[k] = true
			}
		}
	}

	loopPhis := make(map[string]*ir.PhiInst)
	for name := range modVars {
		if val, exists := g.locals[name]; exists {
			phiVal := g.currentFn.NewValue(fmt.Sprintf("%s_loop", name), val.Type())
			phi := &ir.PhiInst{
				Res: phiVal,
				Incoming: []ir.PhiIncoming{
					{Block: preBB, Value: val},
				},
			}
			loopPhis[name] = phi
			condBB.Phis = append(condBB.Phis, phi)
			g.locals[name] = phiVal
		}
	}

	g.currentBB = condBB
	if s.Cond != nil {
		cond := g.lowerExpr(s.Cond)
		condBB.Terminator = &ir.BranchTerm{Cond: cond, Then: bodyBB, Else: exitBB}
	} else {
		condBB.Terminator = &ir.JumpTerm{Target: bodyBB}
	}

	g.currentBB = bodyBB
	g.lowerStatement(s.Body)
	if g.currentBB.Terminator == nil {
		g.currentBB.Terminator = &ir.JumpTerm{Target: postBB}
	}

	g.currentBB = postBB
	if s.Post != nil {
		g.lowerExpr(s.Post)
	}
	postEndBB := g.currentBB
	if postEndBB.Terminator == nil {
		postEndBB.Terminator = &ir.JumpTerm{Target: condBB}
	}

	for name, phi := range loopPhis {
		updatedVal := g.locals[name]
		phi.Incoming = append(phi.Incoming, ir.PhiIncoming{
			Block: postEndBB,
			Value: updatedVal,
		})
		g.locals[name] = phi.Res
	}

	g.currentBB = exitBB
}

func (g *generator) lowerIf(s *ast.IfStmt) {
	cond := g.lowerExpr(s.Cond)
	thenBB := g.currentFn.NewBlock("then")
	elseBB := g.currentFn.NewBlock("else")
	joinBB := g.currentFn.NewBlock("join")

	g.currentBB.Terminator = &ir.BranchTerm{
		Cond: cond,
		Then: thenBB,
		Else: elseBB,
	}

	// Save locals snapshot
	origLocals := make(map[string]ir.Operand)
	for k, v := range g.locals {
		origLocals[k] = v
	}

	// Then block
	g.currentBB = thenBB
	g.lowerStatement(s.Then)
	thenEndBB := g.currentBB
	if thenEndBB.Terminator == nil {
		thenEndBB.Terminator = &ir.JumpTerm{Target: joinBB}
	}
	thenLocals := make(map[string]ir.Operand)
	for k, v := range g.locals {
		thenLocals[k] = v
	}

	// Restore locals for else block
	g.locals = make(map[string]ir.Operand)
	for k, v := range origLocals {
		g.locals[k] = v
	}

	// Else block
	g.currentBB = elseBB
	if s.Else != nil {
		g.lowerStatement(s.Else)
	}
	elseEndBB := g.currentBB
	if elseEndBB.Terminator == nil {
		elseEndBB.Terminator = &ir.JumpTerm{Target: joinBB}
	}
	elseLocals := make(map[string]ir.Operand)
	for k, v := range g.locals {
		elseLocals[k] = v
	}

	// Join block - insert SSA phi nodes for modified variables
	g.currentBB = joinBB
	for name, origVal := range origLocals {
		thenVal := thenLocals[name]
		elseVal := elseLocals[name]
		if thenVal != elseVal {
			phiVal := g.currentFn.NewValue(fmt.Sprintf("%s_phi", name), origVal.Type())
			phi := &ir.PhiInst{
				Res: phiVal,
				Incoming: []ir.PhiIncoming{
					{Block: thenEndBB, Value: thenVal},
					{Block: elseEndBB, Value: elseVal},
				},
			}
			joinBB.Phis = append(joinBB.Phis, phi)
			g.locals[name] = phiVal
		} else {
			g.locals[name] = origVal
		}
	}
}

func (g *generator) lowerWhile(s *ast.WhileStmt) {
	preBB := g.currentBB
	condBB := g.currentFn.NewBlock("while_cond")
	bodyBB := g.currentFn.NewBlock("while_body")
	exitBB := g.currentFn.NewBlock("while_exit")

	preBB.Terminator = &ir.JumpTerm{Target: condBB}

	modVars := findModifiedVars(s.Body)
	loopPhis := make(map[string]*ir.PhiInst)
	for name := range modVars {
		if val, exists := g.locals[name]; exists {
			phiVal := g.currentFn.NewValue(fmt.Sprintf("%s_loop", name), val.Type())
			phi := &ir.PhiInst{
				Res: phiVal,
				Incoming: []ir.PhiIncoming{
					{Block: preBB, Value: val},
				},
			}
			loopPhis[name] = phi
			condBB.Phis = append(condBB.Phis, phi)
			g.locals[name] = phiVal
		}
	}

	g.currentBB = condBB
	cond := g.lowerExpr(s.Cond)
	condBB.Terminator = &ir.BranchTerm{
		Cond: cond,
		Then: bodyBB,
		Else: exitBB,
	}

	g.currentBB = bodyBB
	g.lowerStatement(s.Body)
	bodyEndBB := g.currentBB
	if bodyEndBB.Terminator == nil {
		bodyEndBB.Terminator = &ir.JumpTerm{Target: condBB}
	}

	for name, phi := range loopPhis {
		updatedVal := g.locals[name]
		phi.Incoming = append(phi.Incoming, ir.PhiIncoming{
			Block: bodyEndBB,
			Value: updatedVal,
		})
		g.locals[name] = phi.Res
	}

	g.currentBB = exitBB
}

func (g *generator) lowerExpr(expr ast.Expr) ir.Operand {
	if expr == nil {
		return nil
	}

	switch e := expr.(type) {
	case *ast.NumberLit:
		return ir.ConstNumber{Value: e.Value}
	case *ast.StringLit:
		return ir.ConstString{Value: e.Value}
	case *ast.BoolLit:
		return ir.ConstBool{Value: e.Value}
	case *ast.IdentExpr:
		if op, exists := g.locals[e.Name]; exists {
			return op
		}
		// Undefined or global
		v := g.currentFn.NewValue(e.Name, types.TypeNumber)
		return v
	case *ast.BinaryExpr:
		lhs := g.lowerExpr(e.Left)
		rhs := g.lowerExpr(e.Right)
		op := ir.OpAdd
		switch e.Op {
		case token.Plus:
			op = ir.OpAdd
		case token.Minus:
			op = ir.OpSub
		case token.Star:
			op = ir.OpMul
		case token.Slash:
			op = ir.OpDiv
		case token.Percent:
			op = ir.OpMod
		case token.EqEq, token.EqEqEq:
			op = ir.OpEq
		case token.BangEq, token.BangEqEq:
			op = ir.OpNe
		case token.Lt:
			op = ir.OpLt
		case token.LtEq:
			op = ir.OpLe
		case token.Gt:
			op = ir.OpGt
		case token.GtEq:
			op = ir.OpGe
		}
		resVal := g.currentFn.NewValue("t", types.TypeNumber)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
			Res: resVal,
			Op:  op,
			LHS: lhs,
			RHS: rhs,
		})
		return resVal
	case *ast.UnaryExpr:
		if e.Op == token.PlusPlus || e.Op == token.MinusMinus {
			if ident, ok := e.Target.(*ast.IdentExpr); ok {
				currVal, exists := g.locals[ident.Name]
				if !exists {
					currVal = ir.ConstNumber{Value: 0}
				}
				op := ir.OpAdd
				if e.Op == token.MinusMinus {
					op = ir.OpSub
				}
				nextVal := g.currentFn.NewValue(ident.Name+"_inc", types.TypeNumber)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
					Res: nextVal,
					Op:  op,
					LHS: currVal,
					RHS: ir.ConstNumber{Value: 1},
				})
				g.locals[ident.Name] = nextVal
				if e.Prefix {
					return nextVal
				}
				return currVal
			}
		}
		target := g.lowerExpr(e.Target)
		return target
	case *ast.CallExpr:
		calleeName := "unknown"
		if ident, ok := e.Callee.(*ast.IdentExpr); ok {
			calleeName = ident.Name
		} else if mem, ok := e.Callee.(*ast.MemberExpr); ok {
			if objIdent, ok := mem.Object.(*ast.IdentExpr); ok && objIdent.Name == "console" && mem.Property == "log" {
				calleeName = "ts_print_val"
			}
		}
		var args []ir.Operand
		for _, arg := range e.Args {
			args = append(args, g.lowerExpr(arg))
		}
		resVal := g.currentFn.NewValue("ret", types.TypeNumber)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res:    resVal,
			Callee: calleeName,
			Args:   args,
		})
		return resVal
	case *ast.AssignExpr:
		rhs := g.lowerExpr(e.Right)
		if ident, ok := e.Left.(*ast.IdentExpr); ok {
			g.locals[ident.Name] = rhs
		}
		return rhs
	case *ast.TernaryExpr:
		cond := g.lowerExpr(e.Cond)
		thenBB := g.currentFn.NewBlock("tern_then")
		elseBB := g.currentFn.NewBlock("tern_else")
		joinBB := g.currentFn.NewBlock("tern_join")

		g.currentBB.Terminator = &ir.BranchTerm{Cond: cond, Then: thenBB, Else: elseBB}

		g.currentBB = thenBB
		thenVal := g.lowerExpr(e.Then)
		thenEndBB := g.currentBB
		if thenEndBB.Terminator == nil {
			thenEndBB.Terminator = &ir.JumpTerm{Target: joinBB}
		}

		g.currentBB = elseBB
		elseVal := g.lowerExpr(e.Else)
		elseEndBB := g.currentBB
		if elseEndBB.Terminator == nil {
			elseEndBB.Terminator = &ir.JumpTerm{Target: joinBB}
		}

		g.currentBB = joinBB
		resVal := g.currentFn.NewValue("tern", thenVal.Type())
		joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{
			Res: resVal,
			Incoming: []ir.PhiIncoming{
				{Block: thenEndBB, Value: thenVal},
				{Block: elseEndBB, Value: elseVal},
			},
		})
		return resVal
	default:
		return ir.ConstNumber{Value: 0}
	}
}
