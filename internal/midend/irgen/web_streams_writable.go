package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

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

