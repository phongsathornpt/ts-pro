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
	if f.module.source.Types[target].Kind != frontend.TypeAny || f.module.source.Types[expr.Type].Kind == frontend.TypeAny {
		return value, nil
	}
	kind := hir.BoxInvalid
	switch f.module.source.Types[expr.Type].Kind {
	case frontend.TypeNumber:
		kind = hir.BoxNumber
	case frontend.TypeString:
		kind = hir.BoxString
	default:
		return 0, fmt.Errorf("cannot box semantic type %q into any", f.module.source.Types[expr.Type].Name)
	}
	return f.emit(target, hir.BoxOp{Kind: kind, Value: value}), nil
}
