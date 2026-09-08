package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

const (
	x25519ScalarOff = int32(0)
	x25519X1Off     = int32(32)
	x25519X2Off     = int32(160)
	x25519Z2Off     = int32(288)
	x25519X3Off     = int32(416)
	x25519Z3Off     = int32(544)
	x25519AOff      = int32(672)
	x25519AAOff     = int32(800)
	x25519BOff      = int32(928)
	x25519BBOff     = int32(1056)
	x25519EOff      = int32(1184)
	x25519COff      = int32(1312)
	x25519DOff      = int32(1440)
	x25519DAOff     = int32(1568)
	x25519CBOff     = int32(1696)
	x25519T1Off     = int32(1824)
	x25519T2Off     = int32(1952)
	x25519InvOff    = int32(2080)
	x25519SwapOff   = int32(2208)
	x25519StackSize = int32(2304)
)

func emitAMD64FEReduce(e *amd64.Emitter, base amd64.Register) {
	for pass := 0; pass < 3; pass++ {
		for i := 0; i < 15; i++ {
			e.MovRegDeref(amd64.RAX, base, int32(i*8))
			e.MovRegReg(amd64.RCX, amd64.RAX)
			e.ShrRegImm8(amd64.RCX, 16)
			e.MovRegImm64(amd64.RDX, 0xffff)
			e.AndRegReg(amd64.RAX, amd64.RDX)
			e.MovDerefReg(base, int32(i*8), amd64.RAX)
			e.MovRegDeref(amd64.R8, base, int32((i+1)*8))
			e.AddRegReg(amd64.R8, amd64.RCX)
			e.MovDerefReg(base, int32((i+1)*8), amd64.R8)
		}
		e.MovRegDeref(amd64.RAX, base, 15*8)
		e.MovRegReg(amd64.RCX, amd64.RAX)
		e.ShrRegImm8(amd64.RCX, 15)
		e.MovRegImm64(amd64.RDX, 0x7fff)
		e.AndRegReg(amd64.RAX, amd64.RDX)
		e.MovDerefReg(base, 15*8, amd64.RAX)
		e.MovRegImm64(amd64.RDX, 19)
		e.ImulRegReg(amd64.RCX, amd64.RDX)
		e.MovRegDeref(amd64.R8, base, 0)
		e.AddRegReg(amd64.R8, amd64.RCX)
		e.MovDerefReg(base, 0, amd64.R8)
	}
}
func emitAMD64FEAdd(e *amd64.Emitter) {
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)
	e.MovRegReg(amd64.R13, amd64.RDX)
	for i := 0; i < 16; i++ {
		e.MovRegDeref(amd64.RAX, amd64.R12, int32(i*8))
		e.MovRegDeref(amd64.RCX, amd64.R13, int32(i*8))
		e.AddRegReg(amd64.RAX, amd64.RCX)
		e.MovDerefReg(amd64.RBX, int32(i*8), amd64.RAX)
	}
	emitAMD64FEReduce(e, amd64.RBX)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}
func emitAMD64FESub(e *amd64.Emitter) {
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)
	e.MovRegReg(amd64.R13, amd64.RDX)
	e.MovRegImm64(amd64.R14, 0)
	for i := 0; i < 16; i++ {
		e.MovRegDeref(amd64.RAX, amd64.R12, int32(i*8))
		e.MovRegDeref(amd64.RCX, amd64.R13, int32(i*8))
		e.SubRegReg(amd64.RAX, amd64.RCX)
		e.SubRegReg(amd64.RAX, amd64.R14)
		e.MovRegReg(amd64.RDX, amd64.RAX)
		e.ShrRegImm8(amd64.RDX, 63)
		e.MovRegReg(amd64.R14, amd64.RDX)
		mask := int64(0xffff)
		if i == 15 {
			mask = 0x7fff
		}
		e.MovRegImm64(amd64.RDX, mask)
		e.AndRegReg(amd64.RAX, amd64.RDX)
		e.MovDerefReg(amd64.RBX, int32(i*8), amd64.RAX)
	}
	// A borrowed radix-2^255 subtraction already wrapped by +2^255, which is
	// +19 modulo p. Correct it by conditionally subtracting 19, propagating
	// that borrow across the mixed 16/15-bit radix without data-dependent branches.
	e.MovRegReg(amd64.RDX, amd64.R14)
	e.MovRegImm64(amd64.R10, 19)
	e.ImulRegReg(amd64.RDX, amd64.R10)
	for i := 0; i < 16; i++ {
		e.MovRegDeref(amd64.RAX, amd64.RBX, int32(i*8))
		e.SubRegReg(amd64.RAX, amd64.RDX)
		e.MovRegReg(amd64.RCX, amd64.RAX)
		e.ShrRegImm8(amd64.RCX, 63)
		e.MovRegReg(amd64.RDX, amd64.RCX)
		mask := int64(0xffff)
		if i == 15 {
			mask = 0x7fff
		}
		e.MovRegImm64(amd64.R10, mask)
		e.AndRegReg(amd64.RAX, amd64.R10)
		e.MovDerefReg(amd64.RBX, int32(i*8), amd64.RAX)
	}
	emitAMD64FEReduce(e, amd64.RBX)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}
func emitAMD64FEMul(e *amd64.Emitter) {
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.SubRegImm32(amd64.RSP, 256)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)
	e.MovRegReg(amd64.R13, amd64.RDX)
	e.MovRegImm64(amd64.R10, 0)
	for i := 0; i < 32; i++ {
		e.MovDerefReg(amd64.RSP, int32(i*8), amd64.R10)
	}
	for i := 0; i < 16; i++ {
		for j := 0; j < 16; j++ {
			e.MovRegDeref(amd64.RAX, amd64.R12, int32(i*8))
			e.MovRegDeref(amd64.R10, amd64.R13, int32(j*8))
			e.ImulRegReg(amd64.RAX, amd64.R10)
			off := int32((i + j) * 8)
			e.MovRegDeref(amd64.RCX, amd64.RSP, off)
			e.AddRegReg(amd64.RCX, amd64.RAX)
			e.MovDerefReg(amd64.RSP, off, amd64.RCX)
		}
	}
	for k := 31; k >= 16; k-- {
		e.MovRegDeref(amd64.RAX, amd64.RSP, int32(k*8))
		e.MovRegImm64(amd64.R10, 38)
		e.ImulRegReg(amd64.RAX, amd64.R10)
		lo := int32((k - 16) * 8)
		e.MovRegDeref(amd64.RCX, amd64.RSP, lo)
		e.AddRegReg(amd64.RCX, amd64.RAX)
		e.MovDerefReg(amd64.RSP, lo, amd64.RCX)
	}
	for i := 0; i < 16; i++ {
		e.MovRegDeref(amd64.RAX, amd64.RSP, int32(i*8))
		e.MovDerefReg(amd64.RBX, int32(i*8), amd64.RAX)
	}
	emitAMD64FEReduce(e, amd64.RBX)
	e.AddRegImm32(amd64.RSP, 256)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64FESquare(e *amd64.Emitter, mulOffset int) {
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.MovRegReg(amd64.RDX, amd64.RSI)
	at := len(e.Code)
	e.CallRel32(int32(mulOffset - (at + 5)))
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64FEMul121665(e *amd64.Emitter) {
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)
	for i := 0; i < 16; i++ {
		e.MovRegDeref(amd64.RAX, amd64.R12, int32(i*8))
		e.MovRegImm64(amd64.R10, 121665)
		e.ImulRegReg(amd64.RAX, amd64.R10)
		e.MovDerefReg(amd64.RBX, int32(i*8), amd64.RAX)
	}
	emitAMD64FEReduce(e, amd64.RBX)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64X25519(e *amd64.Emitter, addOffset, subOffset, mulOffset, squareOffset, mul121665Offset, byteBufferNewOffset int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	ptr := func(reg amd64.Register, off int32) {
		e.MovRegReg(reg, amd64.RSP)
		if off != 0 {
			e.AddRegImm32(reg, off)
		}
	}
	call3 := func(target int, out, a, b int32) {
		ptr(amd64.RDI, out)
		ptr(amd64.RSI, a)
		ptr(amd64.RDX, b)
		at := len(e.Code)
		e.CallRel32(int32(target - (at + 5)))
	}
	call2 := func(target int, out, a int32) {
		ptr(amd64.RDI, out)
		ptr(amd64.RSI, a)
		at := len(e.Code)
		e.CallRel32(int32(target - (at + 5)))
	}
	copyFE := func(dst, src int32) {
		for i := 0; i < 16; i++ {
			e.MovRegDeref(amd64.RAX, amd64.RSP, src+int32(i*8))
			e.MovDerefReg(amd64.RSP, dst+int32(i*8), amd64.RAX)
		}
	}
	zeroFE := func(off int32) {
		e.MovRegImm64(amd64.R10, 0)
		for i := 0; i < 16; i++ {
			e.MovDerefReg(amd64.RSP, off+int32(i*8), amd64.R10)
		}
	}
	cswap := func(a, b int32, flag amd64.Register) {
		e.MovRegReg(amd64.R11, flag)
		e.NegReg(amd64.R11)
		for i := 0; i < 16; i++ {
			e.MovRegDeref(amd64.RAX, amd64.RSP, a+int32(i*8))
			e.MovRegDeref(amd64.RCX, amd64.RSP, b+int32(i*8))
			e.MovRegReg(amd64.RDX, amd64.RAX)
			e.XorRegReg(amd64.RDX, amd64.RCX)
			e.AndRegReg(amd64.RDX, amd64.R11)
			e.XorRegReg(amd64.RAX, amd64.RDX)
			e.XorRegReg(amd64.RCX, amd64.RDX)
			e.MovDerefReg(amd64.RSP, a+int32(i*8), amd64.RAX)
			e.MovDerefReg(amd64.RSP, b+int32(i*8), amd64.RCX)
		}
	}

	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.SubRegImm32(amd64.RSP, x25519StackSize)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64ByteBufferLength)
	e.CmpRegImm32(amd64.R10, 32)
	invalidScalar := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.MovRegDeref(amd64.R10, amd64.R12, amd64ByteBufferLength)
	e.CmpRegImm32(amd64.R10, 32)
	invalidU := len(e.Code)
	e.JccRel32(amd64.CondB, 0)

	e.MovRegDeref(amd64.R13, amd64.RBX, amd64ByteBufferData)
	for i := 0; i < 32; i++ {
		e.MovzxRegDeref8(amd64.RAX, amd64.R13, int32(i))
		e.MovDerefReg8(amd64.RSP, x25519ScalarOff+int32(i), amd64.RAX)
	}
	e.MovzxRegDeref8(amd64.RAX, amd64.RSP, x25519ScalarOff)
	e.MovRegImm64(amd64.R10, 248)
	e.AndRegReg(amd64.RAX, amd64.R10)
	e.MovDerefReg8(amd64.RSP, x25519ScalarOff, amd64.RAX)
	e.MovzxRegDeref8(amd64.RAX, amd64.RSP, x25519ScalarOff+31)
	e.MovRegImm64(amd64.R10, 127)
	e.AndRegReg(amd64.RAX, amd64.R10)
	e.MovRegImm64(amd64.R10, 64)
	e.OrRegReg(amd64.RAX, amd64.R10)
	e.MovDerefReg8(amd64.RSP, x25519ScalarOff+31, amd64.RAX)

	e.MovRegDeref(amd64.R13, amd64.R12, amd64ByteBufferData)
	for i := 0; i < 16; i++ {
		e.MovzxRegDeref8(amd64.RAX, amd64.R13, int32(2*i))
		e.MovzxRegDeref8(amd64.R10, amd64.R13, int32(2*i+1))
		e.ShlRegImm8(amd64.R10, 8)
		e.OrRegReg(amd64.RAX, amd64.R10)
		if i == 15 {
			e.MovRegImm64(amd64.R10, 0x7fff)
			e.AndRegReg(amd64.RAX, amd64.R10)
		}
		e.MovDerefReg(amd64.RSP, x25519X1Off+int32(i*8), amd64.RAX)
	}
	zeroFE(x25519X2Off)
	zeroFE(x25519Z2Off)
	zeroFE(x25519X3Off)
	zeroFE(x25519Z3Off)
	e.MovRegImm64(amd64.R10, 1)
	e.MovDerefReg(amd64.RSP, x25519X2Off, amd64.R10)
	e.MovDerefReg(amd64.RSP, x25519Z3Off, amd64.R10)
	copyFE(x25519X3Off, x25519X1Off)
	e.MovRegImm64(amd64.R10, 0)
	e.MovDerefReg(amd64.RSP, x25519SwapOff, amd64.R10)

	e.MovRegImm64(amd64.R14, 254)
	ladderLoop := len(e.Code)
	e.CmpRegImm32(amd64.R14, 0)
	ladderDone := len(e.Code)
	e.JccRel32(amd64.CondL, 0)
	e.MovRegReg(amd64.R10, amd64.R14)
	e.MovRegReg(amd64.R11, amd64.R10)
	e.ShrRegImm8(amd64.R11, 3)
	e.MovRegReg(amd64.RAX, amd64.RSP)
	e.AddRegReg(amd64.RAX, amd64.R11)
	e.MovzxRegDeref8(amd64.RAX, amd64.RAX, x25519ScalarOff)
	e.MovRegReg(amd64.RCX, amd64.R10)
	e.MovRegImm64(amd64.RDX, 7)
	e.AndRegReg(amd64.RCX, amd64.RDX)
	e.ShrRegCL(amd64.RAX)
	e.MovRegImm64(amd64.RDX, 1)
	e.AndRegReg(amd64.RAX, amd64.RDX)
	e.MovRegDeref(amd64.R10, amd64.RSP, x25519SwapOff)
	e.XorRegReg(amd64.R10, amd64.RAX)
	e.MovDerefReg(amd64.RSP, x25519SwapOff, amd64.RAX)
	cswap(x25519X2Off, x25519X3Off, amd64.R10)
	cswap(x25519Z2Off, x25519Z3Off, amd64.R10)

	call3(addOffset, x25519AOff, x25519X2Off, x25519Z2Off)
	call2(squareOffset, x25519AAOff, x25519AOff)
	call3(subOffset, x25519BOff, x25519X2Off, x25519Z2Off)
	call2(squareOffset, x25519BBOff, x25519BOff)
	call3(subOffset, x25519EOff, x25519AAOff, x25519BBOff)
	call3(addOffset, x25519COff, x25519X3Off, x25519Z3Off)
	call3(subOffset, x25519DOff, x25519X3Off, x25519Z3Off)
	call3(mulOffset, x25519DAOff, x25519DOff, x25519AOff)
	call3(mulOffset, x25519CBOff, x25519COff, x25519BOff)
	call3(addOffset, x25519T1Off, x25519DAOff, x25519CBOff)
	call2(squareOffset, x25519X3Off, x25519T1Off)
	call3(subOffset, x25519T1Off, x25519DAOff, x25519CBOff)
	call2(squareOffset, x25519T2Off, x25519T1Off)
	call3(mulOffset, x25519Z3Off, x25519X1Off, x25519T2Off)
	call3(mulOffset, x25519X2Off, x25519AAOff, x25519BBOff)
	call2(mul121665Offset, x25519T1Off, x25519EOff)
	call3(addOffset, x25519T1Off, x25519AAOff, x25519T1Off)
	call3(mulOffset, x25519Z2Off, x25519EOff, x25519T1Off)
	e.SubRegImm32(amd64.R14, 1)
	ladderBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(ladderBack, ladderLoop)
	patchJcc(ladderDone, len(e.Code))
	e.MovRegDeref(amd64.R10, amd64.RSP, x25519SwapOff)
	cswap(x25519X2Off, x25519X3Off, amd64.R10)
	cswap(x25519Z2Off, x25519Z3Off, amd64.R10)

	zeroFE(x25519InvOff)
	e.MovRegImm64(amd64.R10, 1)
	e.MovDerefReg(amd64.RSP, x25519InvOff, amd64.R10)
	e.MovRegImm64(amd64.R14, 254)
	invLoop := len(e.Code)
	e.CmpRegImm32(amd64.R14, 0)
	invDone := len(e.Code)
	e.JccRel32(amd64.CondL, 0)
	call2(squareOffset, x25519T2Off, x25519InvOff)
	e.CmpRegImm32(amd64.R14, 4)
	copyOnly4 := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.CmpRegImm32(amd64.R14, 2)
	copyOnly2 := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	call3(mulOffset, x25519InvOff, x25519T2Off, x25519Z2Off)
	afterInvStep := len(e.Code)
	e.JmpRel32(0)
	copyOnly := len(e.Code)
	patchJcc(copyOnly4, copyOnly)
	patchJcc(copyOnly2, copyOnly)
	copyFE(x25519InvOff, x25519T2Off)
	patchJmp(afterInvStep, len(e.Code))
	e.SubRegImm32(amd64.R14, 1)
	invBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(invBack, invLoop)
	patchJcc(invDone, len(e.Code))
	call3(mulOffset, x25519T1Off, x25519X2Off, x25519InvOff)

	e.MovRegImm64(amd64.R14, 0)
	for i := 0; i < 16; i++ {
		e.MovRegDeref(amd64.RAX, amd64.RSP, x25519T1Off+int32(i*8))
		p := int32(0xffff)
		mask := int64(0xffff)
		if i == 0 {
			p = 0xffed
		} else if i == 15 {
			p = 0x7fff
			mask = 0x7fff
		}
		e.SubRegImm32(amd64.RAX, p)
		e.SubRegReg(amd64.RAX, amd64.R14)
		e.MovRegReg(amd64.RCX, amd64.RAX)
		e.ShrRegImm8(amd64.RCX, 63)
		e.MovRegReg(amd64.R14, amd64.RCX)
		e.MovRegImm64(amd64.RDX, mask)
		e.AndRegReg(amd64.RAX, amd64.RDX)
		e.MovDerefReg(amd64.RSP, x25519T2Off+int32(i*8), amd64.RAX)
	}
	e.MovRegReg(amd64.R8, amd64.R14)
	e.NegReg(amd64.R8)
	e.MovRegImm64(amd64.R9, -1)
	e.XorRegReg(amd64.R9, amd64.R8)
	for i := 0; i < 16; i++ {
		e.MovRegDeref(amd64.RAX, amd64.RSP, x25519T1Off+int32(i*8))
		e.MovRegDeref(amd64.RCX, amd64.RSP, x25519T2Off+int32(i*8))
		e.AndRegReg(amd64.RAX, amd64.R8)
		e.AndRegReg(amd64.RCX, amd64.R9)
		e.OrRegReg(amd64.RAX, amd64.RCX)
		e.MovDerefReg(amd64.RSP, x25519T1Off+int32(i*8), amd64.RAX)
	}

	e.MovRegImm64(amd64.R10, 32)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callNew := len(e.Code)
	e.CallRel32(int32(byteBufferNewOffset - (callNew + 5)))
	e.MovRegReg(amd64.RBX, amd64.RAX)
	e.MovRegDeref(amd64.R12, amd64.RBX, amd64ByteBufferData)
	for i := 0; i < 16; i++ {
		e.MovRegDeref(amd64.RAX, amd64.RSP, x25519T1Off+int32(i*8))
		e.MovDerefReg8(amd64.R12, int32(i*2), amd64.RAX)
		e.ShrRegImm8(amd64.RAX, 8)
		e.MovDerefReg8(amd64.R12, int32(i*2+1), amd64.RAX)
	}
	e.MovRegReg(amd64.RAX, amd64.RBX)
	successReturn := len(e.Code)
	e.JmpRel32(0)

	invalid := len(e.Code)
	patchJcc(invalidScalar, invalid)
	patchJcc(invalidU, invalid)
	e.MovRegImm64(amd64.R10, 0)
	e.Cvtsi2sd(amd64.XMM0, amd64.R10)
	callEmpty := len(e.Code)
	e.CallRel32(int32(byteBufferNewOffset - (callEmpty + 5)))
	patchJmp(successReturn, len(e.Code))
	e.AddRegImm32(amd64.RSP, x25519StackSize)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}
