package lower

import (
	"encoding/binary"
	"math"

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
	amd64TaskCancelled  int32 = 168
	amd64TaskContext    int32 = 176
	amd64TaskGroupNext  int32 = 184
	amd64TaskWakeNS     int32 = 192
	amd64TaskTimer      int32 = 200
	amd64TaskPayload    int32 = 208

	amd64TaskResultNumber int64 = 1
	amd64TaskResultRef    int64 = 3

	amd64TaskStackBytes int64 = 1 << 20
)

func emitAMD64TaskSpawn(e *amd64.Emitter, allocOffset int) {
	emitAMD64TaskSpawnQueued(e, allocOffset, amd64RTTaskHead, amd64RTTaskTail)
}

func emitAMD64MicrotaskSpawn(e *amd64.Emitter, allocOffset int) {
	emitAMD64TaskSpawnQueued(e, allocOffset, amd64RTMicrotaskHead, amd64RTMicrotaskTail)
}

func emitAMD64TaskSpawnQueued(e *amd64.Emitter, allocOffset int, queueHead, queueTail int32) {
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
	for _, off := range []int32{amd64TaskState, amd64TaskResult, amd64TaskNext, amd64TaskSavedRsp, amd64TaskSavedRbp, amd64TaskSavedRbx, amd64TaskSavedR12, amd64TaskSavedR13, amd64TaskSavedR14, amd64TaskSavedRoot, amd64TaskReturnRsp, amd64TaskReturnRbp, amd64TaskReturnRbx, amd64TaskReturnR12, amd64TaskReturnR13, amd64TaskReturnR14, amd64TaskReturnRoot, amd64TaskParent, amd64TaskCancelled, amd64TaskContext, amd64TaskGroupNext, amd64TaskWakeNS, amd64TaskTimer} {
		e.MovDerefReg(amd64.RBX, off, amd64.R10)
	}
	e.MovDerefReg(amd64.RBX, amd64TaskKind, amd64.R12)
	// Inherit task-local string context from the currently running parent.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTCurrentTask)
	e.TestRegReg(amd64.R10, amd64.R10)
	noParentContext := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.R11, amd64.R10, amd64TaskContext)
	e.MovDerefReg(amd64.RBX, amd64TaskContext, amd64.R11)
	noParentContextLabel := len(e.Code)
	patchJcc(noParentContext, noParentContextLabel)

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
	e.MovRegDeref(amd64.R10, amd64.R15, queueTail)
	e.TestRegReg(amd64.R10, amd64.R10)
	emptyQueue := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovDerefReg(amd64.R10, amd64TaskNext, amd64.RBX)
	linkedJump := len(e.Code)
	e.JmpRel32(0)
	emptyQueueLabel := len(e.Code)
	patchJcc(emptyQueue, emptyQueueLabel)
	e.MovDerefReg(amd64.R15, queueHead, amd64.RBX)
	linked := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[linkedJump+1:], uint32(int32(linked-(linkedJump+5))))
	e.MovDerefReg(amd64.R15, queueTail, amd64.RBX)

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
	e.MovRegDeref(amd64.R11, amd64.R10, amd64TaskTimer)
	e.TestRegReg(amd64.R11, amd64.R11)
	runClosureNonTimer := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.R11, amd64.R10, amd64TaskCancelled)
	e.TestRegReg(amd64.R11, amd64.R11)
	runClosureTimer := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegImm64(amd64.RAX, 0)
	skipClosure := len(e.Code)
	e.JmpRel32(0)

	runClosureLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[runClosureNonTimer+2:], uint32(int32(runClosureLabel-(runClosureNonTimer+6))))
	binary.LittleEndian.PutUint32(e.Code[runClosureTimer+2:], uint32(int32(runClosureLabel-(runClosureTimer+6))))
	e.MovRegDeref(amd64.R12, amd64.R10, amd64TaskClosure)
	e.MovRegReg(amd64.RDI, amd64.R12)
	e.MovRegDeref(amd64.R11, amd64.R12, 0)
	e.CallReg(amd64.R11)

	afterClosure := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[skipClosure+1:], uint32(int32(afterClosure-(skipClosure+5))))
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
	// Capture the private stack mapping before clearing task metadata. The task
	// stack cannot be unmapped until RSP has been restored to the caller stack.
	e.MovRegDeref(amd64.RDX, amd64.R11, amd64TaskStackTop)
	e.SubRegImm32(amd64.RDX, int32(amd64TaskStackBytes))
	e.MovRegImm64(amd64.R10, 0)
	e.MovDerefReg(amd64.R11, amd64TaskReturnRoot, amd64.R10)
	e.MovDerefReg(amd64.R11, amd64TaskParent, amd64.R10)
	e.MovDerefReg(amd64.R11, amd64TaskReturnRsp, amd64.R10)
	e.MovDerefReg(amd64.R11, amd64TaskStackTop, amd64.R10)
	e.MovRegReg(amd64.RSP, amd64.RAX)
	// munmap(privateStackBase, 1 MiB) after switching away from it.
	e.MovRegReg(amd64.RDI, amd64.RDX)
	e.MovRegImm64(amd64.RSI, amd64TaskStackBytes)
	e.MovRegImm64(amd64.RAX, 11) // Linux munmap
	e.Syscall()
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

func emitAMD64ClockSampleToContext(e *amd64.Emitter, clockID int64, contextOffset int32) {
	e.SubRegImm32(amd64.RSP, 16)
	e.MovRegImm64(amd64.RAX, 228)
	e.MovRegImm64(amd64.RDI, clockID)
	e.MovRegReg(amd64.RSI, amd64.RSP)
	e.Syscall()
	e.MovRegDeref(amd64.RAX, amd64.RSP, 0)
	e.MovRegImm64(amd64.R10, 1000000000)
	e.ImulRegReg(amd64.RAX, amd64.R10)
	e.MovRegDeref(amd64.R10, amd64.RSP, 8)
	e.AddRegReg(amd64.RAX, amd64.R10)
	e.AddRegImm32(amd64.RSP, 16)
	e.MovDerefReg(amd64.R15, contextOffset, amd64.RAX)
}

func emitAMD64PerformanceNow(e *amd64.Emitter, clockOffset int) {
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	callAt := len(e.Code)
	e.CallRel32(int32(clockOffset - (callAt + 5)))
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTTimeOriginMono)
	e.SubRegReg(amd64.RAX, amd64.R10)
	e.Cvtsi2sd(amd64.XMM0, amd64.RAX)
	e.MovRegImm64(amd64.R10, int64(math.Float64bits(1_000_000)))
	e.MovQXMMReg(amd64.XMM1, amd64.R10)
	e.DivSD(amd64.XMM0, amd64.XMM1)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64PerformanceTimeOrigin(e *amd64.Emitter) {
	e.MovRegDeref(amd64.RAX, amd64.R15, amd64RTTimeOriginEpoch)
	e.Cvtsi2sd(amd64.XMM0, amd64.RAX)
	e.MovRegImm64(amd64.R10, int64(math.Float64bits(1_000_000)))
	e.MovQXMMReg(amd64.XMM1, amd64.R10)
	e.DivSD(amd64.XMM0, amd64.XMM1)
	e.Ret()
}

func emitAMD64ClockNowNS(e *amd64.Emitter) {
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.SubRegImm32(amd64.RSP, 16)
	e.MovRegImm64(amd64.RAX, 228) // clock_gettime
	e.MovRegImm64(amd64.RDI, 1)   // CLOCK_MONOTONIC
	e.MovRegReg(amd64.RSI, amd64.RSP)
	e.Syscall()
	e.MovRegDeref(amd64.RAX, amd64.RSP, 0)
	e.MovRegImm64(amd64.R10, 1000000000)
	e.ImulRegReg(amd64.RAX, amd64.R10)
	e.MovRegDeref(amd64.R10, amd64.RSP, 8)
	e.AddRegReg(amd64.RAX, amd64.R10)
	e.AddRegImm32(amd64.RSP, 16)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64NanosleepNS(e *amd64.Emitter) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.SubRegImm32(amd64.RSP, 16)
	e.TestRegReg(amd64.RDI, amd64.RDI)
	doneJump := len(e.Code)
	e.JccRel32(amd64.CondLE, 0)
	e.MovRegReg(amd64.RAX, amd64.RDI)
	e.Cqo()
	e.MovRegImm64(amd64.R10, 1000000000)
	e.IdivReg(amd64.R10)
	e.MovDerefReg(amd64.RSP, 0, amd64.RAX)
	e.MovDerefReg(amd64.RSP, 8, amd64.RDX)
	e.MovRegImm64(amd64.RAX, 35) // nanosleep
	e.MovRegReg(amd64.RDI, amd64.RSP)
	e.MovRegImm64(amd64.RSI, 0)
	e.Syscall()
	done := len(e.Code)
	patchJcc(doneJump, done)
	e.AddRegImm32(amd64.RSP, 16)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64TaskRunOne(e *amd64.Emitter, resumeOffset, nowOffset, sleepNSOffset int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.SubRegImm32(amd64.RSP, 8)

	// Microtasks always run before timers and ordinary runnable tasks.
	e.MovRegDeref(amd64.RBX, amd64.R15, amd64RTMicrotaskHead)
	e.TestRegReg(amd64.RBX, amd64.RBX)
	noMicrotask := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64TaskNext)
	e.MovDerefReg(amd64.R15, amd64RTMicrotaskHead, amd64.R10)
	e.TestRegReg(amd64.R10, amd64.R10)
	microHasNext := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegImm64(amd64.R11, 0)
	e.MovDerefReg(amd64.R15, amd64RTMicrotaskTail, amd64.R11)
	patchJcc(microHasNext, len(e.Code))
	e.MovRegImm64(amd64.R11, 0)
	e.MovDerefReg(amd64.RBX, amd64TaskNext, amd64.R11)
	microHaveTaskJump := len(e.Code)
	e.JmpRel32(0)

	microtasksEmpty := len(e.Code)
	patchJcc(noMicrotask, microtasksEmpty)

	// Due timers outrank ordinary runnable work so a cooperatively requeued waiter cannot
	// starve a sleeping task that will satisfy it.
	e.MovRegDeref(amd64.RBX, amd64.R15, amd64RTTimerHead)
	e.TestRegReg(amd64.RBX, amd64.RBX)
	noTimerPrecheck := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	callPreNow := len(e.Code)
	e.CallRel32(int32(nowOffset - (callPreNow + 5)))
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64TaskWakeNS)
	e.CmpRegReg(amd64.R10, amd64.RAX)
	timerDue := len(e.Code)
	e.JccRel32(amd64.CondLE, 0)

	// Prefer ordinary runnable work while the earliest timer is still pending.
	runnableLabel := len(e.Code)
	patchJcc(noTimerPrecheck, runnableLabel)
	e.MovRegDeref(amd64.RBX, amd64.R15, amd64RTTaskHead)
	e.TestRegReg(amd64.RBX, amd64.RBX)
	noRunnable := len(e.Code)
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
	haveRunnableJump := len(e.Code)
	e.JmpRel32(0)

	// No runnable work. Wait until the earliest timer if one exists.
	noRunnableLabel := len(e.Code)
	patchJcc(noRunnable, noRunnableLabel)
	e.MovRegDeref(amd64.RBX, amd64.R15, amd64RTTimerHead)
	e.TestRegReg(amd64.RBX, amd64.RBX)
	noWork := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	callNow := len(e.Code)
	e.CallRel32(int32(nowOffset - (callNow + 5)))
	e.MovRegDeref(amd64.RDI, amd64.RBX, amd64TaskWakeNS)
	e.SubRegReg(amd64.RDI, amd64.RAX)
	e.TestRegReg(amd64.RDI, amd64.RDI)
	timerReadyAfterWait := len(e.Code)
	e.JccRel32(amd64.CondLE, 0)
	callSleep := len(e.Code)
	e.CallRel32(int32(sleepNSOffset - (callSleep + 5)))
	timerReadyLabel := len(e.Code)
	patchJcc(timerReadyAfterWait, timerReadyLabel)
	timerDueLabel := len(e.Code)
	patchJcc(timerDue, timerDueLabel)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64TaskNext)
	e.MovDerefReg(amd64.R15, amd64RTTimerHead, amd64.R10)
	e.MovRegImm64(amd64.R11, 0)
	e.MovDerefReg(amd64.RBX, amd64TaskNext, amd64.R11)

	haveTask := len(e.Code)
	patchJmp(haveRunnableJump, haveTask)
	patchJmp(microHaveTaskJump, haveTask)
	e.MovRegReg(amd64.RDI, amd64.RBX)
	callResume := len(e.Code)
	e.CallRel32(int32(resumeOffset - (callResume + 5)))
	e.MovRegImm64(amd64.RAX, 1)
	doneJump := len(e.Code)
	e.JmpRel32(0)

	noWorkLabel := len(e.Code)
	patchJcc(noWork, noWorkLabel)
	e.MovRegImm64(amd64.RAX, 0)
	done := len(e.Code)
	patchJmp(doneJump, done)
	e.AddRegImm32(amd64.RSP, 8)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64TaskDrain(e *amd64.Emitter, runOneOffset int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	loop := len(e.Code)
	callRun := len(e.Code)
	e.CallRel32(int32(runOneOffset - (callRun + 5)))
	e.TestRegReg(amd64.RAX, amd64.RAX)
	doneJump := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	back := len(e.Code)
	e.JmpRel32(0)
	patchJmp(back, loop)
	done := len(e.Code)
	patchJcc(doneJump, done)
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
	e.JccRel32(amd64.CondAE, 0)
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

func emitAMD64TaskReject(e *amd64.Emitter) {
	// RDI = NaN-boxed JSValue rejection reason. Reject the currently running
	// task and restore the context that resumed it. A synchronous top-level
	// throw has no current task; terminate cleanly instead of dereferencing a
	// null task pointer.
	e.MovRegDeref(amd64.R11, amd64.R15, amd64RTCurrentTask)
	e.TestRegReg(amd64.R11, amd64.R11)
	hasTask := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegImm64(amd64.RDI, 1)
	e.MovRegImm64(amd64.RAX, 60) // Linux sys_exit
	e.Syscall()
	hasTaskLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[hasTask+2:], uint32(int32(hasTaskLabel-(hasTask+6))))
	e.MovDerefReg(amd64.R11, amd64TaskResult, amd64.RDI)
	e.MovRegImm64(amd64.R10, 3)
	e.MovDerefReg(amd64.R11, amd64TaskState, amd64.R10)
	e.MovRegImm64(amd64.R10, 0)
	e.MovDerefReg(amd64.R11, amd64TaskSavedRsp, amd64.R10)
	e.MovDerefReg(amd64.R11, amd64TaskSavedRoot, amd64.R10)
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
	e.MovRegDeref(amd64.RDX, amd64.R11, amd64TaskStackTop)
	e.SubRegImm32(amd64.RDX, int32(amd64TaskStackBytes))
	e.MovRegImm64(amd64.R10, 0)
	e.MovDerefReg(amd64.R11, amd64TaskReturnRoot, amd64.R10)
	e.MovDerefReg(amd64.R11, amd64TaskParent, amd64.R10)
	e.MovDerefReg(amd64.R11, amd64TaskReturnRsp, amd64.R10)
	e.MovDerefReg(amd64.R11, amd64TaskStackTop, amd64.R10)
	e.MovRegReg(amd64.RSP, amd64.RAX)
	e.MovRegReg(amd64.RDI, amd64.RDX)
	e.MovRegImm64(amd64.RSI, amd64TaskStackBytes)
	e.MovRegImm64(amd64.RAX, 11) // Linux munmap
	e.Syscall()
	e.Ret()
}

func emitAMD64TaskDone(e *amd64.Emitter) {
	// RDI = task. Return whether the task has fulfilled or rejected.
	e.MovRegDeref(amd64.R10, amd64.RDI, amd64TaskState)
	e.CmpRegImm32(amd64.R10, 2)
	e.Setcc(amd64.CondAE, amd64.RAX)
	e.Ret()
}

func emitAMD64TaskRejected(e *amd64.Emitter) {
	// RDI = task. Return canonical bool in RAX.
	e.MovRegDeref(amd64.R10, amd64.RDI, amd64TaskState)
	e.CmpRegImm32(amd64.R10, 3)
	e.Setcc(amd64.CondE, amd64.RAX)
	e.Ret()
}

func emitAMD64TaskError(e *amd64.Emitter) {
	// RDI = task. Return raw NaN-boxed rejection reason.
	e.MovRegDeref(amd64.RAX, amd64.RDI, amd64TaskResult)
	e.Ret()
}

func emitAMD64TaskSleep(e *amd64.Emitter, nowOffset, sleepNSOffset, suspendOffset int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)

	// Convert milliseconds to integer nanoseconds.
	e.Cvttsd2si(amd64.R12, amd64.XMM0)
	e.TestRegReg(amd64.R12, amd64.R12)
	doneJump := len(e.Code)
	e.JccRel32(amd64.CondLE, 0)
	e.MovRegImm64(amd64.R10, 1000000)
	e.ImulRegReg(amd64.R12, amd64.R10)

	// Main-context sleep remains blocking; task sleep enters the timer queue.
	e.MovRegDeref(amd64.RBX, amd64.R15, amd64RTCurrentTask)
	e.TestRegReg(amd64.RBX, amd64.RBX)
	mainSleep := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	callNow := len(e.Code)
	e.CallRel32(int32(nowOffset - (callNow + 5)))
	e.AddRegReg(amd64.R12, amd64.RAX)
	e.MovDerefReg(amd64.RBX, amd64TaskWakeNS, amd64.R12)

	// Sorted insert by wake deadline. Sleeping tasks reuse task.next while they
	// are not members of the runnable queue.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTTimerHead)
	e.TestRegReg(amd64.R10, amd64.R10)
	emptyTimers := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.R11, amd64.R10, amd64TaskWakeNS)
	e.CmpRegReg(amd64.R12, amd64.R11)
	beforeHead := len(e.Code)
	e.JccRel32(amd64.CondL, 0)

	// R10=previous, R11=current.
	e.MovRegDeref(amd64.R11, amd64.R10, amd64TaskNext)
	insertLoop := len(e.Code)
	e.TestRegReg(amd64.R11, amd64.R11)
	insertAfter := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.RAX, amd64.R11, amd64TaskWakeNS)
	e.CmpRegReg(amd64.R12, amd64.RAX)
	insertAfterCmp := len(e.Code)
	e.JccRel32(amd64.CondL, 0)
	e.MovRegReg(amd64.R10, amd64.R11)
	e.MovRegDeref(amd64.R11, amd64.R11, amd64TaskNext)
	loopBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(loopBack, insertLoop)
	insertAfterLabel := len(e.Code)
	patchJcc(insertAfter, insertAfterLabel)
	patchJcc(insertAfterCmp, insertAfterLabel)
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
	callSuspend := len(e.Code)
	e.CallRel32(int32(suspendOffset - (callSuspend + 5)))
	resumeJump := len(e.Code)
	e.JmpRel32(0)

	mainSleepLabel := len(e.Code)
	patchJcc(mainSleep, mainSleepLabel)
	e.MovRegReg(amd64.RDI, amd64.R12)
	callBlockingSleep := len(e.Code)
	e.CallRel32(int32(sleepNSOffset - (callBlockingSleep + 5)))

	done := len(e.Code)
	patchJcc(doneJump, done)
	patchJmp(resumeJump, done)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64TaskCancel(e *amd64.Emitter) {
	// RDI = task handle.
	e.MovRegImm64(amd64.R10, 1)
	e.MovDerefReg(amd64.RDI, amd64TaskCancelled, amd64.R10)
	e.Ret()
}

func emitAMD64TaskCancelled(e *amd64.Emitter) {
	// Return whether the currently running task has been cancelled. Main context
	// has no current task and therefore observes false.
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTCurrentTask)
	e.TestRegReg(amd64.R10, amd64.R10)
	noTask := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.RAX, amd64.R10, amd64TaskCancelled)
	doneJump := len(e.Code)
	e.JmpRel32(0)
	noTaskLabel := len(e.Code)
	patchJcc(noTask, noTaskLabel)
	e.MovRegImm64(amd64.RAX, 0)
	done := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[doneJump+1:], uint32(int32(done-(doneJump+5))))
	e.Ret()
}

func emitAMD64TaskSetContext(e *amd64.Emitter) {
	// RDI = native string payload. Main context has no task, so this is a no-op.
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTCurrentTask)
	e.TestRegReg(amd64.R10, amd64.R10)
	noTask := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovDerefReg(amd64.R10, amd64TaskContext, amd64.RDI)
	done := len(e.Code)
	patchJcc(noTask, done)
	e.Ret()
}

func emitAMD64TaskContext(e *amd64.Emitter) {
	// Returns the current task's native string payload or nil in main context.
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTCurrentTask)
	e.TestRegReg(amd64.R10, amd64.R10)
	noTask := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.RAX, amd64.R10, amd64TaskContext)
	doneJump := len(e.Code)
	e.JmpRel32(0)
	noTaskLabel := len(e.Code)
	patchJcc(noTask, noTaskLabel)
	e.MovRegImm64(amd64.RAX, 0)
	done := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[doneJump+1:], uint32(int32(done-(doneJump+5))))
	e.Ret()
}

const (
	amd64TaskGroupHead    int32 = 0
	amd64TaskGroupPayload int32 = 8
)

func emitAMD64TaskGroupNew(e *amd64.Emitter, allocOffset int) {
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.MovRegImm64(amd64.RDI, int64(amd64TaskGroupPayload))
	callAlloc := len(e.Code)
	e.CallRel32(int32(allocOffset - (callAlloc + 5)))
	emitAMD64SetObjectType(e, amd64.RAX, amd64ObjectTypeTaskGroup)
	e.MovRegImm64(amd64.R10, 0)
	e.MovDerefReg(amd64.RAX, amd64TaskGroupHead, amd64.R10)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64TaskGroupSpawn(e *amd64.Emitter, spawnOffset int) {
	// RDI=group, RSI=closure, XMM0=result kind. Returns task handle in RAX.
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.SubRegImm32(amd64.RSP, 8)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.RDI, amd64.RSI)
	callSpawn := len(e.Code)
	e.CallRel32(int32(spawnOffset - (callSpawn + 5)))
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64TaskGroupHead)
	e.MovDerefReg(amd64.RAX, amd64TaskGroupNext, amd64.R10)
	e.MovDerefReg(amd64.RBX, amd64TaskGroupHead, amd64.RAX)
	e.AddRegImm32(amd64.RSP, 8)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64TaskGroupJoin(e *amd64.Emitter, joinOffset int) {
	// RDI=group. Join each member task. Group membership remains intact so
	// individual task handles can still be joined afterward.
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.SubRegImm32(amd64.RSP, 8)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegDeref(amd64.R12, amd64.RBX, amd64TaskGroupHead)
	loop := len(e.Code)
	e.TestRegReg(amd64.R12, amd64.R12)
	done := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegReg(amd64.RDI, amd64.R12)
	callJoin := len(e.Code)
	e.CallRel32(int32(joinOffset - (callJoin + 5)))
	e.MovRegDeref(amd64.R12, amd64.R12, amd64TaskGroupNext)
	back := len(e.Code)
	e.JmpRel32(0)
	patchJmp(back, loop)
	patchJcc(done, len(e.Code))
	e.AddRegImm32(amd64.RSP, 8)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64TaskGroupCancel(e *amd64.Emitter) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	e.MovRegDeref(amd64.R10, amd64.RDI, amd64TaskGroupHead)
	loop := len(e.Code)
	e.TestRegReg(amd64.R10, amd64.R10)
	done := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegImm64(amd64.R11, 1)
	e.MovDerefReg(amd64.R10, amd64TaskCancelled, amd64.R11)
	e.MovRegDeref(amd64.R10, amd64.R10, amd64TaskGroupNext)
	back := len(e.Code)
	e.JmpRel32(0)
	patchJmp(back, loop)
	patchJcc(done, len(e.Code))
	e.Ret()
}
