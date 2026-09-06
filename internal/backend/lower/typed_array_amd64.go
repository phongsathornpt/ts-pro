package lower

import "github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"

func emitAMD64ArrayBufferWrap(e *amd64.Emitter, objectNewOffset int) {
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.MovRegReg(amd64.RBX, amd64.RDI)

	e.MovRegImm64(amd64.RDI, 1)
	e.MovRegImm64(amd64.RSI, 1)
	callAt := len(e.Code)
	e.CallRel32(int32(objectNewOffset - (callAt + 5)))
	e.MovDerefReg(amd64.RAX, amd64ObjectFields, amd64.RBX)

	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64Uint8ArrayWrap(e *amd64.Emitter, objectNewOffset int) {
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)
	e.MovQRegXMM(amd64.R13, amd64.XMM0)
	e.MovQRegXMM(amd64.R14, amd64.XMM1)

	e.MovRegImm64(amd64.RDI, 5)
	e.MovRegImm64(amd64.RSI, 0b00011)
	callAt := len(e.Code)
	e.CallRel32(int32(objectNewOffset - (callAt + 5)))
	e.MovDerefReg(amd64.RAX, amd64ObjectFields+0, amd64.RBX)
	e.MovDerefReg(amd64.RAX, amd64ObjectFields+8, amd64.R12)
	// Builtin object layouts are sorted by field name: $data, buffer,
	// byteLength, byteOffset, length.
	e.MovDerefReg(amd64.RAX, amd64ObjectFields+16, amd64.R14)
	e.MovDerefReg(amd64.RAX, amd64ObjectFields+24, amd64.R13)
	e.MovDerefReg(amd64.RAX, amd64ObjectFields+32, amd64.R14)

	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}
