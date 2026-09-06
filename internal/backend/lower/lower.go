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

func emitAMD64CopyStringBytes(e *amd64.Emitter, src, length amd64.Register) {
	// R10 is the destination cursor. Copy full qwords first, then the short tail.
	e.MovRegReg(amd64.R8, src)
	e.AddRegImm32(amd64.R8, 8)
	e.MovRegReg(amd64.R9, length)
	e.CmpRegImm32(amd64.R9, 8)
	tail := len(e.Code)
	e.JccRel32(amd64.CondL, 0)
	qwordLoop := len(e.Code)
	e.MovRegDeref(amd64.RAX, amd64.R8, 0)
	e.MovDerefReg(amd64.R10, 0, amd64.RAX)
	e.AddRegImm32(amd64.R8, 8)
	e.AddRegImm32(amd64.R10, 8)
	e.SubRegImm32(amd64.R9, 8)
	e.CmpRegImm32(amd64.R9, 8)
	qwordBack := len(e.Code)
	e.JccRel32(amd64.CondGE, 0)
	binary.LittleEndian.PutUint32(e.Code[qwordBack+2:], uint32(int32(qwordLoop-(qwordBack+6))))
	tailLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[tail+2:], uint32(int32(tailLabel-(tail+6))))
	e.TestRegReg(amd64.R9, amd64.R9)
	done := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	byteLoop := len(e.Code)
	e.MovzxRegDeref8(amd64.RAX, amd64.R8, 0)
	e.MovDerefReg8(amd64.R10, 0, amd64.RAX)
	e.AddRegImm32(amd64.R8, 1)
	e.AddRegImm32(amd64.R10, 1)
	e.SubRegImm32(amd64.R9, 1)
	byteBack := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	binary.LittleEndian.PutUint32(e.Code[byteBack+2:], uint32(int32(byteLoop-(byteBack+6))))
	doneLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[done+2:], uint32(int32(doneLabel-(done+6))))
}

func emitAMD64StringBuilderSeed(e *amd64.Emitter, allocOffset int) {
	// Clone a proven-unaliased literal into a normal string allocation with spare
	// capacity. The public string ABI stays [len][bytes]; capacity is derived from
	// the allocator object's total size in the hidden 32-byte object header.
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.SubRegImm32(amd64.RSP, 40)
	e.MovRegReg(amd64.RBX, amd64.RDI)

	// Precise root for the source across ts_alloc.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 1)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	e.MovRegDeref(amd64.R11, amd64.RBX, 0)
	e.MovDerefReg(amd64.RSP, 24, amd64.R11)
	e.MovRegReg(amd64.RDI, amd64.R11)
	e.AddRegImm32(amd64.RDI, 8)
	e.CmpRegImm32(amd64.RDI, 72) // 64 bytes of initial data capacity.
	enough := len(e.Code)
	e.JccRel32(amd64.CondGE, 0)
	e.MovRegImm64(amd64.RDI, 72)
	enoughLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[enough+2:], uint32(int32(enoughLabel-(enough+6))))
	callAt := len(e.Code)
	e.CallRel32(int32(allocOffset - (callAt + 5)))

	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.MovDerefReg(amd64.RSP, 32, amd64.RAX)
	e.MovRegDeref(amd64.R11, amd64.RSP, 24)
	e.MovDerefReg(amd64.RAX, 0, amd64.R11)
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.AddRegImm32(amd64.R10, 8)
	emitAMD64CopyStringBytes(e, amd64.RBX, amd64.R11)
	e.MovRegDeref(amd64.RAX, amd64.RSP, 32)
	e.AddRegImm32(amd64.RSP, 40)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64StringAppendOwned(e *amd64.Emitter, allocOffset int) {
	// This helper is only emitted for compiler-proven owned loop accumulators.
	// In-place growth therefore cannot mutate a string value observable through
	// another TypeScript binding.
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.Push(amd64.R15)
	e.SubRegImm32(amd64.RSP, 40)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)
	e.MovRegDeref(amd64.R13, amd64.RBX, 0)
	e.MovRegDeref(amd64.R14, amd64.R12, 0)
	e.MovRegReg(amd64.R11, amd64.R13)
	e.AddRegReg(amd64.R11, amd64.R14) // new length
	e.MovDerefReg(amd64.RSP, 32, amd64.R11)

	// Capacity = object total size - hidden header - visible length word.
	e.MovRegReg(amd64.R10, amd64.RBX)
	e.SubRegImm32(amd64.R10, amd64ObjectHeaderSize)
	e.MovRegDeref(amd64.R10, amd64.R10, amd64ObjectSize)
	e.SubRegImm32(amd64.R10, amd64ObjectHeaderSize+8)
	e.CmpRegReg(amd64.R11, amd64.R10)
	grow := len(e.Code)
	e.JccRel32(amd64.CondA, 0)

	// Fits: append the suffix directly into spare owned capacity.
	e.MovRegReg(amd64.R10, amd64.RBX)
	e.AddRegImm32(amd64.R10, 8)
	e.AddRegReg(amd64.R10, amd64.R13)
	emitAMD64CopyStringBytes(e, amd64.R12, amd64.R14)
	e.MovRegDeref(amd64.R11, amd64.RSP, 32)
	e.MovDerefReg(amd64.RBX, 0, amd64.R11)
	e.MovRegReg(amd64.RAX, amd64.RBX)
	done := len(e.Code)
	e.JmpRel32(0)

	growLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[grow+2:], uint32(int32(growLabel-(grow+6))))
	// Root both strings before allocation. Grow geometrically to make repeated
	// self-append amortized O(n) instead of copying the whole prefix each time.
	e.MovRegDeref(amd64.R11, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R11)
	e.MovRegImm64(amd64.R11, 2)
	e.MovDerefReg(amd64.RSP, 8, amd64.R11)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.RSP, 24, amd64.R12)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	e.AddRegReg(amd64.R10, amd64.R10) // doubled capacity
	e.MovRegDeref(amd64.R11, amd64.RSP, 32)
	e.CmpRegReg(amd64.R10, amd64.R11)
	capEnough := len(e.Code)
	e.JccRel32(amd64.CondAE, 0)
	e.MovRegReg(amd64.R10, amd64.R11)
	capEnoughLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[capEnough+2:], uint32(int32(capEnoughLabel-(capEnough+6))))
	e.MovRegReg(amd64.RDI, amd64.R10)
	e.AddRegImm32(amd64.RDI, 8)
	callAt := len(e.Code)
	e.CallRel32(int32(allocOffset - (callAt + 5)))

	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.MovDerefReg(amd64.RSP, 24, amd64.RAX)
	e.MovRegDeref(amd64.R11, amd64.RSP, 32)
	e.MovDerefReg(amd64.RAX, 0, amd64.R11)
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.AddRegImm32(amd64.R10, 8)
	emitAMD64CopyStringBytes(e, amd64.RBX, amd64.R13)
	emitAMD64CopyStringBytes(e, amd64.R12, amd64.R14)
	e.MovRegDeref(amd64.RAX, amd64.RSP, 24)

	doneLabel := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[done+1:], uint32(int32(doneLabel-(done+5))))
	e.AddRegImm32(amd64.RSP, 40)
	e.Pop(amd64.R15)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64StringConcatFixed(e *amd64.Emitter, allocOffset, count int) {
	// Fixed-arity concat helpers keep inputs in a precise-root frame across the
	// single result allocation. SysV argument registers cover the supported 3/4
	// operand forms without a secondary argument array.
	args := []amd64.Register{amd64.RDI, amd64.RSI, amd64.RDX, amd64.RCX}
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.SubRegImm32(amd64.RSP, 64)

	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, int64(count))
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	for i := 0; i < count; i++ {
		e.MovDerefReg(amd64.RSP, int32(16+i*8), args[i])
	}
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	e.MovRegImm64(amd64.R10, 0)
	for i := 0; i < count; i++ {
		e.MovRegDeref(amd64.R11, amd64.RSP, int32(16+i*8))
		e.MovRegDeref(amd64.R11, amd64.R11, 0)
		e.AddRegReg(amd64.R10, amd64.R11)
	}
	e.MovDerefReg(amd64.RSP, 48, amd64.R10)
	e.MovRegReg(amd64.RDI, amd64.R10)
	e.AddRegImm32(amd64.RDI, 8)
	callAt := len(e.Code)
	e.CallRel32(int32(allocOffset - (callAt + 5)))

	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.MovDerefReg(amd64.RSP, 56, amd64.RAX)
	e.MovRegDeref(amd64.R11, amd64.RSP, 48)
	e.MovDerefReg(amd64.RAX, 0, amd64.R11)
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.AddRegImm32(amd64.R10, 8)
	for i := 0; i < count; i++ {
		e.MovRegDeref(amd64.RDI, amd64.RSP, int32(16+i*8))
		e.MovRegDeref(amd64.R11, amd64.RDI, 0)
		emitAMD64CopyStringBytes(e, amd64.RDI, amd64.R11)
	}
	e.MovRegDeref(amd64.RAX, amd64.RSP, 56)
	e.AddRegImm32(amd64.RSP, 64)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64StringConcat(e *amd64.Emitter, allocOffset int) {
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)
	e.Push(amd64.R15)
	// 32-byte temporary precise-root frame plus 8 bytes of ABI padding.
	e.SubRegImm32(amd64.RSP, 40)

	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)
	e.MovRegDeref(amd64.R13, amd64.RBX, 0)
	e.MovRegDeref(amd64.R14, amd64.R12, 0)

	// Link a temporary precise-root frame for the two input strings. These are
	// live across ts_alloc, where a collection may run.
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTRootHead)
	e.MovDerefReg(amd64.RSP, 0, amd64.R10)
	e.MovRegImm64(amd64.R10, 2)
	e.MovDerefReg(amd64.RSP, 8, amd64.R10)
	e.MovDerefReg(amd64.RSP, 16, amd64.RBX)
	e.MovDerefReg(amd64.RSP, 24, amd64.R12)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.RSP)

	e.MovRegReg(amd64.RDI, amd64.R13)
	e.AddRegReg(amd64.RDI, amd64.R14)
	e.AddRegImm32(amd64.RDI, 8)
	callAt := len(e.Code)
	e.CallRel32(int32(allocOffset - (callAt + 5)))

	// Collection cannot occur again in this helper. Unlink the temporary root
	// frame and reuse its second root slot to keep the result pointer.
	e.MovRegDeref(amd64.R10, amd64.RSP, 0)
	e.MovDerefReg(amd64.R15, amd64RTRootHead, amd64.R10)
	e.MovDerefReg(amd64.RSP, 24, amd64.RAX)

	e.MovRegReg(amd64.R11, amd64.R13)
	e.AddRegReg(amd64.R11, amd64.R14)
	e.MovRegDeref(amd64.R10, amd64.RSP, 24)
	e.MovDerefReg(amd64.R10, 0, amd64.R11)

	// dst = result + 8. Copy qwords on the common path and only byte-copy tails.
	e.AddRegImm32(amd64.R10, 8)
	emitAMD64CopyStringBytes(e, amd64.RBX, amd64.R13)
	emitAMD64CopyStringBytes(e, amd64.R12, amd64.R14)

	e.MovRegDeref(amd64.RAX, amd64.RSP, 24)
	e.AddRegImm32(amd64.RSP, 40)
	e.Pop(amd64.R15)
	e.Pop(amd64.R14)
	e.Pop(amd64.R13)
	e.Pop(amd64.R12)
	e.Pop(amd64.RBX)
	e.Pop(amd64.RBP)
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
