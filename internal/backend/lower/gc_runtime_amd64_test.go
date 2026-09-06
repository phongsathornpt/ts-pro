package lower

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/backend/obj/elf"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func TestAMD64GCReclaimsAndReusesStringBlocks(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("native Linux AMD64 execution required")
	}

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
		Res: less, Op: ir.OpLt, LHS: i, RHS: ir.ConstNumber{Value: 50000},
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

	for _, metric := range []string{"ts_gc_collections", "ts_gc_reclaimed", "ts_gc_mapped_bytes"} {
		value := fn.NewValue(metric, types.TypeNumber)
		exit.Instructions = append(exit.Instructions, &ir.CallInst{Res: value, Callee: metric})
		exit.Instructions = append(exit.Instructions, &ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{value}})
	}
	exit.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, fn)

	code, err := lowerAMD64(prog)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	bin, err := elf.CreateExecutable(code, false)
	if err != nil {
		t.Fatalf("elf: %v", err)
	}
	path := filepath.Join(t.TempDir(), "gc-reuse")
	if err := os.WriteFile(path, bin, 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	out, err := exec.Command(path).CombinedOutput()
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	lines := strings.Fields(string(out))
	if len(lines) != 3 {
		t.Fatalf("metrics output: got %q", out)
	}
	values := make([]int64, 3)
	for i, line := range lines {
		v, err := strconv.ParseInt(line, 10, 64)
		if err != nil {
			t.Fatalf("metric %q: %v", line, err)
		}
		values[i] = v
	}
	if values[0] < 1 {
		t.Fatalf("collections: got %d, want >= 1", values[0])
	}
	if values[1] <= 0 {
		t.Fatalf("reclaimed bytes: got %d, want > 0", values[1])
	}
	if values[2] != 1<<20 {
		t.Fatalf("mapped bytes: got %d, want %d; free-list reuse failed", values[2], 1<<20)
	}
}

func TestAMD64GCSplitsLargeReclaimedBlocksForSmallReuse(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("native Linux AMD64 execution required")
	}

	prog := &ir.Program{}
	fn := ir.NewFunction("@main", types.TypeVoid)
	entry := fn.NewBlock("entry")

	addChurnLoop := func(prefix string, start *ir.BasicBlock, iterations int, left, right string) *ir.BasicBlock {
		cond := fn.NewBlock(prefix + "_cond")
		body := fn.NewBlock(prefix + "_body")
		post := fn.NewBlock(prefix + "_post")
		exit := fn.NewBlock(prefix + "_exit")
		i := fn.NewValue(prefix+"_i", types.TypeNumber)
		next := fn.NewValue(prefix+"_next", types.TypeNumber)
		cond.Phis = append(cond.Phis, &ir.PhiInst{Res: i, Incoming: []ir.PhiIncoming{
			{Block: start, Value: ir.ConstNumber{Value: 0}},
			{Block: post, Value: next},
		}})
		start.Terminator = &ir.JumpTerm{Target: cond}
		less := fn.NewValue(prefix+"_less", types.TypeBoolean)
		cond.Instructions = append(cond.Instructions, &ir.BinaryInst{Res: less, Op: ir.OpLt, LHS: i, RHS: ir.ConstNumber{Value: float64(iterations)}})
		cond.Terminator = &ir.BranchTerm{Cond: less, Then: body, Else: exit}
		tmp := fn.NewValue(prefix+"_tmp", types.TypeString)
		body.Instructions = append(body.Instructions, &ir.CallInst{Res: tmp, Callee: "ts_string_concat", Args: []ir.Operand{ir.ConstString{Value: left}, ir.ConstString{Value: right}}})
		body.Terminator = &ir.JumpTerm{Target: post}
		post.Instructions = append(post.Instructions, &ir.BinaryInst{Res: next, Op: ir.OpAdd, LHS: i, RHS: ir.ConstNumber{Value: 1}})
		post.Terminator = &ir.JumpTerm{Target: cond}
		return exit
	}

	large := strings.Repeat("L", 4096)
	afterLarge := addChurnLoop("large", entry, 1024, large, large)
	afterSmall := addChurnLoop("small", afterLarge, 50000, "a", "b")
	for _, metric := range []string{"ts_gc_collections", "ts_gc_reclaimed", "ts_gc_mapped_bytes"} {
		value := fn.NewValue(metric+"_split", types.TypeNumber)
		afterSmall.Instructions = append(afterSmall.Instructions, &ir.CallInst{Res: value, Callee: metric})
		afterSmall.Instructions = append(afterSmall.Instructions, &ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{value}})
	}
	afterSmall.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, fn)

	code, err := lowerAMD64(prog)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	bin, err := elf.CreateExecutable(code, false)
	if err != nil {
		t.Fatalf("elf: %v", err)
	}
	path := filepath.Join(t.TempDir(), "gc-split-reuse")
	if err := os.WriteFile(path, bin, 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	out, err := exec.Command(path).CombinedOutput()
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	fields := strings.Fields(string(out))
	if len(fields) != 3 {
		t.Fatalf("metrics output: got %q", out)
	}
	collections, _ := strconv.ParseInt(fields[0], 10, 64)
	reclaimed, _ := strconv.ParseInt(fields[1], 10, 64)
	mapped, _ := strconv.ParseInt(fields[2], 10, 64)
	if collections < 2 {
		t.Fatalf("collections: got %d, want >= 2", collections)
	}
	if reclaimed <= 0 {
		t.Fatalf("reclaimed bytes: got %d, want > 0", reclaimed)
	}
	if mapped > 2<<20 {
		t.Fatalf("mapped bytes: got %d, want <= %d after split reuse", mapped, 2<<20)
	}
}

func TestAMD64GCClearsDeadRootsAcrossBlocks(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("native Linux AMD64 execution required")
	}
	prog := &ir.Program{}
	fn := ir.NewFunction("@main", types.TypeVoid)
	blocks := make([]*ir.BasicBlock, 180)
	for i := range blocks {
		blocks[i] = fn.NewBlock("root_churn")
	}
	payload := strings.Repeat("x", 4096)
	for i, bb := range blocks {
		value := fn.NewValue("dead_root", types.TypeString)
		bb.Instructions = append(bb.Instructions, &ir.CallInst{
			Res: value, Callee: "ts_string_concat",
			Args: []ir.Operand{ir.ConstString{Value: payload}, ir.ConstString{Value: payload}},
		})
		if i+1 < len(blocks) {
			bb.Terminator = &ir.JumpTerm{Target: blocks[i+1]}
		}
	}
	exit := blocks[len(blocks)-1]
	mapped := fn.NewValue("mapped", types.TypeNumber)
	exit.Instructions = append(exit.Instructions, &ir.CallInst{Res: mapped, Callee: "ts_gc_mapped_bytes"})
	exit.Instructions = append(exit.Instructions, &ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{mapped}})
	exit.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, fn)

	code, err := lowerAMD64(prog)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	bin, err := elf.CreateExecutable(code, false)
	if err != nil {
		t.Fatalf("elf: %v", err)
	}
	path := filepath.Join(t.TempDir(), "gc-dead-block-roots")
	if err := os.WriteFile(path, bin, 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	out, err := exec.Command(path).CombinedOutput()
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	mappedBytes, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		t.Fatalf("mapped bytes %q: %v", out, err)
	}
	if mappedBytes != 1<<20 {
		t.Fatalf("mapped bytes: got %d, want %d; dead roots crossed block boundaries", mappedBytes, 1<<20)
	}
}

func TestAMD64GCCoalescesAdjacentFreeBlocksForLargeReuse(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("native Linux AMD64 execution required")
	}
	prog := &ir.Program{}
	fn := ir.NewFunction("@main", types.TypeVoid)
	blocks := make([]*ir.BasicBlock, 1200)
	for i := range blocks {
		blocks[i] = fn.NewBlock("small_dead")
	}
	small := strings.Repeat("s", 512)
	for i, bb := range blocks {
		value := fn.NewValue("small", types.TypeString)
		bb.Instructions = append(bb.Instructions, &ir.CallInst{Res: value, Callee: "ts_string_concat", Args: []ir.Operand{ir.ConstString{Value: small}, ir.ConstString{Value: small}}})
		if i+1 < len(blocks) {
			bb.Terminator = &ir.JumpTerm{Target: blocks[i+1]}
		}
	}
	exit := blocks[len(blocks)-1]
	large := strings.Repeat("L", 32768)
	largeValue := fn.NewValue("large_reuse", types.TypeString)
	exit.Instructions = append(exit.Instructions, &ir.CallInst{Res: largeValue, Callee: "ts_string_concat", Args: []ir.Operand{ir.ConstString{Value: large}, ir.ConstString{Value: large}}})
	mapped := fn.NewValue("mapped_coalesce", types.TypeNumber)
	exit.Instructions = append(exit.Instructions, &ir.CallInst{Res: mapped, Callee: "ts_gc_mapped_bytes"})
	exit.Instructions = append(exit.Instructions, &ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{mapped}})
	exit.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, fn)

	code, err := lowerAMD64(prog)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	bin, err := elf.CreateExecutable(code, false)
	if err != nil {
		t.Fatalf("elf: %v", err)
	}
	path := filepath.Join(t.TempDir(), "gc-coalesce")
	if err := os.WriteFile(path, bin, 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	out, err := exec.Command(path).CombinedOutput()
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	mappedBytes, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		t.Fatalf("mapped bytes %q: %v", out, err)
	}
	if mappedBytes != 1<<20 {
		t.Fatalf("mapped bytes: got %d, want %d; adjacent free blocks were not coalesced", mappedBytes, 1<<20)
	}
}
