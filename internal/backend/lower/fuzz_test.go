package lower

import (
	"fmt"
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func FuzzLowerStructuredIR(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0})
	f.Add([]byte{0, 1, 2, 3, 4, 5, 255})
	f.Add([]byte("structured-ir-seed"))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 512 {
			data = data[:512]
		}

		prog := &ir.Program{}
		fn := ir.NewFunction("@main", types.TypeVoid)
		entry := fn.NewBlock("entry")
		var current ir.Operand = ir.ConstNumber{Value: 0}
		ops := []ir.BinaryOp{ir.OpAdd, ir.OpSub, ir.OpMul}

		for i, b := range data {
			result := fn.NewValue(fmt.Sprintf("n%d", i), types.TypeNumber)
			rhs := ir.ConstNumber{Value: float64(int8(b))}
			entry.Instructions = append(entry.Instructions, &ir.BinaryInst{
				Res: result,
				Op:  ops[int(b)%len(ops)],
				LHS: current,
				RHS: rhs,
			})
			current = result
		}

		cond := fn.NewValue("cond", types.TypeBoolean)
		entry.Instructions = append(entry.Instructions, &ir.BinaryInst{
			Res: cond,
			Op:  ir.OpGt,
			LHS: current,
			RHS: ir.ConstNumber{Value: 0},
		})
		thenBB := fn.NewBlock("then")
		elseBB := fn.NewBlock("else")
		mergeBB := fn.NewBlock("merge")
		entry.Terminator = &ir.BranchTerm{Cond: cond, Then: thenBB, Else: elseBB}

		thenValue := fn.NewValue("then_value", types.TypeNumber)
		thenBB.Instructions = append(thenBB.Instructions, &ir.BinaryInst{
			Res: thenValue, Op: ir.OpAdd, LHS: current, RHS: ir.ConstNumber{Value: 1},
		})
		thenBB.Terminator = &ir.JumpTerm{Target: mergeBB}

		elseValue := fn.NewValue("else_value", types.TypeNumber)
		elseBB.Instructions = append(elseBB.Instructions, &ir.BinaryInst{
			Res: elseValue, Op: ir.OpSub, LHS: current, RHS: ir.ConstNumber{Value: 1},
		})
		elseBB.Terminator = &ir.JumpTerm{Target: mergeBB}

		merged := fn.NewValue("merged", types.TypeNumber)
		mergeBB.Phis = append(mergeBB.Phis, &ir.PhiInst{
			Res: merged,
			Incoming: []ir.PhiIncoming{
				{Block: thenBB, Value: thenValue},
				{Block: elseBB, Value: elseValue},
			},
		})
		mergeBB.Terminator = &ir.ReturnTerm{}
		prog.Functions = append(prog.Functions, fn)

		for _, arch := range []Arch{ArchAMD64, ArchARM64} {
			code, err := Lower(prog, arch)
			if err != nil {
				t.Fatalf("lower %s: %v", arch, err)
			}
			if len(code) == 0 {
				t.Fatalf("lower %s returned empty machine code", arch)
			}
		}
	})
}
