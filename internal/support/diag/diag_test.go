package diag

import (
	"strings"
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

func TestDiagnosticFormat(t *testing.T) {
	fs := source.NewFileSet()
	f := fs.AddFile("example.ts", []byte("function add(a: number, b: number) {\n    return a + c;\n}"))

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

	// List formatting & has errors
	list := DiagnosticList{d, Diagnostic{Severity: SeverityWarning, Message: "warn"}}
	if !list.HasErrors() {
		t.Errorf("expected HasErrors to be true")
	}
	_ = list.Format(fs)

	noErrList := DiagnosticList{Diagnostic{Severity: SeverityInfo, Message: "info"}}
	if noErrList.HasErrors() {
		t.Errorf("expected HasErrors to be false")
	}

	// Severities string
	for _, s := range []Severity{SeverityHint, SeverityInfo, SeverityWarning, SeverityError, Severity(99)} {
		_ = s.String()
	}

	// Format without fileset or invalid span
	dNoSpan := Diagnostic{Message: "plain message"}
	_ = dNoSpan.Format(nil)
}
