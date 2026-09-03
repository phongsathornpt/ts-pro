package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

const (
	amd64TaskClosure    int32 = 0
	amd64TaskState      int32 = 8
	amd64TaskResult     int32 = 16
	amd64TaskKind       int32 = 24
	amd64TaskNext       int32 = 32
	amd64TaskStackTop   int32 = 40
	amd64TaskSavedRsp   int32 = 48
	amd64TaskSavedRbp   int32 = 56
	amd64TaskSavedRbx   int32 = 64
	amd64TaskSavedR12   int32 = 72
	amd64TaskSavedR13   int32 = 80
	amd64TaskSavedR14   int32 = 88
	amd64TaskSavedRoot  int32 = 96
	amd64TaskReturnRsp  int32 = 104
	amd64TaskReturnRbp  int32 = 112
	amd64TaskReturnRbx  int32 = 120
	amd64TaskReturnR12  int32 = 128
	amd64TaskReturnR13  int32 = 136
	amd64TaskReturnR14  int32 = 144
	amd64TaskReturnRoot int32 = 152
	amd64TaskParent     int32 = 160
	amd64TaskPayload    int32 = 168

	amd64TaskResultVoid   int64 = 0
	amd64TaskResultNumber int64 = 1
	amd64TaskResultScalar int64 = 2
	amd64TaskResultRef    int64 = 3
	amd64TaskResultJS     int64 = 4

	amd64TaskStackBytes int64 = 1 << 20
)

func emitAMD64TaskSpawn(e *amd64.Emitter, allocOffset int) {
	// RDI=closure payload, XMM0=result-kind number. Allocate a GC task object
	// plus a private native stack and enqueue it without running user code.
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.SubRegImm32(amd64.RSP, 32)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.Cvttsd2si(amd64.R12, amd64.XMM0)

	// Keep closure alive if task allocation collects.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 1)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	e.MovRegImm64(amd64.RDI, int64(amd64TaskPayload))
	callAlloc := len(e.Code)
	e.CallRel32(int32(allocOffset - (callAlloc + 5)))
	e.MovRegReg(amd64.RBX, amd64.RAX)
	emitAMD64SetObjectType(e, amd64.RBX, amd64ObjectTypeTask)
	e.MovRegDeref(amd64.R10, amd64.RSP, 16)
	e.MovDerefReg(amd64.RBX, amd64TaskClosure, amd64.R10)
	e.MovRegImm64(amd64.R10, 0)
	for _, off := range []int32{amd64TaskState, amd64TaskResult, amd64TaskNext, amd64TaskSavedRsp, amd64TaskSavedRbp, amd64TaskSavedRbx, amd64TaskSavedR12, amd64TaskSavedR13, amd64TaskSavedR14, amd64TaskSavedRoot, amd64TaskReturnRsp, amd64TaskReturnRbp, amd64TaskReturnRbx, amd64TaskReturnR12, amd64TaskReturnR13, amd64TaskReturnR14, amd64TaskReturnRoot, amd64TaskParent} {
		e.MovDerefReg(amd64.RBX, off, amd64.R10)
	}
	e.MovDerefReg(amd64.RBX, amd64TaskKind, amd64.R12)

	// mmap one private RW stack. It is outside the GC heap; heap references on
	// suspended task stacks are discovered through the saved precise-root chain.
	e.MovRegImm64(amd64.RDI, 0)
	e.MovRegImm64(amd64.RSI, amd64TaskStackBytes)
	e.MovRegImm64(amd64.RDX, 3)
	e.MovRegImm64(amd64.R10, 0x22)
	e.MovRegImm64(amd64.R8, -1)
	e.MovRegImm64(amd64.R9, 0)
	e.MovRegImm64(amd64.RAX, 9)
	e.Syscall()
	e.AddRegImm32(amd64.RAX, int32(amd64TaskStackBytes))
	e.MovDerefReg(amd64.RBX, amd64TaskStackTop, amd64.RAX)

	// FIFO enqueue.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTTaskTail)
	e.TestRegReg(amd64.R10, amd64.R10)
	emptyQueue := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovDerefReg(amd64.R10, amd64TaskNext, amd64.RBX)
	linkedJump := len(e.Code)
	e.JmpRel32(0)
	emptyQueueLabel := len(e.Code)
	patchJcc(emptyQueue, emptyQueueLabel)
	e.MovDerefReg(amd64.R15, amd64RTTaskHead, amd64.RBX)
	linked := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[linkedJump+1:], uint32(int32(linked-(linkedJump+5))))
	e.MovDerefReg(amd64.R15, amd64RTTaskTail, amd64.RBX)

	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.MovRegReg(amd64.RAX, amd64.RBX)
	e.AddRegImm32(amd64.RSP, 32)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64TaskTrampoline(e *amd64.Emitter) {
	// Entered by JMP on a fresh task stack. Run the closure once, cache its raw
	// result, mark completion, then restore the scheduler context and RET to the
	// call site after ts_task_resume.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTCurrentTask)
	e.MovRegDeref(amd64.R12, amd64.R10, amd64TaskClosure)
	e.MovRegReg(amd64.RDI, amd64.R12)
	e.MovRegDeref(amd64.R11, amd64.R12, 0)
	e.CallReg(amd64.R11)

	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTCurrentTask)
	e.MovRegDeref(amd64.R11, amd64.R10, amd64TaskKind)
	e.CmpRegImm32(amd64.R11, int32(amd64TaskResultNumber))
	notNumber := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovQRegXMM(amd64.RAX, amd64.XMM0)
	notNumberLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[notNumber+2:], uint32(int32(notNumberLabel-(notNumber+6))))
	e.MovDerefReg(amd64.R10, amd64TaskResult, amd64.RAX)
	e.MovRegImm64(amd64.R11, 2)
	e.MovDerefReg(amd64.R10, amd64TaskState, amd64.R11)

	// Clear suspended-stack metadata on completion, then restore the caller
	// context that resumed this task. Return context is task-local so nested
	// join/run-one calls compose correctly.
	e.MovRegReg(amd64.R11, amd64.R10)
	e.MovRegImm64(amd64.RAX, 0)
	e.MovDerefReg(amd64.R11, amd64TaskSavedRsp, amd64.RAX)
	e.MovDerefReg(amd64.R11, amd64TaskSavedRoot, amd64.RAX)
	e.MovRegDeref(amd64.RAX, amd64.R11, amd64TaskReturnRoot)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RAX)
	e.MovRegDeref(amd64.RAX, amd64.R11, amd64TaskParent)
	e.MovDerefReg(amd64.R15, amd64RTCurrentTask, amd64.RAX)
	e.MovRegDeref(amd64.RBP, amd64.R11, amd64TaskReturnRbp)
	e.MovRegDeref(amd64.RBX, amd64.R11, amd64TaskReturnRbx)
	e.MovRegDeref(amd64.R12, amd64.R11, amd64TaskReturnR12)
	e.MovRegDeref(amd64.R13, amd64.R11, amd64TaskReturnR13)
	e.MovRegDeref(amd64.R14, amd64.R11, amd64TaskReturnR14)
	e.MovRegDeref(amd64.RAX, amd64.R11, amd64TaskReturnRsp)
	e.MovRegImm64(amd64.R10, 0)
	e.MovDerefReg(amd64.R11, amd64TaskReturnRoot, amd64.R10)
	e.MovDerefReg(amd64.R11, amd64TaskParent, amd64.R10)
	e.MovDerefReg(amd64.R11, amd64TaskReturnRsp, amd64.R10)
	e.MovRegReg(amd64.RSP, amd64.RAX)
	e.Ret()
}

func emitAMD64TaskResume(e *amd64.Emitter, trampolineOffset int) {
	// RDI=task. Save scheduler context. A never-started task switches to its
	// fresh stack and jumps to the trampoline; a yielded task restores its saved
	// stack and RETs into the instruction after ts_task_suspend.
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	e.MovRegReg(amd64.R11, amd64.RDI)
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTCurrentTask)
	e.MovDerefReg(amd64.R11, amd64TaskParent, amd64.R10)
	e.MovDerefReg(amd64.R11, amd64TaskReturnRsp, amd64.RSP)
	e.MovDerefReg(amd64.R11, amd64TaskReturnRbp, amd64.RBP)
	e.MovDerefReg(amd64.R11, amd64TaskReturnRbx, amd64.RBX)
	e.MovDerefReg(amd64.R11, amd64TaskReturnR12, amd64.R12)
	e.MovDerefReg(amd64.R11, amd64TaskReturnR13, amd64.R13)
	e.MovDerefReg(amd64.R11, amd64TaskReturnR14, amd64.R14)
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.R11, amd64TaskReturnRoot, amd64.R10)
	e.MovDerefReg(amd64.R15, amd64RTCurrentTask, amd64.R11)
	e.MovRegDeref(amd64.R10, amd64.R11, amd64TaskSavedRsp)
	e.TestRegReg(amd64.R10, amd64.R10)
	resumeSaved := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)

	// First execution.
	e.MovRegImm64(amd64.R10, 1)
	e.MovDerefReg(amd64.R11, amd64TaskState, amd64.R10)
	e.MovRegImm64(amd64.R10, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.MovRegDeref(amd64.RSP, amd64.R11, amd64TaskStackTop)
	jmpAt := len(e.Code)
	e.JmpRel32(int32(trampolineOffset - (jmpAt + 5)))

	resumeLabel := len(e.Code)
	patchJcc(resumeSaved, resumeLabel)
	e.MovRegDeref(amd64.R10, amd64.R11, amd64TaskSavedRoot)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.MovRegDeref(amd64.RBP, amd64.R11, amd64TaskSavedRbp)
	e.MovRegDeref(amd64.RBX, amd64.R11, amd64TaskSavedRbx)
	e.MovRegDeref(amd64.R12, amd64.R11, amd64TaskSavedR12)
	e.MovRegDeref(amd64.R13, amd64.R11, amd64TaskSavedR13)
	e.MovRegDeref(amd64.R14, amd64.R11, amd64TaskSavedR14)
	e.MovRegDeref(amd64.RSP, amd64.R11, amd64TaskSavedRsp)
	e.Ret()
}

func emitAMD64TaskSuspend(e *amd64.Emitter) {
	// Save running-task stack/register/root state and restore the scheduler.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTCurrentTask)
	e.MovDerefReg(amd64.R10, amd64TaskSavedRsp, amd64.RSP)
	e.MovDerefReg(amd64.R10, amd64TaskSavedRbp, amd64.RBP)
	e.MovDerefReg(amd64.R10, amd64TaskSavedRbx, amd64.RBX)
	e.MovDerefReg(amd64.R10, amd64TaskSavedR12, amd64.R12)
	e.MovDerefReg(amd64.R10, amd64TaskSavedR13, amd64.R13)
	e.MovDerefReg(amd64.R10, amd64TaskSavedR14, amd64.R14)
	e.MovRegDeref(amd64.R11, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.R10, amd64TaskSavedRoot, amd64.R11)

	e.MovRegReg(amd64.R11, amd64.R10)
	e.MovRegDeref(amd64.RAX, amd64.R11, amd64TaskReturnRoot)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RAX)
	e.MovRegDeref(amd64.RAX, amd64.R11, amd64TaskParent)
	e.MovDerefReg(amd64.R15, amd64RTCurrentTask, amd64.RAX)
	e.MovRegDeref(amd64.RBP, amd64.R11, amd64TaskReturnRbp)
	e.MovRegDeref(amd64.RBX, amd64.R11, amd64TaskReturnRbx)
	e.MovRegDeref(amd64.R12, amd64.R11, amd64TaskReturnR12)
	e.MovRegDeref(amd64.R13, amd64.R11, amd64TaskReturnR13)
	e.MovRegDeref(amd64.R14, amd64.R11, amd64TaskReturnR14)
	e.MovRegDeref(amd64.RAX, amd64.R11, amd64TaskReturnRsp)
	e.MovRegImm64(amd64.R10, 0)
	e.MovDerefReg(amd64.R11, amd64TaskReturnRoot, amd64.R10)
	e.MovDerefReg(amd64.R11, amd64TaskParent, amd64.R10)
	e.MovDerefReg(amd64.R11, amd64TaskReturnRsp, amd64.R10)
	e.MovRegReg(amd64.RSP, amd64.RAX)
	e.Ret()
}

func emitAMD64TaskRunOne(e *amd64.Emitter, resumeOffset int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.SubRegImm32(amd64.RSP, 8)

	e.MovRegDeref(amd64.RBX, amd64.R15, amd64RTTaskHead)
	e.TestRegReg(amd64.RBX, amd64.RBX)
	empty := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64TaskNext)
	e.MovDerefReg(amd64.R15, amd64RTTaskHead, amd64.R10)
	e.TestRegReg(amd64.R10, amd64.R10)
	hasNext := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegImm64(amd64.R11, 0)
	e.MovDerefReg(amd64.R15, amd64RTTaskTail, amd64.R11)
	patchJcc(hasNext, len(e.Code))
	e.MovRegImm64(amd64.R11, 0)
	e.MovDerefReg(amd64.RBX, amd64TaskNext, amd64.R11)

	e.MovRegReg(amd64.RDI, amd64.RBX)
	callResume := len(e.Code)
	e.CallRel32(int32(resumeOffset - (callResume + 5)))
	e.MovRegImm64(amd64.RAX, 1)
	doneJump := len(e.Code)
	e.JmpRel32(0)

	emptyLabel := len(e.Code)
	patchJcc(empty, emptyLabel)
	e.MovRegImm64(amd64.RAX, 0)
	done := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[doneJump+1:], uint32(int32(done-(doneJump+5))))
	e.AddRegImm32(amd64.RSP, 8)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64TaskJoin(e *amd64.Emitter, runOneOffset int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.SubRegImm32(amd64.RSP, 8)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	loop := len(e.Code)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64TaskState)
	e.CmpRegImm32(amd64.R10, 2)
	doneTask := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	callRun := len(e.Code)
	e.CallRel32(int32(runOneOffset - (callRun + 5)))
	back := len(e.Code)
	e.JmpRel32(0)
	patchJmp(back, loop)
	done := len(e.Code)
	patchJcc(doneTask, done)
	e.MovRegDeref(amd64.RAX, amd64.RBX, amd64TaskResult)
	e.MovQXMMReg(amd64.XMM0, amd64.RAX)
	e.AddRegImm32(amd64.RSP, 8)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64TaskYield(e *amd64.Emitter, runOneOffset, suspendOffset int) {
	// Main/scheduler context runs one task. A running task requeues itself and
	// suspends its native stack, resuming later after this call to suspend.
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTCurrentTask)
	e.TestRegReg(amd64.R10, amd64.R10)
	mainContext := len(e.Code)
	e.JccRel32(amd64.CondE, 0)

	// Requeue current task at tail.
	e.MovRegImm64(amd64.R11, 0)
	e.MovDerefReg(amd64.R10, amd64TaskNext, amd64.R11)
	e.MovRegDeref(amd64.R11, amd64.R15, amd64RTTaskTail)
	e.TestRegReg(amd64.R11, amd64.R11)
	emptyQueue := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovDerefReg(amd64.R11, amd64TaskNext, amd64.R10)
	linkedJump := len(e.Code)
	e.JmpRel32(0)
	emptyLabel := len(e.Code)
	patchJcc(emptyQueue, emptyLabel)
	e.MovDerefReg(amd64.R15, amd64RTTaskHead, amd64.R10)
	linked := len(e.Code)
	patchJmp(linkedJump, linked)
	e.MovDerefReg(amd64.R15, amd64RTTaskTail, amd64.R10)
	callSuspend := len(e.Code)
	e.CallRel32(int32(suspendOffset - (callSuspend + 5)))
	// Resumed task continues here.
	doneJump := len(e.Code)
	e.JmpRel32(0)

	mainLabel := len(e.Code)
	patchJcc(mainContext, mainLabel)
	callRun := len(e.Code)
	e.CallRel32(int32(runOneOffset - (callRun + 5)))
	done := len(e.Code)
	patchJmp(doneJump, done)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64TaskSleep(e *amd64.Emitter) {
	// XMM0 = milliseconds. Blocking nanosleep remains the current timer backend;
	// task suspension is handled independently at explicit scheduler/channel waits.
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.SubRegImm32(amd64.RSP, 16)
	e.Cvttsd2si(amd64.RAX, amd64.XMM0)
	e.TestRegReg(amd64.RAX, amd64.RAX)
	nonPositive := len(e.Code)
	e.JccRel32(amd64.CondLE, 0)
	e.Cqo()
	e.MovRegImm64(amd64.R10, 1000)
	e.IdivReg(amd64.R10)
	e.MovDerefReg(amd64.RSP, 0, amd64.RAX)
	e.MovRegImm64(amd64.R10, 1000000)
	e.ImulRegReg(amd64.RDX, amd64.R10)
	e.MovDerefReg(amd64.RSP, 8, amd64.RDX)
	e.MovRegImm64(amd64.RAX, 35)
	e.MovRegReg(amd64.RDI, amd64.RSP)
	e.MovRegImm64(amd64.RSI, 0)
	e.Syscall()
	done := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[nonPositive+2:], uint32(int32(done-(nonPositive+6))))
	e.AddRegImm32(amd64.RSP, 16)
	e.Pop(amd64.RBP)
	e.Ret()
}
