package sema

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/frontend/parser"
	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

func TestPromiseWithoutTypeArgumentDoesNotPanic(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("promise.ts", []byte("function f(): Promise {}"))
	prog, parseDiags := parser.New(file).Parse()
	if parseDiags.HasErrors() {
		t.Fatalf("parser diagnostics: %s", parseDiags.Format(fs))
	}
	result := Check(prog)
	if result == nil {
		t.Fatal("semantic checker returned nil result")
	}
}
