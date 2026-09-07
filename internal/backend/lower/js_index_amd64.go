package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

// emitAMD64JSIndexGet indexes an Array hidden behind an any/unknown JSValue.
// RDI = boxed/raw reference candidate, XMM0 = numeric index, RAX = JSValue.
func emitAMD64JSIndexGet(e *amd64.Emitter, arrayGetOffset int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }

	// Accept the canonical JSRef tag and raw native heap pointers used by a few
	// typed-to-any paths. Reject primitive NaN-boxes before touching a header.
	e.MovRegReg(amd64.R10, amd64.RDI)
	e.MovRegImm64(amd64.R11, amd64JSTagMask)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.MovRegImm64(amd64.R11, amd64JSRefTag)
	e.CmpRegReg(amd64.R10, amd64.R11)
	isTagged := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.TestRegReg(amd64.R10, amd64.R10)
	isRaw := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	invalidJump := len(e.Code)
	e.JmpRel32(0)

	tagged := len(e.Code)
	patchJcc(isTagged, tagged)
	e.MovRegImm64(amd64.R10, amd64JSPayloadMask)
	e.AndRegReg(amd64.RDI, amd64.R10)
	refReadyJump := len(e.Code)
	e.JmpRel32(0)

	raw := len(e.Code)
	patchJcc(isRaw, raw)
	refReady := len(e.Code)
	patchJmp(refReadyJump, refReady)
	e.TestRegReg(amd64.RDI, amd64.RDI)
	nullRef := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegReg(amd64.R10, amd64.RDI)
	e.SubRegImm32(amd64.R10, amd64ObjectHeaderSize)
	e.MovRegDeref(amd64.R11, amd64.R10, amd64ObjectType)
	e.CmpRegImm32(amd64.R11, int32(amd64ObjectTypeArray))
	notArray := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)

	e.Cvttsd2si(amd64.RSI, amd64.XMM0)
	callGet := len(e.Code)
	e.CallRel32(int32(arrayGetOffset - (callGet + 5)))
	doneJump := len(e.Code)
	e.JmpRel32(0)

	invalid := len(e.Code)
	patchJmp(invalidJump, invalid)
	patchJcc(nullRef, invalid)
	patchJcc(notArray, invalid)
	e.MovRegImm64(amd64.RAX, amd64UndefinedBits)
	done := len(e.Code)
	patchJmp(doneJump, done)
	e.Ret()
}
