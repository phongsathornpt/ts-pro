package irgen

import (
	"fmt"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func (g *generator) makeIntervalWorker(callback, delay ir.Operand) ir.Operand {
	outerFn, outerBB, outerLocals, outerProv, outerDirect := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	callbackType, ok := callback.Type().(*types.FunctionType)
	if !ok {
		return g.failExpr("setInterval callback must lower to a function")
	}
	workerType := types.NewFunction(nil, types.TypeVoid)
	name := fmt.Sprintf("$interval_worker%d", g.arrowCounter)
	g.arrowCounter++
	lifted := ir.NewFunction(name, types.TypeVoid)
	g.currentFn = lifted
	entryBB := lifted.NewBlock("entry")
	g.currentBB = entryBB
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)

	env := lifted.NewValue("$env", workerType)
	lifted.Params = append(lifted.Params, env)
	capturedCallback := lifted.NewValue("callback", callbackType)
	capturedDelay := lifted.NewValue("delay", types.TypeNumber)
	entryBB.Instructions = append(entryBB.Instructions,
		&ir.ClosureGetInst{Res: capturedCallback, Closure: env, Index: 0},
		&ir.ClosureGetInst{Res: capturedDelay, Closure: env, Index: 1},
	)

	checkBB := lifted.NewBlock("interval_check")
	sleepBB := lifted.NewBlock("interval_sleep")
	fireBB := lifted.NewBlock("interval_fire")
	doneBB := lifted.NewBlock("interval_done")
	entryBB.Terminator = &ir.JumpTerm{Target: checkBB}

	cancelledBeforeSleep := lifted.NewValue("interval_cancelled_before_sleep", types.TypeBoolean)
	checkBB.Instructions = append(checkBB.Instructions, &ir.CallInst{Res: cancelledBeforeSleep, Callee: "ts_task_cancelled"})
	checkBB.Terminator = &ir.BranchTerm{Cond: cancelledBeforeSleep, Then: doneBB, Else: sleepBB}

	sleepBB.Instructions = append(sleepBB.Instructions, &ir.CallInst{
		Callee:     "ts_task_sleep",
		Args:       []ir.Operand{capturedDelay},
		ParamTypes: []types.Type{types.TypeNumber},
	})
	cancelledAfterSleep := lifted.NewValue("interval_cancelled_after_sleep", types.TypeBoolean)
	sleepBB.Instructions = append(sleepBB.Instructions, &ir.CallInst{Res: cancelledAfterSleep, Callee: "ts_task_cancelled"})
	sleepBB.Terminator = &ir.BranchTerm{Cond: cancelledAfterSleep, Then: doneBB, Else: fireBB}

	fireBB.Instructions = append(fireBB.Instructions, &ir.IndirectCallInst{Closure: capturedCallback})
	fireBB.Terminator = &ir.JumpTerm{Target: checkBB}
	doneBB.Terminator = &ir.ReturnTerm{}

	g.prog.Functions = append(g.prog.Functions, lifted)
	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProv, outerDirect

	worker := g.currentFn.NewValue("interval_worker", workerType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{
		Res:      worker,
		Function: name,
		Captures: []ir.Operand{callback, delay},
		RefMask:  1,
	})
	return worker
}

func (g *generator) lowerSetInterval(e *ast.CallExpr) ir.Operand {
	callback := g.lowerExpr(e.Args[0])
	delay := ir.Operand(ir.ConstNumber{Value: 0})
	if len(e.Args) == 2 {
		delay = g.lowerExpr(e.Args[1])
	}
	worker := g.makeIntervalWorker(callback, delay)
	if worker == nil {
		return nil
	}
	intervalTaskType := types.NewObject("$IntervalTask")
	task := g.currentFn.NewValue("interval_task", intervalTaskType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res:    task,
		Callee: "ts_task_spawn",
		Args:   []ir.Operand{worker, ir.ConstNumber{Value: nativeTaskResultKind(types.TypeVoid)}},
	})
	id := g.currentFn.NewValue("interval_id", types.TypeNumber)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{
		Res:    id,
		Callee: "ts_interval_task_id",
		Args:   []ir.Operand{task},
	})
	return id
}
