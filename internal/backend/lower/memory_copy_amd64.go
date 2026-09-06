package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

// emitAMD64CopyQwords copies count 8-byte words from src to dst. src, dst and
// count are advanced/consumed. Four-word unrolling keeps growth/concat copies
// branch-light without requiring libc or a C runtime.
func emitAMD64CopyQwords(e *amd64.Emitter, dst, src, count amd64.Register) {
	patchJcc := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6))))
	}

	e.CmpRegImm32(count, 4)
	tail := len(e.Code)
	e.JccRel32(amd64.CondL, 0)
	loop := len(e.Code)
	for off := int32(0); off < 32; off += 8 {
		e.MovRegDeref(amd64.RAX, src, off)
		e.MovDerefReg(dst, off, amd64.RAX)
	}
	e.AddRegImm32(src, 32)
	e.AddRegImm32(dst, 32)
	e.SubRegImm32(count, 4)
	e.CmpRegImm32(count, 4)
	back := len(e.Code)
	e.JccRel32(amd64.CondGE, 0)
	patchJcc(back, loop)

	tailLabel := len(e.Code)
	patchJcc(tail, tailLabel)
	e.TestRegReg(count, count)
	done := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	tailLoop := len(e.Code)
	e.MovRegDeref(amd64.RAX, src, 0)
	e.MovDerefReg(dst, 0, amd64.RAX)
	e.AddRegImm32(src, 8)
	e.AddRegImm32(dst, 8)
	e.SubRegImm32(count, 1)
	tailBack := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	patchJcc(tailBack, tailLoop)
	doneLabel := len(e.Code)
	patchJcc(done, doneLabel)
}
