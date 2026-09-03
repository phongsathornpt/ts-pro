package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

// emitAMD64RegExpTest matches the intentionally small native RegExp subset.
// RDI = regex object, RSI = native string, RAX = 0/1.
func emitAMD64RegExpTest(e *amd64.Emitter) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	finish := func(value int64) {
		e.MovRegImm64(amd64.RAX, value)
		e.Pop(amd64.R14)
		e.Pop(amd64.R13)
		e.Pop(amd64.R12)
		e.Pop(amd64.RBX)
		e.Ret()
	}

	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)
	e.MovRegDeref(amd64.R13, amd64.RBX, 24) // compiled literal needle
	e.MovRegDeref(amd64.RAX, amd64.RBX, 32)
	e.MovQXMMReg(amd64.XMM0, amd64.RAX)
	e.Cvttsd2si(amd64.R8, amd64.XMM0) // kind
	e.MovRegDeref(amd64.RAX, amd64.RBX, 40)
	e.MovQXMMReg(amd64.XMM0, amd64.RAX)
	e.Cvttsd2si(amd64.R9, amd64.XMM0)      // flags
	e.MovRegDeref(amd64.R14, amd64.R12, 0) // text length
	e.MovRegDeref(amd64.R11, amd64.R13, 0) // needle length
	e.AddRegImm32(amd64.R12, 8)
	e.AddRegImm32(amd64.R13, 8)
	e.CmpRegReg(amd64.R14, amd64.R11)
	tooShort := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.MovRegReg(amd64.RCX, amd64.R14)
	e.SubRegReg(amd64.RCX, amd64.R11) // last start position
	e.CmpRegImm32(amd64.R8, 1)
	notPrefix := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegImm64(amd64.RCX, 0)
	prefixReady := len(e.Code)
	patchJcc(notPrefix, prefixReady)
	e.MovRegImm64(amd64.R10, 0) // position

	outer := len(e.Code)
	e.CmpRegReg(amd64.R10, amd64.RCX)
	pastEnd := len(e.Code)
	e.JccRel32(amd64.CondA, 0)
	e.MovRegImm64(amd64.RDX, 0) // needle index
	inner := len(e.Code)
	e.CmpRegReg(amd64.RDX, amd64.R11)
	matched := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)

	// text byte at text[pos+j]
	e.MovRegReg(amd64.RSI, amd64.R10)
	e.AddRegReg(amd64.RSI, amd64.RDX)
	e.AddRegReg(amd64.RSI, amd64.R12)
	e.MovzxRegDeref8(amd64.RAX, amd64.RSI, 0)
	// needle byte at needle[j]
	e.MovRegReg(amd64.RSI, amd64.R13)
	e.AddRegReg(amd64.RSI, amd64.RDX)
	e.MovzxRegDeref8(amd64.RSI, amd64.RSI, 0)

	// ASCII case folding when /i is active.
	e.TestRegReg(amd64.R9, amd64.R9)
	compareBytes := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.CmpRegImm32(amd64.RAX, 'A')
	textNoFoldLow := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.CmpRegImm32(amd64.RAX, 'Z')
	textNoFoldHigh := len(e.Code)
	e.JccRel32(amd64.CondA, 0)
	e.AddRegImm32(amd64.RAX, 32)
	textFolded := len(e.Code)
	patchJcc(textNoFoldLow, textFolded)
	patchJcc(textNoFoldHigh, textFolded)
	e.CmpRegImm32(amd64.RSI, 'A')
	needleNoFoldLow := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.CmpRegImm32(amd64.RSI, 'Z')
	needleNoFoldHigh := len(e.Code)
	e.JccRel32(amd64.CondA, 0)
	e.AddRegImm32(amd64.RSI, 32)
	needleFolded := len(e.Code)
	patchJcc(needleNoFoldLow, needleFolded)
	patchJcc(needleNoFoldHigh, needleFolded)
	compareLabel := len(e.Code)
	patchJcc(compareBytes, compareLabel)
	e.CmpRegReg(amd64.RAX, amd64.RSI)
	mismatch := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.AddRegImm32(amd64.RDX, 1)
	innerBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(innerBack, inner)

	matchedLabel := len(e.Code)
	patchJcc(matched, matchedLabel)
	e.CmpRegImm32(amd64.R8, 2)
	plainMatch := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	// \d+ requires one or more ASCII digits immediately after the literal prefix.
	e.MovRegReg(amd64.RAX, amd64.R10)
	e.AddRegReg(amd64.RAX, amd64.R11)
	e.CmpRegReg(amd64.RAX, amd64.R14)
	noDigit := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	e.MovRegReg(amd64.RSI, amd64.R12)
	e.AddRegReg(amd64.RSI, amd64.RAX)
	e.MovzxRegDeref8(amd64.RAX, amd64.RSI, 0)
	e.CmpRegImm32(amd64.RAX, '0')
	noDigitLow := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.CmpRegImm32(amd64.RAX, '9')
	noDigitHigh := len(e.Code)
	e.JccRel32(amd64.CondA, 0)

	trueLabel := len(e.Code)
	patchJcc(plainMatch, trueLabel)
	finish(1)

	nextPosition := len(e.Code)
	patchJcc(mismatch, nextPosition)
	patchJcc(noDigit, nextPosition)
	patchJcc(noDigitLow, nextPosition)
	patchJcc(noDigitHigh, nextPosition)
	e.AddRegImm32(amd64.R10, 1)
	outerBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(outerBack, outer)

	falseLabel := len(e.Code)
	patchJcc(tooShort, falseLabel)
	patchJcc(pastEnd, falseLabel)
	finish(0)
}
