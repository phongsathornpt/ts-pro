package compiler

import (
	"time"

	escapeanalysis "github.com/projectthorn/tsv7-bin/internal/analysis/escape"
	rangeanalysis "github.com/projectthorn/tsv7-bin/internal/analysis/range"
	"github.com/projectthorn/tsv7-bin/internal/hir"
	"github.com/projectthorn/tsv7-bin/internal/mir"
)

type BuildTimings struct {
	TypeScript time.Duration
	HIR        time.Duration
	Repr       time.Duration
	MIR        time.Duration
	Escape     time.Duration
	LLVM       time.Duration
	Codegen    time.Duration
	Runtime    time.Duration
	Link       time.Duration
	Total      time.Duration
}

type BuildMetrics struct {
	Functions              int
	Shapes                 int
	Values                 int
	NativeValues           int
	DynamicValues          int
	BoxingSites            int
	DynamicDispatch        int
	I32Candidates          int
	I64Candidates          int
	I32FastOps             int
	I64FastOps             int
	TaskSpawns             int
	TaskJoins              int
	TaskRetains            int
	TaskReleases           int
	TaskYields             int
	ChannelCreates         int
	ChannelTrySends        int
	ChannelTryRecvs        int
	ChannelSends           int
	ChannelRecvs           int
	Sleeps                 int
	RuntimeCalls           int
	AllocationCandidates   int
	NonEscapingAllocations int
	EscapingAllocations    int
	StackObjectAllocs      int
	StackClosureAllocs     int
	ScalarObjectAllocs     int
	CacheHits              int
	CacheMisses            int
}

func collectBuildMetrics(hirModule hir.Module, mirModule mir.Module, escapes escapeanalysis.Result) BuildMetrics {
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
	metrics.TaskSpawns, metrics.TaskJoins, metrics.TaskRetains, metrics.TaskReleases, metrics.TaskYields = countTaskOps(mirModule)
	metrics.ChannelCreates, metrics.ChannelTrySends, metrics.ChannelTryRecvs, metrics.ChannelSends, metrics.ChannelRecvs = countChannelOps(mirModule)
	metrics.Sleeps = countSleepOps(mirModule)
	stackObjects := escapeanalysis.StackObjects(mirModule, escapes)
	scalarObjects := escapeanalysis.ScalarObjectsWithEscapeAnalysis(mirModule, stackObjects, escapes)
	metrics.RuntimeCalls = countRuntimeCallsWithLocalObjects(mirModule, stackObjects, scalarObjects)
	metrics.AllocationCandidates, metrics.NonEscapingAllocations, metrics.EscapingAllocations = countEscapeAllocations(escapes)
	metrics.ScalarObjectAllocs = countScalarObjects(scalarObjects)
	metrics.StackObjectAllocs, metrics.StackClosureAllocs = countPhysicalStackAllocations(mirModule, stackObjects, scalarObjects)
	return metrics
}

func countEscapeAllocations(result escapeanalysis.Result) (candidates, nonEscaping, escaping int) {
	for _, fn := range result {
		for _, info := range fn {
			candidates++
			if info.Escapes {
				escaping++
			} else {
				nonEscaping++
			}
		}
	}
	return candidates, nonEscaping, escaping
}

func countPhysicalStackAllocations(module mir.Module, stack escapeanalysis.StackObjectResult, scalar escapeanalysis.ScalarObjectResult) (objects, closures int) {
	for _, fn := range module.Functions {
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				if !stack.Contains(fn.ID, inst.Result) {
					continue
				}
				switch inst.Op.(type) {
				case mir.ObjectNew, mir.ObjectAlloc:
					if _, elided := scalar.Get(fn.ID, inst.Result); !elided {
						objects++
					}
				case mir.ClosureNew:
					closures++
				}
			}
		}
	}
	return objects, closures
}

func countScalarObjects(result escapeanalysis.ScalarObjectResult) int {
	count := 0
	for _, fn := range result {
		count += len(fn)
	}
	return count
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
				switch inst.Op.(type) {
				case mir.DispatchCall, mir.DynamicMethodCall:
					count++
				}
			}
		}
	}
	return count
}

func countTaskOps(module mir.Module) (spawns, joins, retains, releases, yields int) {
	for _, fn := range module.Functions {
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				switch inst.Op.(type) {
				case mir.TaskSpawn:
					spawns++
				case mir.TaskJoin:
					joins++
				case mir.TaskRetain:
					retains++
				case mir.TaskRelease:
					releases++
				case mir.TaskYield:
					yields++
				}
			}
		}
	}
	return spawns, joins, retains, releases, yields
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
	return countRuntimeCallsWithLocalObjects(module, nil, nil)
}

func countRuntimeCallsWithLocalObjects(module mir.Module, stackObjects escapeanalysis.StackObjectResult, scalarObjects escapeanalysis.ScalarObjectResult) int {
	count := 0
	for _, fn := range module.Functions {
		for _, block := range fn.Blocks {
			for _, inst := range block.Instructions {
				switch inst.Op.(type) {
				case mir.ObjectNew, mir.ObjectAlloc:
					if stackObjects.Contains(fn.ID, inst.Result) {
						continue
					}
					if _, ok := scalarObjects.Get(fn.ID, inst.Result); ok {
						continue
					}
					count++
				case mir.ClosureNew:
					if stackObjects.Contains(fn.ID, inst.Result) {
						continue
					}
					count++
				case mir.ConstString, mir.StringConcat, mir.ArrayNewF64, mir.ArrayLengthF64,
					mir.ArrayGetF64, mir.ArraySetF64, mir.ArrayNewBool, mir.ArrayLengthBool, mir.ArrayGetBool, mir.ArraySetBool, mir.ArrayNewRef, mir.ArrayLengthRef, mir.ArrayGetRef, mir.ArraySetRef,
					mir.BoxJSValue, mir.UnboxJSValue, mir.DynamicAddJSValue, mir.DynamicFieldGet, mir.DynamicFieldSet, mir.DynamicCall, mir.DynamicMethodCall, mir.DynamicBinaryJSValue, mir.IntrinsicCall,
					mir.PromiseResolve, mir.PromiseAdopt, mir.PromiseThenable, mir.PromiseReject, mir.PromiseAllF64, mir.PromiseRaceF64, mir.PromiseAllBool, mir.PromiseRaceBool, mir.PromiseAllRef, mir.PromiseRaceRef, mir.TaskSpawn, mir.TaskRetain, mir.TaskJoin, mir.TaskYield, mir.TaskCancel, mir.TaskCancelled, mir.TaskGroupNew, mir.TaskGroupJoin, mir.TaskGroupCancel, mir.TaskContextSet, mir.TaskContextGet, mir.ChannelNewF64, mir.ChannelTrySendF64, mir.ChannelTryRecvOrF64, mir.ChannelSendF64, mir.ChannelRecvF64, mir.ChannelNewBool, mir.ChannelTrySendBool, mir.ChannelTryRecvOrBool, mir.ChannelSendBool, mir.ChannelRecvBool, mir.ChannelNewRef, mir.ChannelTrySendRef, mir.ChannelTryRecvOrRef, mir.ChannelSendRef, mir.ChannelRecvRef, mir.Sleep:
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
