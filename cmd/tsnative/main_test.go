package main

import "testing"

func TestParseBuildArgs(t *testing.T) {
	options, err := parseBuildArgs([]string{"examples/fib.ts", "-o", "build/fib", "-O3", "-p", "tsconfig.json", "--report-performance"})
	if err != nil {
		t.Fatal(err)
	}
	if options.Input != "examples/fib.ts" || options.Output != "build/fib" {
		t.Fatalf("paths = %+v", options)
	}
	if options.Optimization != "-O3" || options.Config != "tsconfig.json" || !options.ReportPerformance {
		t.Fatalf("options = %+v", options)
	}
}

func TestParseBuildArgsRejectsMissingOutput(t *testing.T) {
	if _, err := parseBuildArgs([]string{"examples/fib.ts", "-o"}); err == nil {
		t.Fatal("expected missing output error")
	}
}
