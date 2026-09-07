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
	return g.newResponseObject(data, hasBody, headers, status, statusText, ir.ConstString{Value: "default"}, ir.ConstString{Value: ""}, ir.ConstBool{Value: false})
}

func (g *generator) ensureResponseBodyUnused(res ir.Operand) {
	used := g.responseField(res, "bodyUsed", types.TypeBoolean)
	fail := g.currentFn.NewBlock("response_body_used_error")
	ok := g.currentFn.NewBlock("response_body_unused")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: used, Then: fail, Else: ok}
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
