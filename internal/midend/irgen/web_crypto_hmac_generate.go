package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) lowerHMACGenerateKey(e *ast.CallExpr) ir.Operand {
	algorithmName, hashName, requestedLength := g.lowerHMACImportAlgorithm(e.Args[0])
	extractable := g.lowerExpr(e.Args[1])
	usages := g.copyCryptoStringArray(g.lowerExpr(e.Args[2]), g.semanticType(e.Args[2]).(*types.ArrayType))

	taskType := g.semanticType(e).(*types.ObjectType)
	keyType := g.promiseSettledIRType(taskType).(*types.ObjectType)
	captures := []ir.Operand{algorithmName, hashName, requestedLength, extractable, usages}
	captureTypes := []types.Type{types.TypeString, types.TypeString, types.TypeNumber, types.TypeBoolean, types.NewArray(types.TypeString)}

	return g.spawnCryptoTask("hmac_generate", taskType, keyType, captures, captureTypes, func(captured []ir.Operand) {
		algorithmOK := g.cryptoHMACNameMatch(captured[0])
		algorithmOKBB := g.currentFn.NewBlock("hmac_generate_algorithm_ok")
		algorithmBadBB := g.currentFn.NewBlock("hmac_generate_algorithm_bad")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: algorithmOK, Then: algorithmOKBB, Else: algorithmBadBB}

		g.currentBB = algorithmBadBB
		g.rejectCryptoTask("NotSupportedError", "Only HMAC key generation is currently supported.")

		g.currentBB = algorithmOKBB
		validUsages := g.cryptoHMACUsagesValid(captured[4])
		usagesOKBB := g.currentFn.NewBlock("hmac_generate_usages_ok")
		usagesBadBB := g.currentFn.NewBlock("hmac_generate_usages_bad")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: validUsages, Then: usagesOKBB, Else: usagesBadBB}

		g.currentBB = usagesBadBB
		g.rejectCryptoTask("SyntaxError", "HMAC keys only support sign and verify usages.")

		g.currentBB = usagesOKBB
		g.finishHMACGenerateKey(keyType, captured[1], captured[2], captured[3], captured[4])
	})
}

func (g *generator) finishHMACGenerateKey(keyType *types.ObjectType, hashName, requestedLength, extractable, usages ir.Operand) {
	sha1 := g.cryptoSHANameMatch(hashName, "1")
	sha256 := g.cryptoSHANameMatch(hashName, "256")
	sha384 := g.cryptoSHANameMatch(hashName, "384")
	sha512 := g.cryptoSHANameMatch(hashName, "512")
	sha1BB := g.currentFn.NewBlock("hmac_generate_sha1")
	check256BB := g.currentFn.NewBlock("hmac_generate_check_sha256")
	sha256BB := g.currentFn.NewBlock("hmac_generate_sha256")
	check384BB := g.currentFn.NewBlock("hmac_generate_check_sha384")
	sha384BB := g.currentFn.NewBlock("hmac_generate_sha384")
	check512BB := g.currentFn.NewBlock("hmac_generate_check_sha512")
	sha512BB := g.currentFn.NewBlock("hmac_generate_sha512")
	unsupportedBB := g.currentFn.NewBlock("hmac_generate_hash_unsupported")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: sha1, Then: sha1BB, Else: check256BB}

	g.currentBB = check256BB
	g.currentBB.Terminator = &ir.BranchTerm{Cond: sha256, Then: sha256BB, Else: check384BB}
	g.currentBB = check384BB
	g.currentBB.Terminator = &ir.BranchTerm{Cond: sha384, Then: sha384BB, Else: check512BB}
	g.currentBB = check512BB
	g.currentBB.Terminator = &ir.BranchTerm{Cond: sha512, Then: sha512BB, Else: unsupportedBB}

	for _, variant := range []struct {
		block       *ir.BasicBlock
		hash        string
		defaultBits float64
	}{
		{sha1BB, "SHA-1", 512},
		{sha256BB, "SHA-256", 512},
		{sha384BB, "SHA-384", 1024},
		{sha512BB, "SHA-512", 1024},
	} {
		g.currentBB = variant.block
		g.generateHMACKeyForHash(keyType, requestedLength, extractable, usages, variant.hash, variant.defaultBits)
	}

	g.currentBB = unsupportedBB
	g.rejectCryptoTask("NotSupportedError", "HMAC supports SHA-1, SHA-256, SHA-384, and SHA-512.")
}

func (g *generator) generateHMACKeyForHash(keyType *types.ObjectType, requestedLength, extractable, usages ir.Operand, hashName string, defaultBits float64) {
	hasRequested := g.currentFn.NewValue("hmac_generate_has_length", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: hasRequested, Op: ir.OpNe, LHS: requestedLength, RHS: ir.ConstUndefined{}})
	defaultBB := g.currentFn.NewBlock("hmac_generate_length_default")
	validateBB := g.currentFn.NewBlock("hmac_generate_length_validate")
	acceptedBB := g.currentFn.NewBlock("hmac_generate_length_accepted")
	invalidBB := g.currentFn.NewBlock("hmac_generate_length_invalid")
	lengthDoneBB := g.currentFn.NewBlock("hmac_generate_length_done")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: hasRequested, Then: validateBB, Else: defaultBB}
	defaultBB.Terminator = &ir.JumpTerm{Target: lengthDoneBB}

	g.currentBB = validateBB
	nonPositive := g.currentFn.NewValue("hmac_generate_length_non_positive", types.TypeBoolean)
	fraction := g.currentFn.NewValue("hmac_generate_length_fraction", types.TypeNumber)
	fractional := g.currentFn.NewValue("hmac_generate_length_fractional", types.TypeBoolean)
	nan := g.currentFn.NewValue("hmac_generate_length_nan", types.TypeBoolean)
	invalidA := g.currentFn.NewValue("hmac_generate_length_invalid_a", types.TypeBoolean)
	invalid := g.currentFn.NewValue("hmac_generate_length_invalid_any", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: nonPositive, Op: ir.OpLe, LHS: requestedLength, RHS: ir.ConstNumber{Value: 0}},
		&ir.BinaryInst{Res: fraction, Op: ir.OpMod, LHS: requestedLength, RHS: ir.ConstNumber{Value: 1}},
		&ir.BinaryInst{Res: fractional, Op: ir.OpNe, LHS: fraction, RHS: ir.ConstNumber{Value: 0}},
		&ir.BinaryInst{Res: nan, Op: ir.OpNe, LHS: requestedLength, RHS: requestedLength},
		&ir.BinaryInst{Res: invalidA, Op: ir.OpOr, LHS: nonPositive, RHS: fractional},
		&ir.BinaryInst{Res: invalid, Op: ir.OpOr, LHS: invalidA, RHS: nan},
	)
	g.currentBB.Terminator = &ir.BranchTerm{Cond: invalid, Then: invalidBB, Else: acceptedBB}

	g.currentBB = invalidBB
	g.rejectCryptoTask("OperationError", "HMAC key length must be a positive whole number of bits.")

	g.currentBB = acceptedBB
	g.currentBB.Terminator = &ir.JumpTerm{Target: lengthDoneBB}

	keyBits := g.currentFn.NewValue("hmac_generate_key_bits", types.TypeNumber)
	lengthDoneBB.Phis = append(lengthDoneBB.Phis, &ir.PhiInst{Res: keyBits, Incoming: []ir.PhiIncoming{
		{Block: defaultBB, Value: ir.ConstNumber{Value: defaultBits}},
		{Block: acceptedBB, Value: requestedLength},
	}})
	g.currentBB = lengthDoneBB

	padded := g.currentFn.NewValue("hmac_generate_padded_bits", types.TypeNumber)
	remainder := g.currentFn.NewValue("hmac_generate_padding_remainder", types.TypeNumber)
	alignedBits := g.currentFn.NewValue("hmac_generate_aligned_bits", types.TypeNumber)
	byteLength := g.currentFn.NewValue("hmac_generate_byte_length", types.TypeNumber)
	actualBits := g.currentFn.NewValue("hmac_generate_actual_bits", types.TypeNumber)
	dropBits := g.currentFn.NewValue("hmac_generate_drop_bits", types.TypeNumber)
	metadataByteLength := g.currentFn.NewValue("hmac_generate_metadata_bytes", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: padded, Op: ir.OpAdd, LHS: keyBits, RHS: ir.ConstNumber{Value: 7}},
		&ir.BinaryInst{Res: remainder, Op: ir.OpMod, LHS: padded, RHS: ir.ConstNumber{Value: 8}},
		&ir.BinaryInst{Res: alignedBits, Op: ir.OpSub, LHS: padded, RHS: remainder},
		&ir.BinaryInst{Res: byteLength, Op: ir.OpDiv, LHS: alignedBits, RHS: ir.ConstNumber{Value: 8}},
		&ir.BinaryInst{Res: actualBits, Op: ir.OpMul, LHS: byteLength, RHS: ir.ConstNumber{Value: 8}},
		&ir.BinaryInst{Res: dropBits, Op: ir.OpSub, LHS: actualBits, RHS: keyBits},
		&ir.BinaryInst{Res: metadataByteLength, Op: ir.OpDiv, LHS: keyBits, RHS: ir.ConstNumber{Value: 8}},
	)

	data := g.emitSecureRandomBytes(byteLength, "hmac_generate")
	generatedLength := g.currentFn.NewValue("hmac_generate_random_length", types.TypeNumber)
	generated := g.currentFn.NewValue("hmac_generate_random_ok", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.CallInst{Res: generatedLength, Callee: "ts_byte_buffer_len", Args: []ir.Operand{data}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}},
		&ir.BinaryInst{Res: generated, Op: ir.OpEq, LHS: generatedLength, RHS: byteLength},
	)
	generatedOKBB := g.currentFn.NewBlock("hmac_generate_random_ok")
	generatedBadBB := g.currentFn.NewBlock("hmac_generate_random_failed")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: generated, Then: generatedOKBB, Else: generatedBadBB}

	g.currentBB = generatedBadBB
	g.rejectCryptoTask("OperationError", "Unable to generate HMAC key material.")

	g.currentBB = generatedOKBB
	g.maskHMACImportTrailingBits(data, byteLength, dropBits)
	key := g.newHMACCryptoKey(keyType, data, extractable, usages, metadataByteLength, hashName)
	g.currentBB.Terminator = &ir.ReturnTerm{Val: key}
}
