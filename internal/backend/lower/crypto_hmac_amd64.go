package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

// emitAMD64HMACSHA256 emits RFC 2104 HMAC-SHA256.
// ABI: RDI=key ByteBuffer, RSI=message ByteBuffer -> RAX=32-byte digest.
func emitAMD64HMACSHA256(e *amd64.Emitter, sha256Offset, byteBufferNewOffset int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	callNew := func(length int64) {
		e.MovRegImm64(amd64.R10, length)
		e.Cvtsi2sd(amd64.XMM0, amd64.R10)
		at := len(e.Code)
		e.CallRel32(int32(byteBufferNewOffset - (at + 5)))
	}
	fillPad := func(bufferReg amd64.Register, pad int64) {
		e.MovRegImm64(amd64.RCX, 0)
		loop := len(e.Code)
		e.CmpRegImm32(amd64.RCX, 64)
		done := len(e.Code)
		e.JccRel32(amd64.CondAE, 0)
		e.MovRegDeref(amd64.RDX, amd64.R13, amd64ByteBufferLength)
		e.CmpRegReg(amd64.RCX, amd64.RDX)
		missing := len(e.Code)
		e.JccRel32(amd64.CondAE, 0)
		e.MovRegDeref(amd64.R11, amd64.R13, amd64ByteBufferData)
		e.AddRegReg(amd64.R11, amd64.RCX)
		e.MovzxRegDeref8(amd64.RAX, amd64.R11, 0)
		haveByte := len(e.Code)
		e.JmpRel32(0)
		missingLabel := len(e.Code)
		patchJcc(missing, missingLabel)
		e.MovRegImm64(amd64.RAX, 0)
		patchJmp(haveByte, len(e.Code))
		e.MovRegImm64(amd64.R11, pad)
		e.XorRegReg(amd64.RAX, amd64.R11)
		e.MovRegDeref(amd64.R10, bufferReg, amd64ByteBufferData)
		e.AddRegReg(amd64.R10, amd64.RCX)
		e.MovDerefReg8(amd64.R10, 0, amd64.RAX)
		e.AddRegImm32(amd64.RCX, 1)
		back := len(e.Code)
		e.JmpRel32(0)
		patchJmp(back, loop)
		patchJcc(done, len(e.Code))
	}
	copyBytes := func(dstReg amd64.Register, dstOffset int32, srcReg amd64.Register, count int32) {
		e.MovRegImm64(amd64.RCX, 0)
		loop := len(e.Code)
		e.CmpRegImm32(amd64.RCX, count)
		done := len(e.Code)
		e.JccRel32(amd64.CondAE, 0)
		e.MovRegDeref(amd64.R11, srcReg, amd64ByteBufferData)
		e.AddRegReg(amd64.R11, amd64.RCX)
		e.MovzxRegDeref8(amd64.RAX, amd64.R11, 0)
		e.MovRegDeref(amd64.R10, dstReg, amd64ByteBufferData)
		e.AddRegImm32(amd64.R10, dstOffset)
		e.AddRegReg(amd64.R10, amd64.RCX)
		e.MovDerefReg8(amd64.R10, 0, amd64.RAX)
		e.AddRegImm32(amd64.RCX, 1)
		back := len(e.Code)
		e.JmpRel32(0)
		patchJmp(back, loop)
		patchJcc(done, len(e.Code))
	}

	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.SubRegImm32(amd64.RSP, 96)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)

	// Precise roots survive the allocations used to construct HMAC inputs.
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

	// Keys longer than the 64-byte SHA-256 block are hashed first.
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64ByteBufferLength)
	e.CmpRegImm32(amd64.R10, 64)
	useOriginal := len(e.Code)
	e.JccRel32(amd64.CondBE, 0)
	e.MovRegReg(amd64.RDI, amd64.RBX)
	callKeyHash := len(e.Code)
	e.CallRel32(int32(sha256Offset - (callKeyHash + 5)))
	e.MovRegReg(amd64.R13, amd64.RAX)
	normalized := len(e.Code)
	e.JmpRel32(0)
	useOriginalLabel := len(e.Code)
	patchJcc(useOriginal, useOriginalLabel)
	e.MovRegReg(amd64.R13, amd64.RBX)
	patchJmp(normalized, len(e.Code))
	e.MovDerefReg(amd64.RSP, 32, amd64.R13)

	// inner = (K xor ipad) || message
	e.MovRegDeref(amd64.R10, amd64.R12, amd64ByteBufferLength)
	e.AddRegImm32(amd64.R10, 64)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callInnerNew := len(e.Code)
	e.CallRel32(int32(byteBufferNewOffset - (callInnerNew + 5)))
	e.MovRegReg(amd64.R14, amd64.RAX)
	e.MovDerefReg(amd64.RSP, 40, amd64.R14)
	fillPad(amd64.R14, 0x36)

	// Copy arbitrary-length message after the 64-byte ipad.
	e.MovRegImm64(amd64.RCX, 0)
	msgLoop := len(e.Code)
	e.MovRegDeref(amd64.RDX, amd64.R12, amd64ByteBufferLength)
	e.CmpRegReg(amd64.RCX, amd64.RDX)
	msgDone := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	e.MovRegDeref(amd64.R11, amd64.R12, amd64ByteBufferData)
	e.AddRegReg(amd64.R11, amd64.RCX)
	e.MovzxRegDeref8(amd64.RAX, amd64.R11, 0)
	e.MovRegDeref(amd64.R10, amd64.R14, amd64ByteBufferData)
	e.AddRegImm32(amd64.R10, 64)
	e.AddRegReg(amd64.R10, amd64.RCX)
	e.MovDerefReg8(amd64.R10, 0, amd64.RAX)
	e.AddRegImm32(amd64.RCX, 1)
	msgBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(msgBack, msgLoop)
	patchJcc(msgDone, len(e.Code))

	e.MovRegReg(amd64.RDI, amd64.R14)
	callInnerHash := len(e.Code)
	e.CallRel32(int32(sha256Offset - (callInnerHash + 5)))
	e.MovDerefReg(amd64.RSP, 48, amd64.RAX)

	// outer = (K xor opad) || innerDigest
	callNew(96)
	e.MovRegReg(amd64.R14, amd64.RAX)
	e.MovDerefReg(amd64.RSP, 56, amd64.R14)
	fillPad(amd64.R14, 0x5c)
	e.MovRegDeref(amd64.R12, amd64.RSP, 48)
	copyBytes(amd64.R14, 64, amd64.R12, 32)
	e.MovRegReg(amd64.RDI, amd64.R14)
	callOuterHash := len(e.Code)
	e.CallRel32(int32(sha256Offset - (callOuterHash + 5)))

	// RAX is the result. Drop our precise root frame before returning it.
	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.AddRegImm32(amd64.RSP, 96)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}
