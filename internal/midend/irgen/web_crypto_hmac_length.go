package irgen

import (
	"fmt"

	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) resolveHMACImportLength(keyByteLength, requested ir.Operand) (ir.Operand, ir.Operand) {
	fullBits := g.currentFn.NewValue("hmac_import_full_bits", types.TypeNumber)
	hasRequested := g.currentFn.NewValue("hmac_import_has_length", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: fullBits, Op: ir.OpMul, LHS: keyByteLength, RHS: ir.ConstNumber{Value: 8}},
		&ir.BinaryInst{Res: hasRequested, Op: ir.OpNe, LHS: requested, RHS: ir.ConstUndefined{}},
	)

	entry := g.currentBB
	defaultBB := g.currentFn.NewBlock("hmac_import_length_default")
	validateBB := g.currentFn.NewBlock("hmac_import_length_validate")
	acceptedBB := g.currentFn.NewBlock("hmac_import_length_accepted")
	invalidBB := g.currentFn.NewBlock("hmac_import_length_invalid")
	doneBB := g.currentFn.NewBlock("hmac_import_length_done")
	entry.Terminator = &ir.BranchTerm{Cond: hasRequested, Then: validateBB, Else: defaultBB}
	defaultBB.Terminator = &ir.JumpTerm{Target: doneBB}

	g.currentBB = validateBB
	zero := g.currentFn.NewValue("hmac_import_length_zero", types.TypeBoolean)
	tooLong := g.currentFn.NewValue("hmac_import_length_too_long", types.TypeBoolean)
	minimum := g.currentFn.NewValue("hmac_import_length_minimum", types.TypeNumber)
	tooShort := g.currentFn.NewValue("hmac_import_length_too_short", types.TypeBoolean)
	fraction := g.currentFn.NewValue("hmac_import_length_fraction", types.TypeNumber)
	fractional := g.currentFn.NewValue("hmac_import_length_fractional", types.TypeBoolean)
	nan := g.currentFn.NewValue("hmac_import_length_nan", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: zero, Op: ir.OpEq, LHS: requested, RHS: ir.ConstNumber{Value: 0}},
		&ir.BinaryInst{Res: tooLong, Op: ir.OpGt, LHS: requested, RHS: fullBits},
		&ir.BinaryInst{Res: minimum, Op: ir.OpSub, LHS: fullBits, RHS: ir.ConstNumber{Value: 8}},
		&ir.BinaryInst{Res: tooShort, Op: ir.OpLe, LHS: requested, RHS: minimum},
		&ir.BinaryInst{Res: fraction, Op: ir.OpMod, LHS: requested, RHS: ir.ConstNumber{Value: 1}},
		&ir.BinaryInst{Res: fractional, Op: ir.OpNe, LHS: fraction, RHS: ir.ConstNumber{Value: 0}},
		&ir.BinaryInst{Res: nan, Op: ir.OpNe, LHS: requested, RHS: requested},
	)
	invalidA := g.currentFn.NewValue("hmac_import_length_invalid_a", types.TypeBoolean)
	invalidB := g.currentFn.NewValue("hmac_import_length_invalid_b", types.TypeBoolean)
	invalidC := g.currentFn.NewValue("hmac_import_length_invalid_c", types.TypeBoolean)
	invalid := g.currentFn.NewValue("hmac_import_length_invalid_any", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: invalidA, Op: ir.OpOr, LHS: zero, RHS: tooLong},
		&ir.BinaryInst{Res: invalidB, Op: ir.OpOr, LHS: tooShort, RHS: fractional},
		&ir.BinaryInst{Res: invalidC, Op: ir.OpOr, LHS: invalidA, RHS: invalidB},
		&ir.BinaryInst{Res: invalid, Op: ir.OpOr, LHS: invalidC, RHS: nan},
	)
	g.currentBB.Terminator = &ir.BranchTerm{Cond: invalid, Then: invalidBB, Else: acceptedBB}

	g.currentBB = invalidBB
	g.rejectCryptoTask("DataError", "HMAC length must select between one and eight bits from the final key byte.")

	g.currentBB = acceptedBB
	dropBits := g.currentFn.NewValue("hmac_import_drop_bits", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: dropBits, Op: ir.OpSub, LHS: fullBits, RHS: requested})
	g.currentBB.Terminator = &ir.JumpTerm{Target: doneBB}

	keyBits := g.currentFn.NewValue("hmac_import_key_bits", types.TypeNumber)
	drop := g.currentFn.NewValue("hmac_import_drop", types.TypeNumber)
	doneBB.Phis = append(doneBB.Phis,
		&ir.PhiInst{Res: keyBits, Incoming: []ir.PhiIncoming{{Block: defaultBB, Value: fullBits}, {Block: acceptedBB, Value: requested}}},
		&ir.PhiInst{Res: drop, Incoming: []ir.PhiIncoming{{Block: defaultBB, Value: ir.ConstNumber{Value: 0}}, {Block: acceptedBB, Value: dropBits}}},
	)
	g.currentBB = doneBB
	return keyBits, drop
}

func (g *generator) guardHMACPartialImport(hashName, keyByteLength, dropBits ir.Operand) {
	partial := g.currentFn.NewValue("hmac_import_partial", types.TypeBoolean)
	sha1 := g.cryptoSHANameMatch(hashName, "1")
	sha256 := g.cryptoSHANameMatch(hashName, "256")
	sha384 := g.cryptoSHANameMatch(hashName, "384")
	sha512 := g.cryptoSHANameMatch(hashName, "512")
	hash64 := g.currentFn.NewValue("hmac_import_hash_block64", types.TypeBoolean)
	hash128 := g.currentFn.NewValue("hmac_import_hash_block128", types.TypeBoolean)
	over64 := g.currentFn.NewValue("hmac_import_over_block64", types.TypeBoolean)
	over128 := g.currentFn.NewValue("hmac_import_over_block128", types.TypeBoolean)
	bad64a := g.currentFn.NewValue("hmac_import_partial_hash64", types.TypeBoolean)
	bad64 := g.currentFn.NewValue("hmac_import_partial_over64", types.TypeBoolean)
	bad128a := g.currentFn.NewValue("hmac_import_partial_hash128", types.TypeBoolean)
	bad128 := g.currentFn.NewValue("hmac_import_partial_over128", types.TypeBoolean)
	unsupported := g.currentFn.NewValue("hmac_import_partial_unsupported", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: partial, Op: ir.OpNe, LHS: dropBits, RHS: ir.ConstNumber{Value: 0}},
		&ir.BinaryInst{Res: hash64, Op: ir.OpOr, LHS: sha1, RHS: sha256},
		&ir.BinaryInst{Res: hash128, Op: ir.OpOr, LHS: sha384, RHS: sha512},
		&ir.BinaryInst{Res: over64, Op: ir.OpGt, LHS: keyByteLength, RHS: ir.ConstNumber{Value: 64}},
		&ir.BinaryInst{Res: over128, Op: ir.OpGt, LHS: keyByteLength, RHS: ir.ConstNumber{Value: 128}},
		&ir.BinaryInst{Res: bad64a, Op: ir.OpAnd, LHS: partial, RHS: hash64},
		&ir.BinaryInst{Res: bad64, Op: ir.OpAnd, LHS: bad64a, RHS: over64},
		&ir.BinaryInst{Res: bad128a, Op: ir.OpAnd, LHS: partial, RHS: hash128},
		&ir.BinaryInst{Res: bad128, Op: ir.OpAnd, LHS: bad128a, RHS: over128},
		&ir.BinaryInst{Res: unsupported, Op: ir.OpOr, LHS: bad64, RHS: bad128},
	)
	allowedBB := g.currentFn.NewBlock("hmac_import_partial_supported")
	unsupportedBB := g.currentFn.NewBlock("hmac_import_partial_long_key")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: unsupported, Then: unsupportedBB, Else: allowedBB}

	g.currentBB = unsupportedBB
	g.rejectCryptoTask("NotSupportedError", "Non-byte-aligned HMAC keys above the hash block size require bit-length SHA normalization.")
	g.currentBB = allowedBB
}

func (g *generator) maskHMACImportTrailingBits(data, keyByteLength, dropBits ir.Operand) {
	aligned := g.currentFn.NewValue("hmac_import_byte_aligned", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: aligned, Op: ir.OpEq, LHS: dropBits, RHS: ir.ConstNumber{Value: 0}})
	dispatchBB := g.currentFn.NewBlock("hmac_import_mask_dispatch")
	doneBB := g.currentFn.NewBlock("hmac_import_mask_done")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: aligned, Then: doneBB, Else: dispatchBB}

	g.currentBB = dispatchBB
	lastIndex := g.currentFn.NewValue("hmac_import_last_index", types.TypeNumber)
	lastByte := g.currentFn.NewValue("hmac_import_last_byte", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: lastIndex, Op: ir.OpSub, LHS: keyByteLength, RHS: ir.ConstNumber{Value: 1}},
		&ir.CallInst{Res: lastByte, Callee: "ts_byte_buffer_get", Args: []ir.Operand{data, lastIndex}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber}},
	)

	for dropped := 1; dropped <= 7; dropped++ {
		match := g.currentFn.NewValue(fmt.Sprintf("hmac_import_drop_%d", dropped), types.TypeBoolean)
		caseBB := g.currentFn.NewBlock(fmt.Sprintf("hmac_import_mask_%d", dropped))
		nextBB := g.currentFn.NewBlock(fmt.Sprintf("hmac_import_mask_next_%d", dropped))
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: match, Op: ir.OpEq, LHS: dropBits, RHS: ir.ConstNumber{Value: float64(dropped)}})
		g.currentBB.Terminator = &ir.BranchTerm{Cond: match, Then: caseBB, Else: nextBB}

		g.currentBB = caseBB
		divisor := float64(uint64(1) << uint(dropped))
		remainder := g.currentFn.NewValue(fmt.Sprintf("hmac_import_remainder_%d", dropped), types.TypeNumber)
		masked := g.currentFn.NewValue(fmt.Sprintf("hmac_import_masked_%d", dropped), types.TypeNumber)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.BinaryInst{Res: remainder, Op: ir.OpMod, LHS: lastByte, RHS: ir.ConstNumber{Value: divisor}},
			&ir.BinaryInst{Res: masked, Op: ir.OpSub, LHS: lastByte, RHS: remainder},
			&ir.CallInst{Callee: "ts_byte_buffer_set", Args: []ir.Operand{data, lastIndex, masked}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber}},
		)
		g.currentBB.Terminator = &ir.JumpTerm{Target: doneBB}
		g.currentBB = nextBB
	}

	g.rejectCryptoTask("DataError", "HMAC length produced an invalid partial-byte width.")
	g.currentBB = doneBB
}
