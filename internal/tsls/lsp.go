package tsls

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
)

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Diagnostic struct {
	Range    Range           `json:"range"`
	Severity int             `json:"severity,omitempty"`
	Code     json.RawMessage `json:"code,omitempty"`
	Source   string          `json:"source,omitempty"`
	Message  string          `json:"message"`
}

type DocumentDiagnosticReport struct {
	Kind  string       `json:"kind"`
	Items []Diagnostic `json:"items"`
}

func (c *Client) OpenDocument(path, text string) error {
	uri, err := c.fileURI(path)
	if err != nil {
		return err
	}
	params := map[string]any{
		"textDocument": map[string]any{
			"uri":        uri,
			"languageId": "typescript",
			"version":    1,
			"text":       text,
		},
	}
	return c.notify("textDocument/didOpen", params)
}

func (c *Client) CloseDocument(path string) error {
	uri, err := c.fileURI(path)
	if err != nil {
		return err
	}
	return c.notify("textDocument/didClose", map[string]any{
		"textDocument": map[string]any{"uri": uri},
	})
}

func (c *Client) Diagnostics(ctx context.Context, path string) (DocumentDiagnosticReport, error) {
	uri, err := c.fileURI(path)
	if err != nil {
		return DocumentDiagnosticReport{}, err
	}
	var report DocumentDiagnosticReport
	err = c.request(ctx, "textDocument/diagnostic", map[string]any{
		"textDocument": map[string]any{"uri": uri},
	}, &report)
	return report, err
}
func (c *Client) fileURI(path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(c.toolchain.Root, path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve source path: %w", err)
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}).String(), nil
}
