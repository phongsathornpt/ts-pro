package opt

import (
	"fmt"
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func benchmarkOptProgram(n int) *ir.Program {
	prog := &ir.Program{}
	fn := ir.NewFunction("bench", types.TypeNumber)
	bb := fn.NewBlock("entry")
	var prev ir.Operand = ir.ConstNumber{Value: 1}
	for i := 0; i < n; i++ {
		v := fn.NewValue(fmt.Sprintf("v%d", i), types.TypeNumber)
		rhs := ir.Operand(ir.ConstNumber{Value: 1})
		if i%7 == 0 {
			rhs = ir.ConstNumber{Value: float64(i + 1)}
		}
		bb.Instructions = append(bb.Instructions, &ir.BinaryInst{Res: v, Op: ir.OpAdd, LHS: prev, RHS: rhs})
		prev = v
	}
	bb.Terminator = &ir.ReturnTerm{Val: prev}
	prog.Functions = append(prog.Functions, fn)
	return prog
}

func BenchmarkOptimizeLinearIR(b *testing.B) {
	for _, n := range []int{128, 512, 2048} {
		b.Run(fmt.Sprintf("inst_%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				prog := benchmarkOptProgram(n)
				b.StartTimer()
				Optimize(prog, Options{Level: 2})
				b.StopTimer()
			}
		})
	}
}
