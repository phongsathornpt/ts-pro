package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

func emitAMD64SetTimeout(e *amd64.Emitter, allocOffset, nowOffset int) {
	patchJcc := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6))))
	}
	patchJmp := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5))))
	}
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.SubRegImm32(amd64.RSP, 40)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.Cvttsd2si(amd64.R13, amd64.XMM0)
	e.TestRegReg(amd64.R13, amd64.R13)
	nonNegative := len(e.Code)
	e.JccRel32(amd64.CondGE, 0)
	e.MovRegImm64(amd64.R13, 0)
	patchJcc(nonNegative, len(e.Code))

	// Keep the callback closure alive if task allocation collects.
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
	for _, off := range []int32{
		amd64TaskState, amd64TaskResult, amd64TaskNext,
		amd64TaskSavedRsp, amd64TaskSavedRbp, amd64TaskSavedRbx,
		amd64TaskSavedR12, amd64TaskSavedR13, amd64TaskSavedR14,
		amd64TaskSavedRoot, amd64TaskReturnRsp, amd64TaskReturnRbp,
		amd64TaskReturnRbx, amd64TaskReturnR12, amd64TaskReturnR13,
		amd64TaskReturnR14, amd64TaskReturnRoot, amd64TaskParent,
		amd64TaskCancelled, amd64TaskContext, amd64TaskGroupNext,
		amd64TaskWakeNS, amd64TaskTimer,
	} {
		e.MovDerefReg(amd64.RBX, off, amd64.R10)
	}

	// Timer callbacks ignore return values and are tagged so cancellation can
	// suppress first execution without changing ordinary cooperative task semantics.
	e.MovRegImm64(amd64.R10, 0)
	e.MovDerefReg(amd64.RBX, amd64TaskKind, amd64.R10)
	e.MovRegImm64(amd64.R10, 1)
	e.MovDerefReg(amd64.RBX, amd64TaskTimer, amd64.R10)

	// Inherit task-local context from a running parent, matching spawn().
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTCurrentTask)
	e.TestRegReg(amd64.R10, amd64.R10)
	noParentContext := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.R11, amd64.R10, amd64TaskContext)
	e.MovDerefReg(amd64.RBX, amd64TaskContext, amd64.R11)
	patchJcc(noParentContext, len(e.Code))

	// Allocate the private task stack.
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

	// deadline = monotonic_now + max(delay, 0) * 1ms.
	callNow := len(e.Code)
	e.CallRel32(int32(nowOffset - (callNow + 5)))
	e.MovRegReg(amd64.R12, amd64.R13)
	e.MovRegImm64(amd64.R10, 1000000)
	e.ImulRegReg(amd64.R12, amd64.R10)
	e.AddRegReg(amd64.R12, amd64.RAX)
	e.MovDerefReg(amd64.RBX, amd64TaskWakeNS, amd64.R12)

	// Sorted insert into the runtime timer queue.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTTimerHead)
	e.TestRegReg(amd64.R10, amd64.R10)
	emptyTimers := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.R11, amd64.R10, amd64TaskWakeNS)
	e.CmpRegReg(amd64.R12, amd64.R11)
	beforeHead := len(e.Code)
	e.JccRel32(amd64.CondL, 0)
	e.MovRegDeref(amd64.R11, amd64.R10, amd64TaskNext)
	insertLoop := len(e.Code)
	e.TestRegReg(amd64.R11, amd64.R11)
	insertAfter := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.RAX, amd64.R11, amd64TaskWakeNS)
	e.CmpRegReg(amd64.R12, amd64.RAX)
	insertBeforeCurrent := len(e.Code)
	e.JccRel32(amd64.CondL, 0)
	e.MovRegReg(amd64.R10, amd64.R11)
	e.MovRegDeref(amd64.R11, amd64.R11, amd64TaskNext)
	back := len(e.Code)
	e.JmpRel32(0)
	patchJmp(back, insertLoop)
	insertAfterLabel := len(e.Code)
	patchJcc(insertAfter, insertAfterLabel)
	patchJcc(insertBeforeCurrent, insertAfterLabel)
	e.MovDerefReg(amd64.RBX, amd64TaskNext, amd64.R11)
	e.MovDerefReg(amd64.R10, amd64TaskNext, amd64.RBX)
	insertedJump := len(e.Code)
	e.JmpRel32(0)

	beforeHeadLabel := len(e.Code)
	patchJcc(beforeHead, beforeHeadLabel)
	e.MovDerefReg(amd64.RBX, amd64TaskNext, amd64.R10)
	e.MovDerefReg(amd64.R15, amd64RTTimerHead, amd64.RBX)
	beforeHeadJump := len(e.Code)
	e.JmpRel32(0)

	emptyTimersLabel := len(e.Code)
	patchJcc(emptyTimers, emptyTimersLabel)
	e.MovRegImm64(amd64.R11, 0)
	e.MovDerefReg(amd64.RBX, amd64TaskNext, amd64.R11)
	e.MovDerefReg(amd64.R15, amd64RTTimerHead, amd64.RBX)

	inserted := len(e.Code)
	patchJmp(insertedJump, inserted)
	patchJmp(beforeHeadJump, inserted)
	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.Cvtsi2sd(amd64.XMM0, amd64.RBX)
	e.AddRegImm32(amd64.RSP, 40)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64ClearTimeout(e *amd64.Emitter) {
	patchJcc := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6))))
	}
	patchJmp := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5))))
	}
	e.Cvttsd2si(amd64.R10, amd64.XMM0)
	e.TestRegReg(amd64.R10, amd64.R10)
	done := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.R11, amd64.R15, amd64RTTimerHead)
	loop := len(e.Code)
	e.TestRegReg(amd64.R11, amd64.R11)
	end := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.CmpRegReg(amd64.R11, amd64.R10)
	found := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.R11, amd64.R11, amd64TaskNext)
	back := len(e.Code)
	e.JmpRel32(0)
	patchJmp(back, loop)
	foundLabel := len(e.Code)
	patchJcc(found, foundLabel)
	e.MovRegImm64(amd64.RAX, 1)
	e.MovDerefReg(amd64.R11, amd64TaskCancelled, amd64.RAX)
	finishJump := len(e.Code)
	e.JmpRel32(0)
	endLabel := len(e.Code)
	patchJcc(done, endLabel)
	patchJcc(end, endLabel)
	finish := len(e.Code)
	patchJmp(finishJump, finish)
	e.Ret()
}
