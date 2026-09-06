package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/token"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func removeNullishIRType(t types.Type) types.Type {
	union, ok := t.(*types.UnionType)
	if !ok {
		return t
	}
	members := make([]types.Type, 0, len(union.Members))
	for _, member := range union.Members {
		if member != types.TypeNull && member != types.TypeUndefined {
			members = append(members, member)
		}
	}
	return members[0]
}

func (g *generator) lowerOptionalMember(e *ast.MemberExpr) ir.Operand {
	baseType := removeNullishIRType(g.semanticType(e.Object))
	objType, ok := baseType.(*types.ObjectType)
	if !ok {
		return g.failExpr("native optional chaining currently requires a closed object receiver, got %s", baseType)
	}
	offsets, _, _ := g.objectLayout(objType)
	offset := offsets[e.Property]

	obj := g.lowerExpr(e.Object)
	start := g.currentBB
	checkNull := g.currentFn.NewBlock("optional_check_null")
	missing := g.currentFn.NewBlock("optional_missing")
	load := g.currentFn.NewBlock("optional_load")
	join := g.currentFn.NewBlock("optional_join")

	isUndefined := g.currentFn.NewValue("optional_undefined", types.TypeBoolean)
	start.Instructions = append(start.Instructions, &ir.BinaryInst{Res: isUndefined, Op: ir.OpEq, LHS: obj, RHS: ir.ConstUndefined{}})
	start.Terminator = &ir.BranchTerm{Cond: isUndefined, Then: missing, Else: checkNull}

	g.currentBB = checkNull
	isNull := g.currentFn.NewValue("optional_null", types.TypeBoolean)
	checkNull.Instructions = append(checkNull.Instructions, &ir.BinaryInst{Res: isNull, Op: ir.OpEq, LHS: obj, RHS: ir.ConstNull{}})
	checkNull.Terminator = &ir.BranchTerm{Cond: isNull, Then: missing, Else: load}

	missing.Terminator = &ir.JumpTerm{Target: join}

	g.currentBB = load
	resultType := types.TypeAny
	if t := g.semanticType(e); t != nil {
		resultType = t
	}
	loaded := g.currentFn.NewValue("optional_field", resultType)
	load.Instructions = append(load.Instructions, &ir.GetFieldInst{Res: loaded, Obj: obj, Field: e.Property, Offset: offset})
	load.Terminator = &ir.JumpTerm{Target: join}

	g.currentBB = join
	res := g.currentFn.NewValue("optional", resultType)
	join.Phis = append(join.Phis, &ir.PhiInst{Res: res, Incoming: []ir.PhiIncoming{
		{Block: missing, Value: ir.ConstUndefined{}},
		{Block: load, Value: loaded},
	}})
	return res
}

func (g *generator) lowerNullishExpr(e *ast.BinaryExpr) ir.Operand {
	lhs := g.lowerExpr(e.Left)
	lhsBB := g.currentBB
	checkNullBB := g.currentFn.NewBlock("nullish_check_null")
	rhsBB := g.currentFn.NewBlock("nullish_rhs")
	shortBB := g.currentFn.NewBlock("nullish_value")
	joinBB := g.currentFn.NewBlock("nullish_join")

	undef := g.currentFn.NewValue("is_undefined", types.TypeBoolean)
	lhsBB.Instructions = append(lhsBB.Instructions, &ir.BinaryInst{Res: undef, Op: ir.OpEq, LHS: lhs, RHS: ir.ConstUndefined{}})
	lhsBB.Terminator = &ir.BranchTerm{Cond: undef, Then: rhsBB, Else: checkNullBB}

	g.currentBB = checkNullBB
	isNull := g.currentFn.NewValue("is_null", types.TypeBoolean)
	checkNullBB.Instructions = append(checkNullBB.Instructions, &ir.BinaryInst{Res: isNull, Op: ir.OpEq, LHS: lhs, RHS: ir.ConstNull{}})
	checkNullBB.Terminator = &ir.BranchTerm{Cond: isNull, Then: rhsBB, Else: shortBB}

	g.currentBB = shortBB
	shortBB.Terminator = &ir.JumpTerm{Target: joinBB}

	g.currentBB = rhsBB
	rhs := g.lowerExpr(e.Right)
	rhsEnd := g.currentBB
	if rhsEnd.Terminator == nil {
		rhsEnd.Terminator = &ir.JumpTerm{Target: joinBB}
	}

	g.currentBB = joinBB
	resultType := rhs.Type()
	if t := g.semanticType(e); t != nil {
		resultType = t
	}
	res := g.currentFn.NewValue("nullish", resultType)
	joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{Res: res, Incoming: []ir.PhiIncoming{
		{Block: shortBB, Value: lhs},
		{Block: rhsEnd, Value: rhs},
	}})
	return res
}

func (g *generator) lowerLogicalExpr(e *ast.BinaryExpr) ir.Operand {
	lhs := g.lowerExpr(e.Left)
	lhsBB := g.currentBB
	rhsBB := g.currentFn.NewBlock("logical_rhs")
	shortBB := g.currentFn.NewBlock("logical_short")
	joinBB := g.currentFn.NewBlock("logical_join")

	if e.Op == token.AmpAmp {
		lhsBB.Terminator = &ir.BranchTerm{Cond: lhs, Then: rhsBB, Else: shortBB}
	} else {
		lhsBB.Terminator = &ir.BranchTerm{Cond: lhs, Then: shortBB, Else: rhsBB}
	}

	g.currentBB = shortBB
	shortVal := lhs
	shortEnd := g.currentBB
	shortEnd.Terminator = &ir.JumpTerm{Target: joinBB}

	g.currentBB = rhsBB
	rhsVal := g.lowerExpr(e.Right)
	rhsEnd := g.currentBB
	if rhsEnd.Terminator == nil {
		rhsEnd.Terminator = &ir.JumpTerm{Target: joinBB}
	}

	g.currentBB = joinBB
	resultType := rhsVal.Type()
	if t := g.semanticType(e); t != nil {
		resultType = t
	}
	res := g.currentFn.NewValue("logical", resultType)
	joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{
		Res: res,
		Incoming: []ir.PhiIncoming{
			{Block: shortEnd, Value: shortVal},
			{Block: rhsEnd, Value: rhsVal},
		},
	})
	return res
}
