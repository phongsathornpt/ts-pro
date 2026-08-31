package frontend

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/projectthorn/tsv7-bin/internal/tsls"
)

type DiagnosticLevel string

const (
	DiagnosticError   DiagnosticLevel = "error"
	DiagnosticWarning DiagnosticLevel = "warning"
	DiagnosticInfo    DiagnosticLevel = "info"
	DiagnosticHint    DiagnosticLevel = "hint"
)

type Diagnostic struct {
	Level   DiagnosticLevel
	Code    string
	Source  string
	Message string
	Line    int
	Column  int
}

func NormalizeDiagnostics(report tsls.DocumentDiagnosticReport) []Diagnostic {
	out := make([]Diagnostic, 0, len(report.Items))
	for _, item := range report.Items {
		out = append(out, Diagnostic{
			Level:   diagnosticLevel(item.Severity),
			Code:    diagnosticCode(item.Code),
			Source:  item.Source,
			Message: item.Message,
			Line:    item.Range.Start.Line + 1,
			Column:  item.Range.Start.Character + 1,
		})
	}
	return out
}

func diagnosticLevel(severity int) DiagnosticLevel {
	switch severity {
	case 1:
		return DiagnosticError
	case 2:
		return DiagnosticWarning
	case 3:
		return DiagnosticInfo
	default:
		return DiagnosticHint
	}
}

func diagnosticCode(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	text := strings.TrimSpace(string(raw))
	if strings.HasPrefix(text, "\"") {
		if value, err := strconv.Unquote(text); err == nil {
			return value
		}
	}
	return text
}

func (d Diagnostic) Render(path string) string {
	code := ""
	if d.Code != "" {
		code = " " + d.Code
	}
	return fmt.Sprintf("%s:%d:%d: %s%s: %s", path, d.Line, d.Column, d.Level, code, d.Message)
}

func HasErrors(diagnostics []Diagnostic) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Level == DiagnosticError {
			return true
		}
	}
	return false
}
