package compiler

import (
	"time"

	"github.com/projectthorn/tsv7-bin/internal/hir"
	"github.com/projectthorn/tsv7-bin/internal/mir"
)

type BuildTimings struct {
	TypeScript time.Duration
	HIR        time.Duration
	Repr       time.Duration
	MIR        time.Duration
	LLVM       time.Duration
	Codegen    time.Duration
	Runtime    time.Duration
	Link       time.Duration
	Total      time.Duration
}

type BuildMetrics struct {
	Functions       int
	Shapes          int
	Values          int
	NativeValues    int
	DynamicValues   int
	BoxingSites     int
	DynamicDispatch int
	RuntimeCalls    int
	CacheHits       int
	CacheMisses     int
}

func collectBuildMetrics(hirModule hir.Module, mirModule mir.Module) BuildMetrics {
	metrics := BuildMetrics{Functions: len(mirModule.Functions), Shapes: len(mirModule.Shapes)}
	count := func(repr hir.Repr) {
		if repr.Kind == hir.ReprVoid || repr.Kind == hir.ReprUnproven {
			return
		}
		metrics.Values++
		if repr.Kind == hir.ReprJSValue || repr.Kind == hir.ReprTaggedUnion {
			metrics.DynamicValues++
			metrics.BoxingSites++
		} else {
			metrics.NativeValues++
		}
	}
	for _, shape := range hirModule.Shapes {
		for _, field := range shape.Fields {
			count(field.Repr)
		}
	}
	for _, fn := range hirModule.Functions {
		for _, param := range fn.Params {
			count(param.Repr)
		}
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				count(inst.Repr)
			}
		}
	}
	metrics.DynamicDispatch = countDynamicDispatch(mirModule)
	metrics.RuntimeCalls = countRuntimeCalls(mirModule)
	return metrics
}

func countDynamicDispatch(module mir.Module) int {
	count := 0
	for _, fn := range module.Functions {
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				if _, ok := inst.Op.(mir.DispatchCall); ok {
					count++
				}
			}
		}
	}
	return count
}

func countRuntimeCalls(module mir.Module) int {
	count := 0
	for _, fn := range module.Functions {
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				switch inst.Op.(type) {
				case mir.ConstString, mir.StringConcat, mir.ArrayNewF64, mir.ArrayLengthF64,
					mir.ArrayGetF64, mir.ObjectNew, mir.ObjectAlloc, mir.ClosureNew, mir.IntrinsicCall:
					count++
				}
			}
		}
	}
	return count
}

func (m BuildMetrics) NativeCoverage() float64 {
	if m.Values == 0 {
		return 100
	}
	return float64(m.NativeValues) * 100 / float64(m.Values)
}

func (m BuildMetrics) CacheHitRate() float64 {
	total := m.CacheHits + m.CacheMisses
	if total == 0 {
		return 0
	}
	return float64(m.CacheHits) * 100 / float64(total)
}
