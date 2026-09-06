package irgen

import (
	"strings"
	"time"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) lowerNewExpr(e *ast.NewExpr) ir.Operand {
	if e.ClassName == "URL" {
		var baseExpr ast.Expr
		if len(e.Args) > 1 {
			baseExpr = e.Args[1]
		}
		invalidBB := g.currentFn.NewBlock("url_new_invalid")
		res := g.lowerURLResolveAndParse(e.Args[0], baseExpr, invalidBB)
		resultBB := g.currentBB
		g.currentBB = invalidBB
		err := g.newWebError(ir.ConstString{Value: "Invalid URL"}, ir.ConstString{Value: "TypeError"})
		g.routeThrownValue(err)
		g.currentBB = resultBB
		return res
	}
	if e.ClassName == "ArrayBuffer" {
		length := g.lowerExpr(e.Args[0])
		data := g.currentFn.NewValue("array_buffer_data", g.semaResult.ByteBufferType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: data, Callee: "ts_byte_buffer_new", Args: []ir.Operand{length}, ParamTypes: []types.Type{types.TypeNumber}})
		return g.newArrayBufferFromData(data)
	}
	if e.ClassName == "Uint8Array" {
		argType := g.semanticType(e.Args[0])
		if obj, ok := argType.(*types.ObjectType); ok && obj.Name == "$ArrayBuffer" {
			buffer := g.lowerExpr(e.Args[0])
			data := g.arrayBufferData(buffer)
			length := g.currentFn.NewValue("uint8_len", types.TypeNumber)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: length, Callee: "ts_byte_buffer_len", Args: []ir.Operand{data}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}})
			return g.newUint8ArrayView(data, buffer, ir.ConstNumber{Value: 0}, length)
		}
		length := g.lowerExpr(e.Args[0])
		data := g.currentFn.NewValue("uint8_data", g.semaResult.ByteBufferType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: data, Callee: "ts_byte_buffer_new", Args: []ir.Operand{length}, ParamTypes: []types.Type{types.TypeNumber}})
		buffer := g.newArrayBufferFromData(data)
		return g.newUint8ArrayView(data, buffer, ir.ConstNumber{Value: 0}, length)
	}
	if e.ClassName == "URLSearchParams" {
		var init ir.Operand
		if len(e.Args) == 1 {
			init = g.lowerExpr(e.Args[0])
		}
		return g.lowerURLSearchParamsNew(init)
	}
	if e.ClassName == "TextEncoder" {
		t := g.semaResult.TextEncoderType
		res := g.currentFn.NewValue("text_encoder", t)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{Res: res, Shape: "$TextEncoder", FieldCount: 0, RefMask: 0})
		return res
	}
	if e.ClassName == "TextDecoder" {
		label := ir.Operand(ir.ConstString{Value: "utf-8"})
		if len(e.Args) > 0 {
			label = g.lowerExpr(e.Args[0])
		}
		var matches []ir.Operand
		for _, canonical := range []string{"utf-8", "utf8", "unicode-1-1-utf-8"} {
			match := g.currentFn.NewValue("encoding_label_match", types.TypeBoolean)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: match, Callee: "ts_encoding_label_eq", Args: []ir.Operand{label, ir.ConstString{Value: canonical}}, ParamTypes: []types.Type{types.TypeString, types.TypeString}})
			matches = append(matches, match)
		}
		valid := matches[0]
		for _, match := range matches[1:] {
			combined := g.currentFn.NewValue("encoding_label_valid", types.TypeBoolean)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: combined, Op: ir.OpOr, LHS: valid, RHS: match})
			valid = combined
		}
		okBB := g.currentFn.NewBlock("text_decoder_label_ok")
		errBB := g.currentFn.NewBlock("text_decoder_label_invalid")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: valid, Then: okBB, Else: errBB}
		g.currentBB = errBB
		err := g.newWebError(ir.ConstString{Value: "The encoding label provided is invalid."}, ir.ConstString{Value: "RangeError"})
		g.routeThrownValue(err)
		g.currentBB = okBB

		t := g.semaResult.TextDecoderType
		offsets, refMask, shape := g.objectLayout(t)
		res := g.currentFn.NewValue("text_decoder", t)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{Res: res, Shape: shape, FieldCount: len(offsets), RefMask: refMask})
		var optExpr ast.Expr
		var optValue ir.Operand
		if len(e.Args) > 1 {
			optExpr = e.Args[1]
			optValue = g.lowerExpr(optExpr)
		}
		fatal := g.lowerWebIDLDictionaryBool(optExpr, optValue, "fatal", false)
		ignoreBOM := g.lowerWebIDLDictionaryBool(optExpr, optValue, "ignoreBOM", false)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.SetFieldInst{Obj: res, Field: "fatal", Offset: offsets["fatal"], Val: fatal},
			&ir.SetFieldInst{Obj: res, Field: "ignoreBOM", Offset: offsets["ignoreBOM"], Val: ignoreBOM},
		)
		return res
	}
	if e.ClassName == "AbortController" {
		return g.lowerAbortControllerNew()
	}
	if e.ClassName == "EventTarget" {
		t := g.semaResult.EventTargetType
		res := g.currentFn.NewValue("event_target", t)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_event_target_new"})
		return res
	}
	if e.ClassName == "Event" {
		return g.lowerEventConstructor(e, g.semaResult.EventType)
	}
	if e.ClassName == "CustomEvent" {
		return g.lowerEventConstructor(e, g.semaResult.CustomEventType)
	}
	if e.ClassName == "MessageEvent" {
		return g.lowerEventConstructor(e, g.semaResult.MessageEventType)
	}
	if e.ClassName == "ErrorEvent" {
		return g.lowerEventConstructor(e, g.semaResult.ErrorEventType)
	}
	if e.ClassName == "DOMException" {
		message := ir.Operand(ir.ConstString{Value: ""})
		name := ir.Operand(ir.ConstString{Value: "Error"})
		if len(e.Args) > 0 {
			message = g.lowerExpr(e.Args[0])
		}
		if len(e.Args) > 1 {
			name = g.lowerExpr(e.Args[1])
		}
		return g.newDOMException(message, name)
	}
	if e.ClassName == "RegExp" {
		pattern := e.Args[0].(*ast.StringLit)
		flags := ""
		if len(e.Args) == 2 {
			flags = e.Args[1].(*ast.StringLit).Value
		}
		return g.lowerNativeRegExp(pattern.Value, flags, g.semanticType(e))
	}
	if e.ClassName == "Date" {
		arg := e.Args[0]
		value := g.lowerExpr(arg)
		if lit, ok := arg.(*ast.StringLit); ok {
			parsed, _ := time.Parse(time.RFC3339Nano, lit.Value)
			value = ir.ConstNumber{Value: float64(parsed.UnixMilli())}
		}
		res := g.currentFn.NewValue("date", g.semanticType(e))
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_date_from_number", Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeNumber}})
		return res
	}
	if collection := g.builtinCollectionInfo(g.semanticType(e)); collection != nil {
		res := g.currentFn.NewValue(strings.ToLower(collection.Kind), collection.Instance)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_collection_new"})
		return res
	}
	info := g.semaResult.GenericClasses[e]
	if info == nil {
		info = g.semaResult.Classes[e.ClassName]
	}
	g.ensureClassSpecialization(info)
	offsets, refMask, shape := g.objectLayout(info.Instance)
	obj := g.currentFn.NewValue("instance", info.Instance)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
		Res: obj, Shape: shape, FieldCount: len(offsets) + 1, RefMask: refMask,
	})
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: obj, Field: "$class", Offset: 16, Val: ir.ConstNumber{Value: float64(g.classTag(info.Name))}})
	args := make([]ir.Operand, 0, len(e.Args)+1)
	args = append(args, obj)
	for _, arg := range e.Args {
		args = append(args, g.lowerExpr(arg))
	}
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Callee: classConstructorName(info.Name), Args: args,
	})
	return obj
}
