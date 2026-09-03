package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

func emitAMD64CallLiteralString(e *amd64.Emitter, allocOffset int, text string) {
	e.MovRegImm64(amd64.RDI, int64(8+len(text)))
	callAt := len(e.Code)
	e.CallRel32(int32(allocOffset - (callAt + 5)))
	e.MovRegImm64(amd64.R10, int64(len(text)))
	e.MovDerefReg(amd64.RAX, 0, amd64.R10)
	for i, ch := range []byte(text) {
		e.MovRegImm64(amd64.R10, int64(ch))
		e.MovDerefReg8(amd64.RAX, int32(8+i), amd64.R10)
	}
}

func emitAMD64JSArrayToString(e *amd64.Emitter, selfOffset, allocOffset, numberToStringOffset, boolToStringOffset, stringConcatOffset int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	call := func(target int) { at := len(e.Code); e.CallRel32(int32(target - (at + 5))) }

	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.SubRegImm32(amd64.RSP, 32)
	e.MovRegReg(amd64.RBX, amd64.RDI)

	// Root the array and the evolving result across all formatter allocations.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 2)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovRegImm64(amd64.R10, 0)
	e.MovDerefReg(amd64.RSP, 24, amd64.R10)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	emitAMD64CallLiteralString(e, allocOffset, "")
	e.MovRegReg(amd64.R12, amd64.RAX)
	e.MovDerefReg(amd64.RSP, 24, amd64.R12)
	e.MovRegImm64(amd64.R13, 0)
	e.MovRegDeref(amd64.R14, amd64.RBX, amd64ArrayLength)

	loop := len(e.Code)
	e.CmpRegReg(amd64.R13, amd64.R14)
	doneJump := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)

	// Add a comma before every element except the first.
	e.TestRegReg(amd64.R13, amd64.R13)
	first := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	emitAMD64CallLiteralString(e, allocOffset, ",")
	e.MovRegReg(amd64.RDI, amd64.R12)
	e.MovRegReg(amd64.RSI, amd64.RAX)
	call(stringConcatOffset)
	e.MovRegReg(amd64.R12, amd64.RAX)
	e.MovDerefReg(amd64.RSP, 24, amd64.R12)
	afterComma := len(e.Code)
	patchJcc(first, afterComma)

	// Load the raw element payload.
	e.MovRegDeref(amd64.R8, amd64.RBX, amd64ArrayData)
	e.MovRegReg(amd64.R9, amd64.R13)
	e.AddRegReg(amd64.R9, amd64.R9)
	e.AddRegReg(amd64.R9, amd64.R9)
	e.AddRegReg(amd64.R9, amd64.R9)
	e.AddRegReg(amd64.R8, amd64.R9)
	e.MovRegDeref(amd64.R9, amd64.R8, 0)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64ArrayElemKind)
	e.CmpRegImm32(amd64.R10, 0)
	scalar := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.CmpRegImm32(amd64.R10, 1)
	ref := len(e.Code)
	e.JccRel32(amd64.CondE, 0)

	// JSValue element: handle the primitive cases directly.
	e.MovRegReg(amd64.R10, amd64.R9)
	e.MovRegImm64(amd64.R11, amd64JSTagMask)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.MovRegImm64(amd64.R11, amd64JSStringTag)
	e.CmpRegReg(amd64.R10, amd64.R11)
	jsString := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegImm64(amd64.R11, amd64JSBoolTag)
	e.CmpRegReg(amd64.R10, amd64.R11)
	jsBool := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegImm64(amd64.R11, amd64JSRefTag)
	e.CmpRegReg(amd64.R10, amd64.R11)
	jsRef := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegImm64(amd64.R10, amd64UndefinedBits)
	e.CmpRegReg(amd64.R9, amd64.R10)
	jsUndefined := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegImm64(amd64.R10, amd64NullBits)
	e.CmpRegReg(amd64.R9, amd64.R10)
	jsNull := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	// Remaining JSValue payload is a raw number.
	e.MovQXMMReg(amd64.XMM0, amd64.R9)
	call(numberToStringOffset)
	jsNumberDone := len(e.Code)
	e.JmpRel32(0)

	jsStringLabel := len(e.Code)
	patchJcc(jsString, jsStringLabel)
	e.MovRegReg(amd64.RAX, amd64.R9)
	e.MovRegImm64(amd64.R10, amd64JSPayloadMask)
	e.AndRegReg(amd64.RAX, amd64.R10)
	jsStringDone := len(e.Code)
	e.JmpRel32(0)

	jsBoolLabel := len(e.Code)
	patchJcc(jsBool, jsBoolLabel)
	e.MovRegReg(amd64.RDI, amd64.R9)
	e.MovRegImm64(amd64.R10, 1)
	e.AndRegReg(amd64.RDI, amd64.R10)
	call(boolToStringOffset)
	jsBoolDone := len(e.Code)
	e.JmpRel32(0)

	jsRefLabel := len(e.Code)
	patchJcc(jsRef, jsRefLabel)
	e.MovRegReg(amd64.RDI, amd64.R9)
	e.MovRegImm64(amd64.R10, amd64JSPayloadMask)
	e.AndRegReg(amd64.RDI, amd64.R10)
	// Arrays recurse; other heap references use the default object string.
	e.MovRegReg(amd64.R10, amd64.RDI)
	e.SubRegImm32(amd64.R10, amd64ObjectHeaderSize)
	e.MovRegDeref(amd64.R11, amd64.R10, amd64ObjectType)
	e.CmpRegImm32(amd64.R11, int32(amd64ObjectTypeArray))
	jsRefObject := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	call(selfOffset)
	jsRefDone := len(e.Code)
	e.JmpRel32(0)
	jsRefObjectLabel := len(e.Code)
	patchJcc(jsRefObject, jsRefObjectLabel)
	emitAMD64CallLiteralString(e, allocOffset, "[object Object]")
	jsRefObjectDone := len(e.Code)
	e.JmpRel32(0)

	jsUndefinedLabel := len(e.Code)
	patchJcc(jsUndefined, jsUndefinedLabel)
	emitAMD64CallLiteralString(e, allocOffset, "undefined")
	jsUndefinedDone := len(e.Code)
	e.JmpRel32(0)
	jsNullLabel := len(e.Code)
	patchJcc(jsNull, jsNullLabel)
	emitAMD64CallLiteralString(e, allocOffset, "null")
	jsNullDone := len(e.Code)
	e.JmpRel32(0)

	scalarLabel := len(e.Code)
	patchJcc(scalar, scalarLabel)
	e.MovQXMMReg(amd64.XMM0, amd64.R9)
	call(numberToStringOffset)
	scalarDone := len(e.Code)
	e.JmpRel32(0)

	refLabel := len(e.Code)
	patchJcc(ref, refLabel)
	// Atomic reference payloads are native strings; arrays recurse; other refs
	// use the default object coercion string.
	e.MovRegReg(amd64.RDI, amd64.R9)
	e.MovRegReg(amd64.R10, amd64.R9)
	e.SubRegImm32(amd64.R10, amd64ObjectHeaderSize)
	e.MovRegDeref(amd64.R11, amd64.R10, amd64ObjectType)
	e.CmpRegImm32(amd64.R11, int32(amd64ObjectTypeAtomic))
	rawRefNotString := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegReg(amd64.RAX, amd64.R9)
	rawStringDone := len(e.Code)
	e.JmpRel32(0)
	rawRefOther := len(e.Code)
	patchJcc(rawRefNotString, rawRefOther)
	e.CmpRegImm32(amd64.R11, int32(amd64ObjectTypeArray))
	rawRefObject := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	call(selfOffset)
	rawArrayDone := len(e.Code)
	e.JmpRel32(0)
	rawRefObjectLabel := len(e.Code)
	patchJcc(rawRefObject, rawRefObjectLabel)
	emitAMD64CallLiteralString(e, allocOffset, "[object Object]")

	elementReady := len(e.Code)
	for _, at := range []int{jsNumberDone, jsStringDone, jsBoolDone, jsRefDone, jsRefObjectDone, jsUndefinedDone, jsNullDone, scalarDone, rawStringDone, rawArrayDone} {
		patchJmp(at, elementReady)
	}
	// RAX is the element string. Append it to the rooted result.
	e.MovRegReg(amd64.RDI, amd64.R12)
	e.MovRegReg(amd64.RSI, amd64.RAX)
	call(stringConcatOffset)
	e.MovRegReg(amd64.R12, amd64.RAX)
	e.MovDerefReg(amd64.RSP, 24, amd64.R12)
	e.AddRegImm32(amd64.R13, 1)
	back := len(e.Code)
	e.JmpRel32(0)
	patchJmp(back, loop)

	done := len(e.Code)
	patchJcc(doneJump, done)
	e.MovRegReg(amd64.RAX, amd64.R12)
	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.AddRegImm32(amd64.RSP, 32)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64JSToString(e *amd64.Emitter, allocOffset, numberToStringOffset, boolToStringOffset, arrayToStringOffset int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.SubRegImm32(amd64.RSP, 8)
	e.MovRegReg(amd64.RBX, amd64.RDI)

	e.MovRegImm64(amd64.R10, amd64UndefinedBits)
	e.CmpRegReg(amd64.RBX, amd64.R10)
	isUndefined := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegImm64(amd64.R10, amd64NullBits)
	e.CmpRegReg(amd64.RBX, amd64.R10)
	isNull := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegReg(amd64.R10, amd64.RBX)
	e.MovRegImm64(amd64.R11, amd64JSTagMask)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.MovRegImm64(amd64.R11, amd64JSStringTag)
	e.CmpRegReg(amd64.R10, amd64.R11)
	isString := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegImm64(amd64.R11, amd64JSBoolTag)
	e.CmpRegReg(amd64.R10, amd64.R11)
	isBool := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegImm64(amd64.R11, amd64JSRefTag)
	e.CmpRegReg(amd64.R10, amd64.R11)
	isRef := len(e.Code)
	e.JccRel32(amd64.CondE, 0)

	// Raw number.
	e.MovQXMMReg(amd64.XMM0, amd64.RBX)
	callNum := len(e.Code)
	e.CallRel32(int32(numberToStringOffset - (callNum + 5)))
	doneJump := len(e.Code)
	e.JmpRel32(0)

	stringLabel := len(e.Code)
	patchJcc(isString, stringLabel)
	e.MovRegReg(amd64.RAX, amd64.RBX)
	e.MovRegImm64(amd64.R10, amd64JSPayloadMask)
	e.AndRegReg(amd64.RAX, amd64.R10)
	stringDone := len(e.Code)
	e.JmpRel32(0)

	boolLabel := len(e.Code)
	patchJcc(isBool, boolLabel)
	e.MovRegReg(amd64.RDI, amd64.RBX)
	e.MovRegImm64(amd64.R10, 1)
	e.AndRegReg(amd64.RDI, amd64.R10)
	callBool := len(e.Code)
	e.CallRel32(int32(boolToStringOffset - (callBool + 5)))
	boolDone := len(e.Code)
	e.JmpRel32(0)

	refLabel := len(e.Code)
	patchJcc(isRef, refLabel)
	e.MovRegReg(amd64.RDI, amd64.RBX)
	e.MovRegImm64(amd64.R10, amd64JSPayloadMask)
	e.AndRegReg(amd64.RDI, amd64.R10)
	e.MovRegReg(amd64.R10, amd64.RDI)
	e.SubRegImm32(amd64.R10, amd64ObjectHeaderSize)
	e.MovRegDeref(amd64.R11, amd64.R10, amd64ObjectType)
	e.CmpRegImm32(amd64.R11, int32(amd64ObjectTypeArray))
	refObject := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	callArray := len(e.Code)
	e.CallRel32(int32(arrayToStringOffset - (callArray + 5)))
	refArrayDone := len(e.Code)
	e.JmpRel32(0)
	refObjectLabel := len(e.Code)
	patchJcc(refObject, refObjectLabel)
	emitAMD64CallLiteralString(e, allocOffset, "[object Object]")
	refObjectDone := len(e.Code)
	e.JmpRel32(0)

	undefinedLabel := len(e.Code)
	patchJcc(isUndefined, undefinedLabel)
	emitAMD64CallLiteralString(e, allocOffset, "undefined")
	undefinedDone := len(e.Code)
	e.JmpRel32(0)
	nullLabel := len(e.Code)
	patchJcc(isNull, nullLabel)
	emitAMD64CallLiteralString(e, allocOffset, "null")

	done := len(e.Code)
	for _, at := range []int{doneJump, stringDone, boolDone, refArrayDone, refObjectDone, undefinedDone} {
		binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(done-(at+5))))
	}
	e.AddRegImm32(amd64.RSP, 8)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64JSAdd(e *amd64.Emitter, toStringOffset, stringConcatOffset, boxNumberOffset int) {
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

	// Root both JSValues and the temporary left string.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 3)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.RSP, 24, amd64.R12)
	e.MovRegImm64(amd64.R10, 0)
	e.MovDerefReg(amd64.RSP, 32, amd64.R10)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	// If either ToPrimitive input is a string/reference, + concatenates strings.
	var stringJumps []int
	for _, reg := range []amd64.Register{amd64.RBX, amd64.R12} {
		e.MovRegReg(amd64.R10, reg)
		e.MovRegImm64(amd64.R11, amd64JSTagMask)
		e.AndRegReg(amd64.R10, amd64.R11)
		for _, tag := range []int64{amd64JSStringTag, amd64JSRefTag} {
			e.MovRegImm64(amd64.R11, tag)
			e.CmpRegReg(amd64.R10, amd64.R11)
			at := len(e.Code)
			e.JccRel32(amd64.CondE, 0)
			stringJumps = append(stringJumps, at)
		}
	}

	// Numeric ToNumber path for number/bool/null/undefined.
	toNumber := func(src amd64.Register, dst amd64.XMMRegister) {
		e.MovRegImm64(amd64.R10, amd64UndefinedBits)
		e.CmpRegReg(src, amd64.R10)
		undef := len(e.Code)
		e.JccRel32(amd64.CondE, 0)
		e.MovRegImm64(amd64.R10, amd64NullBits)
		e.CmpRegReg(src, amd64.R10)
		nullJump := len(e.Code)
		e.JccRel32(amd64.CondE, 0)
		e.MovRegReg(amd64.R10, src)
		e.MovRegImm64(amd64.R11, amd64JSTagMask)
		e.AndRegReg(amd64.R10, amd64.R11)
		e.MovRegImm64(amd64.R11, amd64JSBoolTag)
		e.CmpRegReg(amd64.R10, amd64.R11)
		boolJump := len(e.Code)
		e.JccRel32(amd64.CondE, 0)
		e.MovQXMMReg(dst, src)
		done := len(e.Code)
		e.JmpRel32(0)
		undefLabel := len(e.Code)
		patchJcc(undef, undefLabel)
		e.MovRegImm64(amd64.R10, amd64JSNumberNaN)
		e.MovQXMMReg(dst, amd64.R10)
		undefDone := len(e.Code)
		e.JmpRel32(0)
		nullLabel := len(e.Code)
		patchJcc(nullJump, nullLabel)
		e.MovRegImm64(amd64.R10, 0)
		e.Cvtsi2sd(dst, amd64.R10)
		nullDone := len(e.Code)
		e.JmpRel32(0)
		boolLabel := len(e.Code)
		patchJcc(boolJump, boolLabel)
		e.MovRegReg(amd64.R10, src)
		e.MovRegImm64(amd64.R11, 1)
		e.AndRegReg(amd64.R10, amd64.R11)
		e.Cvtsi2sd(dst, amd64.R10)
		end := len(e.Code)
		patchJmp(done, end)
		patchJmp(undefDone, end)
		patchJmp(nullDone, end)
	}
	toNumber(amd64.RBX, amd64.XMM0)
	toNumber(amd64.R12, amd64.XMM1)
	e.AddSD(amd64.XMM0, amd64.XMM1)
	call(boxNumberOffset)
	numericDone := len(e.Code)
	e.JmpRel32(0)

	stringLabel := len(e.Code)
	for _, at := range stringJumps {
		patchJcc(at, stringLabel)
	}
	e.MovRegReg(amd64.RDI, amd64.RBX)
	call(toStringOffset)
	e.MovRegReg(amd64.R13, amd64.RAX)
	e.MovDerefReg(amd64.RSP, 32, amd64.R13)
	e.MovRegReg(amd64.RDI, amd64.R12)
	call(toStringOffset)
	e.MovRegReg(amd64.RDI, amd64.R13)
	e.MovRegReg(amd64.RSI, amd64.RAX)
	call(stringConcatOffset)
	// Box the native string result.
	e.MovRegImm64(amd64.R10, amd64JSPayloadMask)
	e.AndRegReg(amd64.RAX, amd64.R10)
	e.MovRegImm64(amd64.R10, amd64JSStringTag)
	e.OrRegReg(amd64.RAX, amd64.R10)

	done := len(e.Code)
	patchJmp(numericDone, done)
	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.AddRegImm32(amd64.RSP, 40)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64JSStringToNumber(e *amd64.Emitter, jsonParseOffset int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.SubRegImm32(amd64.RSP, 8)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegDeref(amd64.R9, amd64.RBX, 0)
	e.MovRegReg(amd64.R8, amd64.RBX)
	e.AddRegImm32(amd64.R8, 8)

	spaceLoop := len(e.Code)
	e.TestRegReg(amd64.R9, amd64.R9)
	empty := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovzxRegDeref8(amd64.RAX, amd64.R8, 0)
	var whitespace []int
	for _, ch := range []int32{' ', '\t', '\n', '\r'} {
		e.CmpRegImm32(amd64.RAX, ch)
		at := len(e.Code)
		e.JccRel32(amd64.CondE, 0)
		whitespace = append(whitespace, at)
	}
	spaceDoneJump := len(e.Code)
	e.JmpRel32(0)
	consume := len(e.Code)
	for _, at := range whitespace {
		patchJcc(at, consume)
	}
	e.AddRegImm32(amd64.R8, 1)
	e.SubRegImm32(amd64.R9, 1)
	back := len(e.Code)
	e.JmpRel32(0)
	patchJmp(back, spaceLoop)
	spaceDone := len(e.Code)
	patchJmp(spaceDoneJump, spaceDone)

	// The JSON scalar parser already handles signed decimal syntax. Guard it so
	// nonnumeric JavaScript strings become NaN instead of JSON keyword values.
	e.CmpRegImm32(amd64.RAX, '-')
	parse := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.CmpRegImm32(amd64.RAX, '0')
	invalidLow := len(e.Code)
	e.JccRel32(amd64.CondB, 0)
	e.CmpRegImm32(amd64.RAX, '9')
	invalidHigh := len(e.Code)
	e.JccRel32(amd64.CondA, 0)
	parseLabel := len(e.Code)
	patchJcc(parse, parseLabel)
	e.MovRegReg(amd64.RDI, amd64.RBX)
	callParse := len(e.Code)
	e.CallRel32(int32(jsonParseOffset - (callParse + 5)))
	parseDone := len(e.Code)
	e.JmpRel32(0)

	invalid := len(e.Code)
	patchJcc(invalidLow, invalid)
	patchJcc(invalidHigh, invalid)
	e.MovRegImm64(amd64.RAX, amd64JSNumberNaN)
	e.MovQXMMReg(amd64.XMM0, amd64.RAX)
	invalidDone := len(e.Code)
	e.JmpRel32(0)

	emptyLabel := len(e.Code)
	patchJcc(empty, emptyLabel)
	e.MovRegImm64(amd64.RAX, 0)
	e.Cvtsi2sd(amd64.XMM0, amd64.RAX)

	done := len(e.Code)
	patchJmp(parseDone, done)
	patchJmp(invalidDone, done)
	e.AddRegImm32(amd64.RSP, 8)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64JSToNumber(e *amd64.Emitter, stringToNumberOffset, arrayToStringOffset int) {
	patchJcc := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6)))) }
	patchJmp := func(at, target int) { binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5)))) }
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.SubRegImm32(amd64.RSP, 8)
	e.MovRegReg(amd64.RBX, amd64.RDI)

	e.MovRegImm64(amd64.R10, amd64UndefinedBits)
	e.CmpRegReg(amd64.RBX, amd64.R10)
	isUndefined := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegImm64(amd64.R10, amd64NullBits)
	e.CmpRegReg(amd64.RBX, amd64.R10)
	isNull := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.MovRegReg(amd64.R10, amd64.RBX)
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

	// Raw number.
	e.MovQXMMReg(amd64.XMM0, amd64.RBX)
	numberDone := len(e.Code)
	e.JmpRel32(0)

	boolLabel := len(e.Code)
	patchJcc(isBool, boolLabel)
	e.MovRegReg(amd64.RAX, amd64.RBX)
	e.MovRegImm64(amd64.R10, 1)
	e.AndRegReg(amd64.RAX, amd64.R10)
	e.Cvtsi2sd(amd64.XMM0, amd64.RAX)
	boolDone := len(e.Code)
	e.JmpRel32(0)

	stringLabel := len(e.Code)
	patchJcc(isString, stringLabel)
	e.MovRegReg(amd64.RDI, amd64.RBX)
	e.MovRegImm64(amd64.R10, amd64JSPayloadMask)
	e.AndRegReg(amd64.RDI, amd64.R10)
	callString := len(e.Code)
	e.CallRel32(int32(stringToNumberOffset - (callString + 5)))
	stringDone := len(e.Code)
	e.JmpRel32(0)

	refLabel := len(e.Code)
	patchJcc(isRef, refLabel)
	e.MovRegReg(amd64.RDI, amd64.RBX)
	e.MovRegImm64(amd64.R10, amd64JSPayloadMask)
	e.AndRegReg(amd64.RDI, amd64.R10)
	e.MovRegReg(amd64.R10, amd64.RDI)
	e.SubRegImm32(amd64.R10, amd64ObjectHeaderSize)
	e.MovRegDeref(amd64.R11, amd64.R10, amd64ObjectType)
	e.CmpRegImm32(amd64.R11, int32(amd64ObjectTypeArray))
	refNotArray := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	callArray := len(e.Code)
	e.CallRel32(int32(arrayToStringOffset - (callArray + 5)))
	e.MovRegReg(amd64.RDI, amd64.RAX)
	callArrayNumber := len(e.Code)
	e.CallRel32(int32(stringToNumberOffset - (callArrayNumber + 5)))
	refDone := len(e.Code)
	e.JmpRel32(0)

	refNotArrayLabel := len(e.Code)
	patchJcc(refNotArray, refNotArrayLabel)
	e.MovRegImm64(amd64.RAX, amd64JSNumberNaN)
	e.MovQXMMReg(amd64.XMM0, amd64.RAX)
	refObjectDone := len(e.Code)
	e.JmpRel32(0)

	undefinedLabel := len(e.Code)
	patchJcc(isUndefined, undefinedLabel)
	e.MovRegImm64(amd64.RAX, amd64JSNumberNaN)
	e.MovQXMMReg(amd64.XMM0, amd64.RAX)
	undefinedDone := len(e.Code)
	e.JmpRel32(0)

	nullLabel := len(e.Code)
	patchJcc(isNull, nullLabel)
	e.MovRegImm64(amd64.RAX, 0)
	e.Cvtsi2sd(amd64.XMM0, amd64.RAX)

	done := len(e.Code)
	for _, at := range []int{numberDone, boolDone, stringDone, refDone, refObjectDone, undefinedDone} {
		patchJmp(at, done)
	}
	e.MovQRegXMM(amd64.RAX, amd64.XMM0)
	e.AddRegImm32(amd64.RSP, 8)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64JSNumericBinary(e *amd64.Emitter, toNumberOffset int, op string) {
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.SubRegImm32(amd64.RSP, 8)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)
	callLeft := len(e.Code)
	e.CallRel32(int32(toNumberOffset - (callLeft + 5)))
	e.MovQRegXMM(amd64.R13, amd64.XMM0)
	e.MovRegReg(amd64.RDI, amd64.R12)
	callRight := len(e.Code)
	e.CallRel32(int32(toNumberOffset - (callRight + 5)))
	e.MovSDRegReg(amd64.XMM1, amd64.XMM0)
	e.MovQXMMReg(amd64.XMM0, amd64.R13)
	switch op {
	case "sub":
		e.SubSD(amd64.XMM0, amd64.XMM1)
	case "mul":
		e.MulSD(amd64.XMM0, amd64.XMM1)
	case "div":
		e.DivSD(amd64.XMM0, amd64.XMM1)
	case "mod":
		e.MovSDRegReg(amd64.XMM2, amd64.XMM0)
		e.DivSD(amd64.XMM2, amd64.XMM1)
		e.Cvttsd2si(amd64.RAX, amd64.XMM2)
		e.Cvtsi2sd(amd64.XMM2, amd64.RAX)
		e.MulSD(amd64.XMM2, amd64.XMM1)
		e.SubSD(amd64.XMM0, amd64.XMM2)
	}
	e.MovQRegXMM(amd64.RAX, amd64.XMM0)
	e.AddRegImm32(amd64.RSP, 8)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

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
