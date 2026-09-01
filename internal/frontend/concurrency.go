package frontend

import (
	"fmt"

	"github.com/projectthorn/tsv7-bin/internal/tsast"
)

func (e *extractor) extractConcurrencyCall(node tsast.Node, expr *Expr, name string) (*Expr, error) {
	switch name {
	case "spawn":
		if len(expr.Args) != 1 {
			return nil, fmt.Errorf("spawn at %d requires exactly one function", node.Pos())
		}
		closure := expr.Args[0]
		if closure.Kind != ExprClosure || closure.CallTarget == nil {
			return nil, fmt.Errorf("spawn at %d requires a statically known closure", node.Pos())
		}
		if int(*closure.CallTarget) >= len(e.result.Functions) {
			return nil, fmt.Errorf("spawn at %d references invalid function", node.Pos())
		}
		target := e.result.Functions[*closure.CallTarget]
		if len(target.Params) != len(closure.Captures) || int(target.ReturnType) >= len(e.result.Types) || e.result.Types[target.ReturnType].Kind != TypeVoid {
			return nil, fmt.Errorf("spawn at %d currently requires a zero-argument closure returning void", node.Pos())
		}
		targetCopy := *closure.CallTarget
		expr.Kind = ExprTaskSpawn
		expr.CallTarget = &targetCopy
		expr.Captures = closure.Captures
		expr.Callee = nil
		expr.Args = nil
		return expr, nil
	case "join":
		if len(expr.Args) != 1 || int(expr.Args[0].Type) >= len(e.result.Types) || e.result.Types[expr.Args[0].Type].Kind != TypeTask {
			return nil, fmt.Errorf("join at %d requires exactly one TsnativeTask", node.Pos())
		}
		expr.Kind = ExprTaskJoin
		expr.Callee = nil
		return expr, nil
	case "yieldNow":
		if len(expr.Args) != 0 {
			return nil, fmt.Errorf("yieldNow at %d takes no arguments", node.Pos())
		}
		expr.Kind = ExprTaskYield
		expr.Callee = nil
		return expr, nil
	default:
		return nil, fmt.Errorf("unknown concurrency intrinsic %q", name)
	}
}
