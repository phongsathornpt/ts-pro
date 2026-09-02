package frontend

import (
	"fmt"

	"github.com/phongsathornpt/ts-pro/internal/tsast"
)

func (e *extractor) extractPromiseStaticCall(node tsast.Node, expr *Expr, name string) (*Expr, error) {
	if int(expr.Type) >= len(e.result.Types) || e.result.Types[expr.Type].Kind != TypePromise {
		return nil, fmt.Errorf("%s at %d must produce Promise<T>", name, node.Pos())
	}
	promiseType := e.result.Types[expr.Type]
	if int(promiseType.ReturnType) >= len(e.result.Types) {
		return nil, fmt.Errorf("%s at %d has invalid Promise result type", name, node.Pos())
	}
	resultKind := e.result.Types[promiseType.ReturnType].Kind
	if !supportedImmediatePromiseResult(resultKind) {
		return nil, fmt.Errorf("%s at %d does not support Promise result type %q yet", name, node.Pos(), e.result.Types[promiseType.ReturnType].Name)
	}
	switch name {
	case "Promise.resolve":
		if len(expr.Args) != 1 {
			return nil, fmt.Errorf("Promise.resolve at %d currently requires exactly one value", node.Pos())
		}
		if int(expr.Args[0].Type) < len(e.result.Types) && e.result.Types[expr.Args[0].Type].Kind == TypePromise {
			input := e.result.Types[expr.Args[0].Type]
			if !e.compatibleTaskResult(input.ReturnType, promiseType.ReturnType) {
				return nil, fmt.Errorf("Promise.resolve at %d cannot adopt incompatible Promise result", node.Pos())
			}
			expr.Kind = ExprPromiseAdopt
		} else if info, ok := e.structuralThenableInfo(expr.Args[0].Type, promiseType.ReturnType); ok {
			e.ensureSemanticType(TypeAny, "any")
			expr.Kind = ExprPromiseThenable
			expr.FieldIndex = uint32(info.Arity)
			expr.Dispatch = info.Dispatch
			expr.ThenResolveJSValue = info.ResolveJSValue
			expr.ThenRejectJSValue = info.RejectJSValue
		} else {
			expr.Kind = ExprPromiseResolve
		}
	case "Promise.reject":
		if len(expr.Args) != 1 {
			return nil, fmt.Errorf("Promise.reject at %d currently requires exactly one reason", node.Pos())
		}
		e.ensureSemanticType(TypeAny, "any")
		expr.Kind = ExprPromiseReject
	case "Promise.all", "Promise.race":
		return e.extractPromiseAggregateStaticCall(node, expr, name)
	default:
		return nil, fmt.Errorf("unsupported Promise static call %q", name)
	}
	expr.Callee = nil
	return expr, nil
}

func (e *extractor) extractPromiseAggregateStaticCall(node tsast.Node, expr *Expr, name string) (*Expr, error) {
	if len(expr.Args) != 1 || expr.Args[0].Kind != ExprArray {
		return nil, fmt.Errorf("%s at %d currently requires one array literal", name, node.Pos())
	}
	input := expr.Args[0]
	if int(input.Type) >= len(e.result.Types) || e.result.Types[input.Type].Kind != TypeArray {
		return nil, fmt.Errorf("%s at %d requires an array literal", name, node.Pos())
	}
	if name == "Promise.race" && len(input.Elements) == 0 {
		return nil, fmt.Errorf("Promise.race at %d does not support an empty input until pending-forever Promise cleanup is modeled", node.Pos())
	}
	if int(expr.Type) >= len(e.result.Types) || e.result.Types[expr.Type].Kind != TypePromise {
		return nil, fmt.Errorf("%s at %d must produce Promise<T>", name, node.Pos())
	}
	outputPromise := e.result.Types[expr.Type]
	if int(outputPromise.ReturnType) >= len(e.result.Types) {
		return nil, fmt.Errorf("%s at %d has invalid output result type", name, node.Pos())
	}
	resultType := outputPromise.ReturnType
	if name == "Promise.all" {
		output := e.result.Types[outputPromise.ReturnType]
		if output.Kind != TypeArray || int(output.Element) >= len(e.result.Types) {
			return nil, fmt.Errorf("Promise.all at %d requires a homogeneous array result", node.Pos())
		}
		resultType = output.Element
		expr.Kind = ExprPromiseAll
	} else {
		expr.Kind = ExprPromiseRace
	}
	result := e.result.Types[resultType]
	switch result.Kind {
	case TypeNumber, TypeBoolean, TypeString, TypeObject, TypeArray, TypeFunction, TypeAny, TypeUnion, TypeNull, TypeUndefined:
	default:
		return nil, fmt.Errorf("%s at %d does not support Promise result type %q yet", name, node.Pos(), result.Name)
	}
	for _, item := range input.Elements {
		if int(item.Type) >= len(e.result.Types) {
			return nil, fmt.Errorf("%s at %d has invalid input type", name, node.Pos())
		}
		itemType := e.result.Types[item.Type]
		if itemType.Kind == TypePromise {
			if !e.compatibleTaskResult(itemType.ReturnType, resultType) {
				return nil, fmt.Errorf("%s at %d has incompatible Promise input", name, node.Pos())
			}
			continue
		}
		if !e.compatibleArrayElement(resultType, item.Type) {
			return nil, fmt.Errorf("%s at %d has raw input incompatible with %s", name, node.Pos(), result.Name)
		}
	}
	expr.Args = append(expr.Args[:0], input.Elements...)
	expr.Callee = nil
	return expr, nil
}

func supportedImmediatePromiseResult(kind TypeKind) bool {
	switch kind {
	case TypeNumber, TypeBoolean, TypeString, TypeObject, TypeArray, TypeFunction, TypeAny, TypeUnion, TypeNull, TypeUndefined:
		return true
	default:
		return false
	}
}

type thenableInfo struct {
	Arity          int
	Dispatch       []DispatchTarget
	ResolveJSValue bool
	RejectJSValue  bool
}

func (e *extractor) structuralThenableInfo(typeID, resultType TypeID) (thenableInfo, bool) {
	if int(typeID) >= len(e.result.Types) {
		return thenableInfo{}, false
	}
	if class, ok := e.thenableClass(typeID); ok {
		targets := e.dispatchTargets(class, "then")
		if len(targets) == 0 {
			return thenableInfo{}, false
		}
		var merged thenableInfo
		for _, target := range targets {
			if int(target.Function) >= len(e.result.Functions) {
				return thenableInfo{}, false
			}
			fn := e.result.Functions[target.Function]
			if len(fn.Params) < 2 {
				return thenableInfo{}, false
			}
			params := make([]TypeID, 0, len(fn.Params)-1)
			for _, param := range fn.Params[1:] {
				params = append(params, param.Type)
			}
			candidate, ok := e.thenableCallbackInfo(params, resultType)
			if !ok {
				return thenableInfo{}, false
			}
			if merged.Arity != 0 && (merged.Arity != candidate.Arity || merged.ResolveJSValue != candidate.ResolveJSValue || merged.RejectJSValue != candidate.RejectJSValue) {
				return thenableInfo{}, false
			}
			merged = candidate
		}
		merged.Dispatch = targets
		return merged, true
	}
	typ := e.result.Types[typeID]
	if typ.Kind != TypeObject || int(typ.Shape) >= len(e.result.Shapes) {
		return thenableInfo{}, false
	}
	for _, field := range e.result.Shapes[typ.Shape].Fields {
		if field.Name != "then" || int(field.Type) >= len(e.result.Types) {
			continue
		}
		thenType := e.result.Types[field.Type]
		if thenType.Kind != TypeFunction {
			return thenableInfo{}, false
		}
		return e.thenableCallbackInfo(thenType.Params, resultType)
	}
	return thenableInfo{}, false
}

func (e *extractor) thenableClass(typeID TypeID) (*classInfo, bool) {
	if class, ok := e.classesByType[typeID]; ok {
		return class, true
	}
	if int(typeID) >= len(e.result.Types) {
		return nil, false
	}
	typ := e.result.Types[typeID]
	if typ.Kind != TypeObject || int(typ.Shape) >= len(e.result.Shapes) || e.result.Shapes[typ.Shape].ClassTag == 0 {
		return nil, false
	}
	seen := map[*classInfo]struct{}{}
	for _, class := range e.classesByType {
		if _, duplicate := seen[class]; duplicate {
			continue
		}
		seen[class] = struct{}{}
		if class.Shape == typ.Shape {
			return class, true
		}
	}
	return nil, false
}

func (e *extractor) thenableCallbackInfo(params []TypeID, resultType TypeID) (thenableInfo, bool) {
	if len(params) < 1 || len(params) > 2 {
		return thenableInfo{}, false
	}
	resolveType, ok := e.thenableCallbackFunction(params[0])
	if !ok || len(resolveType.Params) != 1 || !e.compatibleArrayElement(resultType, resolveType.Params[0]) {
		return thenableInfo{}, false
	}
	resolveJSValue, ok := e.thenableCallbackReturn(resolveType.ReturnType)
	if !ok {
		return thenableInfo{}, false
	}
	info := thenableInfo{Arity: len(params), ResolveJSValue: resolveJSValue}
	if len(params) == 2 {
		rejectType, ok := e.thenableCallbackFunction(params[1])
		if !ok || len(rejectType.Params) != 1 || int(rejectType.Params[0]) >= len(e.result.Types) {
			return thenableInfo{}, false
		}
		reasonKind := e.result.Types[rejectType.Params[0]].Kind
		if reasonKind != TypeAny && reasonKind != TypeUnion && reasonKind != TypeUnknown {
			return thenableInfo{}, false
		}
		rejectJSValue, ok := e.thenableCallbackReturn(rejectType.ReturnType)
		if !ok {
			return thenableInfo{}, false
		}
		info.RejectJSValue = rejectJSValue
	}
	return info, true
}

func (e *extractor) thenableCallbackReturn(typeID TypeID) (bool, bool) {
	if int(typeID) >= len(e.result.Types) {
		return false, false
	}
	switch e.result.Types[typeID].Kind {
	case TypeVoid:
		return false, true
	case TypeAny, TypeUnion, TypeParameter, TypeNull, TypeUndefined:
		return true, true
	default:
		return false, false
	}
}

func (e *extractor) thenableCallbackFunction(typeID TypeID) (Type, bool) {
	if int(typeID) >= len(e.result.Types) {
		return Type{}, false
	}
	typ := e.result.Types[typeID]
	if typ.Kind == TypeFunction {
		return typ, true
	}
	if typ.Kind != TypeUnion {
		return Type{}, false
	}
	var callback *Type
	for _, memberID := range typ.Members {
		if int(memberID) >= len(e.result.Types) {
			return Type{}, false
		}
		member := e.result.Types[memberID]
		switch member.Kind {
		case TypeNull, TypeUndefined:
			continue
		case TypeFunction:
			if callback != nil {
				return Type{}, false
			}
			copy := member
			callback = &copy
		default:
			return Type{}, false
		}
	}
	if callback == nil {
		return Type{}, false
	}
	return *callback, true
}
