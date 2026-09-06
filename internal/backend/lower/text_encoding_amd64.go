package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

func emitAMD64TextEncodeInto(e *amd64.Emitter, objectNewOffset int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.SubRegImm32(amd64.RSP, 16)

	// RDI=source string, RSI=destination ByteBuffer, XMM0=byteOffset, XMM1=capacity.
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)
	e.Cvttsd2si(amd64.R13, amd64.XMM0)
	e.Cvttsd2si(amd64.R14, amd64.XMM1)

	e.MovRegReg(amd64.R8, amd64.RBX)
	e.AddRegImm32(amd64.R8, 8)
	e.MovRegDeref(amd64.R9, amd64.RBX, 0)
	e.MovRegDeref(amd64.R10, amd64.R12, amd64ByteBufferData)
	e.AddRegReg(amd64.R10, amd64.R13)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10) // destination start
	e.MovRegImm64(amd64.RAX, 0)
	e.MovDerefReg(amd64.RSP, 0, amd64.RAX) // UTF-16 units read

	loop := len(e.Code)
	e.TestRegReg(amd64.R9, amd64.R9)
	doneSource := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.TestRegReg(amd64.R14, amd64.R14)
	doneCapacity := len(e.Code)
	e.JccRel32(amd64.CondE, 0)

	e.MovzxRegDeref8(amd64.R11, amd64.R8, 0)
	e.CmpRegImm32(amd64.R11, 0x80)
	isASCII := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.CmpRegImm32(amd64.R11, 0xE0)
	isTwo := len(e.Code)
	e.JccRel32(amd64.CondB, 0)

	e.CmpRegImm32(amd64.R11, 0xF0)
	isThree := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	// Valid internal strings use a canonical four-byte UTF-8 sequence here.
	e.MovRegImm64(amd64.RSI, 4)
	e.MovRegImm64(amd64.RDI, 2)
	seqReadyFour := len(e.Code)
	e.JmpRel32(0)

	asciiLabel := len(e.Code)
	patchJcc(isASCII, asciiLabel)
	e.MovRegImm64(amd64.RSI, 1)
	e.MovRegImm64(amd64.RDI, 1)
	seqReadyASCII := len(e.Code)
	e.JmpRel32(0)

	twoLabel := len(e.Code)
	patchJcc(isTwo, twoLabel)
	e.MovRegImm64(amd64.RSI, 2)
	e.MovRegImm64(amd64.RDI, 1)
	seqReadyTwo := len(e.Code)
	e.JmpRel32(0)

	threeLabel := len(e.Code)
	patchJcc(isThree, threeLabel)
	e.MovRegImm64(amd64.RSI, 3)
	e.MovRegImm64(amd64.RDI, 1)
	seqReadyThree := len(e.Code)
	e.JmpRel32(0)

	seqReady := len(e.Code)
	for _, at := range []int{seqReadyFour, seqReadyASCII, seqReadyTwo, seqReadyThree} {
		patchJmp(at, seqReady)
	}
	// Stop rather than split an encoded scalar value across the destination boundary.
	e.CmpRegReg(amd64.R9, amd64.RSI)
	doneTruncatedSource := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.CmpRegReg(amd64.R14, amd64.RSI)
	doneShortDest := len(e.Code)
	e.JccRel32(amd64.CondB, 0)

	// Add UTF-16 code units consumed for this scalar value.
	e.MovRegDeref(amd64.RAX, amd64.RSP, 0)
	e.AddRegReg(amd64.RAX, amd64.RDI)
	e.MovDerefReg(amd64.RSP, 0, amd64.RAX)

	e.MovRegReg(amd64.RCX, amd64.RSI)
	copyLoop := len(e.Code)
	e.MovzxRegDeref8(amd64.RAX, amd64.R8, 0)
	e.MovDerefReg8(amd64.R10, 0, amd64.RAX)
	e.AddRegImm32(amd64.R8, 1)
	e.AddRegImm32(amd64.R10, 1)
	e.SubRegImm32(amd64.RCX, 1)
	copyBack := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	patchJcc(copyBack, copyLoop)
	e.SubRegReg(amd64.R9, amd64.RSI)
	e.SubRegReg(amd64.R14, amd64.RSI)
	loopBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(loopBack, loop)

	done := len(e.Code)
	for _, at := range []int{doneSource, doneCapacity, doneTruncatedSource, doneShortDest} {
		patchJcc(at, done)
	}
	e.MovRegDeref(amd64.R13, amd64.RSP, 0)
	e.MovRegReg(amd64.R14, amd64.R10)
	e.MovRegDeref(amd64.RAX, amd64.RSP, 8)
	e.SubRegReg(amd64.R14, amd64.RAX)

	// Allocate { read, written } after the copy; preserve counts in callee-saved regs.
	e.MovRegImm64(amd64.RDI, 2)
	e.MovRegImm64(amd64.RSI, 0)
	callResult := len(e.Code)
	e.CallRel32(int32(objectNewOffset - (callResult + 5)))
	e.Cvtsi2sd(amd64.XMM0, amd64.R13)
	e.MovQRegXMM(amd64.R10, amd64.XMM0)
	e.MovDerefReg(amd64.RAX, amd64ObjectFields, amd64.R10)
	e.Cvtsi2sd(amd64.XMM0, amd64.R14)
	e.MovQRegXMM(amd64.R10, amd64.XMM0)
	e.MovDerefReg(amd64.RAX, amd64ObjectFields+8, amd64.R10)

	e.AddRegImm32(amd64.RSP, 16)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}
