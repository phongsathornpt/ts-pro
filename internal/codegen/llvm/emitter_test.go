package llvm_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	repranalysis "github.com/projectthorn/tsv7-bin/internal/analysis/repr"
	llvmcodegen "github.com/projectthorn/tsv7-bin/internal/codegen/llvm"
	"github.com/projectthorn/tsv7-bin/internal/frontend"
	"github.com/projectthorn/tsv7-bin/internal/lowering"
	"github.com/projectthorn/tsv7-bin/internal/tsls"
)

func TestEmitFibLLVMAndCompileObject(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := tsls.StartAPI("../../..")
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
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	semantic, err := frontend.ExtractFile(ctx, client, snapshot.Snapshot, project.ID, filepath.Join(root, "examples", "fib.ts"))
	if err != nil {
		t.Fatal(err)
	}
	hirModule, err := lowering.LowerHIR(semantic, "fib")
	if err != nil {
		t.Fatal(err)
	}
	if diagnostics := repranalysis.Analyze(&hirModule); len(diagnostics) != 0 {
		t.Fatalf("representation diagnostics = %+v", diagnostics)
	}
	mirModule, err := lowering.LowerMIR(hirModule)
	if err != nil {
		t.Fatal(err)
	}
	text, err := llvmcodegen.Emit(mirModule)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"define double @tsnative_f0(double %v0)",
		"fcmp ole double",
		"call double @tsnative_f0",
		"fadd double",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("LLVM IR missing %q:\n%s", want, text)
		}
	}

	clang, err := exec.LookPath("clang")
	if err != nil {
		t.Skip("clang not installed")
	}
	dir := t.TempDir()
	ll := filepath.Join(dir, "fib.ll")
	obj := filepath.Join(dir, "fib.o")
	if err := os.WriteFile(ll, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(clang, "-c", ll, "-o", obj).CombinedOutput(); err != nil {
		t.Fatalf("clang failed: %v\n%s\nIR:\n%s", err, output, text)
	}
}
