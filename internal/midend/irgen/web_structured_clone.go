package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

const structuredCloneAnyWorkerName = "$structured_clone_any"

func (g *generator) lowerStructuredClone(e *ast.CallExpr) ir.Operand {
	sourceType := g.semanticType(e.Args[0])
	source := g.lowerExpr(e.Args[0])
	memoType := types.NewArray(types.TypeAny)
	memoSources := g.currentFn.NewValue("structured_clone_sources", memoType)
	memoClones := g.currentFn.NewValue("structured_clone_clones", memoType)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.AllocArrayInst{Res: memoSources, ElemType: types.TypeAny, Length: ir.ConstNumber{Value: 0}},
		&ir.AllocArrayInst{Res: memoClones, ElemType: types.TypeAny, Length: ir.ConstNumber{Value: 0}},
	)
	return g.cloneStructuredValue(source, sourceType, memoSources, memoClones)
}

func (g *generator) cloneStructuredValue(source ir.Operand, sourceType types.Type, memoSources, memoClones ir.Operand) ir.Operand {
	if sourceType == nil {
		return g.failExpr("structuredClone requires a known source type")
	}

	switch sourceType.Kind() {
	case types.KindNumber, types.KindBoolean, types.KindString, types.KindNull, types.KindUndefined:
		return source
	case types.KindAny, types.KindUnknown:
		return g.cloneStructuredAny(source, memoSources, memoClones)
	}

	switch t := sourceType.(type) {
	case *types.ArrayType:
		return g.cloneStructuredArray(source, t, memoSources, memoClones)
	case *types.ObjectType:
		if t.Name == "$ArrayBuffer" {
			return g.cloneStructuredArrayBuffer(source, t, memoSources, memoClones)
		}
		if t.Name != "" || g.semaResult.Classes[t.Name] != nil {
			return g.failExpr("structuredClone support for platform/class object %q is not implemented", t.Name)
		}
		return g.cloneStructuredObject(source, t, memoSources, memoClones)
	default:
		return g.failExpr("structuredClone support for type %q is not implemented", sourceType.String())
	}
}

func (g *generator) cloneStructuredAny(source, memoSources, memoClones ir.Operand) ir.Operand {
	g.ensureStructuredCloneAnyWorker()
	memoType := types.NewArray(types.TypeAny)
	result := g.currentFn.NewValue("structured_clone_any_result", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res:        result,
		Callee:     structuredCloneAnyWorkerName,
		Args:       []ir.Operand{source, memoSources, memoClones},
		ParamTypes: []types.Type{types.TypeAny, memoType, memoType},
	})
	return result
}

func (g *generator) ensureStructuredCloneAnyWorker() {
	for _, fn := range g.prog.Functions {
		if fn.Name == structuredCloneAnyWorkerName {
			return
		}
	}

	outerFn, outerBB := g.currentFn, g.currentBB
	outerLocals, outerProv, outerDirect := g.locals, g.localProvenance, g.localDirectCallee

	memoType := types.NewArray(types.TypeAny)
	worker := ir.NewFunction(structuredCloneAnyWorkerName, types.TypeAny)
	source := worker.NewValue("source", types.TypeAny)
	memoSources := worker.NewValue("memo_sources", memoType)
	memoClones := worker.NewValue("memo_clones", memoType)
	worker.Params = append(worker.Params, source, memoSources, memoClones)

	g.currentFn = worker
	g.currentBB = worker.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)

	isDynamic := worker.NewValue("structured_clone_any_dynamic", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: isDynamic, Callee: "ts_js_is_dynamic_object", Args: []ir.Operand{source}, ParamTypes: []types.Type{types.TypeAny},
	})
	dynamicBB := worker.NewBlock("dynamic")
	passthroughBB := worker.NewBlock("passthrough")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: isDynamic, Then: dynamicBB, Else: passthroughBB}
	passthroughBB.Terminator = &ir.ReturnTerm{Val: source}

	g.currentBB = dynamicBB
	memoIndex := g.structuredCloneMemoIndex(source, types.TypeAny, memoSources)
	missing := worker.NewValue("structured_clone_dynamic_missing", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: missing, Op: ir.OpEq, LHS: memoIndex, RHS: ir.ConstNumber{Value: -1}})
	foundBB := worker.NewBlock("found")
	createBB := worker.NewBlock("create")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: missing, Then: createBB, Else: foundBB}

	existing := worker.NewValue("structured_clone_dynamic_existing", types.TypeAny)
	foundBB.Instructions = append(foundBB.Instructions, &ir.GetElementInst{Res: existing, Array: memoClones, Index: memoIndex})
	foundBB.Terminator = &ir.ReturnTerm{Val: existing}

	g.currentBB = createBB
	clone := worker.NewValue("structured_clone_dynamic_object", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: clone, Callee: "ts_dynamic_object_new"})
	g.registerStructuredCloneMemo(source, types.TypeAny, clone, types.TypeAny, memoSources, memoClones)
	count := worker.NewValue("structured_clone_dynamic_count", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: count, Callee: "ts_dynamic_count", Args: []ir.Operand{source}, ParamTypes: []types.Type{types.TypeAny},
	})

	pre := g.currentBB
	condBB := worker.NewBlock("property_cond")
	bodyBB := worker.NewBlock("property_body")
	postBB := worker.NewBlock("property_post")
	doneBB := worker.NewBlock("done")
	pre.Terminator = &ir.JumpTerm{Target: condBB}

	index := worker.NewValue("structured_clone_dynamic_i", types.TypeNumber)
	next := worker.NewValue("structured_clone_dynamic_next", types.TypeNumber)
	condBB.Phis = append(condBB.Phis, &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{
		{Block: pre, Value: ir.ConstNumber{Value: 0}},
		{Block: postBB, Value: next},
	}})
	more := worker.NewValue("structured_clone_dynamic_more", types.TypeBoolean)
	condBB.Instructions = append(condBB.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: count})
	condBB.Terminator = &ir.BranchTerm{Cond: more, Then: bodyBB, Else: doneBB}

	key := worker.NewValue("structured_clone_dynamic_key", types.TypeString)
	value := worker.NewValue("structured_clone_dynamic_value", types.TypeAny)
	copied := worker.NewValue("structured_clone_dynamic_copy", types.TypeAny)
	set := worker.NewValue("structured_clone_dynamic_set", types.TypeAny)
	bodyBB.Instructions = append(bodyBB.Instructions,
		&ir.CallInst{Res: key, Callee: "ts_dynamic_key_at", Args: []ir.Operand{source, index}, ParamTypes: []types.Type{types.TypeAny, types.TypeNumber}},
		&ir.CallInst{Res: value, Callee: "ts_dynamic_value_at", Args: []ir.Operand{source, index}, ParamTypes: []types.Type{types.TypeAny, types.TypeNumber}},
		&ir.CallInst{Res: copied, Callee: structuredCloneAnyWorkerName, Args: []ir.Operand{value, memoSources, memoClones}, ParamTypes: []types.Type{types.TypeAny, memoType, memoType}},
		&ir.CallInst{Res: set, Callee: "ts_dynamic_set", Args: []ir.Operand{clone, key, copied}, ParamTypes: []types.Type{types.TypeAny, types.TypeString, types.TypeAny}},
	)
	bodyBB.Terminator = &ir.JumpTerm{Target: postBB}
	postBB.Instructions = append(postBB.Instructions, &ir.BinaryInst{Res: next, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
	postBB.Terminator = &ir.JumpTerm{Target: condBB}
	doneBB.Terminator = &ir.ReturnTerm{Val: clone}

	g.prog.Functions = append(g.prog.Functions, worker)
	g.currentFn, g.currentBB = outerFn, outerBB
	g.locals, g.localProvenance, g.localDirectCallee = outerLocals, outerProv, outerDirect
}

func (g *generator) structuredCloneMemoIndex(source ir.Operand, sourceType types.Type, memoSources ir.Operand) ir.Operand {
	boxedSource := g.boxJSValue(source, sourceType)
	pre := g.currentBB
	cond := g.currentFn.NewBlock("structured_clone_memo_cond")
	body := g.currentFn.NewBlock("structured_clone_memo_body")
	nextBlock := g.currentFn.NewBlock("structured_clone_memo_next")
	done := g.currentFn.NewBlock("structured_clone_memo_done")
	pre.Terminator = &ir.JumpTerm{Target: cond}

	index := g.currentFn.NewValue("structured_clone_memo_i", types.TypeNumber)
	next := g.currentFn.NewValue("structured_clone_memo_next_i", types.TypeNumber)
	cond.Phis = append(cond.Phis, &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{
		{Block: pre, Value: ir.ConstNumber{Value: 0}},
		{Block: nextBlock, Value: next},
	}})
	length := g.currentFn.NewValue("structured_clone_memo_len", types.TypeNumber)
	more := g.currentFn.NewValue("structured_clone_memo_more", types.TypeBoolean)
	cond.Instructions = append(cond.Instructions,
		&ir.ArrayLengthInst{Res: length, Array: memoSources},
		&ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length},
	)
	cond.Terminator = &ir.BranchTerm{Cond: more, Then: body, Else: done}

	candidate := g.currentFn.NewValue("structured_clone_memo_candidate", types.TypeAny)
	matched := g.currentFn.NewValue("structured_clone_memo_match", types.TypeBoolean)
	body.Instructions = append(body.Instructions,
		&ir.GetElementInst{Res: candidate, Array: memoSources, Index: index},
		&ir.CallInst{Res: matched, Callee: "ts_js_strict_eq", Args: []ir.Operand{candidate, boxedSource}, ParamTypes: []types.Type{types.TypeAny, types.TypeAny}},
	)
	body.Terminator = &ir.BranchTerm{Cond: matched, Then: done, Else: nextBlock}

	nextBlock.Instructions = append(nextBlock.Instructions, &ir.BinaryInst{Res: next, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
	nextBlock.Terminator = &ir.JumpTerm{Target: cond}

	result := g.currentFn.NewValue("structured_clone_memo_index", types.TypeNumber)
	done.Phis = append(done.Phis, &ir.PhiInst{Res: result, Incoming: []ir.PhiIncoming{
		{Block: cond, Value: ir.ConstNumber{Value: -1}},
		{Block: body, Value: index},
	}})
	g.currentBB = done
	return result
}

func (g *generator) registerStructuredCloneMemo(source ir.Operand, sourceType types.Type, clone ir.Operand, cloneType types.Type, memoSources, memoClones ir.Operand) {
	boxedSource := g.boxJSValue(source, sourceType)
	boxedClone := g.boxJSValue(clone, cloneType)
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.ArrayPushInst{Res: g.currentFn.NewValue("structured_clone_source_push", types.TypeNumber), Array: memoSources, Val: boxedSource},
		&ir.ArrayPushInst{Res: g.currentFn.NewValue("structured_clone_clone_push", types.TypeNumber), Array: memoClones, Val: boxedClone},
	)
}

func (g *generator) cloneStructuredArrayBuffer(source ir.Operand, bufferType *types.ObjectType, memoSources, memoClones ir.Operand) ir.Operand {
	memoIndex := g.structuredCloneMemoIndex(source, bufferType, memoSources)
	missing := g.currentFn.NewValue("structured_clone_buffer_missing", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: missing, Op: ir.OpEq, LHS: memoIndex, RHS: ir.ConstNumber{Value: -1}})
	found := g.currentFn.NewBlock("structured_clone_buffer_found")
	create := g.currentFn.NewBlock("structured_clone_buffer_create")
	done := g.currentFn.NewBlock("structured_clone_buffer_done")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: missing, Then: create, Else: found}

	g.currentBB = found
	boxedExisting := g.currentFn.NewValue("structured_clone_buffer_boxed", types.TypeAny)
	found.Instructions = append(found.Instructions, &ir.GetElementInst{Res: boxedExisting, Array: memoClones, Index: memoIndex})
	existing := g.coerceJSValueBoundary(boxedExisting, types.TypeAny, bufferType)
	foundEnd := g.currentBB
	foundEnd.Terminator = &ir.JumpTerm{Target: done}

	g.currentBB = create
	data := g.arrayBufferData(source)
	copiedData := g.copyByteBuffer(data)
	clone := g.newArrayBufferFromData(copiedData)
	g.registerStructuredCloneMemo(source, bufferType, clone, bufferType, memoSources, memoClones)
	createEnd := g.currentBB
	createEnd.Terminator = &ir.JumpTerm{Target: done}

	result := g.currentFn.NewValue("structured_clone_buffer_result", bufferType)
	done.Phis = append(done.Phis, &ir.PhiInst{Res: result, Incoming: []ir.PhiIncoming{
		{Block: foundEnd, Value: existing},
		{Block: createEnd, Value: clone},
	}})
	g.currentBB = done
	return result
}

func (g *generator) cloneStructuredObject(source ir.Operand, objectType *types.ObjectType, memoSources, memoClones ir.Operand) ir.Operand {
	memoIndex := g.structuredCloneMemoIndex(source, objectType, memoSources)
	missing := g.currentFn.NewValue("structured_clone_object_missing", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: missing, Op: ir.OpEq, LHS: memoIndex, RHS: ir.ConstNumber{Value: -1}})
	found := g.currentFn.NewBlock("structured_clone_object_found")
	create := g.currentFn.NewBlock("structured_clone_object_create")
	done := g.currentFn.NewBlock("structured_clone_object_done")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: missing, Then: create, Else: found}

	g.currentBB = found
	boxedExisting := g.currentFn.NewValue("structured_clone_object_boxed", types.TypeAny)
	found.Instructions = append(found.Instructions, &ir.GetElementInst{Res: boxedExisting, Array: memoClones, Index: memoIndex})
	existing := g.coerceJSValueBoundary(boxedExisting, types.TypeAny, objectType)
	foundEnd := g.currentBB
	foundEnd.Terminator = &ir.JumpTerm{Target: done}

	g.currentBB = create
	offsets, refMask, shape := g.objectLayout(objectType)
	clone := g.currentFn.NewValue("structured_clone_object", objectType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
		Res: clone, Shape: shape, FieldCount: len(offsets), RefMask: refMask,
	})
	g.registerStructuredCloneMemo(source, objectType, clone, objectType, memoSources, memoClones)
	for _, name := range objectType.FieldOrder {
		field := objectType.Fields[name]
		value := g.currentFn.NewValue("structured_clone_field", field.Type)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{
			Res: value, Obj: source, Field: name, Offset: offsets[name],
		})
		copied := g.cloneStructuredValue(value, field.Type, memoSources, memoClones)
		if copied == nil {
			return nil
		}
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{
			Obj: clone, Field: name, Offset: offsets[name], Val: copied,
		})
	}
	createEnd := g.currentBB
	createEnd.Terminator = &ir.JumpTerm{Target: done}

	result := g.currentFn.NewValue("structured_clone_object_result", objectType)
	done.Phis = append(done.Phis, &ir.PhiInst{Res: result, Incoming: []ir.PhiIncoming{
		{Block: foundEnd, Value: existing},
		{Block: createEnd, Value: clone},
	}})
	g.currentBB = done
	return result
}

func (g *generator) cloneStructuredArray(source ir.Operand, arrayType *types.ArrayType, memoSources, memoClones ir.Operand) ir.Operand {
	memoIndex := g.structuredCloneMemoIndex(source, arrayType, memoSources)
	missing := g.currentFn.NewValue("structured_clone_array_missing", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: missing, Op: ir.OpEq, LHS: memoIndex, RHS: ir.ConstNumber{Value: -1}})
	found := g.currentFn.NewBlock("structured_clone_array_found")
	create := g.currentFn.NewBlock("structured_clone_array_create")
	resultDone := g.currentFn.NewBlock("structured_clone_array_result_done")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: missing, Then: create, Else: found}

	g.currentBB = found
	boxedExisting := g.currentFn.NewValue("structured_clone_array_boxed", types.TypeAny)
	found.Instructions = append(found.Instructions, &ir.GetElementInst{Res: boxedExisting, Array: memoClones, Index: memoIndex})
	existing := g.coerceJSValueBoundary(boxedExisting, types.TypeAny, arrayType)
	foundEnd := g.currentBB
	foundEnd.Terminator = &ir.JumpTerm{Target: resultDone}

	g.currentBB = create
	length := g.currentFn.NewValue("structured_clone_length", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: source})
	clone := g.currentFn.NewValue("structured_clone_array", arrayType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{
		Res: clone, ElemType: arrayType.Elem, Length: ir.ConstNumber{Value: 0},
	})
	g.registerStructuredCloneMemo(source, arrayType, clone, arrayType, memoSources, memoClones)

	pre := g.currentBB
	cond := g.currentFn.NewBlock("structured_clone_array_cond")
	body := g.currentFn.NewBlock("structured_clone_array_body")
	post := g.currentFn.NewBlock("structured_clone_array_post")
	arrayDone := g.currentFn.NewBlock("structured_clone_array_done")
	pre.Terminator = &ir.JumpTerm{Target: cond}

	index := g.currentFn.NewValue("structured_clone_array_i", types.TypeNumber)
	next := g.currentFn.NewValue("structured_clone_array_next", types.TypeNumber)
	cond.Phis = append(cond.Phis, &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{
		{Block: pre, Value: ir.ConstNumber{Value: 0}},
		{Block: post, Value: next},
	}})
	more := g.currentFn.NewValue("structured_clone_array_more", types.TypeBoolean)
	cond.Instructions = append(cond.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	cond.Terminator = &ir.BranchTerm{Cond: more, Then: body, Else: arrayDone}

	g.currentBB = body
	item := g.currentFn.NewValue("structured_clone_array_item", arrayType.Elem)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetElementInst{Res: item, Array: source, Index: index})
	copied := g.cloneStructuredValue(item, arrayType.Elem, memoSources, memoClones)
	if copied == nil {
		return nil
	}
	push := g.currentFn.NewValue("structured_clone_array_push", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayPushInst{Res: push, Array: clone, Val: copied})
	g.currentBB.Terminator = &ir.JumpTerm{Target: post}

	post.Instructions = append(post.Instructions, &ir.BinaryInst{Res: next, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
	post.Terminator = &ir.JumpTerm{Target: cond}
	arrayDone.Terminator = &ir.JumpTerm{Target: resultDone}

	result := g.currentFn.NewValue("structured_clone_array_result", arrayType)
	resultDone.Phis = append(resultDone.Phis, &ir.PhiInst{Res: result, Incoming: []ir.PhiIncoming{
		{Block: foundEnd, Value: existing},
		{Block: arrayDone, Value: clone},
	}})
	g.currentBB = resultDone
	return result
}
