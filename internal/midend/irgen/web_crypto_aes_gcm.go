package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) lowerWebCryptoAESGCMCall(e *ast.CallExpr, member *ast.MemberExpr) (ir.Operand, bool) {
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
		if len(e.Args) < 3 || !isAESGCMAlgorithmExprIR(e.Args[2]) {
			return nil, false
		}
		return g.lowerAESGCMImportKey(e), true
	case "encrypt":
		return g.lowerAESGCMCrypt(e, true), true
	case "decrypt":
		return g.lowerAESGCMCrypt(e, false), true
	default:
		return nil, false
	}
}

func isAESGCMAlgorithmExprIR(expr ast.Expr) bool {
	switch value := expr.(type) {
	case *ast.StringLit:
		return equalFoldASCII(value.Value, "AES-GCM")
	case *ast.ObjectLit:
		for _, prop := range value.Properties {
			if prop.Key != "name" {
				continue
			}
			if name, ok := prop.Value.(*ast.StringLit); ok {
				return equalFoldASCII(name.Value, "AES-GCM")
			}
		}
	}
	return false
}

func equalFoldASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ac, bc := a[i], b[i]
		if ac >= 'a' && ac <= 'z' {
			ac -= 'a' - 'A'
		}
		if bc >= 'a' && bc <= 'z' {
			bc -= 'a' - 'A'
		}
		if ac != bc {
			return false
		}
	}
	return true
}

func (g *generator) lowerAESGCMImportKey(e *ast.CallExpr) ir.Operand {
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

	return g.spawnCryptoTask("aes_gcm_import", taskType, keyType, captures, captureTypes, func(captured []ir.Operand) {
		formatOK := g.cryptoStringEquals(captured[0], "raw", "aes_gcm_import_raw")
		algorithmOK := g.cryptoAESGCMNameMatch(captured[2])
		supported := g.currentFn.NewValue("aes_gcm_import_supported", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: supported, Op: ir.OpAnd, LHS: formatOK, RHS: algorithmOK})

		supportedBB := g.currentFn.NewBlock("aes_gcm_import_supported")
		unsupportedBB := g.currentFn.NewBlock("aes_gcm_import_unsupported")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: supported, Then: supportedBB, Else: unsupportedBB}

		g.currentBB = unsupportedBB
		g.rejectCryptoTask("NotSupportedError", "AES-GCM currently supports raw key import only.")

		g.currentBB = supportedBB
		keyLength := g.currentFn.NewValue("aes_gcm_import_key_length", types.TypeNumber)
		keyLengthOK := g.currentFn.NewValue("aes_gcm_import_key_length_ok", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.CallInst{Res: keyLength, Callee: "ts_byte_buffer_len", Args: []ir.Operand{captured[1]}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}},
			&ir.BinaryInst{Res: keyLengthOK, Op: ir.OpEq, LHS: keyLength, RHS: ir.ConstNumber{Value: 16}},
		)
		keyOKBB := g.currentFn.NewBlock("aes_gcm_import_key_ok")
		keyBadBB := g.currentFn.NewBlock("aes_gcm_import_key_bad")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: keyLengthOK, Then: keyOKBB, Else: keyBadBB}

		g.currentBB = keyBadBB
		g.rejectCryptoTask("DataError", "AES-GCM foundation currently requires a 128-bit key.")

		g.currentBB = keyOKBB
		usagesOK := g.aesGCMUsagesValid(captured[4])
		usagesOKBB := g.currentFn.NewBlock("aes_gcm_import_usages_ok")
		usagesBadBB := g.currentFn.NewBlock("aes_gcm_import_usages_bad")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: usagesOK, Then: usagesOKBB, Else: usagesBadBB}

		g.currentBB = usagesBadBB
		g.rejectCryptoTask("SyntaxError", "AES-GCM keys only support encrypt and decrypt usages.")

		g.currentBB = usagesOKBB
		key := g.newAESGCMCryptoKey(keyType, captured[1], captured[3], captured[4])
		g.currentBB.Terminator = &ir.ReturnTerm{Val: key}
	})
}

func (g *generator) lowerAESGCMCrypt(e *ast.CallExpr, encrypt bool) ir.Operand {
	name, iv, aad, tagLength := g.lowerAESGCMParams(e.Args[0])
	key := g.lowerExpr(e.Args[1])
	data := g.copyCryptoBufferSourceExpr(e.Args[2], "aes_gcm_data")
	taskType := g.semanticType(e).(*types.ObjectType)
	resultType := g.semaResult.ArrayBufferType
	captures := []ir.Operand{name, iv, aad, tagLength, key, data}
	captureTypes := []types.Type{types.TypeString, g.semaResult.ByteBufferType, g.semaResult.ByteBufferType, types.TypeNumber, key.Type(), g.semaResult.ByteBufferType}
	prefix := "aes_gcm_decrypt"
	usage := "decrypt"
	callee := "ts_crypto_aes_128_gcm_decrypt"
	if encrypt {
		prefix = "aes_gcm_encrypt"
		usage = "encrypt"
		callee = "ts_crypto_aes_128_gcm_encrypt"
	}

	return g.spawnCryptoTask(prefix, taskType, resultType, captures, captureTypes, func(captured []ir.Operand) {
		algorithmOK := g.cryptoAESGCMNameMatch(captured[0])
		keyAlgorithm := g.cryptoKeyField(captured[4], "$algorithmName", types.TypeString)
		keyAlgorithmOK := g.cryptoAESGCMNameMatch(keyAlgorithm)
		usageOK := g.cryptoArrayContainsString(g.cryptoKeyField(captured[4], "usages", types.NewArray(types.TypeString)), usage)
		algAndKey := g.currentFn.NewValue(prefix+"_algorithm_key_ok", types.TypeBoolean)
		allowed := g.currentFn.NewValue(prefix+"_allowed", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.BinaryInst{Res: algAndKey, Op: ir.OpAnd, LHS: algorithmOK, RHS: keyAlgorithmOK},
			&ir.BinaryInst{Res: allowed, Op: ir.OpAnd, LHS: algAndKey, RHS: usageOK},
		)
		allowedBB := g.currentFn.NewBlock(prefix + "_access_ok")
		deniedBB := g.currentFn.NewBlock(prefix + "_access_denied")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: allowed, Then: allowedBB, Else: deniedBB}

		g.currentBB = deniedBB
		g.rejectCryptoTask("InvalidAccessError", "The CryptoKey algorithm or usages do not permit AES-GCM "+usage+".")

		g.currentBB = allowedBB
		ivLength := g.currentFn.NewValue(prefix+"_iv_length", types.TypeNumber)
		ivOK := g.currentFn.NewValue(prefix+"_iv_ok", types.TypeBoolean)
		tagOK := g.currentFn.NewValue(prefix+"_tag_ok", types.TypeBoolean)
		paramsOK := g.currentFn.NewValue(prefix+"_params_ok", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.CallInst{Res: ivLength, Callee: "ts_byte_buffer_len", Args: []ir.Operand{captured[1]}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}},
			&ir.BinaryInst{Res: ivOK, Op: ir.OpEq, LHS: ivLength, RHS: ir.ConstNumber{Value: 12}},
			&ir.BinaryInst{Res: tagOK, Op: ir.OpEq, LHS: captured[3], RHS: ir.ConstNumber{Value: 128}},
			&ir.BinaryInst{Res: paramsOK, Op: ir.OpAnd, LHS: ivOK, RHS: tagOK},
		)
		paramsOKBB := g.currentFn.NewBlock(prefix + "_params_ok")
		paramsBadBB := g.currentFn.NewBlock(prefix + "_params_bad")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: paramsOK, Then: paramsOKBB, Else: paramsBadBB}

		g.currentBB = paramsBadBB
		g.rejectCryptoTask("OperationError", "AES-GCM foundation currently requires a 96-bit IV and 128-bit authentication tag.")

		g.currentBB = paramsOKBB
		inputLength := g.currentFn.NewValue(prefix+"_input_length", types.TypeNumber)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: inputLength, Callee: "ts_byte_buffer_len", Args: []ir.Operand{captured[5]}, ParamTypes: []types.Type{g.semaResult.ByteBufferType},
		})
		if !encrypt {
			longEnough := g.currentFn.NewValue(prefix+"_ciphertext_long_enough", types.TypeBoolean)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: longEnough, Op: ir.OpGt, LHS: inputLength, RHS: ir.ConstNumber{Value: 16}})
			longEnoughBB := g.currentFn.NewBlock(prefix + "_ciphertext_ok")
			tooShortBB := g.currentFn.NewBlock(prefix + "_ciphertext_short")
			g.currentBB.Terminator = &ir.BranchTerm{Cond: longEnough, Then: longEnoughBB, Else: tooShortBB}
			g.currentBB = tooShortBB
			g.rejectCryptoTask("OperationError", "AES-GCM ciphertext must contain data plus a 128-bit authentication tag.")
			g.currentBB = longEnoughBB
		}

		keyData := g.cryptoKeyField(captured[4], "$data", g.semaResult.ByteBufferType)
		output := g.currentFn.NewValue(prefix+"_output", g.semaResult.ByteBufferType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: output, Callee: callee, Args: []ir.Operand{keyData, captured[1], captured[2], captured[5]},
			ParamTypes: []types.Type{g.semaResult.ByteBufferType, g.semaResult.ByteBufferType, g.semaResult.ByteBufferType, g.semaResult.ByteBufferType},
		})
		outputLength := g.currentFn.NewValue(prefix+"_output_length", types.TypeNumber)
		expectedLength := g.currentFn.NewValue(prefix+"_expected_length", types.TypeNumber)
		lengthOK := g.currentFn.NewValue(prefix+"_result_ok", types.TypeBoolean)
		op := ir.OpSub
		if encrypt {
			op = ir.OpAdd
		}
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.CallInst{Res: outputLength, Callee: "ts_byte_buffer_len", Args: []ir.Operand{output}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}},
			&ir.BinaryInst{Res: expectedLength, Op: op, LHS: inputLength, RHS: ir.ConstNumber{Value: 16}},
			&ir.BinaryInst{Res: lengthOK, Op: ir.OpEq, LHS: outputLength, RHS: expectedLength},
		)
		successBB := g.currentFn.NewBlock(prefix + "_success")
		failureBB := g.currentFn.NewBlock(prefix + "_failure")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: lengthOK, Then: successBB, Else: failureBB}

		g.currentBB = failureBB
		g.rejectCryptoTask("OperationError", "AES-GCM operation failed.")

		g.currentBB = successBB
		result := g.newArrayBufferFromData(output)
		g.currentBB.Terminator = &ir.ReturnTerm{Val: result}
	})
}

func (g *generator) lowerAESGCMParams(expr ast.Expr) (ir.Operand, ir.Operand, ir.Operand, ir.Operand) {
	objType := g.semanticType(expr).(*types.ObjectType)
	value := g.lowerExpr(expr)
	offsets, _, _ := g.objectLayout(objType)

	field := func(name string) (ir.Operand, types.Type) {
		fieldType := objType.Fields[name].Type
		result := g.currentFn.NewValue("aes_gcm_"+name, fieldType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: result, Obj: value, Field: name, Offset: offsets[name]})
		return result, fieldType
	}

	name, nameType := field("name")
	iv, ivType := field("iv")
	aad, aadType := field("additionalData")
	tagLength, _ := field("tagLength")
	return g.coerceStringType(nameType, name),
		g.copyCryptoBufferSourceOperand(iv, ivType, "aes_gcm_iv"),
		g.copyCryptoBufferSourceOperand(aad, aadType, "aes_gcm_aad"),
		tagLength
}

func (g *generator) copyCryptoBufferSourceExpr(expr ast.Expr, prefix string) ir.Operand {
	typ := g.semanticType(expr)
	return g.copyCryptoBufferSourceOperand(g.lowerExpr(expr), typ, prefix)
}

func (g *generator) cryptoAESGCMNameMatch(value ir.Operand) ir.Operand {
	upper := g.currentFn.NewValue("aes_gcm_name_upper", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: upper, Callee: "ts_string_ascii_upper", Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeString},
	})
	return g.cryptoStringEquals(upper, "AES-GCM", "aes_gcm_name_match")
}

func (g *generator) aesGCMUsagesValid(usages ir.Operand) ir.Operand {
	length := g.currentFn.NewValue("aes_gcm_usages_len", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: usages})
	entry := g.currentBB
	cond := g.currentFn.NewBlock("aes_gcm_usages_cond")
	body := g.currentFn.NewBlock("aes_gcm_usages_body")
	advance := g.currentFn.NewBlock("aes_gcm_usages_advance")
	invalid := g.currentFn.NewBlock("aes_gcm_usages_invalid")
	valid := g.currentFn.NewBlock("aes_gcm_usages_valid")
	done := g.currentFn.NewBlock("aes_gcm_usages_done")
	entry.Terminator = &ir.JumpTerm{Target: cond}

	index := g.currentFn.NewValue("aes_gcm_usages_index", types.TypeNumber)
	phi := &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{{Block: entry, Value: ir.ConstNumber{Value: 0}}}}
	cond.Phis = append(cond.Phis, phi)
	more := g.currentFn.NewValue("aes_gcm_usages_more", types.TypeBoolean)
	cond.Instructions = append(cond.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	cond.Terminator = &ir.BranchTerm{Cond: more, Then: body, Else: valid}

	item := g.currentFn.NewValue("aes_gcm_usage", types.TypeString)
	body.Instructions = append(body.Instructions, &ir.GetElementInst{Res: item, Array: usages, Index: index})
	encrypt := g.currentFn.NewValue("aes_gcm_usage_encrypt", types.TypeBoolean)
	decrypt := g.currentFn.NewValue("aes_gcm_usage_decrypt", types.TypeBoolean)
	allowed := g.currentFn.NewValue("aes_gcm_usage_allowed", types.TypeBoolean)
	body.Instructions = append(body.Instructions,
		&ir.CallInst{Res: encrypt, Callee: "ts_string_eq", Args: []ir.Operand{item, ir.ConstString{Value: "encrypt"}}, ParamTypes: []types.Type{types.TypeString, types.TypeString}},
		&ir.CallInst{Res: decrypt, Callee: "ts_string_eq", Args: []ir.Operand{item, ir.ConstString{Value: "decrypt"}}, ParamTypes: []types.Type{types.TypeString, types.TypeString}},
		&ir.BinaryInst{Res: allowed, Op: ir.OpOr, LHS: encrypt, RHS: decrypt},
	)
	body.Terminator = &ir.BranchTerm{Cond: allowed, Then: advance, Else: invalid}

	next := g.currentFn.NewValue("aes_gcm_usages_next", types.TypeNumber)
	advance.Instructions = append(advance.Instructions, &ir.BinaryInst{Res: next, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
	advance.Terminator = &ir.JumpTerm{Target: cond}
	phi.Incoming = append(phi.Incoming, ir.PhiIncoming{Block: advance, Value: next})
	valid.Terminator = &ir.JumpTerm{Target: done}
	invalid.Terminator = &ir.JumpTerm{Target: done}
	result := g.currentFn.NewValue("aes_gcm_usages_result", types.TypeBoolean)
	done.Phis = append(done.Phis, &ir.PhiInst{Res: result, Incoming: []ir.PhiIncoming{
		{Block: valid, Value: ir.ConstBool{Value: true}},
		{Block: invalid, Value: ir.ConstBool{Value: false}},
	}})
	g.currentBB = done
	return result
}

func (g *generator) newAESGCMCryptoKey(keyType *types.ObjectType, data, extractable, usages ir.Operand) ir.Operand {
	algorithmType := keyType.Fields["algorithm"].Type.(*types.ObjectType)
	algorithmOffsets, algorithmRefMask, algorithmShape := g.objectLayout(algorithmType)
	algorithm := g.currentFn.NewValue("aes_gcm_key_algorithm", algorithmType)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.AllocObjectInst{Res: algorithm, Shape: algorithmShape, FieldCount: len(algorithmOffsets), RefMask: algorithmRefMask},
		&ir.SetFieldInst{Obj: algorithm, Field: "name", Offset: algorithmOffsets["name"], Val: ir.ConstString{Value: "AES-GCM"}},
		&ir.SetFieldInst{Obj: algorithm, Field: "length", Offset: algorithmOffsets["length"], Val: ir.ConstNumber{Value: 128}},
	)

	keyOffsets, keyRefMask, keyShape := g.objectLayout(keyType)
	key := g.currentFn.NewValue("aes_gcm_key", keyType)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.AllocObjectInst{Res: key, Shape: keyShape, FieldCount: len(keyOffsets), RefMask: keyRefMask},
		&ir.SetFieldInst{Obj: key, Field: "$data", Offset: keyOffsets["$data"], Val: data},
		&ir.SetFieldInst{Obj: key, Field: "$algorithmName", Offset: keyOffsets["$algorithmName"], Val: ir.ConstString{Value: "AES-GCM"}},
		&ir.SetFieldInst{Obj: key, Field: "type", Offset: keyOffsets["type"], Val: ir.ConstString{Value: "secret"}},
		&ir.SetFieldInst{Obj: key, Field: "extractable", Offset: keyOffsets["extractable"], Val: extractable},
		&ir.SetFieldInst{Obj: key, Field: "algorithm", Offset: keyOffsets["algorithm"], Val: algorithm},
		&ir.SetFieldInst{Obj: key, Field: "usages", Offset: keyOffsets["usages"], Val: usages},
	)
	return key
}
