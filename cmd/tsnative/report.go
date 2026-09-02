package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/projectthorn/tsv7-bin/internal/compiler"
)

func printBuildReport(result compiler.BuildResult) {
	fmt.Print(formatBuildReport(result))
}

func formatBuildReport(result compiler.BuildResult) string {
	var b strings.Builder
	m, t := result.Metrics, result.Timings
	fmt.Fprintf(&b, "Native performance report\n")
	fmt.Fprintf(&b, "  coverage: %.1f%% (%d/%d values)\n", m.NativeCoverage(), m.NativeValues, m.Values)
	fmt.Fprintf(&b, "  shapes: %d\n", m.Shapes)
	fmt.Fprintf(&b, "  boxing sites: %d\n", m.BoxingSites)
	fmt.Fprintf(&b, "  dynamic values: %d\n", m.DynamicValues)
	fmt.Fprintf(&b, "  dynamic dispatch: %d\n", m.DynamicDispatch)
	fmt.Fprintf(&b, "  integer proof: %d i32 / %d i64-only candidates\n", m.I32Candidates, m.I64Candidates)
	fmt.Fprintf(&b, "  integer fast ops: %d i32 / %d i64\n", m.I32FastOps, m.I64FastOps)
	fmt.Fprintf(&b, "  task ops: %d spawn / %d join / %d yield\n", m.TaskSpawns, m.TaskJoins, m.TaskYields)
	fmt.Fprintf(&b, "  channel ops: %d create / %d send / %d recv / %d try-send / %d try-recv\n", m.ChannelCreates, m.ChannelSends, m.ChannelRecvs, m.ChannelTrySends, m.ChannelTryRecvs)
	fmt.Fprintf(&b, "  timer ops: %d sleep\n", m.Sleeps)
	fmt.Fprintf(&b, "  runtime calls: %d\n", m.RuntimeCalls)
	fmt.Fprintf(&b, "  escape analysis: %d non-escaping / %d escaping / %d allocation candidates\n", m.NonEscapingAllocations, m.EscapingAllocations, m.AllocationCandidates)
	fmt.Fprintf(&b, "  object storage: %d scalar-replaced / %d stack-allocated\n", m.ScalarObjectAllocs, m.StackObjectAllocs)
	fmt.Fprintf(&b, "  closure storage: %d stack-allocated\n", m.StackClosureAllocs)
	fmt.Fprintf(&b, "  object cache: %.1f%% (%d hit / %d miss)\n", m.CacheHitRate(), m.CacheHits, m.CacheMisses)
	fmt.Fprintf(&b, "  timings:\n")
	writeTiming(&b, "typescript", t.TypeScript)
	writeTiming(&b, "hir", t.HIR)
	writeTiming(&b, "representation", t.Repr)
	writeTiming(&b, "mir", t.MIR)
	writeTiming(&b, "escape", t.Escape)
	writeTiming(&b, "llvm-ir", t.LLVM)
	writeTiming(&b, "codegen", t.Codegen)
	writeTiming(&b, "runtime", t.Runtime)
	writeTiming(&b, "link", t.Link)
	writeTiming(&b, "total", t.Total)
	return b.String()
}

func writeTiming(b *strings.Builder, name string, value time.Duration) {
	fmt.Fprintf(b, "    %-14s %s\n", name+":", formatDuration(value))
}

func formatDuration(value time.Duration) string {
	if value < time.Microsecond {
		return value.Round(time.Nanosecond).String()
	}
	if value < time.Millisecond {
		return value.Round(time.Microsecond).String()
	}
	return value.Round(time.Millisecond).String()
}
