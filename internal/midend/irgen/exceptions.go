package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) currentCatch() *catchContext {
	if len(g.catchStack) == 0 {
		return nil
	}
	return g.catchStack[len(g.catchStack)-1]
}

func (g *generator) currentFinally() *finallyContext {
	if len(g.finallyStack) == 0 {
		return nil
	}
	return g.finallyStack[len(g.finallyStack)-1]
}

func (g *generator) routeFinallyCompletion(ctx *finallyContext, kind float64, value ir.Operand) {
	from := g.currentBB
	ctx.kindPhi.Incoming = append(ctx.kindPhi.Incoming, ir.PhiIncoming{Block: from, Value: ir.ConstNumber{Value: kind}})
	ctx.valuePhi.Incoming = append(ctx.valuePhi.Incoming, ir.PhiIncoming{Block: from, Value: value})
	from.Terminator = &ir.JumpTerm{Target: ctx.block}
	g.currentBB = g.currentFn.NewBlock("after_finally_route_dead")
	g.currentBB.Terminator = &ir.ReturnTerm{}
}

func (g *generator) routeThrownValue(value ir.Operand) {
	fctx := g.currentFinally()
	cctx := g.currentCatch()
	// A catch belonging to the innermost active try sees the throw before that
	// try's finally. If the top catch belongs to an outer try, the inner finally
	// must run first and forward the saved throw afterwards.
	if fctx != nil && (cctx == nil || cctx.finallyDepth < len(g.finallyStack)) {
		g.routeFinallyCompletion(fctx, 2, value)
		return
	}
	if cctx != nil {
		from := g.currentBB
		cctx.phi.Incoming = append(cctx.phi.Incoming, ir.PhiIncoming{Block: from, Value: value})
		from.Terminator = &ir.JumpTerm{Target: cctx.block}
		g.currentBB = g.currentFn.NewBlock("after_throw_dead")
		g.currentBB.Terminator = &ir.ReturnTerm{}
		return
	}
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_reject", Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeAny}})
	g.currentBB.Terminator = &ir.ReturnTerm{}
	g.currentBB = g.currentFn.NewBlock("after_reject_dead")
	g.currentBB.Terminator = &ir.ReturnTerm{}
}

func (g *generator) lowerTry(s *ast.TryStmt) {
	if s.Finally != nil {
		g.lowerTryWithFinally(s)
		return
	}
	outerLocals := cloneOperandMap(g.locals)
	catchBB := g.currentFn.NewBlock("catch")
	exitBB := g.currentFn.NewBlock("try_exit")
	errorVal := g.currentFn.NewValue("caught_error", types.TypeAny)
	phi := &ir.PhiInst{Res: errorVal}
	catchBB.Phis = append(catchBB.Phis, phi)
	ctx := &catchContext{block: catchBB, phi: phi, finallyDepth: len(g.finallyStack)}
	g.catchStack = append(g.catchStack, ctx)
	g.lowerStatement(s.Try)
	g.catchStack = g.catchStack[:len(g.catchStack)-1]
	hasExit := false
	if g.currentBB.Terminator == nil {
		g.currentBB.Terminator = &ir.JumpTerm{Target: exitBB}
		hasExit = true
	}
	g.currentBB = catchBB
	g.locals = cloneOperandMap(outerLocals)
	if s.CatchName != "" {
		g.locals[s.CatchName] = errorVal
	}
	g.lowerStatement(s.Catch)
	if g.currentBB.Terminator == nil {
		g.currentBB.Terminator = &ir.JumpTerm{Target: exitBB}
		hasExit = true
	}
	g.currentBB = exitBB
	if !hasExit {
		g.currentBB.Terminator = &ir.ReturnTerm{}
	}
	g.locals = outerLocals
}

func (g *generator) lowerTryWithFinally(s *ast.TryStmt) {
	outerLocals := cloneOperandMap(g.locals)
	finallyBB := g.currentFn.NewBlock("finally")
	afterBB := g.currentFn.NewBlock("finally_after")
	kindVal := g.currentFn.NewValue("completion_kind", types.TypeNumber)
	valueVal := g.currentFn.NewValue("completion_value", types.TypeAny)
	kindPhi := &ir.PhiInst{Res: kindVal}
	valuePhi := &ir.PhiInst{Res: valueVal}
	finallyBB.Phis = append(finallyBB.Phis, kindPhi, valuePhi)
	fctx := &finallyContext{block: finallyBB, kindPhi: kindPhi, valuePhi: valuePhi}
	g.finallyStack = append(g.finallyStack, fctx)
	hasNormalCompletion := false

	if s.Catch != nil {
		catchBB := g.currentFn.NewBlock("catch")
		errorVal := g.currentFn.NewValue("caught_error", types.TypeAny)
		catchPhi := &ir.PhiInst{Res: errorVal}
		catchBB.Phis = append(catchBB.Phis, catchPhi)
		cctx := &catchContext{block: catchBB, phi: catchPhi, finallyDepth: len(g.finallyStack)}
		g.catchStack = append(g.catchStack, cctx)
		g.lowerStatement(s.Try)
		g.catchStack = g.catchStack[:len(g.catchStack)-1]
		if g.currentBB.Terminator == nil {
			hasNormalCompletion = true
			g.routeFinallyCompletion(fctx, 0, ir.ConstUndefined{})
		}

		g.currentBB = catchBB
		g.locals = cloneOperandMap(outerLocals)
		if s.CatchName != "" {
			g.locals[s.CatchName] = errorVal
		}
		g.lowerStatement(s.Catch)
		if g.currentBB.Terminator == nil {
			hasNormalCompletion = true
			g.routeFinallyCompletion(fctx, 0, ir.ConstUndefined{})
		}
	} else {
		g.lowerStatement(s.Try)
		if g.currentBB.Terminator == nil {
			hasNormalCompletion = true
			g.routeFinallyCompletion(fctx, 0, ir.ConstUndefined{})
		}
	}

	g.finallyStack = g.finallyStack[:len(g.finallyStack)-1]
	g.currentBB = finallyBB
	g.locals = cloneOperandMap(outerLocals)
	g.lowerStatement(s.Finally)
	finalLocals := cloneOperandMap(g.locals)
	if g.currentBB.Terminator != nil {
		g.locals = outerLocals
		return
	}

	normalBB := g.currentFn.NewBlock("finally_normal")
	abruptBB := g.currentFn.NewBlock("finally_abrupt")
	isNormal := g.currentFn.NewValue("completion_normal", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: isNormal, Op: ir.OpEq, LHS: kindVal, RHS: ir.ConstNumber{Value: 0}})
	g.currentBB.Terminator = &ir.BranchTerm{Cond: isNormal, Then: normalBB, Else: abruptBB}
	normalBB.Terminator = &ir.JumpTerm{Target: afterBB}

	g.currentBB = abruptBB
	returnBB := g.currentFn.NewBlock("finally_return")
	throwBB := g.currentFn.NewBlock("finally_throw")
	isReturn := g.currentFn.NewValue("completion_return", types.TypeBoolean)
	abruptBB.Instructions = append(abruptBB.Instructions, &ir.BinaryInst{Res: isReturn, Op: ir.OpEq, LHS: kindVal, RHS: ir.ConstNumber{Value: 1}})
	abruptBB.Terminator = &ir.BranchTerm{Cond: isReturn, Then: returnBB, Else: throwBB}

	g.currentBB = returnBB
	ret := g.coerceJSValueBoundary(valueVal, types.TypeAny, g.currentFn.ReturnType)
	returnBB.Terminator = &ir.ReturnTerm{Val: ret}

	g.currentBB = throwBB
	g.routeThrownValue(valueVal)

	g.currentBB = afterBB
	g.locals = finalLocals
	if !hasNormalCompletion {
		g.currentBB.Terminator = &ir.ReturnTerm{}
	}
}
