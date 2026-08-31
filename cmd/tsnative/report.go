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
	fmt.Fprintf(&b, "  runtime calls: %d\n", m.RuntimeCalls)
	fmt.Fprintf(&b, "  timings:\n")
	writeTiming(&b, "typescript", t.TypeScript)
	writeTiming(&b, "hir", t.HIR)
	writeTiming(&b, "representation", t.Repr)
	writeTiming(&b, "mir", t.MIR)
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
