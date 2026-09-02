package frontend

import (
	"encoding/json"
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/tsls"
)

func TestNormalizeDiagnostics(t *testing.T) {
	report := tsls.DocumentDiagnosticReport{
		Kind: "full",
		Items: []tsls.Diagnostic{{
			Range:    tsls.Range{Start: tsls.Position{Line: 2, Character: 4}},
			Severity: 1,
			Code:     json.RawMessage(`2322`),
			Source:   "typescript",
			Message:  "not assignable",
		}},
	}
	diagnostics := NormalizeDiagnostics(report)
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics = %d, want 1", len(diagnostics))
	}
	got := diagnostics[0]
	if got.Level != DiagnosticError || got.Code != "2322" || got.Line != 3 || got.Column != 5 {
		t.Fatalf("unexpected normalized diagnostic: %+v", got)
	}
}
