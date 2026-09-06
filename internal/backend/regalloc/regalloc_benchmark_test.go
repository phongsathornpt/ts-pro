package regalloc

import (
	"fmt"
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func benchmarkLoopFunction(values, blocks int) *ir.Function {
	fn := ir.NewFunction("bench", types.TypeNumber)
	entry := fn.NewBlock("entry")
	live := make([]*ir.Value, 0, values)
	for i := 0; i < values; i++ {
		v := fn.NewValue(fmt.Sprintf("v%d", i), types.TypeNumber)
		entry.Instructions = append(entry.Instructions, &ir.BinaryInst{Res: v, Op: ir.OpAdd, LHS: ir.ConstNumber{Value: float64(i)}, RHS: ir.ConstNumber{Value: 1}})
		live = append(live, v)
	}
	header := fn.NewBlock("header")
	entry.Terminator = &ir.JumpTerm{Target: header}
	prev := header
	for i := 0; i < blocks; i++ {
		bb := fn.NewBlock(fmt.Sprintf("loop%d", i))
		prev.Terminator = &ir.JumpTerm{Target: bb}
		idx := i % len(live)
		out := fn.NewValue(fmt.Sprintf("u%d", i), types.TypeNumber)
		bb.Instructions = append(bb.Instructions, &ir.BinaryInst{Res: out, Op: ir.OpAdd, LHS: live[idx], RHS: ir.ConstNumber{Value: 1}})
		live[idx] = out
		prev = bb
	}
	prev.Terminator = &ir.JumpTerm{Target: header}
	header.Terminator = &ir.ReturnTerm{Val: live[0]}
	return fn
}

func BenchmarkAllocateLoopHeavy(b *testing.B) {
	for _, n := range []int{128, 512, 2048} {
		b.Run(fmt.Sprintf("values_%d", n), func(b *testing.B) {
			fn := benchmarkLoopFunction(n, 32)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ra := New(8)
				_ = ra.Allocate(fn)
			}
		})
	}
}
