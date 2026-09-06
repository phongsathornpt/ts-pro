package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) urlStringLen(value ir.Operand) ir.Operand {
	res := g.currentFn.NewValue("url_string_len", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_string_len", Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeString}})
	return res
}

func (g *generator) urlStringFindByte(value ir.Operand, ch, start, end ir.Operand) ir.Operand {
	res := g.currentFn.NewValue("url_string_find", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_string_find_byte", Args: []ir.Operand{value, ch, start, end}, ParamTypes: []types.Type{types.TypeString, types.TypeNumber, types.TypeNumber, types.TypeNumber}})
	return res
}

func (g *generator) urlStringSlice(value, start, end ir.Operand) ir.Operand {
	res := g.currentFn.NewValue("url_string_slice", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_string_slice_bytes", Args: []ir.Operand{value, start, end}, ParamTypes: []types.Type{types.TypeString, types.TypeNumber, types.TypeNumber}})
	return res
}

func (g *generator) lowerFormURLDecode(value ir.Operand) ir.Operand {
	bytes := g.currentFn.NewValue("form_decoded_bytes", g.semaResult.ByteBufferType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: bytes, Callee: "ts_form_url_decode_bytes", Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeString}})
	length := g.currentFn.NewValue("form_decoded_len", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: length, Callee: "ts_byte_buffer_len", Args: []ir.Operand{bytes}, ParamTypes: []types.Type{g.semaResult.ByteBufferType}})
	res := g.currentFn.NewValue("form_decoded", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_utf8_sanitize", Args: []ir.Operand{bytes, ir.ConstNumber{Value: 0}, length, ir.ConstBool{Value: false}}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber, types.TypeBoolean}})
	return res
}

func (g *generator) lowerFormURLEncode(value ir.Operand) ir.Operand {
	res := g.currentFn.NewValue("form_encoded", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_form_url_encode", Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeString}})
	return res
}

func (g *generator) lowerURLSearchParamsParse(entries, input ir.Operand) {
	total := g.urlStringLen(input)
	pre := g.currentBB
	condBB := g.currentFn.NewBlock("url_params_parse_cond")
	scanBB := g.currentFn.NewBlock("url_params_parse_scan")
	ampBB := g.currentFn.NewBlock("url_params_parse_amp")
	lastBB := g.currentFn.NewBlock("url_params_parse_last")
	pairBB := g.currentFn.NewBlock("url_params_parse_pair")
	parseBB := g.currentFn.NewBlock("url_params_parse_component")
	eqBB := g.currentFn.NewBlock("url_params_parse_eq")
	noEqBB := g.currentFn.NewBlock("url_params_parse_no_eq")
	decodeBB := g.currentFn.NewBlock("url_params_parse_decode")
	advanceBB := g.currentFn.NewBlock("url_params_parse_advance")
	doneBB := g.currentFn.NewBlock("url_params_parse_done")
	pre.Terminator = &ir.JumpTerm{Target: condBB}

	offset := g.currentFn.NewValue("url_params_offset", types.TypeNumber)
	nextOffset := g.currentFn.NewValue("url_params_next_offset", types.TypeNumber)
	condBB.Phis = append(condBB.Phis, &ir.PhiInst{Res: offset, Incoming: []ir.PhiIncoming{{Block: pre, Value: ir.ConstNumber{Value: 0}}, {Block: advanceBB, Value: nextOffset}}})
	more := g.currentFn.NewValue("url_params_more", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: offset, RHS: total})
	condBB.Terminator = &ir.BranchTerm{Cond: more, Then: scanBB, Else: doneBB}

	g.currentBB = scanBB
	amp := g.urlStringFindByte(input, ir.ConstNumber{Value: '&'}, offset, total)
	hasAmp := g.currentFn.NewValue("url_params_has_amp", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: hasAmp, Op: ir.OpGe, LHS: amp, RHS: ir.ConstNumber{Value: 0}})
	g.currentBB.Terminator = &ir.BranchTerm{Cond: hasAmp, Then: ampBB, Else: lastBB}
	ampBB.Terminator = &ir.JumpTerm{Target: pairBB}
	lastBB.Terminator = &ir.JumpTerm{Target: pairBB}

	pairEnd := g.currentFn.NewValue("url_params_pair_end", types.TypeNumber)
	pairBB.Phis = append(pairBB.Phis, &ir.PhiInst{Res: pairEnd, Incoming: []ir.PhiIncoming{{Block: ampBB, Value: amp}, {Block: lastBB, Value: total}}})
	nonEmpty := g.currentFn.NewValue("url_params_pair_nonempty", types.TypeBoolean)
	pairBB.Instructions = append(pairBB.Instructions, &ir.BinaryInst{Res: nonEmpty, Op: ir.OpGt, LHS: pairEnd, RHS: offset})
	pairBB.Terminator = &ir.BranchTerm{Cond: nonEmpty, Then: parseBB, Else: advanceBB}

	g.currentBB = parseBB
	equalAt := g.urlStringFindByte(input, ir.ConstNumber{Value: '='}, offset, pairEnd)
	hasEq := g.currentFn.NewValue("url_params_has_eq", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: hasEq, Op: ir.OpGe, LHS: equalAt, RHS: ir.ConstNumber{Value: 0}})
	g.currentBB.Terminator = &ir.BranchTerm{Cond: hasEq, Then: eqBB, Else: noEqBB}

	eqValueStart := g.currentFn.NewValue("url_params_eq_value_start", types.TypeNumber)
	eqBB.Instructions = append(eqBB.Instructions, &ir.BinaryInst{Res: eqValueStart, Op: ir.OpAdd, LHS: equalAt, RHS: ir.ConstNumber{Value: 1}})
	eqBB.Terminator = &ir.JumpTerm{Target: decodeBB}
	noEqBB.Terminator = &ir.JumpTerm{Target: decodeBB}
	nameEnd := g.currentFn.NewValue("url_params_name_end", types.TypeNumber)
	valueStart := g.currentFn.NewValue("url_params_value_start", types.TypeNumber)
	decodeBB.Phis = append(decodeBB.Phis,
		&ir.PhiInst{Res: nameEnd, Incoming: []ir.PhiIncoming{{Block: eqBB, Value: equalAt}, {Block: noEqBB, Value: pairEnd}}},
		&ir.PhiInst{Res: valueStart, Incoming: []ir.PhiIncoming{{Block: eqBB, Value: eqValueStart}, {Block: noEqBB, Value: pairEnd}}},
	)
	g.currentBB = decodeBB
	rawName := g.urlStringSlice(input, offset, nameEnd)
	rawValue := g.urlStringSlice(input, valueStart, pairEnd)
	name := g.lowerFormURLDecode(rawName)
	value := g.lowerFormURLDecode(rawValue)
	g.pushArrayOperand(entries, name)
	g.pushArrayOperand(entries, value)
	decodeEnd := g.currentBB
	decodeEnd.Terminator = &ir.JumpTerm{Target: advanceBB}

	g.currentBB = advanceBB
	advanceBB.Instructions = append(advanceBB.Instructions, &ir.BinaryInst{Res: nextOffset, Op: ir.OpAdd, LHS: pairEnd, RHS: ir.ConstNumber{Value: 1}})
	advanceBB.Terminator = &ir.JumpTerm{Target: condBB}
	g.currentBB = doneBB
}

func (g *generator) lowerURLSearchParamsNew(init ir.Operand) ir.Operand {
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
	if init != nil {
		g.lowerURLSearchParamsParse(entries, init)
	}
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

func (g *generator) lowerURLSearchParamsSerialize(params ir.Operand) ir.Operand {
	entries := g.urlSearchParamsEntries(params)
	start := g.currentBB
	length := g.currentFn.NewValue("url_params_serialize_len", types.TypeNumber)
	start.Instructions = append(start.Instructions, &ir.ArrayLengthInst{Res: length, Array: entries})
	hasAny := g.currentFn.NewValue("url_params_serialize_has", types.TypeBoolean)
	start.Instructions = append(start.Instructions, &ir.BinaryInst{Res: hasAny, Op: ir.OpGt, LHS: length, RHS: ir.ConstNumber{Value: 0}})
	nonEmpty := g.currentFn.NewBlock("url_params_serialize_nonempty")
	empty := g.currentFn.NewBlock("url_params_serialize_empty")
	cond := g.currentFn.NewBlock("url_params_serialize_cond")
	body := g.currentFn.NewBlock("url_params_serialize_body")
	done := g.currentFn.NewBlock("url_params_serialize_done")
	join := g.currentFn.NewBlock("url_params_serialize_join")
	start.Terminator = &ir.BranchTerm{Cond: hasAny, Then: nonEmpty, Else: empty}
	empty.Terminator = &ir.JumpTerm{Target: join}

	g.currentBB = nonEmpty
	key0 := g.currentFn.NewValue("url_params_key0", types.TypeString)
	val0 := g.currentFn.NewValue("url_params_val0", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.GetElementInst{Res: key0, Array: entries, Index: ir.ConstNumber{Value: 0}},
		&ir.GetElementInst{Res: val0, Array: entries, Index: ir.ConstNumber{Value: 1}},
	)
	encKey0 := g.lowerFormURLEncode(key0)
	encVal0 := g.lowerFormURLEncode(val0)
	pair0 := g.concatNativeStrings(g.concatNativeStrings(encKey0, ir.ConstString{Value: "="}), encVal0)
	nonEmptyEnd := g.currentBB
	nonEmptyEnd.Terminator = &ir.JumpTerm{Target: cond}

	index := g.currentFn.NewValue("url_params_serialize_i", types.TypeNumber)
	acc := g.currentFn.NewValue("url_params_serialize_acc", types.TypeString)
	cond.Phis = append(cond.Phis,
		&ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{{Block: nonEmptyEnd, Value: ir.ConstNumber{Value: 2}}}},
		&ir.PhiInst{Res: acc, Incoming: []ir.PhiIncoming{{Block: nonEmptyEnd, Value: pair0}}},
	)
	more := g.currentFn.NewValue("url_params_serialize_more", types.TypeBoolean)
	cond.Instructions = append(cond.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	cond.Terminator = &ir.BranchTerm{Cond: more, Then: body, Else: done}

	g.currentBB = body
	key := g.currentFn.NewValue("url_params_key", types.TypeString)
	valIndex := g.currentFn.NewValue("url_params_val_i", types.TypeNumber)
	val := g.currentFn.NewValue("url_params_val", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.GetElementInst{Res: key, Array: entries, Index: index},
		&ir.BinaryInst{Res: valIndex, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}},
		&ir.GetElementInst{Res: val, Array: entries, Index: valIndex},
	)
	encKey := g.lowerFormURLEncode(key)
	encVal := g.lowerFormURLEncode(val)
	pair := g.concatNativeStrings(g.concatNativeStrings(encKey, ir.ConstString{Value: "="}), encVal)
	withAmp := g.concatNativeStrings(ir.ConstString{Value: "&"}, pair)
	nextAcc := g.concatNativeStrings(acc, withAmp)
	next := g.currentFn.NewValue("url_params_serialize_next", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: next, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 2}})
	bodyEnd := g.currentBB
	bodyEnd.Terminator = &ir.JumpTerm{Target: cond}
	cond.Phis[0].Incoming = append(cond.Phis[0].Incoming, ir.PhiIncoming{Block: bodyEnd, Value: next})
	cond.Phis[1].Incoming = append(cond.Phis[1].Incoming, ir.PhiIncoming{Block: bodyEnd, Value: nextAcc})

	g.currentBB = done
	doneEnd := g.currentBB
	doneEnd.Terminator = &ir.JumpTerm{Target: join}
	g.currentBB = join
	result := g.currentFn.NewValue("url_params_serialized", types.TypeString)
	join.Phis = append(join.Phis, &ir.PhiInst{Res: result, Incoming: []ir.PhiIncoming{{Block: empty, Value: ir.ConstString{Value: ""}}, {Block: doneEnd, Value: acc}}})
	return result
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
		g.lowerURLSearchParamsSyncOwner(params)
		return nil, true
	case "get":
		return g.lowerURLSearchParamsGet(params, g.lowerExpr(e.Args[0])), true
	case "getAll":
		return g.lowerURLSearchParamsGetAll(params, g.lowerExpr(e.Args[0])), true
	case "has":
		return g.lowerURLSearchParamsHas(params, g.lowerExpr(e.Args[0])), true
	case "delete":
		g.lowerURLSearchParamsRebuild(params, g.lowerExpr(e.Args[0]), nil, false)
		g.lowerURLSearchParamsSyncOwner(params)
		return nil, true
	case "set":
		g.lowerURLSearchParamsRebuild(params, g.lowerExpr(e.Args[0]), g.lowerExpr(e.Args[1]), true)
		g.lowerURLSearchParamsSyncOwner(params)
		return nil, true
	case "toString":
		return g.lowerURLSearchParamsSerialize(params), true
	case "sort":
		g.lowerURLSearchParamsSort(params)
		g.lowerURLSearchParamsSyncOwner(params)
		return nil, true
	}
	return nil, false
}

func (g *generator) lowerURLSearchParamsSort(params ir.Operand) {
	entries := g.urlSearchParamsEntries(params)
	entry := g.currentBB
	outerCond := g.currentFn.NewBlock("url_search_sort_outer_cond")
	outerBody := g.currentFn.NewBlock("url_search_sort_outer_body")
	innerCond := g.currentFn.NewBlock("url_search_sort_inner_cond")
	innerBody := g.currentFn.NewBlock("url_search_sort_inner_body")
	shiftBB := g.currentFn.NewBlock("url_search_sort_shift")
	insertBB := g.currentFn.NewBlock("url_search_sort_insert")
	outerNext := g.currentFn.NewBlock("url_search_sort_outer_next")
	doneBB := g.currentFn.NewBlock("url_search_sort_done")
	entry.Terminator = &ir.JumpTerm{Target: outerCond}

	i := g.currentFn.NewValue("url_search_sort_i", types.TypeNumber)
	nextI := g.currentFn.NewValue("url_search_sort_next_i", types.TypeNumber)
	outerCond.Phis = append(outerCond.Phis, &ir.PhiInst{Res: i, Incoming: []ir.PhiIncoming{
		{Block: entry, Value: ir.ConstNumber{Value: 2}},
		{Block: outerNext, Value: nextI},
	}})
	length := g.currentFn.NewValue("url_search_sort_len", types.TypeNumber)
	outerCond.Instructions = append(outerCond.Instructions, &ir.ArrayLengthInst{Res: length, Array: entries})
	more := g.currentFn.NewValue("url_search_sort_more", types.TypeBoolean)
	outerCond.Instructions = append(outerCond.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: i, RHS: length})
	outerCond.Terminator = &ir.BranchTerm{Cond: more, Then: outerBody, Else: doneBB}

	keyName := g.currentFn.NewValue("url_search_sort_key_name", types.TypeString)
	outerBody.Instructions = append(outerBody.Instructions, &ir.GetElementInst{Res: keyName, Array: entries, Index: i})
	keyValueIndex := g.currentFn.NewValue("url_search_sort_key_value_index", types.TypeNumber)
	outerBody.Instructions = append(outerBody.Instructions, &ir.BinaryInst{Res: keyValueIndex, Op: ir.OpAdd, LHS: i, RHS: ir.ConstNumber{Value: 1}})
	keyValue := g.currentFn.NewValue("url_search_sort_key_value", types.TypeString)
	outerBody.Instructions = append(outerBody.Instructions, &ir.GetElementInst{Res: keyValue, Array: entries, Index: keyValueIndex})
	outerBody.Terminator = &ir.JumpTerm{Target: innerCond}

	j := g.currentFn.NewValue("url_search_sort_j", types.TypeNumber)
	prevJ := g.currentFn.NewValue("url_search_sort_prev_j", types.TypeNumber)
	innerCond.Phis = append(innerCond.Phis, &ir.PhiInst{Res: j, Incoming: []ir.PhiIncoming{
		{Block: outerBody, Value: i},
		{Block: shiftBB, Value: prevJ},
	}})
	hasPrev := g.currentFn.NewValue("url_search_sort_has_prev", types.TypeBoolean)
	innerCond.Instructions = append(innerCond.Instructions, &ir.BinaryInst{Res: hasPrev, Op: ir.OpGt, LHS: j, RHS: ir.ConstNumber{Value: 0}})
	innerCond.Terminator = &ir.BranchTerm{Cond: hasPrev, Then: innerBody, Else: insertBB}

	prevIndex := g.currentFn.NewValue("url_search_sort_prev_index", types.TypeNumber)
	innerBody.Instructions = append(innerBody.Instructions, &ir.BinaryInst{Res: prevIndex, Op: ir.OpSub, LHS: j, RHS: ir.ConstNumber{Value: 2}})
	prevName := g.currentFn.NewValue("url_search_sort_prev_name", types.TypeString)
	innerBody.Instructions = append(innerBody.Instructions, &ir.GetElementInst{Res: prevName, Array: entries, Index: prevIndex})

	g.currentBB = innerBody
	prevBox := g.boxJSValue(prevName, types.TypeString)
	keyBox := g.boxJSValue(keyName, types.TypeString)
	greater := g.currentFn.NewValue("url_search_sort_greater", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: greater, Callee: "ts_js_gt", Args: []ir.Operand{prevBox, keyBox}, ParamTypes: []types.Type{types.TypeAny, types.TypeAny},
	})
	g.currentBB.Terminator = &ir.BranchTerm{Cond: greater, Then: shiftBB, Else: insertBB}

	prevValueIndex := g.currentFn.NewValue("url_search_sort_prev_value_index", types.TypeNumber)
	shiftBB.Instructions = append(shiftBB.Instructions, &ir.BinaryInst{Res: prevValueIndex, Op: ir.OpAdd, LHS: prevIndex, RHS: ir.ConstNumber{Value: 1}})
	prevValue := g.currentFn.NewValue("url_search_sort_prev_value", types.TypeString)
	shiftBB.Instructions = append(shiftBB.Instructions, &ir.GetElementInst{Res: prevValue, Array: entries, Index: prevValueIndex})
	shiftValueIndex := g.currentFn.NewValue("url_search_sort_shift_value_index", types.TypeNumber)
	shiftBB.Instructions = append(shiftBB.Instructions,
		&ir.SetElementInst{Array: entries, Index: j, Val: prevName},
		&ir.BinaryInst{Res: shiftValueIndex, Op: ir.OpAdd, LHS: j, RHS: ir.ConstNumber{Value: 1}},
		&ir.SetElementInst{Array: entries, Index: shiftValueIndex, Val: prevValue},
		&ir.BinaryInst{Res: prevJ, Op: ir.OpSub, LHS: j, RHS: ir.ConstNumber{Value: 2}},
	)
	shiftBB.Terminator = &ir.JumpTerm{Target: innerCond}

	insertValueIndex := g.currentFn.NewValue("url_search_sort_insert_value_index", types.TypeNumber)
	insertBB.Instructions = append(insertBB.Instructions,
		&ir.SetElementInst{Array: entries, Index: j, Val: keyName},
		&ir.BinaryInst{Res: insertValueIndex, Op: ir.OpAdd, LHS: j, RHS: ir.ConstNumber{Value: 1}},
		&ir.SetElementInst{Array: entries, Index: insertValueIndex, Val: keyValue},
	)
	insertBB.Terminator = &ir.JumpTerm{Target: outerNext}

	outerNext.Instructions = append(outerNext.Instructions, &ir.BinaryInst{Res: nextI, Op: ir.OpAdd, LHS: i, RHS: ir.ConstNumber{Value: 2}})
	outerNext.Terminator = &ir.JumpTerm{Target: outerCond}
	g.currentBB = doneBB
}

func (g *generator) urlSearchParamsOwner(params ir.Operand) ir.Operand {
	t := g.semaResult.URLSearchParamsType
	offsets, _, _ := g.objectLayout(t)
	owner := g.currentFn.NewValue("url_search_owner", g.semaResult.URLType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{
		Res: owner, Obj: params, Field: "$url", Offset: offsets["$url"],
	})
	return owner
}

func (g *generator) lowerURLSearchParamsSyncOwner(params ir.Operand) {
	owner := g.urlSearchParamsOwner(params)
	nullOwner := g.nullRef(g.semaResult.URLType)
	isNull := g.currentFn.NewValue("url_search_owner_null", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
		Res: isNull, Op: ir.OpEq, LHS: owner, RHS: nullOwner,
	})
	skipBB := g.currentFn.NewBlock("url_search_sync_skip")
	syncBB := g.currentFn.NewBlock("url_search_sync_owner")
	joinBB := g.currentFn.NewBlock("url_search_sync_join")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: isNull, Then: skipBB, Else: syncBB}
	skipBB.Terminator = &ir.JumpTerm{Target: joinBB}

	g.currentBB = syncBB
	query := g.lowerURLSearchParamsSerialize(params)
	urlOffsets, _, _ := g.objectLayout(g.semaResult.URLType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{
		Obj: owner, Field: "$query", Offset: urlOffsets["$query"], Val: query,
	})
	syncEnd := g.currentBB
	syncEnd.Terminator = &ir.JumpTerm{Target: joinBB}
	g.currentBB = joinBB
}

func (g *generator) lowerURLSearchParamsReplace(params, input ir.Operand) {
	t := g.semaResult.URLSearchParamsType
	offsets, _, _ := g.objectLayout(t)
	entriesType := types.NewArray(types.TypeString)
	entries := g.currentFn.NewValue("url_search_replace_entries", entriesType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{
		Res: entries, ElemType: types.TypeString, Length: ir.ConstNumber{Value: 0},
	})
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{
		Obj: params, Field: "$entries", Offset: offsets["$entries"], Val: entries,
	})
	g.lowerURLSearchParamsParse(entries, input)
}
