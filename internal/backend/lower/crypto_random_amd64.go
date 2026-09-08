package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

// emitAMD64OSRandom fills a new ByteBuffer from Linux getrandom(2).
// ABI: XMM0=requested bytes -> RAX=buffer. Failure returns a zero-length buffer.
func emitAMD64OSRandom(e *amd64.Emitter, byteBufferNewOffset int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.SubRegImm32(amd64.RSP, 32)
	e.Cvttsd2si(amd64.R12, amd64.XMM0)
	e.TestRegReg(amd64.R12, amd64.R12)
	nonNegative := len(e.Code)
	e.JccRel32(amd64.CondGE, 0)
	e.MovRegImm64(amd64.R12, 0)
	patchJcc(nonNegative, len(e.Code))

	e.Cvtsi2sd(amd64.XMM0, amd64.R12)
	callNew := len(e.Code)
	e.CallRel32(int32(byteBufferNewOffset - (callNew + 5)))
	e.MovRegReg(amd64.RBX, amd64.RAX)
	e.MovRegImm64(amd64.R13, 0)
	loop := len(e.Code)
	e.CmpRegReg(amd64.R13, amd64.R12)
	complete := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	e.MovRegImm64(amd64.RAX, 318) // getrandom
	e.MovRegDeref(amd64.RDI, amd64.RBX, amd64ByteBufferData)
	e.AddRegReg(amd64.RDI, amd64.R13)
	e.MovRegReg(amd64.RSI, amd64.R12)
	e.SubRegReg(amd64.RSI, amd64.R13)
	e.MovRegImm64(amd64.RDX, 0)
	e.Syscall()
	e.TestRegReg(amd64.RAX, amd64.RAX)
	progress := len(e.Code)
	e.JccRel32(amd64.CondG, 0)
	e.CmpRegImm32(amd64.RAX, -4) // EINTR
	interrupted := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	failed := len(e.Code)
	e.JmpRel32(0)
	progressLabel := len(e.Code)
	patchJcc(progress, progressLabel)
	e.AddRegReg(amd64.R13, amd64.RAX)
	progressBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(progressBack, loop)
	interruptLabel := len(e.Code)
	patchJcc(interrupted, interruptLabel)
	interruptBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(interruptBack, loop)

	failureLabel := len(e.Code)
	patchJmp(failed, failureLabel)
	e.MovRegImm64(amd64.R10, 0)
	e.MovDerefReg(amd64.RBX, amd64ByteBufferLength, amd64.R10)
	returnJump := len(e.Code)
	e.JmpRel32(0)
	completeLabel := len(e.Code)
	patchJcc(complete, completeLabel)
	patchJmp(returnJump, len(e.Code))
	e.MovRegReg(amd64.RAX, amd64.RBX)
	e.AddRegImm32(amd64.RSP, 32)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}
