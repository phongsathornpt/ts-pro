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
	amd64TaskPayload int32 = 32

	amd64TaskResultVoid   int64 = 0
	amd64TaskResultNumber int64 = 1
	amd64TaskResultScalar int64 = 2
	amd64TaskResultRef    int64 = 3
	amd64TaskResultJS     int64 = 4
)

func emitAMD64TaskSpawn(e *amd64.Emitter, allocOffset int) {
	// RDI=closure payload, XMM0=result-kind number. Returns task payload in RAX.
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.SubRegImm32(amd64.RSP, 32)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.Cvttsd2si(amd64.R12, amd64.XMM0)

	// Keep the closure alive if allocation triggers a collection.
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

	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.AddRegImm32(amd64.RSP, 32)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64TaskJoin(e *amd64.Emitter) {
	// RDI=task payload. Return cached/executed result in RAX and mirror raw bits
	// into XMM0 so number callers use the ordinary SysV number result ABI.
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.MovRegReg(amd64.RBX, amd64.RDI)

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

	e.MovRegDeref(amd64.R10, amd64.RBX, amd64TaskKind)
	e.CmpRegImm32(amd64.R10, int32(amd64TaskResultNumber))
	notNumber := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovQRegXMM(amd64.RAX, amd64.XMM0)
	notNumberLabel := len(e.Code)
	patchJcc(notNumber, notNumberLabel)
	e.MovDerefReg(amd64.RBX, amd64TaskResult, amd64.RAX)
	e.MovRegImm64(amd64.R10, 2)
	e.MovDerefReg(amd64.RBX, amd64TaskState, amd64.R10)

	done := len(e.Code)
	patchJcc(alreadyDone, done)
	e.MovRegDeref(amd64.RAX, amd64.RBX, amd64TaskResult)
	e.MovQXMMReg(amd64.XMM0, amd64.RAX)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}
