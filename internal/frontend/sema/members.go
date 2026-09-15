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
	if obj, ok := objType.(*types.ObjectType); ok && obj.Name == "$GlobalScope" {
		switch e.Property {
		case "onerror":
			// The global object is dynamic-backed at runtime. Mark this member's
			// receiver occurrence as a JS-value boundary so reads and writes lower
			// through dynamic get/set instead of inventing physical object fields.
			c.result.Types[e.Object] = types.TypeAny
			handler := types.NewFunction([]types.Param{
				{Name: "message", Type: types.TypeString},
				{Name: "source", Type: types.TypeString},
				{Name: "lineno", Type: types.TypeNumber},
				{Name: "colno", Type: types.TypeNumber},
				{Name: "error", Type: types.TypeAny},
			}, types.TypeAny)
			memberType := types.NewUnion(handler, types.TypeNull)
			c.result.Types[e] = memberType
			return memberType
		case "onunhandledrejection", "onrejectionhandled":
			c.result.Types[e.Object] = types.TypeAny
			c.result.Types[e] = types.TypeAny
			return types.TypeAny
		}
		c.builtinEventTargetType()
		if member, ok := c.builtinEventTargetMember(e.Property); ok {
			c.result.Types[e] = member
			return member
		}
	}
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
