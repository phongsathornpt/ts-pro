package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

var amd64SHA1Initial = [...]uint32{
	0x67452301, 0xefcdab89, 0x98badcfe, 0x10325476, 0xc3d2e1f0,
}

// emitAMD64SHA1Compress emits the SHA-1 compression function.
// ABI: RDI=64-byte block, RSI=5-word hash state. No allocation or GC occurs.
func emitAMD64SHA1Compress(e *amd64.Emitter) {
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.SubRegImm32(amd64.RSP, 352)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)

	for i := 0; i < 16; i++ {
		e.MovRegDeref32(amd64.RAX, amd64.RBX, int32(i*4))
		e.BswapReg32(amd64.RAX)
		e.MovDerefReg32(amd64.RSP, int32(i*4), amd64.RAX)
	}
	for i := 16; i < 80; i++ {
		e.MovRegDeref32(amd64.RAX, amd64.RSP, int32((i-3)*4))
		e.MovRegDeref32(amd64.RCX, amd64.RSP, int32((i-8)*4))
		e.XorReg32Reg(amd64.RAX, amd64.RCX)
		e.MovRegDeref32(amd64.RCX, amd64.RSP, int32((i-14)*4))
		e.XorReg32Reg(amd64.RAX, amd64.RCX)
		e.MovRegDeref32(amd64.RCX, amd64.RSP, int32((i-16)*4))
		e.XorReg32Reg(amd64.RAX, amd64.RCX)
		e.RorReg32Imm8(amd64.RAX, 31)
		e.MovDerefReg32(amd64.RSP, int32(i*4), amd64.RAX)
	}

	for i := 0; i < 5; i++ {
		e.MovRegDeref32(amd64.RAX, amd64.R12, int32(i*4))
		e.MovDerefReg32(amd64.RSP, int32(320+i*4), amd64.RAX)
	}

	for i := 0; i < 80; i++ {
		// R8=f(b,c,d), R9=k.
		e.MovRegDeref32(amd64.R8, amd64.RSP, 324)
		switch {
		case i < 20:
			e.MovRegReg(amd64.R9, amd64.R8)
			e.MovRegDeref32(amd64.R10, amd64.RSP, 328)
			e.AndReg32Reg(amd64.R8, amd64.R10)
			e.NotReg32(amd64.R9)
			e.MovRegDeref32(amd64.R10, amd64.RSP, 332)
			e.AndReg32Reg(amd64.R9, amd64.R10)
			e.XorReg32Reg(amd64.R8, amd64.R9)
			e.MovRegImm32(amd64.R9, 0x5a827999)
		case i < 40:
			e.MovRegDeref32(amd64.R9, amd64.RSP, 328)
			e.XorReg32Reg(amd64.R8, amd64.R9)
			e.MovRegDeref32(amd64.R9, amd64.RSP, 332)
			e.XorReg32Reg(amd64.R8, amd64.R9)
			e.MovRegImm32(amd64.R9, 0x6ed9eba1)
		case i < 60:
			e.MovRegReg(amd64.R9, amd64.R8)
			e.MovRegDeref32(amd64.R10, amd64.RSP, 328)
			e.MovRegDeref32(amd64.R11, amd64.RSP, 332)
			e.AndReg32Reg(amd64.R8, amd64.R10)
			e.AndReg32Reg(amd64.R9, amd64.R11)
			e.AndReg32Reg(amd64.R10, amd64.R11)
			e.XorReg32Reg(amd64.R8, amd64.R9)
			e.XorReg32Reg(amd64.R8, amd64.R10)
			e.MovRegImm32(amd64.R9, 0x8f1bbcdc)
		default:
			e.MovRegDeref32(amd64.R9, amd64.RSP, 328)
			e.XorReg32Reg(amd64.R8, amd64.R9)
			e.MovRegDeref32(amd64.R9, amd64.RSP, 332)
			e.XorReg32Reg(amd64.R8, amd64.R9)
			e.MovRegImm32(amd64.R9, 0xca62c1d6)
		}

		// temp = rol5(a) + f + e + k + W[i].
		e.MovRegDeref32(amd64.RAX, amd64.RSP, 320)
		e.RorReg32Imm8(amd64.RAX, 27)
		e.AddReg32Reg(amd64.RAX, amd64.R8)
		e.MovRegDeref32(amd64.R10, amd64.RSP, 336)
		e.AddReg32Reg(amd64.RAX, amd64.R10)
		e.AddReg32Reg(amd64.RAX, amd64.R9)
		e.MovRegDeref32(amd64.R10, amd64.RSP, int32(i*4))
		e.AddReg32Reg(amd64.RAX, amd64.R10)

		// e=d; d=c; c=rol30(b); b=a; a=temp.
		e.MovRegDeref32(amd64.R10, amd64.RSP, 332)
		e.MovDerefReg32(amd64.RSP, 336, amd64.R10)
		e.MovRegDeref32(amd64.R10, amd64.RSP, 328)
		e.MovDerefReg32(amd64.RSP, 332, amd64.R10)
		e.MovRegDeref32(amd64.R10, amd64.RSP, 324)
		e.RorReg32Imm8(amd64.R10, 2)
		e.MovDerefReg32(amd64.RSP, 328, amd64.R10)
		e.MovRegDeref32(amd64.R10, amd64.RSP, 320)
		e.MovDerefReg32(amd64.RSP, 324, amd64.R10)
		e.MovDerefReg32(amd64.RSP, 320, amd64.RAX)
	}

	for i := 0; i < 5; i++ {
		e.MovRegDeref32(amd64.RAX, amd64.R12, int32(i*4))
		e.MovRegDeref32(amd64.RCX, amd64.RSP, int32(320+i*4))
		e.AddReg32Reg(amd64.RAX, amd64.RCX)
		e.MovDerefReg32(amd64.R12, int32(i*4), amd64.RAX)
	}

	e.AddRegImm32(amd64.RSP, 352)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

// emitAMD64SHA1 hashes a ByteBuffer and returns a new 20-byte ByteBuffer.
// ABI: RDI=input ByteBuffer -> RAX=digest ByteBuffer.
func emitAMD64SHA1(e *amd64.Emitter, compressOffset, byteBufferNewOffset int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	callCompress := func(blockReg amd64.Register) {
		e.MovRegReg(amd64.RDI, blockReg)
		e.MovRegReg(amd64.RSI, amd64.RSP)
		e.AddRegImm32(amd64.RSI, 64)
		at := len(e.Code)
		e.CallRel32(int32(compressOffset - (at + 5)))
	}
	zeroFinal := func() {
		e.MovRegImm64(amd64.R10, 0)
		for off := int32(0); off < 64; off += 8 {
			e.MovDerefReg(amd64.RSP, off, amd64.R10)
		}
	}
	writeLength := func() {
		for i := 0; i < 8; i++ {
			e.MovRegReg(amd64.R10, amd64.R13)
			shift := byte((7 - i) * 8)
			if shift != 0 {
				e.ShrRegImm8(amd64.R10, shift)
			}
			e.MovDerefReg8(amd64.RSP, int32(56+i), amd64.R10)
		}
	}

	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.SubRegImm32(amd64.RSP, 128)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegDeref(amd64.R12, amd64.RBX, amd64ByteBufferLength)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64ByteBufferData)
	e.MovDerefReg(amd64.RSP, 88, amd64.R12)
	e.MovDerefReg(amd64.RSP, 96, amd64.R10)
	e.MovRegReg(amd64.R10, amd64.R12)
	e.ShrRegImm8(amd64.R10, 6)
	e.MovDerefReg(amd64.RSP, 104, amd64.R10)
	for i, h := range amd64SHA1Initial {
		e.MovRegImm32(amd64.R10, h)
		e.MovDerefReg32(amd64.RSP, int32(64+i*4), amd64.R10)
	}

	e.MovRegImm64(amd64.R14, 0)
	fullLoop := len(e.Code)
	e.MovRegDeref(amd64.R10, amd64.RSP, 104)
	e.CmpRegReg(amd64.R14, amd64.R10)
	fullDone := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	e.MovRegDeref(amd64.RDI, amd64.RSP, 96)
	e.MovRegReg(amd64.R10, amd64.R14)
	e.ShlRegImm8(amd64.R10, 6)
	e.AddRegReg(amd64.RDI, amd64.R10)
	callCompress(amd64.RDI)
	e.AddRegImm32(amd64.R14, 1)
	fullBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(fullBack, fullLoop)
	patchJcc(fullDone, len(e.Code))

	zeroFinal()
	e.MovRegDeref(amd64.R12, amd64.RSP, 88)
	e.MovRegImm64(amd64.R10, 63)
	e.AndRegReg(amd64.R12, amd64.R10)
	e.MovRegImm64(amd64.R14, 0)
	copyLoop := len(e.Code)
	e.CmpRegReg(amd64.R14, amd64.R12)
	copyDone := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	e.MovRegDeref(amd64.R11, amd64.RSP, 96)
	e.MovRegDeref(amd64.R10, amd64.RSP, 88)
	e.SubRegReg(amd64.R10, amd64.R12)
	e.AddRegReg(amd64.R11, amd64.R10)
	e.AddRegReg(amd64.R11, amd64.R14)
	e.MovzxRegDeref8(amd64.RAX, amd64.R11, 0)
	e.MovRegReg(amd64.R10, amd64.RSP)
	e.AddRegReg(amd64.R10, amd64.R14)
	e.MovDerefReg8(amd64.R10, 0, amd64.RAX)
	e.AddRegImm32(amd64.R14, 1)
	copyBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(copyBack, copyLoop)
	patchJcc(copyDone, len(e.Code))
	e.MovRegReg(amd64.R10, amd64.RSP)
	e.AddRegReg(amd64.R10, amd64.R12)
	e.MovRegImm64(amd64.R11, 0x80)
	e.MovDerefReg8(amd64.R10, 0, amd64.R11)

	e.MovRegDeref(amd64.R13, amd64.RSP, 88)
	e.ShlRegImm8(amd64.R13, 3)
	e.CmpRegImm32(amd64.R12, 55)
	twoBlocks := len(e.Code)
	e.JccRel32(amd64.CondA, 0)
	writeLength()
	callCompress(amd64.RSP)
	paddedDone := len(e.Code)
	e.JmpRel32(0)

	twoBlocksLabel := len(e.Code)
	patchJcc(twoBlocks, twoBlocksLabel)
	callCompress(amd64.RSP)
	zeroFinal()
	writeLength()
	callCompress(amd64.RSP)
	patchJmp(paddedDone, len(e.Code))

	e.MovRegImm64(amd64.R10, 20)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callNew := len(e.Code)
	e.CallRel32(int32(byteBufferNewOffset - (callNew + 5)))
	e.MovRegReg(amd64.RBX, amd64.RAX)
	e.MovRegDeref(amd64.R11, amd64.RBX, amd64ByteBufferData)
	for i := 0; i < 5; i++ {
		e.MovRegDeref32(amd64.R10, amd64.RSP, int32(64+i*4))
		e.BswapReg32(amd64.R10)
		e.MovDerefReg32(amd64.R11, int32(i*4), amd64.R10)
	}
	e.MovRegReg(amd64.RAX, amd64.RBX)
	e.AddRegImm32(amd64.RSP, 128)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}
