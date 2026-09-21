package irgen

import (
	"fmt"

	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) emitPromiseRejectionLifecycleEvent(eventName string, task, reason ir.Operand) {
	eventType := g.semaResult.EventType
	offsets, refMask, shape := g.objectLayout(eventType)
	event := g.currentFn.NewValue("promise_rejection_lifecycle_event", eventType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
		Res: event, Shape: shape, FieldCount: len(offsets), RefMask: refMask,
	})
	g.setEventField(event, "type", ir.ConstString{Value: eventName})
	g.setEventField(event, "bubbles", ir.ConstBool{Value: false})
	g.setEventField(event, "cancelable", ir.ConstBool{Value: eventName == "unhandledrejection"})
	g.setEventField(event, "composed", ir.ConstBool{Value: false})
	g.setEventField(event, "currentTarget", ir.ConstNull{})
	g.setEventField(event, "target", ir.ConstNull{})
	g.setEventField(event, "defaultPrevented", ir.ConstBool{Value: false})
	g.setEventField(event, "eventPhase", ir.ConstNumber{Value: 0})
	g.setEventField(event, "isTrusted", ir.ConstBool{Value: false})
	timestamp := g.currentFn.NewValue("promise_rejection_lifecycle_timestamp", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: timestamp, Callee: "ts_performance_now"})
	g.setEventField(event, "timeStamp", timestamp)
	g.setEventField(event, "$dispatching", ir.ConstBool{Value: false})
	g.setEventField(event, "$inPassiveListener", ir.ConstBool{Value: false})
	g.setEventField(event, "$stopImmediate", ir.ConstBool{Value: false})
	g.setEventField(event, "$stopPropagation", ir.ConstBool{Value: false})
	g.initEventVariantFields(event, "", nil, nil)

	boxedTask := task
	if !irJSValueType(task.Type()) {
		boxedTask = g.boxJSValue(task, task.Type())
	}
	boxedReason := reason
	if !irJSValueType(reason.Type()) {
		boxedReason = g.boxJSValue(reason, reason.Type())
	}
	g.setEventField(event, "$promise", boxedTask)
	g.setEventField(event, "$reason", boxedReason)

	g.lowerEventDispatch(g.lowerGlobalEventTarget(), event)

	globalType := types.NewObject("$GlobalScope")
	global := g.currentFn.NewValue("promise_rejection_global", globalType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: global, Callee: "ts_global_object"})
	boxedGlobal := g.boxJSValue(global, globalType)
	handler := g.lowerDynamicGet(boxedGlobal, "on"+eventName)
	hasHandler := g.currentFn.NewValue("promise_rejection_has_handler", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: hasHandler, Callee: "ts_js_to_bool", Args: []ir.Operand{handler}, ParamTypes: []types.Type{types.TypeAny},
	})
	callBB := g.currentFn.NewBlock("promise_rejection_handler")
	doneBB := g.currentFn.NewBlock("promise_rejection_handler_done")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: hasHandler, Then: callBB, Else: doneBB}

	g.currentBB = callBB
	handlerType := types.NewFunction([]types.Param{{Name: "event", Type: eventType}}, types.TypeAny)
	closure := g.coerceJSValueBoundary(handler, types.TypeAny, handlerType)
	callResult := g.currentFn.NewValue("promise_rejection_handler_result", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.IndirectCallInst{
		Res: callResult, Closure: closure, Args: []ir.Operand{event}, ParamTypes: []types.Type{eventType},
	})
	callBB.Terminator = &ir.JumpTerm{Target: doneBB}
	g.currentBB = doneBB
}

func (g *generator) scheduleUnhandledRejectionMonitor(task ir.Operand) {
	outerFn, outerBB, outerLocals, outerProv, outerDirect := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	monitorType := types.NewFunction(nil, types.TypeVoid)
	name := fmt.Sprintf("$promise_rejection_monitor%d", g.arrowCounter)
	g.arrowCounter++
	monitor := ir.NewFunction(name, types.TypeVoid)
	g.currentFn = monitor
	g.currentBB = monitor.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)

	env := monitor.NewValue("$env", monitorType)
	monitor.Params = append(monitor.Params, env)
	capturedTask := monitor.NewValue("promise", task.Type())
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ClosureGetInst{Res: capturedTask, Closure: env, Index: 0})

	pollBB := monitor.NewBlock("rejection_monitor_poll")
	pendingBB := monitor.NewBlock("rejection_monitor_pending")
	settledBB := monitor.NewBlock("rejection_monitor_settled")
	reportBB := monitor.NewBlock("rejection_monitor_report")
	doneBB := monitor.NewBlock("rejection_monitor_done")
	g.currentBB.Terminator = &ir.JumpTerm{Target: pollBB}

	done := monitor.NewValue("rejection_monitor_done_flag", types.TypeBoolean)
	pollBB.Instructions = append(pollBB.Instructions, &ir.CallInst{Res: done, Callee: "ts_task_done", Args: []ir.Operand{capturedTask}})
	pollBB.Terminator = &ir.BranchTerm{Cond: done, Then: settledBB, Else: pendingBB}
	pendingBB.Instructions = append(pendingBB.Instructions, &ir.CallInst{Callee: "ts_task_yield"})
	pendingBB.Terminator = &ir.JumpTerm{Target: pollBB}

	rejected := monitor.NewValue("rejection_monitor_rejected", types.TypeBoolean)
	handled := monitor.NewValue("rejection_monitor_handled", types.TypeBoolean)
	notHandled := monitor.NewValue("rejection_monitor_not_handled", types.TypeBoolean)
	shouldReport := monitor.NewValue("rejection_monitor_should_report", types.TypeBoolean)
	settledBB.Instructions = append(settledBB.Instructions,
		&ir.CallInst{Res: rejected, Callee: "ts_task_rejected", Args: []ir.Operand{capturedTask}},
		&ir.CallInst{Res: handled, Callee: "ts_task_rejection_handled", Args: []ir.Operand{capturedTask}},
		&ir.BinaryInst{Res: notHandled, Op: ir.OpEq, LHS: handled, RHS: ir.ConstBool{Value: false}},
		&ir.BinaryInst{Res: shouldReport, Op: ir.OpAnd, LHS: rejected, RHS: notHandled},
	)
	settledBB.Terminator = &ir.BranchTerm{Cond: shouldReport, Then: reportBB, Else: doneBB}

	g.currentBB = reportBB
	reason := monitor.NewValue("unhandled_rejection_reason", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.CallInst{Res: reason, Callee: "ts_task_peek_error", Args: []ir.Operand{capturedTask}},
		&ir.CallInst{Callee: "ts_task_mark_unhandled_reported", Args: []ir.Operand{capturedTask}},
	)
	g.emitPromiseRejectionLifecycleEvent("unhandledrejection", capturedTask, reason)
	g.currentBB.Terminator = &ir.JumpTerm{Target: doneBB}
	doneBB.Terminator = &ir.ReturnTerm{}

	g.prog.Functions = append(g.prog.Functions, monitor)
	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProv, outerDirect

	closure := g.currentFn.NewValue("promise_rejection_monitor", monitorType)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.MakeClosureInst{Res: closure, Function: name, Captures: []ir.Operand{task}, RefMask: 1},
		&ir.CallInst{Callee: "ts_task_spawn", Args: []ir.Operand{closure, ir.ConstNumber{Value: nativeTaskResultKind(types.TypeVoid)}}},
	)
}

func (g *generator) consumePromiseRejection(task ir.Operand) ir.Operand {
	reported := g.currentFn.NewValue("rejection_was_reported", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: reported, Callee: "ts_task_unhandled_reported", Args: []ir.Operand{task},
	})
	reason := g.currentFn.NewValue("promise_rejection_reason", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: reason, Callee: "ts_task_error", Args: []ir.Operand{task},
	})
	notifyBB := g.currentFn.NewBlock("rejection_handled_notify")
	doneBB := g.currentFn.NewBlock("rejection_handled_done")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: reported, Then: notifyBB, Else: doneBB}
	g.currentBB = notifyBB
	g.emitPromiseRejectionLifecycleEvent("rejectionhandled", task, reason)
	g.currentBB.Terminator = &ir.JumpTerm{Target: doneBB}
	g.currentBB = doneBB
	return reason
}
