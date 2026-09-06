package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) lowerTextEncoderMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || objType.Name != "$TextEncoder" {
		return nil, false
	}
	switch mem.Property {
	case "encode":
		input := ir.Operand(ir.ConstString{Value: ""})
		if len(e.Args) > 0 {
			input = g.lowerExpr(e.Args[0])
		}
		data := g.currentFn.NewValue("encoded_bytes", g.semaResult.ByteBufferType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: data, Callee: "ts_byte_buffer_from_utf8_string", Args: []ir.Operand{input}, ParamTypes: []types.Type{types.TypeString}})
		length := g.currentFn.NewValue("encoded_len", types.TypeNumber)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: length, Callee: "ts_byte_buffer_len", Args: []ir.Operand{data}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}})
		buffer := g.newArrayBufferFromData(data)
		return g.newUint8ArrayView(data, buffer, ir.ConstNumber{Value: 0}, length), true
	case "encodeInto":
		source := g.lowerExpr(e.Args[0])
		destination := g.lowerExpr(e.Args[1])
		data := g.uint8ArrayField(destination, "$data", g.semaResult.ByteBufferType)
		offset := g.uint8ArrayField(destination, "byteOffset", types.TypeNumber)
		capacity := g.uint8ArrayField(destination, "byteLength", types.TypeNumber)
		resultType := g.semanticType(e)
		res := g.currentFn.NewValue("encode_into_result", resultType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_text_encode_into", Args: []ir.Operand{source, data, offset, capacity}, ParamTypes: []types.Type{types.TypeString, g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber}})
		return res, true
	}
	return nil, false
}

func (g *generator) lowerTextDecoderMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || objType.Name != "$TextDecoder" || mem.Property != "decode" {
		return nil, false
	}
	if len(e.Args) == 0 {
		return ir.ConstString{Value: ""}, true
	}
	decoder := g.lowerExpr(mem.Object)
	offsets, _, _ := g.objectLayout(g.semaResult.TextDecoderType)

	view := g.lowerExpr(e.Args[0])
	data := g.uint8ArrayField(view, "$data", g.semaResult.ByteBufferType)
	base := g.uint8ArrayField(view, "byteOffset", types.TypeNumber)
	length := g.uint8ArrayField(view, "byteLength", types.TypeNumber)
	end := g.currentFn.NewValue("decode_end", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: end, Op: ir.OpAdd, LHS: base, RHS: length})
	slice := g.currentFn.NewValue("decode_bytes", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: slice, Callee: "ts_byte_buffer_slice", Args: []ir.Operand{data, base, end}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber}})
	sliceLen := g.currentFn.NewValue("decode_slice_len", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: sliceLen, Callee: "ts_byte_buffer_len", Args: []ir.Operand{slice}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}})

	valid := g.currentFn.NewValue("decode_utf8_valid", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: valid, Callee: "ts_utf8_validate", Args: []ir.Operand{slice, ir.ConstNumber{Value: 0}, sliceLen}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber}})
	fatal := g.currentFn.NewValue("decoder_fatal", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: fatal, Obj: decoder, Field: "fatal", Offset: offsets["fatal"]})
	decodeBB := g.currentFn.NewBlock("decode_utf8")
	invalidBB := g.currentFn.NewBlock("decode_invalid_utf8")
	fatalBB := g.currentFn.NewBlock("decode_fatal_utf8")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: valid, Then: decodeBB, Else: invalidBB}

	g.currentBB = invalidBB
	g.currentBB.Terminator = &ir.BranchTerm{Cond: fatal, Then: fatalBB, Else: decodeBB}

	g.currentBB = fatalBB
	err := g.newWebError(ir.ConstString{Value: "The encoded data was not valid UTF-8."}, ir.ConstString{Value: "TypeError"})
	g.routeThrownValue(err)

	g.currentBB = decodeBB
	ignoreBOM := g.currentFn.NewValue("decoder_ignore_bom", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: ignoreBOM, Obj: decoder, Field: "ignoreBOM", Offset: offsets["ignoreBOM"]})
	res := g.currentFn.NewValue("decoded_text", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_utf8_sanitize", Args: []ir.Operand{slice, ir.ConstNumber{Value: 0}, sliceLen, ignoreBOM}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber, types.TypeBoolean}})
	return res, true
}
