package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func irJSValueType(t types.Type) bool {
	if t.Kind() == types.KindAny || t.Kind() == types.KindUnknown {
		return true
	}
	u, ok := t.(*types.UnionType)
	if !ok {
		return false
	}
	classes := map[int]bool{}
	for _, member := range u.Members {
		switch member.Kind() {
		case types.KindNull, types.KindUndefined, types.KindNever:
			continue
		case types.KindNumber:
			classes[1] = true
		case types.KindBoolean:
			classes[2] = true
		case types.KindString, types.KindArray, types.KindTuple, types.KindObject, types.KindFunction:
			classes[3] = true
		case types.KindAny, types.KindUnknown:
			return true
		}
	}
	return len(classes) > 1
}

func irHeapRefType(t types.Type) bool {
	if irJSValueType(t) {
		return true
	}
	switch t.Kind() {
	case types.KindString, types.KindArray, types.KindTuple, types.KindObject, types.KindFunction:
		return true
	default:
		return true
	}
}

func (g *generator) coerceStringType(t types.Type, op ir.Operand) ir.Operand {
	if t == types.TypeString {
		return op
	}
	if isNumberSemanticType(t) {
		res := g.currentFn.NewValue("num_str", types.TypeString)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_number_to_string", Args: []ir.Operand{op}})
		return res
	}
	if t == types.TypeBoolean {
		res := g.currentFn.NewValue("bool_str", types.TypeString)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_bool_to_string", Args: []ir.Operand{op}})
		return res
	}
	if t == types.TypeNull {
		return ir.ConstString{Value: "null"}
	}
	return ir.ConstString{Value: "undefined"}
}

func (g *generator) coerceNullableUnionString(t *types.UnionType, op ir.Operand) ir.Operand {
	var concrete types.Type
	hasNull, hasUndefined := false, false
	for _, member := range t.Members {
		switch member.Kind() {
		case types.KindNull:
			hasNull = true
		case types.KindUndefined:
			hasUndefined = true
		default:
			concrete = member
		}
	}
	if concrete == nil || (concrete != types.TypeString && concrete != types.TypeBoolean && !isNumberSemanticType(concrete)) {
		concrete = types.TypeString
	}

	join := g.currentFn.NewBlock("str_coerce_join")
	incoming := make([]ir.PhiIncoming, 0, 3)
	emitNullish := func(name string, sentinel ir.Operand, literal string) {
		cond := g.currentFn.NewValue(name+"_match", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: cond, Op: ir.OpEq, LHS: op, RHS: sentinel})
		match := g.currentFn.NewBlock(name)
		next := g.currentFn.NewBlock(name + "_next")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: cond, Then: match, Else: next}
		match.Terminator = &ir.JumpTerm{Target: join}
		incoming = append(incoming, ir.PhiIncoming{Block: match, Value: ir.ConstString{Value: literal}})
		g.currentBB = next
	}
	if hasUndefined {
		emitNullish("str_undefined", ir.ConstUndefined{}, "undefined")
	}
	if hasNull {
		emitNullish("str_null", ir.ConstNull{}, "null")
	}
	fallback := g.currentBB
	converted := g.coerceStringType(concrete, op)
	if fallback.Terminator == nil {
		fallback.Terminator = &ir.JumpTerm{Target: join}
	}
	incoming = append(incoming, ir.PhiIncoming{Block: fallback, Value: converted})
	g.currentBB = join
	res := g.currentFn.NewValue("str_coerce", types.TypeString)
	join.Phis = append(join.Phis, &ir.PhiInst{Res: res, Incoming: incoming})
	return res
}

func (g *generator) coerceStringOperand(expr ast.Expr, op ir.Operand) ir.Operand {
	t := g.semanticType(expr)
	if union, ok := t.(*types.UnionType); ok {
		return g.coerceNullableUnionString(union, op)
	}
	return g.coerceStringType(t, op)
}

func (g *generator) coerceJSValueBoundary(value ir.Operand, sourceType, targetType types.Type) ir.Operand {
	if sourceType == nil {
		sourceType = value.Type()
	}
	actualType := value.Type()

	// Primitive values entering any/unknown must use the boxed JSValue ABI.
	if irJSValueType(targetType) {
		if actualType != nil && irJSValueType(actualType) {
			return value
		}
		switch sourceType.Kind() {
		case types.KindNumber, types.KindString, types.KindBoolean,
			types.KindArray, types.KindTuple, types.KindObject, types.KindFunction:
			return g.boxJSValue(value, sourceType)
		case types.KindNull, types.KindUndefined:
			return value
		default:
			// Reference boxing is handled separately so existing closed-shape
			// provenance remains available until the dynamic-reference milestone.
			return value
		}
	}

	// Primitive typed consumers decode values that crossed an any boundary.
	if irJSValueType(sourceType) || (actualType != nil && irJSValueType(actualType)) {
		var callee string
		switch targetType.Kind() {
		case types.KindNumber:
			callee = "ts_js_unbox_number"
		case types.KindString:
			callee = "ts_js_unbox_string"
		case types.KindBoolean:
			callee = "ts_js_unbox_bool"
		case types.KindArray, types.KindTuple, types.KindObject, types.KindFunction:
			callee = "ts_js_unbox_ref"
		default:
			return value
		}
		res := g.currentFn.NewValue("js_unbox", targetType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: res, Callee: callee, Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeAny},
		})
		return res
	}
	return value
}

func (g *generator) coerceCallOperands(args []ir.Operand, sourceTypes []types.Type, fnType *types.FunctionType) []ir.Operand {
	if fnType == nil {
		return args
	}
	restIndex := restParamIndex(fnType.Params)
	for i := range args {
		var target types.Type
		if restIndex >= 0 && i >= restIndex {
			arr, ok := fnType.Params[restIndex].Type.(*types.ArrayType)
			if ok {
				target = arr.Elem
			}
		} else if i < len(fnType.Params) {
			target = fnType.Params[i].Type
		}
		source := sourceTypes[i]
		args[i] = g.coerceJSValueBoundary(args[i], source, target)
	}
	return args
}

func (g *generator) boxJSValue(value ir.Operand, sourceType types.Type) ir.Operand {
	if irJSValueType(sourceType) {
		return value
	}
	switch sourceType.Kind() {
	case types.KindNull, types.KindUndefined:
		return value
	case types.KindNumber:
		res := g.currentFn.NewValue("js_num", types.TypeAny)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_js_box_number", Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeNumber}})
		return res
	case types.KindBoolean:
		res := g.currentFn.NewValue("js_bool", types.TypeAny)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_js_box_bool", Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeBoolean}})
		return res
	case types.KindString:
		res := g.currentFn.NewValue("js_str", types.TypeAny)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_js_box_string", Args: []ir.Operand{value}, ParamTypes: []types.Type{sourceType}})
		return res
	default:
		res := g.currentFn.NewValue("js_ref", types.TypeAny)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_js_box_ref", Args: []ir.Operand{value}, ParamTypes: []types.Type{sourceType}})
		return res
	}
}
