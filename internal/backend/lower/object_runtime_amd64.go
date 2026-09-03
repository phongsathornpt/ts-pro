package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

const (
	amd64ObjectFieldCount int32 = 0
	amd64ObjectRefMask    int32 = 8
	amd64ObjectFields     int32 = 16
)

func emitAMD64ObjectNew(e *amd64.Emitter, allocOffset int) {
	// RDI=field count, RSI=reference-field mask. The payload stores two metadata
	// words followed by raw 64-bit field slots in deterministic shape order.
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)

	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)
	e.MovRegReg(amd64.RDI, amd64.RBX)
	e.AddRegReg(amd64.RDI, amd64.RDI)
	e.AddRegReg(amd64.RDI, amd64.RDI)
	e.AddRegReg(amd64.RDI, amd64.RDI)
	e.AddRegImm32(amd64.RDI, amd64ObjectFields)
	callAlloc := len(e.Code)
	e.CallRel32(int32(allocOffset - (callAlloc + 5)))

	emitAMD64SetObjectType(e, amd64.RAX, amd64ObjectTypeObject)
	e.MovDerefReg(amd64.RAX, amd64ObjectFieldCount, amd64.RBX)
	e.MovDerefReg(amd64.RAX, amd64ObjectRefMask, amd64.R12)

	// Reused free blocks can contain old heap pointers. Clear every field slot
	// before the object is exposed to generated code or the collector.
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.AddRegImm32(amd64.R10, amd64ObjectFields)
	e.MovRegReg(amd64.R11, amd64.RBX)
	e.TestRegReg(amd64.R11, amd64.R11)
	zeroDone := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegImm64(amd64.RDX, 0)
	zeroLoop := len(e.Code)
	e.MovDerefReg(amd64.R10, 0, amd64.RDX)
	e.AddRegImm32(amd64.R10, 8)
	e.SubRegImm32(amd64.R11, 1)
	zeroBack := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	binary.LittleEndian.PutUint32(e.Code[zeroBack+2:], uint32(int32(zeroLoop-(zeroBack+6))))
	zeroDoneLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[zeroDone+2:], uint32(int32(zeroDoneLabel-(zeroDone+6))))

	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}
