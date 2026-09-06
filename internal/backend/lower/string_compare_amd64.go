package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

// emitAMD64StringEq compares two runtime strings by length and bytes.
// RDI=a, RSI=b, RAX=1 when equal, 0 otherwise.
func emitAMD64StringEq(e *amd64.Emitter) {
	patchJcc := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6))))
	}
	patchJmp := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5))))
	}

	// Identical pointers are always equal, including interned literals.
	e.CmpRegReg(amd64.RDI, amd64.RSI)
	ptrEqual := len(e.Code)
	e.JccRel32(amd64.CondE, 0)

	e.MovRegDeref(amd64.R10, amd64.RDI, 0)
	e.MovRegDeref(amd64.R11, amd64.RSI, 0)
	e.CmpRegReg(amd64.R10, amd64.R11)
	lengthMismatch := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)

	// Empty strings with equal lengths are equal.
	e.TestRegReg(amd64.R10, amd64.R10)
	emptyEqual := len(e.Code)
	e.JccRel32(amd64.CondE, 0)

	e.AddRegImm32(amd64.RDI, 8)
	e.AddRegImm32(amd64.RSI, 8)
	loop := len(e.Code)
	e.MovzxRegDeref8(amd64.RAX, amd64.RDI, 0)
	e.MovzxRegDeref8(amd64.R11, amd64.RSI, 0)
	e.CmpRegReg(amd64.RAX, amd64.R11)
	byteMismatch := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.AddRegImm32(amd64.RDI, 1)
	e.AddRegImm32(amd64.RSI, 1)
	e.SubRegImm32(amd64.R10, 1)
	loopBack := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	patchJcc(loopBack, loop)

	equal := len(e.Code)
	patchJcc(ptrEqual, equal)
	patchJcc(emptyEqual, equal)
	e.MovRegImm64(amd64.RAX, 1)
	doneJump := len(e.Code)
	e.JmpRel32(0)

	notEqual := len(e.Code)
	patchJcc(lengthMismatch, notEqual)
	patchJcc(byteMismatch, notEqual)
	e.MovRegImm64(amd64.RAX, 0)
	done := len(e.Code)
	patchJmp(doneJump, done)
	e.Ret()
}

// emitAMD64StringHash computes a stable FNV-1a hash for a runtime string.
// RDI=raw runtime string, RAX=64-bit hash.
func emitAMD64StringHash(e *amd64.Emitter) {
	patchJcc := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6))))
	}

	e.MovRegDeref(amd64.R10, amd64.RDI, 0)
	e.AddRegImm32(amd64.RDI, 8)
	e.MovRegImm64(amd64.RAX, int64(-3750763034362895579)) // FNV offset basis as signed uint64
	e.TestRegReg(amd64.R10, amd64.R10)
	done := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegImm64(amd64.R11, 1099511628211)
	loop := len(e.Code)
	e.MovzxRegDeref8(amd64.R9, amd64.RDI, 0)
	e.XorRegReg(amd64.RAX, amd64.R9)
	e.ImulRegReg(amd64.RAX, amd64.R11)
	e.AddRegImm32(amd64.RDI, 1)
	e.SubRegImm32(amd64.R10, 1)
	back := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	patchJcc(back, loop)
	doneLabel := len(e.Code)
	patchJcc(done, doneLabel)
	e.Ret()
}
