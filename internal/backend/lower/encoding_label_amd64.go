package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

func emitAMD64EncodingLabelEq(e *amd64.Emitter) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }

	// RDI=input label, RSI=lowercase canonical label.
	e.MovRegDeref(amd64.R8, amd64.RDI, 0)
	e.MovRegReg(amd64.R10, amd64.RDI)
	e.AddRegImm32(amd64.R10, 8)

	leadLoop := len(e.Code)
	e.TestRegReg(amd64.R8, amd64.R8)
	leadDoneEmpty := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovzxRegDeref8(amd64.RAX, amd64.R10, 0)

	var leadWhitespace []int
	for _, ch := range []int32{0x20, 0x09, 0x0a, 0x0c, 0x0d} {
		e.CmpRegImm32(amd64.RAX, ch)
		at := len(e.Code)
		e.JccRel32(amd64.CondE, 0)
		leadWhitespace = append(leadWhitespace, at)
	}
	leadStop := len(e.Code)
	e.JmpRel32(0)
	leadConsume := len(e.Code)
	for _, at := range leadWhitespace {
		patchJcc(at, leadConsume)
	}
	e.AddRegImm32(amd64.R10, 1)
	e.SubRegImm32(amd64.R8, 1)
	leadBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(leadBack, leadLoop)
	leadDone := len(e.Code)
	patchJcc(leadDoneEmpty, leadDone)
	patchJmp(leadStop, leadDone)

	trailLoop := len(e.Code)
	e.TestRegReg(amd64.R8, amd64.R8)
	trailDoneEmpty := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegReg(amd64.R11, amd64.R10)
	e.AddRegReg(amd64.R11, amd64.R8)
	e.SubRegImm32(amd64.R11, 1)
	e.MovzxRegDeref8(amd64.RAX, amd64.R11, 0)
	var trailWhitespace []int
	for _, ch := range []int32{0x20, 0x09, 0x0a, 0x0c, 0x0d} {
		e.CmpRegImm32(amd64.RAX, ch)
		at := len(e.Code)
		e.JccRel32(amd64.CondE, 0)
		trailWhitespace = append(trailWhitespace, at)
	}
	trailStop := len(e.Code)
	e.JmpRel32(0)
	trailConsume := len(e.Code)
	for _, at := range trailWhitespace {
		patchJcc(at, trailConsume)
	}
	e.SubRegImm32(amd64.R8, 1)
	trailBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(trailBack, trailLoop)

	trailDone := len(e.Code)
	patchJcc(trailDoneEmpty, trailDone)
	patchJmp(trailStop, trailDone)

	e.MovRegDeref(amd64.R9, amd64.RSI, 0)
	e.CmpRegReg(amd64.R8, amd64.R9)
	lengthMismatch := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegReg(amd64.RDX, amd64.RSI)
	e.AddRegImm32(amd64.RDX, 8)
	e.MovRegReg(amd64.R9, amd64.R8)

	cmpLoop := len(e.Code)
	e.TestRegReg(amd64.R9, amd64.R9)
	matchDone := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovzxRegDeref8(amd64.RAX, amd64.R10, 0)
	e.CmpRegImm32(amd64.RAX, 'A')
	notUpperLow := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.CmpRegImm32(amd64.RAX, 'Z')
	notUpperHigh := len(e.Code)
	e.JccRel32(amd64.CondA, 0)
	e.AddRegImm32(amd64.RAX, 32)

	normalized := len(e.Code)
	patchJcc(notUpperLow, normalized)
	patchJcc(notUpperHigh, normalized)
	e.MovzxRegDeref8(amd64.RCX, amd64.RDX, 0)
	e.CmpRegReg(amd64.RAX, amd64.RCX)
	charMismatch := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.AddRegImm32(amd64.R10, 1)
	e.AddRegImm32(amd64.RDX, 1)
	e.SubRegImm32(amd64.R9, 1)
	cmpBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(cmpBack, cmpLoop)

	matchLabel := len(e.Code)
	patchJcc(matchDone, matchLabel)
	e.MovRegImm64(amd64.RAX, 1)
	e.Ret()

	falseLabel := len(e.Code)
	patchJcc(lengthMismatch, falseLabel)
	patchJcc(charMismatch, falseLabel)
	e.MovRegImm64(amd64.RAX, 0)
	e.Ret()
}
