package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

const (
	amd64AbortSignalReasonData int32 = amd64ObjectFields + 16
	amd64AbortSignalOnAbort    int32 = amd64ObjectFields + 24
	amd64AbortSignalAborted    int32 = amd64ObjectFields + 32
)

func emitAMD64AbortSignalNew(e *amd64.Emitter, objectNewOffset, cellNewOffset int) {
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.SubRegImm32(amd64.RSP, 32)
	e.MovRegImm64(amd64.RDI, 5)
	e.MovRegImm64(amd64.RSI, 0b1111)
	callSignal := len(e.Code)
	e.CallRel32(int32(objectNewOffset - (callSignal + 5)))
	e.MovRegReg(amd64.RBX, amd64.RAX)
	// Root the signal while allocating its JSValue-aware reason slot.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 1)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	callReason := len(e.Code)
	e.CallRel32(int32(cellNewOffset - (callReason + 5)))
	e.MovDerefReg(amd64.RBX, amd64AbortSignalReasonData, amd64.RAX)

	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.MovRegReg(amd64.RAX, amd64.RBX)
	e.AddRegImm32(amd64.RSP, 32)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64AbortSignalAborted(e *amd64.Emitter) {
	e.MovRegDeref(amd64.RAX, amd64.RDI, amd64AbortSignalAborted)
	e.Ret()
}

func emitAMD64AbortSignalReason(e *amd64.Emitter) {
	e.MovRegDeref(amd64.RAX, amd64.RDI, amd64AbortSignalReasonData)
	e.MovRegDeref(amd64.RAX, amd64.RAX, 0)
	e.Ret()
}

func emitAMD64AbortSignalSetReason(e *amd64.Emitter) {
	patchJcc := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6))))
	}
	e.MovRegDeref(amd64.R10, amd64.RDI, amd64AbortSignalAborted)
	e.TestRegReg(amd64.R10, amd64.R10)
	already := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegImm64(amd64.R10, 1)
	e.MovDerefReg(amd64.RDI, amd64AbortSignalAborted, amd64.R10)
	e.MovRegDeref(amd64.R11, amd64.RDI, amd64AbortSignalReasonData)
	e.MovDerefReg(amd64.R11, 0, amd64.RSI)
	e.MovRegImm64(amd64.RAX, 1)
	e.Ret()
	patchJcc(already, len(e.Code))
	e.MovRegImm64(amd64.RAX, 0)
	e.Ret()
}

func emitAMD64AbortSignalOnAbort(e *amd64.Emitter) {
	e.MovRegDeref(amd64.RAX, amd64.RDI, amd64AbortSignalOnAbort)
	e.Ret()
}

func emitAMD64AbortSignalSetOnAbort(e *amd64.Emitter) {
	e.MovDerefReg(amd64.RDI, amd64AbortSignalOnAbort, amd64.RSI)
	e.Ret()
}
