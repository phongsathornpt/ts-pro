package lower

import (
	"encoding/binary"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
)

func emitAMD64StructuredCloneDynamicRuntimeSymbols(e *amd64.Emitter, fnOffsets map[string]int) {
	fnOffsets["ts_byte_buffer_transfer"] = len(e.Code)
	emitAMD64ByteBufferTransfer(e, fnOffsets["ts_object_new"])
	fnOffsets["ts_js_is_dynamic_object"] = len(e.Code)
	emitAMD64JSIsDynamicObject(e)
	fnOffsets["ts_dynamic_count"] = len(e.Code)
	emitAMD64DynamicCount(e)
	fnOffsets["ts_dynamic_key_at"] = len(e.Code)
	emitAMD64DynamicKeyAt(e)
	fnOffsets["ts_dynamic_value_at"] = len(e.Code)
	emitAMD64DynamicValueAt(e)
}

func emitAMD64ByteBufferTransfer(e *amd64.Emitter, objectNewOffset int) {
	// RDI = source byte-buffer wrapper. Move its raw backing allocation into a
	// fresh wrapper, then detach the old wrapper in-place. Existing typed-array
	// views retain the old wrapper and therefore immediately observe length 0.
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.SubRegImm32(amd64.RSP, 32)
	e.MovRegReg(amd64.RBX, amd64.RDI)

	// ts_object_new may collect. Root the source wrapper until ownership has
	// been installed in the destination wrapper.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 1)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	e.MovRegImm64(amd64.RDI, 3)
	e.MovRegImm64(amd64.RSI, 0b001)
	callObject := len(e.Code)
	e.CallRel32(int32(objectNewOffset - (callObject + 5)))

	e.MovRegDeref(amd64.R10, amd64.RBX, amd64ByteBufferData)
	e.MovDerefReg(amd64.RAX, amd64ByteBufferData, amd64.R10)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64ByteBufferLength)
	e.MovDerefReg(amd64.RAX, amd64ByteBufferLength, amd64.R10)
	e.MovRegDeref(amd64.R10, amd64.RBX, amd64ByteBufferCapacity)
	e.MovDerefReg(amd64.RAX, amd64ByteBufferCapacity, amd64.R10)

	// Detach source wrapper. Leaving stale payload metadata around would make
	// old views appear live even though ownership has moved.
	e.MovRegImm64(amd64.R10, 0)
	e.MovDerefReg(amd64.RBX, amd64ByteBufferData, amd64.R10)
	e.MovDerefReg(amd64.RBX, amd64ByteBufferLength, amd64.R10)
	e.MovDerefReg(amd64.RBX, amd64ByteBufferCapacity, amd64.R10)

	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.AddRegImm32(amd64.RSP, 32)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64DynamicObjectPayloadOrJump(e *amd64.Emitter, invalidJumps *[]int) {
	e.MovRegReg(amd64.R10, amd64.RDI)
	e.MovRegImm64(amd64.R11, amd64JSTagMask)
	e.AndRegReg(amd64.R10, amd64.R11)
	e.MovRegImm64(amd64.R11, amd64JSRefTag)
	e.CmpRegReg(amd64.R10, amd64.R11)
	tagged := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.TestRegReg(amd64.R10, amd64.R10)
	raw := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	*invalidJumps = append(*invalidJumps, len(e.Code))
	e.JmpRel32(0)

	taggedLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[tagged+2:], uint32(int32(taggedLabel-(tagged+6))))
	e.MovRegImm64(amd64.R10, amd64JSPayloadMask)
	e.AndRegReg(amd64.RDI, amd64.R10)
	readyJump := len(e.Code)
	e.JmpRel32(0)

	rawLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[raw+2:], uint32(int32(rawLabel-(raw+6))))
	ready := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[readyJump+1:], uint32(int32(ready-(readyJump+5))))
	e.TestRegReg(amd64.RDI, amd64.RDI)
	nullJump := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	*invalidJumps = append(*invalidJumps, nullJump)
	e.MovRegReg(amd64.R10, amd64.RDI)
	e.SubRegImm32(amd64.R10, amd64ObjectHeaderSize)
	e.MovRegDeref(amd64.R11, amd64.R10, amd64ObjectType)
	e.CmpRegImm32(amd64.R11, int32(amd64ObjectTypeDynamicObject))
	notDynamic := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	*invalidJumps = append(*invalidJumps, notDynamic)
}

func patchAMD64StructuredCloneInvalidJumps(e *amd64.Emitter, jumps []int, target int) {
	for _, at := range jumps {
		if e.Code[at] == 0x0f {
			binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6))))
		} else {
			binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5))))
		}
	}
}

func emitAMD64JSIsDynamicObject(e *amd64.Emitter) {
	var invalid []int
	emitAMD64DynamicObjectPayloadOrJump(e, &invalid)
	e.MovRegImm64(amd64.RAX, 1)
	doneJump := len(e.Code)
	e.JmpRel32(0)
	invalidLabel := len(e.Code)
	patchAMD64StructuredCloneInvalidJumps(e, invalid, invalidLabel)
	e.MovRegImm64(amd64.RAX, 0)
	done := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[doneJump+1:], uint32(int32(done-(doneJump+5))))
	e.Ret()
}

func emitAMD64DynamicCount(e *amd64.Emitter) {
	var invalid []int
	emitAMD64DynamicObjectPayloadOrJump(e, &invalid)
	e.MovRegDeref(amd64.RAX, amd64.RDI, amd64DynamicCount)
	e.Cvtsi2sd(amd64.XMM0, amd64.RAX)
	doneJump := len(e.Code)
	e.JmpRel32(0)
	invalidLabel := len(e.Code)
	patchAMD64StructuredCloneInvalidJumps(e, invalid, invalidLabel)
	e.MovRegImm64(amd64.RAX, 0)
	e.Cvtsi2sd(amd64.XMM0, amd64.RAX)
	done := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[doneJump+1:], uint32(int32(done-(doneJump+5))))
	e.Ret()
}

func emitAMD64DynamicKeyAt(e *amd64.Emitter) {
	emitAMD64DynamicEntryAt(e, true)
}

func emitAMD64DynamicValueAt(e *amd64.Emitter) {
	emitAMD64DynamicEntryAt(e, false)
}

func emitAMD64DynamicEntryAt(e *amd64.Emitter, wantKey bool) {
	var invalid []int
	emitAMD64DynamicObjectPayloadOrJump(e, &invalid)
	e.Cvttsd2si(amd64.RSI, amd64.XMM0)
	e.TestRegReg(amd64.RSI, amd64.RSI)
	negative := len(e.Code)
	e.JccRel32(amd64.CondL, 0)
	invalid = append(invalid, negative)
	e.MovRegDeref(amd64.R10, amd64.RDI, amd64DynamicCount)
	e.CmpRegReg(amd64.RSI, amd64.R10)
	outOfRange := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	invalid = append(invalid, outOfRange)

	e.MovRegDeref(amd64.R10, amd64.RDI, amd64DynamicEntries)
	e.MovRegDeref(amd64.R11, amd64.RDI, amd64DynamicCapacity)
	e.MovRegImm64(amd64.RAX, 0) // physical slot
	e.MovRegImm64(amd64.RDX, 0) // logical occupied index

	loop := len(e.Code)
	e.CmpRegReg(amd64.RAX, amd64.R11)
	exhausted := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	invalid = append(invalid, exhausted)
	emitAMD64DynamicEntryAddress(e, amd64.RCX, amd64.R10, amd64.RAX, amd64.R8)
	e.MovRegDeref(amd64.R8, amd64.RCX, amd64DynamicEntryKey)
	e.TestRegReg(amd64.R8, amd64.R8)
	empty := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.CmpRegReg(amd64.RDX, amd64.RSI)
	found := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.AddRegImm32(amd64.RDX, 1)
	next := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[empty+2:], uint32(int32(next-(empty+6))))
	e.AddRegImm32(amd64.RAX, 1)
	back := len(e.Code)
	e.JmpRel32(0)
	binary.LittleEndian.PutUint32(e.Code[back+1:], uint32(int32(loop-(back+5))))

	foundLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[found+2:], uint32(int32(foundLabel-(found+6))))
	if wantKey {
		e.MovRegDeref(amd64.RAX, amd64.RCX, amd64DynamicEntryKey)
	} else {
		e.MovRegDeref(amd64.RAX, amd64.RCX, amd64DynamicEntryValue)
	}
	doneJump := len(e.Code)
	e.JmpRel32(0)

	invalidLabel := len(e.Code)
	patchAMD64StructuredCloneInvalidJumps(e, invalid, invalidLabel)
	if wantKey {
		e.MovRegImm64(amd64.RAX, 0)
	} else {
		e.MovRegImm64(amd64.RAX, amd64UndefinedBits)
	}
	done := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[doneJump+1:], uint32(int32(done-(doneJump+5))))
	e.Ret()
}
