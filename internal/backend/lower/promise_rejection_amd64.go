package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

const (
	amd64TaskRejectionHandledBit  int64 = 1 << 8
	amd64TaskUnhandledReportedBit int64 = 1 << 9
)

func emitAMD64PromiseRejectionRuntimeSymbols(e *amd64.Emitter, fnOffsets map[string]int) {
	fnOffsets["ts_task_mark_rejection_handled"] = len(e.Code)
	emitAMD64TaskMarkRejectionHandled(e)
	fnOffsets["ts_task_rejection_handled"] = len(e.Code)
	emitAMD64TaskRejectionHandled(e)
	fnOffsets["ts_task_mark_unhandled_reported"] = len(e.Code)
	emitAMD64TaskMarkUnhandledReported(e)
	fnOffsets["ts_task_unhandled_reported"] = len(e.Code)
	emitAMD64TaskUnhandledReported(e)
	fnOffsets["ts_task_peek_error"] = len(e.Code)
	emitAMD64TaskPeekError(e)
	// Override the generic task error accessor with Promise-aware consumption
	// semantics. All existing await/aggregate rejection paths flow through this
	// symbol, so reading a rejection reason marks the source handled exactly once.
	fnOffsets["ts_task_error"] = len(e.Code)
	emitAMD64TaskConsumeError(e)
}

func emitAMD64TaskMarkRejectionHandled(e *amd64.Emitter) {
	// RDI = settled task. Rejection metadata lives in high bits of task.kind.
	// Low bits remain the native result-kind ABI used while the task executes.
	e.MovRegDeref(amd64.R10, amd64.RDI, amd64TaskState)
	e.CmpRegImm32(amd64.R10, 2)
	notSettled := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.MovRegDeref(amd64.R10, amd64.RDI, amd64TaskKind)
	e.MovRegImm64(amd64.R11, amd64TaskRejectionHandledBit)
	e.OrRegReg(amd64.R10, amd64.R11)
	e.MovDerefReg(amd64.RDI, amd64TaskKind, amd64.R10)
	done := len(e.Code)
	patchAMD64RejectionJcc(e, notSettled, done)
	e.Ret()
}

func emitAMD64TaskRejectionHandled(e *amd64.Emitter) {
	// RDI = task. Return canonical bool in RAX.
	e.MovRegDeref(amd64.R10, amd64.RDI, amd64TaskKind)
	e.MovRegImm64(amd64.R11, amd64TaskRejectionHandledBit)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.CmpRegImm32(amd64.R10, 0)
	e.Setcc(amd64.CondNE, amd64.RAX)
	e.Ret()
}

func emitAMD64TaskMarkUnhandledReported(e *amd64.Emitter) {
	// Only rejected tasks may transition to the host-reported state.
	e.MovRegDeref(amd64.R10, amd64.RDI, amd64TaskState)
	e.CmpRegImm32(amd64.R10, 3)
	notRejected := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegDeref(amd64.R10, amd64.RDI, amd64TaskKind)
	e.MovRegImm64(amd64.R11, amd64TaskUnhandledReportedBit)
	e.OrRegReg(amd64.R10, amd64.R11)
	e.MovDerefReg(amd64.RDI, amd64TaskKind, amd64.R10)
	done := len(e.Code)
	patchAMD64RejectionJcc(e, notRejected, done)
	e.Ret()
}

func emitAMD64TaskUnhandledReported(e *amd64.Emitter) {
	// RDI = task. Return canonical bool in RAX.
	e.MovRegDeref(amd64.R10, amd64.RDI, amd64TaskKind)
	e.MovRegImm64(amd64.R11, amd64TaskUnhandledReportedBit)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.CmpRegImm32(amd64.R10, 0)
	e.Setcc(amd64.CondNE, amd64.RAX)
	e.Ret()
}

func emitAMD64TaskPeekError(e *amd64.Emitter) {
	// RDI = task. Return the raw NaN-boxed rejection reason without changing
	// host rejection bookkeeping. Used only by the host monitor.
	e.MovRegDeref(amd64.RAX, amd64.RDI, amd64TaskResult)
	e.Ret()
}

func emitAMD64TaskConsumeError(e *amd64.Emitter) {
	// RDI = rejected task. Preserve the rejection value in RAX while marking
	// the settled task handled in task.kind's metadata bits.
	e.MovRegDeref(amd64.RAX, amd64.RDI, amd64TaskResult)
	e.MovRegDeref(amd64.R10, amd64.RDI, amd64TaskState)
	e.CmpRegImm32(amd64.R10, 3)
	notRejected := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegDeref(amd64.R10, amd64.RDI, amd64TaskKind)
	e.MovRegImm64(amd64.R11, amd64TaskRejectionHandledBit)
	e.OrRegReg(amd64.R10, amd64.R11)
	e.MovDerefReg(amd64.RDI, amd64TaskKind, amd64.R10)
	done := len(e.Code)
	patchAMD64RejectionJcc(e, notRejected, done)
	e.Ret()
}

func patchAMD64RejectionJcc(e *amd64.Emitter, at, target int) {
	binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6))))
}
