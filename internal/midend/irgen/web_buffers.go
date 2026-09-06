package irgen

import (
	"strings"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) lowerArrayBufferMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || objType.Name != "$ArrayBuffer" || mem.Property != "slice" {
		return nil, false
	}
	obj := g.lowerExpr(mem.Object)
	data := g.arrayBufferData(obj)
	begin := g.lowerExpr(e.Args[0])
	end := ir.Operand(nil)
	if len(e.Args) > 1 {
		end = g.lowerExpr(e.Args[1])
	} else {
		length := g.currentFn.NewValue("array_buffer_len", types.TypeNumber)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: length, Callee: "ts_byte_buffer_len", Args: []ir.Operand{data}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}})
		end = length
	}
	sliced := g.currentFn.NewValue("array_buffer_slice_data", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: sliced, Callee: "ts_byte_buffer_slice", Args: []ir.Operand{data, begin, end}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber}})
	return g.newArrayBufferFromData(sliced), true
}

func (g *generator) lowerUint8ArrayMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || objType.Name != "$Uint8Array" || (mem.Property != "slice" && mem.Property != "subarray") {
		return nil, false
	}
	obj := g.lowerExpr(mem.Object)
	data := g.uint8ArrayField(obj, "$data", g.semaResult.ByteBufferType)
	buffer := g.uint8ArrayField(obj, "buffer", g.semaResult.ArrayBufferType)
	base := g.uint8ArrayField(obj, "byteOffset", types.TypeNumber)
	length := g.uint8ArrayField(obj, "length", types.TypeNumber)
	begin := g.lowerExpr(e.Args[0])
	end := ir.Operand(length)
	if len(e.Args) > 1 {
		end = g.lowerExpr(e.Args[1])
	}
	startAbs := g.currentFn.NewValue("uint8_slice_start", types.TypeNumber)
	endAbs := g.currentFn.NewValue("uint8_slice_end", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: startAbs, Op: ir.OpAdd, LHS: base, RHS: begin},
		&ir.BinaryInst{Res: endAbs, Op: ir.OpAdd, LHS: base, RHS: end},
	)
	newLen := g.currentFn.NewValue("uint8_slice_len", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: newLen, Op: ir.OpSub, LHS: end, RHS: begin})
	if mem.Property == "subarray" {
		return g.newUint8ArrayView(data, buffer, startAbs, newLen), true
	}
	sliced := g.currentFn.NewValue("uint8_slice_data", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: sliced, Callee: "ts_byte_buffer_slice", Args: []ir.Operand{data, startAbs, endAbs}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber}})
	newBuffer := g.newArrayBufferFromData(sliced)
	return g.newUint8ArrayView(sliced, newBuffer, ir.ConstNumber{Value: 0}, newLen), true
}

func (g *generator) newArrayBufferFromData(data ir.Operand) ir.Operand {
	t := g.semaResult.ArrayBufferType
	obj := g.currentFn.NewValue("array_buffer", t)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: obj, Callee: "ts_array_buffer_wrap", Args: []ir.Operand{data}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}})
	return obj
}

func (g *generator) arrayBufferData(obj ir.Operand) ir.Operand {
	t := g.semaResult.ArrayBufferType
	offsets, _, _ := g.objectLayout(t)
	data := g.currentFn.NewValue("array_buffer_data", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: data, Obj: obj, Field: "$data", Offset: offsets["$data"]})
	return data
}

func (g *generator) newUint8ArrayView(data, buffer, offset, length ir.Operand) ir.Operand {
	t := g.semaResult.Uint8ArrayType
	obj := g.currentFn.NewValue("uint8_array", t)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: obj, Callee: "ts_uint8_array_wrap", Args: []ir.Operand{data, buffer, offset, length}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, g.semaResult.ArrayBufferType, types.TypeNumber, types.TypeNumber}})
	return obj
}

func (g *generator) uint8ArrayField(obj ir.Operand, name string, typ types.Type) ir.Operand {
	offsets, _, _ := g.objectLayout(g.semaResult.Uint8ArrayType)
	res := g.currentFn.NewValue("uint8_"+strings.TrimPrefix(name, "$"), typ)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: res, Obj: obj, Field: name, Offset: offsets[name]})
	return res
}
