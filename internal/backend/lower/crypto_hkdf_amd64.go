package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

// emitAMD64HKDFExtract emits RFC 5869 HKDF-Extract using SHA-256.
// ABI: RDI=salt ByteBuffer, RSI=IKM ByteBuffer -> RAX=32-byte PRK.
func emitAMD64HKDFExtract(e *amd64.Emitter, hmacOffset int) {
	at := len(e.Code)
	e.CallRel32(int32(hmacOffset - (at + 5)))
	e.Ret()
}

// emitAMD64HKDFExpand emits RFC 5869 HKDF-Expand using SHA-256.
// ABI: RDI=PRK, RSI=info, XMM0=output length -> RAX=OKM ByteBuffer.
// Lengths outside 0..8160 fail closed with an empty ByteBuffer.
func emitAMD64HKDFExpand(e *amd64.Emitter, hmacOffset, newOffset, copyOffset int) {
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
	e.Push(amd64.R14)
	e.SubRegImm32(amd64.RSP, 112)
	e.MovRegReg(amd64.RBX, amd64.RDI) // PRK
	e.MovRegReg(amd64.R12, amd64.RSI) // info
	e.Cvttsd2si(amd64.R13, amd64.XMM0)

	e.TestRegReg(amd64.R13, amd64.R13)
	invalidNegative := len(e.Code)
	e.JccRel32(amd64.CondL, 0)
	e.CmpRegImm32(amd64.R13, 255*32)
	invalidLarge := len(e.Code)
	e.JccRel32(amd64.CondA, 0)

	// Root PRK/info/output/previous/message/digest across all allocations.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 6)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.RSP, 24, amd64.R12)
	e.MovRegImm64(amd64.R10, 0)
	for _, off := range []int32{32, 40, 48, 56} {
		e.MovDerefReg(amd64.RSP, off, amd64.R10)
	}
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	// Allocate the output once; HKDF blocks stream directly into it.
	e.Cvtsi2sd(amd64.XMM0, amd64.R13)
	callOut := len(e.Code)
	e.CallRel32(int32(newOffset - (callOut + 5)))
	e.MovRegReg(amd64.R14, amd64.RAX)
	e.MovDerefReg(amd64.RSP, 32, amd64.R14)
	e.MovRegImm64(amd64.R10, 0)
	e.MovDerefReg(amd64.RSP, 64, amd64.R10) // output offset
	e.MovRegImm64(amd64.R10, 1)
	e.MovDerefReg(amd64.RSP, 72, amd64.R10) // block counter

	e.TestRegReg(amd64.R13, amd64.R13)
	emptyDone := len(e.Code)
	e.JccRel32(amd64.CondE, 0)

	loop := len(e.Code)
	// T(1) has no prefix; subsequent blocks prefix the previous 32-byte T.
	e.MovRegDeref(amd64.R8, amd64.RSP, 72)
	e.CmpRegImm32(amd64.R8, 1)
	firstBlock := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegImm64(amd64.R9, 32)
	havePrefix := len(e.Code)
	e.JmpRel32(0)
	firstBlockLabel := len(e.Code)
	patchJcc(firstBlock, firstBlockLabel)
	e.MovRegImm64(amd64.R9, 0)
	patchJmp(havePrefix, len(e.Code))

	e.MovRegDeref(amd64.R10, amd64.R12, amd64ByteBufferLength)
	e.AddRegReg(amd64.R10, amd64.R9)
	e.AddRegImm32(amd64.R10, 1)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callMsg := len(e.Code)
	e.CallRel32(int32(newOffset - (callMsg + 5)))
	e.MovDerefReg(amd64.RSP, 48, amd64.RAX)

	// Copy T(n-1) for blocks after the first.
	e.MovRegDeref(amd64.R8, amd64.RSP, 72)
	e.CmpRegImm32(amd64.R8, 1)
	skipPrev := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegReg(amd64.RDI, amd64.RAX)
	e.MovRegDeref(amd64.RSI, amd64.RSP, 40)
	e.MovRegImm64(amd64.R10, 0)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	e.Cvtsi2sd(amd64.XMM1, amd64.R10)
	e.MovRegImm64(amd64.R10, 32)
	e.Cvtsi2sd(amd64.XMM2, amd64.R10)
	copyPrev := len(e.Code)
	e.CallRel32(int32(copyOffset - (copyPrev + 5)))
	copyInfoLabel := len(e.Code)
	patchJcc(skipPrev, copyInfoLabel)
	// info starts at offset 0 for T(1), otherwise after the 32-byte prefix.
	e.MovRegDeref(amd64.R8, amd64.RSP, 72)
	e.CmpRegImm32(amd64.R8, 1)
	infoAtZero := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegImm64(amd64.R9, 32)
	infoOffsetReady := len(e.Code)
	e.JmpRel32(0)
	infoAtZeroLabel := len(e.Code)
	patchJcc(infoAtZero, infoAtZeroLabel)
	e.MovRegImm64(amd64.R9, 0)
	patchJmp(infoOffsetReady, len(e.Code))

	e.MovRegDeref(amd64.RDI, amd64.RSP, 48)
	e.MovRegReg(amd64.RSI, amd64.R12)
	e.Cvtsi2sd(amd64.XMM0, amd64.R9)
	e.MovRegImm64(amd64.R10, 0)
	e.Cvtsi2sd(amd64.XMM1, amd64.R10)
	e.MovRegDeref(amd64.R10, amd64.R12, amd64ByteBufferLength)
	e.Cvtsi2sd(amd64.XMM2, amd64.R10)
	copyInfo := len(e.Code)
	e.CallRel32(int32(copyOffset - (copyInfo + 5)))

	// Append the one-byte RFC 5869 block counter.
	e.MovRegDeref(amd64.R10, amd64.RSP, 48)
	e.MovRegDeref(amd64.R11, amd64.R10, amd64ByteBufferData)
	e.MovRegDeref(amd64.RDX, amd64.R10, amd64ByteBufferLength)
	e.SubRegImm32(amd64.RDX, 1)
	e.AddRegReg(amd64.R11, amd64.RDX)
	e.MovRegDeref(amd64.RAX, amd64.RSP, 72)
	e.MovDerefReg8(amd64.R11, 0, amd64.RAX)

	e.MovRegReg(amd64.RDI, amd64.RBX)
	e.MovRegDeref(amd64.RSI, amd64.RSP, 48)
	callHMAC := len(e.Code)
	e.CallRel32(int32(hmacOffset - (callHMAC + 5)))
	e.MovDerefReg(amd64.RSP, 56, amd64.RAX)

	// copyLen = min(32, requestedLength-outputOffset).
	e.MovRegDeref(amd64.R8, amd64.RSP, 64)
	e.MovRegReg(amd64.R10, amd64.R13)
	e.SubRegReg(amd64.R10, amd64.R8)
	e.CmpRegImm32(amd64.R10, 32)
	copyLenReady := len(e.Code)
	e.JccRel32(amd64.CondBE, 0)
	e.MovRegImm64(amd64.R10, 32)
	patchJcc(copyLenReady, len(e.Code))
	e.MovDerefReg(amd64.RSP, 80, amd64.R10)

	e.MovRegReg(amd64.RDI, amd64.R14)
	e.MovRegDeref(amd64.RSI, amd64.RSP, 56)
	e.Cvtsi2sd(amd64.XMM0, amd64.R8)
	e.MovRegImm64(amd64.R10, 0)
	e.Cvtsi2sd(amd64.XMM1, amd64.R10)
	e.MovRegDeref(amd64.R10, amd64.RSP, 80)
	e.Cvtsi2sd(amd64.XMM2, amd64.R10)
	copyDigest := len(e.Code)
	e.CallRel32(int32(copyOffset - (copyDigest + 5)))

	// T(n) becomes the prefix for the next block.
	e.MovRegDeref(amd64.R10, amd64.RSP, 56)
	e.MovDerefReg(amd64.RSP, 40, amd64.R10)
	e.MovRegImm64(amd64.R11, 0)
	e.MovDerefReg(amd64.RSP, 48, amd64.R11)
	e.MovRegDeref(amd64.R8, amd64.RSP, 64)
	e.MovRegDeref(amd64.R10, amd64.RSP, 80)
	e.AddRegReg(amd64.R8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 64, amd64.R8)
	e.MovRegDeref(amd64.R10, amd64.RSP, 72)
	e.AddRegImm32(amd64.R10, 1)
	e.MovDerefReg(amd64.RSP, 72, amd64.R10)
	e.CmpRegReg(amd64.R8, amd64.R13)
	continueLoop := len(e.Code)
	e.JccRel32(amd64.CondB, 0)

	success := len(e.Code)
	patchJcc(emptyDone, success)
	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	patchJcc(continueLoop, loop)
	e.MovRegReg(amd64.RAX, amd64.R14)
	normalDone := len(e.Code)
	e.JmpRel32(0)

	invalid := len(e.Code)
	patchJcc(invalidNegative, invalid)
	patchJcc(invalidLarge, invalid)
	e.MovRegImm64(amd64.R10, 0)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callEmpty := len(e.Code)
	e.CallRel32(int32(newOffset - (callEmpty + 5)))

	epilogue := len(e.Code)
	patchJmp(normalDone, epilogue)
	e.AddRegImm32(amd64.RSP, 112)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}
