package lowering

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	repranalysis "github.com/projectthorn/tsv7-bin/internal/analysis/repr"
	"github.com/projectthorn/tsv7-bin/internal/frontend"
	"github.com/projectthorn/tsv7-bin/internal/mir"
	"github.com/projectthorn/tsv7-bin/internal/tsls"
)

func TestLowerTypedFibToMIR(t *testing.T) {
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
	semantic, err := frontend.ExtractFile(ctx, client, snapshot.Snapshot, project.ID, filepath.Join(root, "examples", "fib.ts"))
	if err != nil {
		t.Fatal(err)
	}
	hirModule, err := LowerHIR(semantic, "fib")
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics := repranalysis.Analyze(&hirModule); len(diagnostics) != 0 {
		t.Fatalf("representation diagnostics = %+v", diagnostics)
	}
	module, err := LowerMIR(hirModule)
	if err != nil {
		t.Fatal(err)
	}
	if err := module.Verify(); err != nil {
		t.Fatal(err)
	}
	fn := module.Functions[0]
	if fn.ReturnRepr != mir.ReprF64 || fn.Params[0].Repr != mir.ReprF64 {
		t.Fatalf("function repr = %v params=%+v", fn.ReturnRepr, fn.Params)
	}
	if len(fn.Blocks) != 3 {
		t.Fatalf("blocks = %d", len(fn.Blocks))
	}
	if module.Entry == nil || len(module.Functions) != 2 {
		t.Fatalf("entry = %+v functions=%d", module.Entry, len(module.Functions))
	}
	entry := module.Functions[1]
	if entry.ID != *module.Entry || entry.ReturnRepr != mir.ReprVoid || len(entry.Params) != 0 {
		t.Fatalf("entry function = %+v", entry)
	}
	if len(entry.Blocks) != 1 || len(entry.Blocks[0].Instructions) != 3 {
		t.Fatalf("entry blocks = %+v", entry.Blocks)
	}
	if call, ok := entry.Blocks[0].Instructions[2].Op.(mir.IntrinsicCall); !ok || call.Intrinsic != mir.IntrinsicConsoleLogF64 {
		t.Fatalf("entry intrinsic = %+v", entry.Blocks[0].Instructions[2].Op)
	}
}
