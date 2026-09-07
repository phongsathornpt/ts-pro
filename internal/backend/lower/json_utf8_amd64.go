package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

// emitAMD64UTF8FromCodePoint returns a freshly allocated native UTF-8 string
// for one Unicode scalar/code-unit value. JSON escape parsing also uses this
// for lone UTF-16 surrogate escapes so their payload is preserved bytewise.
// XMM0 = code point number, RAX = native string.
func emitAMD64UTF8FromCodePoint(e *amd64.Emitter, allocOffset int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }

	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.SubRegImm32(amd64.RSP, 8)
	e.Cvttsd2si(amd64.R12, amd64.XMM0)

	// Clamp nonsensical values to U+FFFD. Valid parser paths only pass
	// 0..0x10FFFF, but keeping this helper total avoids another trap surface.
	e.TestRegReg(amd64.R12, amd64.R12)
	nonNegative := len(e.Code)
	e.JccRel32(amd64.CondGE, 0)
	e.MovRegImm64(amd64.R12, 0xfffd)
	patchJcc(nonNegative, len(e.Code))
	e.CmpRegImm32(amd64.R12, 0x10ffff)
	inRange := len(e.Code)
	e.JccRel32(amd64.CondLE, 0)
	e.MovRegImm64(amd64.R12, 0xfffd)
	patchJcc(inRange, len(e.Code))

	e.MovRegImm64(amd64.RDI, 12) // 8-byte length + at most 4 UTF-8 bytes.
	callAlloc := len(e.Code)
	e.CallRel32(int32(allocOffset - (callAlloc + 5)))
	e.MovRegReg(amd64.RBX, amd64.RAX)

	e.CmpRegImm32(amd64.R12, 0x7f)
	ascii := len(e.Code)
	e.JccRel32(amd64.CondBE, 0)
	e.CmpRegImm32(amd64.R12, 0x7ff)
	two := len(e.Code)
	e.JccRel32(amd64.CondBE, 0)
	e.CmpRegImm32(amd64.R12, 0xffff)
	three := len(e.Code)
	e.JccRel32(amd64.CondBE, 0)

	// Four-byte sequence.
	e.MovRegImm64(amd64.R13, 4)
	e.MovRegReg(amd64.R10, amd64.R12)
	e.ShrRegImm8(amd64.R10, 18)
	e.MovRegImm64(amd64.R11, 0xf0)
	e.OrRegReg(amd64.R10, amd64.R11)
	e.MovDerefReg8(amd64.RBX, 8, amd64.R10)
	e.MovRegReg(amd64.R10, amd64.R12)
	e.ShrRegImm8(amd64.R10, 12)
	e.MovRegImm64(amd64.R11, 0x3f)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.MovRegImm64(amd64.R11, 0x80)
	e.OrRegReg(amd64.R10, amd64.R11)
	e.MovDerefReg8(amd64.RBX, 9, amd64.R10)
	e.MovRegReg(amd64.R10, amd64.R12)
	e.ShrRegImm8(amd64.R10, 6)
	e.MovRegImm64(amd64.R11, 0x3f)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.MovRegImm64(amd64.R11, 0x80)
	e.OrRegReg(amd64.R10, amd64.R11)
	e.MovDerefReg8(amd64.RBX, 10, amd64.R10)
	e.MovRegReg(amd64.R10, amd64.R12)
	e.MovRegImm64(amd64.R11, 0x3f)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.MovRegImm64(amd64.R11, 0x80)
	e.OrRegReg(amd64.R10, amd64.R11)
	e.MovDerefReg8(amd64.RBX, 11, amd64.R10)
	fourDone := len(e.Code)
	e.JmpRel32(0)

	threeLabel := len(e.Code)
	patchJcc(three, threeLabel)
	e.MovRegImm64(amd64.R13, 3)
	e.MovRegReg(amd64.R10, amd64.R12)
	e.ShrRegImm8(amd64.R10, 12)
	e.MovRegImm64(amd64.R11, 0xe0)
	e.OrRegReg(amd64.R10, amd64.R11)
	e.MovDerefReg8(amd64.RBX, 8, amd64.R10)
	e.MovRegReg(amd64.R10, amd64.R12)
	e.ShrRegImm8(amd64.R10, 6)
	e.MovRegImm64(amd64.R11, 0x3f)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.MovRegImm64(amd64.R11, 0x80)
	e.OrRegReg(amd64.R10, amd64.R11)
	e.MovDerefReg8(amd64.RBX, 9, amd64.R10)
	e.MovRegReg(amd64.R10, amd64.R12)
	e.MovRegImm64(amd64.R11, 0x3f)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.MovRegImm64(amd64.R11, 0x80)
	e.OrRegReg(amd64.R10, amd64.R11)
	e.MovDerefReg8(amd64.RBX, 10, amd64.R10)
	threeDone := len(e.Code)
	e.JmpRel32(0)

	twoLabel := len(e.Code)
	patchJcc(two, twoLabel)
	e.MovRegImm64(amd64.R13, 2)
	e.MovRegReg(amd64.R10, amd64.R12)
	e.ShrRegImm8(amd64.R10, 6)
	e.MovRegImm64(amd64.R11, 0xc0)
	e.OrRegReg(amd64.R10, amd64.R11)
	e.MovDerefReg8(amd64.RBX, 8, amd64.R10)
	e.MovRegReg(amd64.R10, amd64.R12)
	e.MovRegImm64(amd64.R11, 0x3f)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.MovRegImm64(amd64.R11, 0x80)
	e.OrRegReg(amd64.R10, amd64.R11)
	e.MovDerefReg8(amd64.RBX, 9, amd64.R10)
	twoDone := len(e.Code)
	e.JmpRel32(0)

	asciiLabel := len(e.Code)
	patchJcc(ascii, asciiLabel)
	e.MovRegImm64(amd64.R13, 1)
	e.MovDerefReg8(amd64.RBX, 8, amd64.R12)

	done := len(e.Code)
	patchJmp(fourDone, done)
	patchJmp(threeDone, done)
	patchJmp(twoDone, done)
	e.MovDerefReg(amd64.RBX, 0, amd64.R13)
	e.MovRegReg(amd64.RAX, amd64.RBX)
	e.AddRegImm32(amd64.RSP, 8)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}
