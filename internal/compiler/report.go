package compiler

import (
	"time"

	rangeanalysis "github.com/projectthorn/tsv7-bin/internal/analysis/range"
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
	I32Candidates   int
	I64Candidates   int
	I32FastOps      int
	I64FastOps      int
	TaskSpawns      int
	TaskJoins       int
	TaskYields      int
	ChannelCreates  int
	ChannelTrySends int
	ChannelTryRecvs int
	ChannelSends    int
	ChannelRecvs    int
	Sleeps          int
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
	ranges := rangeanalysis.Analyze(hirModule)
	for _, fn := range ranges {
		for _, interval := range fn {
			if interval.FitsI32() {
				metrics.I32Candidates++
			} else if interval.FitsI64() {
				metrics.I64Candidates++
			}
		}
	}
	metrics.I32FastOps, metrics.I64FastOps = countIntegerFastOps(mirModule)
	metrics.BoxingSites = countBoxingSites(mirModule)
	metrics.DynamicDispatch = countDynamicDispatch(mirModule)
	metrics.TaskSpawns, metrics.TaskJoins, metrics.TaskYields = countTaskOps(mirModule)
	metrics.ChannelCreates, metrics.ChannelTrySends, metrics.ChannelTryRecvs, metrics.ChannelSends, metrics.ChannelRecvs = countChannelOps(mirModule)
	metrics.Sleeps = countSleepOps(mirModule)
	metrics.RuntimeCalls = countRuntimeCalls(mirModule)
	return metrics
}

func countIntegerFastOps(module mir.Module) (i32, i64 int) {
	for _, fn := range module.Functions {
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				op, ok := inst.Op.(mir.ProvenIntBinary)
				if !ok {
					continue
				}
				if op.Width == mir.IntWidth32 {
					i32++
				} else if op.Width == mir.IntWidth64 {
					i64++
				}
			}
		}
	}
	return i32, i64
}

func countBoxingSites(module mir.Module) int {
	count := 0
	for _, fn := range module.Functions {
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				if _, ok := inst.Op.(mir.BoxJSValue); ok {
					count++
				}
			}
		}
	}
	return count
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

func countTaskOps(module mir.Module) (spawns, joins, yields int) {
	for _, fn := range module.Functions {
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				switch inst.Op.(type) {
				case mir.TaskSpawn:
					spawns++
				case mir.TaskJoin:
					joins++
				case mir.TaskYield:
					yields++
				}
			}
		}
	}
	return spawns, joins, yields
}

func countChannelOps(module mir.Module) (creates, trySends, tryRecvs, sends, recvs int) {
	for _, fn := range module.Functions {
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				switch inst.Op.(type) {
				case mir.ChannelNewF64, mir.ChannelNewBool, mir.ChannelNewRef:
					creates++
				case mir.ChannelTrySendF64, mir.ChannelTrySendBool, mir.ChannelTrySendRef:
					trySends++
				case mir.ChannelTryRecvOrF64, mir.ChannelTryRecvOrBool, mir.ChannelTryRecvOrRef:
					tryRecvs++
				case mir.ChannelSendF64, mir.ChannelSendBool, mir.ChannelSendRef:
					sends++
				case mir.ChannelRecvF64, mir.ChannelRecvBool, mir.ChannelRecvRef:
					recvs++
				}
			}
		}
	}
	return creates, trySends, tryRecvs, sends, recvs
}

func countSleepOps(module mir.Module) int {
	count := 0
	for _, fn := range module.Functions {
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				if _, ok := inst.Op.(mir.Sleep); ok {
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
					mir.ArrayGetF64, mir.ArraySetF64, mir.ObjectNew, mir.ObjectAlloc, mir.ClosureNew,
					mir.BoxJSValue, mir.UnboxJSValue, mir.DynamicAddJSValue, mir.DynamicBinaryJSValue, mir.IntrinsicCall,
					mir.TaskSpawn, mir.TaskJoin, mir.TaskYield, mir.ChannelNewF64, mir.ChannelTrySendF64, mir.ChannelTryRecvOrF64, mir.ChannelSendF64, mir.ChannelRecvF64, mir.ChannelNewBool, mir.ChannelTrySendBool, mir.ChannelTryRecvOrBool, mir.ChannelSendBool, mir.ChannelRecvBool, mir.ChannelNewRef, mir.ChannelTrySendRef, mir.ChannelTryRecvOrRef, mir.ChannelSendRef, mir.ChannelRecvRef, mir.Sleep:
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
