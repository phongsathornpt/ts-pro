package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) lowerHMACExportKey(e *ast.CallExpr) ir.Operand {
	format := g.lowerExpr(e.Args[0])
	key := g.lowerExpr(e.Args[1])
	taskType := g.semanticType(e).(*types.ObjectType)
	resultType := g.semaResult.ArrayBufferType
	captures := []ir.Operand{format, key}
	captureTypes := []types.Type{types.TypeString, key.Type()}

	return g.spawnCryptoTask("hmac_export", taskType, resultType, captures, captureTypes, func(captured []ir.Operand) {
		formatOK := g.cryptoStringEquals(captured[0], "raw", "hmac_export_raw")
		exportBB := g.currentFn.NewBlock("hmac_export_format_ok")
		unsupportedBB := g.currentFn.NewBlock("hmac_export_format_unsupported")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: formatOK, Then: exportBB, Else: unsupportedBB}

		g.currentBB = unsupportedBB
		g.rejectCryptoTask("NotSupportedError", "Only raw HMAC key export is supported.")

		g.currentBB = exportBB
		algorithm := g.cryptoKeyField(captured[1], "$algorithmName", types.TypeString)
		algorithmOK := g.cryptoStringEquals(algorithm, "HMAC", "hmac_export_algorithm")
		extractable := g.cryptoKeyField(captured[1], "extractable", types.TypeBoolean)
		allowed := g.currentFn.NewValue("hmac_export_allowed", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
			Res: allowed, Op: ir.OpAnd, LHS: algorithmOK, RHS: extractable,
		})
		allowedBB := g.currentFn.NewBlock("hmac_export_allowed")
		deniedBB := g.currentFn.NewBlock("hmac_export_denied")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: allowed, Then: allowedBB, Else: deniedBB}

		g.currentBB = deniedBB
		g.rejectCryptoTask("InvalidAccessError", "The CryptoKey is not extractable or is not an HMAC key.")

		g.currentBB = allowedBB
		data := g.cryptoKeyField(captured[1], "$data", g.semaResult.ByteBufferType)
		copy := g.copyByteBuffer(data)
		result := g.newArrayBufferFromData(copy)
		g.currentBB.Terminator = &ir.ReturnTerm{Val: result}
	})
}
