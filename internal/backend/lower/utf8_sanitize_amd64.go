package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

func emitAMD64UTF8Sanitize(e *amd64.Emitter, allocOffset int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.SubRegImm32(amd64.RSP, 48)

	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.Cvttsd2si(amd64.R12, amd64.XMM0)
	e.Cvttsd2si(amd64.R13, amd64.XMM1)
	e.MovDerefReg(amd64.RSP, 32, amd64.RSI)

	// Root the source wrapper across output allocation.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 1)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	// Worst case is one U+FFFD (3 UTF-8 bytes) for every input byte.
	e.MovRegReg(amd64.RDI, amd64.R13)
	e.MovRegReg(amd64.R10, amd64.R13)
	e.AddRegReg(amd64.RDI, amd64.R10)
	e.AddRegReg(amd64.RDI, amd64.R10)
	e.AddRegImm32(amd64.RDI, 8)
	callAlloc := len(e.Code)
	e.CallRel32(int32(allocOffset - (callAlloc + 5)))
	e.MovRegReg(amd64.R14, amd64.RAX)

	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)

	// Resolve source/destination cursors after allocation. The collector is non-moving.
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64ByteBufferData)
	e.AddRegReg(amd64.R10, amd64.R12)
	e.MovRegReg(amd64.R12, amd64.R10)
	e.MovRegReg(amd64.R11, amd64.R14)
	e.AddRegImm32(amd64.R11, 8)

	// Encoding Standard strips an initial UTF-8 BOM unless ignoreBOM is true.
	e.MovRegDeref(amd64.R10, amd64.RSP, 32)
	e.TestRegReg(amd64.R10, amd64.R10)
	keepBOM := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.CmpRegImm32(amd64.R13, 3)
	keepShort := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.MovzxRegDeref8(amd64.R8, amd64.R12, 0)
	e.CmpRegImm32(amd64.R8, 0xEF)
	keep0 := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovzxRegDeref8(amd64.R8, amd64.R12, 1)
	e.CmpRegImm32(amd64.R8, 0xBB)
	keep1 := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)

	e.MovzxRegDeref8(amd64.R8, amd64.R12, 2)
	e.CmpRegImm32(amd64.R8, 0xBF)
	keep2 := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.AddRegImm32(amd64.R12, 3)
	e.SubRegImm32(amd64.R13, 3)
	bomDone := len(e.Code)
	for _, at := range []int{keepBOM, keepShort, keep0, keep1, keep2} {
		patchJcc(at, bomDone)
	}

	loop := len(e.Code)
	e.TestRegReg(amd64.R13, amd64.R13)
	done := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovzxRegDeref8(amd64.R10, amd64.R12, 0)
	e.CmpRegImm32(amd64.R10, 0x80)
	ascii := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.CmpRegImm32(amd64.R10, 0xC2)
	invalidLead := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.CmpRegImm32(amd64.R10, 0xE0)
	two := len(e.Code)
	e.JccRel32(amd64.CondB, 0)

	e.CmpRegImm32(amd64.R10, 0xF0)
	three := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.CmpRegImm32(amd64.R10, 0xF5)
	four := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	invalidHigh := len(e.Code)
	e.JmpRel32(0)

	asciiLabel := len(e.Code)
	patchJcc(ascii, asciiLabel)
	e.MovRegImm64(amd64.R9, 1)
	copyJumpASCII := len(e.Code)
	e.JmpRel32(0)

	twoLabel := len(e.Code)
	patchJcc(two, twoLabel)
	e.CmpRegImm32(amd64.R13, 2)
	twoInvalidShort := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.MovzxRegDeref8(amd64.R8, amd64.R12, 1)
	e.CmpRegImm32(amd64.R8, 0x80)
	twoInvalidLow := len(e.Code)
	e.JccRel32(amd64.CondB, 0)

	e.CmpRegImm32(amd64.R8, 0xBF)
	twoInvalidHigh := len(e.Code)
	e.JccRel32(amd64.CondA, 0)
	e.MovRegImm64(amd64.R9, 2)
	copyJumpTwo := len(e.Code)
	e.JmpRel32(0)

	threeLabel := len(e.Code)
	patchJcc(three, threeLabel)
	e.CmpRegImm32(amd64.R13, 3)
	threeInvalidShort := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.MovzxRegDeref8(amd64.R8, amd64.R12, 1)
	e.CmpRegImm32(amd64.R10, 0xE0)
	threeNotE0 := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.CmpRegImm32(amd64.R8, 0xA0)
	threeInvalidE0Low := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.CmpRegImm32(amd64.R8, 0xBF)
	threeInvalidE0High := len(e.Code)
	e.JccRel32(amd64.CondA, 0)
	threeSecondOK := len(e.Code)
	e.JmpRel32(0)

	threeNotE0Label := len(e.Code)
	patchJcc(threeNotE0, threeNotE0Label)
	e.CmpRegImm32(amd64.R10, 0xED)
	threeDefault := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.CmpRegImm32(amd64.R8, 0x80)
	threeInvalidEDLow := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.CmpRegImm32(amd64.R8, 0x9F)
	threeInvalidEDHigh := len(e.Code)
	e.JccRel32(amd64.CondA, 0)
	threeEDDone := len(e.Code)
	e.JmpRel32(0)

	threeDefaultLabel := len(e.Code)
	patchJcc(threeDefault, threeDefaultLabel)
	e.CmpRegImm32(amd64.R8, 0x80)
	threeInvalidLow := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.CmpRegImm32(amd64.R8, 0xBF)
	threeInvalidHigh := len(e.Code)
	e.JccRel32(amd64.CondA, 0)
	threeSecondDone := len(e.Code)
	patchJmp(threeSecondOK, threeSecondDone)
	patchJmp(threeEDDone, threeSecondDone)

	e.MovzxRegDeref8(amd64.R8, amd64.R12, 2)
	e.CmpRegImm32(amd64.R8, 0x80)
	threeInvalidThirdLow := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.CmpRegImm32(amd64.R8, 0xBF)
	threeInvalidThirdHigh := len(e.Code)
	e.JccRel32(amd64.CondA, 0)
	e.MovRegImm64(amd64.R9, 3)
	copyJumpThree := len(e.Code)
	e.JmpRel32(0)

	fourLabel := len(e.Code)
	patchJcc(four, fourLabel)
	e.CmpRegImm32(amd64.R13, 4)
	fourInvalidShort := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.MovzxRegDeref8(amd64.R8, amd64.R12, 1)
	e.CmpRegImm32(amd64.R10, 0xF0)
	fourNotF0 := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.CmpRegImm32(amd64.R8, 0x90)
	fourInvalidF0Low := len(e.Code)
	e.JccRel32(amd64.CondB, 0)

	e.CmpRegImm32(amd64.R8, 0xBF)
	fourInvalidF0High := len(e.Code)
	e.JccRel32(amd64.CondA, 0)
	fourSecondOK := len(e.Code)
	e.JmpRel32(0)

	fourNotF0Label := len(e.Code)
	patchJcc(fourNotF0, fourNotF0Label)
	e.CmpRegImm32(amd64.R10, 0xF4)
	fourDefault := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.CmpRegImm32(amd64.R8, 0x80)
	fourInvalidF4Low := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.CmpRegImm32(amd64.R8, 0x8F)
	fourInvalidF4High := len(e.Code)
	e.JccRel32(amd64.CondA, 0)
	fourF4Done := len(e.Code)
	e.JmpRel32(0)

	fourDefaultLabel := len(e.Code)
	patchJcc(fourDefault, fourDefaultLabel)
	e.CmpRegImm32(amd64.R8, 0x80)
	fourInvalidLow := len(e.Code)
	e.JccRel32(amd64.CondB, 0)

	e.CmpRegImm32(amd64.R8, 0xBF)
	fourInvalidHigh := len(e.Code)
	e.JccRel32(amd64.CondA, 0)
	fourSecondDone := len(e.Code)
	patchJmp(fourSecondOK, fourSecondDone)
	patchJmp(fourF4Done, fourSecondDone)
	fourInvalidDefaultLow, fourInvalidDefaultHigh := fourInvalidLow, fourInvalidHigh
	fourInvalidF4FirstLow, fourInvalidF4FirstHigh := fourInvalidF4Low, fourInvalidF4High
	for _, disp := range []int32{2, 3} {
		e.MovzxRegDeref8(amd64.R8, amd64.R12, disp)
		e.CmpRegImm32(amd64.R8, 0x80)
		badLow := len(e.Code)
		e.JccRel32(amd64.CondB, 0)
		e.CmpRegImm32(amd64.R8, 0xBF)
		badHigh := len(e.Code)
		e.JccRel32(amd64.CondA, 0)
		// Patch these after the common invalid label is known.
		if disp == 2 {
			fourInvalidLow = badLow
			fourInvalidHigh = badHigh
		} else {
			fourInvalidF4Low = badLow
			fourInvalidF4High = badHigh
		}
	}
	e.MovRegImm64(amd64.R9, 4)
	copyJumpFour := len(e.Code)
	e.JmpRel32(0)

	copyLabel := len(e.Code)
	for _, at := range []int{copyJumpASCII, copyJumpTwo, copyJumpThree, copyJumpFour} {
		patchJmp(at, copyLabel)
	}
	copyLoop := len(e.Code)
	e.MovzxRegDeref8(amd64.RAX, amd64.R12, 0)
	e.MovDerefReg8(amd64.R11, 0, amd64.RAX)
	e.AddRegImm32(amd64.R12, 1)
	e.AddRegImm32(amd64.R11, 1)
	e.SubRegImm32(amd64.R13, 1)
	e.SubRegImm32(amd64.R9, 1)
	copyBack := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	patchJcc(copyBack, copyLoop)
	copyDone := len(e.Code)
	e.JmpRel32(0)

	invalidLabel := len(e.Code)
	patchJcc(invalidLead, invalidLabel)
	patchJmp(invalidHigh, invalidLabel)

	for _, at := range []int{
		twoInvalidShort, twoInvalidLow, twoInvalidHigh,
		threeInvalidShort, threeInvalidE0Low, threeInvalidE0High,
		threeInvalidEDLow, threeInvalidEDHigh, threeInvalidLow, threeInvalidHigh,
		threeInvalidThirdLow, threeInvalidThirdHigh,
		fourInvalidShort, fourInvalidF0Low, fourInvalidF0High,
		fourInvalidF4FirstLow, fourInvalidF4FirstHigh,
		fourInvalidDefaultLow, fourInvalidDefaultHigh,
		fourInvalidLow, fourInvalidHigh, fourInvalidF4Low, fourInvalidF4High,
	} {
		patchJcc(at, invalidLabel)
	}
	// Replacement character U+FFFD encoded as EF BF BD.
	for _, b := range []int64{0xEF, 0xBF, 0xBD} {
		e.MovRegImm64(amd64.RAX, b)
		e.MovDerefReg8(amd64.R11, 0, amd64.RAX)
		e.AddRegImm32(amd64.R11, 1)
	}
	e.AddRegImm32(amd64.R12, 1)
	e.SubRegImm32(amd64.R13, 1)
	invalidDone := len(e.Code)
	e.JmpRel32(0)

	loopContinue := len(e.Code)
	patchJmp(copyDone, loopContinue)
	patchJmp(invalidDone, loopContinue)
	loopBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(loopBack, loop)

	doneLabel := len(e.Code)
	patchJcc(done, doneLabel)
	// Visible string length is bytes written after the 8-byte length word.
	e.MovRegReg(amd64.R10, amd64.R11)
	e.SubRegReg(amd64.R10, amd64.R14)
	e.SubRegImm32(amd64.R10, 8)
	e.MovDerefReg(amd64.R14, 0, amd64.R10)
	e.MovRegReg(amd64.RAX, amd64.R14)
	e.AddRegImm32(amd64.RSP, 48)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}
