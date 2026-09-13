package sema

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (c *Checker) checkWebCryptoCall(e *ast.CallExpr, member *ast.MemberExpr) (types.Type, bool) {
	ident, ok := member.Object.(*ast.IdentExpr)
	if !ok || ident.Name != "crypto" {
		return nil, false
	}

	arrayType := c.builtinUint8ArrayType()
	getRandomValuesType := types.NewFunction([]types.Param{{Name: "array", Type: arrayType}}, arrayType)
	randomUUIDType := types.NewFunction([]types.Param{}, types.TypeString)
	cryptoType := types.NewObject("$Crypto")
	cryptoType.AddField("getRandomValues", getRandomValuesType, false)
	cryptoType.AddField("randomUUID", randomUUIDType, false)

	var memberType *types.FunctionType
	switch member.Property {
	case "getRandomValues":
		memberType = getRandomValuesType
	case "randomUUID":
		memberType = randomUUIDType
	default:
		return nil, false
	}

	c.result.Types[member.Object] = cryptoType
	c.result.Types[member] = memberType
	c.builtinDOMExceptionType()

	switch member.Property {
	case "getRandomValues":
		if len(e.Args) != 1 {
			c.error(e.Span(), "TS2554", "crypto.getRandomValues expects exactly one Uint8Array.")
			c.result.Types[e] = arrayType
			return arrayType, true
		}

		argType := c.checkExpr(e.Args[0])
		if !argType.AssignableTo(arrayType) {
			c.error(e.Args[0].Span(), "TS2345", "crypto.getRandomValues expects a Uint8Array.")
		}
		c.result.Types[e] = arrayType
		return arrayType, true
	case "randomUUID":
		if len(e.Args) != 0 {
			c.error(e.Span(), "TS2554", "crypto.randomUUID expects no arguments.")
		}
		c.result.Types[e] = types.TypeString
		return types.TypeString, true
	}

	return nil, false
}
