package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

const (
	amd64EventTargetHead int32 = amd64ObjectFields
	amd64EventTargetTail int32 = amd64ObjectFields + 8

	amd64EventListenerNext     int32 = amd64ObjectFields
	amd64EventListenerType     int32 = amd64ObjectFields + 8
	amd64EventListenerCallback int32 = amd64ObjectFields + 16
	amd64EventListenerOnce     int32 = amd64ObjectFields + 24
	amd64EventListenerRemoved  int32 = amd64ObjectFields + 32
)

func emitAMD64EventTargetNew(e *amd64.Emitter, objectNewOffset int) {
	e.MovRegImm64(amd64.RDI, 2)
	e.MovRegImm64(amd64.RSI, 0b11)
	at := len(e.Code)
	e.CallRel32(int32(objectNewOffset - (at + 5)))
	e.Ret()
}

func emitAMD64EventTargetAdd(e *amd64.Emitter, objectNewOffset, stringEqOffset int) {
	patchJcc := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6))))
	}
	patchJmp := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5))))
	}
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.SubRegImm32(amd64.RSP, 40)
	e.MovRegReg(amd64.RBX, amd64.RDI) // target
	e.MovRegReg(amd64.R12, amd64.RSI) // type
	e.MovRegReg(amd64.R13, amd64.RDX) // callback
	e.MovRegReg(amd64.R14, amd64.RCX) // once

	// Duplicate registrations with the same type and callback are ignored.
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64EventTargetHead)
	scan := len(e.Code)
	e.TestRegReg(amd64.R10, amd64.R10)
	scanDone := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.R11, amd64.R10, amd64EventListenerRemoved)
	e.TestRegReg(amd64.R11, amd64.R11)
	scanNextRemoved := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegDeref(amd64.RDI, amd64.R10, amd64EventListenerType)
	e.MovRegReg(amd64.RSI, amd64.R12)
	e.Push(amd64.R10)
	callEq := len(e.Code)
	e.CallRel32(int32(stringEqOffset - (callEq + 5)))
	e.Pop(amd64.R10)
	e.TestRegReg(amd64.RAX, amd64.RAX)
	scanNextType := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.R11, amd64.R10, amd64EventListenerCallback)
	e.CmpRegReg(amd64.R11, amd64.R13)
	duplicate := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	scanNextLabel := len(e.Code)
	patchJcc(scanNextRemoved, scanNextLabel)
	patchJcc(scanNextType, scanNextLabel)
	e.MovRegDeref(amd64.R10, amd64.R10, amd64EventListenerNext)
	scanBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(scanBack, scan)

	create := len(e.Code)
	patchJcc(scanDone, create)
	// Root target, type, and callback across listener allocation.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 3)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.RSP, 24, amd64.R12)
	e.MovDerefReg(amd64.RSP, 32, amd64.R13)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)
	e.MovRegImm64(amd64.RDI, 5)
	e.MovRegImm64(amd64.RSI, 0b111)
	callNew := len(e.Code)
	e.CallRel32(int32(objectNewOffset - (callNew + 5)))
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.MovRegImm64(amd64.R11, 0)
	e.MovDerefReg(amd64.R10, amd64EventListenerNext, amd64.R11)
	e.MovDerefReg(amd64.R10, amd64EventListenerType, amd64.R12)
	e.MovDerefReg(amd64.R10, amd64EventListenerCallback, amd64.R13)
	e.MovDerefReg(amd64.R10, amd64EventListenerOnce, amd64.R14)
	e.MovDerefReg(amd64.R10, amd64EventListenerRemoved, amd64.R11)

	e.MovRegDeref(amd64.R11, amd64.RBX, amd64EventTargetTail)
	e.TestRegReg(amd64.R11, amd64.R11)
	empty := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovDerefReg(amd64.R11, amd64EventListenerNext, amd64.R10)
	linked := len(e.Code)
	e.JmpRel32(0)
	emptyLabel := len(e.Code)
	patchJcc(empty, emptyLabel)
	e.MovDerefReg(amd64.RBX, amd64EventTargetHead, amd64.R10)
	linkedLabel := len(e.Code)
	patchJmp(linked, linkedLabel)
	e.MovDerefReg(amd64.RBX, amd64EventTargetTail, amd64.R10)

	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	doneJump := len(e.Code)
	e.JmpRel32(0)
	duplicateLabel := len(e.Code)
	patchJcc(duplicate, duplicateLabel)
	done := len(e.Code)
	patchJmp(doneJump, done)
	e.AddRegImm32(amd64.RSP, 40)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64EventTargetRemove(e *amd64.Emitter, stringEqOffset int) {
	patchJcc := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6))))
	}
	patchJmp := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5))))
	}
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)
	e.MovRegReg(amd64.R13, amd64.RDX)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64EventTargetHead)
	loop := len(e.Code)
	e.TestRegReg(amd64.R10, amd64.R10)
	done := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.R11, amd64.R10, amd64EventListenerRemoved)
	e.TestRegReg(amd64.R11, amd64.R11)
	nextRemoved := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegDeref(amd64.RDI, amd64.R10, amd64EventListenerType)
	e.MovRegReg(amd64.RSI, amd64.R12)
	e.Push(amd64.R10)
	callEq := len(e.Code)
	e.CallRel32(int32(stringEqOffset - (callEq + 5)))
	e.Pop(amd64.R10)
	e.TestRegReg(amd64.RAX, amd64.RAX)
	nextType := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.R11, amd64.R10, amd64EventListenerCallback)
	e.CmpRegReg(amd64.R11, amd64.R13)
	nextCallback := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegImm64(amd64.R11, 1)
	e.MovDerefReg(amd64.R10, amd64EventListenerRemoved, amd64.R11)
	found := len(e.Code)
	e.JmpRel32(0)
	next := len(e.Code)
	patchJcc(nextRemoved, next)
	patchJcc(nextType, next)
	patchJcc(nextCallback, next)
	e.MovRegDeref(amd64.R10, amd64.R10, amd64EventListenerNext)
	back := len(e.Code)
	e.JmpRel32(0)
	patchJmp(back, loop)
	end := len(e.Code)
	patchJcc(done, end)
	patchJmp(found, end)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64EventTargetNext(e *amd64.Emitter, stringEqOffset int) {
	patchJcc := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6))))
	}
	patchJmp := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5))))
	}
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.MovRegReg(amd64.RBX, amd64.RSI) // event type
	e.TestRegReg(amd64.RDX, amd64.RDX)
	fromHead := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.R12, amd64.RDX, amd64EventListenerNext)
	haveStart := len(e.Code)
	e.JmpRel32(0)
	fromHeadLabel := len(e.Code)
	patchJcc(fromHead, fromHeadLabel)
	e.MovRegDeref(amd64.R12, amd64.RDI, amd64EventTargetHead)
	patchJmp(haveStart, len(e.Code))
	loop := len(e.Code)
	e.TestRegReg(amd64.R12, amd64.R12)
	none := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.R10, amd64.R12, amd64EventListenerRemoved)
	e.TestRegReg(amd64.R10, amd64.R10)
	nextRemoved := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegDeref(amd64.RDI, amd64.R12, amd64EventListenerType)
	e.MovRegReg(amd64.RSI, amd64.RBX)
	callEq := len(e.Code)
	e.CallRel32(int32(stringEqOffset - (callEq + 5)))
	e.TestRegReg(amd64.RAX, amd64.RAX)
	found := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	next := len(e.Code)
	patchJcc(nextRemoved, next)
	e.MovRegDeref(amd64.R12, amd64.R12, amd64EventListenerNext)
	back := len(e.Code)
	e.JmpRel32(0)
	patchJmp(back, loop)
	foundLabel := len(e.Code)
	patchJcc(found, foundLabel)
	e.MovRegReg(amd64.RAX, amd64.R12)
	doneJump := len(e.Code)
	e.JmpRel32(0)
	noneLabel := len(e.Code)
	patchJcc(none, noneLabel)
	e.MovRegImm64(amd64.RAX, 0)
	done := len(e.Code)
	patchJmp(doneJump, done)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64EventListenerCallback(e *amd64.Emitter) {
	e.MovRegDeref(amd64.RAX, amd64.RDI, amd64EventListenerCallback)
	e.Ret()
}

func emitAMD64EventListenerOnce(e *amd64.Emitter) {
	e.MovRegDeref(amd64.RAX, amd64.RDI, amd64EventListenerOnce)
	e.Ret()
}

func emitAMD64EventListenerRemove(e *amd64.Emitter) {
	e.MovRegImm64(amd64.R10, 1)
	e.MovDerefReg(amd64.RDI, amd64EventListenerRemoved, amd64.R10)
	e.Ret()
}

func emitAMD64EventTargetTail(e *amd64.Emitter) {
	e.MovRegDeref(amd64.RAX, amd64.RDI, amd64EventTargetTail)
	e.Ret()
}

func emitAMD64NullRef(e *amd64.Emitter) {
	e.MovRegImm64(amd64.RAX, 0)
	e.Ret()
}
