package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

func emitAMD64JSStrictEqual(e *amd64.Emitter, stringEqOffset int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }

	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)

	// Identical payloads are strictly equal except canonical numeric NaN.
	e.CmpRegReg(amd64.RBX, amd64.R12)
	notIdentical := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegImm64(amd64.R10, amd64JSNumberNaN)
	e.CmpRegReg(amd64.RBX, amd64.R10)
	identicalNaN := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegImm64(amd64.RAX, 1)
	trueDone := len(e.Code)
	e.JmpRel32(0)

	// +0 and -0 compare equal even though their payloads differ.
	notIdenticalLabel := len(e.Code)
	patchJcc(notIdentical, notIdenticalLabel)
	e.MovRegReg(amd64.R10, amd64.RBX)
	e.MovRegReg(amd64.R11, amd64.R12)
	e.MovRegImm64(amd64.RAX, 0x7fffffffffffffff)
	e.AndRegReg(amd64.R10, amd64.RAX)
	e.AndRegReg(amd64.R11, amd64.RAX)
	e.TestRegReg(amd64.R10, amd64.R10)
	lhsNonZero := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.TestRegReg(amd64.R11, amd64.R11)
	rhsNonZero := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegImm64(amd64.RAX, 1)
	zeroDone := len(e.Code)
	e.JmpRel32(0)

	// Distinct strings compare by value. Distinct values of all other same or
	// different JS types are not strictly equal.
	stringCheck := len(e.Code)
	patchJcc(lhsNonZero, stringCheck)
	patchJcc(rhsNonZero, stringCheck)
	e.MovRegReg(amd64.R10, amd64.RBX)
	e.MovRegImm64(amd64.R11, amd64JSTagMask)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.MovRegImm64(amd64.RAX, amd64JSStringTag)
	e.CmpRegReg(amd64.R10, amd64.RAX)
	falseLHS := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegReg(amd64.R10, amd64.R12)
	e.MovRegImm64(amd64.R11, amd64JSTagMask)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.MovRegImm64(amd64.RAX, amd64JSStringTag)
	e.CmpRegReg(amd64.R10, amd64.RAX)
	falseRHS := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegReg(amd64.RDI, amd64.RBX)
	e.MovRegReg(amd64.RSI, amd64.R12)
	e.MovRegImm64(amd64.R10, amd64JSPayloadMask)
	e.AndRegReg(amd64.RDI, amd64.R10)
	e.AndRegReg(amd64.RSI, amd64.R10)
	callEq := len(e.Code)
	e.CallRel32(int32(stringEqOffset - (callEq + 5)))
	stringDone := len(e.Code)
	e.JmpRel32(0)

	falseLabel := len(e.Code)
	patchJcc(identicalNaN, falseLabel)
	patchJcc(falseLHS, falseLabel)
	patchJcc(falseRHS, falseLabel)
	e.MovRegImm64(amd64.RAX, 0)

	done := len(e.Code)
	patchJmp(trueDone, done)
	patchJmp(zeroDone, done)
	patchJmp(stringDone, done)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64JSLooseEqual(e *amd64.Emitter, selfOffset, strictEqOffset, toNumberOffset, toStringOffset int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	call := func(target int) { at := len(e.Code); e.CallRel32(int32(target - (at + 5))) }

	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.SubRegImm32(amd64.RSP, 40)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)

	// Keep both boxed operands alive across ToPrimitive allocations.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 2)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.RSP, 24, amd64.R12)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	// Strict equality is a subset of loose equality.
	e.MovRegReg(amd64.RDI, amd64.RBX)
	e.MovRegReg(amd64.RSI, amd64.R12)
	call(strictEqOffset)
	e.TestRegReg(amd64.RAX, amd64.RAX)
	strictFalse := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegImm64(amd64.RAX, 1)
	strictTrueDone := len(e.Code)
	e.JmpRel32(0)
	strictFalseLabel := len(e.Code)
	patchJcc(strictFalse, strictFalseLabel)

	// null == undefined, and neither is loosely equal to other primitive types.
	var lhsNullish []int
	for _, bits := range []int64{amd64NullBits, amd64UndefinedBits} {
		e.MovRegImm64(amd64.R10, bits)
		e.CmpRegReg(amd64.RBX, amd64.R10)
		at := len(e.Code)
		e.JccRel32(amd64.CondE, 0)
		lhsNullish = append(lhsNullish, at)
	}
	var rhsNullish []int
	for _, bits := range []int64{amd64NullBits, amd64UndefinedBits} {
		e.MovRegImm64(amd64.R10, bits)
		e.CmpRegReg(amd64.R12, amd64.R10)
		at := len(e.Code)
		e.JccRel32(amd64.CondE, 0)
		rhsNullish = append(rhsNullish, at)
	}

	// Objects/arrays are converted to their primitive string form when compared
	// to a primitive. Distinct references remain unequal because strict equality
	// already handled identity.
	e.MovRegReg(amd64.R10, amd64.RBX)
	e.MovRegImm64(amd64.R11, amd64JSTagMask)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.MovRegImm64(amd64.R11, amd64JSRefTag)
	e.CmpRegReg(amd64.R10, amd64.R11)
	lhsRef := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegReg(amd64.R10, amd64.R12)
	e.MovRegImm64(amd64.R11, amd64JSTagMask)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.MovRegImm64(amd64.R11, amd64JSRefTag)
	e.CmpRegReg(amd64.R10, amd64.R11)
	rhsRef := len(e.Code)
	e.JccRel32(amd64.CondE, 0)

	// Two distinct strings are already known unequal from strict equality.
	e.MovRegReg(amd64.R10, amd64.RBX)
	e.MovRegImm64(amd64.R11, amd64JSTagMask)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.MovRegImm64(amd64.R13, amd64JSStringTag)
	e.CmpRegReg(amd64.R10, amd64.R13)
	notLHSString := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegReg(amd64.R10, amd64.R12)
	e.MovRegImm64(amd64.R11, amd64JSTagMask)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.CmpRegReg(amd64.R10, amd64.R13)
	bothStrings := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	notLHSStringLabel := len(e.Code)
	patchJcc(notLHSString, notLHSStringLabel)

	// All remaining mixed primitive cases compare after ToNumber.
	e.MovRegReg(amd64.RDI, amd64.RBX)
	call(toNumberOffset)
	e.MovQRegXMM(amd64.R13, amd64.XMM0)
	e.MovRegReg(amd64.RDI, amd64.R12)
	call(toNumberOffset)
	e.MovSDRegReg(amd64.XMM1, amd64.XMM0)
	e.MovQXMMReg(amd64.XMM0, amd64.R13)
	e.Ucomisd(amd64.XMM0, amd64.XMM1)
	e.Setcc(amd64.CondE, amd64.RAX)
	e.Setcc(amd64.CondNP, amd64.R10)
	e.AndRegReg(amd64.RAX, amd64.R10)
	numericDone := len(e.Code)
	e.JmpRel32(0)

	// lhs reference path.
	lhsRefLabel := len(e.Code)
	patchJcc(lhsRef, lhsRefLabel)
	e.MovRegReg(amd64.R10, amd64.R12)
	e.MovRegImm64(amd64.R11, amd64JSTagMask)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.MovRegImm64(amd64.R11, amd64JSRefTag)
	e.CmpRegReg(amd64.R10, amd64.R11)
	bothRefs := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegReg(amd64.RDI, amd64.RBX)
	call(toStringOffset)
	e.MovRegImm64(amd64.R10, amd64JSPayloadMask)
	e.AndRegReg(amd64.RAX, amd64.R10)
	e.MovRegImm64(amd64.R10, amd64JSStringTag)
	e.OrRegReg(amd64.RAX, amd64.R10)
	e.MovRegReg(amd64.RBX, amd64.RAX)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovRegReg(amd64.RDI, amd64.RBX)
	e.MovRegReg(amd64.RSI, amd64.R12)
	call(selfOffset)
	lhsRefDone := len(e.Code)
	e.JmpRel32(0)

	// rhs reference path.
	rhsRefLabel := len(e.Code)
	patchJcc(rhsRef, rhsRefLabel)
	e.MovRegReg(amd64.RDI, amd64.R12)
	call(toStringOffset)
	e.MovRegImm64(amd64.R10, amd64JSPayloadMask)
	e.AndRegReg(amd64.RAX, amd64.R10)
	e.MovRegImm64(amd64.R10, amd64JSStringTag)
	e.OrRegReg(amd64.RAX, amd64.R10)
	e.MovRegReg(amd64.R12, amd64.RAX)
	e.MovDerefReg(amd64.RSP, 24, amd64.R12)
	e.MovRegReg(amd64.RDI, amd64.RBX)
	e.MovRegReg(amd64.RSI, amd64.R12)
	call(selfOffset)
	rhsRefDone := len(e.Code)
	e.JmpRel32(0)

	// LHS nullish: true iff RHS is also nullish.
	lhsNullishLabel := len(e.Code)
	for _, at := range lhsNullish {
		patchJcc(at, lhsNullishLabel)
	}
	var nullishTrue []int
	for _, bits := range []int64{amd64NullBits, amd64UndefinedBits} {
		e.MovRegImm64(amd64.R10, bits)
		e.CmpRegReg(amd64.R12, amd64.R10)
		at := len(e.Code)
		e.JccRel32(amd64.CondE, 0)
		nullishTrue = append(nullishTrue, at)
	}
	lhsNullishFalseDone := len(e.Code)
	e.MovRegImm64(amd64.RAX, 0)
	lhsNullishFalseJump := len(e.Code)
	e.JmpRel32(0)

	falseLabel := len(e.Code)
	for _, at := range rhsNullish {
		patchJcc(at, falseLabel)
	}
	patchJcc(bothStrings, falseLabel)
	patchJcc(bothRefs, falseLabel)
	e.MovRegImm64(amd64.RAX, 0)
	falseDone := len(e.Code)
	e.JmpRel32(0)

	trueLabel := len(e.Code)
	for _, at := range nullishTrue {
		patchJcc(at, trueLabel)
	}
	e.MovRegImm64(amd64.RAX, 1)

	done := len(e.Code)
	patchJmp(strictTrueDone, done)
	patchJmp(numericDone, done)
	patchJmp(lhsRefDone, done)
	patchJmp(rhsRefDone, done)
	patchJmp(lhsNullishFalseJump, done)
	patchJmp(falseDone, done)
	_ = lhsNullishFalseDone
	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.AddRegImm32(amd64.RSP, 40)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64JSStringCompare(e *amd64.Emitter) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }

	e.MovRegDeref(amd64.R8, amd64.RDI, 0)
	e.MovRegDeref(amd64.R9, amd64.RSI, 0)
	e.MovRegReg(amd64.R10, amd64.RDI)
	e.AddRegImm32(amd64.R10, 8)
	e.MovRegReg(amd64.R11, amd64.RSI)
	e.AddRegImm32(amd64.R11, 8)

	loop := len(e.Code)
	e.TestRegReg(amd64.R8, amd64.R8)
	lhsEmpty := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.TestRegReg(amd64.R9, amd64.R9)
	rhsEmpty := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovzxRegDeref8(amd64.RAX, amd64.R10, 0)
	e.MovzxRegDeref8(amd64.RDX, amd64.R11, 0)
	e.CmpRegReg(amd64.RAX, amd64.RDX)
	less := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	greater := len(e.Code)
	e.JccRel32(amd64.CondA, 0)
	e.AddRegImm32(amd64.R10, 1)
	e.AddRegImm32(amd64.R11, 1)
	e.SubRegImm32(amd64.R8, 1)
	e.SubRegImm32(amd64.R9, 1)
	back := len(e.Code)
	e.JmpRel32(0)
	patchJmp(back, loop)

	lhsEmptyLabel := len(e.Code)
	patchJcc(lhsEmpty, lhsEmptyLabel)
	e.TestRegReg(amd64.R9, amd64.R9)
	bothEmpty := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	lessLabel := len(e.Code)
	patchJcc(less, lessLabel)
	e.MovRegImm64(amd64.RAX, -1)
	e.Ret()

	rhsEmptyLabel := len(e.Code)
	patchJcc(rhsEmpty, rhsEmptyLabel)
	greaterLabel := len(e.Code)
	patchJcc(greater, greaterLabel)
	e.MovRegImm64(amd64.RAX, 1)
	e.Ret()

	bothEmptyLabel := len(e.Code)
	patchJcc(bothEmpty, bothEmptyLabel)
	e.MovRegImm64(amd64.RAX, 0)
	e.Ret()
}

func emitAMD64JSRelational(e *amd64.Emitter, toNumberOffset, toStringOffset, stringCompareOffset int, op string) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	call := func(target int) { at := len(e.Code); e.CallRel32(int32(target - (at + 5))) }

	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.SubRegImm32(amd64.RSP, 40)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)

	// Root operands. Reference ToPrimitive may allocate a native string and the
	// converted boxed string replaces the corresponding root slot.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 2)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.RSP, 24, amd64.R12)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	convertRef := func(reg amd64.Register, rootOffset int32) {
		e.MovRegReg(amd64.R10, reg)
		e.MovRegImm64(amd64.R11, amd64JSTagMask)
		e.AndRegReg(amd64.R10, amd64.R11)
		e.MovRegImm64(amd64.R11, amd64JSRefTag)
		e.CmpRegReg(amd64.R10, amd64.R11)
		notRef := len(e.Code)
		e.JccRel32(amd64.CondNE, 0)
		e.MovRegReg(amd64.RDI, reg)
		call(toStringOffset)
		e.MovRegImm64(amd64.R10, amd64JSPayloadMask)
		e.AndRegReg(amd64.RAX, amd64.R10)
		e.MovRegImm64(amd64.R10, amd64JSStringTag)
		e.OrRegReg(amd64.RAX, amd64.R10)
		e.MovRegReg(reg, amd64.RAX)
		e.MovDerefReg(amd64.RSP, rootOffset, reg)
		done := len(e.Code)
		patchJcc(notRef, done)
	}
	convertRef(amd64.RBX, 16)
	convertRef(amd64.R12, 24)

	// If both ToPrimitive results are strings, use lexicographic comparison.
	e.MovRegReg(amd64.R10, amd64.RBX)
	e.MovRegImm64(amd64.R11, amd64JSTagMask)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.MovRegImm64(amd64.R13, amd64JSStringTag)
	e.CmpRegReg(amd64.R10, amd64.R13)
	numeric := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegReg(amd64.R10, amd64.R12)
	e.MovRegImm64(amd64.R11, amd64JSTagMask)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.CmpRegReg(amd64.R10, amd64.R13)
	numericRHS := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegReg(amd64.RDI, amd64.RBX)
	e.MovRegReg(amd64.RSI, amd64.R12)
	e.MovRegImm64(amd64.R10, amd64JSPayloadMask)
	e.AndRegReg(amd64.RDI, amd64.R10)
	e.AndRegReg(amd64.RSI, amd64.R10)
	call(stringCompareOffset)
	e.CmpRegImm32(amd64.RAX, 0)
	switch op {
	case "lt":
		e.Setcc(amd64.CondL, amd64.RAX)
	case "le":
		e.Setcc(amd64.CondLE, amd64.RAX)
	case "gt":
		e.Setcc(amd64.CondG, amd64.RAX)
	case "ge":
		e.Setcc(amd64.CondGE, amd64.RAX)
	}
	stringDone := len(e.Code)
	e.JmpRel32(0)

	// Otherwise both sides follow ToNumber semantics.
	numericLabel := len(e.Code)
	patchJcc(numeric, numericLabel)
	patchJcc(numericRHS, numericLabel)
	e.MovRegReg(amd64.RDI, amd64.RBX)
	call(toNumberOffset)
	e.MovQRegXMM(amd64.R13, amd64.XMM0)
	e.MovRegReg(amd64.RDI, amd64.R12)
	call(toNumberOffset)
	e.MovSDRegReg(amd64.XMM1, amd64.XMM0)
	e.MovQXMMReg(amd64.XMM0, amd64.R13)
	e.Ucomisd(amd64.XMM0, amd64.XMM1)
	switch op {
	case "lt":
		e.Setcc(amd64.CondB, amd64.RAX)
		e.Setcc(amd64.CondNP, amd64.R10)
		e.AndRegReg(amd64.RAX, amd64.R10)
	case "le":
		e.Setcc(amd64.CondBE, amd64.RAX)
		e.Setcc(amd64.CondNP, amd64.R10)
		e.AndRegReg(amd64.RAX, amd64.R10)
	case "gt":
		e.Setcc(amd64.CondA, amd64.RAX)
	case "ge":
		e.Setcc(amd64.CondAE, amd64.RAX)
	}

	done := len(e.Code)
	patchJmp(stringDone, done)
	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.AddRegImm32(amd64.RSP, 40)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}
