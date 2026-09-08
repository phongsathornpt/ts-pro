package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

var amd64SHA256Initial = [...]uint32{
	0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a,
	0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19,
}

var amd64SHA256K = [...]uint32{
	0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
	0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
	0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
	0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
	0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
	0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
	0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
	0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
}

// emitAMD64SHA256Compress emits the SHA-256 compression function.
// ABI: RDI=64-byte block, RSI=8-word hash state. No allocation or GC occurs.
func emitAMD64SHA256Compress(e *amd64.Emitter) {
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.SubRegImm32(amd64.RSP, 320)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)

	// W[0..15] is the big-endian input block converted to host-order words.
	for i := 0; i < 16; i++ {
		e.MovRegDeref32(amd64.RAX, amd64.RBX, int32(i*4))
		e.BswapReg32(amd64.RAX)
		e.MovDerefReg32(amd64.RSP, int32(i*4), amd64.RAX)
	}
	// Expand W[16..63]. All 32-bit arithmetic wraps modulo 2^32.
	for i := 16; i < 64; i++ {
		e.MovRegDeref32(amd64.RAX, amd64.RSP, int32((i-15)*4))
		e.MovRegReg(amd64.RCX, amd64.RAX)
		e.MovRegReg(amd64.RDX, amd64.RAX)
		e.RorReg32Imm8(amd64.RAX, 7)
		e.RorReg32Imm8(amd64.RCX, 18)
		e.ShrReg32Imm8(amd64.RDX, 3)
		e.XorReg32Reg(amd64.RAX, amd64.RCX)
		e.XorReg32Reg(amd64.RAX, amd64.RDX) // s0

		e.MovRegDeref32(amd64.R8, amd64.RSP, int32((i-2)*4))
		e.MovRegReg(amd64.R9, amd64.R8)
		e.MovRegReg(amd64.R10, amd64.R8)
		e.RorReg32Imm8(amd64.R8, 17)
		e.RorReg32Imm8(amd64.R9, 19)
		e.ShrReg32Imm8(amd64.R10, 10)
		e.XorReg32Reg(amd64.R8, amd64.R9)
		e.XorReg32Reg(amd64.R8, amd64.R10) // s1

		e.MovRegDeref32(amd64.R11, amd64.RSP, int32((i-16)*4))
		e.AddReg32Reg(amd64.R11, amd64.RAX)
		e.MovRegDeref32(amd64.R13, amd64.RSP, int32((i-7)*4))
		e.AddReg32Reg(amd64.R11, amd64.R13)
		e.AddReg32Reg(amd64.R11, amd64.R8)
		e.MovDerefReg32(amd64.RSP, int32(i*4), amd64.R11)
	}

	// Working variables a..h occupy stack offsets 256..284.
	for i := 0; i < 8; i++ {
		e.MovRegDeref32(amd64.RAX, amd64.R12, int32(i*4))
		e.MovDerefReg32(amd64.RSP, int32(256+i*4), amd64.RAX)
	}
	for i, k := range amd64SHA256K {
		// Σ1(e)
		e.MovRegDeref32(amd64.RAX, amd64.RSP, 272)
		e.MovRegReg(amd64.RCX, amd64.RAX)
		e.MovRegReg(amd64.RDX, amd64.RAX)
		e.RorReg32Imm8(amd64.RAX, 6)
		e.RorReg32Imm8(amd64.RCX, 11)
		e.RorReg32Imm8(amd64.RDX, 25)
		e.XorReg32Reg(amd64.RAX, amd64.RCX)
		e.XorReg32Reg(amd64.RAX, amd64.RDX)

		// Ch(e,f,g) = (e & f) ^ (~e & g)
		e.MovRegDeref32(amd64.R8, amd64.RSP, 272)
		e.MovRegReg(amd64.R9, amd64.R8)
		e.MovRegDeref32(amd64.R10, amd64.RSP, 276)
		e.AndReg32Reg(amd64.R8, amd64.R10)
		e.NotReg32(amd64.R9)
		e.MovRegDeref32(amd64.R10, amd64.RSP, 280)
		e.AndReg32Reg(amd64.R9, amd64.R10)
		e.XorReg32Reg(amd64.R8, amd64.R9)

		// t1 = h + Σ1 + Ch + K[i] + W[i]
		e.MovRegDeref32(amd64.R11, amd64.RSP, 284)
		e.AddReg32Reg(amd64.R11, amd64.RAX)
		e.AddReg32Reg(amd64.R11, amd64.R8)
		e.MovRegImm32(amd64.R10, k)
		e.AddReg32Reg(amd64.R11, amd64.R10)
		e.MovRegDeref32(amd64.R10, amd64.RSP, int32(i*4))
		e.AddReg32Reg(amd64.R11, amd64.R10)

		// Σ0(a)
		e.MovRegDeref32(amd64.RAX, amd64.RSP, 256)
		e.MovRegReg(amd64.RCX, amd64.RAX)
		e.MovRegReg(amd64.RDX, amd64.RAX)
		e.RorReg32Imm8(amd64.RAX, 2)
		e.RorReg32Imm8(amd64.RCX, 13)
		e.RorReg32Imm8(amd64.RDX, 22)
		e.XorReg32Reg(amd64.RAX, amd64.RCX)
		e.XorReg32Reg(amd64.RAX, amd64.RDX)

		// Maj(a,b,c) = (a&b) ^ (a&c) ^ (b&c)
		e.MovRegDeref32(amd64.R8, amd64.RSP, 256)
		e.MovRegReg(amd64.R9, amd64.R8)
		e.MovRegDeref32(amd64.R10, amd64.RSP, 260)
		e.MovRegDeref32(amd64.R13, amd64.RSP, 264)
		e.AndReg32Reg(amd64.R8, amd64.R10)
		e.AndReg32Reg(amd64.R9, amd64.R13)
		e.AndReg32Reg(amd64.R10, amd64.R13)
		e.XorReg32Reg(amd64.R8, amd64.R9)
		e.XorReg32Reg(amd64.R8, amd64.R10)
		e.AddReg32Reg(amd64.RAX, amd64.R8) // t2

		// h=g; g=f; f=e; e=d+t1; d=c; c=b; b=a; a=t1+t2.
		e.MovRegDeref32(amd64.R10, amd64.RSP, 280)
		e.MovDerefReg32(amd64.RSP, 284, amd64.R10)
		e.MovRegDeref32(amd64.R10, amd64.RSP, 276)
		e.MovDerefReg32(amd64.RSP, 280, amd64.R10)
		e.MovRegDeref32(amd64.R10, amd64.RSP, 272)
		e.MovDerefReg32(amd64.RSP, 276, amd64.R10)
		e.MovRegDeref32(amd64.R10, amd64.RSP, 268)
		e.AddReg32Reg(amd64.R10, amd64.R11)
		e.MovDerefReg32(amd64.RSP, 272, amd64.R10)
		e.MovRegDeref32(amd64.R10, amd64.RSP, 264)
		e.MovDerefReg32(amd64.RSP, 268, amd64.R10)
		e.MovRegDeref32(amd64.R10, amd64.RSP, 260)
		e.MovDerefReg32(amd64.RSP, 264, amd64.R10)
		e.MovRegDeref32(amd64.R10, amd64.RSP, 256)
		e.MovDerefReg32(amd64.RSP, 260, amd64.R10)
		e.AddReg32Reg(amd64.R11, amd64.RAX)
		e.MovDerefReg32(amd64.RSP, 256, amd64.R11)
	}

	for i := 0; i < 8; i++ {
		e.MovRegDeref32(amd64.RAX, amd64.R12, int32(i*4))
		e.MovRegDeref32(amd64.RCX, amd64.RSP, int32(256+i*4))
		e.AddReg32Reg(amd64.RAX, amd64.RCX)
		e.MovDerefReg32(amd64.R12, int32(i*4), amd64.RAX)
	}

	e.AddRegImm32(amd64.RSP, 320)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

// emitAMD64SHA256 hashes a ByteBuffer and returns a new 32-byte ByteBuffer.
// ABI: RDI=input ByteBuffer -> RAX=digest ByteBuffer.
func emitAMD64SHA256(e *amd64.Emitter, compressOffset, byteBufferNewOffset int) {
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
	e.MovDerefReg(amd64.RSP, 96, amd64.R12)  // input length
	e.MovDerefReg(amd64.RSP, 104, amd64.R10) // input data
	e.MovRegReg(amd64.R10, amd64.R12)
	e.ShrRegImm8(amd64.R10, 6)
	e.MovDerefReg(amd64.RSP, 112, amd64.R10) // complete 64-byte blocks
	for i, h := range amd64SHA256Initial {
		e.MovRegImm32(amd64.R10, h)
		e.MovDerefReg32(amd64.RSP, int32(64+i*4), amd64.R10)
	}

	// Compress complete input blocks directly from the backing store.
	e.MovRegImm64(amd64.R14, 0)
	fullLoop := len(e.Code)
	e.MovRegDeref(amd64.R10, amd64.RSP, 112)
	e.CmpRegReg(amd64.R14, amd64.R10)
	fullDone := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	e.MovRegDeref(amd64.RDI, amd64.RSP, 104)
	e.MovRegReg(amd64.R10, amd64.R14)
	e.ShlRegImm8(amd64.R10, 6)
	e.AddRegReg(amd64.RDI, amd64.R10)
	callCompress(amd64.RDI)
	e.AddRegImm32(amd64.R14, 1)
	fullBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(fullBack, fullLoop)
	patchJcc(fullDone, len(e.Code))

	// Build the final padded block from the remaining bytes.
	zeroFinal()
	e.MovRegDeref(amd64.R12, amd64.RSP, 96)
	e.MovRegImm64(amd64.R10, 63)
	e.AndRegReg(amd64.R12, amd64.R10) // remainder
	e.MovRegImm64(amd64.R14, 0)
	copyLoop := len(e.Code)
	e.CmpRegReg(amd64.R14, amd64.R12)
	copyDone := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	e.MovRegDeref(amd64.R11, amd64.RSP, 104)
	e.MovRegDeref(amd64.R10, amd64.RSP, 96)
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

	// R13 holds the 64-bit message length in bits for the padding trailer.
	e.MovRegDeref(amd64.R13, amd64.RSP, 96)
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

	// Allocate and serialize the digest in network byte order.
	e.MovRegImm64(amd64.R10, 32)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callNew := len(e.Code)
	e.CallRel32(int32(byteBufferNewOffset - (callNew + 5)))
	e.MovRegReg(amd64.RBX, amd64.RAX)
	e.MovRegDeref(amd64.R11, amd64.RBX, amd64ByteBufferData)
	for i := 0; i < 8; i++ {
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
