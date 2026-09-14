package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) lowerStructuredClone(e *ast.CallExpr) ir.Operand {
	sourceType := g.semanticType(e.Args[0])
	source := g.lowerExpr(e.Args[0])
	return g.cloneStructuredValue(source, sourceType)
}

func (g *generator) cloneStructuredValue(source ir.Operand, sourceType types.Type) ir.Operand {
	if sourceType == nil {
		return g.failExpr("structuredClone requires a known source type")
	}

	switch sourceType.Kind() {
	case types.KindNumber, types.KindBoolean, types.KindString, types.KindNull, types.KindUndefined:
		return source
	}

	switch t := sourceType.(type) {
	case *types.ArrayType:
		return g.cloneStructuredArray(source, t)
	case *types.ObjectType:
		if t.Name != "" || g.semaResult.Classes[t.Name] != nil {
			return g.failExpr("structuredClone support for platform/class object %q is not implemented", t.Name)
		}
		return g.cloneStructuredObject(source, t)
	default:
		return g.failExpr("structuredClone support for type %q is not implemented", sourceType.String())
	}
}

func (g *generator) cloneStructuredObject(source ir.Operand, objectType *types.ObjectType) ir.Operand {
	offsets, refMask, shape := g.objectLayout(objectType)
	clone := g.currentFn.NewValue("structured_clone_object", objectType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
		Res: clone, Shape: shape, FieldCount: len(offsets), RefMask: refMask,
	})
	for _, name := range objectType.FieldOrder {
		field := objectType.Fields[name]
		value := g.currentFn.NewValue("structured_clone_field", field.Type)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetFieldInst{
			Res: value, Obj: source, Field: name, Offset: offsets[name],
		})
		copied := g.cloneStructuredValue(value, field.Type)
		if copied == nil {
			return nil
		}
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.SetFieldInst{
			Obj: clone, Field: name, Offset: offsets[name], Val: copied,
		})
	}
	return clone
}

func (g *generator) cloneStructuredArray(source ir.Operand, arrayType *types.ArrayType) ir.Operand {
	length := g.currentFn.NewValue("structured_clone_length", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayLengthInst{Res: length, Array: source})
	clone := g.currentFn.NewValue("structured_clone_array", arrayType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocArrayInst{
		Res: clone, ElemType: arrayType.Elem, Length: ir.ConstNumber{Value: 0},
	})

	pre := g.currentBB
	cond := g.currentFn.NewBlock("structured_clone_array_cond")
	body := g.currentFn.NewBlock("structured_clone_array_body")
	post := g.currentFn.NewBlock("structured_clone_array_post")
	done := g.currentFn.NewBlock("structured_clone_array_done")
	pre.Terminator = &ir.JumpTerm{Target: cond}

	index := g.currentFn.NewValue("structured_clone_array_i", types.TypeNumber)
	next := g.currentFn.NewValue("structured_clone_array_next", types.TypeNumber)
	cond.Phis = append(cond.Phis, &ir.PhiInst{Res: index, Incoming: []ir.PhiIncoming{
		{Block: pre, Value: ir.ConstNumber{Value: 0}},
		{Block: post, Value: next},
	}})
	more := g.currentFn.NewValue("structured_clone_array_more", types.TypeBoolean)
	cond.Instructions = append(cond.Instructions, &ir.BinaryInst{Res: more, Op: ir.OpLt, LHS: index, RHS: length})
	cond.Terminator = &ir.BranchTerm{Cond: more, Then: body, Else: done}

	g.currentBB = body
	item := g.currentFn.NewValue("structured_clone_array_item", arrayType.Elem)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.GetElementInst{Res: item, Array: source, Index: index})
	copied := g.cloneStructuredValue(item, arrayType.Elem)
	if copied == nil {
		return nil
	}
	push := g.currentFn.NewValue("structured_clone_array_push", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ArrayPushInst{Res: push, Array: clone, Val: copied})
	g.currentBB.Terminator = &ir.JumpTerm{Target: post}

	post.Instructions = append(post.Instructions, &ir.BinaryInst{Res: next, Op: ir.OpAdd, LHS: index, RHS: ir.ConstNumber{Value: 1}})
	post.Terminator = &ir.JumpTerm{Target: cond}
	g.currentBB = done
	return clone
}
