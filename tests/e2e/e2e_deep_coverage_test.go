package e2e_test

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/backend/lower"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func TestE2EDeepLoweringAndTypes(t *testing.T) {
	// Exercise error branches and defensive paths in lower
	prog := &ir.Program{}
	fn := ir.NewFunction("errFn", types.TypeNumber)
	b := fn.NewBlock("entry")
	v0 := fn.NewValue("v0", types.TypeNumber)
	v1 := fn.NewValue("v1", types.TypeString)
	vObj := fn.NewValue("vObj", types.NewObject("Shape"))
	b.Instructions = append(b.Instructions,
		&ir.CallInst{Callee: "ts_print_literal", Args: []ir.Operand{ir.ConstString{Value: "hello"}}},
		&ir.GetFieldInst{Res: v0, Obj: vObj, Field: "nonexistent", Offset: 9999},
	)
	b.Terminator = &ir.ReturnTerm{Val: v0}
	prog.Functions = append(prog.Functions, fn)

	_, _ = lower.Lower(prog, lower.ArchAMD64)
	_, _ = lower.Lower(prog, lower.ArchARM64)

	// Types assignability and equality branches
	tVar := types.NewTypeVar("T", types.TypeNumber)
	uType := types.NewUnion(types.TypeNumber, types.TypeString)
	_ = tVar.AssignableTo(uType)
	_ = tVar.AssignableTo(types.TypeString)

	objA := types.NewObject("A")
	objA.AddField("a", types.TypeNumber, false)
	objB := types.NewObject("B")
	objB.AddField("a", types.TypeString, false)
	_ = objA.AssignableTo(objB)

	// Types substitute on function with this
	fnWithThis := &types.FunctionType{
		TypeParams: []*types.TypeVar{tVar},
		This:       tVar,
		Params:     []types.Param{{Name: "p", Type: tVar}},
		Return:     tVar,
	}
	_ = types.Substitute(fnWithThis, map[*types.TypeVar]types.Type{tVar: types.TypeNumber})

	// IR instructions
	vVoid := fn.NewValue("vVoid", types.TypeVoid)
	_ = vVoid.Type()
	_ = v1.Type()
}
