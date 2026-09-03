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
