package lower

import "github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"

// A JSValue cell owns one NaN-boxed value in GC-aware storage. Use this for
// runtime state whose WebIDL type is `any` instead of placing tagged values in
// generic raw-reference fields, which the object tracer cannot decode safely.
func emitAMD64JSValueCellNew(e *amd64.Emitter, allocOffset int) {
	e.MovRegImm64(amd64.RDI, 8)
	call := len(e.Code)
	e.CallRel32(int32(allocOffset - (call + 5)))
	emitAMD64SetObjectType(e, amd64.RAX, amd64ObjectTypeJSValueData)
	e.MovRegImm64(amd64.R10, amd64UndefinedBits)
	e.MovDerefReg(amd64.RAX, 0, amd64.R10)
	e.Ret()
}

func emitAMD64JSValueCellGet(e *amd64.Emitter) {
	e.MovRegDeref(amd64.RAX, amd64.RDI, 0)
	e.Ret()
}

func emitAMD64JSValueCellSet(e *amd64.Emitter) {
	e.MovDerefReg(amd64.RDI, 0, amd64.RSI)
	e.Ret()
}
