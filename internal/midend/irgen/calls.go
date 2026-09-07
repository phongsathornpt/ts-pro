package irgen

import (
	"strings"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
	"github.com/phongsathornpt/ts-pro/internal/frontend/sema"
)

func (g *generator) lowerCallExpr(e *ast.CallExpr) ir.Operand {
	if member, ok := e.Callee.(*ast.MemberExpr); ok {
		if ident, ok := member.Object.(*ast.IdentExpr); ok && ident.Name == "performance" {
			switch member.Property {
			case "now":
				res := g.currentFn.NewValue("performance_now", types.TypeNumber)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_performance_now"})
				return res
			case "toJSON":
				t := g.semanticType(e).(*types.ObjectType)
				offsets, refMask, shape := g.objectLayout(t)
				obj := g.currentFn.NewValue("performance_json", t)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{Res: obj, Shape: shape, FieldCount: len(offsets), RefMask: refMask})
				origin := g.currentFn.NewValue("performance_json_origin", types.TypeNumber)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: origin, Callee: "ts_performance_time_origin"})
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: obj, Field: "timeOrigin", Offset: offsets["timeOrigin"], Val: origin})
				return obj
			}
		}
		if promise, handled := g.lowerPromiseStaticCall(e, member); handled {
			return promise
		}
	}
	if ident, ok := e.Callee.(*ast.IdentExpr); ok {
		switch ident.Name {
		case "setTimeout":
			closure := g.lowerExpr(e.Args[0])
			delay := ir.Operand(ir.ConstNumber{Value: 0})
			if len(e.Args) == 2 {
				delay = g.lowerExpr(e.Args[1])
			}
			res := g.currentFn.NewValue("timer_id", types.TypeNumber)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_set_timeout", Args: []ir.Operand{closure, delay}})
			return res
		case "clearTimeout":
			id := g.lowerExpr(e.Args[0])
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_clear_timeout", Args: []ir.Operand{id}})
			return nil
		case "queueMicrotask":
			closure := g.lowerExpr(e.Args[0])
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
				Callee: "ts_microtask_spawn",
				Args:   []ir.Operand{closure, ir.ConstNumber{Value: nativeTaskResultKind(types.TypeVoid)}},
			})
			return nil
		case "atob":
			input := g.lowerExpr(e.Args[0])
			valid := g.currentFn.NewValue("atob_valid", types.TypeBoolean)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: valid, Callee: "ts_atob_valid", Args: []ir.Operand{input}, ParamTypes: []types.Type{types.TypeString}})
			okBB := g.currentFn.NewBlock("atob_decode")
			errBB := g.currentFn.NewBlock("atob_invalid")
			g.currentBB.Terminator = &ir.BranchTerm{Cond: valid, Then: okBB, Else: errBB}
			g.currentBB = errBB
			errObj := g.newDOMException(ir.ConstString{Value: "The string to be decoded is not correctly encoded."}, ir.ConstString{Value: "InvalidCharacterError"})
			g.routeThrownValue(g.boxJSValue(errObj, g.semaResult.DOMExceptionType))
			g.currentBB = okBB
			res := g.currentFn.NewValue("atob_result", types.TypeString)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_atob", Args: []ir.Operand{input}, ParamTypes: []types.Type{types.TypeString}})
			return res
		case "btoa":
			input := g.lowerExpr(e.Args[0])
			valid := g.currentFn.NewValue("btoa_valid", types.TypeBoolean)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: valid, Callee: "ts_btoa_valid", Args: []ir.Operand{input}, ParamTypes: []types.Type{types.TypeString}})
			okBB := g.currentFn.NewBlock("btoa_encode")
			errBB := g.currentFn.NewBlock("btoa_invalid")
			g.currentBB.Terminator = &ir.BranchTerm{Cond: valid, Then: okBB, Else: errBB}
			g.currentBB = errBB
			errObj := g.newDOMException(ir.ConstString{Value: "The string to be encoded contains characters outside of the Latin1 range."}, ir.ConstString{Value: "InvalidCharacterError"})
			g.routeThrownValue(g.boxJSValue(errObj, g.semaResult.DOMExceptionType))
			g.currentBB = okBB
			res := g.currentFn.NewValue("btoa_result", types.TypeString)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_btoa", Args: []ir.Operand{input}, ParamTypes: []types.Type{types.TypeString}})
			return res
		case "taskGroup":
			groupType := g.semanticType(e).(*types.ObjectType)
			res := g.currentFn.NewValue("task_group", groupType)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_task_group_new"})
			return res
		case "groupSpawn":
			group := g.lowerExpr(e.Args[0])
			closure := g.lowerExpr(e.Args[1])
			taskType := g.semanticType(e).(*types.ObjectType)
			resultType := g.semaResult.TaskResults[taskType.Name]
			res := g.currentFn.NewValue("group_task", taskType)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_task_group_spawn", Args: []ir.Operand{group, closure, ir.ConstNumber{Value: nativeTaskResultKind(resultType)}}})
			return res
		case "groupJoin":
			group := g.lowerExpr(e.Args[0])
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_group_join", Args: []ir.Operand{group}})
			return nil
		case "groupCancel":
			group := g.lowerExpr(e.Args[0])
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_group_cancel", Args: []ir.Operand{group}})
			return nil
		case "channel":
			capacity := g.lowerExpr(e.Args[0])
			channelType := g.semanticType(e).(*types.ObjectType)
			res := g.currentFn.NewValue("channel", channelType)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_channel_new", Args: []ir.Operand{capacity}, ParamTypes: []types.Type{types.TypeNumber}})
			return res
		case "channelSend":
			ch := g.lowerExpr(e.Args[0])
			chType := g.semanticType(e.Args[0]).(*types.ObjectType)
			value := g.lowerExpr(e.Args[1])
			value = g.boxJSValue(value, g.semanticType(e.Args[1]))
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_channel_send", Args: []ir.Operand{ch, value}, ParamTypes: []types.Type{chType, types.TypeAny}})
			return nil
		case "channelRecv":
			ch := g.lowerExpr(e.Args[0])
			chType := g.semanticType(e.Args[0]).(*types.ObjectType)
			elem := g.semaResult.ChannelElements[chType.Name]
			boxed := g.currentFn.NewValue("channel_recv", types.TypeAny)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: boxed, Callee: "ts_channel_recv", Args: []ir.Operand{ch}, ParamTypes: []types.Type{chType}})
			return g.coerceJSValueBoundary(boxed, types.TypeAny, elem)
		case "channelTrySend":
			ch := g.lowerExpr(e.Args[0])
			chType := g.semanticType(e.Args[0]).(*types.ObjectType)
			value := g.lowerExpr(e.Args[1])
			value = g.boxJSValue(value, g.semanticType(e.Args[1]))
			res := g.currentFn.NewValue("channel_sent", types.TypeBoolean)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_channel_try_send", Args: []ir.Operand{ch, value}, ParamTypes: []types.Type{chType, types.TypeAny}})
			return res
		case "channelTryRecvOr":
			ch := g.lowerExpr(e.Args[0])
			chType := g.semanticType(e.Args[0]).(*types.ObjectType)
			elem := g.semaResult.ChannelElements[chType.Name]
			fallback := g.lowerExpr(e.Args[1])
			fallback = g.boxJSValue(fallback, g.semanticType(e.Args[1]))
			boxed := g.currentFn.NewValue("channel_boxed", types.TypeAny)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: boxed, Callee: "ts_channel_try_recv_or", Args: []ir.Operand{ch, fallback}, ParamTypes: []types.Type{chType, types.TypeAny}})
			return g.coerceJSValueBoundary(boxed, types.TypeAny, elem)
		case "spawn":
			taskType := g.semanticType(e).(*types.ObjectType)
			resultType := g.semaResult.TaskResults[taskType.Name]
			closure := g.lowerExpr(e.Args[0])
			res := g.currentFn.NewValue("task", taskType)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
				Res: res, Callee: "ts_task_spawn",
				Args: []ir.Operand{closure, ir.ConstNumber{Value: nativeTaskResultKind(resultType)}},
			})
			return res
		case "yieldNow":
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_yield"})
			return nil
		case "sleep":
			ms := g.lowerExpr(e.Args[0])
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_sleep", Args: []ir.Operand{ms}, ParamTypes: []types.Type{types.TypeNumber}})
			return nil
		case "setTaskContext":
			value := g.lowerExpr(e.Args[0])
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_set_context", Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeString}})
			return nil
		case "taskContext":
			res := g.currentFn.NewValue("task_context", types.TypeString)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_task_context"})
			return res
		case "cancelTask":
			task := g.lowerExpr(e.Args[0])
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_cancel", Args: []ir.Operand{task}})
			return nil
		case "taskCancelled":
			res := g.currentFn.NewValue("task_cancelled", types.TypeBoolean)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_task_cancelled"})
			return res
		case "join":
			task := g.lowerExpr(e.Args[0])
			resultType := g.semanticType(e)
			if resultType == nil || resultType.Kind() == types.KindVoid {
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_join", Args: []ir.Operand{task}})
				return nil
			}
			res := g.currentFn.NewValue("task_result", resultType)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_task_join", Args: []ir.Operand{task}})
			return res
		}
	}
	if isConsoleLogCall(e.Callee) {
		return g.lowerConsoleLog(e.Args[0])
	}
	if _, ok := e.Callee.(*ast.SuperExpr); ok {
		thisVal := g.locals["$this"]
		args := make([]ir.Operand, 0, len(e.Args)+1)
		args = append(args, thisVal)
		for _, arg := range e.Args {
			args = append(args, g.lowerExpr(arg))
		}
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: classConstructorName(g.currentClass.BaseName), Args: args})
		return nil
	}
	if mem, ok := e.Callee.(*ast.MemberExpr); ok {
		if res, handled := g.lowerByteLengthQueuingStrategyMethodCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerCountQueuingStrategyMethodCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerReadableStreamStaticCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerReadableStreamMethodCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerReadableStreamDefaultReaderMethodCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerReadableStreamDefaultControllerMethodCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerWritableStreamMethodCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerWritableStreamDefaultWriterMethodCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerWritableStreamDefaultControllerMethodCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerTransformStreamMethodCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerTransformStreamDefaultControllerMethodCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerBlobMethodCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerFormDataMethodCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerHeadersMethodCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerRequestMethodCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerURLPatternMethodCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerURLMethodCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerArrayBufferMethodCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerUint8ArrayMethodCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerTextEncoderMethodCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerTextDecoderMethodCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerURLSearchParamsMethodCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerAbortSignalStaticCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerURLStaticCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerEventMethodCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerEventTargetMethodCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerAbortControllerMethodCall(e, mem); handled {
			return res
		}
		if res, handled := g.lowerAbortSignalMethodCall(e, mem); handled {
			return res
		}
		if proven, ok := g.provenObjectType(mem.Object); ok {
			if field, exists := proven.Fields[mem.Property]; exists {
				if fnType, ok := field.Type.(*types.FunctionType); ok {
					boxedReceiver := g.lowerExpr(mem.Object)
					receiver := g.unboxKnownObject(boxedReceiver, proven)
					offsets, _, _ := g.objectLayout(proven)
					closure := g.currentFn.NewValue("method_closure", fnType)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: closure, Obj: receiver, Field: mem.Property, Offset: offsets[mem.Property]})
					args := make([]ir.Operand, 0, len(e.Args))
					sourceTypes := make([]types.Type, 0, len(e.Args))
					for _, arg := range e.Args {
						args = append(args, g.lowerExpr(arg))
						sourceTypes = append(sourceTypes, g.semanticType(arg))
					}
					args = g.coerceCallOperands(args, sourceTypes, fnType)
					args = g.packRestOperands(args, fnType)
					res := g.currentFn.NewValue("structural_method", fnType.Return)
					paramTypes := make([]types.Type, len(fnType.Params))
					for i := range fnType.Params {
						paramTypes[i] = fnType.Params[i].Type
					}
					var thisArg ir.Operand
					if fnType.This != nil {
						thisArg = receiver
					}
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.IndirectCallInst{Res: res, Closure: closure, ThisArg: thisArg, Args: args, ParamTypes: paramTypes})
					return res
				}
			}
			if info := g.semaResult.Classes[proven.Name]; info != nil && info.Methods[mem.Property] != nil {
				boxed := g.lowerExpr(mem.Object)
				receiver := g.unboxKnownObject(boxed, proven)
				callArgs := make([]ir.Operand, 0, len(e.Args))
				for _, arg := range e.Args {
					callArgs = append(callArgs, g.lowerExpr(arg))
				}
				return g.emitClassMethodCall(receiver, info, mem.Property, callArgs)
			}
		}
		if isBuiltinRegExpType(g.semanticType(mem.Object)) {
			return g.emitRegExpTest(e, mem)
		}
		if ident, ok := mem.Object.(*ast.IdentExpr); ok && ident.Name == "JSON" {
			return g.lowerJSONCall(e, mem)
		}
		if ident, ok := mem.Object.(*ast.IdentExpr); ok && ident.Name == "Date" && mem.Property == "now" {
			res := g.currentFn.NewValue("date_now", types.TypeNumber)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_date_now"})
			return res
		}
		if isBuiltinDateType(g.semanticType(mem.Object)) {
			return g.emitDateMethodCall(e, mem)
		}
		if collection := g.builtinCollectionInfo(g.semanticType(mem.Object)); collection != nil {
			return g.emitBuiltinCollectionCall(e, mem, collection)
		}
		if staticType, ok := g.semanticType(mem.Object).(*types.ObjectType); ok {
			if staticInfo := g.semaResult.Classes[staticType.Name]; staticInfo != nil && staticInfo.Methods[mem.Property] != nil {
				obj := g.lowerExpr(mem.Object)
				callArgs := make([]ir.Operand, 0, len(e.Args))
				for _, arg := range e.Args {
					callArgs = append(callArgs, g.lowerExpr(arg))
				}
				return g.emitClassMethodCall(obj, staticInfo, mem.Property, callArgs)
			}
		}
		if arrType, ok := g.semanticType(mem.Object).(*types.ArrayType); ok {
			array := g.lowerExpr(mem.Object)
			switch mem.Property {
			case "push":
				val := g.lowerExpr(e.Args[0])
				val = g.coerceJSValueBoundary(val, g.semanticType(e.Args[0]), arrType.Elem)
				res := g.currentFn.NewValue("len", types.TypeNumber)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayPushInst{Res: res, Array: array, Val: val})
				return res
			case "pop":
				res := g.currentFn.NewValue("elem", arrType.Elem)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayPopInst{Res: res, Array: array})
				return res
			}
		}
	}
	if fnType, ok := g.provenFunctionType(e.Callee); ok && !isConsoleLogCall(e.Callee) {
		args := make([]ir.Operand, 0, len(e.Args))
		sourceTypes := make([]types.Type, 0, len(e.Args))
		for _, arg := range e.Args {
			args = append(args, g.lowerExpr(arg))
			sourceTypes = append(sourceTypes, g.semanticType(arg))
		}
		args = g.coerceCallOperands(args, sourceTypes, fnType)
		args = g.packRestOperands(args, fnType)
		res := g.currentFn.NewValue("dynamic_call", fnType.Return)
		paramTypes := make([]types.Type, len(fnType.Params))
		for i := range fnType.Params {
			paramTypes[i] = fnType.Params[i].Type
		}
		if target, direct := g.directCalleeForExpr(e.Callee); direct {
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: target, Args: args, ParamTypes: paramTypes})
			return res
		}
		boxedClosure := g.lowerExpr(e.Callee)
		closure := g.coerceJSValueBoundary(boxedClosure, types.TypeAny, fnType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.IndirectCallInst{Res: res, Closure: closure, Args: args, ParamTypes: paramTypes})
		return res
	}

	if fnType, ok := g.semanticType(e.Callee).(*types.FunctionType); ok && !isConsoleLogCall(e.Callee) {
		directNamed := false
		if ident, isIdent := e.Callee.(*ast.IdentExpr); isIdent {
			_, isLocal := g.locals[ident.Name]
			if sym := g.semaResult.Symbols[ident]; !isLocal && sym != nil && sym.Kind == sema.SymFunc {
				directNamed = true
			}
		}
		if !directNamed {
			closure := g.lowerExpr(e.Callee)
			args := make([]ir.Operand, 0, len(e.Args))
			sourceTypes := make([]types.Type, 0, len(e.Args))
			for _, arg := range e.Args {
				args = append(args, g.lowerExpr(arg))
				sourceTypes = append(sourceTypes, g.semanticType(arg))
			}
			args = g.coerceCallOperands(args, sourceTypes, fnType)
			args = g.packRestOperands(args, fnType)
			res := g.currentFn.NewValue("ret", fnType.Return)
			paramTypes := make([]types.Type, len(fnType.Params))
			for i := range fnType.Params {
				paramTypes[i] = fnType.Params[i].Type
			}
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.IndirectCallInst{Res: res, Closure: closure, Args: args, ParamTypes: paramTypes})
			return res
		}
	}
	calleeName := "unknown"
	if ident, ok := e.Callee.(*ast.IdentExpr); ok {
		calleeName = ident.Name
		if imported := g.semaResult.ImportAliases[ident.Name]; imported != "" {
			calleeName = imported
		}
		if decl := g.genericDecls[calleeName]; decl != nil {
			concrete := g.semaResult.GenericCalls[e]
			if len(g.typeBindings) > 0 {
				concrete = types.Substitute(concrete, g.typeBindings).(*types.FunctionType)
			}
			specialized := g.ensureGenericSpecialization(decl, concrete)
			calleeName = specialized
		}
	}
	var args []ir.Operand
	var sourceTypes []types.Type
	for _, arg := range e.Args {
		args = append(args, g.lowerExpr(arg))
		sourceTypes = append(sourceTypes, g.semanticType(arg))
	}
	callType, _ := g.semanticType(e.Callee).(*types.FunctionType)
	if concrete := g.semaResult.GenericCalls[e]; concrete != nil {
		callType = concrete
	}
	if ident, ok := e.Callee.(*ast.IdentExpr); ok {
		if decl := g.functionDecls[ident.Name]; decl != nil {
			fixedLimit := len(decl.Params)
			for i, param := range decl.Params {
				if param.Rest {
					fixedLimit = i
					break
				}
			}
			for i := len(args); i < fixedLimit; i++ {
				p := decl.Params[i]
				if p.Default != nil {
					args = append(args, g.lowerExpr(p.Default))
					sourceTypes = append(sourceTypes, g.semanticType(p.Default))
					continue
				}
				args = append(args, ir.ConstUndefined{})
				sourceTypes = append(sourceTypes, types.TypeUndefined)
				continue
			}
		}
	}
	args = g.coerceCallOperands(args, sourceTypes, callType)
	args = g.packRestOperands(args, callType)
	var paramTypes []types.Type
	if callType != nil && !isConsoleLogCall(e.Callee) && !strings.HasPrefix(calleeName, "ts_") {
		paramTypes = make([]types.Type, len(callType.Params))
		for i := range callType.Params {
			paramTypes[i] = callType.Params[i].Type
		}
	}
	resultType := types.TypeNumber
	if t := g.semanticType(e); t != nil {
		resultType = t
	}
	resVal := g.currentFn.NewValue("ret", resultType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: resVal, Callee: calleeName, Args: args, ParamTypes: paramTypes,
	})
	return resVal
}
