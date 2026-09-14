package irgen

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
	"github.com/phongsathornpt/ts-pro/internal/frontend/sema"
)

func consolePrinterForType(t types.Type) (string, bool) {
	switch t {
	case types.TypeNumber:
		return "ts_print_val", true
	case types.TypeString:
		return "ts_print_str", true
	case types.TypeBoolean:
		return "ts_print_bool", true
	case types.TypeUndefined:
		return "ts_print_undefined", true
	case types.TypeNull:
		return "ts_print_null", true
	default:
		return "", false
	}
}

func (g *generator) staticStringKey(expr ast.Expr) (string, bool) {
	switch key := expr.(type) {
	case *ast.StringLit:
		return key.Value, true
	case *ast.IdentExpr:
		if op, ok := g.locals[key.Name]; ok {
			if value, ok := op.(ir.ConstString); ok {
				return value.Value, true
			}
		}
	}
	return "", false
}

func (g *generator) concatNativeStrings(a, b ir.Operand) ir.Operand {
	res := g.currentFn.NewValue("json_str", types.TypeString)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_string_concat", Args: []ir.Operand{a, b}})
	return res
}

func jsonConstantType(v any) types.Type {
	switch x := v.(type) {
	case nil:
		return types.TypeNull
	case bool:
		return types.TypeBoolean
	case float64:
		return types.TypeNumber
	case string:
		return types.TypeString
	case []any:
		var elem types.Type = types.TypeAny
		if len(x) > 0 {
			elem = jsonConstantType(x[0])
		}
		return types.NewArray(elem)
	default:
		m := v.(map[string]any)
		obj := types.NewObject("")
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			obj.AddField(k, jsonConstantType(m[k]), false)
		}
		return obj
	}
}

func (g *generator) lowerJSONConstant(v any) ir.Operand {
	switch x := v.(type) {
	case nil:
		return ir.ConstNull{}
	case bool:
		return ir.ConstBool{Value: x}
	case float64:
		return ir.ConstNumber{Value: x}
	case string:
		return ir.ConstString{Value: x}
	case []any:
		arrType := jsonConstantType(x).(*types.ArrayType)
		res := g.currentFn.NewValue("json_array", arrType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{Res: res, ElemType: arrType.Elem, Length: ir.ConstNumber{Value: float64(len(x))}})
		for i, item := range x {
			value := g.lowerJSONConstant(item)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetElementInst{Array: res, Index: ir.ConstNumber{Value: float64(i)}, Val: value})
		}
		return res
	default:
		m := v.(map[string]any)
		objType := jsonConstantType(m).(*types.ObjectType)
		offsets, refMask, shape := g.objectLayout(objType)
		res := g.currentFn.NewValue("json_object", objType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{Res: res, Shape: shape, FieldCount: len(offsets), RefMask: refMask})
		for _, name := range objType.FieldOrder {
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: res, Field: name, Offset: offsets[name], Val: g.lowerJSONConstant(m[name])})
		}
		return res
	}
}

func (g *generator) lowerJSONStringifyValue(value ir.Operand, t types.Type) ir.Operand {
	switch t {
	case types.TypeNumber:
		res := g.currentFn.NewValue("json_number", types.TypeString)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_number_to_string", Args: []ir.Operand{value}})
		return res
	case types.TypeString:
		return g.concatNativeStrings(g.concatNativeStrings(ir.ConstString{Value: "\""}, value), ir.ConstString{Value: "\""})
	case types.TypeBoolean:
		res := g.currentFn.NewValue("json_bool", types.TypeString)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_bool_to_string", Args: []ir.Operand{value}})
		return res
	case types.TypeNull:
		return ir.ConstString{Value: "null"}
	}
	if arr, ok := t.(*types.ArrayType); ok {
		return g.lowerJSONStringifyArray(value, arr)
	}
	if obj, ok := t.(*types.ObjectType); ok {
		offsets, _, _ := g.objectLayout(obj)
		acc := ir.Operand(ir.ConstString{Value: "{"})
		for i, name := range obj.FieldOrder {
			prefix := "\"" + name + "\":"
			if i > 0 {
				prefix = "," + prefix
			}
			acc = g.concatNativeStrings(acc, ir.ConstString{Value: prefix})
			field := obj.Fields[name]
			fv := g.currentFn.NewValue("json_field", field.Type)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: fv, Obj: value, Field: name, Offset: offsets[name]})
			acc = g.concatNativeStrings(acc, g.lowerJSONStringifyValue(fv, field.Type))
		}
		return g.concatNativeStrings(acc, ir.ConstString{Value: "}"})
	}
	return ir.ConstString{Value: "{}"}
}

func (g *generator) lowerJSONStringifyArray(array ir.Operand, arr *types.ArrayType) ir.Operand {
	start := g.currentBB
	length := g.currentFn.NewValue("json_len", types.TypeNumber)
	start.Instructions = append(start.Instructions, &ir.ArrayLengthInst{Res: length, Array: array})
	hasAny := g.currentFn.NewValue("json_has", types.TypeBoolean)
	start.Instructions = append(start.Instructions, &ir.BinaryInst{Res: hasAny, Op: ir.OpGt, LHS: length, RHS: ir.ConstNumber{Value: 0}})
	nonEmpty := g.currentFn.NewBlock("json_array_nonempty")
	empty := g.currentFn.NewBlock("json_array_empty")
	cond := g.currentFn.NewBlock("json_array_cond")
	body := g.currentFn.NewBlock("json_array_body")
	done := g.currentFn.NewBlock("json_array_done")
	join := g.currentFn.NewBlock("json_array_join")
	start.Terminator = &ir.BranchTerm{Cond: hasAny, Then: nonEmpty, Else: empty}
	empty.Terminator = &ir.JumpTerm{Target: join}

	g.currentBB = nonEmpty
	first := g.currentFn.NewValue("json_elem0", arr.Elem)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetElementInst{Res: first, Array: array, Index: ir.ConstNumber{Value: 0}})
	firstText := g.lowerJSONStringifyValue(first, arr.Elem)
	acc0 := g.concatNativeStrings(ir.ConstString{Value: "["}, firstText)
	nonEmptyEnd := g.currentBB
	nonEmptyEnd.Terminator = &ir.JumpTerm{Target: cond}

	index := g.currentFn.NewValue("json_i", types.TypeNumber)
	acc := g.currentFn.NewValue("json_acc", types.TypeString)
	cond.Phis = append(cond.Phis, &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{{Block: nonEmptyEnd, Value: ir.ConstNumber{Value: 1}}}}, &ir.PhiInst{Res: acc, Incoming: []ir.PhiIncoming{{Block: nonEmptyEnd, Value: acc0}}})
	more := g.currentFn.NewValue("json_more", types.TypeBoolean)
	cond.Instructions = append(cond.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	cond.Terminator = &ir.BranchTerm{Cond: more, Then: body, Else: done}

	g.currentBB = body
	elem := g.currentFn.NewValue("json_elem", arr.Elem)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetElementInst{Res: elem, Array: array, Index: index})
	text := g.lowerJSONStringifyValue(elem, arr.Elem)
	commaText := g.concatNativeStrings(ir.ConstString{Value: ","}, text)
	nextAcc := g.concatNativeStrings(acc, commaText)
	next := g.currentFn.NewValue("json_next", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: next, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
	bodyEnd := g.currentBB
	bodyEnd.Terminator = &ir.JumpTerm{Target: cond}
	cond.Phis[0].Incoming = append(cond.Phis[0].Incoming, ir.PhiIncoming{Block: bodyEnd, Value: next})
	cond.Phis[1].Incoming = append(cond.Phis[1].Incoming, ir.PhiIncoming{Block: bodyEnd, Value: nextAcc})

	g.currentBB = done
	final := g.concatNativeStrings(acc, ir.ConstString{Value: "]"})
	doneEnd := g.currentBB
	doneEnd.Terminator = &ir.JumpTerm{Target: join}
	g.currentBB = join
	result := g.currentFn.NewValue("json_result", types.TypeString)
	join.Phis = append(join.Phis, &ir.PhiInst{Res: result, Incoming: []ir.PhiIncoming{{Block: empty, Value: ir.ConstString{Value: "[]"}}, {Block: doneEnd, Value: final}}})
	return result
}

func (g *generator) lowerJSONCall(call *ast.CallExpr, mem *ast.MemberExpr) ir.Operand {
	switch mem.Property {
	case "parse":
		if lit, ok := call.Args[0].(*ast.StringLit); ok {
			var decoded any
			if err := json.Unmarshal([]byte(lit.Value), &decoded); err != nil {
				g.routeThrownValue(g.newWebError(ir.ConstString{Value: "Unexpected token in JSON"}, ir.ConstString{Value: "SyntaxError"}))
				return ir.ConstUndefined{}
			}
			value := g.lowerJSONConstant(decoded)
			switch decoded.(type) {
			case []any, map[string]any:
				return value
			default:
				return g.boxJSValue(value, value.Type())
			}
		}
		text := g.lowerExpr(call.Args[0])
		return g.lowerRuntimeJSONParse(text)
	default:
		value := g.lowerExpr(call.Args[0])
		return g.lowerJSONStringifyValue(value, value.Type())
	}
}

func isBuiltinRegExpType(t types.Type) bool {
	obj, ok := t.(*types.ObjectType)
	return ok && obj.Name == "$RegExp"
}

func classifyNativeRegExp(pattern, flags string) (kind float64, needle string, flagBits float64, err error) {
	for _, flag := range flags {
		if flag == 'i' {
			flagBits = 1
			continue
		}
		return 0, "", 0, fmt.Errorf("unsupported native RegExp flag %q", flag)
	}
	if strings.HasPrefix(pattern, "^") {
		needle = pattern[1:]
		if strings.ContainsAny(needle, `.*+?[](){}|^$\\`) {
			return 0, "", 0, fmt.Errorf("unsupported anchored RegExp pattern %q", pattern)
		}
		return 1, needle, flagBits, nil
	}
	if strings.HasSuffix(pattern, `\d+`) {
		needle = strings.TrimSuffix(pattern, `\d+`)
		if needle == "" || strings.ContainsAny(needle, `.*+?[](){}|^$\\`) {
			return 0, "", 0, fmt.Errorf("unsupported digit RegExp pattern %q", pattern)
		}
		return 2, needle, flagBits, nil
	}
	if strings.ContainsAny(pattern, `.*+?[](){}|^$\\`) {
		return 0, "", 0, fmt.Errorf("unsupported native RegExp pattern %q", pattern)
	}
	return 0, pattern, flagBits, nil
}

func (g *generator) lowerNativeRegExp(pattern, flags string, resultType types.Type) ir.Operand {
	kind, needle, flagBits, err := classifyNativeRegExp(pattern, flags)
	if err != nil {
		return g.failExpr("%v", err)
	}
	res := g.currentFn.NewValue("regexp", resultType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{Res: res, Shape: "$RegExp", FieldCount: 4, RefMask: 0b0011})
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: res, Field: "source", Offset: 16, Val: ir.ConstString{Value: pattern}},
		&ir.SetFieldInst{Obj: res, Field: "needle", Offset: 24, Val: ir.ConstString{Value: needle}},
		&ir.SetFieldInst{Obj: res, Field: "kind", Offset: 32, Val: ir.ConstNumber{Value: kind}},
		&ir.SetFieldInst{Obj: res, Field: "flags", Offset: 40, Val: ir.ConstNumber{Value: flagBits}},
	)
	return res
}

func (g *generator) emitRegExpTest(call *ast.CallExpr, mem *ast.MemberExpr) ir.Operand {
	obj := g.lowerExpr(mem.Object)
	text := g.lowerExpr(call.Args[0])
	res := g.currentFn.NewValue("regexp_test", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_regexp_test", Args: []ir.Operand{obj, text}})
	return res
}

func isBuiltinDateType(t types.Type) bool {
	obj, ok := t.(*types.ObjectType)
	return ok && obj.Name == "$Date"
}

func (g *generator) emitDateMethodCall(call *ast.CallExpr, mem *ast.MemberExpr) ir.Operand {
	date := g.lowerExpr(mem.Object)
	callee := map[string]string{
		"toISOString":    "ts_date_to_iso",
		"getUTCFullYear": "ts_date_get_year",
		"getUTCMonth":    "ts_date_get_month",
		"getUTCDate":     "ts_date_get_date",
		"getUTCHours":    "ts_date_get_hours",
		"getUTCMinutes":  "ts_date_get_minutes",
		"getUTCSeconds":  "ts_date_get_seconds",
	}[mem.Property]
	resultType := g.semanticType(call)
	res := g.currentFn.NewValue("date_result", resultType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: callee, Args: []ir.Operand{date}})
	return res
}

func (g *generator) builtinCollectionInfo(t types.Type) *sema.BuiltinCollectionInfo {
	obj, ok := t.(*types.ObjectType)
	if !ok || g.semaResult == nil {
		return nil
	}
	return g.semaResult.BuiltinCollections[obj.Name]
}

func (g *generator) emitBuiltinCollectionCall(call *ast.CallExpr, mem *ast.MemberExpr, info *sema.BuiltinCollectionInfo) ir.Operand {
	obj := g.lowerExpr(mem.Object)
	boxArg := func(i int) ir.Operand {
		v := g.lowerExpr(call.Args[i])
		return g.boxJSValue(v, g.semanticType(call.Args[i]))
	}
	resultType := g.semanticType(call)
	switch info.Kind + "." + mem.Property {
	case "Map.set":
		res := g.currentFn.NewValue("map", info.Instance)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_collection_set", Args: []ir.Operand{obj, boxArg(0), boxArg(1)}})
		return res
	case "Set.add":
		res := g.currentFn.NewValue("set", info.Instance)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_collection_set", Args: []ir.Operand{obj, boxArg(0), ir.ConstUndefined{}}})
		return res
	case "Map.get":
		res := g.currentFn.NewValue("map_value", resultType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_collection_get", Args: []ir.Operand{obj, boxArg(0)}})
		return res
	case "Map.has", "Set.has":
		res := g.currentFn.NewValue("has", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_collection_has", Args: []ir.Operand{obj, boxArg(0)}})
		return res
	case "Map.delete", "Set.delete":
		res := g.currentFn.NewValue("deleted", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_collection_delete", Args: []ir.Operand{obj, boxArg(0)}})
		return res
	default:
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_collection_clear", Args: []ir.Operand{obj}})
		return nil
	}
}

func (g *generator) lowerConsoleLog(expr ast.Expr) ir.Operand {
	t := g.semanticType(expr)
	if t == types.TypeAny {
		value := g.lowerExpr(expr)
		if actual := value.Type(); actual != nil && actual != types.TypeAny {
			if callee, ok := consolePrinterForType(actual); ok {
				if actual == types.TypeUndefined || actual == types.TypeNull {
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: callee})
					return nil
				}
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: callee, Args: []ir.Operand{value}, ParamTypes: []types.Type{actual}})
				return nil
			}
		}
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_js_print", Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeAny}})
		return nil
	}
	if irJSValueType(t) {
		value := g.lowerExpr(expr)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_js_print", Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeAny}})
		return nil
	}
	if callee, ok := consolePrinterForType(t); ok {
		if t == types.TypeUndefined || t == types.TypeNull {
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: callee})
			return nil
		}
		value := g.lowerExpr(expr)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: callee, Args: []ir.Operand{value}, ParamTypes: []types.Type{t}})
		return nil
	}
	union, ok := t.(*types.UnionType)
	if !ok {
		value := g.lowerExpr(expr)
		boxed := g.boxJSValue(value, t)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_js_print", Args: []ir.Operand{boxed}, ParamTypes: []types.Type{types.TypeAny}})
		return nil
	}
	var concrete types.Type
	hasNull, hasUndefined := false, false
	for _, member := range union.Members {
		switch member {
		case types.TypeNull:
			hasNull = true
		case types.TypeUndefined:
			hasUndefined = true
		default:
			concrete = member
		}
	}
	printer, _ := consolePrinterForType(concrete)
	value := g.lowerExpr(expr)
	if actual := value.Type(); actual != nil && irJSValueType(actual) {
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_js_print", Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeAny}})
		return nil
	}
	join := g.currentFn.NewBlock("print_join")
	emitMissing := func(name string, sentinel ir.Operand, callee string) {
		cond := g.currentFn.NewValue(name+"_match", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: cond, Op: ir.OpEq, LHS: value, RHS: sentinel})
		printBB := g.currentFn.NewBlock(name)
		nextBB := g.currentFn.NewBlock(name + "_next")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: cond, Then: printBB, Else: nextBB}
		printBB.Instructions = append(printBB.Instructions, &ir.CallInst{Callee: callee})
		printBB.Terminator = &ir.JumpTerm{Target: join}
		g.currentBB = nextBB
	}
	if hasUndefined {
		emitMissing("print_undefined", ir.ConstUndefined{}, "ts_print_undefined")
	}
	if hasNull {
		emitMissing("print_null", ir.ConstNull{}, "ts_print_null")
	}
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: printer, Args: []ir.Operand{value}, ParamTypes: []types.Type{concrete}})
	g.currentBB.Terminator = &ir.JumpTerm{Target: join}
	g.currentBB = join
	return nil
}

func isConsoleLogCall(expr ast.Expr) bool {
	mem, ok := expr.(*ast.MemberExpr)
	if !ok || mem.Property != "log" {
		return false
	}
	ident, ok := mem.Object.(*ast.IdentExpr)
	return ok && ident.Name == "console"
}
