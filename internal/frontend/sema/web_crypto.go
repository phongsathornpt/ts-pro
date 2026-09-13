package sema

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (c *Checker) checkWebCryptoCall(e *ast.CallExpr, member *ast.MemberExpr) (types.Type, bool) {
	ident, ok := member.Object.(*ast.IdentExpr)
	if !ok || ident.Name != "crypto" || member.Property != "getRandomValues" {
		return nil, false
	}

	arrayType := c.builtinUint8ArrayType()
	fnType := types.NewFunction([]types.Param{{Name: "array", Type: arrayType}}, arrayType)
	cryptoType := types.NewObject("$Crypto")
	cryptoType.AddField("getRandomValues", fnType, false)

	c.result.Types[member.Object] = cryptoType
	c.result.Types[member] = fnType
	c.builtinDOMExceptionType()

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
}
