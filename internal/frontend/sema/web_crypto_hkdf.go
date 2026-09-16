package sema

import (
	"fmt"
	"strings"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (c *Checker) checkWebCryptoHKDFCall(e *ast.CallExpr, member *ast.MemberExpr) (types.Type, bool) {
	subtle, ok := member.Object.(*ast.MemberExpr)
	if !ok || subtle.Property != "subtle" {
		return nil, false
	}
	ident, ok := subtle.Object.(*ast.IdentExpr)
	if !ok || ident.Name != "crypto" {
		return nil, false
	}

	keyType := c.hkdfCryptoKeyType()
	var fnType *types.FunctionType
	var resultType types.Type

	switch member.Property {
	case "importKey":
		if len(e.Args) < 3 || !isHKDFAlgorithmExpr(e.Args[2]) {
			return nil, false
		}
		resultType = c.newPromiseType(keyType)
		fnType = types.NewFunction([]types.Param{
			{Name: "format", Type: types.TypeString},
			{Name: "keyData", Type: c.cryptoBufferSourceType()},
			{Name: "algorithm", Type: c.cryptoAlgorithmIdentifierType()},
			{Name: "extractable", Type: types.TypeBoolean},
			{Name: "keyUsages", Type: types.NewArray(types.TypeString)},
		}, resultType)
	case "deriveBits":
		resultType = c.newPromiseType(c.builtinArrayBufferType())
		fnType = types.NewFunction([]types.Param{
			{Name: "algorithm", Type: c.hkdfParamsType()},
			{Name: "baseKey", Type: keyType},
			{Name: "length", Type: types.TypeNumber},
		}, resultType)
	default:
		return nil, false
	}

	c.result.Types[subtle.Object] = c.builtinCryptoType()
	subtleType := types.NewObject("$SubtleCrypto")
	subtleType.AddField(member.Property, fnType, false)
	c.result.Types[subtle] = subtleType
	c.result.Types[member] = fnType
	c.builtinDOMExceptionType()

	if len(e.Args) != len(fnType.Params) {
		c.error(e.Span(), "TS2554", fmt.Sprintf("crypto.subtle.%s expects exactly %d arguments.", member.Property, len(fnType.Params)))
		c.result.Types[e] = resultType
		return resultType, true
	}
	for i, param := range fnType.Params {
		actual := c.checkExpr(e.Args[i])
		if !actual.AssignableTo(param.Type) {
			c.error(e.Args[i].Span(), "TS2345", fmt.Sprintf("crypto.subtle.%s argument %q has type %s; expected %s.", member.Property, param.Name, actual.String(), param.Type.String()))
		}
	}
	c.result.Types[e] = resultType
	return resultType, true
}

func isHKDFAlgorithmExpr(expr ast.Expr) bool {
	switch value := expr.(type) {
	case *ast.StringLit:
		return strings.EqualFold(value.Value, "HKDF")
	case *ast.ObjectLit:
		for _, prop := range value.Properties {
			if prop.Key != "name" {
				continue
			}
			if name, ok := prop.Value.(*ast.StringLit); ok {
				return strings.EqualFold(name.Value, "HKDF")
			}
		}
	}
	return false
}

func (c *Checker) hkdfParamsType() *types.ObjectType {
	params := types.NewObject("$HKDFParams")
	params.AddField("name", types.TypeString, false)
	params.AddField("hash", types.TypeString, false)
	params.AddField("salt", c.cryptoBufferSourceType(), false)
	params.AddField("info", c.cryptoBufferSourceType(), false)
	return params
}

func (c *Checker) hkdfCryptoKeyType() *types.ObjectType {
	algorithm := types.NewObject("$HKDFKeyAlgorithm")
	algorithm.AddField("name", types.TypeString, false)

	key := types.NewObject("$HKDFCryptoKey")
	key.AddField("$data", c.builtinByteBufferType(), false)
	key.AddField("$algorithmName", types.TypeString, false)
	key.AddField("type", types.TypeString, false)
	key.AddField("extractable", types.TypeBoolean, false)
	key.AddField("algorithm", algorithm, false)
	key.AddField("usages", types.NewArray(types.TypeString), false)
	return key
}
