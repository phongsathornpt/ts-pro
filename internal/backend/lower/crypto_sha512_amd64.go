package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

var amd64SHA512Initial = [...]uint64{
	0x6a09e667f3bcc908, 0xbb67ae8584caa73b, 0x3c6ef372fe94f82b, 0xa54ff53a5f1d36f1,
	0x510e527fade682d1, 0x9b05688c2b3e6c1f, 0x1f83d9abfb41bd6b, 0x5be0cd19137e2179,
}

var amd64SHA384Initial = [...]uint64{
	0xcbbb9d5dc1059ed8, 0x629a292a367cd507, 0x9159015a3070dd17, 0x152fecd8f70e5939,
	0x67332667ffc00b31, 0x8eb44a8768581511, 0xdb0c2e0d64f98fa7, 0x47b5481dbefa4fa4,
}

var amd64SHA512K = [...]uint64{
	0x428a2f98d728ae22, 0x7137449123ef65cd, 0xb5c0fbcfec4d3b2f, 0xe9b5dba58189dbbc,
	0x3956c25bf348b538, 0x59f111f1b605d019, 0x923f82a4af194f9b, 0xab1c5ed5da6d8118,
	0xd807aa98a3030242, 0x12835b0145706fbe, 0x243185be4ee4b28c, 0x550c7dc3d5ffb4e2,
	0x72be5d74f27b896f, 0x80deb1fe3b1696b1, 0x9bdc06a725c71235, 0xc19bf174cf692694,
	0xe49b69c19ef14ad2, 0xefbe4786384f25e3, 0x0fc19dc68b8cd5b5, 0x240ca1cc77ac9c65,
	0x2de92c6f592b0275, 0x4a7484aa6ea6e483, 0x5cb0a9dcbd41fbd4, 0x76f988da831153b5,
	0x983e5152ee66dfab, 0xa831c66d2db43210, 0xb00327c898fb213f, 0xbf597fc7beef0ee4,
	0xc6e00bf33da88fc2, 0xd5a79147930aa725, 0x06ca6351e003826f, 0x142929670a0e6e70,
	0x27b70a8546d22ffc, 0x2e1b21385c26c926, 0x4d2c6dfc5ac42aed, 0x53380d139d95b3df,
	0x650a73548baf63de, 0x766a0abb3c77b2a8, 0x81c2c92e47edaee6, 0x92722c851482353b,
	0xa2bfe8a14cf10364, 0xa81a664bbc423001, 0xc24b8b70d0f89791, 0xc76c51a30654be30,
	0xd192e819d6ef5218, 0xd69906245565a910, 0xf40e35855771202a, 0x106aa07032bbd1b8,
	0x19a4c116b8d2d0c8, 0x1e376c085141ab53, 0x2748774cdf8eeb99, 0x34b0bcb5e19b48a8,
	0x391c0cb3c5c95a63, 0x4ed8aa4ae3418acb, 0x5b9cca4f7763e373, 0x682e6ff3d6b2b8a3,
	0x748f82ee5defb2fc, 0x78a5636f43172f60, 0x84c87814a1f0ab72, 0x8cc702081a6439ec,
	0x90befffa23631e28, 0xa4506cebde82bde9, 0xbef9a3f7b2c67915, 0xc67178f2e372532b,
	0xca273eceea26619c, 0xd186b8c721c0c207, 0xeada7dd6cde0eb1e, 0xf57d4f7fee6ed178,
	0x06f067aa72176fba, 0x0a637dc5a2c898a6, 0x113f9804bef90dae, 0x1b710b35131c471b,
	0x28db77f523047d84, 0x32caab7b40c72493, 0x3c9ebe0a15c9bebc, 0x431d67c49c100d4c,
	0x4cc5d4becb3e42b6, 0x597f299cfc657e2a, 0x5fcb6fab3ad6faec, 0x6c44198c4a475817,
}

// emitAMD64SHA512Compress emits the SHA-512 family compression function.
// ABI: RDI=128-byte block, RSI=8-word hash state. No allocation or GC occurs.
func emitAMD64SHA512Compress(e *amd64.Emitter) {
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.SubRegImm32(amd64.RSP, 704)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)

	for i := 0; i < 16; i++ {
		e.MovRegDeref(amd64.RAX, amd64.RBX, int32(i*8))
		e.BswapReg(amd64.RAX)
		e.MovDerefReg(amd64.RSP, int32(i*8), amd64.RAX)
	}
	for i := 16; i < 80; i++ {
		e.MovRegDeref(amd64.RAX, amd64.RSP, int32((i-15)*8))
		e.MovRegReg(amd64.RCX, amd64.RAX)
		e.MovRegReg(amd64.RDX, amd64.RAX)
		e.RorRegImm8(amd64.RAX, 1)
		e.RorRegImm8(amd64.RCX, 8)
		e.ShrRegImm8(amd64.RDX, 7)
		e.XorRegReg(amd64.RAX, amd64.RCX)
		e.XorRegReg(amd64.RAX, amd64.RDX)

		e.MovRegDeref(amd64.R8, amd64.RSP, int32((i-2)*8))
		e.MovRegReg(amd64.R9, amd64.R8)
		e.MovRegReg(amd64.R10, amd64.R8)
		e.RorRegImm8(amd64.R8, 19)
		e.RorRegImm8(amd64.R9, 61)
		e.ShrRegImm8(amd64.R10, 6)
		e.XorRegReg(amd64.R8, amd64.R9)
		e.XorRegReg(amd64.R8, amd64.R10)

		e.MovRegDeref(amd64.R11, amd64.RSP, int32((i-16)*8))
		e.AddRegReg(amd64.R11, amd64.RAX)
		e.MovRegDeref(amd64.R13, amd64.RSP, int32((i-7)*8))
		e.AddRegReg(amd64.R11, amd64.R13)
		e.AddRegReg(amd64.R11, amd64.R8)
		e.MovDerefReg(amd64.RSP, int32(i*8), amd64.R11)
	}

	for i := 0; i < 8; i++ {
		e.MovRegDeref(amd64.RAX, amd64.R12, int32(i*8))
		e.MovDerefReg(amd64.RSP, int32(640+i*8), amd64.RAX)
	}
	for i, k := range amd64SHA512K {
		e.MovRegDeref(amd64.RAX, amd64.RSP, 672)
		e.MovRegReg(amd64.RCX, amd64.RAX)
		e.MovRegReg(amd64.RDX, amd64.RAX)
		e.RorRegImm8(amd64.RAX, 14)
		e.RorRegImm8(amd64.RCX, 18)
		e.RorRegImm8(amd64.RDX, 41)
		e.XorRegReg(amd64.RAX, amd64.RCX)
		e.XorRegReg(amd64.RAX, amd64.RDX)

		e.MovRegDeref(amd64.R8, amd64.RSP, 672)
		e.MovRegReg(amd64.R9, amd64.R8)
		e.MovRegDeref(amd64.R10, amd64.RSP, 680)
		e.AndRegReg(amd64.R8, amd64.R10)
		e.NotReg(amd64.R9)
		e.MovRegDeref(amd64.R10, amd64.RSP, 688)
		e.AndRegReg(amd64.R9, amd64.R10)
		e.XorRegReg(amd64.R8, amd64.R9)

		e.MovRegDeref(amd64.R11, amd64.RSP, 696)
		e.AddRegReg(amd64.R11, amd64.RAX)
		e.AddRegReg(amd64.R11, amd64.R8)
		e.MovRegImm64(amd64.R10, int64(k))
		e.AddRegReg(amd64.R11, amd64.R10)
		e.MovRegDeref(amd64.R10, amd64.RSP, int32(i*8))
		e.AddRegReg(amd64.R11, amd64.R10)

		e.MovRegDeref(amd64.RAX, amd64.RSP, 640)
		e.MovRegReg(amd64.RCX, amd64.RAX)
		e.MovRegReg(amd64.RDX, amd64.RAX)
		e.RorRegImm8(amd64.RAX, 28)
		e.RorRegImm8(amd64.RCX, 34)
		e.RorRegImm8(amd64.RDX, 39)
		e.XorRegReg(amd64.RAX, amd64.RCX)
		e.XorRegReg(amd64.RAX, amd64.RDX)

		e.MovRegDeref(amd64.R8, amd64.RSP, 640)
		e.MovRegReg(amd64.R9, amd64.R8)
		e.MovRegDeref(amd64.R10, amd64.RSP, 648)
		e.MovRegDeref(amd64.R13, amd64.RSP, 656)
		e.AndRegReg(amd64.R8, amd64.R10)
		e.AndRegReg(amd64.R9, amd64.R13)
		e.AndRegReg(amd64.R10, amd64.R13)
		e.XorRegReg(amd64.R8, amd64.R9)
		e.XorRegReg(amd64.R8, amd64.R10)
		e.AddRegReg(amd64.RAX, amd64.R8)

		e.MovRegDeref(amd64.R10, amd64.RSP, 688)
		e.MovDerefReg(amd64.RSP, 696, amd64.R10)
		e.MovRegDeref(amd64.R10, amd64.RSP, 680)
		e.MovDerefReg(amd64.RSP, 688, amd64.R10)
		e.MovRegDeref(amd64.R10, amd64.RSP, 672)
		e.MovDerefReg(amd64.RSP, 680, amd64.R10)
		e.MovRegDeref(amd64.R10, amd64.RSP, 664)
		e.AddRegReg(amd64.R10, amd64.R11)
		e.MovDerefReg(amd64.RSP, 672, amd64.R10)
		e.MovRegDeref(amd64.R10, amd64.RSP, 656)
		e.MovDerefReg(amd64.RSP, 664, amd64.R10)
		e.MovRegDeref(amd64.R10, amd64.RSP, 648)
		e.MovDerefReg(amd64.RSP, 656, amd64.R10)
		e.MovRegDeref(amd64.R10, amd64.RSP, 640)
		e.MovDerefReg(amd64.RSP, 648, amd64.R10)
		e.AddRegReg(amd64.R11, amd64.RAX)
		e.MovDerefReg(amd64.RSP, 640, amd64.R11)
	}

	for i := 0; i < 8; i++ {
		e.MovRegDeref(amd64.RAX, amd64.R12, int32(i*8))
		e.MovRegDeref(amd64.RCX, amd64.RSP, int32(640+i*8))
		e.AddRegReg(amd64.RAX, amd64.RCX)
		e.MovDerefReg(amd64.R12, int32(i*8), amd64.RAX)
	}

	e.AddRegImm32(amd64.RSP, 704)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64SHA512Variant(e *amd64.Emitter, compressOffset, byteBufferNewOffset int, initial [8]uint64, outputBytes int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	callCompress := func(blockReg amd64.Register) {
		e.MovRegReg(amd64.RDI, blockReg)
		e.MovRegReg(amd64.RSI, amd64.RSP)
		e.AddRegImm32(amd64.RSI, 128)
		at := len(e.Code)
		e.CallRel32(int32(compressOffset - (at + 5)))
	}
	zeroFinal := func() {
		e.MovRegImm64(amd64.R10, 0)
		for off := int32(0); off < 128; off += 8 {
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
			e.MovDerefReg8(amd64.RSP, int32(120+i), amd64.R10)
		}
	}

	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.SubRegImm32(amd64.RSP, 224)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegDeref(amd64.R12, amd64.RBX, amd64ByteBufferLength)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64ByteBufferData)
	e.MovDerefReg(amd64.RSP, 192, amd64.R12)
	e.MovDerefReg(amd64.RSP, 200, amd64.R10)
	e.MovRegReg(amd64.R10, amd64.R12)
	e.ShrRegImm8(amd64.R10, 7)
	e.MovDerefReg(amd64.RSP, 208, amd64.R10)
	for i, h := range initial {
		e.MovRegImm64(amd64.R10, int64(h))
		e.MovDerefReg(amd64.RSP, int32(128+i*8), amd64.R10)
	}

	e.MovRegImm64(amd64.R14, 0)
	fullLoop := len(e.Code)
	e.MovRegDeref(amd64.R10, amd64.RSP, 208)
	e.CmpRegReg(amd64.R14, amd64.R10)
	fullDone := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	e.MovRegDeref(amd64.RDI, amd64.RSP, 200)
	e.MovRegReg(amd64.R10, amd64.R14)
	e.ShlRegImm8(amd64.R10, 7)
	e.AddRegReg(amd64.RDI, amd64.R10)
	callCompress(amd64.RDI)
	e.AddRegImm32(amd64.R14, 1)
	fullBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(fullBack, fullLoop)
	patchJcc(fullDone, len(e.Code))

	zeroFinal()
	e.MovRegDeref(amd64.R12, amd64.RSP, 192)
	e.MovRegImm64(amd64.R10, 127)
	e.AndRegReg(amd64.R12, amd64.R10)
	e.MovRegImm64(amd64.R14, 0)
	copyLoop := len(e.Code)
	e.CmpRegReg(amd64.R14, amd64.R12)
	copyDone := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	e.MovRegDeref(amd64.R11, amd64.RSP, 200)
	e.MovRegDeref(amd64.R10, amd64.RSP, 192)
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

	e.MovRegDeref(amd64.R13, amd64.RSP, 192)
	e.ShlRegImm8(amd64.R13, 3)
	e.CmpRegImm32(amd64.R12, 111)
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

	e.MovRegImm64(amd64.R10, int64(outputBytes))
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callNew := len(e.Code)
	e.CallRel32(int32(byteBufferNewOffset - (callNew + 5)))
	e.MovRegReg(amd64.RBX, amd64.RAX)
	e.MovRegDeref(amd64.R11, amd64.RBX, amd64ByteBufferData)
	for i := 0; i < outputBytes/8; i++ {
		e.MovRegDeref(amd64.R10, amd64.RSP, int32(128+i*8))
		e.BswapReg(amd64.R10)
		e.MovDerefReg(amd64.R11, int32(i*8), amd64.R10)
	}
	e.MovRegReg(amd64.RAX, amd64.RBX)
	e.AddRegImm32(amd64.RSP, 224)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}
