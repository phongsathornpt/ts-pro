package irgen

import (
	"fmt"
	"strings"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) blobData(obj ir.Operand, objType *types.ObjectType) ir.Operand {
	offsets, _, _ := g.objectLayout(objType)
	res := g.currentFn.NewValue("blob_data", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{
		Res: res, Obj: obj, Field: "$data", Offset: offsets["$data"],
	})
	return res
}

func (g *generator) blobSize(obj ir.Operand, objType *types.ObjectType) ir.Operand {
	offsets, _, _ := g.objectLayout(objType)
	res := g.currentFn.NewValue("blob_size", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{
		Res: res, Obj: obj, Field: "size", Offset: offsets["size"],
	})
	return res
}

func (g *generator) blobType(obj ir.Operand, objType *types.ObjectType) ir.Operand {
	offsets, _, _ := g.objectLayout(objType)
	res := g.currentFn.NewValue("blob_type", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{
		Res: res, Obj: obj, Field: "type", Offset: offsets["type"],
	})
	return res
}

func (g *generator) newBlobObject(data, size, typeStr ir.Operand) ir.Operand {
	t := g.semaResult.BlobType
	offsets, refMask, shape := g.objectLayout(t)
	blob := g.currentFn.NewValue("blob", t)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
		Res: blob, Shape: shape, FieldCount: len(offsets), RefMask: refMask,
	})
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: blob, Field: "size", Offset: offsets["size"], Val: size},
		&ir.SetFieldInst{Obj: blob, Field: "type", Offset: offsets["type"], Val: typeStr},
		&ir.SetFieldInst{Obj: blob, Field: "$data", Offset: offsets["$data"], Val: data},
	)
	return blob
}

func (g *generator) newFileObject(data, size, typeStr, name, lastModified, webkitRelativePath ir.Operand) ir.Operand {
	t := g.semaResult.FileType
	offsets, refMask, shape := g.objectLayout(t)
	file := g.currentFn.NewValue("file", t)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
		Res: file, Shape: shape, FieldCount: len(offsets), RefMask: refMask,
	})
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: file, Field: "size", Offset: offsets["size"], Val: size},
		&ir.SetFieldInst{Obj: file, Field: "type", Offset: offsets["type"], Val: typeStr},
		&ir.SetFieldInst{Obj: file, Field: "name", Offset: offsets["name"], Val: name},
		&ir.SetFieldInst{Obj: file, Field: "lastModified", Offset: offsets["lastModified"], Val: lastModified},
		&ir.SetFieldInst{Obj: file, Field: "webkitRelativePath", Offset: offsets["webkitRelativePath"], Val: webkitRelativePath},
		&ir.SetFieldInst{Obj: file, Field: "$data", Offset: offsets["$data"], Val: data},
	)
	return file
}

func (g *generator) newFileFromBlob(blob, filename ir.Operand) ir.Operand {
	blobType := g.semaResult.BlobType
	if obj, ok := blob.Type().(*types.ObjectType); ok && obj.Name == "$File" {
		blobType = g.semaResult.FileType
	}
	data := g.blobData(blob, blobType)
	size := g.blobSize(blob, blobType)
	typeStr := g.blobType(blob, blobType)
	now := g.currentFn.NewValue("file_now", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: now, Callee: "ts_date_now"})
	return g.newFileObject(data, size, typeStr, filename, now, ir.ConstString{Value: ""})
}

func (g *generator) lowerNormalizeMimeType(typeOperand ir.Operand) ir.Operand {
	if c, ok := typeOperand.(ir.ConstString); ok {
		valid := true
		for i := 0; i < len(c.Value); i++ {
			if c.Value[i] < 0x20 || c.Value[i] > 0x7e {
				valid = false
				break
			}
		}
		if !valid {
			return ir.ConstString{Value: ""}
		}
		return ir.ConstString{Value: strings.ToLower(c.Value)}
	}

	lowered := g.currentFn.NewValue("mime_lower", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: lowered, Callee: "ts_string_ascii_lower", Args: []ir.Operand{typeOperand}, ParamTypes: []types.Type{types.TypeString},
	})
	length := g.urlStringLen(lowered)
	entry := g.currentBB
	condBB := g.currentFn.NewBlock("mime_check_cond")
	bodyBB := g.currentFn.NewBlock("mime_check_body")
	nextBB := g.currentFn.NewBlock("mime_check_next")
	validBB := g.currentFn.NewBlock("mime_check_valid")
	invalidBB := g.currentFn.NewBlock("mime_check_invalid")
	joinBB := g.currentFn.NewBlock("mime_check_join")

	entry.Terminator = &ir.JumpTerm{Target: condBB}
	i := g.currentFn.NewValue("mime_check_i", types.TypeNumber)
	nextI := g.currentFn.NewValue("mime_check_next_i", types.TypeNumber)
	condBB.Phis = append(condBB.Phis, &ir.PhiInst{Res: i, Incoming: []ir.PhiIncoming{
		{Block: entry, Value: ir.ConstNumber{Value: 0}},
		{Block: nextBB, Value: nextI},
	}})
	more := g.currentFn.NewValue("mime_check_more", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: i, RHS: length})
	condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: validBB}

	b := g.currentFn.NewValue("mime_byte", types.TypeNumber)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.CallInst{
		Res: b, Callee: "ts_string_byte_at", Args: []ir.Operand{lowered, i}, ParamTypes: []types.Type{types.TypeString, types.TypeNumber},
	})
	tooLow := g.currentFn.NewValue("mime_too_low", types.TypeBoolean)
	tooHigh := g.currentFn.NewValue("mime_too_high", types.TypeBoolean)
	bodyBB.Instructions = append(bodyBB.Instructions,
		&ir.BinaryInst{Res: tooLow, Op: ir.OpLt, LHS: b, RHS: ir.ConstNumber{Value: 32}},
		&ir.BinaryInst{Res: tooHigh, Op: ir.OpGt, LHS: b, RHS: ir.ConstNumber{Value: 126}},
	)
	invalidChar := g.currentFn.NewValue("mime_invalid_char", types.TypeBoolean)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.BinaryInst{Res: invalidChar, Op: ir.OpOr, LHS: tooLow, RHS: tooHigh})
	bodyBB.Terminator = &ir.BranchTerm{Cond: invalidChar, Then: invalidBB, Else: nextBB}

	nextBB.Instructions = append(nextBB.Instructions, &ir.BinaryInst{Res: nextI, Op: ir.OpAdd, LHS: i, RHS: ir.ConstNumber{Value: 1}})
	nextBB.Terminator = &ir.JumpTerm{Target: condBB}

	validBB.Terminator = &ir.JumpTerm{Target: joinBB}
	invalidBB.Terminator = &ir.JumpTerm{Target: joinBB}

	g.currentBB = joinBB
	res := g.currentFn.NewValue("mime_result", types.TypeString)
	joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{Res: res, Incoming: []ir.PhiIncoming{
		{Block: validBB, Value: lowered},
		{Block: invalidBB, Value: ir.ConstString{Value: ""}},
	}})
	return res
}

func (g *generator) extractBlobPart(partExpr ast.Expr) (ir.Operand, ir.Operand) {
	partType := g.semanticType(partExpr)
	if partType == types.TypeString {
		str := g.lowerExpr(partExpr)
		buf := g.currentFn.NewValue("part_buf", g.semaResult.ByteBufferType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: buf, Callee: "ts_byte_buffer_from_utf8_string", Args: []ir.Operand{str}, ParamTypes: []types.Type{types.TypeString},
		})
		lenVal := g.currentFn.NewValue("part_len", types.TypeNumber)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: lenVal, Callee: "ts_byte_buffer_len", Args: []ir.Operand{buf}, ParamTypes: []types.Type{g.semaResult.ByteBufferType},
		})
		return buf, lenVal
	}

	if obj, ok := partType.(*types.ObjectType); ok {
		if obj.Name == "$ArrayBuffer" {
			ab := g.lowerExpr(partExpr)
			buf := g.arrayBufferData(ab)
			lenVal := g.currentFn.NewValue("part_len", types.TypeNumber)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
				Res: lenVal, Callee: "ts_byte_buffer_len", Args: []ir.Operand{buf}, ParamTypes: []types.Type{g.semaResult.ByteBufferType},
			})
			return buf, lenVal
		}
		if obj.Name == "$Uint8Array" {
			u8 := g.lowerExpr(partExpr)
			rawBuf := g.uint8ArrayField(u8, "$data", g.semaResult.ByteBufferType)
			offset := g.uint8ArrayField(u8, "byteOffset", types.TypeNumber)
			lenVal := g.uint8ArrayField(u8, "length", types.TypeNumber)
			end := g.currentFn.NewValue("u8_end", types.TypeNumber)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: end, Op: ir.OpAdd, LHS: offset, RHS: lenVal})
			buf := g.currentFn.NewValue("u8_slice", g.semaResult.ByteBufferType)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
				Res: buf, Callee: "ts_byte_buffer_slice", Args: []ir.Operand{rawBuf, offset, end}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber},
			})
			return buf, lenVal
		}
		if obj.Name == "$Blob" || obj.Name == "$File" {
			b := g.lowerExpr(partExpr)
			buf := g.blobData(b, obj)
			lenVal := g.blobSize(b, obj)
			return buf, lenVal
		}
	}

	// Fallback: coerce to string then to UTF-8 byte buffer
	val := g.lowerExpr(partExpr)
	str := g.coerceStringType(partType, val)
	buf := g.currentFn.NewValue("part_buf", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: buf, Callee: "ts_byte_buffer_from_utf8_string", Args: []ir.Operand{str}, ParamTypes: []types.Type{types.TypeString},
	})
	lenVal := g.currentFn.NewValue("part_len", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: lenVal, Callee: "ts_byte_buffer_len", Args: []ir.Operand{buf}, ParamTypes: []types.Type{g.semaResult.ByteBufferType},
	})
	return buf, lenVal
}

func (g *generator) lowerBlobParts(partsExpr ast.Expr) (ir.Operand, ir.Operand) {
	if partsExpr == nil {
		buf := g.currentFn.NewValue("empty_blob_buf", g.semaResult.ByteBufferType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: buf, Callee: "ts_byte_buffer_new", Args: []ir.Operand{ir.ConstNumber{Value: 0}}, ParamTypes: []types.Type{types.TypeNumber},
		})
		return buf, ir.ConstNumber{Value: 0}
	}

	if lit, ok := partsExpr.(*ast.ArrayLit); ok {
		if len(lit.Elements) == 0 {
			buf := g.currentFn.NewValue("empty_blob_buf", g.semaResult.ByteBufferType)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
				Res: buf, Callee: "ts_byte_buffer_new", Args: []ir.Operand{ir.ConstNumber{Value: 0}}, ParamTypes: []types.Type{types.TypeNumber},
			})
			return buf, ir.ConstNumber{Value: 0}
		}
		if len(lit.Elements) == 1 {
			return g.extractBlobPart(lit.Elements[0])
		}

		var buffers []ir.Operand
		var lengths []ir.Operand
		for _, el := range lit.Elements {
			buf, lenVal := g.extractBlobPart(el)
			buffers = append(buffers, buf)
			lengths = append(lengths, lenVal)
		}

		// Sum lengths
		total := lengths[0]
		for i := 1; i < len(lengths); i++ {
			nextTotal := g.currentFn.NewValue(fmt.Sprintf("total_len_%d", i), types.TypeNumber)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
				Res: nextTotal, Op: ir.OpAdd, LHS: total, RHS: lengths[i],
			})
			total = nextTotal
		}

		combined := g.currentFn.NewValue("combined_blob_buf", g.semaResult.ByteBufferType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: combined, Callee: "ts_byte_buffer_new", Args: []ir.Operand{total}, ParamTypes: []types.Type{types.TypeNumber},
		})

		var offset ir.Operand = ir.ConstNumber{Value: 0}
		for i := 0; i < len(buffers); i++ {
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
				Callee: "ts_byte_buffer_copy",
				Args:   []ir.Operand{combined, offset, buffers[i], ir.ConstNumber{Value: 0}, lengths[i]},
				ParamTypes: []types.Type{
					g.semaResult.ByteBufferType, types.TypeNumber, g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber,
				},
			})
			if i < len(buffers)-1 {
				nextOffset := g.currentFn.NewValue(fmt.Sprintf("offset_%d", i+1), types.TypeNumber)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
					Res: nextOffset, Op: ir.OpAdd, LHS: offset, RHS: lengths[i],
				})
				offset = nextOffset
			}
		}
		return combined, total
	}

	// General array operand
	arr := g.lowerExpr(partsExpr)
	arrLen := g.currentFn.NewValue("arr_parts_len", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayLengthInst{Res: arrLen, Array: arr})

	// Pass 1: compute total length by summing string representation of each element
	entry := g.currentBB
	cond1BB := g.currentFn.NewBlock("blob_arr_len_cond")
	body1BB := g.currentFn.NewBlock("blob_arr_len_body")
	next1BB := g.currentFn.NewBlock("blob_arr_len_next")
	allocBB := g.currentFn.NewBlock("blob_arr_alloc")

	entry.Terminator = &ir.JumpTerm{Target: cond1BB}
	i1 := g.currentFn.NewValue("blob_arr_i1", types.TypeNumber)
	nextI1 := g.currentFn.NewValue("blob_arr_next_i1", types.TypeNumber)
	sum1 := g.currentFn.NewValue("blob_arr_sum1", types.TypeNumber)
	nextSum1 := g.currentFn.NewValue("blob_arr_next_sum1", types.TypeNumber)

	cond1BB.Phis = append(cond1BB.Phis,
		&ir.PhiInst{Res: i1, Incoming: []ir.PhiIncoming{{Block: entry, Value: ir.ConstNumber{Value: 0}}, {Block: next1BB, Value: nextI1}}},
		&ir.PhiInst{Res: sum1, Incoming: []ir.PhiIncoming{{Block: entry, Value: ir.ConstNumber{Value: 0}}, {Block: next1BB, Value: nextSum1}}},
	)
	more1 := g.currentFn.NewValue("blob_arr_more1", types.TypeBoolean)
	cond1BB.Instructions = append(cond1BB.Instructions, &ir.BinaryInst{Res: more1, Op: ir.OpLt, LHS: i1, RHS: arrLen})
	cond1BB.Terminator = &ir.BranchTerm{Cond: more1, Then: body1BB, Else: allocBB}

	el1 := g.currentFn.NewValue("blob_arr_el1", types.TypeAny)
	body1BB.Instructions = append(body1BB.Instructions, &ir.GetElementInst{Res: el1, Array: arr, Index: i1})
	elStr1 := g.currentFn.NewValue("blob_arr_el_str1", types.TypeString)
	body1BB.Instructions = append(body1BB.Instructions, &ir.CallInst{
		Res: elStr1, Callee: "ts_js_to_string", Args: []ir.Operand{el1}, ParamTypes: []types.Type{types.TypeAny},
	})
	elLen1 := g.urlStringLen(elStr1)
	body1BB.Terminator = &ir.JumpTerm{Target: next1BB}

	next1BB.Instructions = append(next1BB.Instructions,
		&ir.BinaryInst{Res: nextI1, Op: ir.OpAdd, LHS: i1, RHS: ir.ConstNumber{Value: 1}},
		&ir.BinaryInst{Res: nextSum1, Op: ir.OpAdd, LHS: sum1, RHS: elLen1},
	)
	next1BB.Terminator = &ir.JumpTerm{Target: cond1BB}

	g.currentBB = allocBB
	combined := g.currentFn.NewValue("combined_blob_buf", g.semaResult.ByteBufferType)
	allocBB.Instructions = append(allocBB.Instructions, &ir.CallInst{
		Res: combined, Callee: "ts_byte_buffer_new", Args: []ir.Operand{sum1}, ParamTypes: []types.Type{types.TypeNumber},
	})

	// Pass 2: copy each element's bytes
	cond2BB := g.currentFn.NewBlock("blob_arr_copy_cond")
	body2BB := g.currentFn.NewBlock("blob_arr_copy_body")
	next2BB := g.currentFn.NewBlock("blob_arr_copy_next")
	done2BB := g.currentFn.NewBlock("blob_arr_copy_done")

	allocBB.Terminator = &ir.JumpTerm{Target: cond2BB}
	i2 := g.currentFn.NewValue("blob_arr_i2", types.TypeNumber)
	nextI2 := g.currentFn.NewValue("blob_arr_next_i2", types.TypeNumber)
	offset2 := g.currentFn.NewValue("blob_arr_offset2", types.TypeNumber)
	nextOffset2 := g.currentFn.NewValue("blob_arr_next_offset2", types.TypeNumber)

	cond2BB.Phis = append(cond2BB.Phis,
		&ir.PhiInst{Res: i2, Incoming: []ir.PhiIncoming{{Block: allocBB, Value: ir.ConstNumber{Value: 0}}, {Block: next2BB, Value: nextI2}}},
		&ir.PhiInst{Res: offset2, Incoming: []ir.PhiIncoming{{Block: allocBB, Value: ir.ConstNumber{Value: 0}}, {Block: next2BB, Value: nextOffset2}}},
	)
	more2 := g.currentFn.NewValue("blob_arr_more2", types.TypeBoolean)
	cond2BB.Instructions = append(cond2BB.Instructions, &ir.BinaryInst{Res: more2, Op: ir.OpLt, LHS: i2, RHS: arrLen})
	cond2BB.Terminator = &ir.BranchTerm{Cond: more2, Then: body2BB, Else: done2BB}

	el2 := g.currentFn.NewValue("blob_arr_el2", types.TypeAny)
	body2BB.Instructions = append(body2BB.Instructions, &ir.GetElementInst{Res: el2, Array: arr, Index: i2})
	elStr2 := g.currentFn.NewValue("blob_arr_el_str2", types.TypeString)
	body2BB.Instructions = append(body2BB.Instructions, &ir.CallInst{
		Res: elStr2, Callee: "ts_js_to_string", Args: []ir.Operand{el2}, ParamTypes: []types.Type{types.TypeAny},
	})
	partBuf2 := g.currentFn.NewValue("blob_arr_part_buf2", g.semaResult.ByteBufferType)
	body2BB.Instructions = append(body2BB.Instructions, &ir.CallInst{
		Res: partBuf2, Callee: "ts_byte_buffer_from_utf8_string", Args: []ir.Operand{elStr2}, ParamTypes: []types.Type{types.TypeString},
	})
	partLen2 := g.currentFn.NewValue("blob_arr_part_len2", types.TypeNumber)
	body2BB.Instructions = append(body2BB.Instructions, &ir.CallInst{
		Res: partLen2, Callee: "ts_byte_buffer_len", Args: []ir.Operand{partBuf2}, ParamTypes: []types.Type{g.semaResult.ByteBufferType},
	})
	body2BB.Instructions = append(body2BB.Instructions, &ir.CallInst{
		Callee: "ts_byte_buffer_copy",
		Args:   []ir.Operand{combined, offset2, partBuf2, ir.ConstNumber{Value: 0}, partLen2},
		ParamTypes: []types.Type{
			g.semaResult.ByteBufferType, types.TypeNumber, g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber,
		},
	})
	body2BB.Terminator = &ir.JumpTerm{Target: next2BB}

	next2BB.Instructions = append(next2BB.Instructions,
		&ir.BinaryInst{Res: nextI2, Op: ir.OpAdd, LHS: i2, RHS: ir.ConstNumber{Value: 1}},
		&ir.BinaryInst{Res: nextOffset2, Op: ir.OpAdd, LHS: offset2, RHS: partLen2},
	)
	next2BB.Terminator = &ir.JumpTerm{Target: cond2BB}

	g.currentBB = done2BB
	return combined, sum1
}

func (g *generator) extractBlobType(optionsExpr ast.Expr) ir.Operand {
	if optionsExpr == nil {
		return ir.ConstString{Value: ""}
	}
	if lit, ok := optionsExpr.(*ast.ObjectLit); ok {
		for _, prop := range lit.Properties {
			if prop.Key == "type" {
				val := g.lowerExpr(prop.Value)
				return g.lowerNormalizeMimeType(val)
			}
		}
		return ir.ConstString{Value: ""}
	}
	optVal := g.lowerExpr(optionsExpr)
	typeVal := g.lowerDynamicGet(g.boxJSValue(optVal, g.semanticType(optionsExpr)), "type")
	strVal := g.coerceJSValueBoundary(typeVal, types.TypeAny, types.TypeString)
	return g.lowerNormalizeMimeType(strVal)
}

func (g *generator) extractFileOptions(optionsExpr ast.Expr) (ir.Operand, ir.Operand) {
	typeStr := g.extractBlobType(optionsExpr)
	if optionsExpr != nil {
		if lit, ok := optionsExpr.(*ast.ObjectLit); ok {
			for _, prop := range lit.Properties {
				if prop.Key == "lastModified" {
					return typeStr, g.lowerExpr(prop.Value)
				}
			}
		}
	}
	now := g.currentFn.NewValue("file_now", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: now, Callee: "ts_date_now"})
	return typeStr, now
}

func (g *generator) lowerBlobNew(e *ast.NewExpr) ir.Operand {
	var partsExpr ast.Expr
	var optionsExpr ast.Expr
	if len(e.Args) > 0 {
		partsExpr = e.Args[0]
	}
	if len(e.Args) > 1 {
		optionsExpr = e.Args[1]
	}

	typeStr := g.extractBlobType(optionsExpr)
	data, size := g.lowerBlobParts(partsExpr)
	return g.newBlobObject(data, size, typeStr)
}

func (g *generator) lowerFileNew(e *ast.NewExpr) ir.Operand {
	var partsExpr ast.Expr
	var nameExpr ast.Expr
	var optionsExpr ast.Expr
	if len(e.Args) > 0 {
		partsExpr = e.Args[0]
	}
	if len(e.Args) > 1 {
		nameExpr = e.Args[1]
	}
	if len(e.Args) > 2 {
		optionsExpr = e.Args[2]
	}

	fileName := g.lowerExpr(nameExpr)
	typeStr, lastModified := g.extractFileOptions(optionsExpr)
	data, size := g.lowerBlobParts(partsExpr)
	return g.newFileObject(data, size, typeStr, fileName, lastModified, ir.ConstString{Value: ""})
}

func (g *generator) emitMin(a, b ir.Operand) ir.Operand {
	cond := g.currentFn.NewValue("min_cond", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: cond, Op: ir.OpLt, LHS: a, RHS: b})
	entry := g.currentBB
	thenBB := g.currentFn.NewBlock("min_then")
	elseBB := g.currentFn.NewBlock("min_else")
	joinBB := g.currentFn.NewBlock("min_join")
	entry.Terminator = &ir.BranchTerm{Cond: cond, Then: thenBB, Else: elseBB}
	thenBB.Terminator = &ir.JumpTerm{Target: joinBB}
	elseBB.Terminator = &ir.JumpTerm{Target: joinBB}

	g.currentBB = joinBB
	res := g.currentFn.NewValue("min_res", types.TypeNumber)
	joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{Res: res, Incoming: []ir.PhiIncoming{
		{Block: thenBB, Value: a},
		{Block: elseBB, Value: b},
	}})
	return res
}

func (g *generator) emitMax(a, b ir.Operand) ir.Operand {
	cond := g.currentFn.NewValue("max_cond", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: cond, Op: ir.OpGt, LHS: a, RHS: b})
	entry := g.currentBB
	thenBB := g.currentFn.NewBlock("max_then")
	elseBB := g.currentFn.NewBlock("max_else")
	joinBB := g.currentFn.NewBlock("max_join")
	entry.Terminator = &ir.BranchTerm{Cond: cond, Then: thenBB, Else: elseBB}
	thenBB.Terminator = &ir.JumpTerm{Target: joinBB}
	elseBB.Terminator = &ir.JumpTerm{Target: joinBB}

	g.currentBB = joinBB
	res := g.currentFn.NewValue("max_res", types.TypeNumber)
	joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{Res: res, Incoming: []ir.PhiIncoming{
		{Block: thenBB, Value: a},
		{Block: elseBB, Value: b},
	}})
	return res
}

func (g *generator) lowerBlobMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || (objType.Name != "$Blob" && objType.Name != "$File") {
		return nil, false
	}

	switch mem.Property {
	case "slice":
		obj := g.lowerExpr(mem.Object)
		data := g.blobData(obj, objType)
		size := g.blobSize(obj, objType)

		var start ir.Operand = ir.ConstNumber{Value: 0}
		if len(e.Args) > 0 {
			start = g.lowerExpr(e.Args[0])
		}
		var end ir.Operand = size
		if len(e.Args) > 1 {
			end = g.lowerExpr(e.Args[1])
		}
		var contentType ir.Operand = ir.ConstString{Value: ""}
		if len(e.Args) > 2 {
			contentType = g.lowerNormalizeMimeType(g.lowerExpr(e.Args[2]))
		}

		// first = start < 0 ? max(size + start, 0) : min(start, size)
		startNeg := g.currentFn.NewValue("start_neg", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: startNeg, Op: ir.OpLt, LHS: start, RHS: ir.ConstNumber{Value: 0}})
		startEntry := g.currentBB
		startNegBB := g.currentFn.NewBlock("start_neg_bb")
		startPosBB := g.currentFn.NewBlock("start_pos_bb")
		startJoinBB := g.currentFn.NewBlock("start_join_bb")
		startEntry.Terminator = &ir.BranchTerm{Cond: startNeg, Then: startNegBB, Else: startPosBB}

		g.currentBB = startNegBB
		negSum := g.currentFn.NewValue("start_neg_sum", types.TypeNumber)
		startNegBB.Instructions = append(startNegBB.Instructions, &ir.BinaryInst{Res: negSum, Op: ir.OpAdd, LHS: size, RHS: start})
		negClamped := g.emitMax(negSum, ir.ConstNumber{Value: 0})
		negEndBB := g.currentBB
		negEndBB.Terminator = &ir.JumpTerm{Target: startJoinBB}

		g.currentBB = startPosBB
		posClamped := g.emitMin(start, size)
		posEndBB := g.currentBB
		posEndBB.Terminator = &ir.JumpTerm{Target: startJoinBB}

		g.currentBB = startJoinBB
		first := g.currentFn.NewValue("slice_first", types.TypeNumber)
		startJoinBB.Phis = append(startJoinBB.Phis, &ir.PhiInst{Res: first, Incoming: []ir.PhiIncoming{
			{Block: negEndBB, Value: negClamped},
			{Block: posEndBB, Value: posClamped},
		}})

		// last = end < 0 ? max(size + end, 0) : min(end, size)
		endNeg := g.currentFn.NewValue("end_neg", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: endNeg, Op: ir.OpLt, LHS: end, RHS: ir.ConstNumber{Value: 0}})
		endEntry := g.currentBB
		endNegBB := g.currentFn.NewBlock("end_neg_bb")
		endPosBB := g.currentFn.NewBlock("end_pos_bb")
		endJoinBB := g.currentFn.NewBlock("end_join_bb")
		endEntry.Terminator = &ir.BranchTerm{Cond: endNeg, Then: endNegBB, Else: endPosBB}

		g.currentBB = endNegBB
		endNegSum := g.currentFn.NewValue("end_neg_sum", types.TypeNumber)
		endNegBB.Instructions = append(endNegBB.Instructions, &ir.BinaryInst{Res: endNegSum, Op: ir.OpAdd, LHS: size, RHS: end})
		endNegClamped := g.emitMax(endNegSum, ir.ConstNumber{Value: 0})
		endNegEndBB := g.currentBB
		endNegEndBB.Terminator = &ir.JumpTerm{Target: endJoinBB}

		g.currentBB = endPosBB
		endPosClamped := g.emitMin(end, size)
		endPosEndBB := g.currentBB
		endPosEndBB.Terminator = &ir.JumpTerm{Target: endJoinBB}

		g.currentBB = endJoinBB
		last := g.currentFn.NewValue("slice_last", types.TypeNumber)
		endJoinBB.Phis = append(endJoinBB.Phis, &ir.PhiInst{Res: last, Incoming: []ir.PhiIncoming{
			{Block: endNegEndBB, Value: endNegClamped},
			{Block: endPosEndBB, Value: endPosClamped},
		}})

		diff := g.currentFn.NewValue("slice_diff", types.TypeNumber)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: diff, Op: ir.OpSub, LHS: last, RHS: first})
		span := g.emitMax(diff, ir.ConstNumber{Value: 0})

		endIdx := g.currentFn.NewValue("slice_end_idx", types.TypeNumber)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: endIdx, Op: ir.OpAdd, LHS: first, RHS: span})

		slicedData := g.currentFn.NewValue("sliced_blob_data", g.semaResult.ByteBufferType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: slicedData, Callee: "ts_byte_buffer_slice", Args: []ir.Operand{data, first, endIdx},
			ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber},
		})
		return g.newBlobObject(slicedData, span, contentType), true

	case "text":
		obj := g.lowerExpr(mem.Object)
		data := g.blobData(obj, objType)
		str := g.currentFn.NewValue("blob_text_str", types.TypeString)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: str, Callee: "ts_byte_buffer_to_utf8_string", Args: []ir.Operand{data}, ParamTypes: []types.Type{g.semaResult.ByteBufferType},
		})
		task := g.makeImmediatePromiseTask(str, types.TypeString, types.TypeString)
		return task, true

	case "arrayBuffer":
		obj := g.lowerExpr(mem.Object)
		data := g.blobData(obj, objType)
		size := g.blobSize(obj, objType)
		copyData := g.currentFn.NewValue("blob_ab_data", g.semaResult.ByteBufferType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: copyData, Callee: "ts_byte_buffer_slice", Args: []ir.Operand{data, ir.ConstNumber{Value: 0}, size},
			ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber},
		})
		ab := g.newArrayBufferFromData(copyData)
		task := g.makeImmediatePromiseTask(ab, g.semaResult.ArrayBufferType, g.semaResult.ArrayBufferType)
		return task, true

	case "bytes":
		obj := g.lowerExpr(mem.Object)
		data := g.blobData(obj, objType)
		size := g.blobSize(obj, objType)
		copyData := g.currentFn.NewValue("blob_bytes_data", g.semaResult.ByteBufferType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: copyData, Callee: "ts_byte_buffer_slice", Args: []ir.Operand{data, ir.ConstNumber{Value: 0}, size},
			ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber},
		})
		ab := g.newArrayBufferFromData(copyData)
		u8 := g.newUint8ArrayView(copyData, ab, ir.ConstNumber{Value: 0}, size)
		task := g.makeImmediatePromiseTask(u8, g.semaResult.Uint8ArrayType, g.semaResult.Uint8ArrayType)
		return task, true

	case "stream":
		obj := g.lowerExpr(mem.Object)
		data := g.blobData(obj, objType)
		size := g.blobSize(obj, objType)
		copyData := g.currentFn.NewValue("blob_stream_data", g.semaResult.ByteBufferType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: copyData, Callee: "ts_byte_buffer_slice", Args: []ir.Operand{data, ir.ConstNumber{Value: 0}, size},
			ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber},
		})
		ab := g.newArrayBufferFromData(copyData)
		u8 := g.newUint8ArrayView(copyData, ab, ir.ConstNumber{Value: 0}, size)

		stream, ctrl := g.newReadableStreamCore(ir.ConstNumber{Value: 1})
		ctrlType := g.semaResult.ReadableStreamDefaultControllerType
		ctrlOffsets, _, _ := g.objectLayout(ctrlType)
		sOffsets, _, _ := g.objectLayout(g.semaResult.ReadableStreamType)

		boxedU8 := g.boxJSValue(u8, g.semaResult.Uint8ArrayType)
		queue := g.currentFn.NewValue("blob_stream_queue", types.NewArray(types.TypeAny))
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.GetFieldInst{Res: queue, Obj: stream, Field: "$queue", Offset: sOffsets["$queue"]},
		)
		pushRes := g.currentFn.NewValue("blob_stream_push", types.TypeNumber)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.ArrayPushInst{Res: pushRes, Array: queue, Val: boxedU8},
			&ir.SetFieldInst{Obj: stream, Field: "$state", Offset: sOffsets["$state"], Val: ir.ConstString{Value: "closed"}},
			&ir.SetFieldInst{Obj: ctrl, Field: "desiredSize", Offset: ctrlOffsets["desiredSize"], Val: ir.ConstNumber{Value: 0}},
		)
		return stream, true
	}

	return nil, false
}

func (g *generator) lowerFormDataNew(e *ast.NewExpr) ir.Operand {
	t := g.semaResult.FormDataType
	offsets, refMask, shape := g.objectLayout(t)
	fd := g.currentFn.NewValue("form_data", t)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
		Res: fd, Shape: shape, FieldCount: len(offsets), RefMask: refMask,
	})
	entriesType := types.NewArray(types.TypeAny)
	entries := g.currentFn.NewValue("form_data_entries", entriesType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{
		Res: entries, ElemType: types.TypeAny, Length: ir.ConstNumber{Value: 0},
	})
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{
		Obj: fd, Field: "$entries", Offset: offsets["$entries"], Val: entries,
	})
	return fd
}

func (g *generator) formDataEntries(fd ir.Operand) ir.Operand {
	t := g.semaResult.FormDataType
	offsets, _, _ := g.objectLayout(t)
	entriesType := types.NewArray(types.TypeAny)
	entries := g.currentFn.NewValue("form_data_entries", entriesType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{
		Res: entries, Obj: fd, Field: "$entries", Offset: offsets["$entries"],
	})
	return entries
}

func (g *generator) lowerFormDataMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || objType.Name != "$FormData" {
		return nil, false
	}

	fd := g.lowerExpr(mem.Object)
	entries := g.formDataEntries(fd)

	switch mem.Property {
	case "append":
		nameVal := g.lowerExpr(e.Args[0])
		nameStr := g.coerceStringType(g.semanticType(e.Args[0]), nameVal)
		val := g.lowerExpr(e.Args[1])
		valType := g.semanticType(e.Args[1])

		var storedVal ir.Operand
		if obj, ok := valType.(*types.ObjectType); ok && (obj.Name == "$Blob" || obj.Name == "$File") {
			var filename ir.Operand = ir.ConstString{Value: "blob"}
			if obj.Name == "$File" {
				offsets, _, _ := g.objectLayout(obj)
				nameOperand := g.currentFn.NewValue("file_orig_name", types.TypeString)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{
					Res: nameOperand, Obj: val, Field: "name", Offset: offsets["name"],
				})
				filename = nameOperand
			}
			if len(e.Args) > 2 {
				filename = g.lowerExpr(e.Args[2])
			}
			fileObj := g.newFileFromBlob(val, filename)
			storedVal = g.boxJSValue(fileObj, g.semaResult.FileType)
		} else {
			strVal := g.coerceStringType(valType, val)
			storedVal = g.boxJSValue(strVal, types.TypeString)
		}

		boxedName := g.boxJSValue(nameStr, types.TypeString)
		g.pushArrayOperand(entries, boxedName)
		g.pushArrayOperand(entries, storedVal)
		return ir.ConstUndefined{}, true

	case "get":
		nameVal := g.lowerExpr(e.Args[0])
		nameStr := g.coerceStringType(g.semanticType(e.Args[0]), nameVal)

		entry := g.currentBB
		condBB := g.currentFn.NewBlock("fd_get_cond")
		bodyBB := g.currentFn.NewBlock("fd_get_body")
		foundBB := g.currentFn.NewBlock("fd_get_found")
		nextBB := g.currentFn.NewBlock("fd_get_next")
		missBB := g.currentFn.NewBlock("fd_get_miss")
		joinBB := g.currentFn.NewBlock("fd_get_join")

		entry.Terminator = &ir.JumpTerm{Target: condBB}
		index := g.currentFn.NewValue("fd_get_i", types.TypeNumber)
		nextIndex := g.currentFn.NewValue("fd_get_next_i", types.TypeNumber)
		condBB.Phis = append(condBB.Phis, &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{
			{Block: entry, Value: ir.ConstNumber{Value: 0}},
			{Block: nextBB, Value: nextIndex},
		}})

		totalLen := g.currentFn.NewValue("fd_entries_len", types.TypeNumber)
		condBB.Instructions = append(condBB.Instructions, &ir.ArrayLengthInst{Res: totalLen, Array: entries})
		more := g.currentFn.NewValue("fd_get_more", types.TypeBoolean)
		condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: totalLen})
		condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: missBB}

		boxedKey := g.currentFn.NewValue("fd_get_boxed_key", types.TypeAny)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: boxedKey, Array: entries, Index: index})
		keyStr := g.currentFn.NewValue("fd_get_key_str", types.TypeString)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.CallInst{
			Res: keyStr, Callee: "ts_js_unbox_string", Args: []ir.Operand{boxedKey}, ParamTypes: []types.Type{types.TypeAny},
		})
		eq := g.currentFn.NewValue("fd_get_eq", types.TypeBoolean)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.CallInst{
			Res: eq, Callee: "ts_string_eq", Args: []ir.Operand{keyStr, nameStr}, ParamTypes: []types.Type{types.TypeString, types.TypeString},
		})
		bodyBB.Terminator = &ir.BranchTerm{Cond: eq, Then: foundBB, Else: nextBB}

		valIdx := g.currentFn.NewValue("fd_get_val_idx", types.TypeNumber)
		foundBB.Instructions = append(foundBB.Instructions, &ir.BinaryInst{Res: valIdx, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
		foundVal := g.currentFn.NewValue("fd_get_val", types.TypeAny)
		foundBB.Instructions = append(foundBB.Instructions, &ir.GetElementInst{Res: foundVal, Array: entries, Index: valIdx})
		foundBB.Terminator = &ir.JumpTerm{Target: joinBB}

		nextBB.Instructions = append(nextBB.Instructions, &ir.BinaryInst{Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 2}})
		nextBB.Terminator = &ir.JumpTerm{Target: condBB}

		missBB.Terminator = &ir.JumpTerm{Target: joinBB}

		g.currentBB = joinBB
		res := g.currentFn.NewValue("fd_get_res", types.TypeAny)
		joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{Res: res, Incoming: []ir.PhiIncoming{
			{Block: foundBB, Value: foundVal},
			{Block: missBB, Value: ir.ConstNull{}},
		}})
		return res, true

	case "getAll":
		nameVal := g.lowerExpr(e.Args[0])
		nameStr := g.coerceStringType(g.semanticType(e.Args[0]), nameVal)

		outArr := g.currentFn.NewValue("fd_getall_arr", types.NewArray(types.TypeAny))
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{
			Res: outArr, ElemType: types.TypeAny, Length: ir.ConstNumber{Value: 0},
		})

		entry := g.currentBB
		condBB := g.currentFn.NewBlock("fd_getall_cond")
		bodyBB := g.currentFn.NewBlock("fd_getall_body")
		matchBB := g.currentFn.NewBlock("fd_getall_match")
		nextBB := g.currentFn.NewBlock("fd_getall_next")
		doneBB := g.currentFn.NewBlock("fd_getall_done")

		entry.Terminator = &ir.JumpTerm{Target: condBB}
		index := g.currentFn.NewValue("fd_getall_i", types.TypeNumber)
		nextIndex := g.currentFn.NewValue("fd_getall_next_i", types.TypeNumber)
		condBB.Phis = append(condBB.Phis, &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{
			{Block: entry, Value: ir.ConstNumber{Value: 0}},
			{Block: nextBB, Value: nextIndex},
		}})

		totalLen := g.currentFn.NewValue("fd_entries_len", types.TypeNumber)
		condBB.Instructions = append(condBB.Instructions, &ir.ArrayLengthInst{Res: totalLen, Array: entries})
		more := g.currentFn.NewValue("fd_getall_more", types.TypeBoolean)
		condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: totalLen})
		condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: doneBB}

		boxedKey := g.currentFn.NewValue("fd_getall_boxed_key", types.TypeAny)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: boxedKey, Array: entries, Index: index})
		keyStr := g.currentFn.NewValue("fd_getall_key_str", types.TypeString)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.CallInst{
			Res: keyStr, Callee: "ts_js_unbox_string", Args: []ir.Operand{boxedKey}, ParamTypes: []types.Type{types.TypeAny},
		})
		eq := g.currentFn.NewValue("fd_getall_eq", types.TypeBoolean)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.CallInst{
			Res: eq, Callee: "ts_string_eq", Args: []ir.Operand{keyStr, nameStr}, ParamTypes: []types.Type{types.TypeString, types.TypeString},
		})
		bodyBB.Terminator = &ir.BranchTerm{Cond: eq, Then: matchBB, Else: nextBB}

		valIdx := g.currentFn.NewValue("fd_getall_val_idx", types.TypeNumber)
		matchBB.Instructions = append(matchBB.Instructions, &ir.BinaryInst{Res: valIdx, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
		val := g.currentFn.NewValue("fd_getall_val", types.TypeAny)
		matchBB.Instructions = append(matchBB.Instructions, &ir.GetElementInst{Res: val, Array: entries, Index: valIdx})
		g.currentBB = matchBB
		g.pushArrayOperand(outArr, val)
		matchEndBB := g.currentBB
		matchEndBB.Terminator = &ir.JumpTerm{Target: nextBB}

		g.currentBB = nextBB
		nextBB.Instructions = append(nextBB.Instructions, &ir.BinaryInst{Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 2}})
		nextBB.Terminator = &ir.JumpTerm{Target: condBB}

		g.currentBB = doneBB
		return outArr, true

	case "has":
		nameVal := g.lowerExpr(e.Args[0])
		nameStr := g.coerceStringType(g.semanticType(e.Args[0]), nameVal)

		entry := g.currentBB
		condBB := g.currentFn.NewBlock("fd_has_cond")
		bodyBB := g.currentFn.NewBlock("fd_has_body")
		foundBB := g.currentFn.NewBlock("fd_has_found")
		nextBB := g.currentFn.NewBlock("fd_has_next")
		missBB := g.currentFn.NewBlock("fd_has_miss")
		joinBB := g.currentFn.NewBlock("fd_has_join")

		entry.Terminator = &ir.JumpTerm{Target: condBB}
		index := g.currentFn.NewValue("fd_has_i", types.TypeNumber)
		nextIndex := g.currentFn.NewValue("fd_has_next_i", types.TypeNumber)
		condBB.Phis = append(condBB.Phis, &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{
			{Block: entry, Value: ir.ConstNumber{Value: 0}},
			{Block: nextBB, Value: nextIndex},
		}})

		totalLen := g.currentFn.NewValue("fd_entries_len", types.TypeNumber)
		condBB.Instructions = append(condBB.Instructions, &ir.ArrayLengthInst{Res: totalLen, Array: entries})
		more := g.currentFn.NewValue("fd_has_more", types.TypeBoolean)
		condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: totalLen})
		condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: missBB}

		boxedKey := g.currentFn.NewValue("fd_has_boxed_key", types.TypeAny)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: boxedKey, Array: entries, Index: index})
		keyStr := g.currentFn.NewValue("fd_has_key_str", types.TypeString)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.CallInst{
			Res: keyStr, Callee: "ts_js_unbox_string", Args: []ir.Operand{boxedKey}, ParamTypes: []types.Type{types.TypeAny},
		})
		eq := g.currentFn.NewValue("fd_has_eq", types.TypeBoolean)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.CallInst{
			Res: eq, Callee: "ts_string_eq", Args: []ir.Operand{keyStr, nameStr}, ParamTypes: []types.Type{types.TypeString, types.TypeString},
		})
		bodyBB.Terminator = &ir.BranchTerm{Cond: eq, Then: foundBB, Else: nextBB}

		foundBB.Terminator = &ir.JumpTerm{Target: joinBB}

		nextBB.Instructions = append(nextBB.Instructions, &ir.BinaryInst{Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 2}})
		nextBB.Terminator = &ir.JumpTerm{Target: condBB}

		missBB.Terminator = &ir.JumpTerm{Target: joinBB}

		g.currentBB = joinBB
		res := g.currentFn.NewValue("fd_has_res", types.TypeBoolean)
		joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{Res: res, Incoming: []ir.PhiIncoming{
			{Block: foundBB, Value: ir.ConstBool{Value: true}},
			{Block: missBB, Value: ir.ConstBool{Value: false}},
		}})
		return res, true

	case "delete":
		nameVal := g.lowerExpr(e.Args[0])
		nameStr := g.coerceStringType(g.semanticType(e.Args[0]), nameVal)

		nextEntries := g.currentFn.NewValue("fd_del_entries", types.NewArray(types.TypeAny))
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{
			Res: nextEntries, ElemType: types.TypeAny, Length: ir.ConstNumber{Value: 0},
		})

		entry := g.currentBB
		condBB := g.currentFn.NewBlock("fd_del_cond")
		bodyBB := g.currentFn.NewBlock("fd_del_body")
		keepBB := g.currentFn.NewBlock("fd_del_keep")
		nextBB := g.currentFn.NewBlock("fd_del_next")
		doneBB := g.currentFn.NewBlock("fd_del_done")

		entry.Terminator = &ir.JumpTerm{Target: condBB}
		index := g.currentFn.NewValue("fd_del_i", types.TypeNumber)
		nextIndex := g.currentFn.NewValue("fd_del_next_i", types.TypeNumber)
		condBB.Phis = append(condBB.Phis, &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{
			{Block: entry, Value: ir.ConstNumber{Value: 0}},
			{Block: nextBB, Value: nextIndex},
		}})

		totalLen := g.currentFn.NewValue("fd_entries_len", types.TypeNumber)
		condBB.Instructions = append(condBB.Instructions, &ir.ArrayLengthInst{Res: totalLen, Array: entries})
		more := g.currentFn.NewValue("fd_del_more", types.TypeBoolean)
		condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: totalLen})
		condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: doneBB}

		boxedKey := g.currentFn.NewValue("fd_del_boxed_key", types.TypeAny)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: boxedKey, Array: entries, Index: index})
		keyStr := g.currentFn.NewValue("fd_del_key_str", types.TypeString)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.CallInst{
			Res: keyStr, Callee: "ts_js_unbox_string", Args: []ir.Operand{boxedKey}, ParamTypes: []types.Type{types.TypeAny},
		})
		eq := g.currentFn.NewValue("fd_del_eq", types.TypeBoolean)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.CallInst{
			Res: eq, Callee: "ts_string_eq", Args: []ir.Operand{keyStr, nameStr}, ParamTypes: []types.Type{types.TypeString, types.TypeString},
		})
		bodyBB.Terminator = &ir.BranchTerm{Cond: eq, Then: nextBB, Else: keepBB}

		valIdx := g.currentFn.NewValue("fd_del_val_idx", types.TypeNumber)
		keepBB.Instructions = append(keepBB.Instructions, &ir.BinaryInst{Res: valIdx, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
		val := g.currentFn.NewValue("fd_del_val", types.TypeAny)
		keepBB.Instructions = append(keepBB.Instructions, &ir.GetElementInst{Res: val, Array: entries, Index: valIdx})
		g.currentBB = keepBB
		g.pushArrayOperand(nextEntries, boxedKey)
		g.pushArrayOperand(nextEntries, val)
		keepEndBB := g.currentBB
		keepEndBB.Terminator = &ir.JumpTerm{Target: nextBB}

		g.currentBB = nextBB
		nextBB.Instructions = append(nextBB.Instructions, &ir.BinaryInst{Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 2}})
		nextBB.Terminator = &ir.JumpTerm{Target: condBB}

		g.currentBB = doneBB
		offsets, _, _ := g.objectLayout(g.semaResult.FormDataType)
		doneBB.Instructions = append(doneBB.Instructions, &ir.SetFieldInst{
			Obj: fd, Field: "$entries", Offset: offsets["$entries"], Val: nextEntries,
		})
		return ir.ConstUndefined{}, true

	case "set":
		nameVal := g.lowerExpr(e.Args[0])
		nameStr := g.coerceStringType(g.semanticType(e.Args[0]), nameVal)
		val := g.lowerExpr(e.Args[1])
		valType := g.semanticType(e.Args[1])

		var storedVal ir.Operand
		if obj, ok := valType.(*types.ObjectType); ok && (obj.Name == "$Blob" || obj.Name == "$File") {
			var filename ir.Operand = ir.ConstString{Value: "blob"}
			if obj.Name == "$File" {
				offsets, _, _ := g.objectLayout(obj)
				nameOperand := g.currentFn.NewValue("file_orig_name", types.TypeString)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{
					Res: nameOperand, Obj: val, Field: "name", Offset: offsets["name"],
				})
				filename = nameOperand
			}
			if len(e.Args) > 2 {
				filename = g.lowerExpr(e.Args[2])
			}
			fileObj := g.newFileFromBlob(val, filename)
			storedVal = g.boxJSValue(fileObj, g.semaResult.FileType)
		} else {
			strVal := g.coerceStringType(valType, val)
			storedVal = g.boxJSValue(strVal, types.TypeString)
		}
		boxedName := g.boxJSValue(nameStr, types.TypeString)

		nextEntries := g.currentFn.NewValue("fd_set_entries", types.NewArray(types.TypeAny))
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{
			Res: nextEntries, ElemType: types.TypeAny, Length: ir.ConstNumber{Value: 0},
		})

		entry := g.currentBB
		condBB := g.currentFn.NewBlock("fd_set_cond")
		bodyBB := g.currentFn.NewBlock("fd_set_body")
		matchBB := g.currentFn.NewBlock("fd_set_match")
		firstMatchBB := g.currentFn.NewBlock("fd_set_first_match")
		copyBB := g.currentFn.NewBlock("fd_set_copy")
		nextBB := g.currentFn.NewBlock("fd_set_next")
		doneBB := g.currentFn.NewBlock("fd_set_done")
		appendBB := g.currentFn.NewBlock("fd_set_append")
		storeBB := g.currentFn.NewBlock("fd_set_store")

		entry.Terminator = &ir.JumpTerm{Target: condBB}
		index := g.currentFn.NewValue("fd_set_i", types.TypeNumber)
		nextIndex := g.currentFn.NewValue("fd_set_next_i", types.TypeNumber)
		found := g.currentFn.NewValue("fd_set_found", types.TypeBoolean)
		nextFound := g.currentFn.NewValue("fd_set_next_found", types.TypeBoolean)

		condBB.Phis = append(condBB.Phis,
			&ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{{Block: entry, Value: ir.ConstNumber{Value: 0}}, {Block: nextBB, Value: nextIndex}}},
			&ir.PhiInst{Res: found, Incoming: []ir.PhiIncoming{{Block: entry, Value: ir.ConstBool{Value: false}}, {Block: nextBB, Value: nextFound}}},
		)

		totalLen := g.currentFn.NewValue("fd_entries_len", types.TypeNumber)
		condBB.Instructions = append(condBB.Instructions, &ir.ArrayLengthInst{Res: totalLen, Array: entries})
		more := g.currentFn.NewValue("fd_set_more", types.TypeBoolean)
		condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: totalLen})
		condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: doneBB}

		boxedKey := g.currentFn.NewValue("fd_set_boxed_key", types.TypeAny)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: boxedKey, Array: entries, Index: index})
		keyStr := g.currentFn.NewValue("fd_set_key_str", types.TypeString)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.CallInst{
			Res: keyStr, Callee: "ts_js_unbox_string", Args: []ir.Operand{boxedKey}, ParamTypes: []types.Type{types.TypeAny},
		})
		eq := g.currentFn.NewValue("fd_set_eq", types.TypeBoolean)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.CallInst{
			Res: eq, Callee: "ts_string_eq", Args: []ir.Operand{keyStr, nameStr}, ParamTypes: []types.Type{types.TypeString, types.TypeString},
		})
		bodyBB.Terminator = &ir.BranchTerm{Cond: eq, Then: matchBB, Else: copyBB}

		valIdx := g.currentFn.NewValue("fd_set_val_idx", types.TypeNumber)
		copyBB.Instructions = append(copyBB.Instructions, &ir.BinaryInst{Res: valIdx, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
		valElem := g.currentFn.NewValue("fd_set_val_elem", types.TypeAny)
		copyBB.Instructions = append(copyBB.Instructions, &ir.GetElementInst{Res: valElem, Array: entries, Index: valIdx})
		g.currentBB = copyBB
		g.pushArrayOperand(nextEntries, boxedKey)
		g.pushArrayOperand(nextEntries, valElem)
		copyEndBB := g.currentBB
		copyEndBB.Terminator = &ir.JumpTerm{Target: nextBB}

		matchBB.Terminator = &ir.BranchTerm{Cond: found, Then: nextBB, Else: firstMatchBB}

		g.currentBB = firstMatchBB
		g.pushArrayOperand(nextEntries, boxedName)
		g.pushArrayOperand(nextEntries, storedVal)
		firstMatchEndBB := g.currentBB
		firstMatchEndBB.Terminator = &ir.JumpTerm{Target: nextBB}

		g.currentBB = nextBB
		nextBB.Phis = append(nextBB.Phis, &ir.PhiInst{Res: nextFound, Incoming: []ir.PhiIncoming{
			{Block: copyEndBB, Value: found},
			{Block: matchBB, Value: found},
			{Block: firstMatchEndBB, Value: ir.ConstBool{Value: true}},
		}})
		nextBB.Instructions = append(nextBB.Instructions, &ir.BinaryInst{Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 2}})
		nextBB.Terminator = &ir.JumpTerm{Target: condBB}

		g.currentBB = doneBB
		doneBB.Terminator = &ir.BranchTerm{Cond: found, Then: storeBB, Else: appendBB}

		g.currentBB = appendBB
		g.pushArrayOperand(nextEntries, boxedName)
		g.pushArrayOperand(nextEntries, storedVal)
		appendEndBB := g.currentBB
		appendEndBB.Terminator = &ir.JumpTerm{Target: storeBB}

		g.currentBB = storeBB
		offsets, _, _ := g.objectLayout(g.semaResult.FormDataType)
		storeBB.Instructions = append(storeBB.Instructions, &ir.SetFieldInst{
			Obj: fd, Field: "$entries", Offset: offsets["$entries"], Val: nextEntries,
		})
		return ir.ConstUndefined{}, true

	case "keys":
		outArr := g.currentFn.NewValue("fd_keys_arr", types.NewArray(types.TypeString))
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{
			Res: outArr, ElemType: types.TypeString, Length: ir.ConstNumber{Value: 0},
		})

		entry := g.currentBB
		condBB := g.currentFn.NewBlock("fd_keys_cond")
		bodyBB := g.currentFn.NewBlock("fd_keys_body")
		nextBB := g.currentFn.NewBlock("fd_keys_next")
		doneBB := g.currentFn.NewBlock("fd_keys_done")

		entry.Terminator = &ir.JumpTerm{Target: condBB}
		index := g.currentFn.NewValue("fd_keys_i", types.TypeNumber)
		nextIndex := g.currentFn.NewValue("fd_keys_next_i", types.TypeNumber)
		condBB.Phis = append(condBB.Phis, &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{
			{Block: entry, Value: ir.ConstNumber{Value: 0}},
			{Block: nextBB, Value: nextIndex},
		}})

		totalLen := g.currentFn.NewValue("fd_entries_len", types.TypeNumber)
		condBB.Instructions = append(condBB.Instructions, &ir.ArrayLengthInst{Res: totalLen, Array: entries})
		more := g.currentFn.NewValue("fd_keys_more", types.TypeBoolean)
		condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: totalLen})
		condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: doneBB}

		boxedKey := g.currentFn.NewValue("fd_keys_boxed_key", types.TypeAny)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: boxedKey, Array: entries, Index: index})
		keyStr := g.currentFn.NewValue("fd_keys_key_str", types.TypeString)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.CallInst{
			Res: keyStr, Callee: "ts_js_unbox_string", Args: []ir.Operand{boxedKey}, ParamTypes: []types.Type{types.TypeAny},
		})
		g.currentBB = bodyBB
		g.pushArrayOperand(outArr, keyStr)
		bodyEndBB := g.currentBB
		bodyEndBB.Terminator = &ir.JumpTerm{Target: nextBB}

		g.currentBB = nextBB
		nextBB.Instructions = append(nextBB.Instructions, &ir.BinaryInst{Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 2}})
		nextBB.Terminator = &ir.JumpTerm{Target: condBB}

		g.currentBB = doneBB
		return outArr, true

	case "values":
		outArr := g.currentFn.NewValue("fd_values_arr", types.NewArray(types.TypeAny))
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{
			Res: outArr, ElemType: types.TypeAny, Length: ir.ConstNumber{Value: 0},
		})

		entry := g.currentBB
		condBB := g.currentFn.NewBlock("fd_values_cond")
		bodyBB := g.currentFn.NewBlock("fd_values_body")
		nextBB := g.currentFn.NewBlock("fd_values_next")
		doneBB := g.currentFn.NewBlock("fd_values_done")

		entry.Terminator = &ir.JumpTerm{Target: condBB}
		index := g.currentFn.NewValue("fd_values_i", types.TypeNumber)
		nextIndex := g.currentFn.NewValue("fd_values_next_i", types.TypeNumber)
		condBB.Phis = append(condBB.Phis, &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{
			{Block: entry, Value: ir.ConstNumber{Value: 0}},
			{Block: nextBB, Value: nextIndex},
		}})

		totalLen := g.currentFn.NewValue("fd_entries_len", types.TypeNumber)
		condBB.Instructions = append(condBB.Instructions, &ir.ArrayLengthInst{Res: totalLen, Array: entries})
		more := g.currentFn.NewValue("fd_values_more", types.TypeBoolean)
		condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: totalLen})
		condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: doneBB}

		valIdx := g.currentFn.NewValue("fd_values_val_idx", types.TypeNumber)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.BinaryInst{Res: valIdx, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
		valElem := g.currentFn.NewValue("fd_values_val_elem", types.TypeAny)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: valElem, Array: entries, Index: valIdx})
		g.currentBB = bodyBB
		g.pushArrayOperand(outArr, valElem)
		bodyEndBB := g.currentBB
		bodyEndBB.Terminator = &ir.JumpTerm{Target: nextBB}

		g.currentBB = nextBB
		nextBB.Instructions = append(nextBB.Instructions, &ir.BinaryInst{Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 2}})
		nextBB.Terminator = &ir.JumpTerm{Target: condBB}

		g.currentBB = doneBB
		return outArr, true

	case "entries":
		outArr := g.currentFn.NewValue("fd_entries_arr", types.NewArray(types.TypeAny))
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{
			Res: outArr, ElemType: types.TypeAny, Length: ir.ConstNumber{Value: 0},
		})

		entry := g.currentBB
		condBB := g.currentFn.NewBlock("fd_entries_cond")
		bodyBB := g.currentFn.NewBlock("fd_entries_body")
		nextBB := g.currentFn.NewBlock("fd_entries_next")
		doneBB := g.currentFn.NewBlock("fd_entries_done")

		entry.Terminator = &ir.JumpTerm{Target: condBB}
		index := g.currentFn.NewValue("fd_entries_i", types.TypeNumber)
		nextIndex := g.currentFn.NewValue("fd_entries_next_i", types.TypeNumber)
		condBB.Phis = append(condBB.Phis, &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{
			{Block: entry, Value: ir.ConstNumber{Value: 0}},
			{Block: nextBB, Value: nextIndex},
		}})

		totalLen := g.currentFn.NewValue("fd_entries_len", types.TypeNumber)
		condBB.Instructions = append(condBB.Instructions, &ir.ArrayLengthInst{Res: totalLen, Array: entries})
		more := g.currentFn.NewValue("fd_entries_more", types.TypeBoolean)
		condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: totalLen})
		condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: doneBB}

		boxedKey := g.currentFn.NewValue("fd_entries_boxed_key", types.TypeAny)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: boxedKey, Array: entries, Index: index})
		valIdx := g.currentFn.NewValue("fd_entries_val_idx", types.TypeNumber)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.BinaryInst{Res: valIdx, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
		valElem := g.currentFn.NewValue("fd_entries_val_elem", types.TypeAny)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: valElem, Array: entries, Index: valIdx})

		// Allocate pair tuple/array [key, val]
		pair := g.currentFn.NewValue("fd_pair", types.NewArray(types.TypeAny))
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.AllocArrayInst{
			Res: pair, ElemType: types.TypeAny, Length: ir.ConstNumber{Value: 2},
		})
		bodyBB.Instructions = append(bodyBB.Instructions,
			&ir.SetElementInst{Array: pair, Index: ir.ConstNumber{Value: 0}, Val: boxedKey},
			&ir.SetElementInst{Array: pair, Index: ir.ConstNumber{Value: 1}, Val: valElem},
		)

		g.currentBB = bodyBB
		g.pushArrayOperand(outArr, pair)
		bodyEndBB := g.currentBB
		bodyEndBB.Terminator = &ir.JumpTerm{Target: nextBB}

		g.currentBB = nextBB
		nextBB.Instructions = append(nextBB.Instructions, &ir.BinaryInst{Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 2}})
		nextBB.Terminator = &ir.JumpTerm{Target: condBB}

		g.currentBB = doneBB
		return outArr, true

	case "forEach":
		cb := g.lowerExpr(e.Args[0])

		entry := g.currentBB
		condBB := g.currentFn.NewBlock("fd_foreach_cond")
		bodyBB := g.currentFn.NewBlock("fd_foreach_body")
		nextBB := g.currentFn.NewBlock("fd_foreach_next")
		doneBB := g.currentFn.NewBlock("fd_foreach_done")

		entry.Terminator = &ir.JumpTerm{Target: condBB}
		index := g.currentFn.NewValue("fd_foreach_i", types.TypeNumber)
		nextIndex := g.currentFn.NewValue("fd_foreach_next_i", types.TypeNumber)
		condBB.Phis = append(condBB.Phis, &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{
			{Block: entry, Value: ir.ConstNumber{Value: 0}},
			{Block: nextBB, Value: nextIndex},
		}})

		totalLen := g.currentFn.NewValue("fd_entries_len", types.TypeNumber)
		condBB.Instructions = append(condBB.Instructions, &ir.ArrayLengthInst{Res: totalLen, Array: entries})
		more := g.currentFn.NewValue("fd_foreach_more", types.TypeBoolean)
		condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: totalLen})
		condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: doneBB}

		boxedKey := g.currentFn.NewValue("fd_foreach_boxed_key", types.TypeAny)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: boxedKey, Array: entries, Index: index})
		keyStr := g.currentFn.NewValue("fd_foreach_key_str", types.TypeString)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.CallInst{
			Res: keyStr, Callee: "ts_js_unbox_string", Args: []ir.Operand{boxedKey}, ParamTypes: []types.Type{types.TypeAny},
		})
		valIdx := g.currentFn.NewValue("fd_foreach_val_idx", types.TypeNumber)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.BinaryInst{Res: valIdx, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
		valElem := g.currentFn.NewValue("fd_foreach_val_elem", types.TypeAny)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: valElem, Array: entries, Index: valIdx})

		// call cb(val, key, fd)
		boxedFd := g.boxJSValue(fd, g.semaResult.FormDataType)
		callRes := g.currentFn.NewValue("fd_cb_res", types.TypeVoid)
		bodyBB.Instructions = append(bodyBB.Instructions, &ir.IndirectCallInst{
			Res:        callRes,
			Closure:    cb,
			ThisArg:    nil,
			Args:       []ir.Operand{valElem, keyStr, boxedFd},
			ParamTypes: []types.Type{types.TypeAny, types.TypeString, types.TypeAny},
		})
		bodyBB.Terminator = &ir.JumpTerm{Target: nextBB}

		nextBB.Instructions = append(nextBB.Instructions, &ir.BinaryInst{Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 2}})
		nextBB.Terminator = &ir.JumpTerm{Target: condBB}

		g.currentBB = doneBB
		return ir.ConstUndefined{}, true
	}

	return nil, false
}
