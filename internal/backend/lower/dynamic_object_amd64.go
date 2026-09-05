package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

const (
	amd64DynamicCount     int32 = 0
	amd64DynamicCapacity  int32 = 8
	amd64DynamicEntries   int32 = 16
	amd64DynamicLastIndex int32 = 24
	amd64DynamicPayload   int32 = 32
	amd64DynamicEntrySize       = 16
)

func emitAMD64DynamicObjectNew(e *amd64.Emitter, allocOffset int) {
	// Returns a boxed JSValue object reference in RAX.
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.SubRegImm32(amd64.RSP, 32)

	e.MovRegImm64(amd64.RDI, int64(amd64DynamicPayload))
	callObject := len(e.Code)
	e.CallRel32(int32(allocOffset - (callObject + 5)))
	e.MovRegReg(amd64.RBX, amd64.RAX)
	emitAMD64SetObjectType(e, amd64.RBX, amd64ObjectTypeDynamicObject)

	// Root the object while allocating its first entry table.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 1)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	e.MovRegImm64(amd64.R12, 4)
	e.MovRegImm64(amd64.RDI, int64(4*amd64DynamicEntrySize))
	callEntries := len(e.Code)
	e.CallRel32(int32(allocOffset - (callEntries + 5)))
	emitAMD64SetObjectType(e, amd64.RAX, amd64ObjectTypeDynamicEntries)

	// Clear 4 {key,value} pairs.
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.MovRegImm64(amd64.R11, 8)
	e.MovRegImm64(amd64.RDX, 0)
	zeroLoop := len(e.Code)
	e.MovDerefReg(amd64.R10, 0, amd64.RDX)
	e.AddRegImm32(amd64.R10, 8)
	e.SubRegImm32(amd64.R11, 1)
	zeroBack := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	binary.LittleEndian.PutUint32(e.Code[zeroBack+2:], uint32(int32(zeroLoop-(zeroBack+6))))

	e.MovRegImm64(amd64.R10, 0)
	e.MovDerefReg(amd64.RBX, amd64DynamicCount, amd64.R10)
	e.MovDerefReg(amd64.RBX, amd64DynamicCapacity, amd64.R12)
	e.MovDerefReg(amd64.RBX, amd64DynamicEntries, amd64.RAX)
	e.MovRegImm64(amd64.R10, -1)
	e.MovDerefReg(amd64.RBX, amd64DynamicLastIndex, amd64.R10)

	// Unlink temporary root frame and box the stable object payload.
	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.MovRegReg(amd64.RAX, amd64.RBX)
	e.MovRegImm64(amd64.R10, amd64JSPayloadMask)
	e.AndRegReg(amd64.RAX, amd64.R10)
	e.MovRegImm64(amd64.R10, amd64JSRefTag)
	e.OrRegReg(amd64.RAX, amd64.R10)

	e.AddRegImm32(amd64.RSP, 32)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64DynamicGet(e *amd64.Emitter, stringEqOffset int) {
	// RDI=boxed object, RSI=raw native string key. Returns boxed JSValue.
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	emitReturn := func() {
		e.Pop(amd64.R14)
		e.Pop(amd64.R13)
		e.Pop(amd64.R12)
		e.Pop(amd64.RBX)
		e.Pop(amd64.RBP)
		e.Ret()
	}

	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)

	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegImm64(amd64.R10, amd64JSPayloadMask)
	e.AndRegReg(amd64.RBX, amd64.R10)
	e.MovRegReg(amd64.R12, amd64.RSI)

	// Repeated property access is common in loops. Probe the last successful
	// slot first, then fall back to the linear table scan on a cache miss.
	e.MovRegDeref(amd64.R13, amd64.RBX, amd64DynamicLastIndex)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64DynamicCount)
	e.CmpRegReg(amd64.R13, amd64.R10)
	cacheInvalid := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64DynamicEntries)
	e.MovRegReg(amd64.R11, amd64.R13)
	for range 4 {
		e.AddRegReg(amd64.R11, amd64.R11)
	}
	e.AddRegReg(amd64.R10, amd64.R11)
	e.MovRegDeref(amd64.RDI, amd64.R10, 0)
	e.MovRegReg(amd64.RSI, amd64.R12)
	cacheEqCall := len(e.Code)
	e.CallRel32(int32(stringEqOffset - (cacheEqCall + 5)))
	e.TestRegReg(amd64.RAX, amd64.RAX)
	cacheMiss := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64DynamicEntries)
	e.MovRegReg(amd64.R11, amd64.R13)
	for range 4 {
		e.AddRegReg(amd64.R11, amd64.R11)
	}
	e.AddRegReg(amd64.R10, amd64.R11)
	e.MovRegDeref(amd64.RAX, amd64.R10, 8)
	emitReturn()

	linearScan := len(e.Code)
	patchJcc(cacheInvalid, linearScan)
	patchJcc(cacheMiss, linearScan)
	e.MovRegImm64(amd64.R13, 0)

	loop := len(e.Code)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64DynamicCount)
	e.CmpRegReg(amd64.R13, amd64.R10)
	missing := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64DynamicEntries)
	e.MovRegReg(amd64.R11, amd64.R13)
	for range 4 {
		e.AddRegReg(amd64.R11, amd64.R11)
	}
	e.AddRegReg(amd64.R10, amd64.R11)
	e.MovRegDeref(amd64.RDI, amd64.R10, 0)
	e.MovRegReg(amd64.RSI, amd64.R12)
	callEq := len(e.Code)
	e.CallRel32(int32(stringEqOffset - (callEq + 5)))
	e.TestRegReg(amd64.RAX, amd64.RAX)
	found := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.AddRegImm32(amd64.R13, 1)
	back := len(e.Code)
	e.JmpRel32(0)
	patchJmp(back, loop)

	foundLabel := len(e.Code)
	patchJcc(found, foundLabel)
	e.MovDerefReg(amd64.RBX, amd64DynamicLastIndex, amd64.R13)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64DynamicEntries)
	e.MovRegReg(amd64.R11, amd64.R13)
	for range 4 {
		e.AddRegReg(amd64.R11, amd64.R11)
	}
	e.AddRegReg(amd64.R10, amd64.R11)
	e.MovRegDeref(amd64.RAX, amd64.R10, 8)
	emitReturn()

	missingLabel := len(e.Code)
	patchJcc(missing, missingLabel)
	e.MovRegImm64(amd64.RAX, amd64UndefinedBits)
	emitReturn()
}

func emitAMD64DynamicSet(e *amd64.Emitter, allocOffset, stringEqOffset int) {
	// RDI=boxed object, RSI=raw key, RDX=boxed value. Returns value in RAX.
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	emitReturn := func() {
		e.AddRegImm32(amd64.RSP, 48)
		e.Pop(amd64.R14)
		e.Pop(amd64.R13)
		e.Pop(amd64.R12)
		e.Pop(amd64.RBX)
		e.Pop(amd64.RBP)
		e.Ret()
	}

	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.SubRegImm32(amd64.RSP, 48)

	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegImm64(amd64.R10, amd64JSPayloadMask)
	e.AndRegReg(amd64.RBX, amd64.R10)
	e.MovRegReg(amd64.R12, amd64.RSI)
	e.MovRegReg(amd64.R13, amd64.RDX)

	e.MovRegDeref(amd64.R14, amd64.RBX, amd64DynamicLastIndex)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64DynamicCount)
	e.CmpRegReg(amd64.R14, amd64.R10)
	cacheInvalid := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64DynamicEntries)
	e.MovRegReg(amd64.R11, amd64.R14)
	for range 4 {
		e.AddRegReg(amd64.R11, amd64.R11)
	}
	e.AddRegReg(amd64.R10, amd64.R11)
	e.MovRegDeref(amd64.RDI, amd64.R10, 0)
	e.MovRegReg(amd64.RSI, amd64.R12)
	cacheEqCall := len(e.Code)
	e.CallRel32(int32(stringEqOffset - (cacheEqCall + 5)))
	e.TestRegReg(amd64.RAX, amd64.RAX)
	cacheMiss := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64DynamicEntries)
	e.MovRegReg(amd64.R11, amd64.R14)
	for range 4 {
		e.AddRegReg(amd64.R11, amd64.R11)
	}
	e.AddRegReg(amd64.R10, amd64.R11)
	e.MovDerefReg(amd64.R10, 8, amd64.R13)
	e.MovRegReg(amd64.RAX, amd64.R13)
	emitReturn()

	linearSearch := len(e.Code)
	patchJcc(cacheInvalid, linearSearch)
	patchJcc(cacheMiss, linearSearch)
	e.MovRegImm64(amd64.R14, 0)

	search := len(e.Code)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64DynamicCount)
	e.CmpRegReg(amd64.R14, amd64.R10)
	notFound := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64DynamicEntries)
	e.MovRegReg(amd64.R11, amd64.R14)
	for range 4 {
		e.AddRegReg(amd64.R11, amd64.R11)
	}
	e.AddRegReg(amd64.R10, amd64.R11)
	e.MovRegDeref(amd64.RDI, amd64.R10, 0)
	e.MovRegReg(amd64.RSI, amd64.R12)
	callEq := len(e.Code)
	e.CallRel32(int32(stringEqOffset - (callEq + 5)))
	e.TestRegReg(amd64.RAX, amd64.RAX)
	found := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.AddRegImm32(amd64.R14, 1)
	searchBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(searchBack, search)

	foundLabel := len(e.Code)
	patchJcc(found, foundLabel)
	e.MovDerefReg(amd64.RBX, amd64DynamicLastIndex, amd64.R14)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64DynamicEntries)
	e.MovRegReg(amd64.R11, amd64.R14)
	for range 4 {
		e.AddRegReg(amd64.R11, amd64.R11)
	}
	e.AddRegReg(amd64.R10, amd64.R11)
	e.MovDerefReg(amd64.R10, 8, amd64.R13)
	e.MovRegReg(amd64.RAX, amd64.R13)
	emitReturn()

	notFoundLabel := len(e.Code)
	patchJcc(notFound, notFoundLabel)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64DynamicCapacity)
	e.CmpRegReg(amd64.R14, amd64.R10)
	hasCapacity := len(e.Code)
	e.JccRel32(amd64.CondB, 0)

	// Grow table x2. Root object, key, and boxed value while allocating.
	e.AddRegReg(amd64.R10, amd64.R10)
	e.MovDerefReg(amd64.RSP, 40, amd64.R10)
	e.MovRegDeref(amd64.R11, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R11)
	e.MovRegImm64(amd64.R11, 3)
	e.MovDerefReg(amd64.RSP, 8, amd64.R11)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.RSP, 24, amd64.R12)
	e.MovDerefReg(amd64.RSP, 32, amd64.R13)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	e.MovRegDeref(amd64.RDI, amd64.RSP, 40)
	for range 4 {
		e.AddRegReg(amd64.RDI, amd64.RDI)
	}
	callAlloc := len(e.Code)
	e.CallRel32(int32(allocOffset - (callAlloc + 5)))
	e.MovRegReg(amd64.R9, amd64.RAX)
	emitAMD64SetObjectType(e, amd64.R9, amd64ObjectTypeDynamicEntries)

	// Clear new table.
	e.MovRegReg(amd64.R10, amd64.R9)
	e.MovRegDeref(amd64.R11, amd64.RSP, 40)
	e.AddRegReg(amd64.R11, amd64.R11) // two qwords per entry
	e.MovRegImm64(amd64.RAX, 0)
	zeroLoop := len(e.Code)
	e.MovDerefReg(amd64.R10, 0, amd64.RAX)
	e.AddRegImm32(amd64.R10, 8)
	e.SubRegImm32(amd64.R11, 1)
	zeroBack := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	patchJcc(zeroBack, zeroLoop)

	// Copy the live old prefix.
	e.MovRegDeref(amd64.R8, amd64.RBX, amd64DynamicEntries)
	e.MovRegReg(amd64.R10, amd64.R9)
	e.MovRegReg(amd64.R11, amd64.R14)
	e.AddRegReg(amd64.R11, amd64.R11)
	e.TestRegReg(amd64.R11, amd64.R11)
	copyDone := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	copyLoop := len(e.Code)
	e.MovRegDeref(amd64.RAX, amd64.R8, 0)
	e.MovDerefReg(amd64.R10, 0, amd64.RAX)
	e.AddRegImm32(amd64.R8, 8)
	e.AddRegImm32(amd64.R10, 8)
	e.SubRegImm32(amd64.R11, 1)
	copyBack := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	patchJcc(copyBack, copyLoop)
	patchJcc(copyDone, len(e.Code))

	e.MovDerefReg(amd64.RBX, amd64DynamicEntries, amd64.R9)
	e.MovRegDeref(amd64.R10, amd64.RSP, 40)
	e.MovDerefReg(amd64.RBX, amd64DynamicCapacity, amd64.R10)
	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)

	hasCapacityLabel := len(e.Code)
	patchJcc(hasCapacity, hasCapacityLabel)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64DynamicEntries)
	e.MovRegReg(amd64.R11, amd64.R14)
	for range 4 {
		e.AddRegReg(amd64.R11, amd64.R11)
	}
	e.AddRegReg(amd64.R10, amd64.R11)
	e.MovDerefReg(amd64.R10, 0, amd64.R12)
	e.MovDerefReg(amd64.R10, 8, amd64.R13)
	e.MovDerefReg(amd64.RBX, amd64DynamicLastIndex, amd64.R14)
	e.AddRegImm32(amd64.R14, 1)
	e.MovDerefReg(amd64.RBX, amd64DynamicCount, amd64.R14)
	e.MovRegReg(amd64.RAX, amd64.R13)
	emitReturn()
}
