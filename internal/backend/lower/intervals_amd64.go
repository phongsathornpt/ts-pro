package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

func emitAMD64IntervalRuntimeSymbols(e *amd64.Emitter, fnOffsets map[string]int) {
	fnOffsets["ts_interval_task_id"] = len(e.Code)
	emitAMD64IntervalTaskID(e)
	fnOffsets["ts_clear_interval"] = len(e.Code)
	emitAMD64ClearInterval(e)
}

func emitAMD64IntervalTaskID(e *amd64.Emitter) {
	// RDI = active task pointer. Timer handles use exact integer-valued f64 ids,
	// matching the existing setTimeout ABI while preserving the task as the
	// scheduler-owned GC root.
	e.Cvtsi2sd(amd64.XMM0, amd64.RDI)
	e.Ret()
}

func emitAMD64ClearInterval(e *amd64.Emitter) {
	patchJcc := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6))))
	}
	patchJmp := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5))))
	}

	// XMM0 = numeric interval handle. Never dereference it until it has been
	// found in one of the runtime-owned active task locations; stale ids are a
	// required no-op rather than a use-after-free.
	e.Cvttsd2si(amd64.R10, amd64.XMM0)
	e.TestRegReg(amd64.R10, amd64.R10)
	emptyID := len(e.Code)
	e.JccRel32(amd64.CondE, 0)

	// A callback may clear its own interval while the worker is current.
	e.MovRegDeref(amd64.R11, amd64.R15, amd64RTCurrentTask)
	e.CmpRegReg(amd64.R11, amd64.R10)
	foundCurrent := len(e.Code)
	e.JccRel32(amd64.CondE, 0)

	// Before its first run the interval worker lives on the runnable task queue.
	e.MovRegDeref(amd64.R11, amd64.R15, amd64RTTaskHead)
	runnableLoop := len(e.Code)
	e.TestRegReg(amd64.R11, amd64.R11)
	runnableDone := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.CmpRegReg(amd64.R11, amd64.R10)
	foundRunnable := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.R11, amd64.R11, amd64TaskNext)
	runnableBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(runnableBack, runnableLoop)

	// While waiting between ticks the worker lives on the timer queue.
	timerScan := len(e.Code)
	patchJcc(runnableDone, timerScan)
	e.MovRegDeref(amd64.R11, amd64.R15, amd64RTTimerHead)
	timerLoop := len(e.Code)
	e.TestRegReg(amd64.R11, amd64.R11)
	timerDone := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.CmpRegReg(amd64.R11, amd64.R10)
	foundTimer := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.R11, amd64.R11, amd64TaskNext)
	timerBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(timerBack, timerLoop)

	// Stale/already-finished handles are harmless.
	done := len(e.Code)
	patchJcc(emptyID, done)
	patchJcc(timerDone, done)
	e.Ret()

	// Current or runnable tasks can simply observe cancellation at the next
	// worker cancellation check.
	foundActiveLabel := len(e.Code)
	patchJcc(foundCurrent, foundActiveLabel)
	patchJcc(foundRunnable, foundActiveLabel)
	e.MovRegImm64(amd64.RAX, 1)
	e.MovDerefReg(amd64.R11, amd64TaskCancelled, amd64.RAX)
	activeDone := len(e.Code)
	e.JmpRel32(0)

	// Sleeping interval workers are cancelled and made immediately due. This
	// lets the normal scheduler resume them so the private task stack is cleaned
	// up by the ordinary trampoline rather than leaked.
	foundTimerLabel := len(e.Code)
	patchJcc(foundTimer, foundTimerLabel)
	e.MovRegImm64(amd64.RAX, 1)
	e.MovDerefReg(amd64.R11, amd64TaskCancelled, amd64.RAX)
	e.MovRegImm64(amd64.RAX, 0)
	e.MovDerefReg(amd64.R11, amd64TaskWakeNS, amd64.RAX)

	finish := len(e.Code)
	patchJmp(activeDone, finish)
	e.Ret()
}
