package irgen

import (
	"strings"
	"time"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) lowerNewExpr(e *ast.NewExpr) ir.Operand {
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
