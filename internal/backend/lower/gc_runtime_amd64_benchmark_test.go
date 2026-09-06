package lower

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

func BenchmarkAMD64GCFragmentedFreeListReuse(b *testing.B) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		b.Skip("native Linux AMD64 execution required")
	}
	path := buildAMD64GCFragmentedReuseBenchmark(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if out, err := exec.Command(path).CombinedOutput(); err != nil {
			b.Fatalf("execute: %v\n%s", err, out)
		}
	}
	b.ReportMetric(10000, "target-allocs/op")
}

func buildAMD64GCFragmentedReuseBenchmark(tb testing.TB) string {
	tb.Helper()
	prog := &ir.Program{}
	fn := ir.NewFunction("@main", types.TypeVoid)
	entry := fn.NewBlock("entry")
	keepers := fn.NewValue("keepers", types.NewArray(types.TypeString))
	entry.Instructions = append(entry.Instructions, &ir.AllocArrayInst{Res: keepers, ElemType: types.TypeString, Length: ir.ConstNumber{Value: 0}})

	appendPhase := func(prefix string, start *ir.BasicBlock, iterations int, deadPayload string, keep bool) *ir.BasicBlock {
		cond := fn.NewBlock(prefix + "_cond")
		body := fn.NewBlock(prefix + "_body")
		post := fn.NewBlock(prefix + "_post")
		exit := fn.NewBlock(prefix + "_exit")
		i := fn.NewValue(prefix+"_i", types.TypeNumber)
		next := fn.NewValue(prefix+"_next", types.TypeNumber)
		cond.Phis = append(cond.Phis, &ir.PhiInst{Res: i, Incoming: []ir.PhiIncoming{{Block: start, Value: ir.ConstNumber{Value: 0}}, {Block: post, Value: next}}})
		start.Terminator = &ir.JumpTerm{Target: cond}
		less := fn.NewValue(prefix+"_less", types.TypeBoolean)
		cond.Instructions = append(cond.Instructions, &ir.BinaryInst{Res: less, Op: ir.OpLt, LHS: i, RHS: ir.ConstNumber{Value: float64(iterations)}})
		cond.Terminator = &ir.BranchTerm{Cond: less, Then: body, Else: exit}
		if keep {
			keeper := fn.NewValue(prefix+"_keeper", types.TypeString)
			body.Instructions = append(body.Instructions, &ir.CallInst{Res: keeper, Callee: "ts_string_concat", Args: []ir.Operand{ir.ConstString{Value: "k"}, ir.ConstString{Value: "v"}}})
			push := fn.NewValue(prefix+"_push", types.TypeNumber)
			body.Instructions = append(body.Instructions, &ir.ArrayPushInst{Res: push, Array: keepers, Val: keeper})
		}
		dead := fn.NewValue(prefix+"_dead", types.TypeString)
		body.Instructions = append(body.Instructions, &ir.CallInst{Res: dead, Callee: "ts_string_concat", Args: []ir.Operand{ir.ConstString{Value: deadPayload}, ir.ConstString{Value: "x"}}})
		body.Terminator = &ir.JumpTerm{Target: post}
		post.Instructions = append(post.Instructions, &ir.BinaryInst{Res: next, Op: ir.OpAdd, LHS: i, RHS: ir.ConstNumber{Value: 1}})
		post.Terminator = &ir.JumpTerm{Target: cond}
		return exit
	}

	largeExit := appendPhase("large", entry, 1200, strings.Repeat("L", 4096), true)
	smallExit := appendPhase("small", largeExit, 5000, strings.Repeat("s", 32), true)
	smallExit.Instructions = append(smallExit.Instructions, &ir.CallInst{Callee: "ts_gc_collect"})
	targetExit := appendPhase("target", smallExit, 10000, strings.Repeat("m", 768), false)
	targetExit.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, fn)

	code, err := lowerAMD64(prog)
	if err != nil {
		tb.Fatalf("lower: %v", err)
	}
	bin, err := elf.CreateExecutable(code, false)
	if err != nil {
		tb.Fatalf("elf: %v", err)
	}
	path := filepath.Join(tb.TempDir(), "gc-fragmented-reuse-bench")
	if err := os.WriteFile(path, bin, 0o755); err != nil {
		tb.Fatalf("write executable: %v", err)
	}
	return path
}

func BenchmarkAMD64GCMarkLocality(b *testing.B) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		b.Skip("native Linux AMD64 execution required")
	}
	const cycles = 20
	path := buildAMD64GCMarkLocalityBenchmark(b, 65521, cycles)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if out, err := exec.Command(path).CombinedOutput(); err != nil {
			b.Fatalf("execute: %v\n%s", err, out)
		}
	}
	b.ReportMetric(cycles, "gc-cycles/op")
}

func buildAMD64GCMarkLocalityBenchmark(tb testing.TB, objects, cycles int) string {
	tb.Helper()
	prog := &ir.Program{}
	fn := ir.NewFunction("@main", types.TypeVoid)
	entry := fn.NewBlock("entry")
	refs := fn.NewValue("refs", types.NewArray(types.TypeString))
	entry.Instructions = append(entry.Instructions, &ir.AllocArrayInst{Res: refs, ElemType: types.TypeString, Length: ir.ConstNumber{Value: float64(objects)}})

	cond := fn.NewBlock("fill_cond")
	body := fn.NewBlock("fill_body")
	post := fn.NewBlock("fill_post")
	afterFill := fn.NewBlock("after_fill")
	i := fn.NewValue("fill_i", types.TypeNumber)
	next := fn.NewValue("fill_next", types.TypeNumber)
	cond.Phis = append(cond.Phis, &ir.PhiInst{Res: i, Incoming: []ir.PhiIncoming{{Block: entry, Value: ir.ConstNumber{Value: 0}}, {Block: post, Value: next}}})
	entry.Terminator = &ir.JumpTerm{Target: cond}
	less := fn.NewValue("fill_less", types.TypeBoolean)
	cond.Instructions = append(cond.Instructions, &ir.BinaryInst{Res: less, Op: ir.OpLt, LHS: i, RHS: ir.ConstNumber{Value: float64(objects)}})
	cond.Terminator = &ir.BranchTerm{Cond: less, Then: body, Else: afterFill}
	value := fn.NewValue("live_string", types.TypeString)
	body.Instructions = append(body.Instructions, &ir.CallInst{Res: value, Callee: "ts_string_concat", Args: []ir.Operand{ir.ConstString{Value: "ab"}, ir.ConstString{Value: "cd"}}})
	mixed := fn.NewValue("mixed_index", types.TypeNumber)
	body.Instructions = append(body.Instructions, &ir.BinaryInst{Res: mixed, Op: ir.OpMul, LHS: i, RHS: ir.ConstNumber{Value: 32719}})
	index := fn.NewValue("index", types.TypeNumber)
	body.Instructions = append(body.Instructions, &ir.BinaryInst{Res: index, Op: ir.OpMod, LHS: mixed, RHS: ir.ConstNumber{Value: float64(objects)}})
	body.Instructions = append(body.Instructions, &ir.SetElementInst{Array: refs, Index: index, Val: value})
	body.Terminator = &ir.JumpTerm{Target: post}
	post.Instructions = append(post.Instructions, &ir.BinaryInst{Res: next, Op: ir.OpAdd, LHS: i, RHS: ir.ConstNumber{Value: 1}})
	post.Terminator = &ir.JumpTerm{Target: cond}

	gcCond := fn.NewBlock("gc_cond")
	gcBody := fn.NewBlock("gc_body")
	gcPost := fn.NewBlock("gc_post")
	exit := fn.NewBlock("exit")
	g := fn.NewValue("gc_i", types.TypeNumber)
	gNext := fn.NewValue("gc_next", types.TypeNumber)
	gcCond.Phis = append(gcCond.Phis, &ir.PhiInst{Res: g, Incoming: []ir.PhiIncoming{{Block: afterFill, Value: ir.ConstNumber{Value: 0}}, {Block: gcPost, Value: gNext}}})
	afterFill.Terminator = &ir.JumpTerm{Target: gcCond}
	gcLess := fn.NewValue("gc_less", types.TypeBoolean)
	gcCond.Instructions = append(gcCond.Instructions, &ir.BinaryInst{Res: gcLess, Op: ir.OpLt, LHS: g, RHS: ir.ConstNumber{Value: float64(cycles)}})
	gcCond.Terminator = &ir.BranchTerm{Cond: gcLess, Then: gcBody, Else: exit}
	gcBody.Instructions = append(gcBody.Instructions, &ir.CallInst{Callee: "ts_gc_collect"})
	gcBody.Terminator = &ir.JumpTerm{Target: gcPost}
	gcPost.Instructions = append(gcPost.Instructions, &ir.BinaryInst{Res: gNext, Op: ir.OpAdd, LHS: g, RHS: ir.ConstNumber{Value: 1}})
	gcPost.Terminator = &ir.JumpTerm{Target: gcCond}
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
	path := filepath.Join(tb.TempDir(), "gc-mark-locality-bench")
	if err := os.WriteFile(path, bin, 0o755); err != nil {
		tb.Fatalf("write executable: %v", err)
	}
	return path
}
