package compiler

import (
	"testing"

	escapeanalysis "github.com/projectthorn/tsv7-bin/internal/analysis/escape"
	"github.com/projectthorn/tsv7-bin/internal/mir"
)

func TestCountRuntimeCallsIncludesClosureAllocation(t *testing.T) {
	module := mir.Module{Functions: []mir.Function{{
		Blocks: []mir.Block{{Instructions: []mir.Instruction{
			{Op: mir.ClosureNew{Callee: 1}},
			{Op: mir.IntrinsicCall{Intrinsic: mir.IntrinsicConsoleLogF64}},
			{Op: mir.TaskSpawn{Callee: 1}},
			{Op: mir.TaskJoin{Task: 1}},
			{Op: mir.TaskYield{}},
		}}},
	}}}
	if got := countRuntimeCalls(module); got != 5 {
		t.Fatalf("runtime calls = %d, want 5", got)
	}
	spawns, joins, releases, yields := countTaskOps(module)
	if spawns != 1 || joins != 1 || releases != 0 || yields != 1 {
		t.Fatalf("task ops = %d/%d/%d, want 1/1/1", spawns, joins, yields)
	}
}

func TestCountEscapeAllocations(t *testing.T) {
	result := escapeanalysis.Result{
		0: {0: {Kind: escapeanalysis.AllocationObject}, 1: {Kind: escapeanalysis.AllocationClosure, Escapes: true, Reasons: escapeanalysis.ReasonReturn}},
	}
	candidates, nonEscaping, escaping := countEscapeAllocations(result)
	if candidates != 2 || nonEscaping != 1 || escaping != 1 {
		t.Fatalf("escape counts = %d/%d/%d, want 2/1/1", candidates, nonEscaping, escaping)
	}
}
