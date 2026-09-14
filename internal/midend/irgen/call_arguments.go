package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
)

// lowerCallArgument materializes JavaScript undefined when a void
// expression is used in argument value position. Internally, void
// lowering returns nil after emitting its side effects; nil must
// never escape into IR call operands.
func (g *generator) lowerCallArgument(expr ast.Expr) ir.Operand {
	value := g.lowerExpr(expr)
	if value == nil {
		return ir.ConstUndefined{}
	}
	return value
}
