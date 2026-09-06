package irgen

import (
	"fmt"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) newAbortReason(name, message string) ir.Operand {
	err := g.newDOMException(ir.ConstString{Value: message}, ir.ConstString{Value: name})
	return g.boxJSValue(err, g.semaResult.DOMExceptionType)
}

func (g *generator) makeAbortTimeoutCallback(signal ir.Operand) ir.Operand {
	outerFn, outerBB, outerLocals, outerProv, outerDirect := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	fnType := types.NewFunction(nil, types.TypeVoid)
	name := fmt.Sprintf("$abort_timeout%d", g.arrowCounter)
	g.arrowCounter++
	lifted := ir.NewFunction(name, types.TypeVoid)
	g.currentFn = lifted
	g.currentBB = lifted.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)
	env := lifted.NewValue("$env", fnType)
	lifted.Params = append(lifted.Params, env)
	captured := lifted.NewValue("signal", g.semaResult.AbortSignalType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ClosureGetInst{Res: captured, Closure: env, Index: 0})
	reason := g.newAbortReason("TimeoutError", "The operation timed out")
	first := lifted.NewValue("timeout_abort_first", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: first, Callee: "ts_abort_signal_set_reason", Args: []ir.Operand{captured, reason}, ParamTypes: []types.Type{g.semaResult.AbortSignalType, types.TypeAny}})
	fireBB := lifted.NewBlock("timeout_abort_fire")
	doneBB := lifted.NewBlock("timeout_abort_done")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: first, Then: fireBB, Else: doneBB}
	g.currentBB = fireBB
	event := g.lowerSimpleEvent("abort")
	g.lowerEventDispatch(captured, event)
	if g.currentBB.Terminator == nil {
		g.currentBB.Terminator = &ir.JumpTerm{Target: doneBB}
	}
	g.currentBB = doneBB
	g.currentBB.Terminator = &ir.ReturnTerm{}
	g.prog.Functions = append(g.prog.Functions, lifted)
	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProv, outerDirect
	closure := g.currentFn.NewValue("abort_timeout_callback", fnType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{Res: closure, Function: name, Captures: []ir.Operand{signal}, RefMask: 1})
	return closure
}

func (g *generator) lowerAbortSignalStaticCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	ident, ok := mem.Object.(*ast.IdentExpr)
	if !ok || ident.Name != "AbortSignal" {
		return nil, false
	}
	signal := g.currentFn.NewValue("abort_signal", g.semaResult.AbortSignalType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: signal, Callee: "ts_abort_signal_new"})
	switch mem.Property {
	case "abort":
		var reason ir.Operand
		if len(e.Args) > 0 {
			reason = g.lowerExpr(e.Args[0])
			if !irJSValueType(reason.Type()) {
				reason = g.boxJSValue(reason, g.semanticType(e.Args[0]))
			}
		} else {
			reason = g.newAbortReason("AbortError", "This operation was aborted")
		}
		set := g.currentFn.NewValue("abort_signal_static_set", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: set, Callee: "ts_abort_signal_set_reason", Args: []ir.Operand{signal, reason}, ParamTypes: []types.Type{g.semaResult.AbortSignalType, types.TypeAny}})
		return signal, true
	case "timeout":
		delay := g.lowerExpr(e.Args[0])
		callback := g.makeAbortTimeoutCallback(signal)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_set_timeout", Args: []ir.Operand{callback, delay}, ParamTypes: []types.Type{callback.Type(), types.TypeNumber}})
		return signal, true
	case "any":
		return g.lowerAbortSignalAny(e, signal), true
	}
	return nil, false
}

func (g *generator) makeAbortDependencyCallback(source, result ir.Operand) ir.Operand {
	outerFn, outerBB, outerLocals, outerProv, outerDirect := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	fnType := types.NewFunction([]types.Param{{Name: "event", Type: g.semaResult.EventType}}, types.TypeVoid)
	name := fmt.Sprintf("$abort_dependency%d", g.arrowCounter)
	g.arrowCounter++
	lifted := ir.NewFunction(name, types.TypeVoid)
	g.currentFn = lifted
	g.currentBB = lifted.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)
	env := lifted.NewValue("$env", fnType)
	lifted.Params = append(lifted.Params, env)
	capturedSource := lifted.NewValue("source", g.semaResult.AbortSignalType)
	capturedResult := lifted.NewValue("result", g.semaResult.AbortSignalType)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.ClosureGetInst{Res: capturedSource, Closure: env, Index: 0},
		&ir.ClosureGetInst{Res: capturedResult, Closure: env, Index: 1})
	eventParam := lifted.NewValue("event", g.semaResult.EventType)
	lifted.Params = append(lifted.Params, eventParam)
	already := lifted.NewValue("dependent_already_aborted", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: already, Callee: "ts_abort_signal_aborted", Args: []ir.Operand{capturedResult}})
	workBB := lifted.NewBlock("dependent_abort_work")
	doneBB := lifted.NewBlock("dependent_abort_done")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: already, Then: doneBB, Else: workBB}
	g.currentBB = workBB
	reason := lifted.NewValue("dependent_reason", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: reason, Callee: "ts_abort_signal_reason", Args: []ir.Operand{capturedSource}})
	first := lifted.NewValue("dependent_first", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: first, Callee: "ts_abort_signal_set_reason", Args: []ir.Operand{capturedResult, reason}, ParamTypes: []types.Type{g.semaResult.AbortSignalType, types.TypeAny}})
	fireBB := lifted.NewBlock("dependent_fire")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: first, Then: fireBB, Else: doneBB}
	g.currentBB = fireBB
	event := g.lowerSimpleEvent("abort")
	g.lowerEventDispatch(capturedResult, event)
	if g.currentBB.Terminator == nil {
		g.currentBB.Terminator = &ir.JumpTerm{Target: doneBB}
	}
	g.currentBB = doneBB
	g.currentBB.Terminator = &ir.ReturnTerm{}
	g.prog.Functions = append(g.prog.Functions, lifted)
	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProv, outerDirect
	closure := g.currentFn.NewValue("abort_dependency_callback", fnType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{Res: closure, Function: name, Captures: []ir.Operand{source, result}, RefMask: 3})
	return closure
}

func (g *generator) lowerAbortSignalAny(e *ast.CallExpr, result ir.Operand) ir.Operand {
	signals := g.lowerExpr(e.Args[0])
	arrType, ok := g.semanticType(e.Args[0]).(*types.ArrayType)
	if !ok {
		return g.failExpr("AbortSignal.any expects an AbortSignal array")
	}
	pre := g.currentBB
	condBB := g.currentFn.NewBlock("abort_any_cond")
	bodyBB := g.currentFn.NewBlock("abort_any_body")
	alreadyBB := g.currentFn.NewBlock("abort_any_already")
	listenBB := g.currentFn.NewBlock("abort_any_listen")
	postBB := g.currentFn.NewBlock("abort_any_post")
	doneBB := g.currentFn.NewBlock("abort_any_done")
	pre.Terminator = &ir.JumpTerm{Target: condBB}
	index := g.currentFn.NewValue("abort_any_i", types.TypeNumber)
	nextIndex := g.currentFn.NewValue("abort_any_next", types.TypeNumber)
	phi := &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{{Block: pre, Value: ir.ConstNumber{Value: 0}}, {Block: postBB, Value: nextIndex}}}
	condBB.Phis = append(condBB.Phis, phi)
	length := g.currentFn.NewValue("abort_any_len", types.TypeNumber)
	condBB.Instructions = append(condBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: signals})
	more := g.currentFn.NewValue("abort_any_more", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: doneBB}

	source := g.currentFn.NewValue("abort_any_source", arrType.Elem)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: source, Array: signals, Index: index})
	aborted := g.currentFn.NewValue("abort_any_source_aborted", types.TypeBoolean)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.CallInst{Res: aborted, Callee: "ts_abort_signal_aborted", Args: []ir.Operand{source}})
	bodyBB.Terminator = &ir.BranchTerm{Cond: aborted, Then: alreadyBB, Else: listenBB}

	g.currentBB = alreadyBB
	reason := g.currentFn.NewValue("abort_any_reason", types.TypeAny)
	alreadyBB.Instructions = append(alreadyBB.Instructions, &ir.CallInst{Res: reason, Callee: "ts_abort_signal_reason", Args: []ir.Operand{source}})
	set := g.currentFn.NewValue("abort_any_set", types.TypeBoolean)
	alreadyBB.Instructions = append(alreadyBB.Instructions, &ir.CallInst{Res: set, Callee: "ts_abort_signal_set_reason", Args: []ir.Operand{result, reason}, ParamTypes: []types.Type{g.semaResult.AbortSignalType, types.TypeAny}})
	alreadyBB.Terminator = &ir.JumpTerm{Target: doneBB}

	g.currentBB = listenBB
	callback := g.makeAbortDependencyCallback(source, result)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_event_target_add", Args: []ir.Operand{source, ir.ConstString{Value: "abort"}, callback, ir.ConstBool{Value: false}, ir.ConstBool{Value: false}, ir.ConstBool{Value: false}}})
	listenBB.Terminator = &ir.JumpTerm{Target: postBB}

	postBB.Instructions = append(postBB.Instructions, &ir.BinaryInst{Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
	postBB.Terminator = &ir.JumpTerm{Target: condBB}
	g.currentBB = doneBB
	return result
}

func (g *generator) lowerAbortControllerNew() ir.Operand {
	signalType := g.semaResult.AbortSignalType
	signal := g.currentFn.NewValue("abort_signal", signalType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: signal, Callee: "ts_abort_signal_new"})
	controllerType := g.semaResult.AbortControllerType
	offsets, refMask, shape := g.objectLayout(controllerType)
	controller := g.currentFn.NewValue("abort_controller", controllerType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{Res: controller, Shape: shape, FieldCount: len(offsets), RefMask: refMask})
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: controller, Field: "signal", Offset: offsets["signal"], Val: signal})
	return controller
}

func (g *generator) lowerAbortControllerMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || objType.Name != "$AbortController" || mem.Property != "abort" {
		return nil, false
	}
	controller := g.lowerExpr(mem.Object)
	offsets, _, _ := g.objectLayout(g.semaResult.AbortControllerType)
	signal := g.currentFn.NewValue("abort_signal", g.semaResult.AbortSignalType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: signal, Obj: controller, Field: "signal", Offset: offsets["signal"]})
	var reason ir.Operand
	if len(e.Args) > 0 {
		reason = g.lowerExpr(e.Args[0])
		if !irJSValueType(reason.Type()) {
			reason = g.boxJSValue(reason, g.semanticType(e.Args[0]))
		}
	} else {
		err := g.newDOMException(ir.ConstString{Value: "This operation was aborted"}, ir.ConstString{Value: "AbortError"})
		reason = g.boxJSValue(err, g.semaResult.DOMExceptionType)
	}
	first := g.currentFn.NewValue("abort_first", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: first, Callee: "ts_abort_signal_set_reason", Args: []ir.Operand{signal, reason}, ParamTypes: []types.Type{g.semaResult.AbortSignalType, types.TypeAny}})
	fireBB := g.currentFn.NewBlock("abort_fire")
	doneBB := g.currentFn.NewBlock("abort_done")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: first, Then: fireBB, Else: doneBB}
	g.currentBB = fireBB
	event := g.lowerSimpleEvent("abort")
	g.lowerEventDispatch(signal, event)
	if g.currentBB.Terminator == nil {
		g.currentBB.Terminator = &ir.JumpTerm{Target: doneBB}
	}
	g.currentBB = doneBB
	return nil, true
}

func (g *generator) lowerAbortSignalMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || objType.Name != "$AbortSignal" || mem.Property != "throwIfAborted" {
		return nil, false
	}
	signal := g.lowerExpr(mem.Object)
	aborted := g.currentFn.NewValue("abort_signal_aborted", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: aborted, Callee: "ts_abort_signal_aborted", Args: []ir.Operand{signal}})
	throwBB := g.currentFn.NewBlock("abort_throw")
	doneBB := g.currentFn.NewBlock("abort_throw_done")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: aborted, Then: throwBB, Else: doneBB}
	g.currentBB = throwBB
	reason := g.currentFn.NewValue("abort_reason", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: reason, Callee: "ts_abort_signal_reason", Args: []ir.Operand{signal}})
	g.routeThrownValue(reason)
	g.currentBB = doneBB
	return nil, true
}
