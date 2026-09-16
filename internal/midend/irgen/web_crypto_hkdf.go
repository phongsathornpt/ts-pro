package irgen

import (
	"strings"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) lowerWebCryptoHKDFCall(e *ast.CallExpr, member *ast.MemberExpr) (ir.Operand, bool) {
	subtle, ok := member.Object.(*ast.MemberExpr)
	if !ok || subtle.Property != "subtle" {
		return nil, false
	}
	ident, ok := subtle.Object.(*ast.IdentExpr)
	if !ok || ident.Name != "crypto" {
		return nil, false
	}

	switch member.Property {
	case "importKey":
		if len(e.Args) < 3 || !isHKDFAlgorithmExprIR(e.Args[2]) {
			return nil, false
		}
		return g.lowerHKDFImportKey(e), true
	case "deriveBits":
		return g.lowerHKDFDeriveBits(e), true
	default:
		return nil, false
	}
}

func isHKDFAlgorithmExprIR(expr ast.Expr) bool {
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

func (g *generator) lowerHKDFImportKey(e *ast.CallExpr) ir.Operand {
	format := g.lowerExpr(e.Args[0])
	keyData := g.lowerCryptoDigestInput(e.Args[1])
	algorithmName := g.lowerCryptoAlgorithmName(e.Args[2])
	extractable := g.lowerExpr(e.Args[3])
	usagesType := g.semanticType(e.Args[4]).(*types.ArrayType)
	usages := g.copyCryptoStringArray(g.lowerExpr(e.Args[4]), usagesType)
	taskType := g.semanticType(e).(*types.ObjectType)
	keyType := g.promiseSettledIRType(taskType).(*types.ObjectType)
	captures := []ir.Operand{format, keyData, algorithmName, extractable, usages}
	captureTypes := []types.Type{types.TypeString, g.semaResult.ByteBufferType, types.TypeString, types.TypeBoolean, types.NewArray(types.TypeString)}

	return g.spawnCryptoTask("hkdf_import", taskType, keyType, captures, captureTypes, func(captured []ir.Operand) {
		formatOK := g.cryptoStringEquals(captured[0], "raw", "hkdf_import_raw")
		algorithmOK := g.cryptoStringEquals(captured[2], "HKDF", "hkdf_import_name")
		supported := g.currentFn.NewValue("hkdf_import_supported", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: supported, Op: ir.OpAnd, LHS: formatOK, RHS: algorithmOK})
		supportedBB := g.currentFn.NewBlock("hkdf_import_supported")
		unsupportedBB := g.currentFn.NewBlock("hkdf_import_unsupported")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: supported, Then: supportedBB, Else: unsupportedBB}

		g.currentBB = unsupportedBB
		g.rejectCryptoTask("NotSupportedError", "HKDF currently supports raw key import only.")

		g.currentBB = supportedBB
		nonExtractable := g.currentFn.NewValue("hkdf_import_non_extractable", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: nonExtractable, Op: ir.OpEq, LHS: captured[3], RHS: ir.ConstBool{Value: false}})
		extractableOKBB := g.currentFn.NewBlock("hkdf_import_extractable_ok")
		extractableBadBB := g.currentFn.NewBlock("hkdf_import_extractable_bad")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: nonExtractable, Then: extractableOKBB, Else: extractableBadBB}

		g.currentBB = extractableBadBB
		g.rejectCryptoTask("SyntaxError", "HKDF keys must not be extractable.")

		g.currentBB = extractableOKBB
		usagesOK := g.hkdfUsagesValid(captured[4])
		usagesOKBB := g.currentFn.NewBlock("hkdf_import_usages_ok")
		usagesBadBB := g.currentFn.NewBlock("hkdf_import_usages_bad")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: usagesOK, Then: usagesOKBB, Else: usagesBadBB}

		g.currentBB = usagesBadBB
		g.rejectCryptoTask("SyntaxError", "HKDF keys only support deriveBits and deriveKey usages.")

		g.currentBB = usagesOKBB
		key := g.newHKDFCryptoKey(keyType, captured[1], captured[4])
		g.currentBB.Terminator = &ir.ReturnTerm{Val: key}
	})
}

func (g *generator) lowerHKDFDeriveBits(e *ast.CallExpr) ir.Operand {
	name, hash, salt, info := g.lowerHKDFParams(e.Args[0])
	key := g.lowerExpr(e.Args[1])
	length := g.lowerExpr(e.Args[2])
	taskType := g.semanticType(e).(*types.ObjectType)
	resultType := g.semaResult.ArrayBufferType
	captures := []ir.Operand{name, hash, salt, info, key, length}
	captureTypes := []types.Type{types.TypeString, types.TypeString, g.semaResult.ByteBufferType, g.semaResult.ByteBufferType, key.Type(), types.TypeNumber}

	return g.spawnCryptoTask("hkdf_derive_bits", taskType, resultType, captures, captureTypes, func(captured []ir.Operand) {
		nameOK := g.cryptoStringEquals(captured[0], "HKDF", "hkdf_derive_name")
		hashOK := g.cryptoStringEquals(captured[1], "SHA-256", "hkdf_derive_hash")
		keyAlgorithm := g.cryptoKeyField(captured[4], "$algorithmName", types.TypeString)
		keyOK := g.cryptoStringEquals(keyAlgorithm, "HKDF", "hkdf_derive_key_algorithm")
		usageOK := g.cryptoArrayContainsString(g.cryptoKeyField(captured[4], "usages", types.NewArray(types.TypeString)), "deriveBits")

		nameAndHash := g.currentFn.NewValue("hkdf_derive_name_hash", types.TypeBoolean)
		keyAndUsage := g.currentFn.NewValue("hkdf_derive_key_usage", types.TypeBoolean)
		allowed := g.currentFn.NewValue("hkdf_derive_allowed", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.BinaryInst{Res: nameAndHash, Op: ir.OpAnd, LHS: nameOK, RHS: hashOK},
			&ir.BinaryInst{Res: keyAndUsage, Op: ir.OpAnd, LHS: keyOK, RHS: usageOK},
			&ir.BinaryInst{Res: allowed, Op: ir.OpAnd, LHS: nameAndHash, RHS: keyAndUsage},
		)
		allowedBB := g.currentFn.NewBlock("hkdf_derive_allowed")
		deniedBB := g.currentFn.NewBlock("hkdf_derive_denied")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: allowed, Then: allowedBB, Else: deniedBB}

		g.currentBB = deniedBB
		unsupportedHashBB := g.currentFn.NewBlock("hkdf_derive_unsupported_hash")
		invalidAccessBB := g.currentFn.NewBlock("hkdf_derive_invalid_access")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: hashOK, Then: invalidAccessBB, Else: unsupportedHashBB}
		g.currentBB = unsupportedHashBB
		g.rejectCryptoTask("NotSupportedError", "HKDF currently supports SHA-256 only.")
		g.currentBB = invalidAccessBB
		g.rejectCryptoTask("InvalidAccessError", "The CryptoKey algorithm or usages do not permit HKDF deriveBits.")

		g.currentBB = allowedBB
		positive := g.currentFn.NewValue("hkdf_length_positive", types.TypeBoolean)
		multiple := g.currentFn.NewValue("hkdf_length_mod", types.TypeNumber)
		aligned := g.currentFn.NewValue("hkdf_length_aligned", types.TypeBoolean)
		withinLimit := g.currentFn.NewValue("hkdf_length_limit", types.TypeBoolean)
		validA := g.currentFn.NewValue("hkdf_length_valid_a", types.TypeBoolean)
		valid := g.currentFn.NewValue("hkdf_length_valid", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.BinaryInst{Res: positive, Op: ir.OpGt, LHS: captured[5], RHS: ir.ConstNumber{Value: 0}},
			&ir.BinaryInst{Res: multiple, Op: ir.OpMod, LHS: captured[5], RHS: ir.ConstNumber{Value: 8}},
			&ir.BinaryInst{Res: aligned, Op: ir.OpEq, LHS: multiple, RHS: ir.ConstNumber{Value: 0}},
			&ir.BinaryInst{Res: withinLimit, Op: ir.OpLe, LHS: captured[5], RHS: ir.ConstNumber{Value: 65280}},
			&ir.BinaryInst{Res: validA, Op: ir.OpAnd, LHS: positive, RHS: aligned},
			&ir.BinaryInst{Res: valid, Op: ir.OpAnd, LHS: validA, RHS: withinLimit},
		)
		lengthOKBB := g.currentFn.NewBlock("hkdf_length_ok")
		lengthBadBB := g.currentFn.NewBlock("hkdf_length_bad")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: valid, Then: lengthOKBB, Else: lengthBadBB}
		g.currentBB = lengthBadBB
		g.rejectCryptoTask("OperationError", "HKDF length must be a positive multiple of 8 and at most 65,280 bits for SHA-256.")

		g.currentBB = lengthOKBB
		keyData := g.cryptoKeyField(captured[4], "$data", g.semaResult.ByteBufferType)
		prk := g.currentFn.NewValue("hkdf_prk", g.semaResult.ByteBufferType)
		byteLength := g.currentFn.NewValue("hkdf_output_bytes", types.TypeNumber)
		okm := g.currentFn.NewValue("hkdf_okm", g.semaResult.ByteBufferType)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.CallInst{Res: prk, Callee: "ts_crypto_hkdf_extract_sha256", Args: []ir.Operand{captured[2], keyData}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, g.semaResult.ByteBufferType}},
			&ir.BinaryInst{Res: byteLength, Op: ir.OpDiv, LHS: captured[5], RHS: ir.ConstNumber{Value: 8}},
			&ir.CallInst{Res: okm, Callee: "ts_crypto_hkdf_expand_sha256", Args: []ir.Operand{prk, captured[3], byteLength}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, g.semaResult.ByteBufferType, types.TypeNumber}},
		)
		result := g.newArrayBufferFromData(okm)
		g.currentBB.Terminator = &ir.ReturnTerm{Val: result}
	})
}

func (g *generator) lowerHKDFParams(expr ast.Expr) (ir.Operand, ir.Operand, ir.Operand, ir.Operand) {
	objType := g.semanticType(expr).(*types.ObjectType)
	value := g.lowerExpr(expr)
	offsets, _, _ := g.objectLayout(objType)

	field := func(name string) (ir.Operand, types.Type) {
		fieldType := objType.Fields[name].Type
		result := g.currentFn.NewValue("hkdf_"+name, fieldType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: result, Obj: value, Field: name, Offset: offsets[name]})
		return result, fieldType
	}

	name, nameType := field("name")
	hash, hashType := field("hash")
	salt, saltType := field("salt")
	info, infoType := field("info")
	return g.coerceStringType(nameType, name), g.coerceStringType(hashType, hash), g.copyCryptoBufferSourceOperand(salt, saltType, "hkdf_salt"), g.copyCryptoBufferSourceOperand(info, infoType, "hkdf_info")
}

func (g *generator) copyCryptoBufferSourceOperand(value ir.Operand, typ types.Type, prefix string) ir.Operand {
	objType, ok := typ.(*types.ObjectType)
	if !ok {
		return g.currentFn.NewValue(prefix+"_invalid", g.semaResult.ByteBufferType)
	}
	if objType.Name == "$ArrayBuffer" {
		return g.copyByteBuffer(g.arrayBufferData(value))
	}

	offsets, _, _ := g.objectLayout(objType)
	raw := g.currentFn.NewValue(prefix+"_data", g.semaResult.ByteBufferType)
	offset := g.currentFn.NewValue(prefix+"_offset", types.TypeNumber)
	length := g.currentFn.NewValue(prefix+"_length", types.TypeNumber)
	end := g.currentFn.NewValue(prefix+"_end", types.TypeNumber)
	copy := g.currentFn.NewValue(prefix+"_copy", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.GetFieldInst{Res: raw, Obj: value, Field: "$data", Offset: offsets["$data"]},
		&ir.GetFieldInst{Res: offset, Obj: value, Field: "byteOffset", Offset: offsets["byteOffset"]},
		&ir.GetFieldInst{Res: length, Obj: value, Field: "byteLength", Offset: offsets["byteLength"]},
		&ir.BinaryInst{Res: end, Op: ir.OpAdd, LHS: offset, RHS: length},
		&ir.CallInst{Res: copy, Callee: "ts_byte_buffer_slice", Args: []ir.Operand{raw, offset, end}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber}},
	)
	return copy
}

func (g *generator) hkdfUsagesValid(usages ir.Operand) ir.Operand {
	length := g.currentFn.NewValue("hkdf_usage_length", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayLenInst{Res: length, Array: usages})
	valid := ir.Operand(ir.ConstBool{Value: true})
	for i := 0; i < 8; i++ {
		inRange := g.currentFn.NewValue("hkdf_usage_in_range", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: inRange, Op: ir.OpLt, LHS: ir.ConstNumber{Value: float64(i)}, RHS: length})
		checkBB := g.currentFn.NewBlock("hkdf_usage_check")
		nextBB := g.currentFn.NewBlock("hkdf_usage_next")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: inRange, Then: checkBB, Else: nextBB}

		g.currentBB = checkBB
		usage := g.currentFn.NewValue("hkdf_usage", types.TypeString)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayGetInst{Res: usage, Array: usages, Index: ir.ConstNumber{Value: float64(i)}})
		deriveBits := g.cryptoStringEquals(usage, "deriveBits", "hkdf_usage_derive_bits")
		deriveKey := g.cryptoStringEquals(usage, "deriveKey", "hkdf_usage_derive_key")
		allowed := g.currentFn.NewValue("hkdf_usage_allowed", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: allowed, Op: ir.OpOr, LHS: deriveBits, RHS: deriveKey})
		checkEnd := g.currentBB
		checkEnd.Terminator = &ir.JumpTerm{Target: nextBB}

		combined := g.currentFn.NewValue("hkdf_usage_valid", types.TypeBoolean)
		nextBB.Phis = append(nextBB.Phis, &ir.PhiInst{Res: combined, Incoming: []ir.PhiIncoming{{Block: checkEnd, Value: allowed}, {Block: g.currentBB, Value: valid}}})
		g.currentBB = nextBB
		valid = combined
	}
	return valid
}

func (g *generator) newHKDFCryptoKey(keyType *types.ObjectType, data, usages ir.Operand) ir.Operand {
	algorithmType := keyType.Fields["algorithm"].Type.(*types.ObjectType)
	algorithmOffsets, algorithmRefMask, algorithmShape := g.objectLayout(algorithmType)
	algorithm := g.currentFn.NewValue("hkdf_key_algorithm", algorithmType)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.AllocObjectInst{Res: algorithm, Shape: algorithmShape, FieldCount: len(algorithmOffsets), RefMask: algorithmRefMask},
		&ir.SetFieldInst{Obj: algorithm, Field: "name", Offset: algorithmOffsets["name"], Val: ir.ConstString{Value: "HKDF"}},
	)

	keyOffsets, keyRefMask, keyShape := g.objectLayout(keyType)
	key := g.currentFn.NewValue("hkdf_key", keyType)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.AllocObjectInst{Res: key, Shape: keyShape, FieldCount: len(keyOffsets), RefMask: keyRefMask},
		&ir.SetFieldInst{Obj: key, Field: "$data", Offset: keyOffsets["$data"], Val: data},
		&ir.SetFieldInst{Obj: key, Field: "$algorithmName", Offset: keyOffsets["$algorithmName"], Val: ir.ConstString{Value: "HKDF"}},
		&ir.SetFieldInst{Obj: key, Field: "type", Offset: keyOffsets["type"], Val: ir.ConstString{Value: "secret"}},
		&ir.SetFieldInst{Obj: key, Field: "extractable", Offset: keyOffsets["extractable"], Val: ir.ConstBool{Value: false}},
		&ir.SetFieldInst{Obj: key, Field: "algorithm", Offset: keyOffsets["algorithm"], Val: algorithm},
		&ir.SetFieldInst{Obj: key, Field: "usages", Offset: keyOffsets["usages"], Val: usages},
	)
	return key
}
