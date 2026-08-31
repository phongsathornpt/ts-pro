package tsls

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestTypeScriptAPISemanticQueries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := StartAPI("../..")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	if _, err := client.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	snapshot, err := client.UpdateSnapshot(ctx, "tsconfig.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.ReleaseSnapshot(context.Background(), snapshot.Snapshot) }()
	project := snapshot.Projects[0]
	file := filepath.Join(client.toolchain.Root, "examples", "fib.ts")

	data, err := client.GetSourceFile(ctx, snapshot.Snapshot, project.ID, file)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 44 {
		t.Fatalf("AST payload too small: %d", len(data))
	}

	symbol, err := client.GetSymbolAtPosition(ctx, snapshot.Snapshot, project.ID, file, 16)
	if err != nil {
		t.Fatal(err)
	}
	if symbol == nil || symbol.Name != "fib" {
		t.Fatalf("symbol = %+v", symbol)
	}

	typeInfo, err := client.GetTypeAtPosition(ctx, snapshot.Snapshot, project.ID, file, 20)
	if err != nil {
		t.Fatal(err)
	}
	if typeInfo == nil || typeInfo.ID == 0 {
		t.Fatalf("type = %+v", typeInfo)
	}
	typeText, err := client.TypeToString(ctx, snapshot.Snapshot, project.ID, typeInfo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if typeText != "number" {
		t.Fatalf("type string = %q", typeText)
	}

	files, err := client.GetSourceFileNames(ctx, snapshot.Snapshot, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("source file list is empty")
	}
}
