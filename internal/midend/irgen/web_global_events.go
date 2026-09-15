package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

const globalEventTargetKey = "__tspro_internal_global_event_target__"

func (g *generator) lowerGlobalEventTarget() ir.Operand {
	globalType := types.NewObject("$GlobalScope")
	global := g.currentFn.NewValue("global_event_scope", globalType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: global, Callee: "ts_global_object"})
	boxedGlobal := g.boxJSValue(global, globalType)

	existing := g.lowerDynamicGet(boxedGlobal, globalEventTargetKey)
	boxedUndefined := g.boxJSValue(ir.ConstUndefined{}, types.TypeUndefined)
	missing := g.currentFn.NewValue("global_event_target_missing", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res:        missing,
		Callee:     "ts_js_strict_eq",
		Args:       []ir.Operand{existing, boxedUndefined},
		ParamTypes: []types.Type{types.TypeAny, types.TypeAny},
	})

	createBB := g.currentFn.NewBlock("global_event_target_create")
	doneBB := g.currentFn.NewBlock("global_event_target_done")
	g.currentBB.Terminator = &ir.BranchTerm{Cond: missing, Then: createBB, Else: doneBB}

	g.currentBB = createBB
	target := g.currentFn.NewValue("global_event_target", g.semaResult.EventTargetType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: target, Callee: "ts_event_target_new"})
	created := g.boxJSValue(target, g.semaResult.EventTargetType)
	set := g.currentFn.NewValue("global_event_target_set", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res:        set,
		Callee:     "ts_dynamic_set",
		Args:       []ir.Operand{boxedGlobal, ir.ConstString{Value: globalEventTargetKey}, created},
		ParamTypes: []types.Type{types.TypeAny, types.TypeString, types.TypeAny},
	})
	createBB.Terminator = &ir.JumpTerm{Target: doneBB}

	g.currentBB = doneBB
	boxed := g.lowerDynamicGet(boxedGlobal, globalEventTargetKey)
	return g.coerceJSValueBoundary(boxed, types.TypeAny, g.semaResult.EventTargetType)
}
