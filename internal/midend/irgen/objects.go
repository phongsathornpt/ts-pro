package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) provenLocalType(expr ast.Expr) (types.Type, bool) {
	ident, ok := expr.(*ast.IdentExpr)
	if !ok || g.localProvenance == nil {
		return nil, false
	}
	t, ok := g.localProvenance[ident.Name]
	return t, ok && t != nil
}

func dynamicBackedObjectType(t *types.ObjectType) bool {
	if t == nil {
		return false
	}
	switch t.Name {
	case "$GlobalScope", "$DOMException":
		return true
	default:
		return false
	}
}

func (g *generator) provenObjectType(expr ast.Expr) (*types.ObjectType, bool) {
	t, ok := g.provenLocalType(expr)
	if !ok {
		return nil, false
	}
	objectType, ok := t.(*types.ObjectType)
	return objectType, ok && objectType != nil
}

func (g *generator) provenFunctionType(expr ast.Expr) (*types.FunctionType, bool) {
	t, ok := g.provenLocalType(expr)
	if !ok {
		return nil, false
	}
	fnType, ok := t.(*types.FunctionType)
	return fnType, ok && fnType != nil
}

func (g *generator) directCalleeForExpr(expr ast.Expr) (string, bool) {
	ident, ok := expr.(*ast.IdentExpr)
	if !ok {
		return "", false
	}
	if target := g.localDirectCallee[ident.Name]; target != "" {
		return target, true
	}
	if _, local := g.locals[ident.Name]; local {
		return "", false
	}
	name := ident.Name
	if imported := g.semaResult.ImportAliases[name]; imported != "" {
		return imported, true
	}
	return name, true
}

func (g *generator) unboxKnownObject(value ir.Operand, objectType *types.ObjectType) ir.Operand {
	return g.coerceJSValueBoundary(value, types.TypeAny, objectType)
}

func (g *generator) materializeDynamicObject(value ir.Operand, objectType *types.ObjectType) ir.Operand {
	if info := g.semaResult.Classes[objectType.Name]; info != nil {
		return g.failExpr("dynamic structural conversion to class %q is not implemented", objectType.Name)
	}
	offsets, refMask, shape := g.objectLayout(objectType)
	res := g.currentFn.NewValue("dynamic_struct", objectType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
		Res: res, Shape: shape, FieldCount: len(offsets), RefMask: refMask,
	})
	for _, name := range objectType.FieldOrder {
		field := objectType.Fields[name]
		boxed := g.lowerDynamicGet(value, name)
		converted := g.coerceJSValueBoundary(boxed, types.TypeAny, field.Type)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{
			Obj: res, Field: name, Offset: offsets[name], Val: converted,
		})
	}
	return res
}

func (g *generator) newWebError(message, name ir.Operand) ir.Operand {
	dyn := g.currentFn.NewValue("web_error_dynamic", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: dyn, Callee: "ts_dynamic_object_new"})
	for key, value := range map[string]ir.Operand{"message": message, "name": name} {
		boxed := value
		if !irJSValueType(value.Type()) {
			boxed = g.boxJSValue(value, value.Type())
		}
		set := g.currentFn.NewValue("web_error_set", types.TypeAny)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: set, Callee: "ts_dynamic_set", Args: []ir.Operand{dyn, ir.ConstString{Value: key}, boxed}, ParamTypes: []types.Type{types.TypeAny, types.TypeString, types.TypeAny}})
	}
	return dyn
}

func (g *generator) newDOMException(message, name ir.Operand) ir.Operand {
	t := g.semaResult.DOMExceptionType
	dyn := g.currentFn.NewValue("dom_exception_dynamic", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: dyn, Callee: "ts_dynamic_object_new"})
	for key, value := range map[string]ir.Operand{
		"code": ir.ConstNumber{Value: 0}, "message": message, "name": name,
	} {
		boxed := value
		if !irJSValueType(value.Type()) {
			boxed = g.boxJSValue(value, value.Type())
		}
		set := g.currentFn.NewValue("dom_exception_set", types.TypeAny)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: set, Callee: "ts_dynamic_set", Args: []ir.Operand{dyn, ir.ConstString{Value: key}, boxed},
			ParamTypes: []types.Type{types.TypeAny, types.TypeString, types.TypeAny},
		})
	}
	return g.coerceJSValueBoundary(dyn, types.TypeAny, t)
}

func (g *generator) lowerDynamicObjectLiteral(lit *ast.ObjectLit) ir.Operand {
	obj := g.currentFn.NewValue("dynamic_object", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: obj, Callee: "ts_dynamic_object_new"})
	for _, prop := range lit.Properties {
		if prop.Spread {
			return g.failExpr("dynamic object spread is not implemented yet")
		}
		value := g.lowerExpr(prop.Value)
		boxed := g.boxJSValue(value, g.semanticType(prop.Value))
		res := g.currentFn.NewValue("dynamic_init", types.TypeAny)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
			Res: res, Callee: "ts_dynamic_set",
			Args:       []ir.Operand{obj, ir.ConstString{Value: prop.Key}, boxed},
			ParamTypes: []types.Type{types.TypeAny, types.TypeString, types.TypeAny},
		})
	}
	return obj
}

func (g *generator) lowerDynamicGet(obj ir.Operand, key string) ir.Operand {
	res := g.currentFn.NewValue("dynamic_get", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: res, Callee: "ts_dynamic_get",
		Args:       []ir.Operand{obj, ir.ConstString{Value: key}},
		ParamTypes: []types.Type{types.TypeAny, types.TypeString},
	})
	return res
}

func (g *generator) lowerDynamicSet(obj ir.Operand, key string, rhs ast.Expr) ir.Operand {
	value := g.lowerExpr(rhs)
	boxed := g.boxJSValue(value, g.semanticType(rhs))
	res := g.currentFn.NewValue("dynamic_set", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res: res, Callee: "ts_dynamic_set",
		Args:       []ir.Operand{obj, ir.ConstString{Value: key}, boxed},
		ParamTypes: []types.Type{types.TypeAny, types.TypeString, types.TypeAny},
	})
	return res
}
