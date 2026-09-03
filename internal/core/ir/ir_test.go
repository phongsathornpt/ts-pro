package ir

import (
	"strings"
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func TestIRDump(t *testing.T) {
	prog := &Program{}
	fn := NewFunction("add", types.TypeNumber)
	v0 := fn.NewValue("a", types.TypeNumber)
	v1 := fn.NewValue("b", types.TypeNumber)
	fn.Params = []*Value{v0, v1}

	entry := fn.NewBlock("entry")
	v2 := fn.NewValue("sum", types.TypeNumber)
	entry.Instructions = append(entry.Instructions, &BinaryInst{
		Res: v2,
		Op:  OpAdd,
		LHS: v0,
		RHS: v1,
	})
	entry.Terminator = &ReturnTerm{Val: v2}

	prog.Functions = append(prog.Functions, fn)

	dump := prog.Dump()
	if !strings.Contains(dump, "define @add(%a: number, %b: number): number") {
		t.Errorf("expected signature in dump, got:\n%s", dump)
	}
	if !strings.Contains(dump, "%sum = add %a, %b") {
		t.Errorf("expected add inst in dump, got:\n%s", dump)
	}
	if !strings.Contains(dump, "ret %sum") {
		t.Errorf("expected ret in dump, got:\n%s", dump)
	}
}
