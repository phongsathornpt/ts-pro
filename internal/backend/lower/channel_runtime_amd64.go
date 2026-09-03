package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

const (
	amd64ChannelCapacity int32 = 0
	amd64ChannelCount    int32 = 8
	amd64ChannelHead     int32 = 16
	amd64ChannelTail     int32 = 24
	amd64ChannelData     int32 = 32
	amd64ChannelPayload  int32 = 40
)

func emitAMD64ChannelNew(e *amd64.Emitter, allocOffset int) {
	// XMM0 = capacity. Returns raw channel payload in RAX.
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.SubRegImm32(amd64.RSP, 32)
	e.Cvttsd2si(amd64.R12, amd64.XMM0)
	e.TestRegReg(amd64.R12, amd64.R12)
	capacityOK := len(e.Code)
	e.JccRel32(amd64.CondGE, 0)
	e.MovRegImm64(amd64.R12, 0)
	patchJcc(capacityOK, len(e.Code))

	e.MovRegImm64(amd64.RDI, int64(amd64ChannelPayload))
	callChannel := len(e.Code)
	e.CallRel32(int32(allocOffset - (callChannel + 5)))
	e.MovRegReg(amd64.RBX, amd64.RAX)
	emitAMD64SetObjectType(e, amd64.RBX, amd64ObjectTypeChannel)
	e.MovDerefReg(amd64.RBX, amd64ChannelCapacity, amd64.R12)
	e.MovRegImm64(amd64.R10, 0)
	e.MovDerefReg(amd64.RBX, amd64ChannelCount, amd64.R10)
	e.MovDerefReg(amd64.RBX, amd64ChannelHead, amd64.R10)
	e.MovDerefReg(amd64.RBX, amd64ChannelTail, amd64.R10)
	e.MovDerefReg(amd64.RBX, amd64ChannelData, amd64.R10)

	e.TestRegReg(amd64.R12, amd64.R12)
	noData := len(e.Code)
	e.JccRel32(amd64.CondE, 0)

	// Root the channel while allocating its backing store.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 1)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	e.MovRegReg(amd64.RDI, amd64.R12)
	e.MovRegImm64(amd64.R10, 8)
	e.ImulRegReg(amd64.RDI, amd64.R10)
	callData := len(e.Code)
	e.CallRel32(int32(allocOffset - (callData + 5)))
	emitAMD64SetObjectType(e, amd64.RAX, amd64ObjectTypeJSValueData)
	e.MovDerefReg(amd64.RBX, amd64ChannelData, amd64.RAX)

	// Clear every slot because swept/reused blocks may contain old tagged refs.
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.MovRegReg(amd64.R11, amd64.R12)
	clearLoop := len(e.Code)
	e.TestRegReg(amd64.R11, amd64.R11)
	clearDone := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegImm64(amd64.RAX, 0)
	e.MovDerefReg(amd64.R10, 0, amd64.RAX)
	e.AddRegImm32(amd64.R10, 8)
	e.SubRegImm32(amd64.R11, 1)
	clearBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(clearBack, clearLoop)
	patchJcc(clearDone, len(e.Code))

	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)

	finishJump := len(e.Code)
	e.JmpRel32(0)
	noDataLabel := len(e.Code)
	patchJcc(noData, noDataLabel)
	finish := len(e.Code)
	patchJmp(finishJump, finish)
	e.MovRegReg(amd64.RAX, amd64.RBX)
	e.AddRegImm32(amd64.RSP, 32)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64ChannelTrySend(e *amd64.Emitter) {
	// RDI=channel, RSI=boxed JSValue. Returns boolean in RAX.
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	e.MovRegDeref(amd64.R10, amd64.RDI, amd64ChannelCapacity)
	e.TestRegReg(amd64.R10, amd64.R10)
	full := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.R11, amd64.RDI, amd64ChannelCount)
	e.CmpRegReg(amd64.R11, amd64.R10)
	full2 := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)

	e.MovRegDeref(amd64.RAX, amd64.RDI, amd64ChannelData)
	e.MovRegDeref(amd64.RDX, amd64.RDI, amd64ChannelTail)
	e.MovRegReg(amd64.R8, amd64.RDX)
	e.MovRegImm64(amd64.R9, 8)
	e.ImulRegReg(amd64.R8, amd64.R9)
	e.AddRegReg(amd64.RAX, amd64.R8)
	e.MovDerefReg(amd64.RAX, 0, amd64.RSI)

	// tail = (tail + 1) % capacity
	e.AddRegImm32(amd64.RDX, 1)
	e.MovRegReg(amd64.RAX, amd64.RDX)
	e.Cqo()
	e.IdivReg(amd64.R10)
	e.MovDerefReg(amd64.RDI, amd64ChannelTail, amd64.RDX)
	e.AddRegImm32(amd64.R11, 1)
	e.MovDerefReg(amd64.RDI, amd64ChannelCount, amd64.R11)
	e.MovRegImm64(amd64.RAX, 1)
	doneJump := len(e.Code)
	e.JmpRel32(0)

	fullLabel := len(e.Code)
	patchJcc(full, fullLabel)
	patchJcc(full2, fullLabel)
	e.MovRegImm64(amd64.RAX, 0)
	done := len(e.Code)
	patchJmp(doneJump, done)
	e.Ret()
}

func emitAMD64ChannelTryRecvOr(e *amd64.Emitter) {
	// RDI=channel, RSI=boxed fallback. Returns boxed JSValue in RAX.
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	e.MovRegDeref(amd64.R10, amd64.RDI, amd64ChannelCount)
	e.TestRegReg(amd64.R10, amd64.R10)
	empty := len(e.Code)
	e.JccRel32(amd64.CondE, 0)

	e.MovRegDeref(amd64.R11, amd64.RDI, amd64ChannelData)
	e.MovRegDeref(amd64.RDX, amd64.RDI, amd64ChannelHead)
	e.MovRegReg(amd64.R8, amd64.RDX)
	e.MovRegImm64(amd64.R9, 8)
	e.ImulRegReg(amd64.R8, amd64.R9)
	e.AddRegReg(amd64.R11, amd64.R8)
	e.MovRegDeref(amd64.RAX, amd64.R11, 0)
	e.MovRegImm64(amd64.R8, 0)
	e.MovDerefReg(amd64.R11, 0, amd64.R8)

	// head = (head + 1) % capacity
	e.AddRegImm32(amd64.RDX, 1)
	e.MovRegReg(amd64.R8, amd64.RAX) // preserve result
	e.MovRegReg(amd64.RAX, amd64.RDX)
	e.Cqo()
	e.MovRegDeref(amd64.R11, amd64.RDI, amd64ChannelCapacity)
	e.IdivReg(amd64.R11)
	e.MovDerefReg(amd64.RDI, amd64ChannelHead, amd64.RDX)
	e.SubRegImm32(amd64.R10, 1)
	e.MovDerefReg(amd64.RDI, amd64ChannelCount, amd64.R10)
	e.MovRegReg(amd64.RAX, amd64.R8)
	doneJump := len(e.Code)
	e.JmpRel32(0)

	emptyLabel := len(e.Code)
	patchJcc(empty, emptyLabel)
	e.MovRegReg(amd64.RAX, amd64.RSI)
	done := len(e.Code)
	patchJmp(doneJump, done)
	e.Ret()
}
