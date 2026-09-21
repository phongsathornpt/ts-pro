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
	workerEntry.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, worker)

	main := ir.NewFunction("@main", types.TypeVoid)
	entry := main.NewBlock("entry")
	closure := main.NewValue("worker_closure", closureType)
	taskType := types.NewObject("$RejectionStateTask")
	task := main.NewValue("task", taskType)
	entry.Instructions = append(entry.Instructions,
		&ir.MakeClosureInst{Res: closure, Function: worker.Name},
		&ir.CallInst{Res: task, Callee: "ts_task_spawn", Args: []ir.Operand{closure, ir.ConstNumber{Value: 0}}},
	)

	printBoolCall := func(name, callee string) {
		value := main.NewValue(name, types.TypeBoolean)
		entry.Instructions = append(entry.Instructions,
			&ir.CallInst{Res: value, Callee: callee, Args: []ir.Operand{task}, ParamTypes: []types.Type{taskType}},
			&ir.CallInst{Callee: "ts_print_val", Args: []ir.Operand{value}, ParamTypes: []types.Type{types.TypeBoolean}},
		)
	}
	printBoolCall("handled_initial", "ts_task_rejection_handled")
	printBoolCall("reported_initial", "ts_task_rejection_reported")
	printBoolCall("handled_first", "ts_task_mark_rejection_handled")
	printBoolCall("handled_after", "ts_task_rejection_handled")
	printBoolCall("handled_second", "ts_task_mark_rejection_handled")
	printBoolCall("reported_first", "ts_task_mark_rejection_reported")
	printBoolCall("reported_after", "ts_task_rejection_reported")
	printBoolCall("reported_second", "ts_task_mark_rejection_reported")

	entry.Terminator = &ir.ReturnTerm{}
	prog.Functions = append(prog.Functions, main)

	runInternalAMD64IR(t, prog, "promise-rejection-lifecycle-state",
		"false\nfalse\ntrue\ntrue\nfalse\ntrue\ntrue\nfalse\n")
}
