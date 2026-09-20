package irgen

import (
	"fmt"
	"sort"
	"strings"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/token"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) lowerStatement(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.BlockStmt:
		for _, child := range s.Statements {
			g.lowerStatement(child)
		}
	case *ast.VarDeclStmt:
		resolved := g.semaResult.VarTypes[s]
		for i, d := range s.Declarations {
			var initOp ir.Operand
			if d.Init != nil {
				if typeNodeIsAny(d.Type) {
					if lit, ok := d.Init.(*ast.ObjectLit); ok {
						initOp = g.lowerDynamicObjectLiteral(lit)
					}
				}
				if initOp == nil {
					initOp = g.lowerExpr(d.Init)
				}
				if i < len(resolved) {
					sourceType := g.semanticType(d.Init)
					targetType := resolved[i]
					if objectType, ok := targetType.(*types.ObjectType); ok && irJSValueType(sourceType) {
						if provenance, known := g.provenObjectType(d.Init); known {
							initOp = g.unboxKnownObject(initOp, provenance)
						} else if strings.HasPrefix(objectType.Name, "$") {
							initOp = g.unboxKnownObject(initOp, objectType)
						} else {
							initOp = g.materializeDynamicObject(initOp, objectType)
						}
					} else {
						initOp = g.coerceJSValueBoundary(initOp, sourceType, targetType)
					}
					if irJSValueType(targetType) {
						switch concrete := sourceType.(type) {
						case *types.ObjectType:
							if _, dynamicLiteral := d.Init.(*ast.ObjectLit); !dynamicLiteral && !dynamicBackedObjectType(concrete) {
								g.localProvenance[d.Name] = concrete
							}
						case *types.FunctionType:
							g.localProvenance[d.Name] = concrete
							if target, ok := g.directCalleeForExpr(d.Init); ok {
								g.localDirectCallee[d.Name] = target
							}
						}
					}
				}
			}
			if initOp == nil {
				initOp = ir.ConstNumber{Value: 0}
			}
			g.locals[d.Name] = initOp
		}
	case *ast.ThrowStmt:
		val := g.lowerExpr(s.Value)
		val = g.boxJSValue(val, g.semanticType(s.Value))
		g.routeThrownValue(val)
	case *ast.TryStmt:
		g.lowerTry(s)
	case *ast.ReturnStmt:
		if fctx := g.currentFinally(); fctx != nil {
			var val ir.Operand = ir.ConstUndefined{}
			if s.Value != nil {
				val = g.lowerExpr(s.Value)
				val = g.boxJSValue(val, g.semanticType(s.Value))
			}
			g.routeFinallyCompletion(fctx, 1, val)
			break
		}
		var val ir.Operand
		if s.Value != nil {
			val = g.lowerExpr(s.Value)
			val = g.coerceJSValueBoundary(val, g.semanticType(s.Value), g.currentFn.ReturnType)
		}
		g.currentBB.Terminator = &ir.ReturnTerm{Val: val}
	case *ast.ExprStmt:
		g.lowerExpr(s.Expr)
	case *ast.IfStmt:
		g.lowerIf(s)
	case *ast.WhileStmt:
		g.lowerWhile(s)
	case *ast.DoWhileStmt:
		g.lowerDoWhile(s)
	case *ast.ForStmt:
		g.lowerFor(s)
	case *ast.ForOfStmt:
		g.lowerForOf(s)
	case *ast.SwitchStmt:
		g.lowerSwitch(s)
	case *ast.BreakStmt:
		if g.err == nil {
			g.err = fmt.Errorf("break outside supported switch lowering")
		}
	case *ast.ContinueStmt:
		if g.err == nil {
			g.err = fmt.Errorf("continue lowering is not implemented")
		}
	}
}

func sortedModifiedVarNames(vars map[string]bool) []string {
	names := make([]string, 0, len(vars))
	for name, modified := range vars {
		if modified {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
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

func (g *generator) lowerForOf(s *ast.ForOfStmt) {
	iterable := g.lowerExpr(s.Iterable)
	var elemType types.Type
	if arrType, ok := g.semaResult.Types[s.Iterable].(*types.ArrayType); ok {
		elemType = arrType.Elem
	} else if objType, ok := g.semaResult.Types[s.Iterable].(*types.ObjectType); ok && objType.Name == "$Headers" {
		iterable = g.lowerHeadersEntries(iterable)
		elemType = types.NewArray(types.TypeString)
	}

	preBB := g.currentBB
	condBB := g.currentFn.NewBlock("forof_cond")
	bodyBB := g.currentFn.NewBlock("forof_body")
	postBB := g.currentFn.NewBlock("forof_post")
	exitBB := g.currentFn.NewBlock("forof_exit")
	if preBB.Terminator == nil {
		preBB.Terminator = &ir.JumpTerm{Target: condBB}
	}

	modVars := findModifiedVars(s.Body)
	delete(modVars, s.Name)
	loopPhis := make(map[string]*ir.PhiInst)
	for _, name := range sortedModifiedVarNames(modVars) {
		if val, exists := g.locals[name]; exists {
			phiVal := g.currentFn.NewValue(fmt.Sprintf("%s_forof", name), val.Type())
			phi := &ir.PhiInst{Res: phiVal, Incoming: []ir.PhiIncoming{{Block: preBB, Value: val}}}
			loopPhis[name] = phi
			condBB.Phis = append(condBB.Phis, phi)
			g.locals[name] = phiVal
		}
	}
	index := g.currentFn.NewValue("forof_i", types.TypeNumber)
	indexPhi := &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{{Block: preBB, Value: ir.ConstNumber{Value: 0}}}}
	condBB.Phis = append(condBB.Phis, indexPhi)

	g.currentBB = condBB
	length := g.currentFn.NewValue("forof_len", types.TypeNumber)
	condBB.Instructions = append(condBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: iterable})
	cond := g.currentFn.NewValue("forof_has", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: cond, Op: ir.OpLt, LHS: index, RHS: length})
	condBB.Terminator = &ir.BranchTerm{Cond: cond, Then: bodyBB, Else: exitBB}

	previousLoopVar, hadPrevious := g.locals[s.Name]
	g.currentBB = bodyBB
	elem := g.currentFn.NewValue(s.Name, elemType)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: elem, Array: iterable, Index: index})
	g.locals[s.Name] = elem
	g.lowerStatement(s.Body)
	bodyEnd := g.currentBB
	if bodyEnd.Terminator == nil {
		bodyEnd.Terminator = &ir.JumpTerm{Target: postBB}
	}

	g.currentBB = postBB
	nextIndex := g.currentFn.NewValue("forof_next", types.TypeNumber)
	postBB.Instructions = append(postBB.Instructions, &ir.BinaryInst{Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
	postEnd := g.currentBB
	if postEnd.Terminator == nil {
		postEnd.Terminator = &ir.JumpTerm{Target: condBB}
	}
	indexPhi.Incoming = append(indexPhi.Incoming, ir.PhiIncoming{Block: postEnd, Value: nextIndex})
	for name, phi := range loopPhis {
		phi.Incoming = append(phi.Incoming, ir.PhiIncoming{Block: postEnd, Value: g.locals[name]})
		g.locals[name] = phi.Res
	}
	if hadPrevious {
		g.locals[s.Name] = previousLoopVar
	} else {
		delete(g.locals, s.Name)
	}
	g.currentBB = exitBB
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
	ownedStringName, ownedStringSuffix, ownedStringAppend := g.ownedStringAppendCandidate(s)
	if ownedStringAppend {
		seed := g.currentFn.NewValue("str_owned", types.TypeString)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: seed, Callee: "ts_string_builder_seed", Args: []ir.Operand{g.locals[ownedStringName]},
		})
		g.locals[ownedStringName] = seed
	}
	if s.Post != nil {
		for k, v := range findModifiedVars(&ast.ExprStmt{Expr: s.Post}) {
			if v {
				modVars[k] = true
			}
		}
	}

	loopPhis := make(map[string]*ir.PhiInst)
	for _, name := range sortedModifiedVarNames(modVars) {
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
	if ownedStringAppend {
		suffix := g.lowerExpr(ownedStringSuffix)
		suffix = g.coerceStringOperand(ownedStringSuffix, suffix)
		res := g.currentFn.NewValue("str_append", types.TypeString)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: res, Callee: "ts_string_append_owned", Args: []ir.Operand{g.locals[ownedStringName], suffix},
		})
		g.locals[ownedStringName] = res
	} else {
		g.lowerStatement(s.Body)
	}
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
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)
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
	for _, name := range sortedModifiedVarNames(modVars) {
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

func (g *generator) lowerSwitch(s *ast.SwitchStmt) {
	discr := g.lowerExpr(s.Expr)
	tempName := fmt.Sprintf("$switch%d", g.arrowCounter)
	g.arrowCounter++
	g.locals[tempName] = discr

	var chain ast.Stmt
	for i := len(s.Cases) - 1; i >= 0; i-- {
		clause := s.Cases[i]
		stmts := append([]ast.Stmt(nil), clause.Statements...)
		terminated := false
		if len(stmts) > 0 {
			switch stmts[len(stmts)-1].(type) {
			case *ast.BreakStmt:
				stmts = stmts[:len(stmts)-1]
				terminated = true
			case *ast.ReturnStmt:
				terminated = true
			}
		}
		if !terminated && i != len(s.Cases)-1 {
			if g.err == nil {
				g.err = fmt.Errorf("switch fallthrough is not yet supported in native lowering")
			}
			return
		}
		block := &ast.BlockStmt{SourceSpan: clause.SourceSpan, Statements: stmts}
		if clause.Test == nil {
			chain = block
			continue
		}
		left := &ast.IdentExpr{SourceSpan: s.Expr.Span(), Name: tempName}
		cond := &ast.BinaryExpr{SourceSpan: clause.SourceSpan, Left: left, Op: token.EqEqEq, Right: clause.Test}
		g.semaResult.Types[left] = discr.Type()
		g.semaResult.Types[cond] = types.TypeBoolean
		chain = &ast.IfStmt{SourceSpan: clause.SourceSpan, Cond: cond, Then: block, Else: chain}
	}
	if chain != nil {
		g.lowerStatement(chain)
	}
	delete(g.locals, tempName)
}

func (g *generator) lowerDoWhile(s *ast.DoWhileStmt) {
	preBB := g.currentBB
	bodyBB := g.currentFn.NewBlock("do_body")
	condBB := g.currentFn.NewBlock("do_cond")
	exitBB := g.currentFn.NewBlock("do_exit")
	if preBB.Terminator == nil {
		preBB.Terminator = &ir.JumpTerm{Target: bodyBB}
	}

	modVars := findModifiedVars(s.Body)
	loopPhis := make(map[string]*ir.PhiInst)
	for _, name := range sortedModifiedVarNames(modVars) {
		if val, exists := g.locals[name]; exists {
			phiVal := g.currentFn.NewValue(fmt.Sprintf("%s_do", name), val.Type())
			phi := &ir.PhiInst{Res: phiVal, Incoming: []ir.PhiIncoming{{Block: preBB, Value: val}}}
			loopPhis[name] = phi
			bodyBB.Phis = append(bodyBB.Phis, phi)
			g.locals[name] = phiVal
		}
	}

	g.currentBB = bodyBB
	g.lowerStatement(s.Body)
	bodyEnd := g.currentBB
	if bodyEnd.Terminator == nil {
		bodyEnd.Terminator = &ir.JumpTerm{Target: condBB}
	}

	g.currentBB = condBB
	cond := g.lowerExpr(s.Cond)
	condEnd := g.currentBB
	if condEnd.Terminator == nil {
		condEnd.Terminator = &ir.BranchTerm{Cond: cond, Then: bodyBB, Else: exitBB}
	}
	for name, phi := range loopPhis {
		updated := g.locals[name]
		phi.Incoming = append(phi.Incoming, ir.PhiIncoming{Block: condEnd, Value: updated})
	}

	g.currentBB = exitBB
}
