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

