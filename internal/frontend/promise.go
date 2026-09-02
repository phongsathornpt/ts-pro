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
			return nil, fmt.Errorf("Promise.resolve at %d does not adopt an existing Promise yet", node.Pos())
		}
		expr.Kind = ExprPromiseResolve
	case "Promise.reject":
		if len(expr.Args) != 1 {
			return nil, fmt.Errorf("Promise.reject at %d currently requires exactly one reason", node.Pos())
		}
		e.ensureSemanticType(TypeAny, "any")
		expr.Kind = ExprPromiseReject
	default:
		return nil, fmt.Errorf("unsupported Promise static call %q", name)
	}
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
