package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

func emitAMD64ChunkBitIndex(e *amd64.Emitter, dst, chunk, header amd64.Register) {
	e.MovRegReg(dst, header)
	e.SubRegReg(dst, chunk)
	e.SubRegImm32(dst, amd64ChunkSize)
	e.ShrRegImm8(dst, 4)
}

func emitAMD64GCSetAllocationStart(e *amd64.Emitter) {
	patchJcc := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6))))
	}
	patchJmp := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5))))
	}

	e.MovRegReg(amd64.R10, amd64.RDI)
	e.MovRegDeref(amd64.R11, amd64.R15, amd64RTChunkHead)
	loop := len(e.Code)
	e.TestRegReg(amd64.R11, amd64.R11)
	done := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegReg(amd64.RAX, amd64.R11)
	e.AddRegImm32(amd64.RAX, amd64ChunkSize)
	e.CmpRegReg(amd64.R10, amd64.RAX)
	nextBelow := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.MovRegDeref(amd64.RAX, amd64.R11, amd64ChunkUsed)
	e.CmpRegReg(amd64.R10, amd64.RAX)
	nextAbove := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	emitAMD64ChunkBitIndex(e, amd64.RAX, amd64.R11, amd64.R10)
	e.BtsDerefReg(amd64.R11, amd64ChunkAllocBitmap, amd64.RAX)
	e.Ret()
	next := len(e.Code)
	patchJcc(nextBelow, next)
	patchJcc(nextAbove, next)
	e.MovRegDeref(amd64.R11, amd64.R11, amd64ChunkNext)
	back := len(e.Code)
	e.JmpRel32(0)
	patchJmp(back, loop)
	doneLabel := len(e.Code)
	patchJcc(done, doneLabel)
	e.Ret()
}
