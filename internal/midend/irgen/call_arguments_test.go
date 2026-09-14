package irgen

import (
	"strings"
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/frontend/parser"
	"github.com/phongsathornpt/ts-pro/internal/frontend/sema"
	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

func TestVoidCallArgumentMaterializesUndefined(t *testing.T) {
	fs := source.NewFileSet()
	file := fs.AddFile("void-argument.ts", []byte(`
function sink(value: any): void {}
sink(console.log(1));
`))
	prog, parseDiags := parser.New(file).Parse()
	if parseDiags.HasErrors() {
		t.Fatalf("parser diagnostics: %s", parseDiags.Format(fs))
	}
	semaResult := sema.Check(prog)
	if semaResult.Diagnostics.HasErrors() {
		t.Fatalf("semantic diagnostics: %s", semaResult.Diagnostics.Format(fs))
	}
	irProg, err := Generate(prog, semaResult)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	dump := irProg.Dump()
	if !strings.Contains(dump, "call @sink(undefined)") {
		t.Fatalf("IR did not materialize undefined call argument:\n%s", dump)
	}
}
