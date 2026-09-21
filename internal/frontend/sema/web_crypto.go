package sema

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (c *Checker) checkWebCryptoCall(e *ast.CallExpr, member *ast.MemberExpr) (types.Type, bool) {
	if result, handled := c.checkWebCryptoAESGCMCall(e, member); handled {
		return result, true
	}
	if result, handled := c.checkWebCryptoHKDFCall(e, member); handled {
		return result, true
	}
	if result, handled := c.checkWebCryptoHMACCall(e, member); handled {
		return result, true
	}
	if ident, ok := member.Object.(*ast.IdentExpr); ok && ident.Name == "crypto" {
		return c.checkCryptoCall(e, member)
	}
	if subtle, ok := member.Object.(*ast.MemberExpr); ok && subtle.Property == "subtle" {
		ident, ok := subtle.Object.(*ast.IdentExpr)
		if ok && ident.Name == "crypto" && member.Property == "digest" {
			return c.checkSubtleCryptoDigestCall(e, subtle, member)
		}
	}
	return nil, false
}

func (c *Checker) checkCryptoCall(e *ast.CallExpr, member *ast.MemberExpr) (types.Type, bool) {
	arrayType := c.builtinUint8ArrayType()
	getRandomValuesType := types.NewFunction([]types.Param{{Name: "array", Type: arrayType}}, arrayType)
	randomUUIDType := types.NewFunction([]types.Param{}, types.TypeString)
	cryptoType := c.builtinCryptoType()

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

func (c *Checker) checkSubtleCryptoDigestCall(e *ast.CallExpr, subtle, member *ast.MemberExpr) (types.Type, bool) {
	arrayBufferType := c.builtinArrayBufferType()
	uint8ArrayType := c.builtinUint8ArrayType()
	bufferSourceType := types.NewUnion(arrayBufferType, uint8ArrayType)
	algorithmObjectType := types.NewObject("$CryptoAlgorithm")
	algorithmObjectType.AddField("name", types.TypeString, false)
	algorithmType := types.NewUnion(types.TypeString, algorithmObjectType)
	resultType := c.newPromiseType(arrayBufferType)
	digestType := types.NewFunction([]types.Param{
		{Name: "algorithm", Type: algorithmType},
		{Name: "data", Type: bufferSourceType},
	}, resultType)

	cryptoType := c.builtinCryptoType()
	subtleType := types.NewObject("$SubtleCrypto")
	subtleType.AddField("digest", digestType, false)
	c.result.Types[subtle.Object] = cryptoType
	c.result.Types[subtle] = subtleType
	c.result.Types[member] = digestType
	c.builtinDOMExceptionType()

	if len(e.Args) != 2 {
		c.error(e.Span(), "TS2554", "crypto.subtle.digest expects exactly two arguments.")
		c.result.Types[e] = resultType
		return resultType, true
	}

	actualAlgorithmType := c.checkExpr(e.Args[0])
	if !actualAlgorithmType.AssignableTo(algorithmType) {
		c.error(e.Args[0].Span(), "TS2345", "crypto.subtle.digest algorithm must be a string or an object with a string name.")
	}
	dataType := c.checkExpr(e.Args[1])
	if !dataType.AssignableTo(bufferSourceType) {
		c.error(e.Args[1].Span(), "TS2345", "crypto.subtle.digest expects ArrayBuffer or Uint8Array data.")
	}
	c.result.Types[e] = resultType
	return resultType, true
}

func (c *Checker) builtinCryptoType() *types.ObjectType {
	arrayType := c.builtinUint8ArrayType()
	cryptoType := types.NewObject("$Crypto")
	cryptoType.AddField("getRandomValues", types.NewFunction([]types.Param{{Name: "array", Type: arrayType}}, arrayType), false)
	cryptoType.AddField("randomUUID", types.NewFunction(nil, types.TypeString), false)
	cryptoType.AddField("subtle", types.NewObject("$SubtleCrypto"), false)
	return cryptoType
}
