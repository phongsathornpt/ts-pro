package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

func emitAMD64FreeInsert(e *amd64.Emitter) {
	patchJcc := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6))))
	}
	prepend := func(offset int32) {
		e.MovRegDeref(amd64.R10, amd64.R15, offset)
		e.MovDerefReg(amd64.RDI, amd64ObjectNextFree, amd64.R10)
		e.MovDerefReg(amd64.R15, offset, amd64.RDI)
		e.Ret()
	}

	e.MovRegDeref(amd64.RAX, amd64.RDI, amd64ObjectSize)
	e.CmpRegImm32(amd64.RAX, 128)
	to128 := len(e.Code)
	e.JccRel32(amd64.CondBE, 0)
	e.CmpRegImm32(amd64.RAX, 512)
	to512 := len(e.Code)
	e.JccRel32(amd64.CondBE, 0)
	e.CmpRegImm32(amd64.RAX, 2048)
	to2048 := len(e.Code)
	e.JccRel32(amd64.CondBE, 0)
	e.CmpRegImm32(amd64.RAX, 8192)
	to8192 := len(e.Code)
	e.JccRel32(amd64.CondBE, 0)
	prepend(amd64RTFreeList)

	bin128 := len(e.Code)
	patchJcc(to128, bin128)
	prepend(amd64RTFree128)
	bin512 := len(e.Code)
	patchJcc(to512, bin512)
	prepend(amd64RTFree512)
	bin2048 := len(e.Code)
	patchJcc(to2048, bin2048)
	prepend(amd64RTFree2048)
	bin8192 := len(e.Code)
	patchJcc(to8192, bin8192)
	prepend(amd64RTFree8192)
}

func emitAMD64FreeTake(e *amd64.Emitter, freeInsertOffset, setAllocationStartOffset int) {
	patchJcc := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6))))
	}
	patchJmp := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5))))
	}
	emitReturn := func() {
		e.Pop(amd64.R14)
		e.Pop(amd64.R13)
		e.Pop(amd64.R12)
		e.Pop(amd64.RBX)
		e.Pop(amd64.RBP)
		e.Ret()
	}

	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.MovRegReg(amd64.RBX, amd64.RDI) // requested aligned total size

	type freeClass struct {
		limit  int32
		offset int32
	}
	classes := []freeClass{
		{128, amd64RTFree128},
		{512, amd64RTFree512},
		{2048, amd64RTFree2048},
		{8192, amd64RTFree8192},
		{0, amd64RTFreeList},
	}
	for _, class := range classes {
		skipClass := -1
		if class.limit != 0 {
			e.CmpRegImm32(amd64.RBX, class.limit)
			skipClass = len(e.Code)
			e.JccRel32(amd64.CondA, 0)
		}
		e.MovRegImm64(amd64.R12, 0) // previous header
		e.MovRegDeref(amd64.R13, amd64.R15, class.offset)
		loop := len(e.Code)
		e.TestRegReg(amd64.R13, amd64.R13)
		miss := len(e.Code)
		e.JccRel32(amd64.CondE, 0)
		e.MovRegDeref(amd64.R10, amd64.R13, amd64ObjectSize)
		e.CmpRegReg(amd64.R10, amd64.RBX)
		found := len(e.Code)
		e.JccRel32(amd64.CondAE, 0)
		e.MovRegReg(amd64.R12, amd64.R13)
		e.MovRegDeref(amd64.R13, amd64.R13, amd64ObjectNextFree)
		back := len(e.Code)
		e.JmpRel32(0)
		patchJmp(back, loop)

		foundLabel := len(e.Code)
		patchJcc(found, foundLabel)
		e.MovRegDeref(amd64.R14, amd64.R13, amd64ObjectNextFree)
		// Unlink selected block from this class list before potentially
		// reclassifying a split remainder.
		e.TestRegReg(amd64.R12, amd64.R12)
		hasPrev := len(e.Code)
		e.JccRel32(amd64.CondNE, 0)
		e.MovDerefReg(amd64.R15, class.offset, amd64.R14)
		unlinked := len(e.Code)
		e.JmpRel32(0)
		hasPrevLabel := len(e.Code)
		patchJcc(hasPrev, hasPrevLabel)
		e.MovDerefReg(amd64.R12, amd64ObjectNextFree, amd64.R14)
		unlinkDone := len(e.Code)
		patchJmp(unlinked, unlinkDone)

		// Split a useful tail and return it to the appropriate size class.
		e.MovRegDeref(amd64.R10, amd64.R13, amd64ObjectSize)
		e.MovRegReg(amd64.R11, amd64.R10)
		e.SubRegReg(amd64.R11, amd64.RBX)
		e.CmpRegImm32(amd64.R11, 48)
		noSplit := len(e.Code)
		e.JccRel32(amd64.CondL, 0)
		e.MovRegReg(amd64.RAX, amd64.R13)
		e.AddRegReg(amd64.RAX, amd64.RBX)
		e.MovDerefReg(amd64.RAX, amd64ObjectSize, amd64.R11)
		e.MovRegImm64(amd64.R10, 2)
		e.MovDerefReg(amd64.RAX, amd64ObjectFlags, amd64.R10)
		e.MovRegImm64(amd64.R10, 0)
		e.MovDerefReg(amd64.RAX, amd64ObjectNextFree, amd64.R10)
		e.MovDerefReg(amd64.RAX, amd64ObjectType, amd64.R10)
		e.MovDerefReg(amd64.R13, amd64ObjectSize, amd64.RBX)
		e.MovRegReg(amd64.RDI, amd64.RAX)
		setStartCall := len(e.Code)
		e.CallRel32(int32(setAllocationStartOffset - (setStartCall + 5)))
		callInsert := len(e.Code)
		e.CallRel32(int32(freeInsertOffset - (callInsert + 5)))
		noSplitLabel := len(e.Code)
		patchJcc(noSplit, noSplitLabel)

		// Reset metadata and scrub the entire physical payload visible to GC.
		e.MovRegImm64(amd64.R10, 0)
		e.MovDerefReg(amd64.R13, amd64ObjectFlags, amd64.R10)
		e.MovDerefReg(amd64.R13, amd64ObjectNextFree, amd64.R10)
		e.MovDerefReg(amd64.R13, amd64ObjectType, amd64.R10)
		e.MovRegDeref(amd64.R11, amd64.R13, amd64ObjectSize)
		e.SubRegImm32(amd64.R11, amd64ObjectHeaderSize)
		e.ShrRegImm8(amd64.R11, 3)
		e.MovRegReg(amd64.RAX, amd64.R13)
		e.AddRegImm32(amd64.RAX, amd64ObjectHeaderSize)
		e.TestRegReg(amd64.R11, amd64.R11)
		scrubDone := len(e.Code)
		e.JccRel32(amd64.CondE, 0)
		scrubLoop := len(e.Code)
		e.MovDerefReg(amd64.RAX, 0, amd64.R10)
		e.AddRegImm32(amd64.RAX, 8)
		e.SubRegImm32(amd64.R11, 1)
		scrubBack := len(e.Code)
		e.JccRel32(amd64.CondNE, 0)
		patchJcc(scrubBack, scrubLoop)
		scrubDoneLabel := len(e.Code)
		patchJcc(scrubDone, scrubDoneLabel)
		e.MovRegReg(amd64.RAX, amd64.R13)
		e.AddRegImm32(amd64.RAX, amd64ObjectHeaderSize)
		e.MovRegImm64(amd64.RDX, 1)
		emitReturn()

		nextClass := len(e.Code)
		patchJcc(miss, nextClass)
		if skipClass >= 0 {
			patchJcc(skipClass, nextClass)
		}
	}

	e.MovRegImm64(amd64.RAX, 0)
	e.MovRegImm64(amd64.RDX, 0)
	emitReturn()
}

func emitAMD64Alloc(e *amd64.Emitter, gcOffset, freeTakeOffset int) {
	patchJcc := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6))))
	}
	emitReturn := func() {
		e.Pop(amd64.R14)
		e.Pop(amd64.R13)
		e.Pop(amd64.R12)
		e.Pop(amd64.RBX)
		e.Pop(amd64.RBP)
		e.Ret()
	}

	// Preserve callee-saved temporaries and keep call sites 16-byte aligned.
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)

	// RBX is the aligned total object size, including its 32-byte header.
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.AddRegImm32(amd64.RBX, amd64ObjectHeaderSize+15)
	e.MovRegImm64(amd64.R11, -16)
	e.AndRegReg(amd64.RBX, amd64.R11)

	// Keep the common allocation path O(1): consume fresh bump space before
	// consulting the reclaimed-block list. Fragment reuse is a pressure path.
	e.MovRegDeref(amd64.RAX, amd64.R15, amd64RTCursor)
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.AddRegReg(amd64.R10, amd64.RBX)
	e.MovRegDeref(amd64.R11, amd64.R15, amd64RTEnd)
	e.CmpRegReg(amd64.R10, amd64.R11)
	collect := len(e.Code)
	e.JccRel32(amd64.CondA, 0)
	emitAMD64InitObjectHeader(e, amd64.RAX, amd64.RBX)
	e.MovDerefReg(amd64.R15, amd64RTCursor, amd64.R10)
	e.MovRegDeref(amd64.R11, amd64.R15, amd64RTChunkHead)
	e.MovDerefReg(amd64.R11, amd64ChunkUsed, amd64.R10)
	emitAMD64ChunkBitIndex(e, amd64.R9, amd64.R11, amd64.RAX)
	e.BtsDerefReg(amd64.R11, amd64ChunkAllocBitmap, amd64.R9)
	e.AddRegImm32(amd64.RAX, amd64ObjectHeaderSize)
	e.MovRegImm64(amd64.RDX, 0) // fresh bump memory
	emitReturn()

	// On bump-space pressure, reuse a reclaimed block before paying for GC.
	collectLabel := len(e.Code)
	patchJcc(collect, collectLabel)
	e.MovRegReg(amd64.RDI, amd64.RBX)
	freeCall := len(e.Code)
	e.CallRel32(int32(freeTakeOffset - (freeCall + 5)))
	e.TestRegReg(amd64.RAX, amd64.RAX)
	freeMiss := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	emitReturn()

	// No reusable block fits, so collect before mapping another chunk.
	gcLabel := len(e.Code)
	patchJcc(freeMiss, gcLabel)
	callAt := len(e.Code)
	e.CallRel32(int32(gcOffset - (callAt + 5)))
	e.MovRegReg(amd64.RDI, amd64.RBX)
	freeAfterGCCall := len(e.Code)
	e.CallRel32(int32(freeTakeOffset - (freeAfterGCCall + 5)))
	e.TestRegReg(amd64.RAX, amd64.RAX)
	mapMiss := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	emitReturn()
	mapMissLabel := len(e.Code)
	patchJcc(mapMiss, mapMissLabel)

	// No reusable block fits. Refill with max(1 MiB, object + chunk header).
	e.MovRegReg(amd64.RSI, amd64.RBX)
	e.AddRegImm32(amd64.RSI, amd64ChunkSize)
	e.CmpRegImm32(amd64.RSI, 1<<20)
	large := len(e.Code)
	e.JccRel32(amd64.CondGE, 0)
	e.MovRegImm64(amd64.RSI, 1<<20)
	mapChunk := len(e.Code)
	patchJcc(large, mapChunk)
	e.MovRegImm64(amd64.RDI, 0)
	e.MovRegImm64(amd64.RDX, 3)
	e.MovRegImm64(amd64.R10, 0x22)
	e.MovRegImm64(amd64.R8, -1)
	e.MovRegImm64(amd64.R9, 0)
	e.MovRegImm64(amd64.RAX, 9)
	e.Syscall()

	// Link and initialize the new chunk.
	e.MovRegDeref(amd64.R11, amd64.R15, amd64RTChunkHead)
	e.MovDerefReg(amd64.RAX, amd64ChunkNext, amd64.R11)
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.AddRegReg(amd64.R10, amd64.RSI)
	e.MovDerefReg(amd64.RAX, amd64ChunkEnd, amd64.R10)
	e.MovRegReg(amd64.R11, amd64.RAX)
	e.AddRegImm32(amd64.R11, amd64ChunkSize)
	e.MovRegReg(amd64.R10, amd64.R11)
	e.AddRegReg(amd64.R10, amd64.RBX)
	e.MovDerefReg(amd64.RAX, amd64ChunkUsed, amd64.R10)
	e.MovDerefReg(amd64.R15, amd64RTChunkHead, amd64.RAX)
	e.MovDerefReg(amd64.R15, amd64RTCursor, amd64.R10)
	e.MovRegDeref(amd64.R10, amd64.RAX, amd64ChunkEnd)
	e.MovDerefReg(amd64.R15, amd64RTEnd, amd64.R10)
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTMappedBytes)
	e.AddRegReg(amd64.R10, amd64.RSI)
	e.MovDerefReg(amd64.R15, amd64RTMappedBytes, amd64.R10)

	// First object in the new chunk. Reserve its allocation-start bit before
	// header initialization clobbers temporary registers.
	emitAMD64ChunkBitIndex(e, amd64.R9, amd64.RAX, amd64.R11)
	e.BtsDerefReg(amd64.RAX, amd64ChunkAllocBitmap, amd64.R9)
	e.MovRegReg(amd64.RAX, amd64.R11)
	emitAMD64InitObjectHeader(e, amd64.RAX, amd64.RBX)
	e.AddRegImm32(amd64.RAX, amd64ObjectHeaderSize)
	e.MovRegImm64(amd64.RDX, 0) // fresh mmap memory
	emitReturn()
}

func emitAMD64InitObjectHeader(e *amd64.Emitter, header, total amd64.Register) {
	e.MovDerefReg(header, amd64ObjectSize, total)
	e.MovRegImm64(amd64.R11, 0)
	e.MovDerefReg(header, amd64ObjectFlags, amd64.R11)
	e.MovDerefReg(header, amd64ObjectNextFree, amd64.R11)
	e.MovDerefReg(header, amd64ObjectType, amd64.R11)
}
