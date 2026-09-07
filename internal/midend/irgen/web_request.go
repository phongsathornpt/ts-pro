package irgen

import (
	"strings"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func objectLiteralProperty(lit *ast.ObjectLit, name string) ast.Expr {
	if lit == nil {
		return nil
	}
	for _, prop := range lit.Properties {
		if !prop.Spread && prop.Key == name {
			return prop.Value
		}
	}
	return nil
}

func (g *generator) newEmptyHeaders() ir.Operand {
	t := g.semaResult.HeadersType
	offsets, refMask, shape := g.objectLayout(t)
	headers := g.currentFn.NewValue("request_headers", t)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
		Res: headers, Shape: shape, FieldCount: len(offsets), RefMask: refMask,
	})
	entries := g.currentFn.NewValue("request_headers_entries", types.NewArray(types.TypeString))
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.AllocArrayInst{Res: entries, ElemType: types.TypeString, Length: ir.ConstNumber{Value: 0}},
		&ir.SetFieldInst{Obj: headers, Field: "$entries", Offset: offsets["$entries"], Val: entries},
	)
	return headers
}

func (g *generator) cloneHeaders(src ir.Operand) ir.Operand {
	dst := g.newEmptyHeaders()
	srcEntries := g.headersEntries(src)
	dstEntries := g.headersEntries(dst)
	length := g.currentFn.NewValue("request_headers_copy_len", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: srcEntries})
	pre := g.currentBB
	cond := g.currentFn.NewBlock("request_headers_copy_cond")
	body := g.currentFn.NewBlock("request_headers_copy_body")
	done := g.currentFn.NewBlock("request_headers_copy_done")
	pre.Terminator = &ir.JumpTerm{Target: cond}
	idx := g.currentFn.NewValue("request_headers_copy_i", types.TypeNumber)
	next := g.currentFn.NewValue("request_headers_copy_next", types.TypeNumber)
	cond.Phis = append(cond.Phis, &ir.PhiInst{Res: idx, Incoming: []ir.PhiIncoming{
		{Block: pre, Value: ir.ConstNumber{Value: 0}}, {Block: body, Value: next},
	}})
	more := g.currentFn.NewValue("request_headers_copy_more", types.TypeBoolean)
	cond.Instructions = append(cond.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: idx, RHS: length})
	cond.Terminator = &ir.BranchTerm{Cond: more, Then: body, Else: done}
	item := g.currentFn.NewValue("request_headers_copy_item", types.TypeString)
	body.Instructions = append(body.Instructions,
		&ir.GetElementInst{Res: item, Array: srcEntries, Index: idx},
		&ir.ArrayPushInst{Res: g.currentFn.NewValue("request_headers_copy_push", types.TypeNumber), Array: dstEntries, Val: item},
		&ir.BinaryInst{Res: next, Op: ir.OpAdd, LHS: idx, RHS: ir.ConstNumber{Value: 1}},
	)
	body.Terminator = &ir.JumpTerm{Target: cond}
	g.currentBB = done
	return dst
}

func (g *generator) requestField(req ir.Operand, name string, typ types.Type) ir.Operand {
	offsets, _, _ := g.objectLayout(g.semaResult.RequestType)
	res := g.currentFn.NewValue("request_"+strings.TrimPrefix(name, "$"), typ)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: res, Obj: req, Field: name, Offset: offsets[name]})
	return res
}

func (g *generator) setRequestField(req ir.Operand, name string, value ir.Operand) {
	offsets, _, _ := g.objectLayout(g.semaResult.RequestType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: req, Field: name, Offset: offsets[name], Val: value})
}

func (g *generator) emptyByteBuffer() ir.Operand {
	buf := g.currentFn.NewValue("request_empty_body", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: buf, Callee: "ts_byte_buffer_new", Args: []ir.Operand{ir.ConstNumber{Value: 0}}, ParamTypes: []types.Type{types.TypeNumber},
	})
	return buf
}

func (g *generator) copyByteBuffer(data ir.Operand) ir.Operand {
	length := g.currentFn.NewValue("request_body_len", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: length, Callee: "ts_byte_buffer_len", Args: []ir.Operand{data}, ParamTypes: []types.Type{g.semaResult.ByteBufferType},
	})
	copy := g.currentFn.NewValue("request_body_copy", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: copy, Callee: "ts_byte_buffer_slice", Args: []ir.Operand{data, ir.ConstNumber{Value: 0}, length}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber},
	})
	return copy
}

func (g *generator) lowerRequestBody(expr ast.Expr) (ir.Operand, ir.Operand) {
	if expr == nil {
		return g.emptyByteBuffer(), ir.ConstBool{Value: false}
	}
	typ := g.semanticType(expr)
	value := g.lowerExpr(expr)
	if typ == types.TypeString {
		data := g.currentFn.NewValue("request_body_string", g.semaResult.ByteBufferType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: data, Callee: "ts_byte_buffer_from_utf8_string", Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeString},
		})
		return data, ir.ConstBool{Value: true}
	}
	if obj, ok := typ.(*types.ObjectType); ok {
		switch obj.Name {
		case "$Blob", "$File":
			return g.copyByteBuffer(g.blobData(value, obj)), ir.ConstBool{Value: true}
		case "$ArrayBuffer":
			return g.copyByteBuffer(g.arrayBufferData(value)), ir.ConstBool{Value: true}
		case "$Uint8Array":
			raw := g.uint8ArrayField(value, "$data", g.semaResult.ByteBufferType)
			offset := g.uint8ArrayField(value, "byteOffset", types.TypeNumber)
			length := g.uint8ArrayField(value, "length", types.TypeNumber)
			end := g.currentFn.NewValue("request_u8_end", types.TypeNumber)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: end, Op: ir.OpAdd, LHS: offset, RHS: length})
			data := g.currentFn.NewValue("request_u8_body", g.semaResult.ByteBufferType)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
				Res: data, Callee: "ts_byte_buffer_slice", Args: []ir.Operand{raw, offset, end}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber},
			})
			return data, ir.ConstBool{Value: true}
		}
	}
	str := g.coerceStringType(typ, value)
	data := g.currentFn.NewValue("request_body_coerced", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: data, Callee: "ts_byte_buffer_from_utf8_string", Args: []ir.Operand{str}, ParamTypes: []types.Type{types.TypeString},
	})
	return data, ir.ConstBool{Value: true}
}

func (g *generator) lowerRequestNew(e *ast.NewExpr) ir.Operand {
	t := g.semaResult.RequestType
	offsets, refMask, shape := g.objectLayout(t)
	req := g.currentFn.NewValue("request", t)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{Res: req, Shape: shape, FieldCount: len(offsets), RefMask: refMask})

	var method ir.Operand = ir.ConstString{Value: "GET"}
	var url ir.Operand
	headers := g.newEmptyHeaders()
	data := g.emptyByteBuffer()
	hasBody := ir.Operand(ir.ConstBool{Value: false})
	signalValue := g.currentFn.NewValue("request_signal", g.semaResult.AbortSignalType)
	var signal ir.Operand = signalValue
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: signalValue, Callee: "ts_abort_signal_new"})

	inputType := g.semanticType(e.Args[0])
	if obj, ok := inputType.(*types.ObjectType); ok && obj.Name == "$Request" {
		source := g.lowerExpr(e.Args[0])
		method = g.requestField(source, "method", types.TypeString)
		url = g.requestField(source, "url", types.TypeString)
		headers = g.cloneHeaders(g.requestField(source, "headers", g.semaResult.HeadersType))
		data = g.copyByteBuffer(g.requestField(source, "$bodyData", g.semaResult.ByteBufferType))
		hasBody = g.requestField(source, "$hasBody", types.TypeBoolean)
		signal = g.requestField(source, "signal", g.semaResult.AbortSignalType)
	} else {
		invalid := g.currentFn.NewBlock("request_url_invalid")
		parsed := g.lowerURLResolveAndParse(e.Args[0], nil, invalid)
		url, _ = g.lowerURLMember(parsed, "href")
		okBB := g.currentBB
		g.currentBB = invalid
		err := g.newWebError(ir.ConstString{Value: "Failed to construct Request: invalid URL"}, ir.ConstString{Value: "TypeError"})
		g.routeThrownValue(err)
		g.currentBB = okBB
	}

	if len(e.Args) > 1 {
		if lit, ok := e.Args[1].(*ast.ObjectLit); ok {
			if methodExpr := objectLiteralProperty(lit, "method"); methodExpr != nil {
				raw := g.lowerExpr(methodExpr)
				method = g.coerceStringType(g.semanticType(methodExpr), raw)
				if c, ok := method.(ir.ConstString); ok {
					method = ir.ConstString{Value: strings.ToUpper(c.Value)}
				}
			}
			if headersExpr := objectLiteralProperty(lit, "headers"); headersExpr != nil {
				fake := &ast.NewExpr{ClassName: "Headers", Args: []ast.Expr{headersExpr}}
				headers = g.lowerHeadersNew(fake)
			}
			if bodyExpr := objectLiteralProperty(lit, "body"); bodyExpr != nil {
				data, hasBody = g.lowerRequestBody(bodyExpr)
			}
			if signalExpr := objectLiteralProperty(lit, "signal"); signalExpr != nil {
				signal = g.lowerExpr(signalExpr)
			}
		}
	}

	g.validateRequestMethodBody(method, hasBody)
	bodyStream := g.newBodyStream(data, hasBody, req, g.semaResult.RequestType)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: req, Field: "$bodyData", Offset: offsets["$bodyData"], Val: data},
		&ir.SetFieldInst{Obj: req, Field: "$hasBody", Offset: offsets["$hasBody"], Val: hasBody},
		&ir.SetFieldInst{Obj: req, Field: "$bodyStream", Offset: offsets["$bodyStream"], Val: bodyStream},
		&ir.SetFieldInst{Obj: req, Field: "bodyUsed", Offset: offsets["bodyUsed"], Val: ir.ConstBool{Value: false}},
		&ir.SetFieldInst{Obj: req, Field: "headers", Offset: offsets["headers"], Val: headers},
		&ir.SetFieldInst{Obj: req, Field: "method", Offset: offsets["method"], Val: method},
		&ir.SetFieldInst{Obj: req, Field: "signal", Offset: offsets["signal"], Val: signal},
		&ir.SetFieldInst{Obj: req, Field: "url", Offset: offsets["url"], Val: url},
	)
	return req
}

func (g *generator) validateRequestMethodBody(method, hasBody ir.Operand) {
	isGet := g.urlStringEqual(method, "GET")
	isHead := g.urlStringEqual(method, "HEAD")
	getOrHead := g.currentFn.NewValue("request_get_or_head", types.TypeBoolean)
	invalid := g.currentFn.NewValue("request_body_method_invalid", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: getOrHead, Op: ir.OpOr, LHS: isGet, RHS: isHead},
		&ir.BinaryInst{Res: invalid, Op: ir.OpAnd, LHS: getOrHead, RHS: hasBody},
	)
	errBB := g.currentFn.NewBlock("request_body_method_error")
	okBB := g.currentFn.NewBlock("request_body_method_ok")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: invalid, Then: errBB, Else: okBB}
	g.currentBB = errBB
	g.routeThrownValue(g.newWebError(ir.ConstString{Value: "Request with GET/HEAD method cannot have body"}, ir.ConstString{Value: "TypeError"}))
	g.currentBB = okBB
}

func (g *generator) ensureRequestBodyUnused(req ir.Operand) {
	used := g.requestField(req, "bodyUsed", types.TypeBoolean)
	fail := g.currentFn.NewBlock("request_body_used_error")
	checkBody := g.currentFn.NewBlock("request_body_lock_check")
	checkLocked := g.currentFn.NewBlock("request_body_locked_check")
	ok := g.currentFn.NewBlock("request_body_unused")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: used, Then: fail, Else: checkBody}

	g.currentBB = checkBody
	hasBody := g.requestField(req, "$hasBody", types.TypeBoolean)
	g.currentBB.Terminator = &ir.BranchTerm{Cond: hasBody, Then: checkLocked, Else: ok}

	g.currentBB = checkLocked
	bodyValue := g.requestField(req, "$bodyStream", types.TypeAny)
	stream := g.coerceJSValueBoundary(bodyValue, types.TypeAny, g.semaResult.ReadableStreamType)
	sOffsets, _, _ := g.objectLayout(g.semaResult.ReadableStreamType)
	locked := g.currentFn.NewValue("request_body_locked", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: locked, Obj: stream, Field: "locked", Offset: sOffsets["locked"]})
	g.currentBB.Terminator = &ir.BranchTerm{Cond: locked, Then: fail, Else: ok}

	g.currentBB = fail
	err := g.newWebError(ir.ConstString{Value: "Body is unusable"}, ir.ConstString{Value: "TypeError"})
	g.routeThrownValue(err)
	g.currentBB = ok
}

func (g *generator) lowerRequestMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || objType.Name != "$Request" {
		return nil, false
	}
	req := g.lowerExpr(mem.Object)
	switch mem.Property {
	case "clone":
		g.ensureRequestBodyUnused(req)
		t := g.semaResult.RequestType
		offsets, refMask, shape := g.objectLayout(t)
		clone := g.currentFn.NewValue("request_clone", t)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{Res: clone, Shape: shape, FieldCount: len(offsets), RefMask: refMask})
		method := g.requestField(req, "method", types.TypeString)
		url := g.requestField(req, "url", types.TypeString)
		headers := g.cloneHeaders(g.requestField(req, "headers", g.semaResult.HeadersType))
		data := g.copyByteBuffer(g.requestField(req, "$bodyData", g.semaResult.ByteBufferType))
		hasBody := g.requestField(req, "$hasBody", types.TypeBoolean)
		signal := g.requestField(req, "signal", g.semaResult.AbortSignalType)
		bodyStream := g.newBodyStream(data, hasBody, clone, g.semaResult.RequestType)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.SetFieldInst{Obj: clone, Field: "$bodyData", Offset: offsets["$bodyData"], Val: data},
			&ir.SetFieldInst{Obj: clone, Field: "$hasBody", Offset: offsets["$hasBody"], Val: hasBody},
			&ir.SetFieldInst{Obj: clone, Field: "$bodyStream", Offset: offsets["$bodyStream"], Val: bodyStream},
			&ir.SetFieldInst{Obj: clone, Field: "bodyUsed", Offset: offsets["bodyUsed"], Val: ir.ConstBool{Value: false}},
			&ir.SetFieldInst{Obj: clone, Field: "headers", Offset: offsets["headers"], Val: headers},
			&ir.SetFieldInst{Obj: clone, Field: "method", Offset: offsets["method"], Val: method},
			&ir.SetFieldInst{Obj: clone, Field: "signal", Offset: offsets["signal"], Val: signal},
			&ir.SetFieldInst{Obj: clone, Field: "url", Offset: offsets["url"], Val: url},
		)
		return clone, true
	case "text", "arrayBuffer", "bytes", "blob":
		g.ensureRequestBodyUnused(req)
		g.setRequestField(req, "bodyUsed", ir.ConstBool{Value: true})
		data := g.requestField(req, "$bodyData", g.semaResult.ByteBufferType)
		copy := g.copyByteBuffer(data)
		switch mem.Property {
		case "text":
			text := g.currentFn.NewValue("request_text", types.TypeString)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: text, Callee: "ts_byte_buffer_to_utf8_string", Args: []ir.Operand{copy}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}})
			return g.makeImmediatePromiseTask(text, types.TypeString, types.TypeString), true
		case "arrayBuffer":
			ab := g.newArrayBufferFromData(copy)
			return g.makeImmediatePromiseTask(ab, g.semaResult.ArrayBufferType, g.semaResult.ArrayBufferType), true
		case "bytes":
			length := g.currentFn.NewValue("request_bytes_len", types.TypeNumber)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: length, Callee: "ts_byte_buffer_len", Args: []ir.Operand{copy}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}})
			ab := g.newArrayBufferFromData(copy)
			u8 := g.newUint8ArrayView(copy, ab, ir.ConstNumber{Value: 0}, length)
			return g.makeImmediatePromiseTask(u8, g.semaResult.Uint8ArrayType, g.semaResult.Uint8ArrayType), true
		case "blob":
			length := g.currentFn.NewValue("request_blob_len", types.TypeNumber)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: length, Callee: "ts_byte_buffer_len", Args: []ir.Operand{copy}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}})
			blob := g.newBlobObject(copy, length, ir.ConstString{Value: ""})
			return g.makeImmediatePromiseTask(blob, g.semaResult.BlobType, g.semaResult.BlobType), true
		}
	}
	return nil, false
}
