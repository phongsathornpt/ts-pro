package frontend

import (
	"fmt"

	"github.com/projectthorn/tsv7-bin/internal/tsast"
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
