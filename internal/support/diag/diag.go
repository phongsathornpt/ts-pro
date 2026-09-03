package diag

import (
	"fmt"
	"strings"

	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

type Severity uint8

const (
	SeverityHint Severity = iota
	SeverityInfo
	SeverityWarning
	SeverityError
)

func (s Severity) String() string {
	switch s {
	case SeverityHint:
		return "hint"
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityError:
		return "error"
	default:
		return "diagnostic"
	}
}

// Diagnostic represents a compiler error, warning, or information message.
type Diagnostic struct {
	Span     source.Span
	Code     string
	Message  string
	Severity Severity
	Hints    []string
}

// DiagnosticList is a collection of diagnostics.
type DiagnosticList []Diagnostic

// HasErrors reports whether any diagnostic in the list is an error.
func (dl DiagnosticList) HasErrors() bool {
	for _, d := range dl {
		if d.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Format formats the diagnostics for display, including source snippets and line carets if available.
func (dl DiagnosticList) Format(fs *source.FileSet) string {
	var sb strings.Builder
	for i, d := range dl {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(d.Format(fs))
	}
	return sb.String()
}

// Format formats a single diagnostic.
func (d Diagnostic) Format(fs *source.FileSet) string {
	var sb strings.Builder
	codeStr := ""
	if d.Code != "" {
		codeStr = fmt.Sprintf("[%s]", d.Code)
	}

	if fs == nil || !d.Span.IsValid() {
		sb.WriteString(fmt.Sprintf("%s%s: %s", d.Severity, codeStr, d.Message))
		return sb.String()
	}

	loc := fs.Location(d.Span.Start)
	file := fs.File(d.Span.Start)

	sb.WriteString(fmt.Sprintf("%s%s: %s\n", d.Severity, codeStr, d.Message))
	if loc.Filename != "" {
		sb.WriteString(fmt.Sprintf("  --> %s:%d:%d\n", loc.Filename, loc.Line, loc.Column))
	}

	if file != nil && loc.Line > 0 {
		lineStr := file.LineContent(loc.Line)
		linePrefix := fmt.Sprintf("%5d | ", loc.Line)
		sb.WriteString(linePrefix)
		sb.WriteString(lineStr)
		sb.WriteString("\n")

		// Visual caret
		caretIndent := strings.Repeat(" ", len(linePrefix)+loc.Column-1)
		caretLen := int(d.Span.End - d.Span.Start)
		if caretLen < 1 {
			caretLen = 1
		}
		if caretLen > len(lineStr)-loc.Column+1 && len(lineStr) >= loc.Column {
			caretLen = len(lineStr) - loc.Column + 1
		}
		if caretLen < 1 {
			caretLen = 1
		}
		sb.WriteString(caretIndent)
		sb.WriteString(strings.Repeat("^", caretLen))
	}

	for _, hint := range d.Hints {
		sb.WriteString("\n  = hint: ")
		sb.WriteString(hint)
	}

	return sb.String()
}
