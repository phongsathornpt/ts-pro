package tsls

import (
	"context"
	"testing"
	"time"
)

func TestDocumentDiagnosticsUsesConfiguredProject(t *testing.T) {
	client, err := Start("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close(ctx) }()

	path := "examples/virtual-diagnostic.ts"
	if err := client.OpenDocument(path, `const value: number = "bad";`); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.CloseDocument(path) }()

	report, err := client.Diagnostics(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Items) == 0 {
		t.Fatal("expected a TypeScript diagnostic")
	}
}
