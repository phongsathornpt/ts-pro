package sema

import (
	"fmt"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (c *Checker) newPromiseType(inner types.Type) *types.ObjectType {
	name := fmt.Sprintf("$Promise$%d", c.taskTypeCount)
	c.taskTypeCount++
	obj := types.NewObject(name)
	c.result.TaskResults[name] = inner
	return obj
}

func functionMemberType(t types.Type) *types.FunctionType {
	if fn, ok := t.(*types.FunctionType); ok {
		return fn
	}
	if u, ok := t.(*types.UnionType); ok {
		for _, m := range u.Members {
			if fn, ok := m.(*types.FunctionType); ok {
				return fn
			}
		}
	}
	return nil
}

func (c *Checker) thenableResultType(t types.Type) (types.Type, bool) {
	obj, ok := t.(*types.ObjectType)
	if !ok {
		return nil, false
	}
	var thenFn *types.FunctionType
	if info := c.result.Classes[obj.Name]; info != nil {
		thenFn = info.Methods["then"]
	}
	if thenFn == nil {
		if field, exists := obj.Fields["then"]; exists {
			thenFn = functionMemberType(field.Type)
		}
	}
	if thenFn != nil && len(thenFn.Params) > 0 {
		if resolveFn := functionMemberType(thenFn.Params[0].Type); resolveFn != nil && len(resolveFn.Params) > 0 {
			return resolveFn.Params[0].Type, true
		}
	}
	return nil, false
}

func (c *Checker) promiseResultType(t types.Type) (types.Type, bool) {
	obj, ok := t.(*types.ObjectType)
	if !ok {
		return nil, false
	}
	inner, ok := c.result.TaskResults[obj.Name]
	return inner, ok
}

func (c *Checker) promiseSettledType(t types.Type) types.Type {
	if adopted, ok := c.promiseResultType(t); ok {
		return adopted
	}
	if assimilated, ok := c.thenableResultType(t); ok {
		return assimilated
	}
	if union, ok := t.(*types.UnionType); ok {
		members := make([]types.Type, 0, len(union.Members))
		for _, member := range union.Members {
			members = append(members, c.promiseSettledType(member))
		}
		return types.NewUnion(members...)
	}
	return t
}

func (c *Checker) checkPromiseAggregateCall(e *ast.CallExpr, member *ast.MemberExpr) types.Type {
	if len(e.Args) != 1 {
		c.error(e.Span(), "TS2554", fmt.Sprintf("Promise.%s expects exactly one array argument.", member.Property))
		c.result.Types[e] = types.TypeAny
		return types.TypeAny
	}
	argType := c.checkExpr(e.Args[0])
	arr, ok := argType.(*types.ArrayType)
	if !ok {
		c.error(e.Args[0].Span(), "TS2345", fmt.Sprintf("Promise.%s expects an array input.", member.Property))
		c.result.Types[e] = types.TypeAny
		return types.TypeAny
	}
	var explicit types.Type
	if len(e.TypeArgs) > 0 {
		if len(e.TypeArgs) != 1 {
			c.error(e.Span(), "TS2558", "Promise aggregate builtin expects one type argument.")
		}
		explicit = c.resolveTypeNode(e.TypeArgs[0])
	}

	var inner types.Type
	switch member.Property {
	case "all":
		if literal, ok := e.Args[0].(*ast.ArrayLit); ok && explicit == nil {
			elems := make([]types.Type, 0, len(literal.Elements))
			for _, element := range literal.Elements {
				elems = append(elems, c.promiseSettledType(c.result.Types[element]))
			}
			inner = types.NewTuple(elems...)
		} else {
			elem := explicit
			if elem == nil {
				elem = c.promiseSettledType(arr.Elem)
			}
			inner = types.NewArray(elem)
		}
	case "race":
		inner = explicit
		if inner == nil {
			if literal, ok := e.Args[0].(*ast.ArrayLit); ok {
				members := make([]types.Type, 0, len(literal.Elements))
				for _, element := range literal.Elements {
					members = append(members, c.promiseSettledType(c.result.Types[element]))
				}
				inner = types.NewUnion(members...)
			} else {
				inner = c.promiseSettledType(arr.Elem)
			}
		}
	}
	pt := c.newPromiseType(inner)
	c.result.Types[member.Object] = types.TypeAny
	c.result.Types[member] = types.TypeAny
	c.result.Types[e.Callee] = types.TypeAny
	c.result.Types[e] = pt
	return pt
}

func (c *Checker) checkPromiseStaticCall(e *ast.CallExpr, member *ast.MemberExpr) (types.Type, bool) {
	ident, ok := member.Object.(*ast.IdentExpr)
	if !ok || ident.Name != "Promise" {
		return nil, false
	}
	if member.Property == "all" || member.Property == "race" {
		return c.checkPromiseAggregateCall(e, member), true
	}
	if member.Property != "resolve" && member.Property != "reject" {
		return nil, false
	}
	if len(e.Args) != 1 {
		c.error(e.Span(), "TS2554", fmt.Sprintf("Promise.%s expects exactly one argument.", member.Property))
		c.result.Types[e] = types.TypeAny
		return types.TypeAny, true
	}
	argType := c.checkExpr(e.Args[0])
	var inner types.Type
	if len(e.TypeArgs) > 0 {
		if len(e.TypeArgs) != 1 {
			c.error(e.Span(), "TS2558", "Promise builtin expects one type argument.")
		}
		inner = c.resolveTypeNode(e.TypeArgs[0])
	} else if member.Property == "resolve" {
		if adopted, ok := c.promiseResultType(argType); ok {
			inner = adopted
		} else if assimilated, ok := c.thenableResultType(argType); ok {
			inner = assimilated
		} else {
			inner = argType
		}
	} else {
		inner = types.TypeAny
	}
	pt := c.newPromiseType(inner)
	c.result.Types[member.Object] = types.TypeAny
	c.result.Types[member] = types.TypeAny
	c.result.Types[e.Callee] = types.TypeAny
	c.result.Types[e] = pt
	return pt, true
}
