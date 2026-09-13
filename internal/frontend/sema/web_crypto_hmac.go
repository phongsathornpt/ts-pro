package sema

import (
	"fmt"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (c *Checker) checkWebCryptoHMACCall(e *ast.CallExpr, member *ast.MemberExpr) (types.Type, bool) {
	subtle, ok := member.Object.(*ast.MemberExpr)
	if !ok || subtle.Property != "subtle" {
		return nil, false
	}
	ident, ok := subtle.Object.(*ast.IdentExpr)
	if !ok || ident.Name != "crypto" {
		return nil, false
	}

	bufferSourceType := c.cryptoBufferSourceType()
	algorithmIDType := c.cryptoAlgorithmIdentifierType()
	cryptoKeyType := c.hmacCryptoKeyType()
	var fnType *types.FunctionType
	var resultType types.Type

	switch member.Property {
	case "importKey":
		resultType = c.newPromiseType(cryptoKeyType)
		fnType = types.NewFunction([]types.Param{
			{Name: "format", Type: types.TypeString},
			{Name: "keyData", Type: bufferSourceType},
			{Name: "algorithm", Type: c.hmacImportParamsType()},
			{Name: "extractable", Type: types.TypeBoolean},
			{Name: "keyUsages", Type: types.NewArray(types.TypeString)},
		}, resultType)
	case "sign":
		resultType = c.newPromiseType(c.builtinArrayBufferType())
		fnType = types.NewFunction([]types.Param{
			{Name: "algorithm", Type: algorithmIDType},
			{Name: "key", Type: cryptoKeyType},
			{Name: "data", Type: bufferSourceType},
		}, resultType)
	case "verify":
		resultType = c.newPromiseType(types.TypeBoolean)
		fnType = types.NewFunction([]types.Param{
			{Name: "algorithm", Type: algorithmIDType},
			{Name: "key", Type: cryptoKeyType},
			{Name: "signature", Type: bufferSourceType},
			{Name: "data", Type: bufferSourceType},
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
		if member.Property == "importKey" && i == 2 {
			c.materializeHMACImportOptionals(actual)
		}
		if !actual.AssignableTo(param.Type) {
			c.error(e.Args[i].Span(), "TS2345", fmt.Sprintf("crypto.subtle.%s argument %q has type %s; expected %s.", member.Property, param.Name, actual.String(), param.Type.String()))
		}
	}
	c.result.Types[e] = resultType
	return resultType, true
}

func (c *Checker) materializeHMACImportOptionals(actual types.Type) {
	obj, ok := actual.(*types.ObjectType)
	if !ok {
		return
	}
	if _, exists := obj.Fields["length"]; !exists {
		obj.AddField("length", types.TypeNumber, true)
	}
}

func (c *Checker) cryptoBufferSourceType() types.Type {
	return types.NewUnion(c.builtinArrayBufferType(), c.builtinUint8ArrayType())
}

func (c *Checker) cryptoAlgorithmIdentifierType() types.Type {
	obj := types.NewObject("$CryptoAlgorithm")
	obj.AddField("name", types.TypeString, false)
	return types.NewUnion(types.TypeString, obj)
}

func (c *Checker) hmacImportParamsType() *types.ObjectType {
	hashObj := types.NewObject("$HMACHashAlgorithm")
	hashObj.AddField("name", types.TypeString, false)

	params := types.NewObject("$HMACImportParams")
	params.AddField("name", types.TypeString, false)
	params.AddField("hash", types.NewUnion(types.TypeString, hashObj), false)
	params.AddField("length", types.TypeNumber, true)
	return params
}

func (c *Checker) hmacCryptoKeyType() *types.ObjectType {
	hash := types.NewObject("$CryptoKeyHash")
	hash.AddField("name", types.TypeString, false)
	algorithm := types.NewObject("$HMACKeyAlgorithm")
	algorithm.AddField("name", types.TypeString, false)
	algorithm.AddField("hash", hash, false)
	algorithm.AddField("length", types.TypeNumber, false)

	key := types.NewObject("$CryptoKey")
	key.AddField("$data", c.builtinByteBufferType(), false)
	key.AddField("$algorithmName", types.TypeString, false)
	key.AddField("$hashName", types.TypeString, false)
	key.AddField("type", types.TypeString, false)
	key.AddField("extractable", types.TypeBoolean, false)
	key.AddField("algorithm", algorithm, false)
	key.AddField("usages", types.NewArray(types.TypeString), false)
	return key
}
