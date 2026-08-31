package tsast

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/projectthorn/tsv7-bin/internal/tsls"
)

func TestExactNodeHandleSemanticQueries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := tsls.StartAPI("../..")
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
	project := snapshot.Projects[0]
	fileName := filepath.Join(projectRoot(t), "examples", "fib.ts")
	payload, err := client.GetSourceFile(ctx, snapshot.Snapshot, project.ID, fileName)
	if err != nil {
		t.Fatal(err)
	}
	file, err := Decode(payload)
	if err != nil {
		t.Fatal(err)
	}
	fn := file.Root().Children()[0]
	params, _ := fn.NamedChild("parameters")
	param := params.ListElements()[0]
	name, _ := param.NamedChild("name")

	handle := name.Handle(fileName)
	symbol, err := client.GetSymbolAtLocation(ctx, snapshot.Snapshot, project.ID, handle)
	if err != nil {
		t.Fatal(err)
	}
	if symbol == nil || symbol.Name != "n" {
		t.Fatalf("symbol = %+v", symbol)
	}
	typeInfo, err := client.GetTypeAtLocation(ctx, snapshot.Snapshot, project.ID, handle)
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
}
