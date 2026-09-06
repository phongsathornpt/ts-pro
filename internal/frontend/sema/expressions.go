package sema

import (
	"fmt"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/token"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (c *Checker) checkExprWithExpected(expr ast.Expr, expected types.Type) types.Type {
	if tuple, ok := expected.(*types.TupleType); ok {
		if lit, ok := expr.(*ast.ArrayLit); ok {
			actual := make([]types.Type, len(lit.Elements))
			for i, elem := range lit.Elements {
				actual[i] = c.checkExpr(elem)
			}
			actualTuple := types.NewTuple(actual...)
			if len(actual) == len(tuple.Elements) && actualTuple.AssignableTo(tuple) {
				c.result.Types[lit] = tuple
				return tuple
			}
			c.result.Types[lit] = actualTuple
			return actualTuple
		}
	}
	if array, ok := expected.(*types.ArrayType); ok {
		if lit, ok := expr.(*ast.ArrayLit); ok {
			compatible := true
			for _, elem := range lit.Elements {
				if !c.checkExpr(elem).AssignableTo(array.Elem) {
					compatible = false
				}
			}
			if compatible {
				c.result.Types[lit] = array
				return array
			}
		}
	}
	if object, ok := expected.(*types.ObjectType); ok {
		if lit, ok := expr.(*ast.ObjectLit); ok {
			actual := c.checkExpr(lit)
			if actual.AssignableTo(object) {
				c.result.Types[lit] = object
				return object
			}
			return actual
		}
	}
	return c.checkExpr(expr)
}

func (c *Checker) checkExpr(expr ast.Expr) types.Type {

	switch e := expr.(type) {
	case *ast.NumberLit:
		c.result.Types[e] = types.TypeNumber
		return types.TypeNumber
	case *ast.StringLit:
		c.result.Types[e] = types.TypeString
		return types.TypeString
	case *ast.RegexLit:
		t := c.builtinRegExpType()
		c.result.Types[e] = t
		return t
	case *ast.BoolLit:
		c.result.Types[e] = types.TypeBoolean
		return types.TypeBoolean
	case *ast.NullLit:
		c.result.Types[e] = types.TypeNull
		return types.TypeNull
	case *ast.UndefinedLit:
		c.result.Types[e] = types.TypeUndefined
		return types.TypeUndefined
	case *ast.ThisExpr:
		if c.currentThisType != nil {
			c.result.Types[e] = c.currentThisType
			return c.currentThisType
		}
		if c.currentClass == nil {
			c.error(e.Span(), "TS2335", "'this' can only be referenced in a class body or a function with a this parameter.")
			c.result.Types[e] = types.TypeAny
			return types.TypeAny
		}
		c.result.Types[e] = c.currentClass.Instance
		return c.currentClass.Instance
	case *ast.SuperExpr:
		if c.currentClass == nil || c.currentClass.BaseName == "" {
			c.error(e.Span(), "TS2335", "'super' can only be referenced in a derived class.")
			c.result.Types[e] = types.TypeAny
			return types.TypeAny
		}
		base := c.result.Classes[c.currentClass.BaseName]
		c.result.Types[e] = base.Constructor
		return base.Constructor
	case *ast.NewExpr:
		return c.checkNewExpr(e)
	case *ast.IdentExpr:
		sym := c.currentScope.Resolve(e.Name)
		if sym == nil {
			if e.Name == "globalThis" || e.Name == "self" {
				obj := types.NewObject("$GlobalScope")
				c.result.Types[e] = obj
				return obj
			}
			if e.Name == "AbortSignal" {
				ctor := types.NewObject("$AbortSignalConstructor")
				c.result.Types[e] = ctor
				return ctor
			}
			if e.Name == "URL" {
				ctor := types.NewObject("$URLConstructor")
				c.result.Types[e] = ctor
				return ctor
			}
			if e.Name == "URLPattern" {
				ctor := types.NewObject("$URLPatternConstructor")
				c.result.Types[e] = ctor
				return ctor
			}
			if e.Name == "Blob" {
				ctor := types.NewObject("$BlobConstructor")
				c.result.Types[e] = ctor
				return ctor
			}
			if e.Name == "File" {
				ctor := types.NewObject("$FileConstructor")
				c.result.Types[e] = ctor
				return ctor
			}
			if e.Name == "FormData" {
				ctor := types.NewObject("$FormDataConstructor")
				c.result.Types[e] = ctor
				return ctor
			}
			if e.Name == "Headers" {
				ctor := types.NewObject("$HeadersConstructor")
				c.result.Types[e] = ctor
				return ctor
			}
			if e.Name == "ReadableStream" {
				ctor := types.NewObject("$ReadableStreamConstructor")
				c.result.Types[e] = ctor
				return ctor
			}
			if e.Name == "Date" {
				ctor := types.NewObject("$DateConstructor")
				c.result.Types[e] = ctor
				return ctor
			}
			if e.Name == "JSON" {
				jsonType := types.NewObject("$JSON")
				c.result.Types[e] = jsonType
				return jsonType
			}
			if e.Name == "console" {
				obj := types.NewObject("console")
				obj.AddField("log", types.NewFunction([]types.Param{{Name: "value", Type: types.TypeAny}}, types.TypeVoid), false)
				c.result.Types[e] = obj
				return obj
			}
			if e.Name == "performance" {
				obj := types.NewObject("$Performance")
				json := types.NewObject("$PerformanceJSON")
				json.AddField("timeOrigin", types.TypeNumber, false)
				obj.AddField("now", types.NewFunction(nil, types.TypeNumber), false)
				obj.AddField("timeOrigin", types.TypeNumber, false)
				obj.AddField("toJSON", types.NewFunction(nil, json), false)
				c.result.Types[e] = obj
				return obj
			}
			if e.Name == "navigator" {
				obj := types.NewObject("$Navigator")
				obj.AddField("userAgent", types.TypeString, false)
				c.result.Types[e] = obj
				return obj
			}
			c.error(e.Span(), "TS2304", fmt.Sprintf("Cannot find name '%s'.", e.Name))
			c.result.Types[e] = types.TypeAny
			return types.TypeAny
		}
		c.result.Symbols[e] = sym
		c.result.Types[e] = sym.Type
		return sym.Type
	case *ast.BinaryExpr:
		lType := c.checkExpr(e.Left)
		rType := c.checkExpr(e.Right)

		switch e.Op {
		case token.Plus:
			if lType == types.TypeAny || rType == types.TypeAny || lType == types.TypeUnknown || rType == types.TypeUnknown {
				c.result.Types[e] = types.TypeAny
				return types.TypeAny
			}
			if lType == types.TypeString || rType == types.TypeString {
				c.result.Types[e] = types.TypeString
				return types.TypeString
			}
			c.result.Types[e] = types.TypeNumber
			return types.TypeNumber
		case token.Minus, token.Star, token.Slash, token.Percent:
			c.result.Types[e] = types.TypeNumber
			return types.TypeNumber
		case token.EqEq, token.EqEqEq, token.BangEq, token.BangEqEq,
			token.Lt, token.LtEq, token.Gt, token.GtEq:
			c.result.Types[e] = types.TypeBoolean
			return types.TypeBoolean
		case token.AmpAmp, token.PipePipe:
			c.result.Types[e] = rType
			return rType
		case token.QuestionQuestion:
			left := removeNullishType(lType)
			if left == types.TypeNever {
				c.result.Types[e] = rType
				return rType
			}
			result := types.NewUnion(left, rType)
			c.result.Types[e] = result
			return result
		default:
			c.result.Types[e] = lType
			return lType
		}
	case *ast.AwaitExpr:
		targetType := c.checkExpr(e.Target)
		obj, ok := targetType.(*types.ObjectType)
		if !ok {
			c.error(e.Target.Span(), "TS1320", "await expects a task or Promise-like value.")
			c.result.Types[e] = types.TypeAny
			return types.TypeAny
		}
		resultType, ok := c.result.TaskResults[obj.Name]
		if !ok {
			if res, isThenable := c.thenableResultType(obj); isThenable {
				resultType = res
			} else {
				c.error(e.Target.Span(), "TS1320", "await received an unknown task/Promise handle.")
				c.result.Types[e] = types.TypeAny
				return types.TypeAny
			}
		}
		c.result.Types[e] = resultType
		return resultType
	case *ast.UnaryExpr:
		targetType := c.checkExpr(e.Target)
		if e.Op == token.Bang {
			c.result.Types[e] = types.TypeBoolean
			return types.TypeBoolean
		}
		if e.Op == token.PlusPlus || e.Op == token.MinusMinus {
			c.result.Types[e] = types.TypeNumber
			return types.TypeNumber
		}
		c.result.Types[e] = targetType
		return targetType
	case *ast.ArrowFuncExpr:
		params := make([]types.Param, 0, len(e.Params))
		var thisType types.Type
		for _, param := range e.Params {
			pt := c.resolveTypeNode(param.Type)
			if pt == nil {
				pt = types.TypeAny
			}
			if param.IsThis {
				thisType = pt
				continue
			}
			params = append(params, types.Param{Name: param.Name, Type: pt, Optional: param.Optional, Rest: param.Rest})
		}

		parentScope := c.currentScope
		parentFnRet := c.currentFnRet
		parentThis := c.currentThisType
		c.currentScope = NewScope(parentScope)
		if thisType != nil {
			c.currentThisType = thisType
		}
		defer func() {
			c.currentScope = parentScope
			c.currentFnRet = parentFnRet
			c.currentThisType = parentThis
		}()
		runtimeIndex := 0
		for _, param := range e.Params {
			if param.IsThis {
				continue
			}
			_ = c.currentScope.Define(&Symbol{Name: param.Name, Kind: SymParam, Type: params[runtimeIndex].Type, Node: e})
			runtimeIndex++
		}

		declaredReturn := c.resolveTypeNode(e.ReturnType)
		var returnType types.Type
		if e.IsExprBody {
			bodyExpr := e.Body.(ast.Expr)
			bodyType := c.checkExpr(bodyExpr)
			returnType = bodyType
			if declaredReturn != nil {
				if !bodyType.AssignableTo(declaredReturn) {
					c.error(bodyExpr.Span(), "TS2322", fmt.Sprintf("Type '%s' is not assignable to return type '%s'.", bodyType, declaredReturn))
				}
				returnType = declaredReturn
			}
		} else {
			if declaredReturn == nil {
				declaredReturn = types.TypeVoid
			}
			c.currentFnRet = declaredReturn
			if block, ok := e.Body.(*ast.BlockStmt); ok {
				for _, stmt := range block.Statements {
					c.checkStatement(stmt)
				}
			}
			returnType = declaredReturn
		}
		fnType := types.NewFunction(params, returnType)
		fnType.This = thisType
		c.result.Types[e] = fnType
		return fnType
	case *ast.FunctionExpr:
		params := make([]types.Param, 0, len(e.Params))
		var thisType types.Type
		for _, param := range e.Params {
			pt := c.resolveTypeNode(param.Type)
			if pt == nil {
				pt = types.TypeAny
			}
			if param.IsThis {
				thisType = pt
				continue
			}
			params = append(params, types.Param{Name: param.Name, Type: pt, Optional: param.Optional, Rest: param.Rest})
		}
		declaredReturn := c.resolveTypeNode(e.ReturnType)
		if declaredReturn == nil {
			declaredReturn = types.TypeVoid
		}
		parentScope, parentFnRet, parentThis := c.currentScope, c.currentFnRet, c.currentThisType
		c.currentScope = NewScope(parentScope)
		c.currentFnRet = declaredReturn
		c.currentThisType = thisType
		runtimeIndex := 0
		for _, param := range e.Params {
			if param.IsThis {
				continue
			}
			_ = c.currentScope.Define(&Symbol{Name: param.Name, Kind: SymParam, Type: params[runtimeIndex].Type, Node: e})
			runtimeIndex++
		}
		for _, stmt := range e.Body.Statements {
			c.checkStatement(stmt)
		}
		c.currentScope, c.currentFnRet, c.currentThisType = parentScope, parentFnRet, parentThis
		fnType := types.NewFunction(params, declaredReturn)
		fnType.This = thisType
		c.result.Types[e] = fnType
		return fnType
	case *ast.CallExpr:
		return c.checkCallExpr(e)
	case *ast.AssignExpr:
		targetType := c.checkExpr(e.Left)
		valType := c.checkExpr(e.Right)
		if !valType.AssignableTo(targetType) {
			c.error(e.Right.Span(), "TS2322", fmt.Sprintf("Type '%s' is not assignable to type '%s'.", valType, targetType))
		}
		c.result.Types[e] = targetType
		return targetType
	case *ast.TernaryExpr:
		c.checkExpr(e.Cond)
		t1 := c.checkExpr(e.Then)
		t2 := c.checkExpr(e.Else)
		resType := types.NewUnion(t1, t2)
		c.result.Types[e] = resType
		return resType
	case *ast.SpreadExpr:
		t := c.checkExpr(e.Value)
		c.result.Types[e] = t
		return t
	case *ast.ArrayLit:
		var elemType types.Type = types.TypeNever
		for _, el := range e.Elements {
			t := c.checkExpr(el)
			if spread, ok := el.(*ast.SpreadExpr); ok {
				arr, ok := t.(*types.ArrayType)
				if !ok {
					c.error(spread.Span(), "TS2488", fmt.Sprintf("Type '%s' is not spreadable by native array spread lowering.", t))
					t = types.TypeAny
				} else {
					t = arr.Elem
				}
			}
			if elemType == types.TypeNever {
				elemType = t
			} else {
				elemType = types.NewUnion(elemType, t)
			}
		}
		if elemType == types.TypeNever {
			elemType = types.TypeAny
		}
		arrType := types.NewArray(elemType)
		c.result.Types[e] = arrType
		return arrType
	case *ast.ObjectLit:
		obj := types.NewObject("")
		for _, prop := range e.Properties {
			pType := c.checkExpr(prop.Value)
			if prop.Spread {
				source, ok := pType.(*types.ObjectType)
				if !ok {
					c.error(prop.SourceSpan, "TS2698", fmt.Sprintf("Spread types may only be created from closed object types, got '%s'.", pType))
					continue
				}
				for _, name := range source.FieldOrder {
					field := source.Fields[name]
					obj.AddField(name, field.Type, field.Optional)
				}
				continue
			}
			obj.AddField(prop.Key, pType, false)
		}
		c.result.Types[e] = obj
		return obj
	case *ast.IndexExpr:
		targetType := c.checkExpr(e.Target)
		indexType := c.checkExpr(e.Index)
		if object, ok := targetType.(*types.ObjectType); ok {
			if key, ok := e.Index.(*ast.StringLit); ok {
				if field, exists := object.Fields[key.Value]; exists {
					fieldType := field.Type
					if field.Optional {
						fieldType = types.NewUnion(fieldType, types.TypeUndefined)
					}
					c.result.Types[e] = fieldType
					return fieldType
				}
				c.error(e.Span(), "TS7053", fmt.Sprintf("Property '%s' does not exist on type '%s'.", key.Value, targetType))
				c.result.Types[e] = types.TypeAny
				return types.TypeAny
			}
		}
		if object, ok := targetType.(*types.ObjectType); ok && object.Name == "$Uint8Array" {
			if indexType != types.TypeNumber && indexType != types.TypeAny {
				c.error(e.Index.Span(), "TS7015", "Uint8Array index expression must be a number.")
			}
			c.result.Types[e] = types.TypeNumber
			return types.TypeNumber
		}
		if targetType != types.TypeAny && indexType != types.TypeNumber && indexType != types.TypeAny {
			c.error(e.Index.Span(), "TS7015", "Array index expression must be a number.")
		}
		if tuple, ok := targetType.(*types.TupleType); ok {
			if lit, ok := e.Index.(*ast.NumberLit); ok {
				idx := int(lit.Value)
				if float64(idx) == lit.Value && idx >= 0 && idx < len(tuple.Elements) {
					c.result.Types[e] = tuple.Elements[idx]
					return tuple.Elements[idx]
				}
			}
			if len(tuple.Elements) > 0 {
				u := types.NewUnion(tuple.Elements...)
				c.result.Types[e] = u
				return u
			}
			c.result.Types[e] = types.TypeNever
			return types.TypeNever
		}
		if arr, ok := targetType.(*types.ArrayType); ok {
			c.result.Types[e] = arr.Elem
			return arr.Elem
		}
		if targetType != types.TypeAny {
			c.error(e.Span(), "TS7053", fmt.Sprintf("Element implicitly has an 'any' type because type '%s' has no numeric index signature.", targetType))
		}
		c.result.Types[e] = types.TypeAny
		return types.TypeAny
	case *ast.MemberExpr:
		return c.checkMemberExpr(e)
	default:
		panic(fmt.Sprintf("unsupported expression %T", expr))
	}
}
