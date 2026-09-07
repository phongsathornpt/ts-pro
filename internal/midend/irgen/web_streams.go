package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

// --- Queuing Strategies ---

func (g *generator) lowerByteLengthQueuingStrategyNew(e *ast.NewExpr) ir.Operand {
	hwm := ir.Operand(ir.ConstNumber{Value: 0})
	if len(e.Args) > 0 {
		if lit, ok := e.Args[0].(*ast.ObjectLit); ok {
			for _, prop := range lit.Properties {
				if prop.Key == "highWaterMark" {
					val := g.lowerExpr(prop.Value)
					hwm = g.coerceNumberOperand(val, g.semanticType(prop.Value))
					break
				}
			}
		} else {
			opts := g.lowerExpr(e.Args[0])
			boxed := g.boxJSValue(opts, g.semanticType(e.Args[0]))
			dynHwm := g.lowerDynamicGet(boxed, "highWaterMark")
			hwm = g.coerceNumberOperand(dynHwm, types.TypeAny)
		}
	}
	t := g.semaResult.ByteLengthQueuingStrategyType
	offsets, refMask, shape := g.objectLayout(t)
	res := g.currentFn.NewValue("bl_strategy", t)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
		Res: res, Shape: shape, FieldCount: len(offsets), RefMask: refMask,
	})
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: res, Field: "highWaterMark", Offset: offsets["highWaterMark"], Val: hwm},
	)
	return res
}

func (g *generator) lowerCountQueuingStrategyNew(e *ast.NewExpr) ir.Operand {
	hwm := ir.Operand(ir.ConstNumber{Value: 0})
	if len(e.Args) > 0 {
		if lit, ok := e.Args[0].(*ast.ObjectLit); ok {
			for _, prop := range lit.Properties {
				if prop.Key == "highWaterMark" {
					val := g.lowerExpr(prop.Value)
					hwm = g.coerceNumberOperand(val, g.semanticType(prop.Value))
					break
				}
			}
		} else {
			opts := g.lowerExpr(e.Args[0])
			boxed := g.boxJSValue(opts, g.semanticType(e.Args[0]))
			dynHwm := g.lowerDynamicGet(boxed, "highWaterMark")
			hwm = g.coerceNumberOperand(dynHwm, types.TypeAny)
		}
	}
	t := g.semaResult.CountQueuingStrategyType
	offsets, refMask, shape := g.objectLayout(t)
	res := g.currentFn.NewValue("count_strategy", t)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
		Res: res, Shape: shape, FieldCount: len(offsets), RefMask: refMask,
	})
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: res, Field: "highWaterMark", Offset: offsets["highWaterMark"], Val: hwm},
	)
	return res
}

func (g *generator) lowerByteLengthQueuingStrategyMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || objType.Name != "$ByteLengthQueuingStrategy" {
		return nil, false
	}
	if mem.Property == "size" {
		if len(e.Args) == 0 {
			return ir.ConstNumber{Value: 0}, true
		}
		chunkArg := e.Args[0]
		chunkType := g.semanticType(chunkArg)
		if ot, ok := chunkType.(*types.ObjectType); ok {
			if ot.Name == "$ArrayBuffer" {
				chunk := g.lowerExpr(chunkArg)
				data := g.arrayBufferData(chunk)
				res := g.currentFn.NewValue("ab_byte_len", types.TypeNumber)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
					Res: res, Callee: "ts_byte_buffer_len", Args: []ir.Operand{data}, ParamTypes: []types.Type{g.semaResult.ByteBufferType},
				})
				return res, true
			}
			if ot.Name == "$Uint8Array" {
				chunk := g.lowerExpr(chunkArg)
				offsets, _, _ := g.objectLayout(ot)
				res := g.currentFn.NewValue("u8_byte_len", types.TypeNumber)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{
					Res: res, Obj: chunk, Field: "byteLength", Offset: offsets["byteLength"],
				})
				return res, true
			}
			if _, ok := ot.Fields["byteLength"]; ok {
				chunk := g.lowerExpr(chunkArg)
				offsets, _, _ := g.objectLayout(ot)
				res := g.currentFn.NewValue("chunk_byte_len", types.TypeNumber)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{
					Res: res, Obj: chunk, Field: "byteLength", Offset: offsets["byteLength"],
				})
				return res, true
			}
		}
		chunk := g.lowerExpr(chunkArg)
		boxed := g.boxJSValue(chunk, chunkType)
		byteLen := g.lowerDynamicGet(boxed, "byteLength")
		res := g.coerceNumberOperand(byteLen, types.TypeAny)
		return res, true
	}
	return nil, false
}

func (g *generator) lowerCountQueuingStrategyMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || objType.Name != "$CountQueuingStrategy" {
		return nil, false
	}
	if mem.Property == "size" {
		return ir.ConstNumber{Value: 1}, true
	}
	return nil, false
}

// --- ReadableStream Core ---

func (g *generator) newReadableStreamCore(hwm ir.Operand) (ir.Operand, ir.Operand) {
	t := g.semaResult.ReadableStreamType
	offsets, refMask, shape := g.objectLayout(t)
	stream := g.currentFn.NewValue("stream", t)

	queue := g.currentFn.NewValue("stream_queue", types.NewArray(types.TypeAny))
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{
		Res: queue, Length: ir.ConstNumber{Value: 0}, ElemType: types.TypeAny,
	})

	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
		Res: stream, Shape: shape, FieldCount: len(offsets), RefMask: refMask,
	})
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: stream, Field: "locked", Offset: offsets["locked"], Val: ir.ConstBool{Value: false}},
		&ir.SetFieldInst{Obj: stream, Field: "$state", Offset: offsets["$state"], Val: ir.ConstString{Value: "readable"}},
		&ir.SetFieldInst{Obj: stream, Field: "$queue", Offset: offsets["$queue"], Val: queue},
		&ir.SetFieldInst{Obj: stream, Field: "$queueIndex", Offset: offsets["$queueIndex"], Val: ir.ConstNumber{Value: 0}},
		&ir.SetFieldInst{Obj: stream, Field: "$reader", Offset: offsets["$reader"], Val: ir.ConstUndefined{}},
		&ir.SetFieldInst{Obj: stream, Field: "$source", Offset: offsets["$source"], Val: ir.ConstUndefined{}},
		&ir.SetFieldInst{Obj: stream, Field: "$pullFn", Offset: offsets["$pullFn"], Val: ir.ConstUndefined{}},
		&ir.SetFieldInst{Obj: stream, Field: "$cancelFn", Offset: offsets["$cancelFn"], Val: ir.ConstUndefined{}},
		&ir.SetFieldInst{Obj: stream, Field: "$highWaterMark", Offset: offsets["$highWaterMark"], Val: hwm},
		&ir.SetFieldInst{Obj: stream, Field: "$storedError", Offset: offsets["$storedError"], Val: ir.ConstUndefined{}},
	)

	ctrlType := g.semaResult.ReadableStreamDefaultControllerType
	ctrlOffsets, ctrlRefMask, ctrlShape := g.objectLayout(ctrlType)
	ctrl := g.currentFn.NewValue("ctrl", ctrlType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
		Res: ctrl, Shape: ctrlShape, FieldCount: len(ctrlOffsets), RefMask: ctrlRefMask,
	})
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: ctrl, Field: "$stream", Offset: ctrlOffsets["$stream"], Val: stream},
		&ir.SetFieldInst{Obj: ctrl, Field: "desiredSize", Offset: ctrlOffsets["desiredSize"], Val: hwm},
	)

	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: stream, Field: "$controller", Offset: offsets["$controller"], Val: ctrl},
	)

	return stream, ctrl
}

func (g *generator) newReadableStreamFromByteBuffer(data ir.Operand) ir.Operand {
	length := g.currentFn.NewValue("body_stream_len", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: length, Callee: "ts_byte_buffer_len", Args: []ir.Operand{data}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}})
	// Request/Response own their body buffer, so the stream can expose a view over
	// the same immutable bytes. Avoid an eager full-body copy, especially for fetch.
	ab := g.newArrayBufferFromData(data)
	u8 := g.newUint8ArrayView(data, ab, ir.ConstNumber{Value: 0}, length)

	stream, ctrl := g.newReadableStreamCore(ir.ConstNumber{Value: 1})
	sOffsets, _, _ := g.objectLayout(g.semaResult.ReadableStreamType)
	ctrlOffsets, _, _ := g.objectLayout(g.semaResult.ReadableStreamDefaultControllerType)
	boxedU8 := g.boxJSValue(u8, g.semaResult.Uint8ArrayType)
	queue := g.currentFn.NewValue("body_stream_queue", types.NewArray(types.TypeAny))
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.GetFieldInst{Res: queue, Obj: stream, Field: "$queue", Offset: sOffsets["$queue"]},
		&ir.ArrayPushInst{Res: g.currentFn.NewValue("body_stream_push", types.TypeNumber), Array: queue, Val: boxedU8},
		&ir.SetFieldInst{Obj: stream, Field: "$state", Offset: sOffsets["$state"], Val: ir.ConstString{Value: "closed"}},
		&ir.SetFieldInst{Obj: ctrl, Field: "desiredSize", Offset: ctrlOffsets["desiredSize"], Val: ir.ConstNumber{Value: 0}},
	)
	return stream
}

func (g *generator) newBodyStream(data, hasBody ir.Operand) ir.Operand {
	pre := g.currentBB
	withBody := g.currentFn.NewBlock("body_stream_present")
	withoutBody := g.currentFn.NewBlock("body_stream_absent")
	join := g.currentFn.NewBlock("body_stream_join")
	pre.Terminator = &ir.BranchTerm{Cond: hasBody, Then: withBody, Else: withoutBody}
	g.currentBB = withBody
	stream := g.newReadableStreamFromByteBuffer(data)
	boxedStream := g.boxJSValue(stream, g.semaResult.ReadableStreamType)
	withBodyEnd := g.currentBB
	withBodyEnd.Terminator = &ir.JumpTerm{Target: join}
	g.currentBB = withoutBody
	withoutBodyEnd := g.currentBB
	withoutBodyEnd.Terminator = &ir.JumpTerm{Target: join}
	g.currentBB = join
	bodyStream := g.currentFn.NewValue("body_stream", types.TypeAny)
	join.Phis = append(join.Phis, &ir.PhiInst{Res: bodyStream, Incoming: []ir.PhiIncoming{{Block: withBodyEnd, Value: boxedStream}, {Block: withoutBodyEnd, Value: ir.ConstNull{}}}})
	return bodyStream
}

func (g *generator) lowerReadableStreamNew(e *ast.NewExpr) ir.Operand {
	hwm := ir.Operand(ir.ConstNumber{Value: 1})
	if len(e.Args) > 1 {
		if lit, ok := e.Args[1].(*ast.ObjectLit); ok {
			for _, prop := range lit.Properties {
				if prop.Key == "highWaterMark" {
					val := g.lowerExpr(prop.Value)
					hwm = g.coerceNumberOperand(val, g.semanticType(prop.Value))
					break
				}
			}
		} else {
			opts := g.lowerExpr(e.Args[1])
			boxed := g.boxJSValue(opts, g.semanticType(e.Args[1]))
			dynHwm := g.lowerDynamicGet(boxed, "highWaterMark")
			hwm = g.coerceNumberOperand(dynHwm, types.TypeAny)
		}
	}

	stream, ctrl := g.newReadableStreamCore(hwm)
	streamType := g.semaResult.ReadableStreamType
	offsets, _, _ := g.objectLayout(streamType)

	if len(e.Args) > 0 {
		srcExpr := e.Args[0]
		ctrlType := g.semaResult.ReadableStreamDefaultControllerType
		boxedCtrl := g.boxJSValue(ctrl, ctrlType)

		if lit, ok := srcExpr.(*ast.ObjectLit); ok {
			for _, prop := range lit.Properties {
				switch prop.Key {
				case "start":
					prevKind := g.activeStreamControllerKind
					g.activeStreamControllerKind = "readable"
					startVal := g.lowerExpr(prop.Value)
					g.activeStreamControllerKind = prevKind
					arg := ir.Operand(ctrl)
					argType := types.Type(ctrlType)
					if fnType, ok := g.semanticType(prop.Value).(*types.FunctionType); ok && len(fnType.Params) > 0 {
						if irJSValueType(fnType.Params[0].Type) {
							arg = boxedCtrl
							argType = types.TypeAny
						}
					}
					callRes := g.currentFn.NewValue("start_res", types.TypeVoid)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.IndirectCallInst{
						Res:        callRes,
						Closure:    startVal,
						ThisArg:    nil,
						Args:       []ir.Operand{arg},
						ParamTypes: []types.Type{argType},
					})
				case "pull":
					prevKind := g.activeStreamControllerKind
					g.activeStreamControllerKind = "readable"
					pullVal := g.lowerExpr(prop.Value)
					g.activeStreamControllerKind = prevKind
					boxedPull := g.boxJSValue(pullVal, g.semanticType(prop.Value))
					g.currentBB.Instructions = append(g.currentBB.Instructions,
						&ir.SetFieldInst{Obj: stream, Field: "$pullFn", Offset: offsets["$pullFn"], Val: boxedPull},
					)
				case "cancel":
					prevKind := g.activeStreamControllerKind
					g.activeStreamControllerKind = "readable"
					cancelVal := g.lowerExpr(prop.Value)
					g.activeStreamControllerKind = prevKind
					boxedCancel := g.boxJSValue(cancelVal, g.semanticType(prop.Value))
					g.currentBB.Instructions = append(g.currentBB.Instructions,
						&ir.SetFieldInst{Obj: stream, Field: "$cancelFn", Offset: offsets["$cancelFn"], Val: boxedCancel},
					)
				}
			}
		} else {
			srcVal := g.lowerExpr(srcExpr)
			boxedSrc := g.boxJSValue(srcVal, g.semanticType(srcExpr))
			g.currentBB.Instructions = append(g.currentBB.Instructions,
				&ir.SetFieldInst{Obj: stream, Field: "$source", Offset: offsets["$source"], Val: boxedSrc},
			)
			// Extract and invoke start if present
			startVal := g.lowerDynamicGet(boxedSrc, "start")
			hasStart := g.currentFn.NewValue("has_start", types.TypeBoolean)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
				Res: hasStart, Op: ir.OpNe, LHS: startVal, RHS: ir.ConstUndefined{},
			})
			startBB := g.currentFn.NewBlock("stream_has_start")
			nextBB := g.currentFn.NewBlock("stream_no_start")
			g.currentBB.Terminator = &ir.BranchTerm{Cond: hasStart, Then: startBB, Else: nextBB}

			g.currentBB = startBB
			callRes := g.currentFn.NewValue("dyn_start_res", types.TypeVoid)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.IndirectCallInst{
				Res:        callRes,
				Closure:    startVal,
				ThisArg:    boxedSrc,
				Args:       []ir.Operand{boxedCtrl},
				ParamTypes: []types.Type{types.TypeAny},
			})
			g.currentBB.Terminator = &ir.JumpTerm{Target: nextBB}

			g.currentBB = nextBB
			pullVal := g.lowerDynamicGet(boxedSrc, "pull")
			cancelVal := g.lowerDynamicGet(boxedSrc, "cancel")
			g.currentBB.Instructions = append(g.currentBB.Instructions,
				&ir.SetFieldInst{Obj: stream, Field: "$pullFn", Offset: offsets["$pullFn"], Val: pullVal},
				&ir.SetFieldInst{Obj: stream, Field: "$cancelFn", Offset: offsets["$cancelFn"], Val: cancelVal},
			)
		}
	}

	return stream
}

func (g *generator) lowerReadableStreamDefaultReaderNew(e *ast.NewExpr) ir.Operand {
	if len(e.Args) == 0 {
		return ir.ConstUndefined{}
	}
	stream := g.lowerExpr(e.Args[0])
	return g.lockReaderForStream(stream)
}

func (g *generator) lockReaderForStream(stream ir.Operand) ir.Operand {
	streamType := g.semaResult.ReadableStreamType
	sOffsets, _, _ := g.objectLayout(streamType)

	// Lock stream
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: stream, Field: "locked", Offset: sOffsets["locked"], Val: ir.ConstBool{Value: true}},
	)

	// Allocate reader
	rType := g.semaResult.ReadableStreamDefaultReaderType
	rOffsets, rRefMask, rShape := g.objectLayout(rType)
	reader := g.currentFn.NewValue("reader", rType)
	closedPromise := g.makeImmediatePromiseTask(ir.ConstUndefined{}, types.TypeUndefined, types.TypeUndefined)

	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
		Res: reader, Shape: rShape, FieldCount: len(rOffsets), RefMask: rRefMask,
	})
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: reader, Field: "$stream", Offset: rOffsets["$stream"], Val: stream},
		&ir.SetFieldInst{Obj: reader, Field: "closed", Offset: rOffsets["closed"], Val: closedPromise},
	)

	// Store reader onto stream
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: stream, Field: "$reader", Offset: sOffsets["$reader"], Val: reader},
	)

	return reader
}

func (g *generator) lowerReadableStreamMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || objType.Name != "$ReadableStream" {
		return nil, false
	}
	stream := g.lowerExpr(mem.Object)

	switch mem.Property {
	case "getReader":
		reader := g.lockReaderForStream(stream)
		return reader, true

	case "cancel":
		reason := ir.Operand(ir.ConstUndefined{})
		if len(e.Args) > 0 {
			reason = g.lowerExpr(e.Args[0])
			reason = g.boxJSValue(reason, g.semanticType(e.Args[0]))
		}
		streamType := g.semaResult.ReadableStreamType
		offsets, _, _ := g.objectLayout(streamType)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.SetFieldInst{Obj: stream, Field: "$state", Offset: offsets["$state"], Val: ir.ConstString{Value: "closed"}},
		)
		cancelFn := g.currentFn.NewValue("cancel_fn", types.TypeAny)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.GetFieldInst{Res: cancelFn, Obj: stream, Field: "$cancelFn", Offset: offsets["$cancelFn"]},
		)
		hasCancel := g.currentFn.NewValue("has_cancel", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
			Res: hasCancel, Op: ir.OpNe, LHS: cancelFn, RHS: ir.ConstUndefined{},
		})
		callBB := g.currentFn.NewBlock("cancel_call")
		doneBB := g.currentFn.NewBlock("cancel_done")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: hasCancel, Then: callBB, Else: doneBB}

		g.currentBB = callBB
		fnType := types.NewFunction([]types.Param{{Name: "reason", Type: types.TypeAny}}, types.TypeVoid)
		unboxedCancelFn := g.coerceJSValueBoundary(cancelFn, types.TypeAny, fnType)
		cRes := g.currentFn.NewValue("cancel_call_res", types.TypeVoid)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.IndirectCallInst{
			Res:        cRes,
			Closure:    unboxedCancelFn,
			ThisArg:    nil,
			Args:       []ir.Operand{reason},
			ParamTypes: []types.Type{types.TypeAny},
		})
		g.currentBB.Terminator = &ir.JumpTerm{Target: doneBB}

		g.currentBB = doneBB
		task := g.makeImmediatePromiseTask(ir.ConstUndefined{}, types.TypeUndefined, types.TypeUndefined)
		return task, true

	case "pipeThrough":
		if len(e.Args) == 0 {
			return stream, true
		}
		transformArg := e.Args[0]
		transformObj := g.lowerExpr(transformArg)
		transformType := g.semanticType(transformArg)
		if transformType == nil || transformType == types.TypeAny {
			transformType = transformObj.Type()
		}
		if proven, ok := g.provenObjectType(transformArg); ok {
			transformType = proven
		}

		var writable, readable ir.Operand
		if tt, ok := transformType.(*types.ObjectType); ok && (tt.Name == "$TransformStream" || tt.Name == "$TextEncoderStream" || tt.Name == "$TextDecoderStream") {
			offsets, _, _ := g.objectLayout(tt)
			wVal := g.currentFn.NewValue("pipe_writable", g.semaResult.WritableStreamType)
			rVal := g.currentFn.NewValue("pipe_readable", g.semaResult.ReadableStreamType)
			g.currentBB.Instructions = append(g.currentBB.Instructions,
				&ir.GetFieldInst{Res: wVal, Obj: transformObj, Field: "writable", Offset: offsets["writable"]},
				&ir.GetFieldInst{Res: rVal, Obj: transformObj, Field: "readable", Offset: offsets["readable"]},
			)
			writable = wVal
			readable = rVal
		} else {
			boxed := g.boxJSValue(transformObj, transformType)
			writable = g.lowerDynamicGet(boxed, "writable")
			readable = g.lowerDynamicGet(boxed, "readable")
		}

		g.lowerPipeToOperation(stream, writable)
		return readable, true

	case "pipeTo":
		if len(e.Args) == 0 {
			task := g.makeImmediatePromiseTask(ir.ConstUndefined{}, types.TypeVoid, types.TypeVoid)
			return task, true
		}
		writableArg := e.Args[0]
		writable := g.lowerExpr(writableArg)
		g.lowerPipeToOperation(stream, writable)
		task := g.makeImmediatePromiseTask(ir.ConstUndefined{}, types.TypeVoid, types.TypeVoid)
		return task, true

	case "tee":
		return g.lowerStreamTee(stream), true
	}

	return nil, false
}

func (g *generator) lowerStreamTee(stream ir.Operand) ir.Operand {
	streamType := g.semaResult.ReadableStreamType
	offsets, _, _ := g.objectLayout(streamType)

	// Lock original stream
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: stream, Field: "locked", Offset: offsets["locked"], Val: ir.ConstBool{Value: true}},
	)

	hwm := g.currentFn.NewValue("tee_hwm", types.TypeNumber)
	queue := g.currentFn.NewValue("tee_orig_queue", types.NewArray(types.TypeAny))
	qIdx := g.currentFn.NewValue("tee_orig_qidx", types.TypeNumber)
	state := g.currentFn.NewValue("tee_orig_state", types.TypeString)

	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.GetFieldInst{Res: hwm, Obj: stream, Field: "$highWaterMark", Offset: offsets["$highWaterMark"]},
		&ir.GetFieldInst{Res: queue, Obj: stream, Field: "$queue", Offset: offsets["$queue"]},
		&ir.GetFieldInst{Res: qIdx, Obj: stream, Field: "$queueIndex", Offset: offsets["$queueIndex"]},
		&ir.GetFieldInst{Res: state, Obj: stream, Field: "$state", Offset: offsets["$state"]},
	)

	branch1, _ := g.newReadableStreamCore(hwm)
	branch2, _ := g.newReadableStreamCore(hwm)

	// Copy remaining elements into branch queues
	q1 := g.currentFn.NewValue("b1_queue", types.NewArray(types.TypeAny))
	q2 := g.currentFn.NewValue("b2_queue", types.NewArray(types.TypeAny))
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.GetFieldInst{Res: q1, Obj: branch1, Field: "$queue", Offset: offsets["$queue"]},
		&ir.GetFieldInst{Res: q2, Obj: branch2, Field: "$queue", Offset: offsets["$queue"]},
		&ir.SetFieldInst{Obj: branch1, Field: "$state", Offset: offsets["$state"], Val: state},
		&ir.SetFieldInst{Obj: branch2, Field: "$state", Offset: offsets["$state"], Val: state},
	)

	qLen := g.currentFn.NewValue("orig_qlen", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayLengthInst{Res: qLen, Array: queue})

	preBB := g.currentBB
	condBB := g.currentFn.NewBlock("tee_cond")
	bodyBB := g.currentFn.NewBlock("tee_body")
	doneBB := g.currentFn.NewBlock("tee_done")

	preBB.Terminator = &ir.JumpTerm{Target: condBB}

	g.currentBB = condBB
	curIdx := g.currentFn.NewValue("tee_i", types.TypeNumber)
	idxPhi := &ir.PhiInst{Res: curIdx, Incoming: []ir.PhiIncoming{{Block: preBB, Value: qIdx}}}
	condBB.Phis = append(condBB.Phis, idxPhi)

	hasMore := g.currentFn.NewValue("tee_has_more", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: hasMore, Op: ir.OpLt, LHS: curIdx, RHS: qLen})
	condBB.Terminator = &ir.BranchTerm{Cond: hasMore, Then: bodyBB, Else: doneBB}

	g.currentBB = bodyBB
	elem := g.currentFn.NewValue("tee_elem", types.TypeAny)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: elem, Array: queue, Index: curIdx})
	push1 := g.currentFn.NewValue("tee_push1", types.TypeNumber)
	push2 := g.currentFn.NewValue("tee_push2", types.TypeNumber)
	bodyBB.Instructions = append(bodyBB.Instructions,
		&ir.ArrayPushInst{Res: push1, Array: q1, Val: elem},
		&ir.ArrayPushInst{Res: push2, Array: q2, Val: elem},
	)
	nextIdx := g.currentFn.NewValue("tee_next_i", types.TypeNumber)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.BinaryInst{Res: nextIdx, Op: ir.OpAdd, LHS: curIdx, RHS: ir.ConstNumber{Value: 1}})
	bodyBB.Terminator = &ir.JumpTerm{Target: condBB}
	idxPhi.Incoming = append(idxPhi.Incoming, ir.PhiIncoming{Block: bodyBB, Value: nextIdx})

	g.currentBB = doneBB

	arr := g.currentFn.NewValue("tee_pair", types.NewArray(g.semaResult.ReadableStreamType))
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.AllocArrayInst{Res: arr, ElemType: g.semaResult.ReadableStreamType, Length: ir.ConstNumber{Value: 2}},
		&ir.SetElementInst{Array: arr, Index: ir.ConstNumber{Value: 0}, Val: branch1},
		&ir.SetElementInst{Array: arr, Index: ir.ConstNumber{Value: 1}, Val: branch2},
	)
	return arr
}

func (g *generator) lowerPipeToOperation(stream, destination ir.Operand) {
	streamType := g.semaResult.ReadableStreamType
	offsets, _, _ := g.objectLayout(streamType)

	queue := g.currentFn.NewValue("pipe_queue", types.NewArray(types.TypeAny))
	qIdx := g.currentFn.NewValue("pipe_qidx", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.GetFieldInst{Res: queue, Obj: stream, Field: "$queue", Offset: offsets["$queue"]},
		&ir.GetFieldInst{Res: qIdx, Obj: stream, Field: "$queueIndex", Offset: offsets["$queueIndex"]},
	)

	qLen := g.currentFn.NewValue("pipe_qlen", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayLengthInst{Res: qLen, Array: queue})

	preBB := g.currentBB
	condBB := g.currentFn.NewBlock("pipe_cond")
	bodyBB := g.currentFn.NewBlock("pipe_body")
	doneBB := g.currentFn.NewBlock("pipe_done")

	preBB.Terminator = &ir.JumpTerm{Target: condBB}

	g.currentBB = condBB
	curIdx := g.currentFn.NewValue("pipe_i", types.TypeNumber)
	idxPhi := &ir.PhiInst{Res: curIdx, Incoming: []ir.PhiIncoming{{Block: preBB, Value: qIdx}}}
	condBB.Phis = append(condBB.Phis, idxPhi)

	hasMore := g.currentFn.NewValue("pipe_has_more", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: hasMore, Op: ir.OpLt, LHS: curIdx, RHS: qLen})
	condBB.Terminator = &ir.BranchTerm{Cond: hasMore, Then: bodyBB, Else: doneBB}

	g.currentBB = bodyBB
	elem := g.currentFn.NewValue("pipe_elem", types.TypeAny)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: elem, Array: queue, Index: curIdx})

	// Invoke write on destination
	g.writeChunkToDestination(destination, elem)

	afterWriteBB := g.currentBB
	nextIdx := g.currentFn.NewValue("pipe_next_i", types.TypeNumber)
	afterWriteBB.Instructions = append(afterWriteBB.Instructions, &ir.BinaryInst{Res: nextIdx, Op: ir.OpAdd, LHS: curIdx, RHS: ir.ConstNumber{Value: 1}})
	afterWriteBB.Terminator = &ir.JumpTerm{Target: condBB}
	idxPhi.Incoming = append(idxPhi.Incoming, ir.PhiIncoming{Block: afterWriteBB, Value: nextIdx})

	g.currentBB = doneBB
	// Close destination writable stream
	g.closeDestination(destination)

	// Mark source stream as closed and exhausted
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: stream, Field: "$queueIndex", Offset: offsets["$queueIndex"], Val: qLen},
		&ir.SetFieldInst{Obj: stream, Field: "$state", Offset: offsets["$state"], Val: ir.ConstString{Value: "closed"}},
	)
}

func (g *generator) writeChunkToDestination(dest, chunk ir.Operand) {
	destType := g.semanticTypeFromOperand(dest)
	if ot, ok := destType.(*types.ObjectType); ok && ot.Name == "$WritableStream" {
		offsets, _, _ := g.objectLayout(ot)
		writeFn := g.currentFn.NewValue("dest_write_fn", types.TypeAny)
		ctrl := g.currentFn.NewValue("dest_ctrl", types.TypeAny)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.GetFieldInst{Res: writeFn, Obj: dest, Field: "$writeFn", Offset: offsets["$writeFn"]},
			&ir.GetFieldInst{Res: ctrl, Obj: dest, Field: "$controller", Offset: offsets["$controller"]},
		)
		hasWrite := g.currentFn.NewValue("dest_has_write", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
			Res: hasWrite, Op: ir.OpNe, LHS: writeFn, RHS: ir.ConstUndefined{},
		})
		callBB := g.currentFn.NewBlock("dest_write_call")
		nextBB := g.currentFn.NewBlock("dest_write_next")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: hasWrite, Then: callBB, Else: nextBB}

		g.currentBB = callBB
		fnType := types.NewFunction([]types.Param{{Name: "chunk", Type: types.TypeAny}, {Name: "controller", Type: types.TypeAny}}, types.TypeVoid)
		unboxedWriteFn := g.coerceJSValueBoundary(writeFn, types.TypeAny, fnType)
		wRes := g.currentFn.NewValue("dest_wres", types.TypeVoid)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.IndirectCallInst{
			Res:        wRes,
			Closure:    unboxedWriteFn,
			ThisArg:    nil,
			Args:       []ir.Operand{chunk, ctrl},
			ParamTypes: []types.Type{types.TypeAny, types.TypeAny},
		})
		g.currentBB.Terminator = &ir.JumpTerm{Target: nextBB}

		g.currentBB = nextBB
	}
}

func (g *generator) closeDestination(dest ir.Operand) {
	destType := g.semanticTypeFromOperand(dest)
	if ot, ok := destType.(*types.ObjectType); ok && ot.Name == "$WritableStream" {
		offsets, _, _ := g.objectLayout(ot)
		closeFn := g.currentFn.NewValue("dest_close_fn", types.TypeAny)
		ctrl := g.currentFn.NewValue("dest_close_ctrl", types.TypeAny)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.GetFieldInst{Res: closeFn, Obj: dest, Field: "$closeFn", Offset: offsets["$closeFn"]},
			&ir.GetFieldInst{Res: ctrl, Obj: dest, Field: "$controller", Offset: offsets["$controller"]},
			&ir.SetFieldInst{Obj: dest, Field: "$state", Offset: offsets["$state"], Val: ir.ConstString{Value: "closed"}},
		)
		hasClose := g.currentFn.NewValue("dest_has_close", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
			Res: hasClose, Op: ir.OpNe, LHS: closeFn, RHS: ir.ConstUndefined{},
		})
		callBB := g.currentFn.NewBlock("dest_close_call")
		nextBB := g.currentFn.NewBlock("dest_close_next")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: hasClose, Then: callBB, Else: nextBB}

		g.currentBB = callBB
		cFnType := types.NewFunction([]types.Param{{Name: "controller", Type: types.TypeAny}}, types.TypeVoid)
		unboxedCloseFn := g.coerceJSValueBoundary(closeFn, types.TypeAny, cFnType)
		cRes := g.currentFn.NewValue("dest_cres", types.TypeVoid)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.IndirectCallInst{
			Res:        cRes,
			Closure:    unboxedCloseFn,
			ThisArg:    nil,
			Args:       []ir.Operand{ctrl},
			ParamTypes: []types.Type{types.TypeAny},
		})
		g.currentBB.Terminator = &ir.JumpTerm{Target: nextBB}

		g.currentBB = nextBB
	}
}

func (g *generator) lowerReadableStreamStaticCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	if mem.Property == "from" && len(e.Args) > 0 {
		iterableArg := e.Args[0]
		iterableType := g.semanticType(iterableArg)
		iterable := g.lowerExpr(iterableArg)

		stream, ctrl := g.newReadableStreamCore(ir.ConstNumber{Value: 1})
		ctrlType := g.semaResult.ReadableStreamDefaultControllerType
		ctrlOffsets, _, _ := g.objectLayout(ctrlType)
		sOffsets, _, _ := g.objectLayout(g.semaResult.ReadableStreamType)

		if arrType, ok := iterableType.(*types.ArrayType); ok {
			arrLen := g.currentFn.NewValue("from_arr_len", types.TypeNumber)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayLengthInst{Res: arrLen, Array: iterable})

			preBB := g.currentBB
			condBB := g.currentFn.NewBlock("from_cond")
			bodyBB := g.currentFn.NewBlock("from_body")
			doneBB := g.currentFn.NewBlock("from_done")

			preBB.Terminator = &ir.JumpTerm{Target: condBB}

			g.currentBB = condBB
			curIdx := g.currentFn.NewValue("from_i", types.TypeNumber)
			idxPhi := &ir.PhiInst{Res: curIdx, Incoming: []ir.PhiIncoming{{Block: preBB, Value: ir.ConstNumber{Value: 0}}}}
			condBB.Phis = append(condBB.Phis, idxPhi)

			hasMore := g.currentFn.NewValue("from_has_more", types.TypeBoolean)
			condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: hasMore, Op: ir.OpLt, LHS: curIdx, RHS: arrLen})
			condBB.Terminator = &ir.BranchTerm{Cond: hasMore, Then: bodyBB, Else: doneBB}

			g.currentBB = bodyBB
			elem := g.currentFn.NewValue("from_elem", arrType.Elem)
			bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: elem, Array: iterable, Index: curIdx})
			boxedElem := g.boxJSValue(elem, arrType.Elem)

			queue := g.currentFn.NewValue("from_queue", types.NewArray(types.TypeAny))
			bodyBB.Instructions = append(bodyBB.Instructions,
				&ir.GetFieldInst{Res: queue, Obj: stream, Field: "$queue", Offset: sOffsets["$queue"]},
			)
			pushRes := g.currentFn.NewValue("from_push", types.TypeNumber)
			bodyBB.Instructions = append(bodyBB.Instructions, &ir.ArrayPushInst{Res: pushRes, Array: queue, Val: boxedElem})

			nextIdx := g.currentFn.NewValue("from_next_i", types.TypeNumber)
			bodyBB.Instructions = append(bodyBB.Instructions, &ir.BinaryInst{Res: nextIdx, Op: ir.OpAdd, LHS: curIdx, RHS: ir.ConstNumber{Value: 1}})
			bodyBB.Terminator = &ir.JumpTerm{Target: condBB}
			idxPhi.Incoming = append(idxPhi.Incoming, ir.PhiIncoming{Block: bodyBB, Value: nextIdx})

			g.currentBB = doneBB
			g.currentBB.Instructions = append(g.currentBB.Instructions,
				&ir.SetFieldInst{Obj: stream, Field: "$state", Offset: sOffsets["$state"], Val: ir.ConstString{Value: "closed"}},
				&ir.SetFieldInst{Obj: ctrl, Field: "desiredSize", Offset: ctrlOffsets["desiredSize"], Val: ir.ConstNumber{Value: 0}},
			)
		}
		return stream, true
	}
	return nil, false
}

// --- ReadableStreamDefaultReader ---

func (g *generator) lowerReadableStreamDefaultReaderMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || objType.Name != "$ReadableStreamDefaultReader" {
		return nil, false
	}
	reader := g.lowerExpr(mem.Object)
	rOffsets, _, _ := g.objectLayout(objType)
	streamType := g.semaResult.ReadableStreamType
	sOffsets, _, _ := g.objectLayout(streamType)

	switch mem.Property {
	case "releaseLock":
		stream := g.currentFn.NewValue("reader_stream", streamType)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.GetFieldInst{Res: stream, Obj: reader, Field: "$stream", Offset: rOffsets["$stream"]},
			&ir.SetFieldInst{Obj: stream, Field: "locked", Offset: sOffsets["locked"], Val: ir.ConstBool{Value: false}},
			&ir.SetFieldInst{Obj: stream, Field: "$reader", Offset: sOffsets["$reader"], Val: ir.ConstUndefined{}},
			&ir.SetFieldInst{Obj: reader, Field: "$stream", Offset: rOffsets["$stream"], Val: ir.ConstUndefined{}},
		)
		return ir.ConstUndefined{}, true

	case "cancel":
		stream := g.currentFn.NewValue("reader_stream", streamType)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.GetFieldInst{Res: stream, Obj: reader, Field: "$stream", Offset: rOffsets["$stream"]},
			&ir.SetFieldInst{Obj: stream, Field: "$state", Offset: sOffsets["$state"], Val: ir.ConstString{Value: "closed"}},
		)
		task := g.makeImmediatePromiseTask(ir.ConstUndefined{}, types.TypeUndefined, types.TypeUndefined)
		return task, true

	case "read":
		stream := g.currentFn.NewValue("r_stream", streamType)
		queue := g.currentFn.NewValue("r_queue", types.NewArray(types.TypeAny))
		qIdx := g.currentFn.NewValue("r_qidx", types.TypeNumber)
		qLen := g.currentFn.NewValue("r_qlen", types.TypeNumber)
		pullFn := g.currentFn.NewValue("r_pull_fn", types.TypeAny)
		ctrl := g.currentFn.NewValue("r_ctrl", types.TypeAny)

		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.GetFieldInst{Res: stream, Obj: reader, Field: "$stream", Offset: rOffsets["$stream"]},
			&ir.GetFieldInst{Res: queue, Obj: stream, Field: "$queue", Offset: sOffsets["$queue"]},
			&ir.GetFieldInst{Res: qIdx, Obj: stream, Field: "$queueIndex", Offset: sOffsets["$queueIndex"]},
			&ir.GetFieldInst{Res: pullFn, Obj: stream, Field: "$pullFn", Offset: sOffsets["$pullFn"]},
			&ir.GetFieldInst{Res: ctrl, Obj: stream, Field: "$controller", Offset: sOffsets["$controller"]},
			&ir.ArrayLengthInst{Res: qLen, Array: queue},
		)

		hasItem := g.currentFn.NewValue("r_has_item", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
			Res: hasItem, Op: ir.OpLt, LHS: qIdx, RHS: qLen,
		})

		entryBB := g.currentBB
		yieldBB := g.currentFn.NewBlock("r_yield")
		emptyBB := g.currentFn.NewBlock("r_empty")
		pullBB := g.currentFn.NewBlock("r_pull")
		postPullBB := g.currentFn.NewBlock("r_post_pull")
		joinBB := g.currentFn.NewBlock("r_join")

		entryBB.Terminator = &ir.BranchTerm{Cond: hasItem, Then: yieldBB, Else: emptyBB}

		// Yield current item from queue
		g.currentBB = yieldBB
		yieldVal := g.currentFn.NewValue("yield_val", types.TypeAny)
		yieldBB.Instructions = append(yieldBB.Instructions,
			&ir.GetElementInst{Res: yieldVal, Array: queue, Index: qIdx},
		)
		nextIdx := g.currentFn.NewValue("yield_next_idx", types.TypeNumber)
		yieldBB.Instructions = append(yieldBB.Instructions,
			&ir.BinaryInst{Res: nextIdx, Op: ir.OpAdd, LHS: qIdx, RHS: ir.ConstNumber{Value: 1}},
			&ir.SetFieldInst{Obj: stream, Field: "$queueIndex", Offset: sOffsets["$queueIndex"], Val: nextIdx},
		)
		yieldBB.Terminator = &ir.JumpTerm{Target: joinBB}

		// Empty queue: check pull
		g.currentBB = emptyBB
		hasPull := g.currentFn.NewValue("r_has_pull", types.TypeBoolean)
		emptyBB.Instructions = append(emptyBB.Instructions, &ir.BinaryInst{
			Res: hasPull, Op: ir.OpNe, LHS: pullFn, RHS: ir.ConstUndefined{},
		})
		emptyBB.Terminator = &ir.BranchTerm{Cond: hasPull, Then: pullBB, Else: postPullBB}

		g.currentBB = pullBB
		pFnType := types.NewFunction([]types.Param{{Name: "controller", Type: types.TypeAny}}, types.TypeVoid)
		unboxedPullFn := g.coerceJSValueBoundary(pullFn, types.TypeAny, pFnType)
		pRes := g.currentFn.NewValue("pull_call_res", types.TypeVoid)
		pullBB.Instructions = append(pullBB.Instructions, &ir.IndirectCallInst{
			Res:        pRes,
			Closure:    unboxedPullFn,
			ThisArg:    nil,
			Args:       []ir.Operand{ctrl},
			ParamTypes: []types.Type{types.TypeAny},
		})
		pullBB.Terminator = &ir.JumpTerm{Target: postPullBB}

		g.currentBB = postPullBB
		newQLen := g.currentFn.NewValue("post_pull_qlen", types.TypeNumber)
		postPullBB.Instructions = append(postPullBB.Instructions, &ir.ArrayLengthInst{Res: newQLen, Array: queue})
		postHasItem := g.currentFn.NewValue("post_has_item", types.TypeBoolean)
		postPullBB.Instructions = append(postPullBB.Instructions, &ir.BinaryInst{
			Res: postHasItem, Op: ir.OpLt, LHS: qIdx, RHS: newQLen,
		})
		postYieldBB := g.currentFn.NewBlock("post_yield")
		doneEndBB := g.currentFn.NewBlock("done_end")
		postPullBB.Terminator = &ir.BranchTerm{Cond: postHasItem, Then: postYieldBB, Else: doneEndBB}

		g.currentBB = postYieldBB
		postVal := g.currentFn.NewValue("post_val", types.TypeAny)
		postYieldBB.Instructions = append(postYieldBB.Instructions,
			&ir.GetElementInst{Res: postVal, Array: queue, Index: qIdx},
		)
		postNextIdx := g.currentFn.NewValue("post_next_idx", types.TypeNumber)
		postYieldBB.Instructions = append(postYieldBB.Instructions,
			&ir.BinaryInst{Res: postNextIdx, Op: ir.OpAdd, LHS: qIdx, RHS: ir.ConstNumber{Value: 1}},
			&ir.SetFieldInst{Obj: stream, Field: "$queueIndex", Offset: sOffsets["$queueIndex"], Val: postNextIdx},
		)
		postYieldBB.Terminator = &ir.JumpTerm{Target: joinBB}

		g.currentBB = doneEndBB
		doneEndBB.Terminator = &ir.JumpTerm{Target: joinBB}

		g.currentBB = joinBB
		resVal := g.currentFn.NewValue("final_read_val", types.TypeAny)
		resDone := g.currentFn.NewValue("final_read_done", types.TypeBoolean)

		joinBB.Phis = append(joinBB.Phis,
			&ir.PhiInst{Res: resVal, Incoming: []ir.PhiIncoming{
				{Block: yieldBB, Value: yieldVal},
				{Block: postYieldBB, Value: postVal},
				{Block: doneEndBB, Value: ir.ConstUndefined{}},
			}},
			&ir.PhiInst{Res: resDone, Incoming: []ir.PhiIncoming{
				{Block: yieldBB, Value: ir.ConstBool{Value: false}},
				{Block: postYieldBB, Value: ir.ConstBool{Value: false}},
				{Block: doneEndBB, Value: ir.ConstBool{Value: true}},
			}},
		)

		resType := g.semaResult.ReadableStreamReadResultType
		resOffsets, resRefMask, resShape := g.objectLayout(resType)
		resObj := g.currentFn.NewValue("read_result_obj", resType)
		joinBB.Instructions = append(joinBB.Instructions,
			&ir.AllocObjectInst{Res: resObj, Shape: resShape, FieldCount: len(resOffsets), RefMask: resRefMask},
			&ir.SetFieldInst{Obj: resObj, Field: "value", Offset: resOffsets["value"], Val: resVal},
			&ir.SetFieldInst{Obj: resObj, Field: "done", Offset: resOffsets["done"], Val: resDone},
		)

		task := g.makeImmediatePromiseTask(resObj, resType, resType)
		return task, true
	}

	return nil, false
}

// --- ReadableStreamDefaultController ---

func (g *generator) lowerReadableStreamDefaultControllerMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || objType.Name != "$ReadableStreamDefaultController" {
		if proven, ok := g.provenObjectType(mem.Object); ok && proven.Name == "$ReadableStreamDefaultController" {
			objType = proven
		} else {
			return nil, false
		}
	}
	ctrl := g.lowerExpr(mem.Object)
	if irJSValueType(ctrl.Type()) {
		ctrl = g.coerceJSValueBoundary(ctrl, ctrl.Type(), objType)
	}
	cOffsets, _, _ := g.objectLayout(objType)
	streamType := g.semaResult.ReadableStreamType
	sOffsets, _, _ := g.objectLayout(streamType)

	switch mem.Property {
	case "enqueue":
		chunk := ir.Operand(ir.ConstUndefined{})
		if len(e.Args) > 0 {
			chunk = g.lowerExpr(e.Args[0])
			chunk = g.boxJSValue(chunk, g.semanticType(e.Args[0]))
		}
		stream := g.currentFn.NewValue("ctrl_stream", streamType)
		queue := g.currentFn.NewValue("ctrl_queue", types.NewArray(types.TypeAny))
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.GetFieldInst{Res: stream, Obj: ctrl, Field: "$stream", Offset: cOffsets["$stream"]},
			&ir.GetFieldInst{Res: queue, Obj: stream, Field: "$queue", Offset: sOffsets["$queue"]},
		)
		pushRes := g.currentFn.NewValue("ctrl_push", types.TypeNumber)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.ArrayPushInst{Res: pushRes, Array: queue, Val: chunk},
		)
		return ir.ConstUndefined{}, true

	case "close":
		stream := g.currentFn.NewValue("ctrl_stream", streamType)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.GetFieldInst{Res: stream, Obj: ctrl, Field: "$stream", Offset: cOffsets["$stream"]},
			&ir.SetFieldInst{Obj: stream, Field: "$state", Offset: sOffsets["$state"], Val: ir.ConstString{Value: "closed"}},
			&ir.SetFieldInst{Obj: ctrl, Field: "desiredSize", Offset: cOffsets["desiredSize"], Val: ir.ConstNumber{Value: 0}},
		)
		return ir.ConstUndefined{}, true

	case "error":
		errVal := ir.Operand(ir.ConstUndefined{})
		if len(e.Args) > 0 {
			errVal = g.lowerExpr(e.Args[0])
			errVal = g.boxJSValue(errVal, g.semanticType(e.Args[0]))
		}
		stream := g.currentFn.NewValue("ctrl_stream", streamType)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.GetFieldInst{Res: stream, Obj: ctrl, Field: "$stream", Offset: cOffsets["$stream"]},
			&ir.SetFieldInst{Obj: stream, Field: "$state", Offset: sOffsets["$state"], Val: ir.ConstString{Value: "errored"}},
			&ir.SetFieldInst{Obj: stream, Field: "$storedError", Offset: sOffsets["$storedError"], Val: errVal},
		)
		return ir.ConstUndefined{}, true
	}

	return nil, false
}

// --- WritableStream Core ---

func (g *generator) newWritableStreamCore(hwm ir.Operand) (ir.Operand, ir.Operand) {
	t := g.semaResult.WritableStreamType
	offsets, refMask, shape := g.objectLayout(t)
	stream := g.currentFn.NewValue("wstream", t)

	ctrlType := g.semaResult.WritableStreamDefaultControllerType
	ctrlOffsets, ctrlRefMask, ctrlShape := g.objectLayout(ctrlType)
	ctrl := g.currentFn.NewValue("wctrl", ctrlType)

	signal := g.currentFn.NewValue("abort_signal", g.semaResult.AbortSignalType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: signal, Callee: "ts_abort_signal_new"})

	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.AllocObjectInst{Res: stream, Shape: shape, FieldCount: len(offsets), RefMask: refMask},
		&ir.AllocObjectInst{Res: ctrl, Shape: ctrlShape, FieldCount: len(ctrlOffsets), RefMask: ctrlRefMask},
	)

	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: ctrl, Field: "$stream", Offset: ctrlOffsets["$stream"], Val: stream},
		&ir.SetFieldInst{Obj: ctrl, Field: "signal", Offset: ctrlOffsets["signal"], Val: signal},
		&ir.SetFieldInst{Obj: stream, Field: "locked", Offset: offsets["locked"], Val: ir.ConstBool{Value: false}},
		&ir.SetFieldInst{Obj: stream, Field: "$state", Offset: offsets["$state"], Val: ir.ConstString{Value: "writable"}},
		&ir.SetFieldInst{Obj: stream, Field: "$writer", Offset: offsets["$writer"], Val: ir.ConstUndefined{}},
		&ir.SetFieldInst{Obj: stream, Field: "$controller", Offset: offsets["$controller"], Val: ctrl},
		&ir.SetFieldInst{Obj: stream, Field: "$sink", Offset: offsets["$sink"], Val: ir.ConstUndefined{}},
		&ir.SetFieldInst{Obj: stream, Field: "$writeFn", Offset: offsets["$writeFn"], Val: ir.ConstUndefined{}},
		&ir.SetFieldInst{Obj: stream, Field: "$closeFn", Offset: offsets["$closeFn"], Val: ir.ConstUndefined{}},
		&ir.SetFieldInst{Obj: stream, Field: "$abortFn", Offset: offsets["$abortFn"], Val: ir.ConstUndefined{}},
		&ir.SetFieldInst{Obj: stream, Field: "$highWaterMark", Offset: offsets["$highWaterMark"], Val: hwm},
		&ir.SetFieldInst{Obj: stream, Field: "$storedError", Offset: offsets["$storedError"], Val: ir.ConstUndefined{}},
	)

	return stream, ctrl
}

func (g *generator) lowerWritableStreamNew(e *ast.NewExpr) ir.Operand {
	hwm := ir.Operand(ir.ConstNumber{Value: 1})
	if len(e.Args) > 1 {
		if lit, ok := e.Args[1].(*ast.ObjectLit); ok {
			for _, prop := range lit.Properties {
				if prop.Key == "highWaterMark" {
					val := g.lowerExpr(prop.Value)
					hwm = g.coerceNumberOperand(val, g.semanticType(prop.Value))
					break
				}
			}
		}
	}

	stream, ctrl := g.newWritableStreamCore(hwm)
	streamType := g.semaResult.WritableStreamType
	offsets, _, _ := g.objectLayout(streamType)

	if len(e.Args) > 0 {
		sinkExpr := e.Args[0]
		ctrlType := g.semaResult.WritableStreamDefaultControllerType
		boxedCtrl := g.boxJSValue(ctrl, ctrlType)

		if lit, ok := sinkExpr.(*ast.ObjectLit); ok {
			for _, prop := range lit.Properties {
				switch prop.Key {
				case "start":
					prevKind := g.activeStreamControllerKind
					g.activeStreamControllerKind = "writable"
					startVal := g.lowerExpr(prop.Value)
					g.activeStreamControllerKind = prevKind
					callRes := g.currentFn.NewValue("wstart_res", types.TypeVoid)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.IndirectCallInst{
						Res:        callRes,
						Closure:    startVal,
						ThisArg:    nil,
						Args:       []ir.Operand{boxedCtrl},
						ParamTypes: []types.Type{types.TypeAny},
					})
				case "write":
					prevKind := g.activeStreamControllerKind
					g.activeStreamControllerKind = "writable"
					writeVal := g.lowerExpr(prop.Value)
					g.activeStreamControllerKind = prevKind
					boxedWrite := g.boxJSValue(writeVal, g.semanticType(prop.Value))
					g.currentBB.Instructions = append(g.currentBB.Instructions,
						&ir.SetFieldInst{Obj: stream, Field: "$writeFn", Offset: offsets["$writeFn"], Val: boxedWrite},
					)
				case "close":
					prevKind := g.activeStreamControllerKind
					g.activeStreamControllerKind = "writable"
					closeVal := g.lowerExpr(prop.Value)
					g.activeStreamControllerKind = prevKind
					boxedClose := g.boxJSValue(closeVal, g.semanticType(prop.Value))
					g.currentBB.Instructions = append(g.currentBB.Instructions,
						&ir.SetFieldInst{Obj: stream, Field: "$closeFn", Offset: offsets["$closeFn"], Val: boxedClose},
					)
				case "abort":
					prevKind := g.activeStreamControllerKind
					g.activeStreamControllerKind = "writable"
					abortVal := g.lowerExpr(prop.Value)
					g.activeStreamControllerKind = prevKind
					boxedAbort := g.boxJSValue(abortVal, g.semanticType(prop.Value))
					g.currentBB.Instructions = append(g.currentBB.Instructions,
						&ir.SetFieldInst{Obj: stream, Field: "$abortFn", Offset: offsets["$abortFn"], Val: boxedAbort},
					)
				}
			}
		}
	}

	return stream
}

func (g *generator) lowerWritableStreamDefaultWriterNew(e *ast.NewExpr) ir.Operand {
	if len(e.Args) == 0 {
		return ir.ConstUndefined{}
	}
	stream := g.lowerExpr(e.Args[0])
	return g.lockWriterForStream(stream)
}

func (g *generator) lockWriterForStream(stream ir.Operand) ir.Operand {
	streamType := g.semaResult.WritableStreamType
	sOffsets, _, _ := g.objectLayout(streamType)

	// Lock stream
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: stream, Field: "locked", Offset: sOffsets["locked"], Val: ir.ConstBool{Value: true}},
	)

	wType := g.semaResult.WritableStreamDefaultWriterType
	wOffsets, wRefMask, wShape := g.objectLayout(wType)
	writer := g.currentFn.NewValue("writer", wType)
	closedPromise := g.makeImmediatePromiseTask(ir.ConstUndefined{}, types.TypeUndefined, types.TypeUndefined)
	readyPromise := g.makeImmediatePromiseTask(ir.ConstUndefined{}, types.TypeUndefined, types.TypeUndefined)
	hwm := g.currentFn.NewValue("writer_hwm", types.TypeNumber)

	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.GetFieldInst{Res: hwm, Obj: stream, Field: "$highWaterMark", Offset: sOffsets["$highWaterMark"]},
		&ir.AllocObjectInst{Res: writer, Shape: wShape, FieldCount: len(wOffsets), RefMask: wRefMask},
	)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: writer, Field: "$stream", Offset: wOffsets["$stream"], Val: stream},
		&ir.SetFieldInst{Obj: writer, Field: "closed", Offset: wOffsets["closed"], Val: closedPromise},
		&ir.SetFieldInst{Obj: writer, Field: "ready", Offset: wOffsets["ready"], Val: readyPromise},
		&ir.SetFieldInst{Obj: writer, Field: "desiredSize", Offset: wOffsets["desiredSize"], Val: hwm},
		&ir.SetFieldInst{Obj: stream, Field: "$writer", Offset: sOffsets["$writer"], Val: writer},
	)

	return writer
}

func (g *generator) lowerWritableStreamMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || objType.Name != "$WritableStream" {
		return nil, false
	}
	stream := g.lowerExpr(mem.Object)

	switch mem.Property {
	case "getWriter":
		writer := g.lockWriterForStream(stream)
		return writer, true

	case "close":
		g.closeDestination(stream)
		task := g.makeImmediatePromiseTask(ir.ConstUndefined{}, types.TypeVoid, types.TypeVoid)
		return task, true

	case "abort":
		reason := ir.Operand(ir.ConstUndefined{})
		if len(e.Args) > 0 {
			reason = g.lowerExpr(e.Args[0])
			reason = g.boxJSValue(reason, g.semanticType(e.Args[0]))
		}
		streamType := g.semaResult.WritableStreamType
		offsets, _, _ := g.objectLayout(streamType)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.SetFieldInst{Obj: stream, Field: "$state", Offset: offsets["$state"], Val: ir.ConstString{Value: "errored"}},
			&ir.SetFieldInst{Obj: stream, Field: "$storedError", Offset: offsets["$storedError"], Val: reason},
		)
		task := g.makeImmediatePromiseTask(reason, types.TypeAny, types.TypeAny)
		return task, true
	}

	return nil, false
}

func (g *generator) lowerWritableStreamDefaultWriterMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || objType.Name != "$WritableStreamDefaultWriter" {
		return nil, false
	}
	writer := g.lowerExpr(mem.Object)
	wOffsets, _, _ := g.objectLayout(objType)
	streamType := g.semaResult.WritableStreamType
	sOffsets, _, _ := g.objectLayout(streamType)

	switch mem.Property {
	case "write":
		chunk := ir.Operand(ir.ConstUndefined{})
		if len(e.Args) > 0 {
			chunk = g.lowerExpr(e.Args[0])
			chunk = g.boxJSValue(chunk, g.semanticType(e.Args[0]))
		}
		stream := g.currentFn.NewValue("w_stream", streamType)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.GetFieldInst{Res: stream, Obj: writer, Field: "$stream", Offset: wOffsets["$stream"]},
		)
		g.writeChunkToDestination(stream, chunk)
		task := g.makeImmediatePromiseTask(ir.ConstUndefined{}, types.TypeVoid, types.TypeVoid)
		return task, true

	case "close":
		stream := g.currentFn.NewValue("w_stream", streamType)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.GetFieldInst{Res: stream, Obj: writer, Field: "$stream", Offset: wOffsets["$stream"]},
		)
		g.closeDestination(stream)
		task := g.makeImmediatePromiseTask(ir.ConstUndefined{}, types.TypeVoid, types.TypeVoid)
		return task, true

	case "abort":
		reason := ir.Operand(ir.ConstUndefined{})
		if len(e.Args) > 0 {
			reason = g.lowerExpr(e.Args[0])
			reason = g.boxJSValue(reason, g.semanticType(e.Args[0]))
		}
		stream := g.currentFn.NewValue("w_stream", streamType)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.GetFieldInst{Res: stream, Obj: writer, Field: "$stream", Offset: wOffsets["$stream"]},
			&ir.SetFieldInst{Obj: stream, Field: "$state", Offset: sOffsets["$state"], Val: ir.ConstString{Value: "errored"}},
			&ir.SetFieldInst{Obj: stream, Field: "$storedError", Offset: sOffsets["$storedError"], Val: reason},
		)
		task := g.makeImmediatePromiseTask(reason, types.TypeAny, types.TypeAny)
		return task, true

	case "releaseLock":
		stream := g.currentFn.NewValue("w_stream", streamType)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.GetFieldInst{Res: stream, Obj: writer, Field: "$stream", Offset: wOffsets["$stream"]},
			&ir.SetFieldInst{Obj: stream, Field: "locked", Offset: sOffsets["locked"], Val: ir.ConstBool{Value: false}},
			&ir.SetFieldInst{Obj: stream, Field: "$writer", Offset: sOffsets["$writer"], Val: ir.ConstUndefined{}},
			&ir.SetFieldInst{Obj: writer, Field: "$stream", Offset: wOffsets["$stream"], Val: ir.ConstUndefined{}},
		)
		return ir.ConstUndefined{}, true
	}

	return nil, false
}

func (g *generator) lowerWritableStreamDefaultControllerMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || objType.Name != "$WritableStreamDefaultController" {
		if proven, ok := g.provenObjectType(mem.Object); ok && proven.Name == "$WritableStreamDefaultController" {
			objType = proven
		} else {
			return nil, false
		}
	}
	ctrl := g.lowerExpr(mem.Object)
	if irJSValueType(ctrl.Type()) {
		ctrl = g.coerceJSValueBoundary(ctrl, ctrl.Type(), objType)
	}
	cOffsets, _, _ := g.objectLayout(objType)
	streamType := g.semaResult.WritableStreamType
	sOffsets, _, _ := g.objectLayout(streamType)

	switch mem.Property {
	case "error":
		errVal := ir.Operand(ir.ConstUndefined{})
		if len(e.Args) > 0 {
			errVal = g.lowerExpr(e.Args[0])
			errVal = g.boxJSValue(errVal, g.semanticType(e.Args[0]))
		}
		stream := g.currentFn.NewValue("wctrl_stream", streamType)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.GetFieldInst{Res: stream, Obj: ctrl, Field: "$stream", Offset: cOffsets["$stream"]},
			&ir.SetFieldInst{Obj: stream, Field: "$state", Offset: sOffsets["$state"], Val: ir.ConstString{Value: "errored"}},
			&ir.SetFieldInst{Obj: stream, Field: "$storedError", Offset: sOffsets["$storedError"], Val: errVal},
		)
		return ir.ConstUndefined{}, true
	}

	return nil, false
}

// --- TransformStream Core ---

func (g *generator) lowerTransformStreamNew(e *ast.NewExpr) ir.Operand {
	readable, readableCtrl := g.newReadableStreamCore(ir.ConstNumber{Value: 1})
	writable, _ := g.newWritableStreamCore(ir.ConstNumber{Value: 1})

	tsType := g.semaResult.TransformStreamType
	offsets, refMask, shape := g.objectLayout(tsType)
	ts := g.currentFn.NewValue("transform_stream", tsType)

	ctrlType := g.semaResult.TransformStreamDefaultControllerType
	ctrlOffsets, ctrlRefMask, ctrlShape := g.objectLayout(ctrlType)
	ctrl := g.currentFn.NewValue("ts_ctrl", ctrlType)

	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.AllocObjectInst{Res: ctrl, Shape: ctrlShape, FieldCount: len(ctrlOffsets), RefMask: ctrlRefMask},
		&ir.AllocObjectInst{Res: ts, Shape: shape, FieldCount: len(offsets), RefMask: refMask},
	)

	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: ctrl, Field: "$transformStream", Offset: ctrlOffsets["$transformStream"], Val: ts},
		&ir.SetFieldInst{Obj: ctrl, Field: "desiredSize", Offset: ctrlOffsets["desiredSize"], Val: ir.ConstNumber{Value: 1}},
		&ir.SetFieldInst{Obj: ts, Field: "readable", Offset: offsets["readable"], Val: readable},
		&ir.SetFieldInst{Obj: ts, Field: "writable", Offset: offsets["writable"], Val: writable},
		&ir.SetFieldInst{Obj: ts, Field: "$controller", Offset: offsets["$controller"], Val: ctrl},
		&ir.SetFieldInst{Obj: ts, Field: "$transformer", Offset: offsets["$transformer"], Val: ir.ConstUndefined{}},
		&ir.SetFieldInst{Obj: ts, Field: "$transformFn", Offset: offsets["$transformFn"], Val: ir.ConstUndefined{}},
		&ir.SetFieldInst{Obj: ts, Field: "$flushFn", Offset: offsets["$flushFn"], Val: ir.ConstUndefined{}},
	)

	wOffsets, _, _ := g.objectLayout(g.semaResult.WritableStreamType)
	rOffsets, _, _ := g.objectLayout(g.semaResult.ReadableStreamType)

	// Setup transformer callbacks
	if len(e.Args) > 0 {
		trExpr := e.Args[0]
		boxedCtrl := g.boxJSValue(ctrl, ctrlType)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.SetFieldInst{Obj: writable, Field: "$controller", Offset: wOffsets["$controller"], Val: boxedCtrl},
		)
		if lit, ok := trExpr.(*ast.ObjectLit); ok {
			for _, prop := range lit.Properties {
				switch prop.Key {
				case "start":
					prevKind := g.activeStreamControllerKind
					g.activeStreamControllerKind = "transform"
					startVal := g.lowerExpr(prop.Value)
					g.activeStreamControllerKind = prevKind
					sRes := g.currentFn.NewValue("ts_start_res", types.TypeVoid)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.IndirectCallInst{
						Res:        sRes,
						Closure:    startVal,
						ThisArg:    nil,
						Args:       []ir.Operand{boxedCtrl},
						ParamTypes: []types.Type{types.TypeAny},
					})
				case "transform":
					prevKind := g.activeStreamControllerKind
					g.activeStreamControllerKind = "transform"
					trVal := g.lowerExpr(prop.Value)
					g.activeStreamControllerKind = prevKind
					boxedTr := g.boxJSValue(trVal, g.semanticType(prop.Value))
					g.currentBB.Instructions = append(g.currentBB.Instructions,
						&ir.SetFieldInst{Obj: ts, Field: "$transformFn", Offset: offsets["$transformFn"], Val: boxedTr},
						&ir.SetFieldInst{Obj: writable, Field: "$writeFn", Offset: wOffsets["$writeFn"], Val: boxedTr},
					)
				case "flush":
					prevKind := g.activeStreamControllerKind
					g.activeStreamControllerKind = "transform"
					flushVal := g.lowerExpr(prop.Value)
					g.activeStreamControllerKind = prevKind
					boxedFlush := g.boxJSValue(flushVal, g.semanticType(prop.Value))
					g.currentBB.Instructions = append(g.currentBB.Instructions,
						&ir.SetFieldInst{Obj: ts, Field: "$flushFn", Offset: offsets["$flushFn"], Val: boxedFlush},
						&ir.SetFieldInst{Obj: writable, Field: "$closeFn", Offset: wOffsets["$closeFn"], Val: boxedFlush},
					)
				}
			}
		}
	}

	_ = readableCtrl
	_ = rOffsets
	return ts
}

func (g *generator) lowerTransformStreamMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	return nil, false
}

func (g *generator) lowerTransformStreamDefaultControllerMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || objType.Name != "$TransformStreamDefaultController" {
		if proven, ok := g.provenObjectType(mem.Object); ok && proven.Name == "$TransformStreamDefaultController" {
			objType = proven
		} else {
			return nil, false
		}
	}
	ctrl := g.lowerExpr(mem.Object)
	if irJSValueType(ctrl.Type()) {
		ctrl = g.coerceJSValueBoundary(ctrl, ctrl.Type(), objType)
	}
	cOffsets, _, _ := g.objectLayout(objType)
	tsType := g.semaResult.TransformStreamType
	tsOffsets, _, _ := g.objectLayout(tsType)
	rsType := g.semaResult.ReadableStreamType
	rsOffsets, _, _ := g.objectLayout(rsType)

	ts := g.currentFn.NewValue("ts_ctrl_stream", tsType)
	readable := g.currentFn.NewValue("ts_readable", rsType)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.GetFieldInst{Res: ts, Obj: ctrl, Field: "$transformStream", Offset: cOffsets["$transformStream"]},
		&ir.GetFieldInst{Res: readable, Obj: ts, Field: "readable", Offset: tsOffsets["readable"]},
	)

	switch mem.Property {
	case "enqueue":
		chunk := ir.Operand(ir.ConstUndefined{})
		if len(e.Args) > 0 {
			chunk = g.lowerExpr(e.Args[0])
			chunk = g.boxJSValue(chunk, g.semanticType(e.Args[0]))
		}
		queue := g.currentFn.NewValue("ts_queue", types.NewArray(types.TypeAny))
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.GetFieldInst{Res: queue, Obj: readable, Field: "$queue", Offset: rsOffsets["$queue"]},
		)
		pushRes := g.currentFn.NewValue("ts_push", types.TypeNumber)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.ArrayPushInst{Res: pushRes, Array: queue, Val: chunk},
		)
		return ir.ConstUndefined{}, true

	case "terminate":
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.SetFieldInst{Obj: readable, Field: "$state", Offset: rsOffsets["$state"], Val: ir.ConstString{Value: "closed"}},
		)
		return ir.ConstUndefined{}, true

	case "error":
		errVal := ir.Operand(ir.ConstUndefined{})
		if len(e.Args) > 0 {
			errVal = g.lowerExpr(e.Args[0])
			errVal = g.boxJSValue(errVal, g.semanticType(e.Args[0]))
		}
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.SetFieldInst{Obj: readable, Field: "$state", Offset: rsOffsets["$state"], Val: ir.ConstString{Value: "errored"}},
			&ir.SetFieldInst{Obj: readable, Field: "$storedError", Offset: rsOffsets["$storedError"], Val: errVal},
		)
		return ir.ConstUndefined{}, true
	}

	return nil, false
}

// --- TextEncoderStream and TextDecoderStream ---

func (g *generator) lowerTextEncoderStreamNew(e *ast.NewExpr) ir.Operand {
	// TextEncoderStream is a TransformStream converting strings to Uint8Arrays
	ts := g.lowerTransformStreamNew(&ast.NewExpr{
		SourceSpan: e.SourceSpan,
		ClassName:  "TransformStream",
	})
	t := g.semaResult.TextEncoderStreamType
	offsets, refMask, shape := g.objectLayout(t)
	res := g.currentFn.NewValue("text_encoder_stream", t)

	tsOffsets, _, _ := g.objectLayout(g.semaResult.TransformStreamType)
	readable := g.currentFn.NewValue("tes_readable", g.semaResult.ReadableStreamType)
	writable := g.currentFn.NewValue("tes_writable", g.semaResult.WritableStreamType)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.GetFieldInst{Res: readable, Obj: ts, Field: "readable", Offset: tsOffsets["readable"]},
		&ir.GetFieldInst{Res: writable, Obj: ts, Field: "writable", Offset: tsOffsets["writable"]},
		&ir.AllocObjectInst{Res: res, Shape: shape, FieldCount: len(offsets), RefMask: refMask},
	)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: res, Field: "readable", Offset: offsets["readable"], Val: readable},
		&ir.SetFieldInst{Obj: res, Field: "writable", Offset: offsets["writable"], Val: writable},
		&ir.SetFieldInst{Obj: res, Field: "encoding", Offset: offsets["encoding"], Val: ir.ConstString{Value: "utf-8"}},
		&ir.SetFieldInst{Obj: res, Field: "$transform", Offset: offsets["$transform"], Val: ts},
	)
	return res
}

func (g *generator) lowerTextDecoderStreamNew(e *ast.NewExpr) ir.Operand {
	ts := g.lowerTransformStreamNew(&ast.NewExpr{
		SourceSpan: e.SourceSpan,
		ClassName:  "TransformStream",
	})
	t := g.semaResult.TextDecoderStreamType
	offsets, refMask, shape := g.objectLayout(t)
	res := g.currentFn.NewValue("text_decoder_stream", t)

	tsOffsets, _, _ := g.objectLayout(g.semaResult.TransformStreamType)
	readable := g.currentFn.NewValue("tds_readable", g.semaResult.ReadableStreamType)
	writable := g.currentFn.NewValue("tds_writable", g.semaResult.WritableStreamType)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.GetFieldInst{Res: readable, Obj: ts, Field: "readable", Offset: tsOffsets["readable"]},
		&ir.GetFieldInst{Res: writable, Obj: ts, Field: "writable", Offset: tsOffsets["writable"]},
		&ir.AllocObjectInst{Res: res, Shape: shape, FieldCount: len(offsets), RefMask: refMask},
	)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: res, Field: "readable", Offset: offsets["readable"], Val: readable},
		&ir.SetFieldInst{Obj: res, Field: "writable", Offset: offsets["writable"], Val: writable},
		&ir.SetFieldInst{Obj: res, Field: "encoding", Offset: offsets["encoding"], Val: ir.ConstString{Value: "utf-8"}},
		&ir.SetFieldInst{Obj: res, Field: "fatal", Offset: offsets["fatal"], Val: ir.ConstBool{Value: false}},
		&ir.SetFieldInst{Obj: res, Field: "ignoreBOM", Offset: offsets["ignoreBOM"], Val: ir.ConstBool{Value: false}},
		&ir.SetFieldInst{Obj: res, Field: "$transform", Offset: offsets["$transform"], Val: ts},
	)
	return res
}

func (g *generator) semanticTypeFromOperand(op ir.Operand) types.Type {
	if op == nil {
		return types.TypeAny
	}
	return op.Type()
}

func (g *generator) coerceNumberOperand(val ir.Operand, src types.Type) ir.Operand {
	if val.Type() == types.TypeNumber {
		return val
	}
	return g.coerceJSValueBoundary(val, src, types.TypeNumber)
}
