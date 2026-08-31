package lowering

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/projectthorn/tsv7-bin/internal/frontend"
	"github.com/projectthorn/tsv7-bin/internal/hir"
	"github.com/projectthorn/tsv7-bin/internal/tsls"
)

func TestLowerTypedFibToHIR(t *testing.T) {
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
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "examples", "fib.ts")
	semantic, err := frontend.ExtractFile(ctx, client, snapshot.Snapshot, project.ID, file)
	if err != nil {
		t.Fatal(err)
	}
	module, err := LowerHIR(semantic, "fib")
	if err != nil {
		t.Fatal(err)
	}
	if err := module.Verify(); err != nil {
		t.Fatal(err)
	}
	dump := hir.DumpText(module)
	t.Log("\n" + dump)
	for _, want := range []string{
		`func f0 @"fib"`,
		`branch`,
		`call f0(`,
		`return`,
	} {
		if !strings.Contains(dump, want) {
			t.Fatalf("HIR dump missing %q:\n%s", want, dump)
		}
	}
}
