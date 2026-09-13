package irgen

import (
	"fmt"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) lowerWebCryptoHMACCall(e *ast.CallExpr, member *ast.MemberExpr) (ir.Operand, bool) {
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
		return g.lowerHMACImportKey(e), true
	case "sign":
		return g.lowerHMACSign(e), true
	case "verify":
		return g.lowerHMACVerify(e), true
	default:
		return nil, false
	}
}

func (g *generator) lowerHMACImportKey(e *ast.CallExpr) ir.Operand {
	format := g.lowerExpr(e.Args[0])
	keyData := g.lowerCryptoDigestInput(e.Args[1])
	algorithmName, hashName := g.lowerHMACImportAlgorithm(e.Args[2])
	extractable := g.lowerExpr(e.Args[3])
	usages := g.copyCryptoStringArray(g.lowerExpr(e.Args[4]), g.semanticType(e.Args[4]).(*types.ArrayType))

	taskType := g.semanticType(e).(*types.ObjectType)
	keyType := g.promiseSettledIRType(taskType).(*types.ObjectType)
	captures := []ir.Operand{format, keyData, algorithmName, hashName, extractable, usages}
	captureTypes := []types.Type{types.TypeString, g.semaResult.ByteBufferType, types.TypeString, types.TypeString, types.TypeBoolean, types.NewArray(types.TypeString)}

	return g.spawnCryptoTask("hmac_import", taskType, keyType, captures, captureTypes, func(captured []ir.Operand) {
		formatOK := g.cryptoStringEquals(captured[0], "raw", "hmac_import_raw")
		algorithmOK := g.cryptoHMACNameMatch(captured[2])
		hashOK := g.cryptoSHANameMatch(captured[3], "256")
		formatAndAlgorithm := g.currentFn.NewValue("hmac_import_format_algorithm_ok", types.TypeBoolean)
		allSupported := g.currentFn.NewValue("hmac_import_supported", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.BinaryInst{Res: formatAndAlgorithm, Op: ir.OpAnd, LHS: formatOK, RHS: algorithmOK},
			&ir.BinaryInst{Res: allSupported, Op: ir.OpAnd, LHS: formatAndAlgorithm, RHS: hashOK},
		)
		supportedBB := g.currentFn.NewBlock("hmac_import_supported")
		unsupportedBB := g.currentFn.NewBlock("hmac_import_unsupported")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: allSupported, Then: supportedBB, Else: unsupportedBB}

		g.currentBB = unsupportedBB
		g.rejectCryptoTask("NotSupportedError", "Only raw HMAC keys with SHA-256 are currently supported.")

		g.currentBB = supportedBB
		validUsages := g.cryptoHMACUsagesValid(captured[5])
		usagesOKBB := g.currentFn.NewBlock("hmac_import_usages_ok")
		usagesBadBB := g.currentFn.NewBlock("hmac_import_usages_bad")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: validUsages, Then: usagesOKBB, Else: usagesBadBB}

		g.currentBB = usagesBadBB
		g.rejectCryptoTask("SyntaxError", "HMAC keys only support sign and verify usages.")

		g.currentBB = usagesOKBB
		keyLength := g.currentFn.NewValue("hmac_import_key_bytes", types.TypeNumber)
		nonEmpty := g.currentFn.NewValue("hmac_import_non_empty", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.CallInst{Res: keyLength, Callee: "ts_byte_buffer_len", Args: []ir.Operand{captured[1]}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}},
			&ir.BinaryInst{Res: nonEmpty, Op: ir.OpGt, LHS: keyLength, RHS: ir.ConstNumber{Value: 0}},
		)
		keyOKBB := g.currentFn.NewBlock("hmac_import_key_ok")
		keyBadBB := g.currentFn.NewBlock("hmac_import_key_empty")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: nonEmpty, Then: keyOKBB, Else: keyBadBB}

		g.currentBB = keyBadBB
		g.rejectCryptoTask("DataError", "HMAC key data must not be empty.")

		g.currentBB = keyOKBB
		key := g.newHMACCryptoKey(keyType, captured[1], captured[4], captured[5], keyLength)
		g.currentBB.Terminator = &ir.ReturnTerm{Val: key}
	})
}

func (g *generator) lowerHMACSign(e *ast.CallExpr) ir.Operand {
	algorithmName := g.lowerCryptoAlgorithmName(e.Args[0])
	key := g.lowerExpr(e.Args[1])
	data := g.lowerCryptoDigestInput(e.Args[2])
	taskType := g.semanticType(e).(*types.ObjectType)
	resultType := g.semaResult.ArrayBufferType
	captures := []ir.Operand{algorithmName, key, data}
	captureTypes := []types.Type{types.TypeString, key.Type(), g.semaResult.ByteBufferType}

	return g.spawnCryptoTask("hmac_sign", taskType, resultType, captures, captureTypes, func(captured []ir.Operand) {
		if !g.branchHMACKeyAccess(captured[0], captured[1], "sign", "hmac_sign") {
			return
		}
		keyData := g.cryptoKeyField(captured[1], "$data", g.semaResult.ByteBufferType)
		digest := g.currentFn.NewValue("hmac_signature", g.semaResult.ByteBufferType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: digest, Callee: "ts_crypto_hmac_sha256", Args: []ir.Operand{keyData, captured[2]}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, g.semaResult.ByteBufferType},
		})
		result := g.newArrayBufferFromData(digest)
		g.currentBB.Terminator = &ir.ReturnTerm{Val: result}
	})
}

func (g *generator) lowerHMACVerify(e *ast.CallExpr) ir.Operand {
	algorithmName := g.lowerCryptoAlgorithmName(e.Args[0])
	key := g.lowerExpr(e.Args[1])
	signature := g.lowerCryptoDigestInput(e.Args[2])
	data := g.lowerCryptoDigestInput(e.Args[3])
	taskType := g.semanticType(e).(*types.ObjectType)
	captures := []ir.Operand{algorithmName, key, signature, data}
	captureTypes := []types.Type{types.TypeString, key.Type(), g.semaResult.ByteBufferType, g.semaResult.ByteBufferType}

	return g.spawnCryptoTask("hmac_verify", taskType, types.TypeBoolean, captures, captureTypes, func(captured []ir.Operand) {
		if !g.branchHMACKeyAccess(captured[0], captured[1], "verify", "hmac_verify") {
			return
		}
		keyData := g.cryptoKeyField(captured[1], "$data", g.semaResult.ByteBufferType)
		expected := g.currentFn.NewValue("hmac_expected", g.semaResult.ByteBufferType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: expected, Callee: "ts_crypto_hmac_sha256", Args: []ir.Operand{keyData, captured[3]}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, g.semaResult.ByteBufferType},
		})
		valid := g.cryptoByteBufferEqual(expected, captured[2])
		g.currentBB.Terminator = &ir.ReturnTerm{Val: valid}
	})
}

func (g *generator) spawnCryptoTask(prefix string, taskType *types.ObjectType, resultType types.Type, captures []ir.Operand, captureTypes []types.Type, build func([]ir.Operand)) ir.Operand {
	outerFn, outerBB, outerLocals, outerProv, outerDirect := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	driverName := fmt.Sprintf("$crypto_%s_%d", prefix, g.arrowCounter)
	g.arrowCounter++
	driverType := types.NewFunction(nil, resultType)
	driver := ir.NewFunction(driverName, resultType)
	g.currentFn = driver
	g.currentBB = driver.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)
	env := driver.NewValue("$env", driverType)
	driver.Params = append(driver.Params, env)

	captured := make([]ir.Operand, len(captures))
	for i, typ := range captureTypes {
		value := driver.NewValue(fmt.Sprintf("%s_capture_%d", prefix, i), typ)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ClosureGetInst{Res: value, Closure: env, Index: i})
		captured[i] = value
	}
	build(captured)
	g.prog.Functions = append(g.prog.Functions, driver)

	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProv, outerDirect
	closure := g.currentFn.NewValue(prefix+"_closure", driverType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{
		Res: closure, Function: driverName, Captures: captures, RefMask: cryptoCaptureRefMask(captures),
	})
	task := g.currentFn.NewValue(prefix+"_task", taskType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: task, Callee: "ts_task_spawn", Args: []ir.Operand{closure, ir.ConstNumber{Value: nativeTaskResultKind(resultType)}},
	})
	return task
}

func cryptoCaptureRefMask(captures []ir.Operand) uint64 {
	var mask uint64
	for i, capture := range captures {
		if i < 64 && irHeapRefType(capture.Type()) {
			mask |= uint64(1) << uint(i)
		}
	}
	return mask
}

func (g *generator) lowerHMACImportAlgorithm(expr ast.Expr) (ir.Operand, ir.Operand) {
	objType := g.semanticType(expr).(*types.ObjectType)
	value := g.lowerExpr(expr)
	offsets, _, _ := g.objectLayout(objType)

	nameField := objType.Fields["name"]
	name := g.currentFn.NewValue("hmac_import_algorithm", nameField.Type)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: name, Obj: value, Field: "name", Offset: offsets["name"]})
	nameString := g.coerceStringType(nameField.Type, name)

	hashField := objType.Fields["hash"]
	hash := g.currentFn.NewValue("hmac_import_hash", hashField.Type)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: hash, Obj: value, Field: "hash", Offset: offsets["hash"]})
	if hashField.Type == types.TypeString {
		return nameString, hash
	}
	if hashObj, ok := hashField.Type.(*types.ObjectType); ok {
		hashOffsets, _, _ := g.objectLayout(hashObj)
		hashNameField := hashObj.Fields["name"]
		hashName := g.currentFn.NewValue("hmac_import_hash_name", hashNameField.Type)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: hashName, Obj: hash, Field: "name", Offset: hashOffsets["name"]})
		return nameString, g.coerceStringType(hashNameField.Type, hashName)
	}
	return nameString, ir.ConstString{Value: ""}
}

func (g *generator) newHMACCryptoKey(keyType *types.ObjectType, data, extractable, usages, keyByteLength ir.Operand) ir.Operand {
	algorithmType := keyType.Fields["algorithm"].Type.(*types.ObjectType)
	hashType := algorithmType.Fields["hash"].Type.(*types.ObjectType)

	hashOffsets, hashRefMask, hashShape := g.objectLayout(hashType)
	hash := g.currentFn.NewValue("hmac_key_hash", hashType)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.AllocObjectInst{Res: hash, Shape: hashShape, FieldCount: len(hashOffsets), RefMask: hashRefMask},
		&ir.SetFieldInst{Obj: hash, Field: "name", Offset: hashOffsets["name"], Val: ir.ConstString{Value: "SHA-256"}},
	)

	algorithmOffsets, algorithmRefMask, algorithmShape := g.objectLayout(algorithmType)
	algorithm := g.currentFn.NewValue("hmac_key_algorithm", algorithmType)
	keyBits := g.currentFn.NewValue("hmac_key_bits", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: keyBits, Op: ir.OpMul, LHS: keyByteLength, RHS: ir.ConstNumber{Value: 8}},
		&ir.AllocObjectInst{Res: algorithm, Shape: algorithmShape, FieldCount: len(algorithmOffsets), RefMask: algorithmRefMask},
		&ir.SetFieldInst{Obj: algorithm, Field: "name", Offset: algorithmOffsets["name"], Val: ir.ConstString{Value: "HMAC"}},
		&ir.SetFieldInst{Obj: algorithm, Field: "hash", Offset: algorithmOffsets["hash"], Val: hash},
		&ir.SetFieldInst{Obj: algorithm, Field: "length", Offset: algorithmOffsets["length"], Val: keyBits},
	)

	keyOffsets, keyRefMask, keyShape := g.objectLayout(keyType)
	key := g.currentFn.NewValue("crypto_key", keyType)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.AllocObjectInst{Res: key, Shape: keyShape, FieldCount: len(keyOffsets), RefMask: keyRefMask},
		&ir.SetFieldInst{Obj: key, Field: "$data", Offset: keyOffsets["$data"], Val: data},
		&ir.SetFieldInst{Obj: key, Field: "$algorithmName", Offset: keyOffsets["$algorithmName"], Val: ir.ConstString{Value: "HMAC"}},
		&ir.SetFieldInst{Obj: key, Field: "$hashName", Offset: keyOffsets["$hashName"], Val: ir.ConstString{Value: "SHA-256"}},
		&ir.SetFieldInst{Obj: key, Field: "type", Offset: keyOffsets["type"], Val: ir.ConstString{Value: "secret"}},
		&ir.SetFieldInst{Obj: key, Field: "extractable", Offset: keyOffsets["extractable"], Val: extractable},
		&ir.SetFieldInst{Obj: key, Field: "algorithm", Offset: keyOffsets["algorithm"], Val: algorithm},
		&ir.SetFieldInst{Obj: key, Field: "usages", Offset: keyOffsets["usages"], Val: usages},
	)
	return key
}

func (g *generator) cryptoKeyField(key ir.Operand, name string, typ types.Type) ir.Operand {
	keyType := key.Type().(*types.ObjectType)
	offsets, _, _ := g.objectLayout(keyType)
	value := g.currentFn.NewValue("crypto_key_"+name, typ)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: value, Obj: key, Field: name, Offset: offsets[name]})
	return value
}

func (g *generator) branchHMACKeyAccess(algorithmName, key ir.Operand, usage, prefix string) bool {
	algorithmOK := g.cryptoHMACNameMatch(algorithmName)
	keyAlgorithm := g.cryptoKeyField(key, "$algorithmName", types.TypeString)
	keyAlgorithmOK := g.cryptoStringEquals(keyAlgorithm, "HMAC", prefix+"_key_algorithm")
	keyHash := g.cryptoKeyField(key, "$hashName", types.TypeString)
	keyHashOK := g.cryptoStringEquals(keyHash, "SHA-256", prefix+"_key_hash")
	usageOK := g.cryptoArrayContainsString(g.cryptoKeyField(key, "usages", types.NewArray(types.TypeString)), usage)

	algAndKey := g.currentFn.NewValue(prefix+"_alg_key_ok", types.TypeBoolean)
	hashAndUsage := g.currentFn.NewValue(prefix+"_hash_usage_ok", types.TypeBoolean)
	allowed := g.currentFn.NewValue(prefix+"_allowed", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: algAndKey, Op: ir.OpAnd, LHS: algorithmOK, RHS: keyAlgorithmOK},
		&ir.BinaryInst{Res: hashAndUsage, Op: ir.OpAnd, LHS: keyHashOK, RHS: usageOK},
		&ir.BinaryInst{Res: allowed, Op: ir.OpAnd, LHS: algAndKey, RHS: hashAndUsage},
	)
	allowedBB := g.currentFn.NewBlock(prefix + "_allowed")
	deniedBB := g.currentFn.NewBlock(prefix + "_denied")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: allowed, Then: allowedBB, Else: deniedBB}

	g.currentBB = deniedBB
	g.rejectCryptoTask("InvalidAccessError", "The CryptoKey algorithm or usages do not permit this operation.")
	g.currentBB = allowedBB
	return true
}

func (g *generator) rejectCryptoTask(name, message string) {
	err := g.newDOMException(ir.ConstString{Value: message}, ir.ConstString{Value: name})
	boxed := g.boxJSValue(err, g.semaResult.DOMExceptionType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_reject", Args: []ir.Operand{boxed}, ParamTypes: []types.Type{types.TypeAny}})
	g.currentBB.Terminator = &ir.ReturnTerm{}
}

func (g *generator) cryptoStringEquals(value ir.Operand, expected, name string) ir.Operand {
	result := g.currentFn.NewValue(name, types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: result, Callee: "ts_string_eq", Args: []ir.Operand{value, ir.ConstString{Value: expected}}, ParamTypes: []types.Type{types.TypeString, types.TypeString},
	})
	return result
}

func (g *generator) cryptoHMACNameMatch(value ir.Operand) ir.Operand {
	variants := []string{
		"HMAC", "HMAc", "HMaC", "HMac", "HmAC", "HmAc", "HmaC", "Hmac",
		"hMAC", "hMAc", "hMaC", "hMac", "hmAC", "hmAc", "hmaC", "hmac",
	}
	var match ir.Operand
	for i, variant := range variants {
		equal := g.cryptoStringEquals(value, variant, fmt.Sprintf("hmac_name_match_%d", i))
		if match == nil {
			match = equal
			continue
		}
		combined := g.currentFn.NewValue(fmt.Sprintf("hmac_name_match_any_%d", i), types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: combined, Op: ir.OpOr, LHS: match, RHS: equal})
		match = combined
	}
	return match
}

func (g *generator) copyCryptoStringArray(src ir.Operand, arrayType *types.ArrayType) ir.Operand {
	length := g.currentFn.NewValue("crypto_array_copy_len", types.TypeNumber)
	dst := g.currentFn.NewValue("crypto_array_copy", arrayType)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.ArrayLengthInst{Res: length, Array: src},
		&ir.AllocArrayInst{Res: dst, ElemType: arrayType.Elem, Length: length},
	)
	entry := g.currentBB
	cond := g.currentFn.NewBlock("crypto_array_copy_cond")
	body := g.currentFn.NewBlock("crypto_array_copy_body")
	done := g.currentFn.NewBlock("crypto_array_copy_done")
	entry.Terminator = &ir.JumpTerm{Target: cond}
	index := g.currentFn.NewValue("crypto_array_copy_index", types.TypeNumber)
	phi := &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{{Block: entry, Value: ir.ConstNumber{Value: 0}}}}
	cond.Phis = append(cond.Phis, phi)
	more := g.currentFn.NewValue("crypto_array_copy_more", types.TypeBoolean)
	cond.Instructions = append(cond.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	cond.Terminator = &ir.BranchTerm{Cond: more, Then: body, Else: done}
	item := g.currentFn.NewValue("crypto_array_copy_item", arrayType.Elem)
	next := g.currentFn.NewValue("crypto_array_copy_next", types.TypeNumber)
	body.Instructions = append(body.Instructions,
		&ir.GetElementInst{Res: item, Array: src, Index: index},
		&ir.SetElementInst{Array: dst, Index: index, Val: item},
		&ir.BinaryInst{Res: next, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}},
	)
	body.Terminator = &ir.JumpTerm{Target: cond}
	phi.Incoming = append(phi.Incoming, ir.PhiIncoming{Block: body, Value: next})
	g.currentBB = done
	return dst
}

func (g *generator) cryptoArrayContainsString(array ir.Operand, expected string) ir.Operand {
	length := g.currentFn.NewValue("crypto_usage_len", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: array})
	entry := g.currentBB
	cond := g.currentFn.NewBlock("crypto_usage_cond")
	body := g.currentFn.NewBlock("crypto_usage_body")
	found := g.currentFn.NewBlock("crypto_usage_found")
	notFound := g.currentFn.NewBlock("crypto_usage_not_found")
	done := g.currentFn.NewBlock("crypto_usage_done")
	entry.Terminator = &ir.JumpTerm{Target: cond}
	index := g.currentFn.NewValue("crypto_usage_index", types.TypeNumber)
	phi := &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{{Block: entry, Value: ir.ConstNumber{Value: 0}}}}
	cond.Phis = append(cond.Phis, phi)
	more := g.currentFn.NewValue("crypto_usage_more", types.TypeBoolean)
	cond.Instructions = append(cond.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	cond.Terminator = &ir.BranchTerm{Cond: more, Then: body, Else: notFound}
	item := g.currentFn.NewValue("crypto_usage_item", types.TypeString)
	body.Instructions = append(body.Instructions, &ir.GetElementInst{Res: item, Array: array, Index: index})
	match := g.currentFn.NewValue("crypto_usage_match", types.TypeBoolean)
	body.Instructions = append(body.Instructions, &ir.CallInst{Res: match, Callee: "ts_string_eq", Args: []ir.Operand{item, ir.ConstString{Value: expected}}, ParamTypes: []types.Type{types.TypeString, types.TypeString}})
	advance := g.currentFn.NewBlock("crypto_usage_advance")
	body.Terminator = &ir.BranchTerm{Cond: match, Then: found, Else: advance}
	next := g.currentFn.NewValue("crypto_usage_next", types.TypeNumber)
	advance.Instructions = append(advance.Instructions, &ir.BinaryInst{Res: next, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
	advance.Terminator = &ir.JumpTerm{Target: cond}
	phi.Incoming = append(phi.Incoming, ir.PhiIncoming{Block: advance, Value: next})
	found.Terminator = &ir.JumpTerm{Target: done}
	notFound.Terminator = &ir.JumpTerm{Target: done}
	result := g.currentFn.NewValue("crypto_usage_result", types.TypeBoolean)
	done.Phis = append(done.Phis, &ir.PhiInst{Res: result, Incoming: []ir.PhiIncoming{{Block: found, Value: ir.ConstBool{Value: true}}, {Block: notFound, Value: ir.ConstBool{Value: false}}}})
	g.currentBB = done
	return result
}

func (g *generator) cryptoHMACUsagesValid(usages ir.Operand) ir.Operand {
	length := g.currentFn.NewValue("hmac_usages_len", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: usages})
	entry := g.currentBB
	cond := g.currentFn.NewBlock("hmac_usages_cond")
	body := g.currentFn.NewBlock("hmac_usages_body")
	advance := g.currentFn.NewBlock("hmac_usages_advance")
	invalid := g.currentFn.NewBlock("hmac_usages_invalid")
	valid := g.currentFn.NewBlock("hmac_usages_valid")
	done := g.currentFn.NewBlock("hmac_usages_done")
	entry.Terminator = &ir.JumpTerm{Target: cond}
	index := g.currentFn.NewValue("hmac_usages_index", types.TypeNumber)
	phi := &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{{Block: entry, Value: ir.ConstNumber{Value: 0}}}}
	cond.Phis = append(cond.Phis, phi)
	more := g.currentFn.NewValue("hmac_usages_more", types.TypeBoolean)
	cond.Instructions = append(cond.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	cond.Terminator = &ir.BranchTerm{Cond: more, Then: body, Else: valid}
	item := g.currentFn.NewValue("hmac_usage", types.TypeString)
	body.Instructions = append(body.Instructions, &ir.GetElementInst{Res: item, Array: usages, Index: index})
	sign := g.currentFn.NewValue("hmac_usage_sign", types.TypeBoolean)
	verify := g.currentFn.NewValue("hmac_usage_verify", types.TypeBoolean)
	allowed := g.currentFn.NewValue("hmac_usage_allowed", types.TypeBoolean)
	body.Instructions = append(body.Instructions,
		&ir.CallInst{Res: sign, Callee: "ts_string_eq", Args: []ir.Operand{item, ir.ConstString{Value: "sign"}}, ParamTypes: []types.Type{types.TypeString, types.TypeString}},
		&ir.CallInst{Res: verify, Callee: "ts_string_eq", Args: []ir.Operand{item, ir.ConstString{Value: "verify"}}, ParamTypes: []types.Type{types.TypeString, types.TypeString}},
		&ir.BinaryInst{Res: allowed, Op: ir.OpOr, LHS: sign, RHS: verify},
	)
	body.Terminator = &ir.BranchTerm{Cond: allowed, Then: advance, Else: invalid}
	next := g.currentFn.NewValue("hmac_usages_next", types.TypeNumber)
	advance.Instructions = append(advance.Instructions, &ir.BinaryInst{Res: next, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
	advance.Terminator = &ir.JumpTerm{Target: cond}
	phi.Incoming = append(phi.Incoming, ir.PhiIncoming{Block: advance, Value: next})
	valid.Terminator = &ir.JumpTerm{Target: done}
	invalid.Terminator = &ir.JumpTerm{Target: done}
	result := g.currentFn.NewValue("hmac_usages_result", types.TypeBoolean)
	done.Phis = append(done.Phis, &ir.PhiInst{Res: result, Incoming: []ir.PhiIncoming{{Block: valid, Value: ir.ConstBool{Value: true}}, {Block: invalid, Value: ir.ConstBool{Value: false}}}})
	g.currentBB = done
	return result
}

func (g *generator) cryptoByteBufferEqual(left, right ir.Operand) ir.Operand {
	leftLen := g.currentFn.NewValue("crypto_compare_left_len", types.TypeNumber)
	rightLen := g.currentFn.NewValue("crypto_compare_right_len", types.TypeNumber)
	sameLen := g.currentFn.NewValue("crypto_compare_same_len", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.CallInst{Res: leftLen, Callee: "ts_byte_buffer_len", Args: []ir.Operand{left}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}},
		&ir.CallInst{Res: rightLen, Callee: "ts_byte_buffer_len", Args: []ir.Operand{right}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}},
		&ir.BinaryInst{Res: sameLen, Op: ir.OpEq, LHS: leftLen, RHS: rightLen},
	)
	entry := g.currentBB
	cond := g.currentFn.NewBlock("crypto_compare_cond")
	body := g.currentFn.NewBlock("crypto_compare_body")
	lengthMismatch := g.currentFn.NewBlock("crypto_compare_length_mismatch")
	finish := g.currentFn.NewBlock("crypto_compare_finish")
	done := g.currentFn.NewBlock("crypto_compare_done")
	entry.Terminator = &ir.BranchTerm{Cond: sameLen, Then: cond, Else: lengthMismatch}

	index := g.currentFn.NewValue("crypto_compare_index", types.TypeNumber)
	allEqual := g.currentFn.NewValue("crypto_compare_all", types.TypeBoolean)
	indexPhi := &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{{Block: entry, Value: ir.ConstNumber{Value: 0}}}}
	allPhi := &ir.PhiInst{Res: allEqual, Incoming: []ir.PhiIncoming{{Block: entry, Value: ir.ConstBool{Value: true}}}}
	cond.Phis = append(cond.Phis, indexPhi, allPhi)
	more := g.currentFn.NewValue("crypto_compare_more", types.TypeBoolean)
	cond.Instructions = append(cond.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: leftLen})
	cond.Terminator = &ir.BranchTerm{Cond: more, Then: body, Else: finish}

	leftByte := g.currentFn.NewValue("crypto_compare_left", types.TypeNumber)
	rightByte := g.currentFn.NewValue("crypto_compare_right", types.TypeNumber)
	byteEqual := g.currentFn.NewValue("crypto_compare_byte_equal", types.TypeBoolean)
	nextAll := g.currentFn.NewValue("crypto_compare_next_all", types.TypeBoolean)
	nextIndex := g.currentFn.NewValue("crypto_compare_next_index", types.TypeNumber)
	body.Instructions = append(body.Instructions,
		&ir.CallInst{Res: leftByte, Callee: "ts_byte_buffer_get", Args: []ir.Operand{left, index}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber}},
		&ir.CallInst{Res: rightByte, Callee: "ts_byte_buffer_get", Args: []ir.Operand{right, index}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber}},
		&ir.BinaryInst{Res: byteEqual, Op: ir.OpEq, LHS: leftByte, RHS: rightByte},
		&ir.BinaryInst{Res: nextAll, Op: ir.OpAnd, LHS: allEqual, RHS: byteEqual},
		&ir.BinaryInst{Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}},
	)
	body.Terminator = &ir.JumpTerm{Target: cond}
	indexPhi.Incoming = append(indexPhi.Incoming, ir.PhiIncoming{Block: body, Value: nextIndex})
	allPhi.Incoming = append(allPhi.Incoming, ir.PhiIncoming{Block: body, Value: nextAll})

	lengthMismatch.Terminator = &ir.JumpTerm{Target: done}
	finish.Terminator = &ir.JumpTerm{Target: done}
	result := g.currentFn.NewValue("crypto_compare_result", types.TypeBoolean)
	done.Phis = append(done.Phis, &ir.PhiInst{Res: result, Incoming: []ir.PhiIncoming{{Block: lengthMismatch, Value: ir.ConstBool{Value: false}}, {Block: finish, Value: allEqual}}})
	g.currentBB = done
	return result
}
