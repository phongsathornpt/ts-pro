package irgen

import (
	"fmt"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) lowerWebCryptoCall(e *ast.CallExpr, member *ast.MemberExpr) (ir.Operand, bool) {
	if result, handled := g.lowerWebCryptoHMACCall(e, member); handled {
		return result, true
	}
	if ident, ok := member.Object.(*ast.IdentExpr); ok && ident.Name == "crypto" {
		switch member.Property {
		case "getRandomValues":
			return g.lowerCryptoGetRandomValues(e), true
		case "randomUUID":
			return g.lowerCryptoRandomUUID(), true
		default:
			return nil, false
		}
	}
	if subtle, ok := member.Object.(*ast.MemberExpr); ok && subtle.Property == "subtle" {
		ident, ok := subtle.Object.(*ast.IdentExpr)
		if ok && ident.Name == "crypto" {
			switch member.Property {
			case "digest":
				return g.lowerSubtleCryptoDigest(e), true
			case "generateKey":
				return g.lowerHMACGenerateKey(e), true
			case "exportKey":
				return g.lowerHMACExportKey(e), true
			}
		}
	}
	return nil, false
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

func (g *generator) lowerSubtleCryptoDigest(e *ast.CallExpr) ir.Operand {
	algorithm := g.lowerCryptoAlgorithmName(e.Args[0])
	data := g.lowerCryptoDigestInput(e.Args[1])
	arrayBufferType := g.semaResult.ArrayBufferType
	taskType, _ := g.semanticType(e).(*types.ObjectType)
	if taskType == nil {
		taskType = types.NewObject("$SubtleCryptoDigestTask")
	}

	outerFn, outerBB, outerLocals, outerProv, outerDirect := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	driverName := fmt.Sprintf("$crypto_digest%d", g.arrowCounter)
	g.arrowCounter++
	driverType := types.NewFunction(nil, arrayBufferType)
	driver := ir.NewFunction(driverName, arrayBufferType)
	g.currentFn = driver
	g.currentBB = driver.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)
	env := driver.NewValue("$env", driverType)
	driver.Params = append(driver.Params, env)
	capturedAlgorithm := driver.NewValue("digest_algorithm", types.TypeString)
	capturedData := driver.NewValue("digest_data", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.ClosureGetInst{Res: capturedAlgorithm, Closure: env, Index: 0},
		&ir.ClosureGetInst{Res: capturedData, Closure: env, Index: 1},
	)

	sha1 := g.cryptoSHANameMatch(capturedAlgorithm, "1")
	sha256 := g.cryptoSHANameMatch(capturedAlgorithm, "256")
	sha384 := g.cryptoSHANameMatch(capturedAlgorithm, "384")
	sha512 := g.cryptoSHANameMatch(capturedAlgorithm, "512")
	sha1BB := driver.NewBlock("digest_sha1")
	check256BB := driver.NewBlock("digest_check_sha256")
	sha256BB := driver.NewBlock("digest_sha256")
	check384BB := driver.NewBlock("digest_check_sha384")
	sha384BB := driver.NewBlock("digest_sha384")
	check512BB := driver.NewBlock("digest_check_sha512")
	sha512BB := driver.NewBlock("digest_sha512")
	unsupportedBB := driver.NewBlock("digest_unsupported")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: sha1, Then: sha1BB, Else: check256BB}

	g.currentBB = check256BB
	g.currentBB.Terminator = &ir.BranchTerm{Cond: sha256, Then: sha256BB, Else: check384BB}
	g.currentBB = check384BB
	g.currentBB.Terminator = &ir.BranchTerm{Cond: sha384, Then: sha384BB, Else: check512BB}
	g.currentBB = check512BB
	g.currentBB.Terminator = &ir.BranchTerm{Cond: sha512, Then: sha512BB, Else: unsupportedBB}

	g.currentBB = sha1BB
	g.lowerCryptoDigestHash(capturedData, "ts_crypto_sha1", "sha1")
	g.currentBB = sha256BB
	g.lowerCryptoDigestHash(capturedData, "ts_crypto_sha256", "sha256")
	g.currentBB = sha384BB
	g.lowerCryptoDigestHash(capturedData, "ts_crypto_sha384", "sha384")
	g.currentBB = sha512BB
	g.lowerCryptoDigestHash(capturedData, "ts_crypto_sha512", "sha512")

	g.currentBB = unsupportedBB
	err := g.newDOMException(
		ir.ConstString{Value: "Unrecognized digest algorithm."},
		ir.ConstString{Value: "NotSupportedError"},
	)
	boxedErr := g.boxJSValue(err, g.semaResult.DOMExceptionType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Callee: "ts_task_reject", Args: []ir.Operand{boxedErr}, ParamTypes: []types.Type{types.TypeAny},
	})
	g.currentBB.Terminator = &ir.ReturnTerm{}
	g.prog.Functions = append(g.prog.Functions, driver)

	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProv, outerDirect
	closure := g.currentFn.NewValue("crypto_digest_closure", driverType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{
		Res: closure, Function: driverName, Captures: []ir.Operand{algorithm, data}, RefMask: 3,
	})
	task := g.currentFn.NewValue("crypto_digest_task", taskType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: task, Callee: "ts_task_spawn", Args: []ir.Operand{closure, ir.ConstNumber{Value: nativeTaskResultKind(arrayBufferType)}},
	})
	return task
}

func (g *generator) lowerCryptoDigestHash(data ir.Operand, callee, name string) {
	digest := g.currentFn.NewValue("digest_"+name+"_bytes", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: digest, Callee: callee, Args: []ir.Operand{data}, ParamTypes: []types.Type{g.semaResult.ByteBufferType},
	})
	result := g.newArrayBufferFromData(digest)
	g.currentBB.Terminator = &ir.ReturnTerm{Val: result}
}

func (g *generator) lowerCryptoAlgorithmName(expr ast.Expr) ir.Operand {
	semanticType := g.semanticType(expr)
	if semanticType == types.TypeString {
		return g.lowerExpr(expr)
	}
	if objType, ok := semanticType.(*types.ObjectType); ok {
		value := g.lowerExpr(expr)
		field, exists := objType.Fields["name"]
		if exists {
			offsets, _, _ := g.objectLayout(objType)
			name := g.currentFn.NewValue("crypto_algorithm_name", field.Type)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{
				Res: name, Obj: value, Field: "name", Offset: offsets["name"],
			})
			return g.coerceStringType(field.Type, name)
		}
	}
	return ir.ConstString{Value: ""}
}

func (g *generator) cryptoSHANameMatch(value ir.Operand, bits string) ir.Operand {
	prefixes := []string{"SHA", "SHa", "ShA", "Sha", "sHA", "sHa", "shA", "sha"}
	var match ir.Operand
	for i, prefix := range prefixes {
		equal := g.currentFn.NewValue(fmt.Sprintf("digest_sha%s_match_%d", bits, i), types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: equal, Callee: "ts_string_eq", Args: []ir.Operand{value, ir.ConstString{Value: prefix + "-" + bits}}, ParamTypes: []types.Type{types.TypeString, types.TypeString},
		})
		if match == nil {
			match = equal
			continue
		}
		combined := g.currentFn.NewValue(fmt.Sprintf("digest_sha%s_match_any_%d", bits, i), types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
			Res: combined, Op: ir.OpOr, LHS: match, RHS: equal,
		})
		match = combined
	}
	return match
}

func (g *generator) lowerCryptoDigestInput(expr ast.Expr) ir.Operand {
	objType, _ := g.semanticType(expr).(*types.ObjectType)
	value := g.lowerExpr(expr)
	if objType != nil && objType.Name == "$ArrayBuffer" {
		return g.copyByteBuffer(g.arrayBufferData(value))
	}

	raw := g.uint8ArrayField(value, "$data", g.semaResult.ByteBufferType)
	offset := g.uint8ArrayField(value, "byteOffset", types.TypeNumber)
	length := g.uint8ArrayField(value, "byteLength", types.TypeNumber)
	end := g.currentFn.NewValue("crypto_digest_end", types.TypeNumber)
	copy := g.currentFn.NewValue("crypto_digest_input", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.BinaryInst{Res: end, Op: ir.OpAdd, LHS: offset, RHS: length},
		&ir.CallInst{Res: copy, Callee: "ts_byte_buffer_slice", Args: []ir.Operand{raw, offset, end}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber}},
	)
	return copy
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
