package irgen

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/frontend/parser"
	"github.com/phongsathornpt/ts-pro/internal/frontend/sema"
	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

func TestBuiltinIdentifierReturnsLoweringError(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("builtin.ts", []byte("console;"))
	prog, parseDiags := parser.New(file).Parse()
	if parseDiags.HasErrors() {
		t.Fatalf("parser diagnostics: %s", parseDiags.Format(fs))
	}
	semaResult := sema.Check(prog)
	if semaResult.Diagnostics.HasErrors() {
		t.Fatalf("sema diagnostics: %s", semaResult.Diagnostics.Format(fs))
	}
	if _, err := Generate(prog, semaResult); err == nil {
		t.Fatal("expected controlled lowering error for standalone builtin identifier")
	}
}
