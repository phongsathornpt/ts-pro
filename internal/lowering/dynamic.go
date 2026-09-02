package lowering

import (
	"fmt"

	"github.com/projectthorn/tsv7-bin/internal/frontend"
	"github.com/projectthorn/tsv7-bin/internal/hir"
)

func (f *functionLowerer) lowerExprAs(expr *frontend.Expr, target frontend.TypeID) (hir.ValueID, error) {
	value, err := f.lowerExpr(expr)
	if err != nil {
		return 0, err
	}
	if expr == nil || int(target) >= len(f.module.source.Types) || int(expr.Type) >= len(f.module.source.Types) {
		return value, nil
	}
	targetKind := f.module.source.Types[target].Kind
	sourceKind := f.module.source.Types[expr.Type].Kind
	targetDynamic := targetKind == frontend.TypeAny || targetKind == frontend.TypeUnion
	sourceDynamic := sourceKind == frontend.TypeAny || sourceKind == frontend.TypeUnion

	if targetDynamic {
		if expr.Kind == frontend.ExprClosure {
			if expr.CallTarget == nil {
				return 0, fmt.Errorf("cannot box closure without a concrete native target")
			}
			return f.emit(target, hir.BoxOp{
				Kind: hir.BoxFunction, Value: value, Function: hir.NewFunctionID(uint32(*expr.CallTarget)), HasFunction: true,
			}), nil
		}
		if sourceDynamic || sourceKind == frontend.TypeNull || sourceKind == frontend.TypeUndefined {
			return value, nil
		}
		kind := hir.BoxInvalid
		shape := hir.ShapeID(0)
		switch sourceKind {
		case frontend.TypeNumber:
			kind = hir.BoxNumber
		case frontend.TypeString:
			kind = hir.BoxString
		case frontend.TypeBoolean:
			kind = hir.BoxBoolean
		case frontend.TypeObject:
			kind = hir.BoxObject
			shape = hir.NewShapeID(uint32(f.module.source.Types[expr.Type].Shape))
		case frontend.TypeArray:
			kind = hir.BoxArray
		case frontend.TypeFunction:
			kind = hir.BoxFunction
		default:
			return 0, fmt.Errorf("cannot box semantic type %q into dynamic value", f.module.source.Types[expr.Type].Name)
		}
		box := hir.BoxOp{Kind: kind, Value: value, Shape: shape}
		if kind == hir.BoxFunction {
			if expr.CallTarget == nil {
				return 0, fmt.Errorf("cannot box function value without a concrete native closure target")
			}
			box.Function = hir.NewFunctionID(uint32(*expr.CallTarget))
			box.HasFunction = true
		}
		return f.emit(target, box), nil
	}

	if !sourceDynamic {
		return value, nil
	}
	kind := hir.UnboxInvalid
	switch targetKind {
	case frontend.TypeNumber:
		kind = hir.UnboxNumber
	case frontend.TypeString:
		kind = hir.UnboxString
	case frontend.TypeBoolean:
		kind = hir.UnboxBoolean
	case frontend.TypeArray:
		kind = hir.UnboxArray
	default:
		return 0, fmt.Errorf("cannot safely unbox dynamic value into semantic type %q without runtime representation metadata", f.module.source.Types[target].Name)
	}
	return f.emit(target, hir.UnboxOp{Kind: kind, Value: value}), nil
}
