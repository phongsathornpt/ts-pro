package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/backend/lower"
	"github.com/phongsathornpt/ts-pro/internal/backend/obj/elf"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func TestLinuxAMD64BranchPhiEdges(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("requires linux/amd64 execution")
	}
	prog := &ir.Program{}
	fn := ir.NewFunction("@main", types.TypeVoid)
	entry := fn.NewBlock("entry")
	value := fn.NewValue("value", types.TypeNumber)
	cond := fn.NewValue("cond", types.TypeBoolean)
	entry.Instructions = append(entry.Instructions,
		&ir.BinaryInst{Res: value, Op: ir.OpAdd, LHS: ir.ConstNumber{Value: 40}, RHS: ir.ConstNumber{Value: 2}},
		&ir.BinaryInst{Res: cond, Op: ir.OpEq, LHS: ir.ConstNumber{Value: 1}, RHS: ir.ConstNumber{Value: 1}},
	)

	thenJoin := fn.NewBlock("then_join")
	dead := fn.NewBlock("dead")
	entry.Terminator = &ir.BranchTerm{Cond: cond, Then: thenJoin, Else: dead}

	thenValue := fn.NewValue("then_value", types.TypeNumber)
	thenJoin.Phis = append(thenJoin.Phis, &ir.PhiInst{
		Res:      thenValue,
		Incoming: []ir.PhiIncoming{{Block: entry, Value: value}},
	})
	thenJoin.Instructions = append(thenJoin.Instructions,
		&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{thenValue}, ParamTypes: []types.Type{types.TypeNumber}},
	)
	second := fn.NewBlock("second")
	thenJoin.Terminator = &ir.JumpTerm{Target: second}
	dead.Instructions = append(dead.Instructions,
		&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{ir.ConstNumber{Value: -1}}, ParamTypes: []types.Type{types.TypeNumber}},
	)
	dead.Terminator = &ir.ReturnTerm{}

	value2 := fn.NewValue("value2", types.TypeNumber)
	cond2 := fn.NewValue("cond2", types.TypeBoolean)
	second.Instructions = append(second.Instructions,
		&ir.BinaryInst{Res: value2, Op: ir.OpAdd, LHS: ir.ConstNumber{Value: 5}, RHS: ir.ConstNumber{Value: 2}},
		&ir.BinaryInst{Res: cond2, Op: ir.OpEq, LHS: ir.ConstNumber{Value: 1}, RHS: ir.ConstNumber{Value: 2}},
	)

	dead2 := fn.NewBlock("dead2")
	elseJoin := fn.NewBlock("else_join")
	second.Terminator = &ir.BranchTerm{Cond: cond2, Then: dead2, Else: elseJoin}
	dead2.Instructions = append(dead2.Instructions,
		&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{ir.ConstNumber{Value: -2}}, ParamTypes: []types.Type{types.TypeNumber}},
	)
	dead2.Terminator = &ir.ReturnTerm{}

	elseValue := fn.NewValue("else_value", types.TypeNumber)
	elseJoin.Phis = append(elseJoin.Phis, &ir.PhiInst{
		Res:      elseValue,
		Incoming: []ir.PhiIncoming{{Block: second, Value: value2}},
	})
	elseJoin.Instructions = append(elseJoin.Instructions,
		&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{elseValue}, ParamTypes: []types.Type{types.TypeNumber}},
	)
	elseJoin.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, fn)

	code, err := lower.Lower(prog, lower.ArchAMD64)
	if err != nil {
		t.Fatalf("lower branch phi IR: %v", err)
	}
	bin, err := elf.CreateExecutable(code, false)
	if err != nil {
		t.Fatalf("create branch phi ELF: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "branch-phi")
	if err := os.WriteFile(path, bin, 0o755); err != nil {
		t.Fatalf("write branch phi ELF: %v", err)
	}
	out, err := exec.Command(path).CombinedOutput()
	if err != nil {
		t.Fatalf("execute branch phi ELF: %v\nOutput:\n%s", err, out)
	}
	if got, want := string(out), "42\n7\n"; got != want {
		t.Fatalf("branch phi output = %q, want %q", got, want)
	}
}
