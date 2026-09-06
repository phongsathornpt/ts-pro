package sema

import (
	"fmt"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (c *Checker) checkMemberExpr(e *ast.MemberExpr) types.Type {
	if ident, ok := e.Object.(*ast.IdentExpr); ok {
		if members := c.result.Enums[ident.Name]; members != nil {
			if _, exists := members[e.Property]; !exists {
				c.error(e.Span(), "TS2339", fmt.Sprintf("Enum '%s' has no member '%s'.", ident.Name, e.Property))
			}
			c.result.Types[ident] = types.TypeNumber
			c.result.Types[e] = types.TypeNumber
			return types.TypeNumber
		}
	}
	objType := c.checkExpr(e.Object)
	lookupType := objType
	if e.Optional {
		lookupType = removeNullishType(objType)
	}
	memberType, ok := c.lookupMemberType(lookupType, e.Property)
	if !ok {
		c.error(e.Span(), "TS2339", fmt.Sprintf("Property '%s' does not exist on type '%s'.", e.Property, objType))
		c.result.Types[e] = types.TypeAny
		return types.TypeAny
	}
	if e.Optional {
		memberType = types.NewUnion(memberType, types.TypeUndefined)
	}
	c.result.Types[e] = memberType
	return memberType
}
