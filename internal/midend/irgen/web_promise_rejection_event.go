package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) lowerPromiseRejectionEventNew(e *ast.NewExpr) ir.Operand {
	resultType := g.semanticType(e).(*types.ObjectType)
	typeValue := g.lowerExpr(e.Args[0])
	initExpr := e.Args[1]
	initValue := g.lowerExpr(initExpr)

	offsets, refMask, shape := g.objectLayout(resultType)
	obj := g.currentFn.NewValue("promise_rejection_event", resultType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.AllocObjectInst{
		Res: obj, Shape: shape, FieldCount: len(offsets), RefMask: refMask,
	})

	g.setEventField(obj, "type", typeValue)
	g.setEventField(obj, "bubbles", g.lowerEventInitBool(initExpr, initValue, "bubbles"))
	g.setEventField(obj, "cancelable", g.lowerEventInitBool(initExpr, initValue, "cancelable"))
	g.setEventField(obj, "composed", g.lowerEventInitBool(initExpr, initValue, "composed"))
	g.setEventField(obj, "currentTarget", ir.ConstNull{})
	g.setEventField(obj, "target", ir.ConstNull{})
	g.setEventField(obj, "defaultPrevented", ir.ConstBool{Value: false})
	g.setEventField(obj, "eventPhase", ir.ConstNumber{Value: 0})
	g.setEventField(obj, "isTrusted", ir.ConstBool{Value: false})
	timestamp := g.currentFn.NewValue("promise_rejection_event_timestamp", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: timestamp, Callee: "ts_performance_now"})
	g.setEventField(obj, "timeStamp", timestamp)
	g.setEventField(obj, "$dispatching", ir.ConstBool{Value: false})
	g.setEventField(obj, "$inPassiveListener", ir.ConstBool{Value: false})
	g.setEventField(obj, "$stopImmediate", ir.ConstBool{Value: false})
	g.setEventField(obj, "$stopPropagation", ir.ConstBool{Value: false})
	g.initEventVariantFields(obj, "", nil, nil)

	promise, _ := g.lowerWebIDLDictionaryMember(initExpr, initValue, "promise", types.TypeAny, ir.ConstUndefined{})
	reason, _ := g.lowerWebIDLDictionaryMember(initExpr, initValue, "reason", types.TypeAny, ir.ConstUndefined{})
	g.currentBB.Instructions = append(g.currentBB.Instructions,
		&ir.SetFieldInst{Obj: obj, Field: "promise", Offset: offsets["promise"], Val: promise},
		&ir.SetFieldInst{Obj: obj, Field: "reason", Offset: offsets["reason"], Val: reason},
	)
	return obj
}
