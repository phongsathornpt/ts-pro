package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

const (
	amd64TaskClosure int32 = 0
	amd64TaskState   int32 = 8
	amd64TaskResult  int32 = 16
	amd64TaskKind    int32 = 24
	amd64TaskNext    int32 = 32
	amd64TaskPayload int32 = 40

	amd64TaskResultVoid   int64 = 0
	amd64TaskResultNumber int64 = 1
	amd64TaskResultScalar int64 = 2
	amd64TaskResultRef    int64 = 3
	amd64TaskResultJS     int64 = 4
)

func emitAMD64TaskSpawn(e *amd64.Emitter, allocOffset int) {
	// RDI=closure payload, XMM0=result-kind number. Returns task payload in RAX.
	// Spawning is deferred: the task is appended to the cooperative runnable queue.
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.SubRegImm32(amd64.RSP, 32)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.Cvttsd2si(amd64.R12, amd64.XMM0)

	// Keep the closure alive if task allocation triggers a collection.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 1)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	e.MovRegImm64(amd64.RDI, int64(amd64TaskPayload))
	callAlloc := len(e.Code)
	e.CallRel32(int32(allocOffset - (callAlloc + 5)))
	emitAMD64SetObjectType(e, amd64.RAX, amd64ObjectTypeTask)
	e.MovDerefReg(amd64.RAX, amd64TaskClosure, amd64.RBX)
	e.MovRegImm64(amd64.R10, 0)
	e.MovDerefReg(amd64.RAX, amd64TaskState, amd64.R10)
	e.MovDerefReg(amd64.RAX, amd64TaskResult, amd64.R10)
	e.MovDerefReg(amd64.RAX, amd64TaskKind, amd64.R12)
	e.MovDerefReg(amd64.RAX, amd64TaskNext, amd64.R10)

	// FIFO enqueue.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTTaskTail)
	e.TestRegReg(amd64.R10, amd64.R10)
	emptyQueue := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovDerefReg(amd64.R10, amd64TaskNext, amd64.RAX)
	queueLinked := len(e.Code)
	e.JmpRel32(0)
	emptyQueueLabel := len(e.Code)
	patchJcc(emptyQueue, emptyQueueLabel)
	e.MovDerefReg(amd64.R15, amd64RTTaskHead, amd64.RAX)
	queueLinkedLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[queueLinked+1:], uint32(int32(queueLinkedLabel-(queueLinked+5))))
	e.MovDerefReg(amd64.R15, amd64RTTaskTail, amd64.RAX)

	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.AddRegImm32(amd64.RSP, 32)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64TaskRunOne(e *amd64.Emitter) {
	// Pop and execute one runnable task. Returns 1 in RAX if a task ran, 0 if
	// the queue was empty. The running task is linked into the precise-root chain
	// so it and its closure remain alive across allocations/GC in user code.
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.SubRegImm32(amd64.RSP, 40) // 24-byte root frame + alignment

	e.MovRegDeref(amd64.RBX, amd64.R15, amd64RTTaskHead)
	e.TestRegReg(amd64.RBX, amd64.RBX)
	empty := len(e.Code)
	e.JccRel32(amd64.CondE, 0)

	// Dequeue head and maintain tail.
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64TaskNext)
	e.MovDerefReg(amd64.R15, amd64RTTaskHead, amd64.R10)
	e.TestRegReg(amd64.R10, amd64.R10)
	hasNext := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegImm64(amd64.R11, 0)
	e.MovDerefReg(amd64.R15, amd64RTTaskTail, amd64.R11)
	hasNextLabel := len(e.Code)
	patchJcc(hasNext, hasNextLabel)
	e.MovRegImm64(amd64.R11, 0)
	e.MovDerefReg(amd64.RBX, amd64TaskNext, amd64.R11)

	// Root current task while it is no longer reachable from the queue.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 1)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	// Completed tasks should not normally be queued, but tolerate it without
	// executing their closure twice.
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64TaskState)
	e.CmpRegImm32(amd64.R10, 2)
	alreadyDone := len(e.Code)
	e.JccRel32(amd64.CondE, 0)

	e.MovRegImm64(amd64.R10, 1)
	e.MovDerefReg(amd64.RBX, amd64TaskState, amd64.R10)
	e.MovRegDeref(amd64.R12, amd64.RBX, amd64TaskClosure)
	e.MovRegReg(amd64.RDI, amd64.R12)
	e.MovRegDeref(amd64.R11, amd64.R12, 0)
	e.CallReg(amd64.R11)

	e.MovRegDeref(amd64.R13, amd64.RBX, amd64TaskKind)
	e.CmpRegImm32(amd64.R13, int32(amd64TaskResultNumber))
	notNumber := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovQRegXMM(amd64.RAX, amd64.XMM0)
	notNumberLabel := len(e.Code)
	patchJcc(notNumber, notNumberLabel)
	e.MovDerefReg(amd64.RBX, amd64TaskResult, amd64.RAX)
	e.MovRegImm64(amd64.R10, 2)
	e.MovDerefReg(amd64.RBX, amd64TaskState, amd64.R10)

	finished := len(e.Code)
	patchJcc(alreadyDone, finished)
	// Unlink running-task root frame.
	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.MovRegImm64(amd64.RAX, 1)
	doneJump := len(e.Code)
	e.JmpRel32(0)

	emptyLabel := len(e.Code)
	patchJcc(empty, emptyLabel)
	e.MovRegImm64(amd64.RAX, 0)

	done := len(e.Code)
	patchJmp(doneJump, done)
	e.AddRegImm32(amd64.RSP, 40)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64TaskJoin(e *amd64.Emitter, runOneOffset int) {
	// RDI=target task. Drain runnable work until target completes, then return
	// its cached result in both RAX and XMM0 raw-bit channels.
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
	e.TestRegReg(amd64.RAX, amd64.RAX)
	noProgress := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	back := len(e.Code)
	e.JmpRel32(0)
	patchJmp(back, loop)

	// No runnable task can complete a still-pending target. Keep waiting rather
	// than returning fabricated data; future blocking primitives will make work
	// runnable before returning control here.
	noProgressLabel := len(e.Code)
	patchJcc(noProgress, noProgressLabel)
	spinBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(spinBack, loop)

	done := len(e.Code)
	patchJcc(doneTask, done)
	e.MovRegDeref(amd64.RAX, amd64.RBX, amd64TaskResult)
	e.MovQXMMReg(amd64.XMM0, amd64.RAX)
	e.AddRegImm32(amd64.RSP, 8)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64TaskSleep(e *amd64.Emitter) {
	// XMM0 = milliseconds. Linux nanosleep expects a timespec {sec,nsec}.
	// This is intentionally a blocking sleep primitive for the current
	// cooperative scheduler; timer-queue suspension can replace the syscall
	// later without changing the frontend ABI.
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.SubRegImm32(amd64.RSP, 16)
	e.Cvttsd2si(amd64.RAX, amd64.XMM0)
	e.TestRegReg(amd64.RAX, amd64.RAX)
	nonPositive := len(e.Code)
	e.JccRel32(amd64.CondLE, 0)

	// quotient = milliseconds / 1000, remainder = milliseconds % 1000
	e.Cqo()
	e.MovRegImm64(amd64.R10, 1000)
	e.IdivReg(amd64.R10)
	e.MovDerefReg(amd64.RSP, 0, amd64.RAX)
	e.MovRegImm64(amd64.R10, 1000000)
	e.ImulRegReg(amd64.RDX, amd64.R10)
	e.MovDerefReg(amd64.RSP, 8, amd64.RDX)

	e.MovRegImm64(amd64.RAX, 35) // Linux nanosleep
	e.MovRegReg(amd64.RDI, amd64.RSP)
	e.MovRegImm64(amd64.RSI, 0)
	e.Syscall()

	done := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[nonPositive+2:], uint32(int32(done-(nonPositive+6))))
	e.AddRegImm32(amd64.RSP, 16)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64TaskYield(e *amd64.Emitter, runOneOffset int) {
	// Run at most one queued task and return to the yielding task/caller.
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	callRun := len(e.Code)
	e.CallRel32(int32(runOneOffset - (callRun + 5)))
	e.Pop(amd64.RBP)
	e.Ret()
}
