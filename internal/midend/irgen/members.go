package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) lowerMemberExpr(e *ast.MemberExpr) ir.Operand {
	if e.Optional {
		return g.lowerOptionalMember(e)
	}
	if ident, ok := e.Object.(*ast.IdentExpr); ok {
		if ident.Name == "performance" && e.Property == "timeOrigin" {
			res := g.currentFn.NewValue("performance_time_origin", types.TypeNumber)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_performance_time_origin"})
			return res
		}
		if ident.Name == "navigator" && e.Property == "userAgent" {
			return ir.ConstString{Value: "ts-pro"}
		}
		if members := g.semaResult.Enums[ident.Name]; members != nil {
			return ir.ConstNumber{Value: members[e.Property]}
		}
	}
	if tuple, ok := g.semanticType(e.Object).(*types.TupleType); ok && e.Property == "length" {
		return ir.ConstNumber{Value: float64(len(tuple.Elements))}
	}
	if _, ok := g.semanticType(e.Object).(*types.ArrayType); ok && e.Property == "length" {
		array := g.lowerExpr(e.Object)
		res := g.currentFn.NewValue("len", types.TypeNumber)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayLengthInst{Res: res, Array: array})
		return res
	}
	if objType, ok := g.semanticType(e.Object).(*types.ObjectType); ok {
		if objType.Name == "$ArrayBuffer" && e.Property == "byteLength" {
			obj := g.lowerExpr(e.Object)
			data := g.arrayBufferData(obj)
			res := g.currentFn.NewValue("array_buffer_len", types.TypeNumber)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_byte_buffer_len", Args: []ir.Operand{data}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}})
			return res
		}
		if objType.Name == "$AbortSignal" {
			signal := g.lowerExpr(e.Object)
			switch e.Property {
			case "aborted":
				res := g.currentFn.NewValue("abort_signal_aborted", types.TypeBoolean)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_abort_signal_aborted", Args: []ir.Operand{signal}})
				return res
			case "reason":
				res := g.currentFn.NewValue("abort_signal_reason", types.TypeAny)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_abort_signal_reason", Args: []ir.Operand{signal}})
				return res
			case "onabort":
				fnType := types.NewFunction([]types.Param{{Name: "event", Type: g.semaResult.EventType}}, types.TypeVoid)
				raw := g.currentFn.NewValue("abort_signal_onabort", fnType)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: raw, Callee: "ts_abort_signal_onabort", Args: []ir.Operand{signal}})
				return g.boxJSValue(raw, fnType)
			}
		}
		if isEventObjectType(objType) {
			if physical, ok := eventPhysicalProperty(objType, e.Property); ok {
				obj := g.lowerExpr(e.Object)
				return g.eventField(obj, physical, g.semanticType(e))
			}
		}
		if objType.Name == "$DOMException" {
			raw := g.lowerExpr(e.Object)
			boxed := g.boxJSValue(raw, objType)
			value := g.lowerDynamicGet(boxed, e.Property)
			if field, ok := objType.Fields[e.Property]; ok {
				return g.coerceJSValueBoundary(value, types.TypeAny, field.Type)
			}
			return value
		}
		if objType.Name == "$RegExp" && e.Property == "source" {
			obj := g.lowerExpr(e.Object)
			res := g.currentFn.NewValue("regexp_source", types.TypeString)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: res, Obj: obj, Field: "source", Offset: 16})
			return res
		}
		if collection := g.semaResult.BuiltinCollections[objType.Name]; collection != nil && e.Property == "size" {
			obj := g.lowerExpr(e.Object)
			res := g.currentFn.NewValue("collection_size", types.TypeNumber)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_collection_size", Args: []ir.Operand{obj}})
			return res
		}
		offsets, _, _ := g.objectLayout(objType)
		offset := offsets[e.Property]
		obj := g.lowerExpr(e.Object)
		resultType := types.TypeAny
		if t := g.semanticType(e); t != nil {
			resultType = t
		}
		res := g.currentFn.NewValue("field", resultType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: res, Obj: obj, Field: e.Property, Offset: offset})
		return res
	}
	if _, ok := g.semanticType(e.Object).(*types.UnionType); ok {
		obj := g.lowerExpr(e.Object)
		if concrete, ok := obj.Type().(*types.ObjectType); ok {
			offsets, _, _ := g.objectLayout(concrete)
			if offset, exists := offsets[e.Property]; exists {
				resultType := types.TypeAny
				if t := g.semanticType(e); t != nil {
					resultType = t
				}
				res := g.currentFn.NewValue("union_field", resultType)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: res, Obj: obj, Field: e.Property, Offset: offset})
				return res
			}
		}
	}
	obj := g.lowerExpr(e.Object)
	if concrete, ok := obj.Type().(*types.ObjectType); ok {
		offsets, _, _ := g.objectLayout(concrete)
		field := concrete.Fields[e.Property]
		res := g.currentFn.NewValue("any_typed_field", field.Type)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: res, Obj: obj, Field: e.Property, Offset: offsets[e.Property]})
		return res
	}
	if concrete, ok := g.provenObjectType(e.Object); ok {
		offsets, _, _ := g.objectLayout(concrete)
		if offset, exists := offsets[e.Property]; exists {
			field := concrete.Fields[e.Property]
			raw := g.unboxKnownObject(obj, concrete)
			res := g.currentFn.NewValue("any_field", field.Type)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: res, Obj: raw, Field: e.Property, Offset: offset})
			return res
		}
		return ir.ConstUndefined{}
	}
	return g.lowerDynamicGet(obj, e.Property)
}
