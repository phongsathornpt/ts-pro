package frontend

import (
	"fmt"

	"github.com/projectthorn/tsv7-bin/internal/tsast"
)

func (e *extractor) compatibleArrayElement(expected, actual TypeID) bool {
	if expected == actual {
		return true
	}
	if int(expected) >= len(e.result.Types) || int(actual) >= len(e.result.Types) {
		return false
	}
	want, got := e.result.Types[expected], e.result.Types[actual]
	if want.Kind == TypeAny {
		switch got.Kind {
		case TypeNumber, TypeString, TypeBoolean, TypeObject, TypeArray, TypeFunction, TypeAny, TypeUnion, TypeNull, TypeUndefined:
			return true
		default:
			return false
		}
	}
	if want.Kind == TypeUnion {
		for _, member := range want.Members {
			if e.compatibleArrayElement(member, actual) {
				return true
			}
		}
		return false
	}
	if want.Kind != got.Kind {
		return false
	}
	switch want.Kind {
	case TypeNumber, TypeString, TypeBoolean, TypeNull, TypeUndefined:
		return true
	case TypeObject:
		if int(want.Shape) >= len(e.result.Shapes) || int(got.Shape) >= len(e.result.Shapes) {
			return false
		}
		a, b := e.result.Shapes[want.Shape], e.result.Shapes[got.Shape]
		if len(a.Fields) != len(b.Fields) {
			return false
		}
		for i := range a.Fields {
			if a.Fields[i].Name != b.Fields[i].Name || !e.compatibleArrayElement(a.Fields[i].Type, b.Fields[i].Type) {
				return false
			}
		}
		return true
	case TypeArray:
		return e.compatibleArrayElement(want.Element, got.Element)
	case TypeFunction:
		if len(want.Params) != len(got.Params) {
			return false
		}
		for i := range want.Params {
			if !e.compatibleArrayElement(want.Params[i], got.Params[i]) {
				return false
			}
		}
		return e.compatibleArrayElement(want.ReturnType, got.ReturnType)
	default:
		return false
	}
}

func (e *extractor) buildArrayAssignment(node, target, rhs tsast.Node) (Statement, bool, error) {
	arrayNode, ok := target.NamedChild("expression")
	if !ok {
		return Statement{}, true, fmt.Errorf("array assignment at %d has no receiver", target.Pos())
	}
	indexNode, ok := target.NamedChild("argumentExpression")
	if !ok {
		return Statement{}, true, fmt.Errorf("array assignment at %d has no index", target.Pos())
	}
	array, err := e.extractExpr(arrayNode)
	if err != nil {
		return Statement{}, true, err
	}
	index, err := e.extractExpr(indexNode)
	if err != nil {
		return Statement{}, true, err
	}
	value, err := e.extractExpr(rhs)
	if err != nil {
		return Statement{}, true, err
	}
	if int(array.Type) >= len(e.result.Types) {
		return Statement{}, true, fmt.Errorf("array assignment has invalid receiver type t%d", array.Type)
	}
	arrayType := e.result.Types[array.Type]
	if arrayType.Kind != TypeArray || int(arrayType.Element) >= len(e.result.Types) {
		return Statement{}, true, fmt.Errorf("native indexed assignment requires a concrete array type")
	}
	elementKind := e.result.Types[arrayType.Element].Kind
	switch elementKind {
	case TypeNumber, TypeString, TypeObject, TypeArray, TypeFunction, TypeAny, TypeUnion:
	default:
		return Statement{}, true, fmt.Errorf("native indexed assignment does not support %s[] yet", e.result.Types[arrayType.Element].Name)
	}
	if int(index.Type) >= len(e.result.Types) || e.result.Types[index.Type].Kind != TypeNumber {
		return Statement{}, true, fmt.Errorf("native array index must be number")
	}
	if int(value.Type) >= len(e.result.Types) || !e.compatibleArrayElement(arrayType.Element, value.Type) {
		return Statement{}, true, fmt.Errorf("native array assignment requires %s value", e.result.Types[arrayType.Element].Name)
	}
	return Statement{
		Kind: StmtArrayAssign, Span: e.span(node), Type: arrayType.Element,
		Object: array, Index: index, Value: value,
	}, true, nil
}
