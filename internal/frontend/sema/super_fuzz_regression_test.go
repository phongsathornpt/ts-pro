package sema

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/frontend/parser"
	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

func TestMissingBaseSuperDoesNotPanic(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("missing-base.ts", []byte(`
class Derived extends Missing {
  constructor() { super(); }
}
`))
	prog, parseDiags := parser.New(file).Parse()
	if parseDiags.HasErrors() {
		t.Fatalf("parser diagnostics: %s", parseDiags.Format(fs))
	}
	result := Check(prog)
	if !result.Diagnostics.HasErrors() {
		t.Fatal("expected missing base class diagnostic")
	}
}
