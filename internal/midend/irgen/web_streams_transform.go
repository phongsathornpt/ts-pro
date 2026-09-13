package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

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
