package irgen

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

const performanceEventTargetKey = "__tspro_internal_performance_event_target__"

func (g *generator) lowerPerformanceEventTarget() ir.Operand {
	globalType := types.NewObject("$GlobalScope")
	global := g.currentFn.NewValue("performance_global", globalType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: global, Callee: "ts_global_object"})
	boxedGlobal := g.boxJSValue(global, globalType)

	existing := g.lowerDynamicGet(boxedGlobal, performanceEventTargetKey)
	boxedUndefined := g.boxJSValue(ir.ConstUndefined{}, types.TypeUndefined)
	isUndefined := g.currentFn.NewValue("performance_target_missing", types.TypeBoolean)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res:        isUndefined,
		Callee:     "ts_js_strict_eq",
		Args:       []ir.Operand{existing, boxedUndefined},
		ParamTypes: []types.Type{types.TypeAny, types.TypeAny},
	})

	entry := g.currentBB
	createBB := g.currentFn.NewBlock("performance_target_create")
	doneBB := g.currentFn.NewBlock("performance_target_done")
	entry.Terminator = &ir.BranchTerm{Cond: isUndefined, Then: createBB, Else: doneBB}

	g.currentBB = createBB
	target := g.currentFn.NewValue("performance_target", g.semaResult.EventTargetType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: target, Callee: "ts_event_target_new"})
	created := g.boxJSValue(target, g.semaResult.EventTargetType)
	set := g.currentFn.NewValue("performance_target_set", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res:        set,
		Callee:     "ts_dynamic_set",
		Args:       []ir.Operand{boxedGlobal, ir.ConstString{Value: performanceEventTargetKey}, created},
		ParamTypes: []types.Type{types.TypeAny, types.TypeString, types.TypeAny},
	})
	createBB.Terminator = &ir.JumpTerm{Target: doneBB}

	g.currentBB = doneBB
	boxed := g.currentFn.NewValue("performance_target_boxed", types.TypeAny)
	doneBB.Phis = append(doneBB.Phis, &ir.PhiInst{
		Res:      boxed,
		Incoming: []ir.PhiIncoming{
			{Block: entry, Value: existing},
			{Block: createBB, Value: created},
		},
	})
	return g.coerceJSValueBoundary(boxed, types.TypeAny, g.semaResult.EventTargetType)
}
