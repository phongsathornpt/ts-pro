package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

func emitAMD64GCCollect(e *amd64.Emitter) {
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

	// collectionCount++
	e.MovRegDeref(amd64.RAX, amd64.R15, amd64RTCollections)
	e.AddRegImm32(amd64.RAX, 1)
	e.MovDerefReg(amd64.R15, amd64RTCollections, amd64.RAX)

	// Mark exact string roots from the linked shadow-root frames.
	e.MovRegDeref(amd64.R12, amd64.R15, amd64RTRootHead)
	frameLoop := len(e.Code)
	e.TestRegReg(amd64.R12, amd64.R12)
	markDoneJump := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.R13, amd64.R12, 8) // root count
	e.MovRegReg(amd64.R14, amd64.R12)
	e.AddRegImm32(amd64.R14, 16) // first root slot

	rootLoop := len(e.Code)
	e.TestRegReg(amd64.R13, amd64.R13)
	frameNextJump := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.RAX, amd64.R14, 0)
	e.TestRegReg(amd64.RAX, amd64.RAX)
	rootNextIfNil := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.SubRegImm32(amd64.R10, amd64ObjectHeaderSize)
	e.MovRegDeref(amd64.R11, amd64.R15, amd64RTChunkHead)

	chunkFindLoop := len(e.Code)
	e.TestRegReg(amd64.R11, amd64.R11)
	rootNextIfNoChunk := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegReg(amd64.RBX, amd64.R11)
	e.AddRegImm32(amd64.RBX, amd64ChunkSize)
	e.CmpRegReg(amd64.R10, amd64.RBX)
	chunkNextIfBelow := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.MovRegDeref(amd64.RBX, amd64.R11, amd64ChunkUsed)
	e.CmpRegReg(amd64.R10, amd64.RBX)
	chunkNextIfAbove := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)

	// Exact heap payload found. Mark its header.
	e.MovRegImm64(amd64.RBX, 1)
	e.MovDerefReg(amd64.R10, amd64ObjectFlags, amd64.RBX)
	rootMarkedJump := len(e.Code)
	e.JmpRel32(0)

	chunkNext := len(e.Code)
	patchJcc(chunkNextIfBelow, chunkNext)
	patchJcc(chunkNextIfAbove, chunkNext)
	e.MovRegDeref(amd64.R11, amd64.R11, amd64ChunkNext)
	chunkFindBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(chunkFindBack, chunkFindLoop)

	rootNext := len(e.Code)
	patchJcc(rootNextIfNil, rootNext)
	patchJcc(rootNextIfNoChunk, rootNext)
	patchJmp(rootMarkedJump, rootNext)
	e.AddRegImm32(amd64.R14, 8)
	e.SubRegImm32(amd64.R13, 1)
	rootBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(rootBack, rootLoop)

	frameNext := len(e.Code)
	patchJcc(frameNextJump, frameNext)
	e.MovRegDeref(amd64.R12, amd64.R12, 0)
	frameBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(frameBack, frameLoop)

	markDone := len(e.Code)
	patchJcc(markDoneJump, markDone)

	// Sweep every object in every chunk. R12 is the rebuilt free-list head;
	// R14 accumulates bytes newly reclaimed by this collection.
	e.MovRegImm64(amd64.R12, 0)
	e.MovRegImm64(amd64.R14, 0)
	e.MovRegDeref(amd64.R13, amd64.R15, amd64RTChunkHead)

	sweepChunkLoop := len(e.Code)
	e.TestRegReg(amd64.R13, amd64.R13)
	sweepDoneJump := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegReg(amd64.R10, amd64.R13)
	e.AddRegImm32(amd64.R10, amd64ChunkSize)
	e.MovRegDeref(amd64.R11, amd64.R13, amd64ChunkUsed)

	sweepObjectLoop := len(e.Code)
	e.CmpRegReg(amd64.R10, amd64.R11)
	nextChunkJump := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	e.MovRegDeref(amd64.RBX, amd64.R10, amd64ObjectSize)
	e.MovRegDeref(amd64.RAX, amd64.R10, amd64ObjectFlags)
	e.CmpRegImm32(amd64.RAX, 1)
	liveJump := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.CmpRegImm32(amd64.RAX, 2)
	alreadyFreeJump := len(e.Code)
	e.JccRel32(amd64.CondE, 0)

	// Newly unreachable object.
	e.MovRegImm64(amd64.RAX, 2)
	e.MovDerefReg(amd64.R10, amd64ObjectFlags, amd64.RAX)
	e.MovDerefReg(amd64.R10, amd64ObjectNextFree, amd64.R12)
	e.MovRegReg(amd64.R12, amd64.R10)
	e.AddRegReg(amd64.R14, amd64.RBX)
	objectNextJump := len(e.Code)
	e.JmpRel32(0)

	alreadyFree := len(e.Code)
	patchJcc(alreadyFreeJump, alreadyFree)
	e.MovDerefReg(amd64.R10, amd64ObjectNextFree, amd64.R12)
	e.MovRegReg(amd64.R12, amd64.R10)
	freeNextJump := len(e.Code)
	e.JmpRel32(0)

	live := len(e.Code)
	patchJcc(liveJump, live)
	e.MovRegImm64(amd64.RAX, 0)
	e.MovDerefReg(amd64.R10, amd64ObjectFlags, amd64.RAX)
	e.MovDerefReg(amd64.R10, amd64ObjectNextFree, amd64.RAX)

	objectNext := len(e.Code)
	patchJmp(objectNextJump, objectNext)
	patchJmp(freeNextJump, objectNext)
	e.AddRegReg(amd64.R10, amd64.RBX)
	objectBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(objectBack, sweepObjectLoop)

	nextChunk := len(e.Code)
	patchJcc(nextChunkJump, nextChunk)
	e.MovRegDeref(amd64.R13, amd64.R13, amd64ChunkNext)
	chunkBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(chunkBack, sweepChunkLoop)

	sweepDone := len(e.Code)
	patchJcc(sweepDoneJump, sweepDone)
	e.MovDerefReg(amd64.R15, amd64RTFreeList, amd64.R12)
	e.MovRegDeref(amd64.RAX, amd64.R15, amd64RTReclaimed)
	e.AddRegReg(amd64.RAX, amd64.R14)
	e.MovDerefReg(amd64.R15, amd64RTReclaimed, amd64.RAX)

	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64GCMetricNumber(e *amd64.Emitter, contextOffset int32) {
	e.MovRegDeref(amd64.RAX, amd64.R15, contextOffset)
	e.Cvtsi2sd(amd64.XMM0, amd64.RAX)
	e.Ret()
}
