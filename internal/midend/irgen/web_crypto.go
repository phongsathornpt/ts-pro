package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) lowerWebCryptoCall(e *ast.CallExpr, member *ast.MemberExpr) (ir.Operand, bool) {
	ident, ok := member.Object.(*ast.IdentExpr)
	if !ok || ident.Name != "crypto" || member.Property != "getRandomValues" {
		return nil, false
	}

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
	random := g.currentFn.NewValue("crypto_random_bytes", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: random, Callee: "ts_os_random", Args: []ir.Operand{length}, ParamTypes: []types.Type{types.TypeNumber},
	})
	randomLength := g.currentFn.NewValue("crypto_random_length", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: randomLength, Callee: "ts_byte_buffer_len", Args: []ir.Operand{random}, ParamTypes: []types.Type{g.semaResult.ByteBufferType},
	})
	generated := g.currentFn.NewValue("crypto_random_generated", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: generated, Op: ir.OpEq, LHS: randomLength, RHS: length,
	})
	copyBB := g.currentFn.NewBlock("crypto_random_copy")
	failureBB := g.currentFn.NewBlock("crypto_random_failed")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: generated, Then: copyBB, Else: failureBB}

	g.currentBB = failureBB
	operationErr := g.newDOMException(
		ir.ConstString{Value: "Unable to generate secure random values."},
		ir.ConstString{Value: "OperationError"},
	)
	g.routeThrownValue(g.boxJSValue(operationErr, g.semaResult.DOMExceptionType))

	g.currentBB = copyBB
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Callee:     "ts_byte_buffer_copy",
		Args:       []ir.Operand{data, random, offset, ir.ConstNumber{Value: 0}, length},
		ParamTypes: []types.Type{g.semaResult.ByteBufferType, g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber, types.TypeNumber},
	})
	return view, true
}
