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
	if targetKind != frontend.TypeAny && targetKind != frontend.TypeUnion {
		return value, nil
	}
	if sourceKind == frontend.TypeAny || sourceKind == frontend.TypeUnion || sourceKind == frontend.TypeNull || sourceKind == frontend.TypeUndefined {
		return value, nil
	}
	kind := hir.BoxInvalid
	switch sourceKind {
	case frontend.TypeNumber:
		kind = hir.BoxNumber
	case frontend.TypeString:
		kind = hir.BoxString
	case frontend.TypeBoolean:
		kind = hir.BoxBoolean
	case frontend.TypeObject, frontend.TypeArray:
		kind = hir.BoxObject
	case frontend.TypeFunction:
		kind = hir.BoxFunction
	default:
		return 0, fmt.Errorf("cannot box semantic type %q into dynamic value", f.module.source.Types[expr.Type].Name)
	}
	return f.emit(target, hir.BoxOp{Kind: kind, Value: value}), nil
}
