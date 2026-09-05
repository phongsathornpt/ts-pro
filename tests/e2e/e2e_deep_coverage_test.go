package e2e_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/backend/lower"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
	"github.com/phongsathornpt/ts-pro/pkg/tspro"
)

func TestE2EDeepLoweringAndTypes(t *testing.T) {
	// Exercise error branches and defensive paths in lower
	prog := &ir.Program{}
	fn := ir.NewFunction("errFn", types.TypeNumber)
	b := fn.NewBlock("entry")
	v0 := fn.NewValue("v0", types.TypeNumber)
	v1 := fn.NewValue("v1", types.TypeString)
	vObj := fn.NewValue("vObj", types.NewObject("Shape"))
	vBool := fn.NewValue("vBool", types.TypeBoolean)
	b.Instructions = append(b.Instructions,
		&ir.CallInst{Callee: "ts_print_literal", Args: []ir.Operand{ir.ConstString{Value: "hello"}}},
		&ir.CallInst{Callee: "ts_print_literal", Args: []ir.Operand{ir.ConstString{Value: ""}}},
		&ir.GetFieldInst{Res: v0, Obj: vObj, Field: "nonexistent", Offset: 9999},
		&ir.BinaryInst{Res: vBool, Op: ir.OpAnd, LHS: vBool, RHS: vBool},
		&ir.BinaryInst{Res: vBool, Op: ir.OpOr, LHS: vBool, RHS: vBool},
		&ir.BinaryInst{Res: vBool, Op: ir.OpNe, LHS: vBool, RHS: vBool},
	)
	b.Terminator = &ir.ReturnTerm{Val: v0}
	prog.Functions = append(prog.Functions, fn)

	_, _ = lower.Lower(prog, lower.ArchAMD64)
	_, _ = lower.Lower(prog, lower.ArchARM64)

	// Unsupported number op error
	progErrNum := &ir.Program{}
	fnErrNum := ir.NewFunction("errNum", types.TypeNumber)
	bErrNum := fnErrNum.NewBlock("entry")
	vNum := fnErrNum.NewValue("vNum", types.TypeNumber)
	bErrNum.Instructions = append(bErrNum.Instructions, &ir.BinaryInst{Res: vNum, Op: ir.BinaryOp(99), LHS: vNum, RHS: vNum})
	bErrNum.Terminator = &ir.ReturnTerm{Val: vNum}
	progErrNum.Functions = append(progErrNum.Functions, fnErrNum)
	_, _ = lower.Lower(progErrNum, lower.ArchAMD64)

	// Unsupported non-number op error
	progErrNonNum := &ir.Program{}
	fnErrNonNum := ir.NewFunction("errNonNum", types.TypeBoolean)
	bErrNonNum := fnErrNonNum.NewBlock("entry")
	vB := fnErrNonNum.NewValue("vB", types.TypeBoolean)
	bErrNonNum.Instructions = append(bErrNonNum.Instructions, &ir.BinaryInst{Res: vB, Op: ir.BinaryOp(99), LHS: vB, RHS: vB})
	bErrNonNum.Terminator = &ir.ReturnTerm{Val: vB}
	progErrNonNum.Functions = append(progErrNonNum.Functions, fnErrNonNum)
	_, _ = lower.Lower(progErrNonNum, lower.ArchAMD64)

	// Unsupported unary op error
	progErrUnary := &ir.Program{}
	fnErrUnary := ir.NewFunction("errUnary", types.TypeBoolean)
	bErrUnary := fnErrUnary.NewBlock("entry")
	vU := fnErrUnary.NewValue("vU", types.TypeBoolean)
	bErrUnary.Instructions = append(bErrUnary.Instructions, &ir.UnaryInst{Res: vU, Op: "!", Val: vU})
	bErrUnary.Terminator = &ir.ReturnTerm{Val: vU}
	progErrUnary.Functions = append(progErrUnary.Functions, fnErrUnary)
	_, _ = lower.Lower(progErrUnary, lower.ArchAMD64)

	// AMD64 constant conditional branch and phi incoming types
	progBrPhi := &ir.Program{}
	fnBrPhi := ir.NewFunction("brPhi", types.TypeNumber)
	bEntry := fnBrPhi.NewBlock("entry")
	bT := fnBrPhi.NewBlock("t")
	bF := fnBrPhi.NewBlock("f")
	vPhiRes := fnBrPhi.NewValue("phiRes", types.TypeNumber)
	bT.Phis = append(bT.Phis, &ir.PhiInst{Res: vPhiRes, Incoming: []ir.PhiIncoming{
		{Block: bEntry, Value: ir.ConstNumber{Value: 1.5}},
		{Block: bEntry, Value: ir.ConstBool{Value: true}},
		{Block: bF, Value: ir.ConstBool{Value: false}},
		{Block: bEntry, Value: ir.ConstUndefined{}},
		{Block: bF, Value: ir.ConstNull{}},
		{Block: bEntry, Value: ir.ConstString{Value: "s"}},
	}})
	bT.Terminator = &ir.ReturnTerm{Val: ir.ConstNumber{Value: 1}}
	bF.Terminator = &ir.JumpTerm{Target: bT}
	bEntry.Terminator = &ir.BranchTerm{Cond: ir.ConstNumber{Value: 10}, Then: bT, Else: bF}
	progBrPhi.Functions = append(progBrPhi.Functions, fnBrPhi)
	_, _ = lower.Lower(progBrPhi, lower.ArchAMD64)

	// Branch with const false and const 0
	progBrFalse := &ir.Program{}
	fnBrFalse := ir.NewFunction("brFalse", types.TypeVoid)
	bEntryF := fnBrFalse.NewBlock("entryF")
	bTF := fnBrFalse.NewBlock("tF")
	bFF := fnBrFalse.NewBlock("fF")
	bTF.Terminator = &ir.ReturnTerm{}
	bFF.Terminator = &ir.ReturnTerm{}
	bEntryF.Terminator = &ir.BranchTerm{Cond: ir.ConstBool{Value: false}, Then: bTF, Else: bFF}
	bTFF := fnBrFalse.NewBlock("tFF")
	bTFF.Terminator = &ir.ReturnTerm{}
	bTF.Terminator = &ir.BranchTerm{Cond: ir.ConstBool{Value: true}, Then: bTFF, Else: bFF}
	progBrFalse.Functions = append(progBrFalse.Functions, fnBrFalse)
	_, _ = lower.Lower(progBrFalse, lower.ArchAMD64)

	progBrZero := &ir.Program{}
	fnBrZero := ir.NewFunction("brZero", types.TypeVoid)
	bEntryZ := fnBrZero.NewBlock("entryZ")
	bTZ := fnBrZero.NewBlock("tZ")
	bFZ := fnBrZero.NewBlock("fZ")
	bTZZ := fnBrZero.NewBlock("tZZ")
	bTZZ.Terminator = &ir.ReturnTerm{}
	bTZ.Terminator = &ir.BranchTerm{Cond: ir.ConstNumber{Value: 5}, Then: bTZZ, Else: bFZ}
	bFZ.Terminator = &ir.ReturnTerm{}
	bEntryZ.Terminator = &ir.BranchTerm{Cond: ir.ConstNumber{Value: 0}, Then: bTZ, Else: bFZ}
	progBrZero.Functions = append(progBrZero.Functions, fnBrZero)
	_, _ = lower.Lower(progBrZero, lower.ArchAMD64)

	// IndirectCall and direct Call with many stack arguments (> 6 GPRs and > 8 XMMs)
	progManyArgs := &ir.Program{}
	fnCalleeMany := ir.NewFunction("calleeMany", types.TypeVoid)
	fnCalleeMany.Params = []*ir.Value{
		fnCalleeMany.NewValue("n1", types.TypeNumber),
		fnCalleeMany.NewValue("n2", types.TypeNumber),
		fnCalleeMany.NewValue("n3", types.TypeNumber),
		fnCalleeMany.NewValue("n4", types.TypeNumber),
		fnCalleeMany.NewValue("n5", types.TypeNumber),
		fnCalleeMany.NewValue("n6", types.TypeNumber),
		fnCalleeMany.NewValue("n7", types.TypeNumber),
		fnCalleeMany.NewValue("n8", types.TypeNumber),
		fnCalleeMany.NewValue("n9", types.TypeNumber),
		fnCalleeMany.NewValue("g1", types.TypeString),
		fnCalleeMany.NewValue("g2", types.TypeString),
		fnCalleeMany.NewValue("g3", types.TypeString),
		fnCalleeMany.NewValue("g4", types.TypeString),
		fnCalleeMany.NewValue("g5", types.TypeString),
		fnCalleeMany.NewValue("g6", types.TypeString),
		fnCalleeMany.NewValue("g7", types.TypeString),
	}
	bCallee := fnCalleeMany.NewBlock("entry")
	bCallee.Terminator = &ir.ReturnTerm{}
	progManyArgs.Functions = append(progManyArgs.Functions, fnCalleeMany)

	fnCallerMany := ir.NewFunction("callerMany", types.TypeVoid)
	bCaller := fnCallerMany.NewBlock("entry")
	var callArgs []ir.Operand
	for i := 0; i < 9; i++ {
		callArgs = append(callArgs, ir.ConstNumber{Value: float64(i)})
	}
	for i := 0; i < 7; i++ {
		callArgs = append(callArgs, ir.ConstString{Value: "s"})
	}
	vClosureDummy := fnCallerMany.NewValue("closureDummy", types.TypeAny)
	bCaller.Instructions = append(bCaller.Instructions,
		&ir.CallInst{Callee: "calleeMany", Args: callArgs},
		&ir.IndirectCallInst{Closure: vClosureDummy, Args: callArgs},
		&ir.IndirectCallInst{Closure: vClosureDummy, ThisArg: ir.ConstString{Value: "thisStr"}, Args: callArgs},
	)
	bCaller.Terminator = &ir.ReturnTerm{}
	progManyArgs.Functions = append(progManyArgs.Functions, fnCallerMany)
	_, _ = lower.Lower(progManyArgs, lower.ArchAMD64)

	// Unresolved closure and unresolved branch error paths
	progUnresolved := &ir.Program{}
	fnUnres := ir.NewFunction("unres", types.TypeVoid)
	bUnres := fnUnres.NewBlock("entry")
	vCl := fnUnres.NewValue("cl", types.TypeAny)
	bUnres.Instructions = append(bUnres.Instructions, &ir.MakeClosureInst{Res: vCl, Function: "nonexistent_func"})
	bUnres.Terminator = &ir.ReturnTerm{}
	progUnresolved.Functions = append(progUnresolved.Functions, fnUnres)
	_, _ = lower.Lower(progUnresolved, lower.ArchAMD64)
	_, _ = lower.Lower(progUnresolved, lower.ArchARM64)

	progUnresBB := &ir.Program{}
	fnUnresBB := ir.NewFunction("unresBB", types.TypeVoid)
	bUnresBB := fnUnresBB.NewBlock("entry")
	bUnresBB.Terminator = &ir.JumpTerm{Target: &ir.BasicBlock{Name: "missing_bb"}}
	progUnresBB.Functions = append(progUnresBB.Functions, fnUnresBB)
	_, _ = lower.Lower(progUnresBB, lower.ArchAMD64)
	_, _ = lower.Lower(progUnresBB, lower.ArchARM64)

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

func TestTypesRemainingEdgeCases(t *testing.T) {
	// 1. Any type AssignableTo and Equals
	_ = types.TypeAny.AssignableTo(types.TypeNumber)

	// 2. Tuple Equals mismatched elements and Tuple AssignableTo Union
	t1 := types.NewTuple(types.TypeNumber, types.TypeString)
	t2 := types.NewTuple(types.TypeNumber, types.TypeBoolean)
	_ = t1.Equals(t2)
	uTup := types.NewUnion(t1, types.TypeBoolean)
	_ = t1.AssignableTo(uTup)
	_ = t1.AssignableTo(types.TypeBoolean) // false branch

	// 3. Object AssignableTo non-union non-object
	obj := types.NewObject("O")
	_ = obj.AssignableTo(types.TypeNumber)

	// 4. FunctionType String with type params and this
	tvX := types.NewTypeVar("X", nil)
	fnGeneric := types.NewGenericFunction([]*types.TypeVar{tvX}, []types.Param{{Name: "x", Type: tvX}}, tvX)
	fnGeneric.This = types.TypeString
	_ = fnGeneric.String()

	// 5. FunctionType Equals branches
	fnA := types.NewFunction([]types.Param{{Name: "p", Type: types.TypeNumber}}, types.TypeString)
	fnB := types.NewFunction([]types.Param{{Name: "p", Type: types.TypeNumber}}, types.TypeString)
	fnB.This = types.TypeNumber
	_ = fnA.Equals(fnB)
	fnC := types.NewFunction([]types.Param{{Name: "p", Type: types.TypeNumber}}, types.TypeString)
	fnC.This = types.TypeString
	_ = fnB.Equals(fnC) // different this types

	fnGen1 := types.NewGenericFunction([]*types.TypeVar{tvX}, nil, types.TypeVoid)
	tvY := types.NewTypeVar("Y", nil)
	fnGen2 := types.NewGenericFunction([]*types.TypeVar{tvY}, nil, types.TypeVoid)
	_ = fnGen1.Equals(fnGen2) // different type params

	fnRest1 := types.NewFunction([]types.Param{{Name: "a", Type: types.TypeNumber, Rest: true}}, types.TypeVoid)
	fnRest2 := types.NewFunction([]types.Param{{Name: "a", Type: types.TypeNumber, Rest: false}}, types.TypeVoid)
	_ = fnRest1.Equals(fnRest2)

	// 6. FunctionType AssignableTo branches
	fnRetMismatch := types.NewFunction(nil, types.TypeString)
	fnRetTarget := types.NewFunction(nil, types.TypeNumber)
	_ = fnRetMismatch.AssignableTo(fnRetTarget) // return mismatch
	fnParamCountMismatch := types.NewFunction([]types.Param{{Name: "a", Type: types.TypeNumber}, {Name: "b", Type: types.TypeNumber}}, types.TypeVoid)
	fnTargetLessParams := types.NewFunction([]types.Param{{Name: "a", Type: types.TypeNumber}}, types.TypeVoid)
	_ = fnParamCountMismatch.AssignableTo(fnTargetLessParams)
	fnParamTypeMismatch := types.NewFunction([]types.Param{{Name: "a", Type: types.TypeNumber}}, types.TypeVoid)
	fnTargetMismatchParam := types.NewFunction([]types.Param{{Name: "a", Type: types.TypeString}}, types.TypeVoid)
	_ = fnParamTypeMismatch.AssignableTo(fnTargetMismatchParam)

	// 7. UnionType Equals and ContainsAssignable
	u1 := types.NewUnion(types.TypeNumber, types.TypeString)
	u2 := types.NewUnion(types.TypeNumber, types.TypeBoolean)
	_ = u1.Equals(u2)                      // len same, not equal
	_ = u1.AssignableTo(types.TypeBoolean) // false branch

	// 8. Substitute function with unbounded type params
	tvUnbound := types.NewTypeVar("Unbound", nil)
	fnPartial := types.NewGenericFunction([]*types.TypeVar{tvX, tvUnbound}, nil, tvX)
	_ = types.Substitute(fnPartial, map[*types.TypeVar]types.Type{tvX: types.TypeNumber})

	// 9. inferTypeBindings error branches
	_, _ = types.InferFunction(nil, nil)
	_ = types.Substitute(nil, nil)

	// Array pattern with non-array actual
	tvElem := types.NewTypeVar("Elem", nil)
	fnArrPat := types.NewGenericFunction([]*types.TypeVar{tvElem}, []types.Param{{Name: "a", Type: types.NewArray(tvElem)}}, tvElem)
	_, _ = types.InferFunction(fnArrPat, []types.Type{types.TypeNumber}) // non-array actual

	// Tuple pattern with non-tuple or wrong length actual
	fnTupPat := types.NewGenericFunction([]*types.TypeVar{tvElem}, []types.Param{{Name: "t", Type: types.NewTuple(tvElem, tvElem)}}, tvElem)
	_, _ = types.InferFunction(fnTupPat, []types.Type{types.TypeNumber})                 // non-tuple actual
	_, _ = types.InferFunction(fnTupPat, []types.Type{types.NewTuple(types.TypeNumber)}) // length mismatch

	// Function pattern with non-function or wrong param count actual
	fnFnPat := types.NewGenericFunction([]*types.TypeVar{tvElem}, []types.Param{{Name: "f", Type: types.NewFunction([]types.Param{{Name: "x", Type: tvElem}}, types.TypeVoid)}}, tvElem)
	_, _ = types.InferFunction(fnFnPat, []types.Type{types.TypeNumber})                       // non-function
	_, _ = types.InferFunction(fnFnPat, []types.Type{types.NewFunction(nil, types.TypeVoid)}) // param count mismatch

	// Object pattern with non-object actual
	objPat := types.NewObject("PatObj")
	objPat.AddField("f", tvElem, false)
	fnObjPat := types.NewGenericFunction([]*types.TypeVar{tvElem}, []types.Param{{Name: "o", Type: objPat}}, tvElem)
	_, _ = types.InferFunction(fnObjPat, []types.Type{types.TypeNumber}) // non-object
	objArgMissing := types.NewObject("Missing")
	_, _ = types.InferFunction(fnObjPat, []types.Type{objArgMissing}) // missing field

	// InferFunction when params > actualArgs
	fnTwoParams := types.NewGenericFunction([]*types.TypeVar{tvElem}, []types.Param{{Name: "a", Type: tvElem}, {Name: "b", Type: types.TypeNumber}}, tvElem)
	_, _ = types.InferFunction(fnTwoParams, []types.Type{types.TypeNumber})

	// FunctionBindings error paths
	_, _ = types.FunctionBindings(fnGeneric, fnA) // type params len mismatch
	fnNoTypeParams := types.NewFunction(nil, types.TypeVoid)
	fnWithTypeParams := types.NewGenericFunction([]*types.TypeVar{tvX}, nil, types.TypeVoid)
	_, _ = types.FunctionBindings(fnWithTypeParams, fnNoTypeParams)
	_, _ = types.FunctionBindings(fnGeneric, types.NewGenericFunction([]*types.TypeVar{tvX}, []types.Param{{Name: "x", Type: types.TypeNumber}}, types.TypeNumber))

	// 10. TypeNever AssignableTo
	_ = types.TypeNever.AssignableTo(types.TypeNumber)

	// 11. Tuple AssignableTo Tuple length mismatch & Tuple AssignableTo Array matching
	tTup2 := types.NewTuple(types.TypeNumber, types.TypeString)
	tTup3 := types.NewTuple(types.TypeNumber, types.TypeString, types.TypeBoolean)
	_ = tTup2.AssignableTo(tTup3)
	tTupNum := types.NewTuple(types.TypeNumber, types.TypeNumber)
	arrNum := types.NewArray(types.TypeNumber)
	_ = tTupNum.AssignableTo(arrNum)

	// 12. Object String with multiple fields (; delimiter)
	objMulti := types.NewObject("")
	objMulti.AddField("f1", types.TypeNumber, false)
	objMulti.AddField("f2", types.TypeString, false)
	_ = objMulti.String()

	// 13. UnionType ContainsAssignable false branch
	uNumStr := types.NewUnion(types.TypeNumber, types.TypeString).(*types.UnionType)
	_ = uNumStr.ContainsAssignable(types.TypeBoolean)

	// 14. Substitute unbound TypeVar
	tvUnboundVar := types.NewTypeVar("UnboundVar", nil)
	tvBoundVar := types.NewTypeVar("BoundVar", nil)
	_ = types.Substitute(tvUnboundVar, map[*types.TypeVar]types.Type{tvBoundVar: types.TypeNumber})

	// 15. InstantiateFunction nil argument
	tvInst := types.NewTypeVar("T", nil)
	fnToInst := types.NewGenericFunction([]*types.TypeVar{tvInst}, []types.Param{{Name: "x", Type: tvInst}}, tvInst)
	_, _ = types.InstantiateFunction(fnToInst, []types.Type{nil})

	// 15b. InferFunction with nil param pattern
	fnWithNilParam := types.NewGenericFunction([]*types.TypeVar{tvInst}, []types.Param{{Name: "x", Type: nil}}, tvInst)
	_, _ = types.InferFunction(fnWithNilParam, []types.Type{types.TypeNumber})

	// 16. InferTypeBindings constraint failure & nested failures
	tvConstrained := types.NewTypeVar("T", types.TypeNumber)
	fnConstrained := types.NewGenericFunction([]*types.TypeVar{tvConstrained}, []types.Param{{Name: "x", Type: tvConstrained}}, tvConstrained)
	_, _ = types.InferFunction(fnConstrained, []types.Type{types.TypeString})

	// Tuple constraint failure in inferTypeBindings
	fnTupConstrained := types.NewGenericFunction([]*types.TypeVar{tvConstrained}, []types.Param{{Name: "t", Type: types.NewTuple(tvConstrained)}}, tvConstrained)
	_, _ = types.InferFunction(fnTupConstrained, []types.Type{types.NewTuple(types.TypeString)})

	// Function parameter constraint failure in inferTypeBindings
	fnParamConstrained := types.NewGenericFunction([]*types.TypeVar{tvConstrained}, []types.Param{{Name: "f", Type: types.NewFunction([]types.Param{{Name: "p", Type: tvConstrained}}, types.TypeVoid)}}, tvConstrained)
	_, _ = types.InferFunction(fnParamConstrained, []types.Type{types.NewFunction([]types.Param{{Name: "p", Type: types.TypeString}}, types.TypeVoid)})

	// Object field constraint failure in inferTypeBindings
	objConstrained := types.NewObject("ObjC")
	objConstrained.AddField("x", tvConstrained, false)
	fnObjConstrained := types.NewGenericFunction([]*types.TypeVar{tvConstrained}, []types.Param{{Name: "o", Type: objConstrained}}, tvConstrained)
	objActualBad := types.NewObject("ObjA")
	objActualBad.AddField("x", types.TypeString, false)
	_, _ = types.InferFunction(fnObjConstrained, []types.Type{objActualBad})

	// 17. InferFunction when actualArgs > fn.Params
	fnSingleParam := types.NewGenericFunction([]*types.TypeVar{tvInst}, []types.Param{{Name: "x", Type: tvInst}}, tvInst)
	_, _ = types.InferFunction(fnSingleParam, []types.Type{types.TypeNumber, types.TypeString})

	// 18. FunctionBindings param count mismatch and constraint failure
	fnGenTwo := types.NewGenericFunction([]*types.TypeVar{tvInst}, []types.Param{{Name: "x", Type: tvInst}, {Name: "y", Type: tvInst}}, tvInst)
	fnConcOne := types.NewFunction([]types.Param{{Name: "x", Type: types.TypeNumber}}, types.TypeNumber)
	_, _ = types.FunctionBindings(fnGenTwo, fnConcOne)
	fnConcBadParam := types.NewFunction([]types.Param{{Name: "x", Type: types.TypeString}}, types.TypeString)
	_, _ = types.FunctionBindings(fnConstrained, fnConcBadParam)
}

func TestTsproRemainingErrors(t *testing.T) {
	dir := t.TempDir()
	c := tspro.New(tspro.Options{TargetOS: "linux", TargetArch: "amd64"})

	// 1. Cyclic import in e2e
	fA := filepath.Join(dir, "cyc_a.ts")
	fB := filepath.Join(dir, "cyc_b.ts")
	_ = os.WriteFile(fA, []byte("import { b } from './cyc_b';"), 0o644)
	_ = os.WriteFile(fB, []byte("import { a } from './cyc_a';"), 0o644)
	_, _ = c.CompileFile(fA, filepath.Join(dir, "out_cyc"))

	// 2. Module read error
	unreadable := filepath.Join(dir, "unreadable_caller.ts")
	_ = os.WriteFile(unreadable, []byte("import { x } from './non_existent_module_dir/index';"), 0o644)
	_, _ = c.CompileFile(unreadable, filepath.Join(dir, "out_unreadable"))

	// 3. Nested module parse error
	badSyntaxDep := filepath.Join(dir, "bad_syntax_dep.ts")
	_ = os.WriteFile(badSyntaxDep, []byte("let x: = ;"), 0o644)
	badCaller := filepath.Join(dir, "bad_caller.ts")
	_ = os.WriteFile(badCaller, []byte("import { x } from './bad_syntax_dep';"), 0o644)
	_, _ = c.CompileFile(badCaller, filepath.Join(dir, "out_bad_syntax"))

	// 4. Output write error (unwritable path)
	goodFile := filepath.Join(dir, "good.ts")
	_ = os.WriteFile(goodFile, []byte("console.log(1);"), 0o644)
	_, _ = c.CompileFile(goodFile, "/non_existent_directory_9999/out")

	// 5. Darwin and Windows compileProgram emission
	cDarwin := tspro.New(tspro.Options{TargetOS: "darwin", TargetArch: "arm64"})
	_, _, _ = cDarwin.CompileSource("darwin.ts", []byte("console.log(1);"))
	cWin := tspro.New(tspro.Options{TargetOS: "windows", TargetArch: "amd64"})
	_, _, _ = cWin.CompileSource("win.ts", []byte("console.log(1);"))

	// 6. Diamond import (already loaded module)
	fShared := filepath.Join(dir, "shared.ts")
	fMod1 := filepath.Join(dir, "mod1.ts")
	fMod2 := filepath.Join(dir, "mod2.ts")
	fDiamond := filepath.Join(dir, "diamond.ts")
	_ = os.WriteFile(fShared, []byte("export const c = 10;"), 0o644)
	_ = os.WriteFile(fMod1, []byte("import { c } from './shared';"), 0o644)
	_ = os.WriteFile(fMod2, []byte("import { c } from './shared';"), 0o644)
	_ = os.WriteFile(fDiamond, []byte("import { c as c1 } from './mod1'; import { c as c2 } from './mod2';"), 0o644)
	_, _ = c.CompileFile(fDiamond, filepath.Join(dir, "out_diamond"))

	// 7. CompileFile with type checking failure
	fTypeErr := filepath.Join(dir, "type_err.ts")
	_ = os.WriteFile(fTypeErr, []byte("let n: number = 'bad';"), 0o644)
	_, _ = c.CompileFile(fTypeErr, filepath.Join(dir, "out_type_err"))

	// 8. CompileFile with nonexistent root module
	_, _ = c.CompileFile(filepath.Join(dir, "does_not_exist.ts"), filepath.Join(dir, "out_missing"))

	// 9. CompileSource with irgen error
	_, _, _ = c.CompileSource("irgen_fail.ts", []byte("function f() { const t: [number, string] = [1, 'a']; t[10] = 5; }"))
}
