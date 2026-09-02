package frontend

import (
	"context"
	"fmt"

	"github.com/phongsathornpt/ts-pro/internal/tsls"
)

func CheckSource(ctx context.Context, client *tsls.Client, path, text string) ([]Diagnostic, error) {
	if err := client.OpenDocument(path, text); err != nil {
		return nil, fmt.Errorf("open TypeScript document: %w", err)
	}
	defer func() { _ = client.CloseDocument(path) }()

	report, err := client.Diagnostics(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("get TypeScript diagnostics: %w", err)
	}
	return NormalizeDiagnostics(report), nil
}
