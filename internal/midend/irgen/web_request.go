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

func (g *generator) lowerRequestBodyValue(typ types.Type, value ir.Operand) (ir.Operand, ir.Operand) {
	if typ == types.TypeNull || typ == types.TypeUndefined {
		return g.emptyByteBuffer(), ir.ConstBool{Value: false}
	}
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

func (g *generator) lowerRequestBody(expr ast.Expr) (ir.Operand, ir.Operand) {
	if expr == nil {
		return g.emptyByteBuffer(), ir.ConstBool{Value: false}
	}
	typ := g.semanticType(expr)
	if typ == types.TypeNull || typ == types.TypeUndefined {
		return g.emptyByteBuffer(), ir.ConstBool{Value: false}
	}
	return g.lowerRequestBodyValue(typ, g.lowerExpr(expr))
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
		initExpr := e.Args[1]
		initType := g.semanticType(initExpr)
		initValue := g.lowerExpr(initExpr)
		if obj, ok := initType.(*types.ObjectType); ok {
			offsetsInit, _, _ := g.objectLayout(obj)
			readField := func(name string) (ir.Operand, types.Type, bool) {
				field, exists := obj.Fields[name]
				if !exists {
					return nil, nil, false
				}
				raw := g.currentFn.NewValue("request_init_"+name, field.Type)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: raw, Obj: initValue, Field: name, Offset: offsetsInit[name]})
				return raw, field.Type, true
			}
			if raw, typ, ok := readField("method"); ok {
				method = g.normalizeRequestMethod(g.coerceStringType(typ, raw))
			}
			if raw, typ, ok := readField("headers"); ok {
				headers = g.lowerHeadersInitValue(typ, raw)
			}
			if raw, typ, ok := readField("body"); ok {
				data, hasBody = g.lowerRequestBodyValue(typ, raw)
			}
			if raw, typ, ok := readField("signal"); ok {
				signal = raw
				if irJSValueType(raw.Type()) {
					signal = g.coerceJSValueBoundary(raw, typ, g.semaResult.AbortSignalType)
				}
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

func (g *generator) normalizeRequestMethod(method ir.Operand) ir.Operand {
	// HTTP methods use the same RFC token grammar as header names. Reuse the
	// token validator, then apply Fetch's forbidden-method check case-insensitively.
	g.validateHeaderName(method)
	upperForForbidden := g.stringAsciiUpper(method)
	isConnect := g.urlStringEqual(upperForForbidden, "CONNECT")
	isTrace := g.urlStringEqual(upperForForbidden, "TRACE")
	isTrack := g.urlStringEqual(upperForForbidden, "TRACK")
	forbidden1 := g.currentFn.NewValue("request_method_forbidden_1", types.TypeBoolean)
	forbidden := g.currentFn.NewValue("request_method_forbidden", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: forbidden1, Op: ir.OpOr, LHS: isConnect, RHS: isTrace},
		&ir.BinaryInst{Res: forbidden, Op: ir.OpOr, LHS: forbidden1, RHS: isTrack},
	)
	forbiddenBB := g.currentFn.NewBlock("request_method_forbidden_error")
	methodOKBB := g.currentFn.NewBlock("request_method_allowed")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: forbidden, Then: forbiddenBB, Else: methodOKBB}
	g.currentBB = forbiddenBB
	g.routeThrownValue(g.newWebError(ir.ConstString{Value: "Forbidden request method"}, ir.ConstString{Value: "TypeError"}))
	g.currentBB = methodOKBB

	if c, ok := method.(ir.ConstString); ok {
		upper := strings.ToUpper(c.Value)
		for _, standard := range []string{"DELETE", "GET", "HEAD", "OPTIONS", "POST", "PUT"} {
			if upper == standard {
				return ir.ConstString{Value: upper}
			}
		}
		return method
	}

	upper := g.stringAsciiUpper(method)
	var standardMatch ir.Operand
	for i, standard := range []string{"DELETE", "GET", "HEAD", "OPTIONS", "POST", "PUT"} {
		eq := g.urlStringEqual(upper, standard)
		if i == 0 {
			standardMatch = eq
		} else {
			combined := g.currentFn.NewValue("request_method_standard_any", types.TypeBoolean)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: combined, Op: ir.OpOr, LHS: standardMatch, RHS: eq})
			standardMatch = combined
		}
	}
	standardBB := g.currentFn.NewBlock("request_method_standard")
	customBB := g.currentFn.NewBlock("request_method_custom")
	joinBB := g.currentFn.NewBlock("request_method_normalized")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: standardMatch, Then: standardBB, Else: customBB}
	standardBB.Terminator = &ir.JumpTerm{Target: joinBB}
	customBB.Terminator = &ir.JumpTerm{Target: joinBB}
	g.currentBB = joinBB
	result := g.currentFn.NewValue("request_method", types.TypeString)
	joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{Res: result, Incoming: []ir.PhiIncoming{
		{Block: standardBB, Value: upper},
		{Block: customBB, Value: method},
	}})
	return result
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

func (g *generator) lowerBodyJSON(data ir.Operand) ir.Operand {
	text := g.currentFn.NewValue("body_json_text", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: text, Callee: "ts_byte_buffer_to_utf8_string", Args: []ir.Operand{data}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}})
	value := g.lowerRuntimeJSONParse(text)
	return g.makeImmediatePromiseTask(value, types.TypeAny, types.TypeAny)
}

func (g *generator) lowerBodyURLEncodedFormData(data, headers ir.Operand) ir.Operand {
	const mime = "application/x-www-form-urlencoded"
	hasContentType := g.lowerHeadersHas(headers, ir.ConstString{Value: "content-type"})
	checkBB := g.currentFn.NewBlock("body_form_data_content_type_check")
	failBB := g.currentFn.NewBlock("body_form_data_content_type_error")
	parseBB := g.currentFn.NewBlock("body_form_data_parse")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: hasContentType, Then: checkBB, Else: failBB}

	g.currentBB = checkBB
	contentType := g.lowerHeadersGet(headers, ir.ConstString{Value: "content-type"})
	contentTypeLen := g.urlStringLen(contentType)
	longEnough := g.currentFn.NewValue("body_form_data_mime_long_enough", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: longEnough, Op: ir.OpGe, LHS: contentTypeLen, RHS: ir.ConstNumber{Value: float64(len(mime))}})
	prefixBB := g.currentFn.NewBlock("body_form_data_mime_prefix")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: longEnough, Then: prefixBB, Else: failBB}

	g.currentBB = prefixBB
	prefix := g.urlStringSlice(contentType, ir.ConstNumber{Value: 0}, ir.ConstNumber{Value: float64(len(mime))})
	isForm := g.urlStringEqual(prefix, mime)
	g.currentBB.Terminator = &ir.BranchTerm{Cond: isForm, Then: parseBB, Else: failBB}

	g.currentBB = failBB
	g.routeThrownValue(g.newWebError(ir.ConstString{Value: "Body MIME type is not supported by formData()"}, ir.ConstString{Value: "TypeError"}))

	g.currentBB = parseBB
	text := g.currentFn.NewValue("body_form_data_text", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: text, Callee: "ts_byte_buffer_to_utf8_string", Args: []ir.Operand{data}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}})
	pairs := g.currentFn.NewValue("body_form_data_pairs", types.NewArray(types.TypeString))
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{Res: pairs, ElemType: types.TypeString, Length: ir.ConstNumber{Value: 0}})
	g.lowerURLSearchParamsParse(pairs, text)

	fd := g.lowerFormDataNew(&ast.NewExpr{ClassName: "FormData"})
	entries := g.formDataEntries(fd)
	length := g.currentFn.NewValue("body_form_data_pairs_len", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: pairs})
	entryBB := g.currentBB
	condBB := g.currentFn.NewBlock("body_form_data_copy_cond")
	bodyBB := g.currentFn.NewBlock("body_form_data_copy_body")
	doneBB := g.currentFn.NewBlock("body_form_data_copy_done")
	entryBB.Terminator = &ir.JumpTerm{Target: condBB}
	i := g.currentFn.NewValue("body_form_data_copy_i", types.TypeNumber)
	next := g.currentFn.NewValue("body_form_data_copy_next", types.TypeNumber)
	condBB.Phis = append(condBB.Phis, &ir.PhiInst{Res: i, Incoming: []ir.PhiIncoming{{Block: entryBB, Value: ir.ConstNumber{Value: 0}}, {Block: bodyBB, Value: next}}})
	more := g.currentFn.NewValue("body_form_data_copy_more", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: i, RHS: length})
	condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: doneBB}

	g.currentBB = bodyBB
	item := g.currentFn.NewValue("body_form_data_copy_item", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetElementInst{Res: item, Array: pairs, Index: i})
	boxed := g.boxJSValue(item, types.TypeString)
	g.pushArrayOperand(entries, boxed)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: next, Op: ir.OpAdd, LHS: i, RHS: ir.ConstNumber{Value: 1}})
	g.currentBB.Terminator = &ir.JumpTerm{Target: condBB}

	g.currentBB = doneBB
	return g.makeImmediatePromiseTask(fd, g.semaResult.FormDataType, g.semaResult.FormDataType)
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
	case "text", "arrayBuffer", "bytes", "blob", "formData", "json":
		g.ensureRequestBodyUnused(req)
		g.setRequestField(req, "bodyUsed", ir.ConstBool{Value: true})
		data := g.requestField(req, "$bodyData", g.semaResult.ByteBufferType)
		if mem.Property == "formData" {
			headers := g.requestField(req, "headers", g.semaResult.HeadersType)
			return g.lowerBodyFormData(data, headers), true
		}
		if mem.Property == "json" {
			return g.lowerBodyJSON(data), true
		}
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

func (g *generator) lowerHeadersInitValue(initType types.Type, initValue ir.Operand) ir.Operand {
	if obj, ok := initType.(*types.ObjectType); ok && obj.Name == "$Headers" {
		return g.cloneHeaders(initValue)
	}
	headers := g.newEmptyHeaders()
	if obj, ok := initType.(*types.ObjectType); ok {
		offsets, _, _ := g.objectLayout(obj)
		for _, name := range obj.FieldOrder {
			field := obj.Fields[name]
			value := g.currentFn.NewValue("fetch_init_header_"+name, field.Type)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: value, Obj: initValue, Field: name, Offset: offsets[name]})
			g.lowerHeadersAppendDirect(headers, ir.ConstString{Value: name}, g.coerceStringType(field.Type, value))
		}
		return headers
	}
	if arr, ok := initType.(*types.ArrayType); ok {
		length := g.currentFn.NewValue("fetch_init_headers_len", types.TypeNumber)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: initValue})
		pre := g.currentBB
		cond := g.currentFn.NewBlock("fetch_init_headers_cond")
		body := g.currentFn.NewBlock("fetch_init_headers_body")
		done := g.currentFn.NewBlock("fetch_init_headers_done")
		pre.Terminator = &ir.JumpTerm{Target: cond}
		i := g.currentFn.NewValue("fetch_init_headers_i", types.TypeNumber)
		next := g.currentFn.NewValue("fetch_init_headers_next", types.TypeNumber)
		cond.Phis = append(cond.Phis, &ir.PhiInst{Res: i, Incoming: []ir.PhiIncoming{{Block: pre, Value: ir.ConstNumber{Value: 0}}, {Block: body, Value: next}}})
		more := g.currentFn.NewValue("fetch_init_headers_more", types.TypeBoolean)
		cond.Instructions = append(cond.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: i, RHS: length})
		cond.Terminator = &ir.BranchTerm{Cond: more, Then: body, Else: done}
		g.currentBB = body
		pair := g.currentFn.NewValue("fetch_init_header_pair", arr.Elem)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetElementInst{Res: pair, Array: initValue, Index: i})
		if pairType, ok := arr.Elem.(*types.ArrayType); ok {
			key := g.currentFn.NewValue("fetch_init_header_key", pairType.Elem)
			value := g.currentFn.NewValue("fetch_init_header_value", pairType.Elem)
			g.currentBB.Instructions = append(g.currentBB.Instructions,
				&ir.GetElementInst{Res: key, Array: pair, Index: ir.ConstNumber{Value: 0}},
				&ir.GetElementInst{Res: value, Array: pair, Index: ir.ConstNumber{Value: 1}},
			)
			g.lowerHeadersAppendDirect(headers, g.coerceStringType(pairType.Elem, key), g.coerceStringType(pairType.Elem, value))
		}
		loopEnd := g.currentBB
		loopEnd.Instructions = append(loopEnd.Instructions, &ir.BinaryInst{Res: next, Op: ir.OpAdd, LHS: i, RHS: ir.ConstNumber{Value: 1}})
		loopEnd.Terminator = &ir.JumpTerm{Target: cond}
		cond.Phis[0].Incoming[1].Block = loopEnd
		g.currentBB = done
	}
	return headers
}
