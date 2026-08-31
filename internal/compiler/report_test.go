package compiler

import (
	"testing"

	"github.com/projectthorn/tsv7-bin/internal/mir"
)

func TestCountRuntimeCallsIncludesClosureAllocation(t *testing.T) {
	module := mir.Module{Functions: []mir.Function{{
		Blocks: []mir.Block{{Instructions: []mir.Instruction{
			{Op: mir.ClosureNew{Callee: 1}},
			{Op: mir.IntrinsicCall{Intrinsic: mir.IntrinsicConsoleLogF64}},
		}}},
	}}}
	if got := countRuntimeCalls(module); got != 2 {
		t.Fatalf("runtime calls = %d, want 2", got)
	}
}
