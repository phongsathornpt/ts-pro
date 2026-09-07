package irgen

import (
	"fmt"
	"strconv"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func irFunctionMemberType(t types.Type) *types.FunctionType {
	if fn, ok := t.(*types.FunctionType); ok {
		return fn
	}
	u := t.(*types.UnionType)
	return u.Members[0].(*types.FunctionType)
}

func (g *generator) thenableMethodType(t types.Type) (*types.ObjectType, *types.FunctionType, bool) {
	obj := t.(*types.ObjectType)
	if info := g.semaResult.Classes[obj.Name]; info != nil {
		if fn := info.Methods["then"]; fn != nil {
			return obj, fn, true
		}
	}
	field, ok := obj.Fields["then"]
	if !ok || field.Type == nil {
		return nil, nil, false
	}
	return obj, irFunctionMemberType(field.Type), true
}

func (g *generator) emitThenableMethodCall(receiver ir.Operand, obj *types.ObjectType, fn *types.FunctionType, args []ir.Operand) {
	if info := g.semaResult.Classes[obj.Name]; info != nil && info.Methods["then"] != nil {
		_ = g.emitClassMethodCall(receiver, info, "then", args)
		return
	}
	field := obj.Fields["then"]
	offsets, _, _ := g.objectLayout(obj)
	rawType := field.Type
	raw := g.currentFn.NewValue("then_method_raw", rawType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: raw, Obj: receiver, Field: "then", Offset: offsets["then"]})
	closure := ir.Operand(raw)
	if rawType != fn {
		unboxed := g.currentFn.NewValue("then_method", fn)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: unboxed, Callee: "ts_js_unbox_ref", Args: []ir.Operand{raw}, ParamTypes: []types.Type{types.TypeAny}})
		closure = unboxed
	}
	paramTypes := make([]types.Type, len(fn.Params))
	for i := range fn.Params {
		paramTypes[i] = fn.Params[i].Type
	}
	var thisArg ir.Operand
	if fn.This != nil {
		thisArg = receiver
	}
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.IndirectCallInst{Closure: closure, ThisArg: thisArg, Args: args, ParamTypes: paramTypes})
}

func (g *generator) makeThenableSettlementCallback(statusCh, valueCh ir.Operand, valueType types.Type, fulfilled bool) ir.Operand {
	outerFn, outerBB, outerLocals, outerProv, outerDirect := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	name := fmt.Sprintf("$promise_settle%d", g.arrowCounter)
	g.arrowCounter++
	fnType := types.NewFunction([]types.Param{{Name: "value", Type: valueType}}, types.TypeVoid)
	lifted := ir.NewFunction(name, types.TypeVoid)
	g.currentFn = lifted
	g.currentBB = lifted.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)
	env := lifted.NewValue("$env", fnType)
	lifted.Params = append(lifted.Params, env)
	status := lifted.NewValue("status_ch", statusCh.Type())
	payload := lifted.NewValue("value_ch", valueCh.Type())
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.ClosureGetInst{Res: status, Closure: env, Index: 0},
		&ir.ClosureGetInst{Res: payload, Closure: env, Index: 1})
	arg := lifted.NewValue("value", valueType)
	lifted.Params = append(lifted.Params, arg)
	boxedStatus := g.boxJSValue(ir.ConstBool{Value: fulfilled}, types.TypeBoolean)
	sent := lifted.NewValue("settled", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: sent, Callee: "ts_channel_try_send", Args: []ir.Operand{status, boxedStatus}, ParamTypes: []types.Type{status.Type(), types.TypeAny}})
	sendBB := lifted.NewBlock("settle_send")
	doneBB := lifted.NewBlock("settle_done")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: sent, Then: sendBB, Else: doneBB}
	g.currentBB = sendBB
	boxedArg := g.boxJSValue(arg, valueType)
	sendBB.Instructions = append(sendBB.Instructions, &ir.CallInst{Callee: "ts_channel_send", Args: []ir.Operand{payload, boxedArg}, ParamTypes: []types.Type{payload.Type(), types.TypeAny}})
	sendBB.Terminator = &ir.JumpTerm{Target: doneBB}
	doneBB.Terminator = &ir.ReturnTerm{}
	g.prog.Functions = append(g.prog.Functions, lifted)
	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProv, outerDirect
	closure := g.currentFn.NewValue("settler", fnType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{Res: closure, Function: name, Captures: []ir.Operand{statusCh, valueCh}, RefMask: 3})
	return closure
}

func (g *generator) lowerThenablePromise(e *ast.CallExpr, taskType *types.ObjectType, inner types.Type, thenableType *types.ObjectType, thenFn *types.FunctionType) ir.Operand {
	thenable := g.lowerExpr(e.Args[0])
	outerFn, outerBB, outerLocals, outerProv, outerDirect := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	driverType := types.NewFunction(nil, inner)
	driverName := fmt.Sprintf("$thenable%d", g.arrowCounter)
	g.arrowCounter++
	driver := ir.NewFunction(driverName, inner)
	g.currentFn = driver
	g.currentBB = driver.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)
	env := driver.NewValue("$env", driverType)
	driver.Params = append(driver.Params, env)
	receiver := driver.NewValue("thenable", thenableType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ClosureGetInst{Res: receiver, Closure: env, Index: 0})
	channelType := types.NewObject(fmt.Sprintf("$ThenableChannel$%d", g.arrowCounter))
	statusCh := driver.NewValue("settle_status", channelType)
	valueCh := driver.NewValue("settle_value", channelType)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.CallInst{Res: statusCh, Callee: "ts_channel_new", Args: []ir.Operand{ir.ConstNumber{Value: 1}}, ParamTypes: []types.Type{types.TypeNumber}},
		&ir.CallInst{Res: valueCh, Callee: "ts_channel_new", Args: []ir.Operand{ir.ConstNumber{Value: 1}}, ParamTypes: []types.Type{types.TypeNumber}})
	resolve := g.makeThenableSettlementCallback(statusCh, valueCh, inner, true)
	reject := g.makeThenableSettlementCallback(statusCh, valueCh, types.TypeAny, false)
	g.emitThenableMethodCall(receiver, thenableType, thenFn, []ir.Operand{resolve, reject})
	statusBox := driver.NewValue("settle_status_box", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: statusBox, Callee: "ts_channel_recv", Args: []ir.Operand{statusCh}, ParamTypes: []types.Type{channelType}})
	status := g.coerceJSValueBoundary(statusBox, types.TypeAny, types.TypeBoolean)
	payload := driver.NewValue("settle_payload", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: payload, Callee: "ts_channel_recv", Args: []ir.Operand{valueCh}, ParamTypes: []types.Type{channelType}})
	okBB := driver.NewBlock("thenable_fulfilled")
	rejectBB := driver.NewBlock("thenable_rejected")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: status, Then: okBB, Else: rejectBB}
	g.currentBB = okBB
	resolved := g.coerceJSValueBoundary(payload, types.TypeAny, inner)
	okBB.Terminator = &ir.ReturnTerm{Val: resolved}
	g.currentBB = rejectBB
	rejectBB.Instructions = append(rejectBB.Instructions, &ir.CallInst{Callee: "ts_task_reject", Args: []ir.Operand{payload}, ParamTypes: []types.Type{types.TypeAny}})
	rejectBB.Terminator = &ir.ReturnTerm{}
	g.prog.Functions = append(g.prog.Functions, driver)
	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProv, outerDirect
	closure := g.currentFn.NewValue("thenable_driver", driverType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{Res: closure, Function: driverName, Captures: []ir.Operand{thenable}, RefMask: 1})
	task := g.currentFn.NewValue("thenable_task", taskType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: task, Callee: "ts_task_spawn", Args: []ir.Operand{closure, ir.ConstNumber{Value: nativeTaskResultKind(inner)}}})
	return task
}

func (g *generator) promiseSettledIRType(t types.Type) types.Type {
	if obj, ok := t.(*types.ObjectType); ok {
		if inner := g.semaResult.TaskResults[obj.Name]; inner != nil {
			return inner
		}
		if _, fn, ok := g.thenableMethodType(obj); ok && len(fn.Params) > 0 {
			if resolve := irFunctionMemberType(fn.Params[0].Type); resolve != nil && len(resolve.Params) > 0 {
				return resolve.Params[0].Type
			}
		}
	}
	return t
}

func (g *generator) makeImmediatePromiseTask(value ir.Operand, sourceType, resultType types.Type) ir.Operand {
	value = g.coerceJSValueBoundary(value, sourceType, resultType)
	outerFn, outerBB, outerLocals, outerProv, outerDirect := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	name := fmt.Sprintf("$promise_immediate%d", g.arrowCounter)
	g.arrowCounter++
	closureType := types.NewFunction(nil, resultType)
	lifted := ir.NewFunction(name, resultType)
	g.currentFn = lifted
	g.currentBB = lifted.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)
	env := lifted.NewValue("$env", closureType)
	lifted.Params = append(lifted.Params, env)
	captured := lifted.NewValue("promise_value", resultType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ClosureGetInst{Res: captured, Closure: env, Index: 0})
	g.currentBB.Terminator = &ir.ReturnTerm{Val: captured}
	g.prog.Functions = append(g.prog.Functions, lifted)
	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProv, outerDirect
	closure := g.currentFn.NewValue("promise_immediate_closure", closureType)
	var refMask uint64
	if irHeapRefType(resultType) {
		refMask = 1
	}
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{Res: closure, Function: name, Captures: []ir.Operand{value}, RefMask: refMask})
	taskType := types.NewObject(fmt.Sprintf("$PromiseImmediate$%d", g.arrowCounter))
	task := g.currentFn.NewValue("promise_immediate_task", taskType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: task, Callee: "ts_task_spawn", Args: []ir.Operand{closure, ir.ConstNumber{Value: nativeTaskResultKind(resultType)}}})
	return task
}

func (g *generator) lowerPromiseAggregateInput(expr ast.Expr) (ir.Operand, types.Type) {
	sourceType := g.semanticType(expr)
	settledType := g.promiseSettledIRType(sourceType)
	if obj, ok := sourceType.(*types.ObjectType); ok {
		if inner := g.semaResult.TaskResults[obj.Name]; inner != nil {
			return g.lowerExpr(expr), inner
		}
		if thenObj, thenFn, isThenable := g.thenableMethodType(obj); isThenable {
			taskType := types.NewObject(fmt.Sprintf("$PromiseAggregateThenable$%d", g.arrowCounter))
			fake := &ast.CallExpr{Args: []ast.Expr{expr}}
			return g.lowerThenablePromise(fake, taskType, settledType, thenObj, thenFn), settledType
		}
	}
	value := g.lowerExpr(expr)
	return g.makeImmediatePromiseTask(value, sourceType, settledType), settledType
}

func (g *generator) lowerPromiseLiteralAggregate(member *ast.MemberExpr, taskType *types.ObjectType, inner types.Type, literal *ast.ArrayLit) ir.Operand {

	tasks := make([]ir.Operand, 0, len(literal.Elements))
	resultTypes := make([]types.Type, 0, len(literal.Elements))
	for _, element := range literal.Elements {
		task, resultType := g.lowerPromiseAggregateInput(element)
		tasks = append(tasks, task)
		resultTypes = append(resultTypes, resultType)
	}

	outerFn, outerBB, outerLocals, outerProv, outerDirect := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	driverType := types.NewFunction(nil, inner)
	driverName := fmt.Sprintf("$promise_%s%d", member.Property, g.arrowCounter)
	g.arrowCounter++
	driver := ir.NewFunction(driverName, inner)
	g.currentFn = driver
	g.currentBB = driver.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)
	env := driver.NewValue("$env", driverType)
	driver.Params = append(driver.Params, env)
	captured := make([]ir.Operand, len(tasks))
	for i, task := range tasks {
		v := driver.NewValue(fmt.Sprintf("aggregate_task_%d", i), task.Type())
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ClosureGetInst{Res: v, Closure: env, Index: i})
		captured[i] = v
	}

	if member.Property == "all" {
		g.lowerPromiseAllDriver(captured, resultTypes, inner)
	} else {
		g.lowerPromiseRaceDriver(captured, resultTypes, inner)
	}
	g.prog.Functions = append(g.prog.Functions, driver)
	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProv, outerDirect

	closure := g.currentFn.NewValue("promise_aggregate_driver", driverType)
	var refMask uint64
	if len(tasks) > 0 {
		refMask = (uint64(1) << len(tasks)) - 1
	}
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{Res: closure, Function: driverName, Captures: tasks, RefMask: refMask})
	result := g.currentFn.NewValue("promise_aggregate_task", taskType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: result, Callee: "ts_task_spawn", Args: []ir.Operand{closure, ir.ConstNumber{Value: nativeTaskResultKind(inner)}}})
	return result
}

func (g *generator) lowerPromiseAllDriver(tasks []ir.Operand, resultTypes []types.Type, inner types.Type) {
	poll := g.currentFn.NewBlock("promise_all_poll")
	pending := g.currentFn.NewBlock("promise_all_pending")
	g.currentBB.Terminator = &ir.JumpTerm{Target: poll}
	g.currentBB = poll

	for i, task := range tasks {
		rejected := g.currentFn.NewValue(fmt.Sprintf("all_rejected_%d", i), types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: rejected, Callee: "ts_task_rejected", Args: []ir.Operand{task}})
		rejectBB := g.currentFn.NewBlock(fmt.Sprintf("promise_all_reject_%d", i))
		nextBB := g.currentFn.NewBlock(fmt.Sprintf("promise_all_reject_next_%d", i))
		g.currentBB.Terminator = &ir.BranchTerm{Cond: rejected, Then: rejectBB, Else: nextBB}
		g.currentBB = rejectBB
		errVal := g.currentFn.NewValue("aggregate_error", types.TypeAny)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.CallInst{Res: errVal, Callee: "ts_task_error", Args: []ir.Operand{task}},
			&ir.CallInst{Callee: "ts_task_reject", Args: []ir.Operand{errVal}, ParamTypes: []types.Type{types.TypeAny}},
		)
		g.currentBB.Terminator = &ir.ReturnTerm{}
		g.currentBB = nextBB
	}

	for i, task := range tasks {
		done := g.currentFn.NewValue(fmt.Sprintf("all_done_%d", i), types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: done, Callee: "ts_task_done", Args: []ir.Operand{task}})
		nextBB := g.currentFn.NewBlock(fmt.Sprintf("promise_all_done_next_%d", i))
		g.currentBB.Terminator = &ir.BranchTerm{Cond: done, Then: nextBB, Else: pending}
		g.currentBB = nextBB
	}

	values := make([]ir.Operand, len(tasks))
	for i, task := range tasks {
		resultType := resultTypes[i]
		if resultType == nil || resultType.Kind() == types.KindVoid {
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_join", Args: []ir.Operand{task}})
			values[i] = ir.ConstUndefined{}
			continue
		}
		value := g.currentFn.NewValue(fmt.Sprintf("all_value_%d", i), resultType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: value, Callee: "ts_task_join", Args: []ir.Operand{task}})
		values[i] = value
	}
	g.finishPromiseAllResult(values, resultTypes, inner)

	g.currentBB = pending
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_yield"})
	g.currentBB.Terminator = &ir.JumpTerm{Target: poll}
}

func (g *generator) finishPromiseAllResult(values []ir.Operand, resultTypes []types.Type, inner types.Type) {
	switch out := inner.(type) {
	case *types.TupleType:
		res := g.currentFn.NewValue("promise_all_tuple", out)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{Res: res, Shape: out.String(), FieldCount: len(out.Elements), RefMask: g.tupleRefMask(out)})
		for i, value := range values {
			coerced := g.coerceJSValueBoundary(value, resultTypes[i], out.Elements[i])
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: res, Field: strconv.Itoa(i), Offset: 16 + i*8, Val: coerced})
		}
		g.currentBB.Terminator = &ir.ReturnTerm{Val: res}
	case *types.ArrayType:
		res := g.currentFn.NewValue("promise_all_array", out)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{Res: res, ElemType: out.Elem, Length: ir.ConstNumber{Value: float64(len(values))}})
		for i, value := range values {
			coerced := g.coerceJSValueBoundary(value, resultTypes[i], out.Elem)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetElementInst{Array: res, Index: ir.ConstNumber{Value: float64(i)}, Val: coerced})
		}
		g.currentBB.Terminator = &ir.ReturnTerm{Val: res}
	}
}

func (g *generator) lowerPromiseRaceDriver(tasks []ir.Operand, resultTypes []types.Type, inner types.Type) {
	poll := g.currentFn.NewBlock("promise_race_poll")
	g.currentBB.Terminator = &ir.JumpTerm{Target: poll}
	g.currentBB = poll
	for i, task := range tasks {
		done := g.currentFn.NewValue(fmt.Sprintf("race_done_%d", i), types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: done, Callee: "ts_task_done", Args: []ir.Operand{task}})
		settledBB := g.currentFn.NewBlock(fmt.Sprintf("promise_race_settled_%d", i))
		nextBB := g.currentFn.NewBlock(fmt.Sprintf("promise_race_next_%d", i))
		g.currentBB.Terminator = &ir.BranchTerm{Cond: done, Then: settledBB, Else: nextBB}
		g.currentBB = settledBB
		rejected := g.currentFn.NewValue(fmt.Sprintf("race_rejected_%d", i), types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: rejected, Callee: "ts_task_rejected", Args: []ir.Operand{task}})
		rejectBB := g.currentFn.NewBlock(fmt.Sprintf("promise_race_reject_%d", i))
		fulfillBB := g.currentFn.NewBlock(fmt.Sprintf("promise_race_fulfill_%d", i))
		g.currentBB.Terminator = &ir.BranchTerm{Cond: rejected, Then: rejectBB, Else: fulfillBB}
		g.currentBB = rejectBB
		errVal := g.currentFn.NewValue("race_error", types.TypeAny)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.CallInst{Res: errVal, Callee: "ts_task_error", Args: []ir.Operand{task}},
			&ir.CallInst{Callee: "ts_task_reject", Args: []ir.Operand{errVal}, ParamTypes: []types.Type{types.TypeAny}},
		)
		g.currentBB.Terminator = &ir.ReturnTerm{}
		g.currentBB = fulfillBB
		resultType := resultTypes[i]
		if resultType == nil || resultType.Kind() == types.KindVoid {
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_join", Args: []ir.Operand{task}})
			g.currentBB.Terminator = &ir.ReturnTerm{Val: ir.ConstUndefined{}}
		} else {
			value := g.currentFn.NewValue(fmt.Sprintf("race_value_%d", i), resultType)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: value, Callee: "ts_task_join", Args: []ir.Operand{task}})
			coerced := g.coerceJSValueBoundary(value, resultType, inner)
			g.currentBB.Terminator = &ir.ReturnTerm{Val: coerced}
		}
		g.currentBB = nextBB
	}
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_yield"})
	g.currentBB.Terminator = &ir.JumpTerm{Target: poll}
}

func (g *generator) promiseArrayTaskResultType(t types.Type) (types.Type, bool) {
	if obj, ok := t.(*types.ObjectType); ok {
		inner := g.semaResult.TaskResults[obj.Name]
		return inner, inner != nil
	}
	if union, ok := t.(*types.UnionType); ok {
		members := make([]types.Type, 0, len(union.Members))
		for _, member := range union.Members {
			inner, _ := g.promiseArrayTaskResultType(member)
			members = append(members, inner)
		}
		return types.NewUnion(members...), true
	}
	return t, true
}

func (g *generator) lowerPromiseArrayAggregate(e *ast.CallExpr, member *ast.MemberExpr, taskType *types.ObjectType, inner types.Type) ir.Operand {
	arrType := g.semanticType(e.Args[0]).(*types.ArrayType)
	resultType, _ := g.promiseArrayTaskResultType(arrType.Elem)
	source := g.lowerExpr(e.Args[0])
	outerFn, outerBB, outerLocals, outerProv, outerDirect := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	driverType := types.NewFunction(nil, inner)
	driverName := fmt.Sprintf("$promise_%s_array%d", member.Property, g.arrowCounter)
	g.arrowCounter++
	driver := ir.NewFunction(driverName, inner)
	g.currentFn = driver
	g.currentBB = driver.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)
	env := driver.NewValue("$env", driverType)
	driver.Params = append(driver.Params, env)
	array := driver.NewValue("aggregate_array", arrType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ClosureGetInst{Res: array, Closure: env, Index: 0})
	length := driver.NewValue("aggregate_length", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: array})
	if member.Property == "all" {
		g.lowerPromiseAllArrayDriver(array, length, arrType.Elem, resultType, inner)
	} else {
		g.lowerPromiseRaceArrayDriver(array, length, arrType.Elem, resultType, inner)
	}
	g.prog.Functions = append(g.prog.Functions, driver)
	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProv, outerDirect
	closure := g.currentFn.NewValue("promise_array_driver", driverType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{Res: closure, Function: driverName, Captures: []ir.Operand{source}, RefMask: 1})
	result := g.currentFn.NewValue("promise_array_task", taskType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: result, Callee: "ts_task_spawn", Args: []ir.Operand{closure, ir.ConstNumber{Value: nativeTaskResultKind(inner)}}})
	return result
}

func (g *generator) lowerPromiseAllArrayDriver(array, length ir.Operand, taskElemType, resultType, inner types.Type) {
	out := inner.(*types.ArrayType)
	poll := g.currentFn.NewBlock("promise_all_array_poll")
	rejectCond := g.currentFn.NewBlock("promise_all_array_reject_cond")
	rejectBody := g.currentFn.NewBlock("promise_all_array_reject_body")
	rejectNext := g.currentFn.NewBlock("promise_all_array_reject_next")
	doneCond := g.currentFn.NewBlock("promise_all_array_done_cond")
	doneBody := g.currentFn.NewBlock("promise_all_array_done_body")
	doneNext := g.currentFn.NewBlock("promise_all_array_done_next")
	pending := g.currentFn.NewBlock("promise_all_array_pending")
	allocBB := g.currentFn.NewBlock("promise_all_array_alloc")
	fillCond := g.currentFn.NewBlock("promise_all_array_fill_cond")
	fillBody := g.currentFn.NewBlock("promise_all_array_fill_body")
	fillDone := g.currentFn.NewBlock("promise_all_array_fill_done")
	g.currentBB.Terminator = &ir.JumpTerm{Target: poll}
	poll.Terminator = &ir.JumpTerm{Target: rejectCond}

	rejectIndex := g.currentFn.NewValue("all_reject_i", types.TypeNumber)
	rejectNextIndex := g.currentFn.NewValue("all_reject_next", types.TypeNumber)
	rejectCond.Phis = append(rejectCond.Phis, &ir.PhiInst{Res: rejectIndex, Incoming: []ir.PhiIncoming{{Block: poll, Value: ir.ConstNumber{Value: 0}}, {Block: rejectNext, Value: rejectNextIndex}}})
	rejectMore := g.currentFn.NewValue("all_reject_more", types.TypeBoolean)
	rejectCond.Instructions = append(rejectCond.Instructions, &ir.BinaryInst{Res: rejectMore, Op: ir.OpLt, LHS: rejectIndex, RHS: length})
	rejectCond.Terminator = &ir.BranchTerm{Cond: rejectMore, Then: rejectBody, Else: doneCond}
	task := g.currentFn.NewValue("all_reject_task", taskElemType)
	rejected := g.currentFn.NewValue("all_array_rejected", types.TypeBoolean)
	rejectBody.Instructions = append(rejectBody.Instructions,
		&ir.GetElementInst{Res: task, Array: array, Index: rejectIndex},
		&ir.CallInst{Res: rejected, Callee: "ts_task_rejected", Args: []ir.Operand{task}},
	)
	rejectFail := g.currentFn.NewBlock("promise_all_array_reject")
	rejectBody.Terminator = &ir.BranchTerm{Cond: rejected, Then: rejectFail, Else: rejectNext}
	errVal := g.currentFn.NewValue("all_array_error", types.TypeAny)
	rejectFail.Instructions = append(rejectFail.Instructions,
		&ir.CallInst{Res: errVal, Callee: "ts_task_error", Args: []ir.Operand{task}},
		&ir.CallInst{Callee: "ts_task_reject", Args: []ir.Operand{errVal}, ParamTypes: []types.Type{types.TypeAny}},
	)
	rejectFail.Terminator = &ir.ReturnTerm{}
	rejectNext.Instructions = append(rejectNext.Instructions, &ir.BinaryInst{Res: rejectNextIndex, Op: ir.OpAdd, LHS: rejectIndex, RHS: ir.ConstNumber{Value: 1}})
	rejectNext.Terminator = &ir.JumpTerm{Target: rejectCond}

	doneIndex := g.currentFn.NewValue("all_done_i", types.TypeNumber)
	doneNextIndex := g.currentFn.NewValue("all_done_next", types.TypeNumber)
	doneCond.Phis = append(doneCond.Phis, &ir.PhiInst{Res: doneIndex, Incoming: []ir.PhiIncoming{{Block: rejectCond, Value: ir.ConstNumber{Value: 0}}, {Block: doneNext, Value: doneNextIndex}}})
	doneMore := g.currentFn.NewValue("all_done_more", types.TypeBoolean)
	doneCond.Instructions = append(doneCond.Instructions, &ir.BinaryInst{Res: doneMore, Op: ir.OpLt, LHS: doneIndex, RHS: length})
	doneCond.Terminator = &ir.BranchTerm{Cond: doneMore, Then: doneBody, Else: allocBB}
	doneTask := g.currentFn.NewValue("all_done_task", taskElemType)
	done := g.currentFn.NewValue("all_array_done", types.TypeBoolean)
	doneBody.Instructions = append(doneBody.Instructions,
		&ir.GetElementInst{Res: doneTask, Array: array, Index: doneIndex},
		&ir.CallInst{Res: done, Callee: "ts_task_done", Args: []ir.Operand{doneTask}},
	)
	doneBody.Terminator = &ir.BranchTerm{Cond: done, Then: doneNext, Else: pending}
	doneNext.Instructions = append(doneNext.Instructions, &ir.BinaryInst{Res: doneNextIndex, Op: ir.OpAdd, LHS: doneIndex, RHS: ir.ConstNumber{Value: 1}})
	doneNext.Terminator = &ir.JumpTerm{Target: doneCond}
	pending.Instructions = append(pending.Instructions, &ir.CallInst{Callee: "ts_task_yield"})
	pending.Terminator = &ir.JumpTerm{Target: poll}

	result := g.currentFn.NewValue("promise_all_array", out)
	allocBB.Instructions = append(allocBB.Instructions, &ir.AllocArrayInst{Res: result, ElemType: out.Elem, Length: length})
	allocBB.Terminator = &ir.JumpTerm{Target: fillCond}
	fillIndex := g.currentFn.NewValue("all_fill_i", types.TypeNumber)
	fillNextIndex := g.currentFn.NewValue("all_fill_next", types.TypeNumber)
	fillCond.Phis = append(fillCond.Phis, &ir.PhiInst{Res: fillIndex, Incoming: []ir.PhiIncoming{{Block: allocBB, Value: ir.ConstNumber{Value: 0}}, {Block: fillBody, Value: fillNextIndex}}})
	fillMore := g.currentFn.NewValue("all_fill_more", types.TypeBoolean)
	fillCond.Instructions = append(fillCond.Instructions, &ir.BinaryInst{Res: fillMore, Op: ir.OpLt, LHS: fillIndex, RHS: length})
	fillCond.Terminator = &ir.BranchTerm{Cond: fillMore, Then: fillBody, Else: fillDone}
	fillTask := g.currentFn.NewValue("all_fill_task", taskElemType)
	fillValue := g.currentFn.NewValue("all_fill_value", resultType)
	fillBody.Instructions = append(fillBody.Instructions,
		&ir.GetElementInst{Res: fillTask, Array: array, Index: fillIndex},
		&ir.CallInst{Res: fillValue, Callee: "ts_task_join", Args: []ir.Operand{fillTask}},
	)
	g.currentBB = fillBody
	stored := g.coerceJSValueBoundary(fillValue, resultType, out.Elem)
	fillBody.Instructions = append(fillBody.Instructions,
		&ir.SetElementInst{Array: result, Index: fillIndex, Val: stored},
		&ir.BinaryInst{Res: fillNextIndex, Op: ir.OpAdd, LHS: fillIndex, RHS: ir.ConstNumber{Value: 1}},
	)
	fillBody.Terminator = &ir.JumpTerm{Target: fillCond}
	fillDone.Terminator = &ir.ReturnTerm{Val: result}
	g.currentBB = fillDone
}

func (g *generator) lowerPromiseRaceArrayDriver(array, length ir.Operand, taskElemType, resultType, inner types.Type) {
	poll := g.currentFn.NewBlock("promise_race_array_poll")
	cond := g.currentFn.NewBlock("promise_race_array_cond")
	body := g.currentFn.NewBlock("promise_race_array_body")
	next := g.currentFn.NewBlock("promise_race_array_next")
	pending := g.currentFn.NewBlock("promise_race_array_pending")
	g.currentBB.Terminator = &ir.JumpTerm{Target: poll}
	poll.Terminator = &ir.JumpTerm{Target: cond}

	index := g.currentFn.NewValue("race_array_i", types.TypeNumber)
	nextIndex := g.currentFn.NewValue("race_array_next_i", types.TypeNumber)
	cond.Phis = append(cond.Phis, &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{{Block: poll, Value: ir.ConstNumber{Value: 0}}, {Block: next, Value: nextIndex}}})
	more := g.currentFn.NewValue("race_array_more", types.TypeBoolean)
	cond.Instructions = append(cond.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	cond.Terminator = &ir.BranchTerm{Cond: more, Then: body, Else: pending}
	task := g.currentFn.NewValue("race_array_task", taskElemType)
	done := g.currentFn.NewValue("race_array_done", types.TypeBoolean)
	body.Instructions = append(body.Instructions,
		&ir.GetElementInst{Res: task, Array: array, Index: index},
		&ir.CallInst{Res: done, Callee: "ts_task_done", Args: []ir.Operand{task}},
	)
	settled := g.currentFn.NewBlock("promise_race_array_settled")
	body.Terminator = &ir.BranchTerm{Cond: done, Then: settled, Else: next}
	next.Instructions = append(next.Instructions, &ir.BinaryInst{Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
	next.Terminator = &ir.JumpTerm{Target: cond}
	pending.Instructions = append(pending.Instructions, &ir.CallInst{Callee: "ts_task_yield"})
	pending.Terminator = &ir.JumpTerm{Target: poll}

	rejected := g.currentFn.NewValue("race_array_rejected", types.TypeBoolean)
	settled.Instructions = append(settled.Instructions, &ir.CallInst{Res: rejected, Callee: "ts_task_rejected", Args: []ir.Operand{task}})
	rejectBB := g.currentFn.NewBlock("promise_race_array_reject")
	fulfillBB := g.currentFn.NewBlock("promise_race_array_fulfill")
	settled.Terminator = &ir.BranchTerm{Cond: rejected, Then: rejectBB, Else: fulfillBB}
	errVal := g.currentFn.NewValue("race_array_error", types.TypeAny)
	rejectBB.Instructions = append(rejectBB.Instructions,
		&ir.CallInst{Res: errVal, Callee: "ts_task_error", Args: []ir.Operand{task}},
		&ir.CallInst{Callee: "ts_task_reject", Args: []ir.Operand{errVal}, ParamTypes: []types.Type{types.TypeAny}},
	)
	rejectBB.Terminator = &ir.ReturnTerm{}
	value := g.currentFn.NewValue("race_array_value", resultType)
	fulfillBB.Instructions = append(fulfillBB.Instructions, &ir.CallInst{Res: value, Callee: "ts_task_join", Args: []ir.Operand{task}})
	g.currentBB = fulfillBB
	coerced := g.coerceJSValueBoundary(value, resultType, inner)
	fulfillBB.Terminator = &ir.ReturnTerm{Val: coerced}
	g.currentBB = fulfillBB
}

func (g *generator) lowerPromiseStaticCall(e *ast.CallExpr, member *ast.MemberExpr) (ir.Operand, bool) {
	ident, ok := member.Object.(*ast.IdentExpr)
	if !ok || ident.Name != "Promise" {
		return nil, false
	}
	if member.Property == "all" || member.Property == "race" {
		taskType := g.semanticType(e).(*types.ObjectType)
		inner := g.semaResult.TaskResults[taskType.Name]
		if literal, ok := e.Args[0].(*ast.ArrayLit); ok {
			return g.lowerPromiseLiteralAggregate(member, taskType, inner, literal), true
		}
		return g.lowerPromiseArrayAggregate(e, member, taskType, inner), true
	}
	taskType := g.semanticType(e).(*types.ObjectType)
	inner := g.semaResult.TaskResults[taskType.Name]
	if member.Property == "resolve" {
		if argObj, ok := g.semanticType(e.Args[0]).(*types.ObjectType); ok {
			if _, isTask := g.semaResult.TaskResults[argObj.Name]; isTask {
				return g.lowerExpr(e.Args[0]), true
			}
			if thenObj, thenFn, isThenable := g.thenableMethodType(argObj); isThenable {
				return g.lowerThenablePromise(e, taskType, inner, thenObj, thenFn), true
			}
		}
	}

	outerFn, outerBB, outerLocals, outerProv, outerDirect := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	closureType := types.NewFunction(nil, inner)
	liftedName := fmt.Sprintf("$promise%d", g.arrowCounter)
	g.arrowCounter++
	var capture ir.Operand
	var captureType types.Type
	if member.Property == "resolve" {
		capture = g.lowerExpr(e.Args[0])
		captureType = g.semanticType(e.Args[0])
		capture = g.coerceJSValueBoundary(capture, captureType, inner)
		captureType = inner
	} else {
		capture = g.lowerExpr(e.Args[0])
		capture = g.boxJSValue(capture, g.semanticType(e.Args[0]))
		captureType = types.TypeAny
	}

	lifted := ir.NewFunction(liftedName, inner)
	g.currentFn = lifted
	g.currentBB = lifted.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)
	env := lifted.NewValue("$env", closureType)
	lifted.Params = append(lifted.Params, env)
	captured := lifted.NewValue("promise_value", captureType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ClosureGetInst{Res: captured, Closure: env, Index: 0})
	if member.Property == "resolve" {
		g.currentBB.Terminator = &ir.ReturnTerm{Val: captured}
	} else {
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_reject", Args: []ir.Operand{captured}, ParamTypes: []types.Type{types.TypeAny}})
		g.currentBB.Terminator = &ir.ReturnTerm{}
	}
	g.prog.Functions = append(g.prog.Functions, lifted)
	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProv, outerDirect

	closure := g.currentFn.NewValue("promise_closure", closureType)
	var refMask uint64
	if irHeapRefType(captureType) {
		refMask = 1
	}
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{Res: closure, Function: liftedName, Captures: []ir.Operand{capture}, RefMask: refMask})
	task := g.currentFn.NewValue("promise_task", taskType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: task, Callee: "ts_task_spawn", Args: []ir.Operand{closure, ir.ConstNumber{Value: nativeTaskResultKind(inner)}}})
	return task, true
}
