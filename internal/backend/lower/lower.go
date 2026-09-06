package lower

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
	"github.com/phongsathornpt/ts-pro/internal/backend/asm/arm64"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
	"github.com/phongsathornpt/ts-pro/internal/target"
)

func isNumberType(t types.Type) bool {
	if t == nil {
		return false
	}
	if t.Kind() == types.KindNumber {
		return true
	}
	u, ok := t.(*types.UnionType)
	if !ok {
		return false
	}
	hasNumber := false
	for _, m := range u.Members {
		switch m.Kind() {
		case types.KindNumber:
			hasNumber = true
		case types.KindNull, types.KindUndefined:
		default:
			return false
		}
	}
	return hasNumber
}
func numberBits(v float64) int64 { return int64(math.Float64bits(v)) }

const (
	amd64UndefinedBits int64 = 0x7ff8000000000001
	amd64NullBits      int64 = 0x7ff8000000000002
)

type Arch string

const (
	ArchAMD64 Arch = "amd64"
	ArchARM64 Arch = "arm64"
)

// Lower lowers an IR program by architecture only. It is retained for focused
// backend tests; production compilation should use LowerTarget so OS-specific
// startup/runtime code cannot be selected by architecture accidentally.
func Lower(prog *ir.Program, arch Arch) ([]byte, error) {
	switch arch {
	case ArchAMD64:
		return lowerAMD64(prog)
	case ArchARM64:
		return lowerARM64(prog)
	default:
		return nil, fmt.Errorf("unsupported architecture: %s", arch)
	}
}

// LowerTarget selects the OS/architecture-specific native lowering path.
func LowerTarget(prog *ir.Program, tgt target.Target) ([]byte, error) {
	switch {
	case tgt.OS == target.OSLinux && tgt.Arch == target.ArchAMD64:
		return lowerAMD64(prog)
	case tgt.OS == target.OSDarwin && tgt.Arch == target.ArchARM64:
		return lowerARM64(prog)
	default:
		return nil, fmt.Errorf("unsupported lowering target: %s", tgt)
	}
}

// System V AMD64 parameter registers
var amd64ParamRegs = []amd64.Register{
	amd64.RDI, amd64.RSI, amd64.RDX, amd64.RCX, amd64.R8, amd64.R9,
}

var amd64NumberParamRegs = []amd64.XMMRegister{
	amd64.XMM0, amd64.XMM1, amd64.XMM2, amd64.XMM3, amd64.XMM4, amd64.XMM5, amd64.XMM6, amd64.XMM7,
}

// Callee-saved scratch registers for AMD64 regalloc (preserved across calls)
var amd64ScratchRegs = []amd64.Register{
	// R15 is reserved as the Linux runtime-context pointer.
	amd64.RBX, amd64.R12, amd64.R13, amd64.R14,
}

// ARM64 parameter registers
var arm64ParamRegs = []arm64.Register{
	arm64.X0, arm64.X1, arm64.X2, arm64.X3, arm64.X4, arm64.X5, arm64.X6, arm64.X7,
}

// Callee-saved scratch registers for ARM64 regalloc (preserved across calls)
var arm64ScratchRegs = []arm64.Register{
	arm64.X19, arm64.X20, arm64.X21, arm64.X22, arm64.X23, arm64.X24,
}

type callFixup struct {
	offset int
	callee string
}

type branchFixupARM64 struct {
	offset   int
	targetBB *ir.BasicBlock
	isCond   bool
	condReg  arm64.Register
}

type branchFixupAMD64 struct {
	offset   int
	targetBB *ir.BasicBlock
	isCond   bool
}

type stringFixupARM64 struct {
	offset    int
	targetReg arm64.Register
	str       string
}

type stringFixupAMD64 struct {
	offset    int
	targetReg amd64.Register
	str       string
}

type closureCodeFixupAMD64 struct {
	offset   int
	function string
}

func emitAMD64PrintBool(e *amd64.Emitter, trueOffset, falseOffset int) {
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.TestRegReg(amd64.RDI, amd64.RDI)
	falseJump := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	callTrue := len(e.Code)
	e.CallRel32(int32(trueOffset - (callTrue + 5)))
	doneJump := len(e.Code)
	e.JmpRel32(0)
	falseLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[falseJump+2:], uint32(int32(falseLabel-(falseJump+6))))
	callFalse := len(e.Code)
	e.CallRel32(int32(falseOffset - (callFalse + 5)))
	done := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[doneJump+1:], uint32(int32(done-(doneJump+5))))
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64PrintLiteral(e *amd64.Emitter, text string) {
	size := ((len(text) + 15) / 16) * 16
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.SubRegImm32(amd64.RSP, int32(size))
	for i, ch := range []byte(text) {
		e.MovRegImm64(amd64.RAX, int64(ch))
		e.MovDerefReg8(amd64.RSP, int32(i), amd64.RAX)
	}
	e.MovRegImm64(amd64.RDI, 1)
	e.MovRegReg(amd64.RSI, amd64.RSP)
	e.MovRegImm64(amd64.RDX, int64(len(text)))
	e.MovRegImm64(amd64.RAX, 1)
	e.Syscall()
	e.MovRegReg(amd64.RSP, amd64.RBP)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64BoolToString(e *amd64.Emitter, allocOffset int) {
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.SubRegImm32(amd64.RSP, 8)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegImm64(amd64.RDI, 13)
	callAt := len(e.Code)
	e.CallRel32(int32(allocOffset - (callAt + 5)))
	e.TestRegReg(amd64.RBX, amd64.RBX)
	falseJump := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	write := func(offset int32, ch byte) {
		e.MovRegImm64(amd64.R10, int64(ch))
		e.MovDerefReg8(amd64.RAX, offset, amd64.R10)
	}
	e.MovRegImm64(amd64.R10, 4)
	e.MovDerefReg(amd64.RAX, 0, amd64.R10)
	for i, ch := range []byte("true") {
		write(int32(8+i), ch)
	}
	doneJump := len(e.Code)
	e.JmpRel32(0)
	falseLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[falseJump+2:], uint32(int32(falseLabel-(falseJump+6))))
	e.MovRegImm64(amd64.R10, 5)
	e.MovDerefReg(amd64.RAX, 0, amd64.R10)
	for i, ch := range []byte("false") {
		write(int32(8+i), ch)
	}
	doneLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[doneJump+1:], uint32(int32(doneLabel-(doneJump+5))))
	e.AddRegImm32(amd64.RSP, 8)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64RuntimeInit(e *amd64.Emitter) {
	// Runtime context (R15): cursor, end, precise-root head, chunk head,
	// free-list head, collection count, reclaimed bytes, mapped bytes, and
	// cooperative task queue head/tail.
	e.MovRegImm64(amd64.RDI, 0)
	e.MovRegImm64(amd64.RSI, 1<<20)
	e.MovRegImm64(amd64.RDX, 3)
	e.MovRegImm64(amd64.R10, 0x22)
	e.MovRegImm64(amd64.R8, -1)
	e.MovRegImm64(amd64.R9, 0)
	e.MovRegImm64(amd64.RAX, 9)
	e.Syscall()

	// First mapping doubles as the first chunk. Objects begin after its 32-byte
	// chunk header and each object has its own 32-byte header.
	e.MovRegImm64(amd64.R11, 0)
	e.MovDerefReg(amd64.RAX, amd64ChunkNext, amd64.R11)
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.AddRegImm32(amd64.R10, 1<<20)
	e.MovDerefReg(amd64.RAX, amd64ChunkEnd, amd64.R10)
	e.MovRegReg(amd64.R11, amd64.RAX)
	e.AddRegImm32(amd64.R11, amd64ChunkSize)
	e.MovDerefReg(amd64.RAX, amd64ChunkUsed, amd64.R11)

	e.MovDerefReg(amd64.R15, amd64RTCursor, amd64.R11)
	e.MovDerefReg(amd64.R15, amd64RTEnd, amd64.R10)
	e.MovRegImm64(amd64.R11, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R11)
	e.MovDerefReg(amd64.R15, amd64RTChunkHead, amd64.RAX)
	e.MovDerefReg(amd64.R15, amd64RTFreeList, amd64.R11)
	e.MovDerefReg(amd64.R15, amd64RTCollections, amd64.R11)
	e.MovDerefReg(amd64.R15, amd64RTReclaimed, amd64.R11)
	e.MovRegImm64(amd64.R11, 1<<20)
	e.MovDerefReg(amd64.R15, amd64RTMappedBytes, amd64.R11)
	e.MovRegImm64(amd64.R11, 0)
	e.MovDerefReg(amd64.R15, amd64RTTaskHead, amd64.R11)
	e.MovDerefReg(amd64.R15, amd64RTTaskTail, amd64.R11)
	for _, off := range []int32{amd64RTCurrentTask, amd64RTSchedRsp, amd64RTSchedRbp, amd64RTSchedRbx, amd64RTSchedR12, amd64RTSchedR13, amd64RTSchedR14, amd64RTSchedRoot, amd64RTTimerHead, amd64RTMarkChunk, amd64RTMarkStack, amd64RTFree128, amd64RTFree512, amd64RTFree2048, amd64RTFree8192, amd64RTDenseChunkStack, amd64RTGlobalObject, amd64RTMicrotaskHead, amd64RTMicrotaskTail} {
		e.MovDerefReg(amd64.R15, off, amd64.R11)
	}
	emitAMD64ClockSampleToContext(e, 1, amd64RTTimeOriginMono)
	emitAMD64ClockSampleToContext(e, 0, amd64RTTimeOriginEpoch)
	e.Ret()
}

func emitAMD64SysExit(e *amd64.Emitter) {
	e.MovRegImm64(amd64.RDI, 0)
	e.MovRegImm64(amd64.RAX, 60) // Linux sys_exit
	e.Syscall()
	e.Ret()
}

func emitAMD64PrintStr(e *amd64.Emitter) {
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.SubRegImm32(amd64.RSP, 32)

	e.MovRegReg(amd64.R10, amd64.RDI)
	e.MovRegDeref(amd64.RDX, amd64.R10, 0) // count = length

	e.MovRegReg(amd64.RSI, amd64.R10)
	e.AddRegImm32(amd64.RSI, 8) // buf = r10 + 8

	e.MovRegImm64(amd64.RDI, 1) // stdout
	e.MovRegImm64(amd64.RAX, 1) // Linux sys_write = 1
	e.Syscall()

	// Newline '\n'
	e.MovRegImm64(amd64.RAX, 10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RAX)
	e.MovRegReg(amd64.RSI, amd64.RSP)
	e.AddRegImm32(amd64.RSI, 16)
	e.MovRegImm64(amd64.RDX, 1)
	e.MovRegImm64(amd64.RDI, 1)
	e.MovRegImm64(amd64.RAX, 1)
	e.Syscall()

	e.AddRegImm32(amd64.RSP, 32)
	e.Pop(amd64.RBP)
	e.Ret()
}
