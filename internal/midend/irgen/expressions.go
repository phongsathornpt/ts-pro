package irgen

import (
	"strconv"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/token"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) lowerExpr(expr ast.Expr) ir.Operand {

	switch e := expr.(type) {
	case *ast.NumberLit:
		return ir.ConstNumber{Value: e.Value}
	case *ast.StringLit:
		return ir.ConstString{Value: e.Value}
	case *ast.RegexLit:
		return g.lowerNativeRegExp(e.Pattern, e.Flags, g.semanticType(e))
	case *ast.BoolLit:
		return ir.ConstBool{Value: e.Value}
	case *ast.NullLit:
		return ir.ConstNull{}
	case *ast.UndefinedLit:
		return ir.ConstUndefined{}
	case *ast.ThisExpr:
		return g.locals["$this"]

	case *ast.NewExpr:
		return g.lowerNewExpr(e)
	case *ast.ArrayLit:
		arrType := types.NewArray(types.TypeAny)
		if semantic := g.semanticType(e); semantic != nil {
			if tuple, ok := semantic.(*types.TupleType); ok {
				res := g.currentFn.NewValue("tuple", tuple)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
					Res: res, Shape: tuple.String(), FieldCount: len(tuple.Elements), RefMask: g.tupleRefMask(tuple),
				})
				for i, el := range e.Elements {
					val := g.lowerExpr(el)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{
						Obj: res, Field: strconv.Itoa(i), Offset: 16 + i*8, Val: val,
					})
				}
				return res
			}
			if t, ok := semantic.(*types.ArrayType); ok {
				arrType = t
			}
		}
		hasSpread := false
		for _, el := range e.Elements {
			if _, ok := el.(*ast.SpreadExpr); ok {
				hasSpread = true
				break
			}
		}
		res := g.currentFn.NewValue("arr", arrType)
		initialLength := float64(len(e.Elements))
		if hasSpread {
			initialLength = 0
		}
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{
			Res: res, ElemType: arrType.Elem, Length: ir.ConstNumber{Value: initialLength},
		})
		for i, el := range e.Elements {
			if spread, ok := el.(*ast.SpreadExpr); ok {
				sourceType := g.semanticType(spread.Value).(*types.ArrayType)
				source := g.lowerExpr(spread.Value)
				g.appendSpreadArray(res, source, sourceType.Elem, arrType.Elem)
				continue
			}
			val := g.lowerExpr(el)
			stored := val
			if irJSValueType(arrType.Elem) {
				stored = g.boxJSValue(val, g.semanticType(el))
			}
			if hasSpread {
				g.pushArrayOperand(res, stored)
			} else {
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetElementInst{
					Array: res, Index: ir.ConstNumber{Value: float64(i)}, Val: stored,
				})
			}
		}
		return res
	case *ast.ObjectLit:
		objType := g.semanticType(e).(*types.ObjectType)
		offsets, refMask, shape := g.objectLayout(objType)
		res := g.currentFn.NewValue("obj", objType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{Res: res, Shape: shape, FieldCount: len(offsets), RefMask: refMask})
		written := make(map[string]bool, len(objType.Fields))
		for _, prop := range e.Properties {
			if prop.Spread {
				sourceType := g.semanticType(prop.Value).(*types.ObjectType)
				source := g.lowerExpr(prop.Value)
				sourceOffsets, _, _ := g.objectLayout(sourceType)
				for _, name := range sourceType.FieldOrder {
					field := sourceType.Fields[name]
					value := g.currentFn.NewValue("spread_field", field.Type)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: value, Obj: source, Field: name, Offset: sourceOffsets[name]})
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: res, Field: name, Offset: offsets[name], Val: value})
					written[name] = true
				}
				continue
			}
			val := g.lowerExpr(prop.Value)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: res, Field: prop.Key, Offset: offsets[prop.Key], Val: val})
			written[prop.Key] = true
		}
		for _, name := range objType.FieldOrder {
			field := objType.Fields[name]
			if written[name] || !field.Optional {
				continue
			}
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: res, Field: name, Offset: offsets[name], Val: ir.ConstUndefined{}})
		}
		return res
	case *ast.IdentExpr:
		if e.Name == "globalThis" || e.Name == "self" {
			t := g.semanticType(e)
			res := g.currentFn.NewValue("global_scope", t)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_global_object"})
			return res
		}
		if _, exists := g.locals[e.Name]; exists {
			return g.readLocal(e.Name)
		}
		sym := g.semaResult.Symbols[e]
		fnType := sym.Type.(*types.FunctionType)
		res := g.currentFn.NewValue("closure", fnType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{Res: res, Function: e.Name})
		return res
	case *ast.BinaryExpr:
		if e.Op == token.AmpAmp || e.Op == token.PipePipe {
			return g.lowerLogicalExpr(e)
		}
		if e.Op == token.QuestionQuestion {
			return g.lowerNullishExpr(e)
		}
		if g.canFuseStringConcat(e) {
			if fused, ok := g.lowerStringConcatChain(e); ok {
				return fused
			}
		}

		lhs := g.lowerExpr(e.Left)
		rhs := g.lowerExpr(e.Right)

		if e.Op == token.Plus && (irJSValueType(g.semanticType(e.Left)) || irJSValueType(g.semanticType(e.Right))) {
			if !irJSValueType(lhs.Type()) {
				lhs = g.boxJSValue(lhs, lhs.Type())
			}
			if !irJSValueType(rhs.Type()) {
				rhs = g.boxJSValue(rhs, rhs.Type())
			}
			resVal := g.currentFn.NewValue("js_add", types.TypeAny)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
				Res: resVal, Callee: "ts_js_add", Args: []ir.Operand{lhs, rhs}, ParamTypes: []types.Type{types.TypeAny, types.TypeAny},
			})
			return resVal
		}

		if (e.Op == token.Minus || e.Op == token.Star || e.Op == token.Slash || e.Op == token.Percent) &&
			(irJSValueType(g.semanticType(e.Left)) || irJSValueType(g.semanticType(e.Right))) {
			if !irJSValueType(lhs.Type()) {
				lhs = g.boxJSValue(lhs, lhs.Type())
			}
			if !irJSValueType(rhs.Type()) {
				rhs = g.boxJSValue(rhs, rhs.Type())
			}
			callee := "ts_js_sub"
			switch e.Op {
			case token.Star:
				callee = "ts_js_mul"
			case token.Slash:
				callee = "ts_js_div"
			case token.Percent:
				callee = "ts_js_mod"
			}
			resVal := g.currentFn.NewValue("js_num_op", types.TypeNumber)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
				Res: resVal, Callee: callee, Args: []ir.Operand{lhs, rhs}, ParamTypes: []types.Type{types.TypeAny, types.TypeAny},
			})
			return resVal
		}

		if (e.Op == token.EqEq || e.Op == token.EqEqEq || e.Op == token.BangEq || e.Op == token.BangEqEq) &&
			(irJSValueType(g.semanticType(e.Left)) || irJSValueType(g.semanticType(e.Right))) {
			if !irJSValueType(lhs.Type()) {
				lhs = g.boxJSValue(lhs, lhs.Type())
			}
			if !irJSValueType(rhs.Type()) {
				rhs = g.boxJSValue(rhs, rhs.Type())
			}
			callee := "ts_js_loose_eq"
			if e.Op == token.EqEqEq || e.Op == token.BangEqEq {
				callee = "ts_js_strict_eq"
			}
			eq := g.currentFn.NewValue("js_eq", types.TypeBoolean)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
				Res: eq, Callee: callee, Args: []ir.Operand{lhs, rhs}, ParamTypes: []types.Type{types.TypeAny, types.TypeAny},
			})
			if e.Op == token.BangEq || e.Op == token.BangEqEq {
				resVal := g.currentFn.NewValue("js_ne", types.TypeBoolean)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: resVal, Op: ir.OpEq, LHS: eq, RHS: ir.ConstBool{Value: false}})
				return resVal
			}
			return eq
		}

		if (e.Op == token.Lt || e.Op == token.LtEq || e.Op == token.Gt || e.Op == token.GtEq) &&
			(irJSValueType(g.semanticType(e.Left)) || irJSValueType(g.semanticType(e.Right)) ||
				g.semanticType(e.Left) == types.TypeString || g.semanticType(e.Right) == types.TypeString) {
			if !irJSValueType(lhs.Type()) {
				lhs = g.boxJSValue(lhs, lhs.Type())
			}
			if !irJSValueType(rhs.Type()) {
				rhs = g.boxJSValue(rhs, rhs.Type())
			}
			callee := "ts_js_lt"
			switch e.Op {
			case token.LtEq:
				callee = "ts_js_le"
			case token.Gt:
				callee = "ts_js_gt"
			case token.GtEq:
				callee = "ts_js_ge"
			}
			resVal := g.currentFn.NewValue("js_rel", types.TypeBoolean)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
				Res: resVal, Callee: callee, Args: []ir.Operand{lhs, rhs}, ParamTypes: []types.Type{types.TypeAny, types.TypeAny},
			})
			return resVal
		}

		if e.Op == token.Plus {
			isString := false
			if g.semanticType(e) == types.TypeString || g.semanticType(e.Left) == types.TypeString || g.semanticType(e.Right) == types.TypeString {
				isString = true
			}
			if isString {
				lhs = g.coerceStringOperand(e.Left, lhs)
				rhs = g.coerceStringOperand(e.Right, rhs)
				resVal := g.currentFn.NewValue("str", types.TypeString)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: resVal, Callee: "ts_string_concat", Args: []ir.Operand{lhs, rhs}})
				return resVal
			}
		}

		op := ir.OpAdd
		switch e.Op {
		case token.Plus:
			op = ir.OpAdd
		case token.Minus:
			op = ir.OpSub
		case token.Star:
			op = ir.OpMul
		case token.Slash:
			op = ir.OpDiv
		case token.Percent:
			op = ir.OpMod
		case token.EqEq, token.EqEqEq:
			op = ir.OpEq
		case token.BangEq, token.BangEqEq:
			op = ir.OpNe
		case token.Lt:
			op = ir.OpLt
		case token.LtEq:
			op = ir.OpLe
		case token.Gt:
			op = ir.OpGt
		case token.GtEq:
			op = ir.OpGe
		case token.Pipe:
			op = ir.OpOr
		case token.Amp:
			op = ir.OpAnd
		default:
			op = ir.OpAnd
		}
		resultType := types.TypeNumber
		if t := g.semanticType(e); t != nil {
			resultType = t
		}
		resVal := g.currentFn.NewValue("t", resultType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
			Res: resVal,
			Op:  op,
			LHS: lhs,
			RHS: rhs,
		})
		return resVal
	case *ast.ArrowFuncExpr:
		return g.lowerArrowExpr(e)
	case *ast.FunctionExpr:
		return g.lowerFunctionExpr(e)
	case *ast.AwaitExpr:
		task := g.lowerExpr(e.Target)
		resultType := g.semanticType(e)
		var res ir.Operand
		if resultType == nil || resultType.Kind() == types.KindVoid {
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_task_join", Args: []ir.Operand{task}})
		} else {
			v := g.currentFn.NewValue("await_result", resultType)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: v, Callee: "ts_task_join", Args: []ir.Operand{task}})
			res = v
		}
		rejected := g.currentFn.NewValue("await_rejected", types.TypeBoolean)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: rejected, Callee: "ts_task_rejected", Args: []ir.Operand{task}})
		okBB := g.currentFn.NewBlock("await_ok")
		rejectBB := g.currentFn.NewBlock("await_reject")
		g.currentBB.Terminator = &ir.BranchTerm{Cond: rejected, Then: rejectBB, Else: okBB}
		g.currentBB = rejectBB
		errVal := g.currentFn.NewValue("await_error", types.TypeAny)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: errVal, Callee: "ts_task_error", Args: []ir.Operand{task}})
		g.routeThrownValue(errVal)
		g.currentBB = okBB
		return res
	case *ast.UnaryExpr:
		if e.Op == token.PlusPlus || e.Op == token.MinusMinus {
			if ident, ok := e.Target.(*ast.IdentExpr); ok {
				currVal := g.readLocal(ident.Name)
				op := ir.OpAdd
				if e.Op == token.MinusMinus {
					op = ir.OpSub
				}
				nextVal := g.currentFn.NewValue(ident.Name+"_inc", types.TypeNumber)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
					Res: nextVal,
					Op:  op,
					LHS: currVal,
					RHS: ir.ConstNumber{Value: 1},
				})
				if !g.writeCapturedLocal(ident.Name, nextVal) {
					g.locals[ident.Name] = nextVal
				}
				if e.Prefix {
					return nextVal
				}
				return currVal
			}
		} else if e.Op == token.Minus {
			target := g.lowerExpr(e.Target)
			if c, ok := target.(ir.ConstNumber); ok {
				return ir.ConstNumber{Value: -c.Value}
			}
			resVal := g.currentFn.NewValue("neg", types.TypeNumber)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.UnaryInst{Res: resVal, Op: "-", Val: target})
			return resVal
		} else if e.Op == token.Bang {
			target := g.lowerExpr(e.Target)
			resVal := g.currentFn.NewValue("not", types.TypeBoolean)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{
				Res: resVal,
				Op:  ir.OpEq,
				LHS: target,
				RHS: ir.ConstNumber{Value: 0},
			})
			return resVal
		}
		return g.lowerExpr(e.Target)
	case *ast.IndexExpr:
		if obj, ok := g.semanticType(e.Target).(*types.ObjectType); ok && obj.Name == "$Uint8Array" {
			target := g.lowerExpr(e.Target)
			index := g.lowerExpr(e.Index)
			data := g.uint8ArrayField(target, "$data", g.semaResult.ByteBufferType)
			offset := g.uint8ArrayField(target, "byteOffset", types.TypeNumber)
			actual := g.currentFn.NewValue("uint8_index", types.TypeNumber)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: actual, Op: ir.OpAdd, LHS: offset, RHS: index})
			res := g.currentFn.NewValue("uint8_value", types.TypeNumber)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: res, Callee: "ts_byte_buffer_get", Args: []ir.Operand{data, actual}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber}})
			return res
		}
		if key, ok := g.staticStringKey(e.Index); ok {
			target := g.lowerExpr(e.Target)
			if object, ok := target.Type().(*types.ObjectType); ok {
				offsets, _, _ := g.objectLayout(object)
				offset := offsets[key]
				resultType := object.Fields[key].Type
				if semantic := g.semanticType(e); semantic != nil && semantic != types.TypeAny {
					resultType = semantic
				}
				res := g.currentFn.NewValue("computed_field", resultType)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: res, Obj: target, Field: key, Offset: offset})
				return res
			}
			if target.Type() == types.TypeAny {
				if concrete, ok := g.provenObjectType(e.Target); ok {
					offsets, _, _ := g.objectLayout(concrete)
					if offset, exists := offsets[key]; exists {
						field := concrete.Fields[key]
						raw := g.unboxKnownObject(target, concrete)
						res := g.currentFn.NewValue("computed_any_field", field.Type)
						g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: res, Obj: raw, Field: key, Offset: offset})
						return res
					}
					return ir.ConstUndefined{}
				}
				return g.lowerDynamicGet(target, key)
			}
		}
		if tuple, ok := g.semanticType(e.Target).(*types.TupleType); ok {
			lit := e.Index.(*ast.NumberLit)
			idx := int(lit.Value)
			tupleVal := g.lowerExpr(e.Target)
			resultType := tuple.Elements[idx]
			if t := g.semanticType(e); t != nil {
				resultType = t
			}
			res := g.currentFn.NewValue("tuple_elem", resultType)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{
				Res: res, Obj: tupleVal, Field: strconv.Itoa(idx), Offset: 16 + idx*8,
			})
			return res
		}
		array := g.lowerExpr(e.Target)
		index := g.lowerExpr(e.Index)
		resultType := types.TypeAny
		if t := g.semanticType(e); t != nil {
			resultType = t
		}
		res := g.currentFn.NewValue("elem", resultType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetElementInst{Res: res, Array: array, Index: index})
		return res
	case *ast.MemberExpr:
		return g.lowerMemberExpr(e)
	case *ast.CallExpr:
		return g.lowerCallExpr(e)
	case *ast.AssignExpr:
		if mem, ok := e.Left.(*ast.MemberExpr); ok {
			if objType, ok := g.semanticType(mem.Object).(*types.ObjectType); ok && objType.Name == "$URL" {
				if e.Op != token.Eq {
					return g.failExpr("compound assignment to URL.%s is not supported", mem.Property)
				}
				url := g.lowerExpr(mem.Object)
				rhs := g.lowerExpr(e.Right)
				return g.lowerURLMemberAssignment(url, mem.Property, rhs)
			}
			if objType, ok := g.semanticType(mem.Object).(*types.ObjectType); ok && objType.Name == "$AbortSignal" && mem.Property == "onabort" {
				if e.Op != token.Eq {
					return g.failExpr("compound assignment to AbortSignal.onabort is not supported")
				}
				signal := g.lowerExpr(mem.Object)
				rhs := g.lowerExpr(e.Right)
				fnType := types.NewFunction([]types.Param{{Name: "event", Type: g.semaResult.EventType}}, types.TypeVoid)
				oldHandler := g.currentFn.NewValue("abort_old_handler", fnType)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: oldHandler, Callee: "ts_abort_signal_onabort", Args: []ir.Operand{signal}})
				removeBB := g.currentFn.NewBlock("abort_onabort_remove")
				setBB := g.currentFn.NewBlock("abort_onabort_set")
				g.currentBB.Terminator = &ir.BranchTerm{Cond: oldHandler, Then: removeBB, Else: setBB}
				removeBB.Instructions = append(removeBB.Instructions, &ir.CallInst{Callee: "ts_event_target_remove", Args: []ir.Operand{signal, ir.ConstString{Value: "abort"}, oldHandler, ir.ConstBool{Value: false}}})
				removeBB.Terminator = &ir.JumpTerm{Target: setBB}
				g.currentBB = setBB
				handler := rhs
				isNull := false
				if _, ok := e.Right.(*ast.NullLit); ok {
					isNull = true
					handler = g.nullRef(fnType)
				} else if irJSValueType(rhs.Type()) {
					handler = g.coerceJSValueBoundary(rhs, rhs.Type(), fnType)
				}
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_abort_signal_set_onabort", Args: []ir.Operand{signal, handler}})
				if !isNull {
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_event_target_add", Args: []ir.Operand{signal, ir.ConstString{Value: "abort"}, handler, ir.ConstBool{Value: false}, ir.ConstBool{Value: false}, ir.ConstBool{Value: false}}})
				}
				return rhs
			}

			if objType, ok := g.semanticType(mem.Object).(*types.ObjectType); ok {
				offsets, _, _ := g.objectLayout(objType)
				offset := offsets[mem.Property]
				obj := g.lowerExpr(mem.Object)
				if e.Op == token.Eq {
					rhs := g.lowerExpr(e.Right)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: obj, Field: mem.Property, Offset: offset, Val: rhs})
					return rhs
				}
				fieldType := types.TypeAny
				if t := g.semanticType(mem); t != nil {
					fieldType = t
				}
				current := g.currentFn.NewValue("field_old", fieldType)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: current, Obj: obj, Field: mem.Property, Offset: offset})
				rhs := g.lowerExpr(e.Right)
				value := g.lowerAssignmentValue(e, current, rhs)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: obj, Field: mem.Property, Offset: offset, Val: value})
				return value
			}
			obj := g.lowerExpr(mem.Object)
			if concrete, ok := g.provenObjectType(mem.Object); ok {
				offsets, _, _ := g.objectLayout(concrete)
				if offset, exists := offsets[mem.Property]; exists {
					if e.Op != token.Eq {
						return g.failExpr("compound assignment through any alias is not implemented yet")
					}
					rhs := g.lowerExpr(e.Right)
					raw := g.unboxKnownObject(obj, concrete)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: raw, Field: mem.Property, Offset: offset, Val: rhs})
					return rhs
				}
				return g.failExpr("cannot add property %q to a proven closed shape through any", mem.Property)
			}
			if e.Op != token.Eq {
				return g.failExpr("dynamic compound property assignment is not implemented yet")
			}
			return g.lowerDynamicSet(obj, mem.Property, e.Right)
		}

		if idx, ok := e.Left.(*ast.IndexExpr); ok {
			if obj, ok := g.semanticType(idx.Target).(*types.ObjectType); ok && obj.Name == "$Uint8Array" {
				target := g.lowerExpr(idx.Target)
				index := g.lowerExpr(idx.Index)
				value := g.lowerExpr(e.Right)
				data := g.uint8ArrayField(target, "$data", g.semaResult.ByteBufferType)
				offset := g.uint8ArrayField(target, "byteOffset", types.TypeNumber)
				actual := g.currentFn.NewValue("uint8_index", types.TypeNumber)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.BinaryInst{Res: actual, Op: ir.OpAdd, LHS: offset, RHS: index})
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_byte_buffer_set", Args: []ir.Operand{data, actual, value}, ParamTypes: []types.Type{g.semaResult.ByteBufferType, types.TypeNumber, types.TypeNumber}})
				return value
			}
			if key, ok := g.staticStringKey(idx.Index); ok {
				target := g.lowerExpr(idx.Target)
				if object, ok := target.Type().(*types.ObjectType); ok {
					offsets, _, _ := g.objectLayout(object)
					offset := offsets[key]
					fieldType := object.Fields[key].Type
					if e.Op == token.Eq {
						rhs := g.lowerExpr(e.Right)
						g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: target, Field: key, Offset: offset, Val: rhs})
						return rhs
					}
					current := g.currentFn.NewValue("computed_old", fieldType)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: current, Obj: target, Field: key, Offset: offset})
					rhs := g.lowerExpr(e.Right)
					value := g.lowerAssignmentValue(e, current, rhs)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: target, Field: key, Offset: offset, Val: value})
					return value
				}
				if concrete, ok := g.provenObjectType(idx.Target); ok {
					offsets, _, _ := g.objectLayout(concrete)
					if offset, exists := offsets[key]; exists {
						if e.Op != token.Eq {
							return g.failExpr("compound computed assignment through any alias is not implemented yet")
						}
						rhs := g.lowerExpr(e.Right)
						raw := g.unboxKnownObject(target, concrete)
						g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: raw, Field: key, Offset: offset, Val: rhs})
						return rhs
					}
					return g.failExpr("cannot add computed property %q to a proven closed shape through any", key)
				}
				if e.Op != token.Eq {
					return g.failExpr("dynamic computed compound assignment is not implemented yet")
				}
				return g.lowerDynamicSet(target, key, e.Right)
			}
			if tuple, isTuple := g.semanticType(idx.Target).(*types.TupleType); isTuple {
				lit := idx.Index.(*ast.NumberLit)
				i := int(lit.Value)
				tupleVal := g.lowerExpr(idx.Target)
				field := strconv.Itoa(i)
				offset := 16 + i*8
				if e.Op == token.Eq {
					rhs := g.lowerExpr(e.Right)
					g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: tupleVal, Field: field, Offset: offset, Val: rhs})
					return rhs
				}
				current := g.currentFn.NewValue("tuple_old", tuple.Elements[i])
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{Res: current, Obj: tupleVal, Field: field, Offset: offset})
				rhs := g.lowerExpr(e.Right)
				value := g.lowerAssignmentValue(e, current, rhs)
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{Obj: tupleVal, Field: field, Offset: offset, Val: value})
				return value
			}
			array := g.lowerExpr(idx.Target)
			index := g.lowerExpr(idx.Index)
			if e.Op == token.Eq {
				rhs := g.lowerExpr(e.Right)
				stored := rhs
				if arrType, ok := array.Type().(*types.ArrayType); ok && irJSValueType(arrType.Elem) {
					stored = g.boxJSValue(rhs, g.semanticType(e.Right))
				}
				g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetElementInst{Array: array, Index: index, Val: stored})
				return rhs
			}
			currentType := types.TypeAny
			if t := g.semanticType(idx); t != nil {
				currentType = t
			}
			current := g.currentFn.NewValue("elem_old", currentType)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetElementInst{Res: current, Array: array, Index: index})
			rhs := g.lowerExpr(e.Right)
			value := g.lowerAssignmentValue(e, current, rhs)
			g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetElementInst{Array: array, Index: index, Val: value})
			return value
		}
		ident := e.Left.(*ast.IdentExpr)
		current := g.readLocal(ident.Name)
		rhs := g.lowerExpr(e.Right)
		if e.Op == token.Eq {
			targetType := g.semanticType(e.Left)
			sourceType := g.semanticType(e.Right)
			rhs = g.coerceJSValueBoundary(rhs, sourceType, targetType)
			if !g.writeCapturedLocal(ident.Name, rhs) {
				g.locals[ident.Name] = rhs
			}
			delete(g.localProvenance, ident.Name)
			delete(g.localDirectCallee, ident.Name)
			if irJSValueType(targetType) {
				switch concrete := sourceType.(type) {
				case *types.ObjectType:
					g.localProvenance[ident.Name] = concrete
				case *types.FunctionType:
					g.localProvenance[ident.Name] = concrete
					if target, ok := g.directCalleeForExpr(e.Right); ok {
						g.localDirectCallee[ident.Name] = target
					}
				}
			}
			return rhs
		}
		value := g.lowerAssignmentValue(e, current, rhs)
		if !g.writeCapturedLocal(ident.Name, value) {
			g.locals[ident.Name] = value
		}
		return value

	default:
		te := expr.(*ast.TernaryExpr)
		cond := g.lowerExpr(te.Cond)
		thenBB := g.currentFn.NewBlock("tern_then")
		elseBB := g.currentFn.NewBlock("tern_else")
		joinBB := g.currentFn.NewBlock("tern_join")

		g.currentBB.Terminator = &ir.BranchTerm{Cond: cond, Then: thenBB, Else: elseBB}

		g.currentBB = thenBB
		thenVal := g.lowerExpr(te.Then)
		thenEndBB := g.currentBB
		if thenEndBB.Terminator == nil {
			thenEndBB.Terminator = &ir.JumpTerm{Target: joinBB}
		}

		g.currentBB = elseBB
		elseVal := g.lowerExpr(te.Else)
		elseEndBB := g.currentBB
		if elseEndBB.Terminator == nil {
			elseEndBB.Terminator = &ir.JumpTerm{Target: joinBB}
		}

		g.currentBB = joinBB
		resVal := g.currentFn.NewValue("tern", thenVal.Type())
		joinBB.Phis = append(joinBB.Phis, &ir.PhiInst{
			Res: resVal,
			Incoming: []ir.PhiIncoming{
				{Block: thenEndBB, Value: thenVal},
				{Block: elseEndBB, Value: elseVal},
			},
		})
		return resVal
	}
}
