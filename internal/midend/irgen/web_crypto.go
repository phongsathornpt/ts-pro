package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) lowerWebCryptoCall(e *ast.CallExpr, member *ast.MemberExpr) (ir.Operand, bool) {
	ident, ok := member.Object.(*ast.IdentExpr)
	if !ok || ident.Name != "crypto" {
		return nil, false
	}

	switch member.Property {
	case "getRandomValues":
		return g.lowerCryptoGetRandomValues(e), true
	case "randomUUID":
		return g.lowerCryptoRandomUUID(), true
	default:
		return nil, false
	}
}

func (g *generator) lowerCryptoGetRandomValues(e *ast.CallExpr) ir.Operand {
	view := g.lowerExpr(e.Args[0])
	data := g.uint8ArrayField(view, "$data", g.semaResult.ByteBufferType)
	offset := g.uint8ArrayField(view, "byteOffset", types.TypeNumber)
	length := g.uint8ArrayField(view, "length", types.TypeNumber)

	tooLarge := g.currentFn.NewValue("crypto_random_too_large", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: tooLarge, Op: ir.OpGt, LHS: length, RHS: ir.ConstNumber{Value: 65536},
	})
	quotaBB := g.currentFn.NewBlock("crypto_random_quota_exceeded")
	generateBB := g.currentFn.NewBlock("crypto_random_generate")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: tooLarge, Then: quotaBB, Else: generateBB}

	g.currentBB = quotaBB
	quotaErr := g.newDOMException(
		ir.ConstString{Value: "The requested length exceeds 65,536 bytes."},
		ir.ConstString{Value: "QuotaExceededError"},
	)
	g.routeThrownValue(g.boxJSValue(quotaErr, g.semaResult.DOMExceptionType))

	g.currentBB = generateBB
	random := g.emitSecureRandomBytes(length, "crypto_random")
	generated := g.currentFn.NewValue("crypto_random_generated", types.TypeBoolean)
	randomLength := g.currentFn.NewValue("crypto_random_length", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.CallInst{
			Res: randomLength, Callee: "ts_byte_buffer_len", Args: []ir.Operand{random}, ParamTypes: []types.Type{g.semaResult.ByteBufferType},
		},
		&ir.BinaryInst{Res: generated, Op: ir.OpEq, LHS: randomLength, RHS: length},
	)
	copyBB := g.currentFn.NewBlock("crypto_random_copy")
	failureBB := g.currentFn.NewBlock("crypto_random_failed")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: generated, Then: copyBB, Else: failureBB}

	g.currentBB = failureBB
	g.throwCryptoOperationError()

	g.currentBB = copyBB
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Callee:     "ts_byte_buffer_copy",
		Args:       []ir.Operand{data, random, offset, ir.ConstNumber{Value: 0}, length},
		ParamTypes: []types.Type{g.semaResult.ByteBufferType, g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber, types.TypeNumber},
	})
	return view
}

func (g *generator) lowerCryptoRandomUUID() ir.Operand {
	const uuidByteCount = 16

	random := g.emitSecureRandomBytes(ir.ConstNumber{Value: uuidByteCount}, "crypto_uuid")
	randomLength := g.currentFn.NewValue("crypto_uuid_length", types.TypeNumber)
	generated := g.currentFn.NewValue("crypto_uuid_generated", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.CallInst{
			Res: randomLength, Callee: "ts_byte_buffer_len", Args: []ir.Operand{random}, ParamTypes: []types.Type{g.semaResult.ByteBufferType},
		},
		&ir.BinaryInst{Res: generated, Op: ir.OpEq, LHS: randomLength, RHS: ir.ConstNumber{Value: uuidByteCount}},
	)
	formatBB := g.currentFn.NewBlock("crypto_uuid_format")
	failureBB := g.currentFn.NewBlock("crypto_uuid_failed")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: generated, Then: formatBB, Else: failureBB}

	g.currentBB = failureBB
	g.throwCryptoOperationError()

	g.currentBB = formatBB
	g.setUUIDVersionAndVariant(random)
	return g.formatUUID(random)
}

func (g *generator) emitSecureRandomBytes(length ir.Operand, prefix string) ir.Operand {
	random := g.currentFn.NewValue(prefix+"_bytes", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: random, Callee: "ts_os_random", Args: []ir.Operand{length}, ParamTypes: []types.Type{types.TypeNumber},
	})
	return random
}

func (g *generator) throwCryptoOperationError() {
	operationErr := g.newDOMException(
		ir.ConstString{Value: "Unable to generate secure random values."},
		ir.ConstString{Value: "OperationError"},
	)
	g.routeThrownValue(g.boxJSValue(operationErr, g.semaResult.DOMExceptionType))
}

func (g *generator) setUUIDVersionAndVariant(random ir.Operand) {
	versionByte := g.byteBufferNumber(random, 6, "crypto_uuid_version_source")
	versionLowNibble := g.currentFn.NewValue("crypto_uuid_version_low", types.TypeNumber)
	versionValue := g.currentFn.NewValue("crypto_uuid_version", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: versionLowNibble, Op: ir.OpMod, LHS: versionByte, RHS: ir.ConstNumber{Value: 16}},
		&ir.BinaryInst{Res: versionValue, Op: ir.OpAdd, LHS: versionLowNibble, RHS: ir.ConstNumber{Value: 64}},
		&ir.CallInst{
			Callee:     "ts_byte_buffer_set",
			Args:       []ir.Operand{random, ir.ConstNumber{Value: 6}, versionValue},
			ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber},
		},
	)

	variantByte := g.byteBufferNumber(random, 8, "crypto_uuid_variant_source")
	variantLowBits := g.currentFn.NewValue("crypto_uuid_variant_low", types.TypeNumber)
	variantValue := g.currentFn.NewValue("crypto_uuid_variant", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: variantLowBits, Op: ir.OpMod, LHS: variantByte, RHS: ir.ConstNumber{Value: 64}},
		&ir.BinaryInst{Res: variantValue, Op: ir.OpAdd, LHS: variantLowBits, RHS: ir.ConstNumber{Value: 128}},
		&ir.CallInst{
			Callee:     "ts_byte_buffer_set",
			Args:       []ir.Operand{random, ir.ConstNumber{Value: 8}, variantValue},
			ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber},
		},
	)
}

func (g *generator) formatUUID(random ir.Operand) ir.Operand {
	hexLookup := g.currentFn.NewValue("crypto_uuid_hex_lookup", g.semaResult.ByteBufferType)
	output := g.currentFn.NewValue("crypto_uuid_output", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.CallInst{
			Res: hexLookup, Callee: "ts_byte_buffer_from_utf8_string", Args: []ir.Operand{ir.ConstString{Value: "0123456789abcdef"}}, ParamTypes: []types.Type{types.TypeString},
		},
		&ir.CallInst{
			Res: output, Callee: "ts_byte_buffer_new", Args: []ir.Operand{ir.ConstNumber{Value: 36}}, ParamTypes: []types.Type{types.TypeNumber},
		},
	)

	outputIndex := 0
	for inputIndex := 0; inputIndex < 16; inputIndex++ {
		if inputIndex == 4 || inputIndex == 6 || inputIndex == 8 || inputIndex == 10 {
			g.setByteBufferNumber(output, outputIndex, ir.ConstNumber{Value: 45})
			outputIndex++
		}

		value := g.byteBufferNumber(random, inputIndex, "crypto_uuid_byte")
		low := g.currentFn.NewValue("crypto_uuid_low", types.TypeNumber)
		highBase := g.currentFn.NewValue("crypto_uuid_high_base", types.TypeNumber)
		high := g.currentFn.NewValue("crypto_uuid_high", types.TypeNumber)
		g.currentBB.Instructions = append(g.currentBB.Instructions,
			&ir.BinaryInst{Res: low, Op: ir.OpMod, LHS: value, RHS: ir.ConstNumber{Value: 16}},
			&ir.BinaryInst{Res: highBase, Op: ir.OpSub, LHS: value, RHS: low},
			&ir.BinaryInst{Res: high, Op: ir.OpDiv, LHS: highBase, RHS: ir.ConstNumber{Value: 16}},
		)

		highASCII := g.byteBufferNumberAt(hexLookup, high, "crypto_uuid_high_ascii")
		lowASCII := g.byteBufferNumberAt(hexLookup, low, "crypto_uuid_low_ascii")
		g.setByteBufferNumber(output, outputIndex, highASCII)
		g.setByteBufferNumber(output, outputIndex+1, lowASCII)
		outputIndex += 2
	}

	uuid := g.currentFn.NewValue("crypto_uuid_string", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: uuid, Callee: "ts_byte_buffer_to_utf8_string", Args: []ir.Operand{output}, ParamTypes: []types.Type{g.semaResult.ByteBufferType},
	})
	return uuid
}

func (g *generator) byteBufferNumber(buffer ir.Operand, index int, name string) ir.Operand {
	return g.byteBufferNumberAt(buffer, ir.ConstNumber{Value: float64(index)}, name)
}

func (g *generator) byteBufferNumberAt(buffer, index ir.Operand, name string) ir.Operand {
	value := g.currentFn.NewValue(name, types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: value, Callee: "ts_byte_buffer_get",
		Args:       []ir.Operand{buffer, index},
		ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber},
	})
	return value
}

func (g *generator) setByteBufferNumber(buffer ir.Operand, index int, value ir.Operand) {
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Callee:     "ts_byte_buffer_set",
		Args:       []ir.Operand{buffer, ir.ConstNumber{Value: float64(index)}, value},
		ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber},
	})
}
