package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

const (
	amd64JSStringTag   int64 = 0x7ff9000000000000
	amd64JSBoolTag     int64 = 0x7ffa000000000000
	amd64JSRefTag      int64 = 0x7ffb000000000000
	amd64JSTagMask     int64 = -281474976710656 // 0xffff000000000000
	amd64JSPayloadMask int64 = 0x0000ffffffffffff
	amd64JSNumberNaN   int64 = 0x7ff8000000000000
)

func amd64JSValueType(t types.Type) bool {
	if t == nil {
		return false
	}
	if t.Kind() == types.KindAny || t.Kind() == types.KindUnknown {
		return true
	}
	u, ok := t.(*types.UnionType)
	if !ok {
		return false
	}
	classes := map[int]bool{}
	for _, member := range u.Members {
		switch member.Kind() {
		case types.KindNull, types.KindUndefined, types.KindNever:
			continue
		case types.KindNumber:
			classes[1] = true
		case types.KindBoolean:
			classes[2] = true
		case types.KindString, types.KindArray, types.KindTuple, types.KindObject, types.KindFunction:
			classes[3] = true
		case types.KindAny, types.KindUnknown:
			return true
		default:
			classes[4] = true
		}
	}
	return len(classes) > 1
}

func emitAMD64JSBoxNumber(e *amd64.Emitter) {
	// Numbers remain raw IEEE-754 payloads. Canonicalize NaN so reserved NaN
	// payloads can never be mistaken for tagged JavaScript values.
	e.MovQRegXMM(amd64.RAX, amd64.XMM0)
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.MovRegImm64(amd64.R11, int64(0x7ff0000000000000))
	e.AndRegReg(amd64.R10, amd64.R11)
	e.CmpRegReg(amd64.R10, amd64.R11)
	notSpecial := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.MovRegImm64(amd64.R11, int64(0x000fffffffffffff))
	e.AndRegReg(amd64.R10, amd64.R11)
	e.TestRegReg(amd64.R10, amd64.R10)
	infinity := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegImm64(amd64.RAX, amd64JSNumberNaN)
	done := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[notSpecial+2:], uint32(int32(done-(notSpecial+6))))
	binary.LittleEndian.PutUint32(e.Code[infinity+2:], uint32(int32(done-(infinity+6))))
	e.Ret()
}

func emitAMD64JSBoxBool(e *amd64.Emitter) {
	e.MovRegReg(amd64.RAX, amd64.RDI)
	e.MovRegImm64(amd64.R10, 1)
	e.AndRegReg(amd64.RAX, amd64.R10)
	e.MovRegImm64(amd64.R10, amd64JSBoolTag)
	e.OrRegReg(amd64.RAX, amd64.R10)
	e.Ret()
}

func emitAMD64JSBoxTaggedRef(e *amd64.Emitter, tag int64) {
	e.MovRegReg(amd64.RAX, amd64.RDI)
	e.MovRegImm64(amd64.R10, amd64JSPayloadMask)
	e.AndRegReg(amd64.RAX, amd64.R10)
	e.MovRegImm64(amd64.R10, tag)
	e.OrRegReg(amd64.RAX, amd64.R10)
	e.Ret()
}

func emitAMD64JSBoxString(e *amd64.Emitter) { emitAMD64JSBoxTaggedRef(e, amd64JSStringTag) }
func emitAMD64JSBoxRef(e *amd64.Emitter)    { emitAMD64JSBoxTaggedRef(e, amd64JSRefTag) }

func emitAMD64JSPrint(e *amd64.Emitter, printNumber, printString, printUndefined, printNull, printObject, printTrue, printFalse int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	call := func(target int) {
		at := len(e.Code)
		e.CallRel32(int32(target - (at + 5)))
	}

	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)

	e.MovRegImm64(amd64.R10, amd64UndefinedBits)
	e.CmpRegReg(amd64.RDI, amd64.R10)
	isUndefined := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegImm64(amd64.R10, amd64NullBits)
	e.CmpRegReg(amd64.RDI, amd64.R10)
	isNull := len(e.Code)
	e.JccRel32(amd64.CondE, 0)

	e.MovRegReg(amd64.R10, amd64.RDI)
	e.MovRegImm64(amd64.R11, amd64JSTagMask)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.MovRegImm64(amd64.R11, amd64JSBoolTag)
	e.CmpRegReg(amd64.R10, amd64.R11)
	isBool := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegImm64(amd64.R11, amd64JSStringTag)
	e.CmpRegReg(amd64.R10, amd64.R11)
	isString := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegImm64(amd64.R11, amd64JSRefTag)
	e.CmpRegReg(amd64.R10, amd64.R11)
	isRef := len(e.Code)
	e.JccRel32(amd64.CondE, 0)

	// Remaining payloads are raw numbers, including canonical numeric NaN.
	e.MovQXMMReg(amd64.XMM0, amd64.RDI)
	call(printNumber)
	numberDone := len(e.Code)
	e.JmpRel32(0)

	boolLabel := len(e.Code)
	patchJcc(isBool, boolLabel)
	e.MovRegImm64(amd64.R10, 1)
	e.AndRegReg(amd64.RDI, amd64.R10)
	e.TestRegReg(amd64.RDI, amd64.RDI)
	boolFalse := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	call(printTrue)
	boolDone := len(e.Code)
	e.JmpRel32(0)
	boolFalseLabel := len(e.Code)
	patchJcc(boolFalse, boolFalseLabel)
	call(printFalse)
	boolFalseDone := len(e.Code)
	e.JmpRel32(0)

	stringLabel := len(e.Code)
	patchJcc(isString, stringLabel)
	e.MovRegImm64(amd64.R10, amd64JSPayloadMask)
	e.AndRegReg(amd64.RDI, amd64.R10)
	call(printString)
	stringDone := len(e.Code)
	e.JmpRel32(0)

	refLabel := len(e.Code)
	patchJcc(isRef, refLabel)
	e.MovRegImm64(amd64.R10, amd64JSPayloadMask)
	e.AndRegReg(amd64.RDI, amd64.R10)
	call(printObject)
	refDone := len(e.Code)
	e.JmpRel32(0)

	undefinedLabel := len(e.Code)
	patchJcc(isUndefined, undefinedLabel)
	call(printUndefined)
	undefinedDone := len(e.Code)
	e.JmpRel32(0)

	nullLabel := len(e.Code)
	patchJcc(isNull, nullLabel)
	call(printNull)

	done := len(e.Code)
	patchJmp(numberDone, done)
	patchJmp(boolDone, done)
	patchJmp(boolFalseDone, done)
	patchJmp(stringDone, done)
	patchJmp(refDone, done)
	patchJmp(undefinedDone, done)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64JSUnboxNumber(e *amd64.Emitter) {
	e.MovQXMMReg(amd64.XMM0, amd64.RDI)
	e.Ret()
}

func emitAMD64JSUnboxBool(e *amd64.Emitter) {
	e.MovRegReg(amd64.RAX, amd64.RDI)
	e.MovRegImm64(amd64.R10, 1)
	e.AndRegReg(amd64.RAX, amd64.R10)
	e.Ret()
}

func emitAMD64JSUnboxString(e *amd64.Emitter) {
	e.MovRegReg(amd64.RAX, amd64.RDI)
	e.MovRegImm64(amd64.R10, amd64JSPayloadMask)
	e.AndRegReg(amd64.RAX, amd64.R10)
	e.Ret()
}

func emitAMD64JSUnboxRef(e *amd64.Emitter) {
	e.MovRegReg(amd64.RAX, amd64.RDI)
	e.MovRegImm64(amd64.R10, amd64JSPayloadMask)
	e.AndRegReg(amd64.RAX, amd64.R10)
	e.Ret()
}
