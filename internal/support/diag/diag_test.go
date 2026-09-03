package diag

import (
	"strings"
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

func TestDiagnosticFormat(t *testing.T) {
	fs := source.NewFileSet()
	f := fs.AddFile("example.ts", []byte("function add(a: number, b: number) {\n    return a + c;\n}"))

	// 'c' is at line 2, offset 48 (0-indexed: 36 bytes in line 1 + \n = 37. Line 2: "    return a + c;" -> 'c' is offset 37+15 = 52)
	// Let's find index of 'c'
	idx := strings.Index(string(f.Src), "c;")
	cPos := f.Base + source.Pos(idx)

	d := Diagnostic{
		Span:     source.Span{Start: cPos, End: cPos + 1},
		Code:     "TS2304",
		Message:  "Cannot find name 'c'.",
		Severity: SeverityError,
		Hints:    []string{"Did you mean 'b'?"},
	}

	formatted := d.Format(fs)
	if !strings.Contains(formatted, "error[TS2304]: Cannot find name 'c'.") {
		t.Errorf("expected header with code, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "^") {
		t.Errorf("expected caret pointer in:\n%s", formatted)
	}
	if !strings.Contains(formatted, "hint: Did you mean 'b'?") {
		t.Errorf("expected hint in:\n%s", formatted)
	}
}
