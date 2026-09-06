package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) lowerURLSearchParamsNew() ir.Operand {
	t := g.semaResult.URLSearchParamsType
	offsets, refMask, shape := g.objectLayout(t)
	params := g.currentFn.NewValue("url_search_params", t)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
		Res: params, Shape: shape, FieldCount: len(offsets), RefMask: refMask,
	})
	entriesType := types.NewArray(types.TypeString)
	entries := g.currentFn.NewValue("url_search_entries", entriesType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{
		Res: entries, ElemType: types.TypeString, Length: ir.ConstNumber{Value: 0},
	})
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{
		Obj: params, Field: "$entries", Offset: offsets["$entries"], Val: entries,
	})
	return params
}

func (g *generator) urlSearchParamsEntries(params ir.Operand) ir.Operand {
	t := g.semaResult.URLSearchParamsType
	offsets, _, _ := g.objectLayout(t)
	entriesType := types.NewArray(types.TypeString)
	entries := g.currentFn.NewValue("url_search_entries", entriesType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{
		Res: entries, Obj: params, Field: "$entries", Offset: offsets["$entries"],
	})
	return entries
}

func (g *generator) lowerURLSearchParamsSize(params ir.Operand) ir.Operand {
	entries := g.urlSearchParamsEntries(params)
	length := g.currentFn.NewValue("url_search_entries_len", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: entries})
	size := g.currentFn.NewValue("url_search_size", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: size, Op: ir.OpDiv, LHS: length, RHS: ir.ConstNumber{Value: 2}})
	return size
}

func (g *generator) lowerURLSearchParamsHas(params, name ir.Operand) ir.Operand {
	entries := g.urlSearchParamsEntries(params)
	entry := g.currentBB
	condBB := g.currentFn.NewBlock("url_search_has_cond")
	bodyBB := g.currentFn.NewBlock("url_search_has_body")
	foundBB := g.currentFn.NewBlock("url_search_has_found")
	nextBB := g.currentFn.NewBlock("url_search_has_next")
	missBB := g.currentFn.NewBlock("url_search_has_miss")
	joinBB := g.currentFn.NewBlock("url_search_has_join")
	entry.Terminator = &ir.JumpTerm{Target: condBB}

	index := g.currentFn.NewValue("url_search_i", types.TypeNumber)
	nextIndex := g.currentFn.NewValue("url_search_next", types.TypeNumber)
	condBB.Phis = append(condBB.Phis, &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{
		{Block: entry, Value: ir.ConstNumber{Value: 0}},
		{Block: nextBB, Value: nextIndex},
	}})
	length := g.currentFn.NewValue("url_search_len", types.TypeNumber)
	condBB.Instructions = append(condBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: entries})
	more := g.currentFn.NewValue("url_search_more", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: missBB}

	key := g.currentFn.NewValue("url_search_key", types.TypeString)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: key, Array: entries, Index: index})
	equal := g.currentFn.NewValue("url_search_key_eq", types.TypeBoolean)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.CallInst{
		Res: equal, Callee: "ts_string_eq", Args: []ir.Operand{key, name},
		ParamTypes: []types.Type{types.TypeString, types.TypeString},
	})
	bodyBB.Terminator = &ir.BranchTerm{Cond: equal, Then: foundBB, Else: nextBB}

	nextBB.Instructions = append(nextBB.Instructions, &ir.BinaryInst{Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 2}})
	nextBB.Terminator = &ir.JumpTerm{Target: condBB}
	foundBB.Terminator = &ir.JumpTerm{Target: joinBB}
	missBB.Terminator = &ir.JumpTerm{Target: joinBB}

	result := g.currentFn.NewValue("url_search_has", types.TypeBoolean)
	joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{Res: result, Incoming: []ir.PhiIncoming{
		{Block: foundBB, Value: ir.ConstBool{Value: true}},
		{Block: missBB, Value: ir.ConstBool{Value: false}},
	}})
	g.currentBB = joinBB
	return result
}

func (g *generator) lowerURLSearchParamsGet(params, name ir.Operand) ir.Operand {
	entries := g.urlSearchParamsEntries(params)
	entry := g.currentBB
	condBB := g.currentFn.NewBlock("url_search_get_cond")
	bodyBB := g.currentFn.NewBlock("url_search_get_body")
	foundBB := g.currentFn.NewBlock("url_search_get_found")
	nextBB := g.currentFn.NewBlock("url_search_get_next")
	missBB := g.currentFn.NewBlock("url_search_get_miss")
	joinBB := g.currentFn.NewBlock("url_search_get_join")
	entry.Terminator = &ir.JumpTerm{Target: condBB}

	index := g.currentFn.NewValue("url_search_i", types.TypeNumber)
	nextIndex := g.currentFn.NewValue("url_search_next", types.TypeNumber)
	condBB.Phis = append(condBB.Phis, &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{
		{Block: entry, Value: ir.ConstNumber{Value: 0}},
		{Block: nextBB, Value: nextIndex},
	}})
	length := g.currentFn.NewValue("url_search_len", types.TypeNumber)
	condBB.Instructions = append(condBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: entries})
	more := g.currentFn.NewValue("url_search_more", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: missBB}

	key := g.currentFn.NewValue("url_search_key", types.TypeString)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: key, Array: entries, Index: index})
	equal := g.currentFn.NewValue("url_search_key_eq", types.TypeBoolean)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.CallInst{
		Res: equal, Callee: "ts_string_eq", Args: []ir.Operand{key, name},
		ParamTypes: []types.Type{types.TypeString, types.TypeString},
	})
	bodyBB.Terminator = &ir.BranchTerm{Cond: equal, Then: foundBB, Else: nextBB}

	nextBB.Instructions = append(nextBB.Instructions, &ir.BinaryInst{Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 2}})
	nextBB.Terminator = &ir.JumpTerm{Target: condBB}

	valueIndex := g.currentFn.NewValue("url_search_value_index", types.TypeNumber)
	foundBB.Instructions = append(foundBB.Instructions, &ir.BinaryInst{Res: valueIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
	value := g.currentFn.NewValue("url_search_value", types.TypeString)
	foundBB.Instructions = append(foundBB.Instructions, &ir.GetElementInst{Res: value, Array: entries, Index: valueIndex})
	g.currentBB = foundBB
	boxed := g.boxJSValue(value, types.TypeString)
	foundEnd := g.currentBB
	foundEnd.Terminator = &ir.JumpTerm{Target: joinBB}
	missBB.Terminator = &ir.JumpTerm{Target: joinBB}

	result := g.currentFn.NewValue("url_search_get", types.TypeAny)
	joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{Res: result, Incoming: []ir.PhiIncoming{
		{Block: foundEnd, Value: boxed},
		{Block: missBB, Value: ir.ConstNull{}},
	}})
	g.currentBB = joinBB
	return result
}

func (g *generator) lowerURLSearchParamsGetAll(params, name ir.Operand) ir.Operand {
	entries := g.urlSearchParamsEntries(params)
	resultType := types.NewArray(types.TypeString)
	result := g.currentFn.NewValue("url_search_get_all", resultType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{Res: result, ElemType: types.TypeString, Length: ir.ConstNumber{Value: 0}})
	entry := g.currentBB
	condBB := g.currentFn.NewBlock("url_search_get_all_cond")
	bodyBB := g.currentFn.NewBlock("url_search_get_all_body")
	appendBB := g.currentFn.NewBlock("url_search_get_all_append")
	nextBB := g.currentFn.NewBlock("url_search_get_all_next")
	doneBB := g.currentFn.NewBlock("url_search_get_all_done")
	entry.Terminator = &ir.JumpTerm{Target: condBB}
	index := g.currentFn.NewValue("url_search_get_all_i", types.TypeNumber)
	nextIndex := g.currentFn.NewValue("url_search_get_all_next_i", types.TypeNumber)
	condBB.Phis = append(condBB.Phis, &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{{Block: entry, Value: ir.ConstNumber{Value: 0}}, {Block: nextBB, Value: nextIndex}}})
	length := g.currentFn.NewValue("url_search_get_all_len", types.TypeNumber)
	condBB.Instructions = append(condBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: entries})
	more := g.currentFn.NewValue("url_search_get_all_more", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: doneBB}
	key := g.currentFn.NewValue("url_search_get_all_key", types.TypeString)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: key, Array: entries, Index: index})
	equal := g.currentFn.NewValue("url_search_get_all_eq", types.TypeBoolean)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.CallInst{Res: equal, Callee: "ts_string_eq", Args: []ir.Operand{key, name}, ParamTypes: []types.Type{types.TypeString, types.TypeString}})
	bodyBB.Terminator = &ir.BranchTerm{Cond: equal, Then: appendBB, Else: nextBB}
	valueIndex := g.currentFn.NewValue("url_search_get_all_value_i", types.TypeNumber)
	appendBB.Instructions = append(appendBB.Instructions, &ir.BinaryInst{Res: valueIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
	value := g.currentFn.NewValue("url_search_get_all_value", types.TypeString)
	appendBB.Instructions = append(appendBB.Instructions, &ir.GetElementInst{Res: value, Array: entries, Index: valueIndex})
	g.currentBB = appendBB
	g.pushArrayOperand(result, value)
	appendBB.Terminator = &ir.JumpTerm{Target: nextBB}
	nextBB.Instructions = append(nextBB.Instructions, &ir.BinaryInst{Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 2}})
	nextBB.Terminator = &ir.JumpTerm{Target: condBB}
	g.currentBB = doneBB
	return result
}

func (g *generator) lowerURLSearchParamsRebuild(params, name, replacement ir.Operand, setMode bool) {
	entries := g.urlSearchParamsEntries(params)
	entriesType := types.NewArray(types.TypeString)
	nextEntries := g.currentFn.NewValue("url_search_rebuilt", entriesType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{Res: nextEntries, ElemType: types.TypeString, Length: ir.ConstNumber{Value: 0}})
	entry := g.currentBB
	condBB := g.currentFn.NewBlock("url_search_rebuild_cond")
	bodyBB := g.currentFn.NewBlock("url_search_rebuild_body")
	matchBB := g.currentFn.NewBlock("url_search_rebuild_match")
	copyBB := g.currentFn.NewBlock("url_search_rebuild_copy")
	firstBB := g.currentFn.NewBlock("url_search_rebuild_first")
	nextBB := g.currentFn.NewBlock("url_search_rebuild_next")
	doneBB := g.currentFn.NewBlock("url_search_rebuild_done")
	entry.Terminator = &ir.JumpTerm{Target: condBB}
	index := g.currentFn.NewValue("url_search_rebuild_i", types.TypeNumber)
	found := g.currentFn.NewValue("url_search_rebuild_found", types.TypeBoolean)
	nextIndex := g.currentFn.NewValue("url_search_rebuild_next_i", types.TypeNumber)
	nextFound := g.currentFn.NewValue("url_search_rebuild_next_found", types.TypeBoolean)
	condBB.Phis = append(condBB.Phis,
		&ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{{Block: entry, Value: ir.ConstNumber{Value: 0}}, {Block: nextBB, Value: nextIndex}}},
		&ir.PhiInst{Res: found, Incoming: []ir.PhiIncoming{{Block: entry, Value: ir.ConstBool{Value: false}}, {Block: nextBB, Value: nextFound}}},
	)
	length := g.currentFn.NewValue("url_search_rebuild_len", types.TypeNumber)
	condBB.Instructions = append(condBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: entries})
	more := g.currentFn.NewValue("url_search_rebuild_more", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: doneBB}
	key := g.currentFn.NewValue("url_search_rebuild_key", types.TypeString)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: key, Array: entries, Index: index})
	valueIndex := g.currentFn.NewValue("url_search_rebuild_value_i", types.TypeNumber)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.BinaryInst{Res: valueIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
	value := g.currentFn.NewValue("url_search_rebuild_value", types.TypeString)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.GetElementInst{Res: value, Array: entries, Index: valueIndex})
	equal := g.currentFn.NewValue("url_search_rebuild_eq", types.TypeBoolean)
	bodyBB.Instructions = append(bodyBB.Instructions, &ir.CallInst{Res: equal, Callee: "ts_string_eq", Args: []ir.Operand{key, name}, ParamTypes: []types.Type{types.TypeString, types.TypeString}})
	bodyBB.Terminator = &ir.BranchTerm{Cond: equal, Then: matchBB, Else: copyBB}
	g.currentBB = copyBB
	g.pushArrayOperand(nextEntries, key)
	g.pushArrayOperand(nextEntries, value)
	copyEnd := g.currentBB
	copyEnd.Terminator = &ir.JumpTerm{Target: nextBB}
	matchEnd := matchBB
	if setMode {
		matchBB.Terminator = &ir.BranchTerm{Cond: found, Then: nextBB, Else: firstBB}
		g.currentBB = firstBB
		g.pushArrayOperand(nextEntries, name)
		g.pushArrayOperand(nextEntries, replacement)
		firstEnd := g.currentBB
		firstEnd.Terminator = &ir.JumpTerm{Target: nextBB}
		matchEnd = firstEnd
	} else {
		matchBB.Terminator = &ir.JumpTerm{Target: nextBB}
	}
	nextBB.Phis = append(nextBB.Phis, &ir.PhiInst{Res: nextFound})
	nextBB.Phis[0].Incoming = append(nextBB.Phis[0].Incoming, ir.PhiIncoming{Block: copyEnd, Value: found})
	if setMode {
		nextBB.Phis[0].Incoming = append(nextBB.Phis[0].Incoming,
			ir.PhiIncoming{Block: matchBB, Value: found},
			ir.PhiIncoming{Block: matchEnd, Value: ir.ConstBool{Value: true}},
		)
	} else {
		nextBB.Phis[0].Incoming = append(nextBB.Phis[0].Incoming, ir.PhiIncoming{Block: matchBB, Value: found})
	}
	nextBB.Instructions = append(nextBB.Instructions, &ir.BinaryInst{Res: nextIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 2}})
	nextBB.Terminator = &ir.JumpTerm{Target: condBB}
	g.currentBB = doneBB
	if setMode {
		appendBB := g.currentFn.NewBlock("url_search_rebuild_append")
		storeBB := g.currentFn.NewBlock("url_search_rebuild_store")
		doneBB.Terminator = &ir.BranchTerm{Cond: found, Then: storeBB, Else: appendBB}
		g.currentBB = appendBB
		g.pushArrayOperand(nextEntries, name)
		g.pushArrayOperand(nextEntries, replacement)
		appendBB.Terminator = &ir.JumpTerm{Target: storeBB}
		g.currentBB = storeBB
	}
	offsets, _, _ := g.objectLayout(g.semaResult.URLSearchParamsType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: params, Field: "$entries", Offset: offsets["$entries"], Val: nextEntries})
}

func (g *generator) lowerURLSearchParamsMethodCall(e *ast.CallExpr, mem *ast.MemberExpr) (ir.Operand, bool) {
	objType, ok := g.semanticType(mem.Object).(*types.ObjectType)
	if !ok || objType.Name != "$URLSearchParams" {
		return nil, false
	}
	params := g.lowerExpr(mem.Object)
	switch mem.Property {
	case "append":
		entries := g.urlSearchParamsEntries(params)
		name := g.lowerExpr(e.Args[0])
		value := g.lowerExpr(e.Args[1])
		g.pushArrayOperand(entries, name)
		g.pushArrayOperand(entries, value)
		return nil, true
	case "get":
		return g.lowerURLSearchParamsGet(params, g.lowerExpr(e.Args[0])), true
	case "getAll":
		return g.lowerURLSearchParamsGetAll(params, g.lowerExpr(e.Args[0])), true
	case "has":
		return g.lowerURLSearchParamsHas(params, g.lowerExpr(e.Args[0])), true
	case "delete":
		g.lowerURLSearchParamsRebuild(params, g.lowerExpr(e.Args[0]), nil, false)
		return nil, true
	case "set":
		g.lowerURLSearchParamsRebuild(params, g.lowerExpr(e.Args[0]), g.lowerExpr(e.Args[1]), true)
		return nil, true
	}
	return nil, false
}
