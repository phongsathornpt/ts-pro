package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

const (
	amd64ArrayLength   int32 = 0
	amd64ArrayCapacity int32 = 8
	amd64ArrayElemKind int32 = 16
	amd64ArrayData     int32 = 24
	amd64ArrayPayload  int32 = 32
)

func emitAMD64SetObjectType(e *amd64.Emitter, payload amd64.Register, typ int64) {
	e.MovRegReg(amd64.R10, payload)
	e.SubRegImm32(amd64.R10, amd64ObjectHeaderSize)
	e.MovRegImm64(amd64.R11, typ)
	e.MovDerefReg(amd64.R10, amd64ObjectType, amd64.R11)
}

func emitAMD64ArrayNew(e *amd64.Emitter, allocOffset int) {
	// RDI=len (integer), RSI=element kind (0 scalar, 1 heap reference).
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.SubRegImm32(amd64.RSP, 32)

	e.MovRegReg(amd64.R12, amd64.RDI)
	e.MovRegReg(amd64.R13, amd64.RSI)
	e.MovRegReg(amd64.R14, amd64.R12)
	e.CmpRegImm32(amd64.R14, 4)
	capOK := len(e.Code)
	e.JccRel32(amd64.CondGE, 0)
	e.MovRegImm64(amd64.R14, 4)
	capReady := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[capOK+2:], uint32(int32(capReady-(capOK+6))))

	// Allocate stable array identity first.
	e.MovRegImm64(amd64.RDI, int64(amd64ArrayPayload))
	callArray := len(e.Code)
	e.CallRel32(int32(allocOffset - (callArray + 5)))
	e.MovRegReg(amd64.RBX, amd64.RAX)
	emitAMD64SetObjectType(e, amd64.RBX, amd64ObjectTypeArray)

	// Root the array across backing-store allocation.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 1)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	e.MovRegReg(amd64.RDI, amd64.R14)
	e.AddRegReg(amd64.RDI, amd64.RDI)
	e.AddRegReg(amd64.RDI, amd64.RDI)
	e.AddRegReg(amd64.RDI, amd64.RDI) // capacity * 8
	callData := len(e.Code)
	e.CallRel32(int32(allocOffset - (callData + 5)))
	e.MovRegReg(amd64.R9, amd64.RAX)

	// Backing-store object kind controls future GC tracing.
	e.TestRegReg(amd64.R13, amd64.R13)
	scalar := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	emitAMD64SetObjectType(e, amd64.R9, amd64ObjectTypeRefData)
	typeDoneJump := len(e.Code)
	e.JmpRel32(0)
	scalarLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[scalar+2:], uint32(int32(scalarLabel-(scalar+6))))
	emitAMD64SetObjectType(e, amd64.R9, amd64ObjectTypeArrayData)
	typeDone := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[typeDoneJump+1:], uint32(int32(typeDone-(typeDoneJump+5))))

	// Reused free blocks may contain stale payloads; zero the full capacity.
	e.MovRegReg(amd64.R10, amd64.R9)
	e.MovRegReg(amd64.R11, amd64.R14)
	e.MovRegImm64(amd64.RAX, 0)
	zeroLoop := len(e.Code)
	e.MovDerefReg(amd64.R10, 0, amd64.RAX)
	e.AddRegImm32(amd64.R10, 8)
	e.SubRegImm32(amd64.R11, 1)
	zeroBack := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	binary.LittleEndian.PutUint32(e.Code[zeroBack+2:], uint32(int32(zeroLoop-(zeroBack+6))))

	e.MovDerefReg(amd64.RBX, amd64ArrayLength, amd64.R12)
	e.MovDerefReg(amd64.RBX, amd64ArrayCapacity, amd64.R14)
	e.MovDerefReg(amd64.RBX, amd64ArrayElemKind, amd64.R13)
	e.MovDerefReg(amd64.RBX, amd64ArrayData, amd64.R9)

	// Unlink temporary root frame.
	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.MovRegReg(amd64.RAX, amd64.RBX)
	e.AddRegImm32(amd64.RSP, 32)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64ArrayGet(e *amd64.Emitter) {
	// RDI=array, RSI=index. Return raw 64-bit element payload in RAX.
	e.MovRegDeref(amd64.R10, amd64.RDI, amd64ArrayLength)
	e.CmpRegReg(amd64.RSI, amd64.R10)
	oob := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	e.MovRegDeref(amd64.R10, amd64.RDI, amd64ArrayData)
	e.MovRegReg(amd64.R11, amd64.RSI)
	e.AddRegReg(amd64.R11, amd64.R11)
	e.AddRegReg(amd64.R11, amd64.R11)
	e.AddRegReg(amd64.R11, amd64.R11)
	e.AddRegReg(amd64.R10, amd64.R11)
	e.MovRegDeref(amd64.RAX, amd64.R10, 0)
	e.Ret()
	oobLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[oob+2:], uint32(int32(oobLabel-(oob+6))))
	e.MovRegDeref(amd64.R10, amd64.RDI, amd64ArrayElemKind)
	e.CmpRegImm32(amd64.R10, 2)
	notJSValue := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegImm64(amd64.RAX, amd64UndefinedBits)
	e.Ret()
	notJSValueLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[notJSValue+2:], uint32(int32(notJSValueLabel-(notJSValue+6))))
	e.MovRegImm64(amd64.RAX, 0)
	e.Ret()
}

func emitAMD64ArraySet(e *amd64.Emitter, allocOffset int) {
	// RDI=array, RSI=index, RDX=raw element. Grow the backing store when a
	// non-negative index exceeds capacity, preserving the array and a reference
	// element across a collection triggered by the allocation.
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.SubRegImm32(amd64.RSP, 32)

	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)
	e.MovRegReg(amd64.R13, amd64.RDX)

	// Negative numeric indices are not array elements in JavaScript. Property
	// lowering is a separate path; keep this element intrinsic side-effect free.
	e.CmpRegImm32(amd64.R12, 0)
	negative := len(e.Code)
	e.JccRel32(amd64.CondL, 0)

	e.MovRegDeref(amd64.R14, amd64.RBX, amd64ArrayCapacity)
	e.CmpRegReg(amd64.R12, amd64.R14)
	haveCapacity := len(e.Code)
	e.JccRel32(amd64.CondB, 0)

	// Find the smallest doubled capacity that contains index.
	growCap := len(e.Code)
	e.AddRegReg(amd64.R14, amd64.R14)
	e.CmpRegReg(amd64.R12, amd64.R14)
	growAgain := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	binary.LittleEndian.PutUint32(e.Code[growAgain+2:], uint32(int32(growCap-(growAgain+6))))

	// Root the stable array identity. Reference arrays also root the incoming
	// element until it has been published into the new backing store.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R11, 1)
	e.MovDerefReg(amd64.RSP, 8, amd64.R11)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.RSP, 24, amd64.R13)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64ArrayElemKind)
	e.TestRegReg(amd64.R10, amd64.R10)
	scalarRootCount := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegImm64(amd64.R11, 2)
	e.MovDerefReg(amd64.RSP, 8, amd64.R11)
	scalarRootCountLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[scalarRootCount+2:], uint32(int32(scalarRootCountLabel-(scalarRootCount+6))))
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	// Allocate and type the replacement backing store.
	e.MovRegReg(amd64.RDI, amd64.R14)
	e.AddRegReg(amd64.RDI, amd64.RDI)
	e.AddRegReg(amd64.RDI, amd64.RDI)
	e.AddRegReg(amd64.RDI, amd64.RDI)
	callData := len(e.Code)
	e.CallRel32(int32(allocOffset - (callData + 5)))
	e.MovRegReg(amd64.R9, amd64.RAX)

	e.MovRegDeref(amd64.R10, amd64.RBX, amd64ArrayElemKind)
	e.TestRegReg(amd64.R10, amd64.R10)
	growScalar := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	emitAMD64SetObjectType(e, amd64.R9, amd64ObjectTypeRefData)
	growTypeDoneJump := len(e.Code)
	e.JmpRel32(0)
	growScalarLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[growScalar+2:], uint32(int32(growScalarLabel-(growScalar+6))))
	emitAMD64SetObjectType(e, amd64.R9, amd64ObjectTypeArrayData)
	growTypeDone := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[growTypeDoneJump+1:], uint32(int32(growTypeDone-(growTypeDoneJump+5))))

	// Copy the live prefix first. Only the unused tail needs clearing: reused
	// blocks may contain stale references, while the copied prefix is overwritten.
	e.MovRegDeref(amd64.R8, amd64.RBX, amd64ArrayData)
	e.MovRegReg(amd64.R10, amd64.R9)
	e.MovRegDeref(amd64.R11, amd64.RBX, amd64ArrayLength)
	e.TestRegReg(amd64.R11, amd64.R11)
	copyDoneIfZero := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	copyLoop := len(e.Code)
	e.MovRegDeref(amd64.RAX, amd64.R8, 0)
	e.MovDerefReg(amd64.R10, 0, amd64.RAX)
	e.AddRegImm32(amd64.R8, 8)
	e.AddRegImm32(amd64.R10, 8)
	e.SubRegImm32(amd64.R11, 1)
	copyBack := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	binary.LittleEndian.PutUint32(e.Code[copyBack+2:], uint32(int32(copyLoop-(copyBack+6))))
	copyDone := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[copyDoneIfZero+2:], uint32(int32(copyDone-(copyDoneIfZero+6))))

	// R10 already points just past the copied prefix. Clear only capacity-length.
	e.MovRegReg(amd64.R11, amd64.R14)
	e.MovRegDeref(amd64.RAX, amd64.RBX, amd64ArrayLength)
	e.SubRegReg(amd64.R11, amd64.RAX)
	e.TestRegReg(amd64.R11, amd64.R11)
	clearTailDone := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegImm64(amd64.RAX, 0)
	clearTailLoop := len(e.Code)
	e.MovDerefReg(amd64.R10, 0, amd64.RAX)
	e.AddRegImm32(amd64.R10, 8)
	e.SubRegImm32(amd64.R11, 1)
	clearTailBack := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	binary.LittleEndian.PutUint32(e.Code[clearTailBack+2:], uint32(int32(clearTailLoop-(clearTailBack+6))))
	clearTailDoneLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[clearTailDone+2:], uint32(int32(clearTailDoneLabel-(clearTailDone+6))))

	e.MovDerefReg(amd64.RBX, amd64ArrayData, amd64.R9)
	e.MovDerefReg(amd64.RBX, amd64ArrayCapacity, amd64.R14)
	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)

	haveCapacityLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[haveCapacity+2:], uint32(int32(haveCapacityLabel-(haveCapacity+6))))

	// Store the element and extend logical length when necessary.
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64ArrayData)
	e.MovRegReg(amd64.R11, amd64.R12)
	e.AddRegReg(amd64.R11, amd64.R11)
	e.AddRegReg(amd64.R11, amd64.R11)
	e.AddRegReg(amd64.R11, amd64.R11)
	e.AddRegReg(amd64.R10, amd64.R11)
	e.MovDerefReg(amd64.R10, 0, amd64.R13)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64ArrayLength)
	e.CmpRegReg(amd64.R12, amd64.R10)
	lengthOK := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.MovRegReg(amd64.R10, amd64.R12)
	e.AddRegImm32(amd64.R10, 1)
	e.MovDerefReg(amd64.RBX, amd64ArrayLength, amd64.R10)
	end := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[lengthOK+2:], uint32(int32(end-(lengthOK+6))))
	binary.LittleEndian.PutUint32(e.Code[negative+2:], uint32(int32(end-(negative+6))))

	e.AddRegImm32(amd64.RSP, 32)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64ArrayLength(e *amd64.Emitter) {
	e.MovRegDeref(amd64.RAX, amd64.RDI, amd64ArrayLength)
	e.Cvtsi2sd(amd64.XMM0, amd64.RAX)
	e.Ret()
}

func emitAMD64ArrayPush(e *amd64.Emitter, allocOffset int) {
	// RDI=array, RSI=raw element; returns new length as F64 in XMM0.
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.SubRegImm32(amd64.RSP, 32)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)
	e.MovRegDeref(amd64.R13, amd64.RBX, amd64ArrayLength)
	e.MovRegDeref(amd64.R14, amd64.RBX, amd64ArrayCapacity)
	e.CmpRegReg(amd64.R13, amd64.R14)
	haveCapacity := len(e.Code)
	e.JccRel32(amd64.CondB, 0)

	// Grow capacity x2. Root both array identity and old backing store across GC.
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64ArrayData)
	e.MovRegDeref(amd64.R11, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R11)
	e.MovRegImm64(amd64.R11, 2)
	e.MovDerefReg(amd64.RSP, 8, amd64.R11)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.RSP, 24, amd64.R10)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	e.AddRegReg(amd64.R14, amd64.R14)
	e.MovRegReg(amd64.RDI, amd64.R14)
	e.AddRegReg(amd64.RDI, amd64.RDI)
	e.AddRegReg(amd64.RDI, amd64.RDI)
	e.AddRegReg(amd64.RDI, amd64.RDI)
	callData := len(e.Code)
	e.CallRel32(int32(allocOffset - (callData + 5)))
	e.MovRegReg(amd64.R9, amd64.RAX)

	// Apply backing-store type from array element kind.
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64ArrayElemKind)
	e.TestRegReg(amd64.R10, amd64.R10)
	growScalar := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	emitAMD64SetObjectType(e, amd64.R9, amd64ObjectTypeRefData)
	growTypeDoneJump := len(e.Code)
	e.JmpRel32(0)
	growScalarLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[growScalar+2:], uint32(int32(growScalarLabel-(growScalar+6))))
	emitAMD64SetObjectType(e, amd64.R9, amd64ObjectTypeArrayData)
	growTypeDone := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[growTypeDoneJump+1:], uint32(int32(growTypeDone-(growTypeDoneJump+5))))

	// Copy the live prefix first; clearing it before immediately overwriting it
	// only burns memory bandwidth on every growth.
	e.MovRegDeref(amd64.R8, amd64.RBX, amd64ArrayData)
	e.MovRegReg(amd64.R10, amd64.R9)
	e.MovRegReg(amd64.R11, amd64.R13)
	e.TestRegReg(amd64.R11, amd64.R11)
	copyDoneIfZero := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	copyLoop := len(e.Code)
	e.MovRegDeref(amd64.RAX, amd64.R8, 0)
	e.MovDerefReg(amd64.R10, 0, amd64.RAX)
	e.AddRegImm32(amd64.R8, 8)
	e.AddRegImm32(amd64.R10, 8)
	e.SubRegImm32(amd64.R11, 1)
	copyBack := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	binary.LittleEndian.PutUint32(e.Code[copyBack+2:], uint32(int32(copyLoop-(copyBack+6))))
	copyDone := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[copyDoneIfZero+2:], uint32(int32(copyDone-(copyDoneIfZero+6))))

	// R10 points at the first unused slot. Clear only the new tail so reclaimed
	// blocks cannot retain stale references while avoiding duplicate prefix writes.
	e.MovRegReg(amd64.R11, amd64.R14)
	e.SubRegReg(amd64.R11, amd64.R13)
	e.TestRegReg(amd64.R11, amd64.R11)
	clearTailDone := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegImm64(amd64.RAX, 0)
	clearTailLoop := len(e.Code)
	e.MovDerefReg(amd64.R10, 0, amd64.RAX)
	e.AddRegImm32(amd64.R10, 8)
	e.SubRegImm32(amd64.R11, 1)
	clearTailBack := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	binary.LittleEndian.PutUint32(e.Code[clearTailBack+2:], uint32(int32(clearTailLoop-(clearTailBack+6))))
	clearTailDoneLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[clearTailDone+2:], uint32(int32(clearTailDoneLabel-(clearTailDone+6))))

	e.MovDerefReg(amd64.RBX, amd64ArrayData, amd64.R9)
	e.MovDerefReg(amd64.RBX, amd64ArrayCapacity, amd64.R14)
	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)

	haveCapacityLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[haveCapacity+2:], uint32(int32(haveCapacityLabel-(haveCapacity+6))))
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64ArrayData)
	e.MovRegReg(amd64.R11, amd64.R13)
	e.AddRegReg(amd64.R11, amd64.R11)
	e.AddRegReg(amd64.R11, amd64.R11)
	e.AddRegReg(amd64.R11, amd64.R11)
	e.AddRegReg(amd64.R10, amd64.R11)
	e.MovDerefReg(amd64.R10, 0, amd64.R12)
	e.AddRegImm32(amd64.R13, 1)
	e.MovDerefReg(amd64.RBX, amd64ArrayLength, amd64.R13)
	e.Cvtsi2sd(amd64.XMM0, amd64.R13)
	e.AddRegImm32(amd64.RSP, 32)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64ArrayPop(e *amd64.Emitter) {
	// RDI=array. Return raw element payload in RAX, clearing the slot for GC.
	e.MovRegDeref(amd64.R10, amd64.RDI, amd64ArrayLength)
	e.TestRegReg(amd64.R10, amd64.R10)
	empty := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.SubRegImm32(amd64.R10, 1)
	e.MovDerefReg(amd64.RDI, amd64ArrayLength, amd64.R10)
	e.MovRegDeref(amd64.R11, amd64.RDI, amd64ArrayData)
	e.MovRegReg(amd64.RAX, amd64.R10)
	e.AddRegReg(amd64.RAX, amd64.RAX)
	e.AddRegReg(amd64.RAX, amd64.RAX)
	e.AddRegReg(amd64.RAX, amd64.RAX)
	e.AddRegReg(amd64.R11, amd64.RAX)
	e.MovRegDeref(amd64.RAX, amd64.R11, 0)
	e.MovRegImm64(amd64.R10, 0)
	e.MovDerefReg(amd64.R11, 0, amd64.R10)
	e.Ret()
	emptyLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[empty+2:], uint32(int32(emptyLabel-(empty+6))))
	e.MovRegDeref(amd64.R10, amd64.RDI, amd64ArrayElemKind)
	e.CmpRegImm32(amd64.R10, 2)
	notJSValue := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegImm64(amd64.RAX, amd64UndefinedBits)
	e.Ret()
	notJSValueLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[notJSValue+2:], uint32(int32(notJSValueLabel-(notJSValue+6))))
	e.MovRegImm64(amd64.RAX, 0)
	e.Ret()
}
