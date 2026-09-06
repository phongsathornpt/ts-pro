package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

func emitAMD64CopyStringBytes(e *amd64.Emitter, src, length amd64.Register) {
	// R10 is the destination cursor. Copy full qwords first, then the short tail.
	e.MovRegReg(amd64.R8, src)
	e.AddRegImm32(amd64.R8, 8)
	e.MovRegReg(amd64.R9, length)
	e.CmpRegImm32(amd64.R9, 8)
	tail := len(e.Code)
	e.JccRel32(amd64.CondL, 0)
	qwordLoop := len(e.Code)
	e.MovRegDeref(amd64.RAX, amd64.R8, 0)
	e.MovDerefReg(amd64.R10, 0, amd64.RAX)
	e.AddRegImm32(amd64.R8, 8)
	e.AddRegImm32(amd64.R10, 8)
	e.SubRegImm32(amd64.R9, 8)
	e.CmpRegImm32(amd64.R9, 8)
	qwordBack := len(e.Code)
	e.JccRel32(amd64.CondGE, 0)
	binary.LittleEndian.PutUint32(e.Code[qwordBack+2:], uint32(int32(qwordLoop-(qwordBack+6))))
	tailLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[tail+2:], uint32(int32(tailLabel-(tail+6))))
	e.TestRegReg(amd64.R9, amd64.R9)
	done := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	byteLoop := len(e.Code)
	e.MovzxRegDeref8(amd64.RAX, amd64.R8, 0)
	e.MovDerefReg8(amd64.R10, 0, amd64.RAX)
	e.AddRegImm32(amd64.R8, 1)
	e.AddRegImm32(amd64.R10, 1)
	e.SubRegImm32(amd64.R9, 1)
	byteBack := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	binary.LittleEndian.PutUint32(e.Code[byteBack+2:], uint32(int32(byteLoop-(byteBack+6))))
	doneLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[done+2:], uint32(int32(doneLabel-(done+6))))
}

func emitAMD64StringBuilderSeed(e *amd64.Emitter, allocOffset int) {
	// Clone a proven-unaliased literal into a normal string allocation with spare
	// capacity. The public string ABI stays [len][bytes]; capacity is derived from
	// the allocator object's total size in the hidden 32-byte object header.
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.SubRegImm32(amd64.RSP, 40)
	e.MovRegReg(amd64.RBX, amd64.RDI)

	// Precise root for the source across ts_alloc.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 1)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	e.MovRegDeref(amd64.R11, amd64.RBX, 0)
	e.MovDerefReg(amd64.RSP, 24, amd64.R11)
	e.MovRegReg(amd64.RDI, amd64.R11)
	e.AddRegImm32(amd64.RDI, 8)
	e.CmpRegImm32(amd64.RDI, 72) // 64 bytes of initial data capacity.
	enough := len(e.Code)
	e.JccRel32(amd64.CondGE, 0)
	e.MovRegImm64(amd64.RDI, 72)
	enoughLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[enough+2:], uint32(int32(enoughLabel-(enough+6))))
	callAt := len(e.Code)
	e.CallRel32(int32(allocOffset - (callAt + 5)))

	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.MovDerefReg(amd64.RSP, 32, amd64.RAX)
	e.MovRegDeref(amd64.R11, amd64.RSP, 24)
	e.MovDerefReg(amd64.RAX, 0, amd64.R11)
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.AddRegImm32(amd64.R10, 8)
	emitAMD64CopyStringBytes(e, amd64.RBX, amd64.R11)
	e.MovRegDeref(amd64.RAX, amd64.RSP, 32)
	e.AddRegImm32(amd64.RSP, 40)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64StringAppendOwned(e *amd64.Emitter, allocOffset int) {
	// This helper is only emitted for compiler-proven owned loop accumulators.
	// In-place growth therefore cannot mutate a string value observable through
	// another TypeScript binding.
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.Push(amd64.R15)
	e.SubRegImm32(amd64.RSP, 40)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)
	e.MovRegDeref(amd64.R13, amd64.RBX, 0)
	e.MovRegDeref(amd64.R14, amd64.R12, 0)
	e.MovRegReg(amd64.R11, amd64.R13)
	e.AddRegReg(amd64.R11, amd64.R14) // new length
	e.MovDerefReg(amd64.RSP, 32, amd64.R11)

	// Capacity = object total size - hidden header - visible length word.
	e.MovRegReg(amd64.R10, amd64.RBX)
	e.SubRegImm32(amd64.R10, amd64ObjectHeaderSize)
	e.MovRegDeref(amd64.R10, amd64.R10, amd64ObjectSize)
	e.SubRegImm32(amd64.R10, amd64ObjectHeaderSize+8)
	e.CmpRegReg(amd64.R11, amd64.R10)
	grow := len(e.Code)
	e.JccRel32(amd64.CondA, 0)

	// Fits: append the suffix directly into spare owned capacity.
	e.MovRegReg(amd64.R10, amd64.RBX)
	e.AddRegImm32(amd64.R10, 8)
	e.AddRegReg(amd64.R10, amd64.R13)
	emitAMD64CopyStringBytes(e, amd64.R12, amd64.R14)
	e.MovRegDeref(amd64.R11, amd64.RSP, 32)
	e.MovDerefReg(amd64.RBX, 0, amd64.R11)
	e.MovRegReg(amd64.RAX, amd64.RBX)
	done := len(e.Code)
	e.JmpRel32(0)

	growLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[grow+2:], uint32(int32(growLabel-(grow+6))))
	// Root both strings before allocation. Grow geometrically to make repeated
	// self-append amortized O(n) instead of copying the whole prefix each time.
	e.MovRegDeref(amd64.R11, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R11)
	e.MovRegImm64(amd64.R11, 2)
	e.MovDerefReg(amd64.RSP, 8, amd64.R11)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.RSP, 24, amd64.R12)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	e.AddRegReg(amd64.R10, amd64.R10) // doubled capacity
	e.MovRegDeref(amd64.R11, amd64.RSP, 32)
	e.CmpRegReg(amd64.R10, amd64.R11)
	capEnough := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	e.MovRegReg(amd64.R10, amd64.R11)
	capEnoughLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[capEnough+2:], uint32(int32(capEnoughLabel-(capEnough+6))))
	e.MovRegReg(amd64.RDI, amd64.R10)
	e.AddRegImm32(amd64.RDI, 8)
	callAt := len(e.Code)
	e.CallRel32(int32(allocOffset - (callAt + 5)))

	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.MovDerefReg(amd64.RSP, 24, amd64.RAX)
	e.MovRegDeref(amd64.R11, amd64.RSP, 32)
	e.MovDerefReg(amd64.RAX, 0, amd64.R11)
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.AddRegImm32(amd64.R10, 8)
	emitAMD64CopyStringBytes(e, amd64.RBX, amd64.R13)
	emitAMD64CopyStringBytes(e, amd64.R12, amd64.R14)
	e.MovRegDeref(amd64.RAX, amd64.RSP, 24)

	doneLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[done+1:], uint32(int32(doneLabel-(done+5))))
	e.AddRegImm32(amd64.RSP, 40)
	e.Pop(amd64.R15)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64StringConcatFixed(e *amd64.Emitter, allocOffset, count int) {
	// Fixed-arity concat helpers keep inputs in a precise-root frame across the
	// single result allocation. SysV argument registers cover the supported 3/4
	// operand forms without a secondary argument array.
	args := []amd64.Register{amd64.RDI, amd64.RSI, amd64.RDX, amd64.RCX}
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.SubRegImm32(amd64.RSP, 64)

	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, int64(count))
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	for i := 0; i < count; i++ {
		e.MovDerefReg(amd64.RSP, int32(16+i*8), args[i])
	}
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	e.MovRegImm64(amd64.R10, 0)
	for i := 0; i < count; i++ {
		e.MovRegDeref(amd64.R11, amd64.RSP, int32(16+i*8))
		e.MovRegDeref(amd64.R11, amd64.R11, 0)
		e.AddRegReg(amd64.R10, amd64.R11)
	}
	e.MovDerefReg(amd64.RSP, 48, amd64.R10)
	e.MovRegReg(amd64.RDI, amd64.R10)
	e.AddRegImm32(amd64.RDI, 8)
	callAt := len(e.Code)
	e.CallRel32(int32(allocOffset - (callAt + 5)))

	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.MovDerefReg(amd64.RSP, 56, amd64.RAX)
	e.MovRegDeref(amd64.R11, amd64.RSP, 48)
	e.MovDerefReg(amd64.RAX, 0, amd64.R11)
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.AddRegImm32(amd64.R10, 8)
	for i := 0; i < count; i++ {
		e.MovRegDeref(amd64.RDI, amd64.RSP, int32(16+i*8))
		e.MovRegDeref(amd64.R11, amd64.RDI, 0)
		emitAMD64CopyStringBytes(e, amd64.RDI, amd64.R11)
	}
	e.MovRegDeref(amd64.RAX, amd64.RSP, 56)
	e.AddRegImm32(amd64.RSP, 64)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64StringConcat(e *amd64.Emitter, allocOffset int) {
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.Push(amd64.R15)
	// 32-byte temporary precise-root frame plus 8 bytes of ABI padding.
	e.SubRegImm32(amd64.RSP, 40)

	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)
	e.MovRegDeref(amd64.R13, amd64.RBX, 0)
	e.MovRegDeref(amd64.R14, amd64.R12, 0)

	// Link a temporary precise-root frame for the two input strings. These are
	// live across ts_alloc, where a collection may run.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 2)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.RSP, 24, amd64.R12)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	e.MovRegReg(amd64.RDI, amd64.R13)
	e.AddRegReg(amd64.RDI, amd64.R14)
	e.AddRegImm32(amd64.RDI, 8)
	callAt := len(e.Code)
	e.CallRel32(int32(allocOffset - (callAt + 5)))

	// Collection cannot occur again in this helper. Unlink the temporary root
	// frame and reuse its second root slot to keep the result pointer.
	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.MovDerefReg(amd64.RSP, 24, amd64.RAX)

	e.MovRegReg(amd64.R11, amd64.R13)
	e.AddRegReg(amd64.R11, amd64.R14)
	e.MovRegDeref(amd64.R10, amd64.RSP, 24)
	e.MovDerefReg(amd64.R10, 0, amd64.R11)

	// dst = result + 8. Copy qwords on the common path and only byte-copy tails.
	e.AddRegImm32(amd64.R10, 8)
	emitAMD64CopyStringBytes(e, amd64.RBX, amd64.R13)
	emitAMD64CopyStringBytes(e, amd64.R12, amd64.R14)

	e.MovRegDeref(amd64.RAX, amd64.RSP, 24)
	e.AddRegImm32(amd64.RSP, 40)
	e.Pop(amd64.R15)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}
