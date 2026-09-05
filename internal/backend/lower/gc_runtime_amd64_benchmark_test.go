package lower

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/backend/obj/elf"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

const gcBenchmarkAllocations = 50000

func BenchmarkAMD64GCStringAllocationChurn(b *testing.B) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		b.Skip("native Linux AMD64 execution required")
	}

	path := buildAMD64GCChurnBenchmark(b, gcBenchmarkAllocations)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if out, err := exec.Command(path).CombinedOutput(); err != nil {
			b.Fatalf("execute: %v\n%s", err, out)
		}
	}
	b.ReportMetric(gcBenchmarkAllocations, "native-allocs/op")
}

func buildAMD64GCChurnBenchmark(tb testing.TB, iterations int) string {
	tb.Helper()
	prog := &ir.Program{}
	fn := ir.NewFunction("@main", types.TypeVoid)
	entry := fn.NewBlock("entry")
	cond := fn.NewBlock("cond")
	body := fn.NewBlock("body")
	post := fn.NewBlock("post")
	exit := fn.NewBlock("exit")

	i := fn.NewValue("i", types.TypeNumber)
	next := fn.NewValue("next", types.TypeNumber)
	cond.Phis = append(cond.Phis, &ir.PhiInst{Res: i, Incoming: []ir.PhiIncoming{
		{Block: entry, Value: ir.ConstNumber{Value: 0}},
		{Block: post, Value: next},
	}})
	entry.Terminator = &ir.JumpTerm{Target: cond}

	less := fn.NewValue("less", types.TypeBoolean)
	cond.Instructions = append(cond.Instructions, &ir.BinaryInst{
		Res: less, Op: ir.OpLt, LHS: i, RHS: ir.ConstNumber{Value: float64(iterations)},
	})
	cond.Terminator = &ir.BranchTerm{Cond: less, Then: body, Else: exit}

	tmp := fn.NewValue("tmp", types.TypeString)
	body.Instructions = append(body.Instructions, &ir.CallInst{
		Res: tmp, Callee: "ts_string_concat",
		Args: []ir.Operand{ir.ConstString{Value: "ab"}, ir.ConstString{Value: "cd"}},
	})
	body.Terminator = &ir.JumpTerm{Target: post}

	post.Instructions = append(post.Instructions, &ir.BinaryInst{
		Res: next, Op: ir.OpAdd, LHS: i, RHS: ir.ConstNumber{Value: 1},
	})
	post.Terminator = &ir.JumpTerm{Target: cond}
	exit.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, fn)

	code, err := lowerAMD64(prog)
	if err != nil {
		tb.Fatalf("lower: %v", err)
	}
	bin, err := elf.CreateExecutable(code, false)
	if err != nil {
		tb.Fatalf("elf: %v", err)
	}

	path := filepath.Join(tb.TempDir(), "gc-bench")
	if err := os.WriteFile(path, bin, 0o755); err != nil {
		tb.Fatalf("write executable: %v", err)
	}
	return path
}
