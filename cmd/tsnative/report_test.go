package main

import (
	"strings"
	"testing"
	"time"

	"github.com/projectthorn/tsv7-bin/internal/compiler"
)

func TestFormatBuildReport(t *testing.T) {
	result := compiler.BuildResult{
		Metrics: compiler.BuildMetrics{Values: 10, NativeValues: 9, DynamicValues: 1, BoxingSites: 1, DynamicDispatch: 2, I32Candidates: 5, I64Candidates: 1, I32FastOps: 3, I64FastOps: 1, RuntimeCalls: 3, Shapes: 4},
		Timings: compiler.BuildTimings{TypeScript: 12 * time.Millisecond, HIR: 50 * time.Microsecond, Total: 20 * time.Millisecond},
	}
	text := formatBuildReport(result)
	for _, want := range []string{
		"coverage: 90.0% (9/10 values)",
		"shapes: 4",
		"boxing sites: 1",
		"dynamic dispatch: 2",
		"integer proof: 5 i32 / 1 i64-only candidates",
		"integer fast ops: 3 i32 / 1 i64",
		"runtime calls: 3",
		"typescript:    12ms",
		"hir:           50µs",
		"total:         20ms",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("report missing %q:\n%s", want, text)
		}
	}
}
