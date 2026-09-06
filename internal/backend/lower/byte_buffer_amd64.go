package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

const (
	amd64ByteBufferData     int32 = amd64ObjectFields
	amd64ByteBufferLength   int32 = amd64ObjectFields + 8
	amd64ByteBufferCapacity int32 = amd64ObjectFields + 16
)

func emitAMD64ByteBufferNew(e *amd64.Emitter, objectNewOffset, allocOffset int) {
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.SubRegImm32(amd64.RSP, 32)
	e.Cvttsd2si(amd64.R12, amd64.XMM0)
	e.TestRegReg(amd64.R12, amd64.R12)
	nonNegative := len(e.Code)
	e.JccRel32(amd64.CondGE, 0)
	e.MovRegImm64(amd64.R12, 0)
	binary.LittleEndian.PutUint32(e.Code[nonNegative+2:], uint32(int32(len(e.Code)-(nonNegative+6))))

	e.MovRegImm64(amd64.RDI, 3)
	e.MovRegImm64(amd64.RSI, 0b001)
	callObject := len(e.Code)
	e.CallRel32(int32(objectNewOffset - (callObject + 5)))
	e.MovRegReg(amd64.RBX, amd64.RAX)

	// Root the wrapper while allocating its raw byte backing store.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 1)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	e.MovRegReg(amd64.RDI, amd64.R12)
	e.TestRegReg(amd64.RDI, amd64.RDI)
	nonZero := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegImm64(amd64.RDI, 1)
	binary.LittleEndian.PutUint32(e.Code[nonZero+2:], uint32(int32(len(e.Code)-(nonZero+6))))
	callData := len(e.Code)
	e.CallRel32(int32(allocOffset - (callData + 5)))
	emitAMD64SetObjectType(e, amd64.RAX, amd64ObjectTypeArrayData)

	e.MovDerefReg(amd64.RBX, amd64ByteBufferData, amd64.RAX)
	e.MovDerefReg(amd64.RBX, amd64ByteBufferLength, amd64.R12)
	e.MovRegReg(amd64.R10, amd64.R12)
	e.TestRegReg(amd64.R10, amd64.R10)
	capacityReady := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegImm64(amd64.R10, 1)
	binary.LittleEndian.PutUint32(e.Code[capacityReady+2:], uint32(int32(len(e.Code)-(capacityReady+6))))
	e.MovDerefReg(amd64.RBX, amd64ByteBufferCapacity, amd64.R10)

	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.MovRegReg(amd64.RAX, amd64.RBX)
	e.AddRegImm32(amd64.RSP, 32)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64ByteBufferLength(e *amd64.Emitter) {
	e.MovRegDeref(amd64.RAX, amd64.RDI, amd64ByteBufferLength)
	e.Cvtsi2sd(amd64.XMM0, amd64.RAX)
	e.Ret()
}

func emitAMD64ByteBufferGet(e *amd64.Emitter) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	e.Cvttsd2si(amd64.R10, amd64.XMM0)
	e.MovRegDeref(amd64.R11, amd64.RDI, amd64ByteBufferLength)
	e.CmpRegReg(amd64.R10, amd64.R11)
	oob := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	e.MovRegDeref(amd64.R11, amd64.RDI, amd64ByteBufferData)
	e.AddRegReg(amd64.R11, amd64.R10)
	e.MovzxRegDeref8(amd64.RAX, amd64.R11, 0)
	e.Cvtsi2sd(amd64.XMM0, amd64.RAX)
	e.Ret()
	oobLabel := len(e.Code)
	patchJcc(oob, oobLabel)
	e.MovRegImm64(amd64.RAX, 0)
	e.Cvtsi2sd(amd64.XMM0, amd64.RAX)
	e.Ret()
}

func emitAMD64ByteBufferSet(e *amd64.Emitter) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	e.Cvttsd2si(amd64.R10, amd64.XMM0)
	e.Cvttsd2si(amd64.R11, amd64.XMM1)

	e.MovRegDeref(amd64.RAX, amd64.RDI, amd64ByteBufferLength)
	e.CmpRegReg(amd64.R10, amd64.RAX)
	oob := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	e.MovRegDeref(amd64.RAX, amd64.RDI, amd64ByteBufferData)
	e.AddRegReg(amd64.RAX, amd64.R10)
	e.MovRegImm64(amd64.R10, 255)
	e.AndRegReg(amd64.R11, amd64.R10)
	e.MovDerefReg8(amd64.RAX, 0, amd64.R11)
	e.Ret()
	patchJcc(oob, len(e.Code))
	e.Ret()
}

func emitAMD64ByteBufferCopy(e *amd64.Emitter) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	e.Cvttsd2si(amd64.R8, amd64.XMM0)  // dst offset
	e.Cvttsd2si(amd64.R9, amd64.XMM1)  // src offset
	e.Cvttsd2si(amd64.R10, amd64.XMM2) // requested count
	e.MovRegDeref(amd64.R11, amd64.RDI, amd64ByteBufferLength)
	e.CmpRegReg(amd64.R8, amd64.R11)
	doneDst := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)

	e.MovRegDeref(amd64.RAX, amd64.RSI, amd64ByteBufferLength)
	e.CmpRegReg(amd64.R9, amd64.RAX)
	doneSrc := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)

	// Clamp count to destination and source availability.
	e.SubRegReg(amd64.R11, amd64.R8)
	e.CmpRegReg(amd64.R10, amd64.R11)
	countDstOK := len(e.Code)
	e.JccRel32(amd64.CondBE, 0)
	e.MovRegReg(amd64.R10, amd64.R11)
	patchJcc(countDstOK, len(e.Code))
	e.SubRegReg(amd64.RAX, amd64.R9)
	e.CmpRegReg(amd64.R10, amd64.RAX)
	countSrcOK := len(e.Code)
	e.JccRel32(amd64.CondBE, 0)
	e.MovRegReg(amd64.R10, amd64.RAX)
	patchJcc(countSrcOK, len(e.Code))

	e.MovRegDeref(amd64.R11, amd64.RDI, amd64ByteBufferData)
	e.AddRegReg(amd64.R11, amd64.R8)
	e.MovRegDeref(amd64.RAX, amd64.RSI, amd64ByteBufferData)
	e.AddRegReg(amd64.RAX, amd64.R9)
	loop := len(e.Code)
	e.TestRegReg(amd64.R10, amd64.R10)
	doneCount := len(e.Code)
	e.JccRel32(amd64.CondE, 0)

	e.MovzxRegDeref8(amd64.RDX, amd64.RAX, 0)
	e.MovDerefReg8(amd64.R11, 0, amd64.RDX)
	e.AddRegImm32(amd64.RAX, 1)
	e.AddRegImm32(amd64.R11, 1)
	e.SubRegImm32(amd64.R10, 1)
	back := len(e.Code)
	e.JmpRel32(0)
	patchJmp(back, loop)
	done := len(e.Code)
	patchJcc(doneDst, done)
	patchJcc(doneSrc, done)
	patchJcc(doneCount, done)
	e.Ret()
}

func emitAMD64ByteBufferSlice(e *amd64.Emitter, newOffset, copyOffset int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.Cvttsd2si(amd64.R12, amd64.XMM0)
	e.Cvttsd2si(amd64.R13, amd64.XMM1)
	e.TestRegReg(amd64.R12, amd64.R12)
	startOK := len(e.Code)
	e.JccRel32(amd64.CondGE, 0)
	e.MovRegImm64(amd64.R12, 0)
	patchJcc(startOK, len(e.Code))
	e.TestRegReg(amd64.R13, amd64.R13)
	endOK := len(e.Code)
	e.JccRel32(amd64.CondGE, 0)
	e.MovRegImm64(amd64.R13, 0)
	patchJcc(endOK, len(e.Code))

	e.MovRegDeref(amd64.R10, amd64.RBX, amd64ByteBufferLength)
	e.CmpRegReg(amd64.R12, amd64.R10)
	startInRange := len(e.Code)
	e.JccRel32(amd64.CondBE, 0)
	e.MovRegReg(amd64.R12, amd64.R10)
	patchJcc(startInRange, len(e.Code))
	e.CmpRegReg(amd64.R13, amd64.R10)
	endInRange := len(e.Code)
	e.JccRel32(amd64.CondBE, 0)
	e.MovRegReg(amd64.R13, amd64.R10)
	patchJcc(endInRange, len(e.Code))
	e.CmpRegReg(amd64.R13, amd64.R12)
	ordered := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	e.MovRegReg(amd64.R13, amd64.R12)
	patchJcc(ordered, len(e.Code))

	e.MovRegReg(amd64.R14, amd64.R13)
	e.SubRegReg(amd64.R14, amd64.R12)
	e.Cvtsi2sd(amd64.XMM0, amd64.R14)
	callNew := len(e.Code)
	e.CallRel32(int32(newOffset - (callNew + 5)))
	e.MovRegReg(amd64.R13, amd64.RAX)

	e.MovRegReg(amd64.RDI, amd64.R13)
	e.MovRegReg(amd64.RSI, amd64.RBX)
	e.MovRegImm64(amd64.R10, 0)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	e.Cvtsi2sd(amd64.XMM1, amd64.R12)
	e.Cvtsi2sd(amd64.XMM2, amd64.R14)
	callCopy := len(e.Code)
	e.CallRel32(int32(copyOffset - (callCopy + 5)))
	e.MovRegReg(amd64.RAX, amd64.R13)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64ByteBufferFromUTF8String(e *amd64.Emitter, newOffset int) {
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.SubRegImm32(amd64.RSP, 40)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegDeref(amd64.R12, amd64.RBX, 0)

	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 1)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)
	e.Cvtsi2sd(amd64.XMM0, amd64.R12)
	callNew := len(e.Code)
	e.CallRel32(int32(newOffset - (callNew + 5)))
	e.MovRegReg(amd64.R13, amd64.RAX)

	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.MovRegDeref(amd64.R10, amd64.R13, amd64ByteBufferData)
	emitAMD64CopyStringBytes(e, amd64.RBX, amd64.R12)
	e.MovRegReg(amd64.RAX, amd64.R13)
	e.AddRegImm32(amd64.RSP, 40)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64ByteBufferToUTF8String(e *amd64.Emitter, allocOffset int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.SubRegImm32(amd64.RSP, 40)

	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegDeref(amd64.R12, amd64.RBX, amd64ByteBufferLength)
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 1)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	e.MovRegReg(amd64.RDI, amd64.R12)
	e.AddRegImm32(amd64.RDI, 8)
	callAlloc := len(e.Code)
	e.CallRel32(int32(allocOffset - (callAlloc + 5)))
	e.MovRegReg(amd64.R13, amd64.RAX)
	e.MovDerefReg(amd64.R13, 0, amd64.R12)

	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.MovRegDeref(amd64.R8, amd64.RBX, amd64ByteBufferData)
	e.MovRegReg(amd64.R10, amd64.R13)
	e.AddRegImm32(amd64.R10, 8)
	e.MovRegReg(amd64.R9, amd64.R12)

	e.TestRegReg(amd64.R9, amd64.R9)
	done := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	loop := len(e.Code)
	e.MovzxRegDeref8(amd64.RAX, amd64.R8, 0)
	e.MovDerefReg8(amd64.R10, 0, amd64.RAX)
	e.AddRegImm32(amd64.R8, 1)
	e.AddRegImm32(amd64.R10, 1)
	e.SubRegImm32(amd64.R9, 1)
	back := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	patchJcc(back, loop)
	patchJcc(done, len(e.Code))
	e.MovRegReg(amd64.RAX, amd64.R13)
	e.AddRegImm32(amd64.RSP, 40)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}
