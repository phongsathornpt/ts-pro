package tsls

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestTypeScriptAPIInitializesSnapshot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := StartAPI("../..")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			t.Errorf("close API: %v", err)
		}
	}()
	init, err := client.Initialize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	root, _ := filepath.Abs("../..")
	if init.CurrentDirectory != root {
		t.Fatalf("cwd = %q, want %q", init.CurrentDirectory, root)
	}
	snapshot, err := client.UpdateSnapshot(ctx, "tsconfig.json")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Snapshot == 0 || len(snapshot.Projects) != 1 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	project := snapshot.Projects[0]
	if filepath.Base(project.ConfigFileName) != "tsconfig.json" {
		t.Fatalf("config = %q", project.ConfigFileName)
	}
	foundFib := false
	for _, file := range project.RootFiles {
		if filepath.Base(file) == "fib.ts" {
			foundFib = true
			break
		}
	}
	if !foundFib {
		t.Fatalf("fib.ts missing from root files: %v", project.RootFiles)
	}
	if strict, ok := project.CompilerOptions["strict"].(bool); !ok || !strict {
		t.Fatalf("strict option = %#v", project.CompilerOptions["strict"])
	}
	if err := client.ReleaseSnapshot(ctx, snapshot.Snapshot); err != nil {
		t.Fatal(err)
	}
}
