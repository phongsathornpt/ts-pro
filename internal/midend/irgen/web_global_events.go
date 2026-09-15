package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

const globalEventTargetKey = "__tspro_internal_global_event_target__"

func (g *generator) lowerGlobalEventTarget() ir.Operand {
	globalType := types.NewObject("$GlobalScope")
	global := g.currentFn.NewValue("global_event_scope", globalType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: global, Callee: "ts_global_object"})
	boxedGlobal := g.boxJSValue(global, globalType)

	existing := g.lowerDynamicGet(boxedGlobal, globalEventTargetKey)
	boxedUndefined := g.boxJSValue(ir.ConstUndefined{}, types.TypeUndefined)
	missing := g.currentFn.NewValue("global_event_target_missing", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res:        missing,
		Callee:     "ts_js_strict_eq",
		Args:       []ir.Operand{existing, boxedUndefined},
		ParamTypes: []types.Type{types.TypeAny, types.TypeAny},
	})

	createBB := g.currentFn.NewBlock("global_event_target_create")
	doneBB := g.currentFn.NewBlock("global_event_target_done")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: missing, Then: createBB, Else: doneBB}

	g.currentBB = createBB
	target := g.currentFn.NewValue("global_event_target", g.semaResult.EventTargetType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: target, Callee: "ts_event_target_new"})
	created := g.boxJSValue(target, g.semaResult.EventTargetType)
	set := g.currentFn.NewValue("global_event_target_set", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res:        set,
		Callee:     "ts_dynamic_set",
		Args:       []ir.Operand{boxedGlobal, ir.ConstString{Value: globalEventTargetKey}, created},
		ParamTypes: []types.Type{types.TypeAny, types.TypeString, types.TypeAny},
	})
	createBB.Terminator = &ir.JumpTerm{Target: doneBB}

	g.currentBB = doneBB
	boxed := g.lowerDynamicGet(boxedGlobal, globalEventTargetKey)
	return g.coerceJSValueBoundary(boxed, types.TypeAny, g.semaResult.EventTargetType)
}

func (g *generator) lowerReportError(expr ast.Expr) ir.Operand {
	value := g.lowerExpr(expr)
	boxedValue := value
	if !irJSValueType(value.Type()) {
		boxedValue = g.boxJSValue(value, g.semanticType(expr))
	}
	message := g.currentFn.NewValue("report_error_message", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: message, Callee: "ts_js_to_string", Args: []ir.Operand{boxedValue}, ParamTypes: []types.Type{types.TypeAny},
	})

	// Reported exceptions use the ordinary global EventTarget listener path.
	// Keep the physical Event ABI so generic `error` listeners observe the same
	// dispatch state as explicitly-created Event/ErrorEvent values.
	eventType := g.semaResult.EventType
	offsets, refMask, shape := g.objectLayout(eventType)
	event := g.currentFn.NewValue("reported_error_event", eventType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
		Res: event, Shape: shape, FieldCount: len(offsets), RefMask: refMask,
	})
	g.setEventField(event, "type", ir.ConstString{Value: "error"})
	g.setEventField(event, "bubbles", ir.ConstBool{Value: false})
	g.setEventField(event, "cancelable", ir.ConstBool{Value: true})
	g.setEventField(event, "composed", ir.ConstBool{Value: false})
	g.setEventField(event, "currentTarget", ir.ConstNull{})
	g.setEventField(event, "target", ir.ConstNull{})
	g.setEventField(event, "defaultPrevented", ir.ConstBool{Value: false})
	g.setEventField(event, "eventPhase", ir.ConstNumber{Value: 0})
	g.setEventField(event, "isTrusted", ir.ConstBool{Value: false})
	timestamp := g.currentFn.NewValue("reported_error_timestamp", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: timestamp, Callee: "ts_performance_now"})
	g.setEventField(event, "timeStamp", timestamp)
	g.setEventField(event, "$dispatching", ir.ConstBool{Value: false})
	g.setEventField(event, "$inPassiveListener", ir.ConstBool{Value: false})
	g.setEventField(event, "$stopImmediate", ir.ConstBool{Value: false})
	g.setEventField(event, "$stopPropagation", ir.ConstBool{Value: false})
	g.initEventVariantFields(event, "ErrorEvent", nil, nil)
	g.setEventField(event, "$message", message)
	g.setEventField(event, "$error", boxedValue)
	g.lowerEventDispatch(g.lowerGlobalEventTarget(), event)

	globalType := types.NewObject("$GlobalScope")
	global := g.currentFn.NewValue("report_error_global", globalType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: global, Callee: "ts_global_object"})
	boxedGlobal := g.boxJSValue(global, globalType)
	handler := g.lowerDynamicGet(boxedGlobal, "onerror")
	hasHandler := g.currentFn.NewValue("report_error_has_handler", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: hasHandler, Callee: "ts_js_to_bool", Args: []ir.Operand{handler}, ParamTypes: []types.Type{types.TypeAny},
	})
	callBB := g.currentFn.NewBlock("report_error_handler")
	doneBB := g.currentFn.NewBlock("report_error_done")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: hasHandler, Then: callBB, Else: doneBB}

	g.currentBB = callBB
	handlerType := types.NewFunction([]types.Param{
		{Name: "message", Type: types.TypeString},
		{Name: "source", Type: types.TypeString},
		{Name: "lineno", Type: types.TypeNumber},
		{Name: "colno", Type: types.TypeNumber},
		{Name: "error", Type: types.TypeAny},
	}, types.TypeAny)
	closure := g.coerceJSValueBoundary(handler, types.TypeAny, handlerType)
	callResult := g.currentFn.NewValue("report_error_handler_result", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.IndirectCallInst{
		Res:        callResult,
		Closure:    closure,
		Args:       []ir.Operand{message, ir.ConstString{Value: ""}, ir.ConstNumber{Value: 0}, ir.ConstNumber{Value: 0}, boxedValue},
		ParamTypes: []types.Type{types.TypeString, types.TypeString, types.TypeNumber, types.TypeNumber, types.TypeAny},
	})
	callBB.Terminator = &ir.JumpTerm{Target: doneBB}
	g.currentBB = doneBB
	return nil
}
