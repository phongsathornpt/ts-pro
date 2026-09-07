package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) responseField(res ir.Operand, name string, typ types.Type) ir.Operand {
	offsets, _, _ := g.objectLayout(g.semaResult.ResponseType)
	out := g.currentFn.NewValue("response_"+name, typ)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: out, Obj: res, Field: name, Offset: offsets[name]})
	return out
}

func (g *generator) newResponseObject(data, hasBody, headers, status, statusText, typ, url, redirected ir.Operand) ir.Operand {
	t := g.semaResult.ResponseType
	offsets, refMask, shape := g.objectLayout(t)
	res := g.currentFn.NewValue("response", t)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{Res: res, Shape: shape, FieldCount: len(offsets), RefMask: refMask})
	bodyStream := g.newBodyStream(data, hasBody, res, g.semaResult.ResponseType)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: res, Field: "$bodyData", Offset: offsets["$bodyData"], Val: data},
		&ir.SetFieldInst{Obj: res, Field: "$hasBody", Offset: offsets["$hasBody"], Val: hasBody},
		&ir.SetFieldInst{Obj: res, Field: "$bodyStream", Offset: offsets["$bodyStream"], Val: bodyStream},
		&ir.SetFieldInst{Obj: res, Field: "bodyUsed", Offset: offsets["bodyUsed"], Val: ir.ConstBool{Value: false}},
		&ir.SetFieldInst{Obj: res, Field: "headers", Offset: offsets["headers"], Val: headers},
		&ir.SetFieldInst{Obj: res, Field: "ok", Offset: offsets["ok"], Val: g.responseOK(status)},
		&ir.SetFieldInst{Obj: res, Field: "redirected", Offset: offsets["redirected"], Val: redirected},
		&ir.SetFieldInst{Obj: res, Field: "status", Offset: offsets["status"], Val: status},
		&ir.SetFieldInst{Obj: res, Field: "statusText", Offset: offsets["statusText"], Val: statusText},
		&ir.SetFieldInst{Obj: res, Field: "type", Offset: offsets["type"], Val: typ},
		&ir.SetFieldInst{Obj: res, Field: "url", Offset: offsets["url"], Val: url},
	)
	return res
}

func (g *generator) responseOK(status ir.Operand) ir.Operand {
	ge := g.currentFn.NewValue("response_ok_ge", types.TypeBoolean)
	lt := g.currentFn.NewValue("response_ok_lt", types.TypeBoolean)
	ok := g.currentFn.NewValue("response_ok", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: ge, Op: ir.OpGe, LHS: status, RHS: ir.ConstNumber{Value: 200}},
		&ir.BinaryInst{Res: lt, Op: ir.OpLt, LHS: status, RHS: ir.ConstNumber{Value: 300}},
		&ir.BinaryInst{Res: ok, Op: ir.OpAnd, LHS: ge, RHS: lt},
	)
	return ok
}

func (g *generator) validateResponseStatus(status ir.Operand) {
	ge200 := g.currentFn.NewValue("response_status_ge_200", types.TypeBoolean)
	le599 := g.currentFn.NewValue("response_status_le_599", types.TypeBoolean)
	valid := g.currentFn.NewValue("response_status_valid", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: ge200, Op: ir.OpGe, LHS: status, RHS: ir.ConstNumber{Value: 200}},
		&ir.BinaryInst{Res: le599, Op: ir.OpLe, LHS: status, RHS: ir.ConstNumber{Value: 599}},
		&ir.BinaryInst{Res: valid, Op: ir.OpAnd, LHS: ge200, RHS: le599},
	)
	okBB := g.currentFn.NewBlock("response_status_ok")
	errBB := g.currentFn.NewBlock("response_status_error")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: valid, Then: okBB, Else: errBB}
	g.currentBB = errBB
	g.routeThrownValue(g.newWebError(ir.ConstString{Value: "Response status must be in the range 200 to 599"}, ir.ConstString{Value: "RangeError"}))
	g.currentBB = okBB
}

func (g *generator) validateResponseNullBodyStatus(status, hasBody ir.Operand) {
	var nullStatus ir.Operand
	for i, code := range []float64{204, 205, 304} {
		eq := g.currentFn.NewValue("response_null_body_status", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: eq, Op: ir.OpEq, LHS: status, RHS: ir.ConstNumber{Value: code}})
		if i == 0 {
			nullStatus = eq
		} else {
			combined := g.currentFn.NewValue("response_null_body_status_any", types.TypeBoolean)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: combined, Op: ir.OpOr, LHS: nullStatus, RHS: eq})
			nullStatus = combined
		}
	}
	invalid := g.currentFn.NewValue("response_null_body_invalid", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: invalid, Op: ir.OpAnd, LHS: nullStatus, RHS: hasBody})
	errBB := g.currentFn.NewBlock("response_null_body_error")
	okBB := g.currentFn.NewBlock("response_null_body_ok")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: invalid, Then: errBB, Else: okBB}
	g.currentBB = errBB
	g.routeThrownValue(g.newWebError(ir.ConstString{Value: "Response with null body status cannot have body"}, ir.ConstString{Value: "TypeError"}))
	g.currentBB = okBB
}

func (g *generator) validateResponseRedirectStatus(status ir.Operand) {
	var valid ir.Operand
	for i, code := range []float64{301, 302, 303, 307, 308} {
		eq := g.currentFn.NewValue("response_redirect_status", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: eq, Op: ir.OpEq, LHS: status, RHS: ir.ConstNumber{Value: code}})
		if i == 0 {
			valid = eq
		} else {
			combined := g.currentFn.NewValue("response_redirect_status_any", types.TypeBoolean)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: combined, Op: ir.OpOr, LHS: valid, RHS: eq})
			valid = combined
		}
	}
	okBB := g.currentFn.NewBlock("response_redirect_status_ok")
	errBB := g.currentFn.NewBlock("response_redirect_status_error")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: valid, Then: okBB, Else: errBB}
	g.currentBB = errBB
	g.routeThrownValue(g.newWebError(ir.ConstString{Value: "Invalid redirect status"}, ir.ConstString{Value: "RangeError"}))
	g.currentBB = okBB
}

func (g *generator) lowerResponseNew(e *ast.NewExpr) ir.Operand {
	data, hasBody := g.lowerRequestBody(nil)
	headers := g.newEmptyHeaders()
	status := ir.Operand(ir.ConstNumber{Value: 200})
	statusText := ir.Operand(ir.ConstString{Value: ""})
	if len(e.Args) > 0 {
		data, hasBody = g.lowerRequestBody(e.Args[0])
	}
	if len(e.Args) > 1 {
		if lit, ok := e.Args[1].(*ast.ObjectLit); ok {
			if ex := objectLiteralProperty(lit, "status"); ex != nil {
				status = g.lowerExpr(ex)
			}
			if ex := objectLiteralProperty(lit, "statusText"); ex != nil {
				statusText = g.coerceStringType(g.semanticType(ex), g.lowerExpr(ex))
			}
			if ex := objectLiteralProperty(lit, "headers"); ex != nil {
				headers = g.lowerHeadersNew(&ast.NewExpr{ClassName: "Headers", Args: []ast.Expr{ex}})
			}
		}
	}
	g.validateResponseStatus(status)
	g.validateHeaderValue(statusText)
	g.validateResponseNullBodyStatus(status, hasBody)
	return g.newResponseObject(data, hasBody, headers, status, statusText, ir.ConstString{Value: "default"}, ir.ConstString{Value: ""}, ir.ConstBool{Value: false})
}

func (g *generator) ensureResponseBodyUnused(res ir.Operand) {
	used := g.responseField(res, "bodyUsed", types.TypeBoolean)
	fail := g.currentFn.NewBlock("response_body_used_error")
	checkBody := g.currentFn.NewBlock("response_body_lock_check")
	checkLocked := g.currentFn.NewBlock("response_body_locked_check")
	ok := g.currentFn.NewBlock("response_body_unused")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: used, Then: fail, Else: checkBody}

	g.currentBB = checkBody
	hasBody := g.responseField(res, "$hasBody", types.TypeBoolean)
	g.currentBB.Terminator = &ir.BranchTerm{Cond: hasBody, Then: checkLocked, Else: ok}

	g.currentBB = checkLocked
	bodyValue := g.responseField(res, "$bodyStream", types.TypeAny)
	stream := g.coerceJSValueBoundary(bodyValue, types.TypeAny, g.semaResult.ReadableStreamType)
	sOffsets, _, _ := g.objectLayout(g.semaResult.ReadableStreamType)
	locked := g.currentFn.NewValue("response_body_locked", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: locked, Obj: stream, Field: "locked", Offset: sOffsets["locked"]})
	g.currentBB.Terminator = &ir.BranchTerm{Cond: locked, Then: fail, Else: ok}

	g.currentBB = fail
	g.routeThrownValue(g.newWebError(ir.ConstString{Value: "Body is unusable"}, ir.ConstString{Value: "TypeError"}))
	g.currentBB = ok
}

func (g *generator) lowerResponseCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	if ident, ok := mem.Object.(*ast.IdentExpr); ok && ident.Name == "Response" {
		switch mem.Property {
		case "error":
			return g.newResponseObject(g.emptyByteBuffer(), ir.ConstBool{Value: false}, g.newEmptyHeaders(), ir.ConstNumber{Value: 0}, ir.ConstString{Value: ""}, ir.ConstString{Value: "error"}, ir.ConstString{Value: ""}, ir.ConstBool{Value: false}), true
		case "redirect":
			status := ir.Operand(ir.ConstNumber{Value: 302})
			if len(e.Args) > 1 {
				status = g.lowerExpr(e.Args[1])
			}
			g.validateResponseRedirectStatus(status)
			headers := g.newEmptyHeaders()
			target := g.lowerExpr(e.Args[0])
			g.lowerHeadersAppendDirect(headers, ir.ConstString{Value: "location"}, g.coerceStringType(g.semanticType(e.Args[0]), target))
			return g.newResponseObject(g.emptyByteBuffer(), ir.ConstBool{Value: false}, headers, status, ir.ConstString{Value: ""}, ir.ConstString{Value: "default"}, ir.ConstString{Value: ""}, ir.ConstBool{Value: false}), true
		case "json":
			value := g.lowerExpr(e.Args[0])
			text := g.lowerJSONStringifyValue(value, value.Type())
			_, hasBody := g.lowerRequestBody(&ast.StringLit{Value: ""})
			dataValue := g.currentFn.NewValue("response_json_body", g.semaResult.ByteBufferType)
			var data ir.Operand = dataValue
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: dataValue, Callee: "ts_byte_buffer_from_utf8_string", Args: []ir.Operand{text}, ParamTypes: []types.Type{types.TypeString}})
			hasBody = ir.ConstBool{Value: true}
			headers := g.newEmptyHeaders()
			g.lowerHeadersAppendDirect(headers, ir.ConstString{Value: "content-type"}, ir.ConstString{Value: "application/json"})
			status := ir.Operand(ir.ConstNumber{Value: 200})
			if len(e.Args) > 1 {
				if lit, ok := e.Args[1].(*ast.ObjectLit); ok {
					if ex := objectLiteralProperty(lit, "status"); ex != nil {
						status = g.lowerExpr(ex)
					}
				}
			}
			g.validateResponseStatus(status)
			g.validateResponseNullBodyStatus(status, hasBody)
			return g.newResponseObject(data, hasBody, headers, status, ir.ConstString{Value: ""}, ir.ConstString{Value: "default"}, ir.ConstString{Value: ""}, ir.ConstBool{Value: false}), true
		}
		return nil, false
	}
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || objType.Name != "$Response" {
		return nil, false
	}
	res := g.lowerExpr(mem.Object)
	switch mem.Property {
	case "clone":
		g.ensureResponseBodyUnused(res)
		return g.newResponseObject(
			g.copyByteBuffer(g.responseField(res, "$bodyData", g.semaResult.ByteBufferType)),
			g.responseField(res, "$hasBody", types.TypeBoolean),
			g.cloneHeaders(g.responseField(res, "headers", g.semaResult.HeadersType)),
			g.responseField(res, "status", types.TypeNumber), g.responseField(res, "statusText", types.TypeString),
			g.responseField(res, "type", types.TypeString), g.responseField(res, "url", types.TypeString), g.responseField(res, "redirected", types.TypeBoolean)), true
	case "text", "arrayBuffer", "bytes", "blob":
		g.ensureResponseBodyUnused(res)
		offsets, _, _ := g.objectLayout(g.semaResult.ResponseType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: res, Field: "bodyUsed", Offset: offsets["bodyUsed"], Val: ir.ConstBool{Value: true}})
		data := g.copyByteBuffer(g.responseField(res, "$bodyData", g.semaResult.ByteBufferType))
		switch mem.Property {
		case "text":
			text := g.currentFn.NewValue("response_text", types.TypeString)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: text, Callee: "ts_byte_buffer_to_utf8_string", Args: []ir.Operand{data}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}})
			return g.makeImmediatePromiseTask(text, types.TypeString, types.TypeString), true
		case "arrayBuffer":
			ab := g.newArrayBufferFromData(data)
			return g.makeImmediatePromiseTask(ab, g.semaResult.ArrayBufferType, g.semaResult.ArrayBufferType), true
		case "bytes":
			ln := g.currentFn.NewValue("response_bytes_len", types.TypeNumber)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: ln, Callee: "ts_byte_buffer_len", Args: []ir.Operand{data}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}})
			ab := g.newArrayBufferFromData(data)
			u8 := g.newUint8ArrayView(data, ab, ir.ConstNumber{Value: 0}, ln)
			return g.makeImmediatePromiseTask(u8, g.semaResult.Uint8ArrayType, g.semaResult.Uint8ArrayType), true
		case "blob":
			ln := g.currentFn.NewValue("response_blob_len", types.TypeNumber)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: ln, Callee: "ts_byte_buffer_len", Args: []ir.Operand{data}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}})
			blob := g.newBlobObject(data, ln, ir.ConstString{Value: ""})
			return g.makeImmediatePromiseTask(blob, g.semaResult.BlobType, g.semaResult.BlobType), true
		}
	}
	return nil, false
}
