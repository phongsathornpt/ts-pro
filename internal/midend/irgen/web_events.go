package irgen

import (
	"fmt"
	"strings"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) eventField(obj ir.Operand, name string, resultType types.Type) ir.Operand {
	offsets, _, _ := g.objectLayout(g.semaResult.EventType)
	res := g.currentFn.NewValue("event_"+strings.TrimPrefix(name, "$"), resultType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: res, Obj: obj, Field: name, Offset: offsets[name]})
	return res
}

func (g *generator) setEventField(obj ir.Operand, name string, value ir.Operand) {
	offsets, _, _ := g.objectLayout(g.semaResult.EventType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: obj, Field: name, Offset: offsets[name], Val: value})
}

func (g *generator) lowerWebIDLBoolean(value ir.Operand, sourceType types.Type) ir.Operand {
	if sourceType == types.TypeBoolean && value.Type() == types.TypeBoolean {
		return value
	}
	boxed := value
	if !irJSValueType(value.Type()) {
		boxed = g.boxJSValue(value, sourceType)
	}
	res := g.currentFn.NewValue("webidl_bool", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: res, Callee: "ts_js_to_bool", Args: []ir.Operand{boxed}, ParamTypes: []types.Type{types.TypeAny},
	})
	return res
}

func (g *generator) lowerWebIDLDictionaryMember(expr ast.Expr, value ir.Operand, name string, targetType types.Type, fallback ir.Operand) (ir.Operand, bool) {
	if expr == nil || value == nil {
		return fallback, false
	}
	convert := func(raw ir.Operand, sourceType types.Type) ir.Operand {
		if targetType == types.TypeBoolean {
			return g.lowerWebIDLBoolean(raw, sourceType)
		}
		if targetType == types.TypeAny {
			if irJSValueType(raw.Type()) {
				return raw
			}
			return g.boxJSValue(raw, sourceType)
		}
		return g.coerceJSValueBoundary(raw, sourceType, targetType)
	}
	if objType, ok := g.semanticType(expr).(*types.ObjectType); ok {
		field, exists := objType.Fields[name]
		if !exists {
			return fallback, false
		}
		offsets, _, _ := g.objectLayout(objType)
		raw := g.currentFn.NewValue("webidl_dict_"+name, field.Type)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: raw, Obj: value, Field: name, Offset: offsets[name]})
		return convert(raw, field.Type), true
	}
	if irJSValueType(value.Type()) {
		raw := g.lowerDynamicGet(value, name)
		return convert(raw, types.TypeAny), true
	}
	return fallback, false
}

func (g *generator) lowerWebIDLDictionaryBool(expr ast.Expr, value ir.Operand, name string, fallback bool) ir.Operand {
	fallbackValue := ir.Operand(ir.ConstBool{Value: fallback})
	result, _ := g.lowerWebIDLDictionaryMember(expr, value, name, types.TypeBoolean, fallbackValue)
	return result
}

func (g *generator) lowerEventInitBool(init ast.Expr, initValue ir.Operand, name string) ir.Operand {
	return g.lowerWebIDLDictionaryBool(init, initValue, name, false)
}

func (g *generator) lowerEventInitValue(init ast.Expr, initValue ir.Operand, name string, targetType types.Type, fallback ir.Operand) ir.Operand {
	result, _ := g.lowerWebIDLDictionaryMember(init, initValue, name, targetType, fallback)
	return result
}

func (g *generator) initEventVariantFields(obj ir.Operand, className string, init ast.Expr, initValue ir.Operand) {
	g.setEventField(obj, "$detail", ir.ConstNull{})
	g.setEventField(obj, "$data", ir.ConstNull{})
	g.setEventField(obj, "$origin", ir.ConstString{Value: ""})
	g.setEventField(obj, "$lastEventId", ir.ConstString{Value: ""})
	g.setEventField(obj, "$source", ir.ConstNull{})
	g.setEventField(obj, "$ports", ir.ConstNull{})
	g.setEventField(obj, "$message", ir.ConstString{Value: ""})
	g.setEventField(obj, "$filename", ir.ConstString{Value: ""})
	g.setEventField(obj, "$lineno", ir.ConstNumber{Value: 0})
	g.setEventField(obj, "$colno", ir.ConstNumber{Value: 0})
	g.setEventField(obj, "$error", ir.ConstNull{})
	switch className {
	case "CustomEvent":
		g.setEventField(obj, "$detail", g.lowerEventInitValue(init, initValue, "detail", types.TypeAny, ir.ConstNull{}))
	case "MessageEvent":
		g.setEventField(obj, "$data", g.lowerEventInitValue(init, initValue, "data", types.TypeAny, ir.ConstNull{}))
		g.setEventField(obj, "$origin", g.lowerEventInitValue(init, initValue, "origin", types.TypeString, ir.ConstString{Value: ""}))
		g.setEventField(obj, "$lastEventId", g.lowerEventInitValue(init, initValue, "lastEventId", types.TypeString, ir.ConstString{Value: ""}))
		g.setEventField(obj, "$source", g.lowerEventInitValue(init, initValue, "source", types.TypeAny, ir.ConstNull{}))
		g.setEventField(obj, "$ports", g.lowerEventInitValue(init, initValue, "ports", types.TypeAny, ir.ConstNull{}))
	case "ErrorEvent":
		g.setEventField(obj, "$message", g.lowerEventInitValue(init, initValue, "message", types.TypeString, ir.ConstString{Value: ""}))
		g.setEventField(obj, "$filename", g.lowerEventInitValue(init, initValue, "filename", types.TypeString, ir.ConstString{Value: ""}))
		g.setEventField(obj, "$lineno", g.lowerEventInitValue(init, initValue, "lineno", types.TypeNumber, ir.ConstNumber{Value: 0}))
		g.setEventField(obj, "$colno", g.lowerEventInitValue(init, initValue, "colno", types.TypeNumber, ir.ConstNumber{Value: 0}))
		g.setEventField(obj, "$error", g.lowerEventInitValue(init, initValue, "error", types.TypeAny, ir.ConstNull{}))
	}
}

func (g *generator) lowerEventConstructor(e *ast.NewExpr, resultType *types.ObjectType) ir.Operand {
	eventType := g.semaResult.EventType
	typeValue := g.lowerExpr(e.Args[0])
	var initExpr ast.Expr
	var initValue ir.Operand
	if len(e.Args) > 1 {
		initExpr = e.Args[1]
		initValue = g.lowerExpr(initExpr)
	}
	bubbles := g.lowerEventInitBool(initExpr, initValue, "bubbles")
	cancelable := g.lowerEventInitBool(initExpr, initValue, "cancelable")
	composed := g.lowerEventInitBool(initExpr, initValue, "composed")
	offsets, refMask, shape := g.objectLayout(eventType)
	obj := g.currentFn.NewValue("event", resultType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{Res: obj, Shape: shape, FieldCount: len(offsets), RefMask: refMask})
	g.setEventField(obj, "type", typeValue)
	g.setEventField(obj, "bubbles", bubbles)
	g.setEventField(obj, "cancelable", cancelable)
	g.setEventField(obj, "composed", composed)
	g.setEventField(obj, "currentTarget", ir.ConstNull{})
	g.setEventField(obj, "target", ir.ConstNull{})
	g.setEventField(obj, "defaultPrevented", ir.ConstBool{Value: false})
	g.setEventField(obj, "eventPhase", ir.ConstNumber{Value: 0})
	g.setEventField(obj, "isTrusted", ir.ConstBool{Value: false})
	timestamp := g.currentFn.NewValue("event_timestamp", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: timestamp, Callee: "ts_performance_now"})
	g.setEventField(obj, "timeStamp", timestamp)
	g.setEventField(obj, "$dispatching", ir.ConstBool{Value: false})
	g.setEventField(obj, "$inPassiveListener", ir.ConstBool{Value: false})
	g.setEventField(obj, "$stopImmediate", ir.ConstBool{Value: false})
	g.setEventField(obj, "$stopPropagation", ir.ConstBool{Value: false})
	g.initEventVariantFields(obj, e.ClassName, initExpr, initValue)
	return obj
}

func (g *generator) nullRef(t types.Type) ir.Operand {
	res := g.currentFn.NewValue("null_ref", t)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_null_ref"})
	return res
}

func eventPhysicalProperty(objType *types.ObjectType, property string) (string, bool) {
	switch property {
	case "type", "target", "currentTarget", "bubbles", "cancelable", "defaultPrevented", "composed", "isTrusted", "eventPhase", "timeStamp":
		return property, true
	}
	switch objType.Name {
	case "$CustomEvent":
		if property == "detail" {
			return "$detail", true
		}
	case "$MessageEvent":
		switch property {
		case "data":
			return "$data", true
		case "origin":
			return "$origin", true
		case "lastEventId":
			return "$lastEventId", true
		case "source":
			return "$source", true
		case "ports":
			return "$ports", true
		}
	case "$ErrorEvent":
		switch property {
		case "message":
			return "$message", true
		case "filename":
			return "$filename", true
		case "lineno":
			return "$lineno", true
		case "colno":
			return "$colno", true
		case "error":
			return "$error", true
		}
	}
	return "", false
}

func isEventObjectType(t *types.ObjectType) bool {
	if t == nil {
		return false
	}
	switch t.Name {
	case "$Event", "$CustomEvent", "$MessageEvent", "$ErrorEvent":
		return true
	default:
		return false
	}
}

func (g *generator) lowerEventMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || (objType.Name != "$Event" && objType.Name != "$CustomEvent" && objType.Name != "$MessageEvent" && objType.Name != "$ErrorEvent") {
		return nil, false
	}
	event := g.lowerExpr(mem.Object)
	switch mem.Property {
	case "preventDefault":
		cancelable := g.eventField(event, "cancelable", types.TypeBoolean)
		passive := g.eventField(event, "$inPassiveListener", types.TypeBoolean)
		notPassive := g.currentFn.NewValue("event_not_passive", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: notPassive, Op: ir.OpEq, LHS: passive, RHS: ir.ConstBool{Value: false}})
		canPrevent := g.currentFn.NewValue("event_can_prevent", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: canPrevent, Op: ir.OpAnd, LHS: cancelable, RHS: notPassive})
		setBB := g.currentFn.NewBlock("event_prevent_default")
		doneBB := g.currentFn.NewBlock("event_prevent_default_done")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: canPrevent, Then: setBB, Else: doneBB}
		g.currentBB = setBB
		g.setEventField(event, "defaultPrevented", ir.ConstBool{Value: true})
		setBB.Terminator = &ir.JumpTerm{Target: doneBB}
		g.currentBB = doneBB
		return nil, true
	case "stopPropagation":
		g.setEventField(event, "$stopPropagation", ir.ConstBool{Value: true})
		return nil, true
	case "stopImmediatePropagation":
		g.setEventField(event, "$stopPropagation", ir.ConstBool{Value: true})
		g.setEventField(event, "$stopImmediate", ir.ConstBool{Value: true})
		return nil, true
	case "composedPath":
		arrType := types.NewArray(types.TypeAny)
		arr := g.currentFn.NewValue("event_path", arrType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{Res: arr, Length: ir.ConstNumber{Value: 0}, ElemType: types.TypeAny})
		target := g.eventField(event, "target", types.TypeAny)
		length := g.currentFn.NewValue("event_path_len", types.TypeNumber)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayPushInst{Res: length, Array: arr, Val: target})
		return arr, true
	}
	return nil, false
}

func (g *generator) lowerEventListenerOptionBool(expr ast.Expr, value ir.Operand, name string) ir.Operand {
	if expr == nil {
		return ir.ConstBool{Value: false}
	}
	if g.semanticType(expr) == types.TypeBoolean {
		if name == "capture" {
			return value
		}
		return ir.ConstBool{Value: false}
	}
	return g.lowerWebIDLDictionaryBool(expr, value, name, false)
}

func (g *generator) lowerEventListenerSignal(expr ast.Expr, value ir.Operand) (ir.Operand, bool) {
	if expr == nil {
		return nil, false
	}
	objType, ok := g.semanticType(expr).(*types.ObjectType)
	if !ok {
		return nil, false
	}
	if _, exists := objType.Fields["signal"]; !exists {
		return nil, false
	}
	return g.lowerWebIDLDictionaryMember(expr, value, "signal", g.semaResult.AbortSignalType, ir.ConstNull{})
}

func (g *generator) makeEventListenerAbortRemovalCallback(target, eventType, callback, capture ir.Operand) ir.Operand {
	outerFn, outerBB, outerLocals, outerProv, outerDirect := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	fnType := types.NewFunction([]types.Param{{Name: "event", Type: g.semaResult.EventType}}, types.TypeVoid)
	name := fmt.Sprintf("$event_listener_abort%d", g.arrowCounter)
	g.arrowCounter++
	lifted := ir.NewFunction(name, types.TypeVoid)
	g.currentFn = lifted
	g.currentBB = lifted.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)
	env := lifted.NewValue("$env", fnType)
	lifted.Params = append(lifted.Params, env)
	capturedTarget := lifted.NewValue("target", target.Type())
	capturedType := lifted.NewValue("type", types.TypeString)
	capturedCallback := lifted.NewValue("callback", callback.Type())
	capturedCapture := lifted.NewValue("capture", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.ClosureGetInst{Res: capturedTarget, Closure: env, Index: 0},
		&ir.ClosureGetInst{Res: capturedType, Closure: env, Index: 1},
		&ir.ClosureGetInst{Res: capturedCallback, Closure: env, Index: 2},
		&ir.ClosureGetInst{Res: capturedCapture, Closure: env, Index: 3})
	ignoredEvent := lifted.NewValue("event", g.semaResult.EventType)
	lifted.Params = append(lifted.Params, ignoredEvent)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_event_target_remove", Args: []ir.Operand{capturedTarget, capturedType, capturedCallback, capturedCapture}})
	g.currentBB.Terminator = &ir.ReturnTerm{}
	g.prog.Functions = append(g.prog.Functions, lifted)
	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProv, outerDirect
	closure := g.currentFn.NewValue("event_listener_abort_callback", fnType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{Res: closure, Function: name, Captures: []ir.Operand{target, eventType, callback, capture}, RefMask: 0b111})
	return closure
}

func (g *generator) lowerEventTargetMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || (objType.Name != "$EventTarget" && objType.Name != "$AbortSignal" && objType.Name != "$Performance") {
		return nil, false
	}
	var target ir.Operand
	if objType.Name == "$Performance" {
		target = g.lowerPerformanceEventTarget()
	} else {
		target = g.lowerExpr(mem.Object)
	}
	switch mem.Property {
	case "addEventListener":
		typeArg := g.lowerExpr(e.Args[0])
		callback := g.lowerExpr(e.Args[1])
		capture := ir.Operand(ir.ConstBool{Value: false})
		once := ir.Operand(ir.ConstBool{Value: false})
		passive := ir.Operand(ir.ConstBool{Value: false})
		var signal ir.Operand
		hasSignal := false
		if len(e.Args) > 2 {
			options := g.lowerExpr(e.Args[2])
			capture = g.lowerEventListenerOptionBool(e.Args[2], options, "capture")
			once = g.lowerEventListenerOptionBool(e.Args[2], options, "once")
			passive = g.lowerEventListenerOptionBool(e.Args[2], options, "passive")
			signal, hasSignal = g.lowerEventListenerSignal(e.Args[2], options)
		}
		if !hasSignal {
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
				Callee: "ts_event_target_add", Args: []ir.Operand{target, typeArg, callback, once, capture, passive},
			})
			return nil, true
		}
		aborted := g.currentFn.NewValue("event_listener_signal_aborted", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: aborted, Callee: "ts_abort_signal_aborted", Args: []ir.Operand{signal}})
		addBB := g.currentFn.NewBlock("event_listener_signal_add")
		doneBB := g.currentFn.NewBlock("event_listener_signal_done")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: aborted, Then: doneBB, Else: addBB}
		g.currentBB = addBB
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Callee: "ts_event_target_add", Args: []ir.Operand{target, typeArg, callback, once, capture, passive},
		})
		removal := g.makeEventListenerAbortRemovalCallback(target, typeArg, callback, capture)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Callee: "ts_event_target_add", Args: []ir.Operand{signal, ir.ConstString{Value: "abort"}, removal, ir.ConstBool{Value: true}, ir.ConstBool{Value: false}, ir.ConstBool{Value: false}},
		})
		g.currentBB.Terminator = &ir.JumpTerm{Target: doneBB}
		g.currentBB = doneBB
		return nil, true
	case "removeEventListener":
		typeArg := g.lowerExpr(e.Args[0])
		callback := g.lowerExpr(e.Args[1])
		capture := ir.Operand(ir.ConstBool{Value: false})
		if len(e.Args) > 2 {
			options := g.lowerExpr(e.Args[2])
			capture = g.lowerEventListenerOptionBool(e.Args[2], options, "capture")
		}
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Callee: "ts_event_target_remove", Args: []ir.Operand{target, typeArg, callback, capture},
		})
		return nil, true
	case "dispatchEvent":
		return g.lowerEventDispatch(target, g.lowerExpr(e.Args[0])), true
	}
	return nil, false
}

func (g *generator) lowerEventDispatch(target, event ir.Operand) ir.Operand {
	eventType := g.semaResult.EventType
	listenerType := types.NewObject("$EventListener")
	listenerFn := types.NewFunction([]types.Param{{Name: "event", Type: eventType}}, types.TypeVoid)
	g.setEventField(event, "target", g.boxJSValue(target, g.semaResult.EventTargetType))
	g.setEventField(event, "currentTarget", g.boxJSValue(target, g.semaResult.EventTargetType))
	g.setEventField(event, "eventPhase", ir.ConstNumber{Value: 2})
	g.setEventField(event, "$dispatching", ir.ConstBool{Value: true})
	g.setEventField(event, "$stopImmediate", ir.ConstBool{Value: false})
	g.setEventField(event, "$stopPropagation", ir.ConstBool{Value: false})
	boundary := g.currentFn.NewValue("event_listener_boundary", listenerType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: boundary, Callee: "ts_event_target_tail", Args: []ir.Operand{target}})

	entry := g.currentBB
	loopBB := g.currentFn.NewBlock("event_listener_loop")
	bodyBB := g.currentFn.NewBlock("event_listener_body")
	onceBB := g.currentFn.NewBlock("event_listener_once")
	invokeBB := g.currentFn.NewBlock("event_listener_invoke")
	doneBB := g.currentFn.NewBlock("event_dispatch_done")
	entry.Terminator = &ir.JumpTerm{Target: loopBB}

	prev := g.currentFn.NewValue("event_prev_listener", listenerType)
	loopBB.Phis = append(loopBB.Phis, &ir.PhiInst{Res: prev, Incoming: []ir.PhiIncoming{{Block: entry, Value: g.nullRefAt(entry, listenerType)}}})
	eventName := g.currentFn.NewValue("event_type_for_listener", types.TypeString)
	eventOffsets, _, _ := g.objectLayout(eventType)
	loopBB.Instructions = append(loopBB.Instructions, &ir.GetFieldInst{Res: eventName, Obj: event, Field: "type", Offset: eventOffsets["type"]})
	next := g.currentFn.NewValue("event_listener", listenerType)
	loopBB.Instructions = append(loopBB.Instructions, &ir.CallInst{Res: next, Callee: "ts_event_target_next", Args: []ir.Operand{target, eventName, prev}})
	loopBB.Terminator = &ir.BranchTerm{Cond: next, Then: bodyBB, Else: doneBB}

	once := g.currentFn.NewValue("event_listener_once", types.TypeBoolean)
	passive := g.currentFn.NewValue("event_listener_passive", types.TypeBoolean)
	bodyBB.Instructions = append(bodyBB.Instructions,
		&ir.CallInst{Res: once, Callee: "ts_event_listener_once", Args: []ir.Operand{next}},
		&ir.CallInst{Res: passive, Callee: "ts_event_listener_passive", Args: []ir.Operand{next}})
	bodyBB.Terminator = &ir.BranchTerm{Cond: once, Then: onceBB, Else: invokeBB}
	onceBB.Instructions = append(onceBB.Instructions, &ir.CallInst{Callee: "ts_event_listener_remove", Args: []ir.Operand{next}})
	onceBB.Terminator = &ir.JumpTerm{Target: invokeBB}

	callback := g.currentFn.NewValue("event_callback", listenerFn)
	invokeBB.Instructions = append(invokeBB.Instructions, &ir.CallInst{Res: callback, Callee: "ts_event_listener_callback", Args: []ir.Operand{next}})
	invokeBB.Instructions = append(invokeBB.Instructions, &ir.SetFieldInst{Obj: event, Field: "$inPassiveListener", Offset: eventOffsets["$inPassiveListener"], Val: passive})
	invokeBB.Instructions = append(invokeBB.Instructions, &ir.IndirectCallInst{Closure: callback, Args: []ir.Operand{event}, ParamTypes: []types.Type{eventType}})
	invokeBB.Instructions = append(invokeBB.Instructions, &ir.SetFieldInst{Obj: event, Field: "$inPassiveListener", Offset: eventOffsets["$inPassiveListener"], Val: ir.ConstBool{Value: false}})
	stop := g.currentFn.NewValue("event_stop_immediate", types.TypeBoolean)
	offsets, _, _ := g.objectLayout(eventType)
	invokeBB.Instructions = append(invokeBB.Instructions, &ir.GetFieldInst{Res: stop, Obj: event, Field: "$stopImmediate", Offset: offsets["$stopImmediate"]})
	atBoundary := g.currentFn.NewValue("event_at_boundary", types.TypeBoolean)
	invokeBB.Instructions = append(invokeBB.Instructions, &ir.BinaryInst{Res: atBoundary, Op: ir.OpEq, LHS: next, RHS: boundary})
	finish := g.currentFn.NewValue("event_finish_dispatch", types.TypeBoolean)
	invokeBB.Instructions = append(invokeBB.Instructions, &ir.BinaryInst{Res: finish, Op: ir.OpOr, LHS: stop, RHS: atBoundary})
	invokeBB.Terminator = &ir.BranchTerm{Cond: finish, Then: doneBB, Else: loopBB}
	loopBB.Phis[0].Incoming = append(loopBB.Phis[0].Incoming, ir.PhiIncoming{Block: invokeBB, Value: next})

	g.currentBB = doneBB
	g.setEventField(event, "currentTarget", ir.ConstNull{})
	g.setEventField(event, "eventPhase", ir.ConstNumber{Value: 0})
	g.setEventField(event, "$dispatching", ir.ConstBool{Value: false})
	g.setEventField(event, "$inPassiveListener", ir.ConstBool{Value: false})
	defaultPrevented := g.eventField(event, "defaultPrevented", types.TypeBoolean)
	result := g.currentFn.NewValue("event_dispatch_result", types.TypeBoolean)
	doneBB.Instructions = append(doneBB.Instructions, &ir.BinaryInst{Res: result, Op: ir.OpEq, LHS: defaultPrevented, RHS: ir.ConstBool{Value: false}})
	return result
}
