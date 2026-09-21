package e2e_test

import (
	"runtime"
	"testing"

	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func TestLinuxAMD64InternalPromiseRejectionLifecycleState(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("requires linux/amd64 execution")
	}

	prog := &ir.Program{}
	closureType := types.NewFunction(nil, types.TypeVoid)
	worker := ir.NewFunction("$rejection_state_worker", types.TypeVoid)
	workerEntry := worker.NewBlock("entry")
	env := worker.NewValue("$env", closureType)
	worker.Params = append(worker.Params, env)
	workerEntry.Instructions = append(workerEntry.Instructions,
		&ir.CallInst{
			Callee:     "ts_task_reject",
			Args:       []ir.Operand{ir.ConstUndefined{}},
			ParamTypes: []types.Type{types.TypeAny},
		},
	)
	workerEntry.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, worker)

	main := ir.NewFunction("@main", types.TypeVoid)
	entry := main.NewBlock("entry")
	closure := main.NewValue("worker_closure", closureType)
	taskType := types.NewObject("$RejectionStateTask")
	task := main.NewValue("task", taskType)
	entry.Instructions = append(entry.Instructions,
		&ir.MakeClosureInst{Res: closure, Function: worker.Name},
		&ir.CallInst{
			Res:    task,
			Callee: "ts_task_spawn",
			Args:   []ir.Operand{closure, ir.ConstNumber{Value: 0}},
		},
		&ir.CallInst{Callee: "ts_task_join", Args: []ir.Operand{task}},
	)

	printBoolCall := func(name, callee string) {
		value := main.NewValue(name, types.TypeBoolean)
		entry.Instructions = append(entry.Instructions,
			&ir.CallInst{
				Res:        value,
				Callee:     callee,
				Args:       []ir.Operand{task},
				ParamTypes: []types.Type{taskType},
			},
			&ir.CallInst{
				Callee:     "ts_print_val",
				Args:       []ir.Operand{value},
				ParamTypes: []types.Type{types.TypeBoolean},
			},
		)
	}

	printBoolCall("rejected", "ts_task_rejected")
	printBoolCall("handled_initial", "ts_task_rejection_handled")
	printBoolCall("reported_initial", "ts_task_unhandled_reported")

	peeked := main.NewValue("peeked_error", types.TypeAny)
	entry.Instructions = append(entry.Instructions, &ir.CallInst{
		Res:        peeked,
		Callee:     "ts_task_peek_error",
		Args:       []ir.Operand{task},
		ParamTypes: []types.Type{taskType},
	})
	printBoolCall("handled_after_peek", "ts_task_rejection_handled")

	entry.Instructions = append(entry.Instructions, &ir.CallInst{
		Callee:     "ts_task_mark_unhandled_reported",
		Args:       []ir.Operand{task},
		ParamTypes: []types.Type{taskType},
	})
	printBoolCall("reported_after_mark", "ts_task_unhandled_reported")

	consumed := main.NewValue("consumed_error", types.TypeAny)
	entry.Instructions = append(entry.Instructions, &ir.CallInst{
		Res:        consumed,
		Callee:     "ts_task_error",
		Args:       []ir.Operand{task},
		ParamTypes: []types.Type{taskType},
	})
	printBoolCall("handled_after_consume", "ts_task_rejection_handled")
	printBoolCall("reported_stays_marked", "ts_task_unhandled_reported")

	entry.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, main)

	runInternalAMD64IR(t, prog, "promise-rejection-lifecycle-state",
		"true\nfalse\nfalse\nfalse\ntrue\ntrue\ntrue\n")
}
