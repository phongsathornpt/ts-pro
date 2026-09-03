package lower

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
	"github.com/phongsathornpt/ts-pro/internal/backend/asm/arm64"
	"github.com/phongsathornpt/ts-pro/internal/backend/regalloc"
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
	condReg  amd64.Register
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

func lowerARM64(prog *ir.Program) ([]byte, error) {
	e := arm64.NewEmitter()

	fnOffsets := make(map[string]int)
	var callFixups []callFixup
	var branchFixups []branchFixupARM64
	var strFixups []stringFixupARM64
	bbOffsets := make(map[*ir.BasicBlock]int)

	for _, fn := range prog.Functions {
		fnOffsets[fn.Name] = len(e.Code)

		ra := regalloc.New(len(arm64ScratchRegs))
		locs := ra.Allocate(fn)

		// Prologue: save FP, LR, and callee-saved registers X19-X24
		e.SubImm(arm64.SP, arm64.SP, 80)
		e.Stp(arm64.X29, arm64.X30, arm64.SP, 64)
		e.AddImm(arm64.X29, arm64.SP, 64)
		e.Stp(arm64.X19, arm64.X20, arm64.SP, 48)
		e.Stp(arm64.X21, arm64.X22, arm64.SP, 32)
		e.Stp(arm64.X23, arm64.X24, arm64.SP, 16)

		// Copy incoming parameters into allocated locations
		for i, param := range fn.Params {
			if i < len(arm64ParamRegs) {
				loc := locs[param.ID]
				if loc.IsReg {
					targetReg := arm64ScratchRegs[loc.Reg]
					e.MovReg(targetReg, arm64ParamRegs[i])
				}
			}
		}

		// Emit basic blocks
		for _, bb := range fn.Blocks {
			bbOffsets[bb] = len(e.Code)

			for _, inst := range bb.Instructions {
				switch bi := inst.(type) {
				case *ir.BinaryInst:
					dstLoc := locs[bi.Res.ID]
					if !dstLoc.IsReg {
						continue
					}
					dstReg := arm64ScratchRegs[dstLoc.Reg]

					// LHS
					var lhsReg arm64.Register = arm64.X8
					if vLHS, ok := bi.LHS.(*ir.Value); ok {
						srcLoc := locs[vLHS.ID]
						if srcLoc.IsReg {
							lhsReg = arm64ScratchRegs[srcLoc.Reg]
						}
					} else if cLHS, ok := bi.LHS.(ir.ConstNumber); ok {
						if cLHS.Value < 0 {
							e.Movz(arm64.X8, uint16(-cLHS.Value))
							e.Sub(arm64.X8, arm64.XZR, arm64.X8)
						} else {
							e.Movz(arm64.X8, uint16(cLHS.Value))
						}
						lhsReg = arm64.X8
					}

					// RHS
					var rhsReg arm64.Register = arm64.X16
					if vRHS, ok := bi.RHS.(*ir.Value); ok {
						rhsLoc := locs[vRHS.ID]
						if rhsLoc.IsReg {
							rhsReg = arm64ScratchRegs[rhsLoc.Reg]
						}
					} else if cRHS, ok := bi.RHS.(ir.ConstNumber); ok {
						if cRHS.Value < 0 {
							e.Movz(arm64.X16, uint16(-cRHS.Value))
							e.Sub(arm64.X16, arm64.XZR, arm64.X16)
						} else {
							e.Movz(arm64.X16, uint16(cRHS.Value))
						}
						rhsReg = arm64.X16
					}

					switch bi.Op {
					case ir.OpAdd:
						e.Add(dstReg, lhsReg, rhsReg)
					case ir.OpSub:
						e.Sub(dstReg, lhsReg, rhsReg)
					case ir.OpMul:
						e.Mul(dstReg, lhsReg, rhsReg)
					case ir.OpDiv:
						e.Sdiv(dstReg, lhsReg, rhsReg)
					case ir.OpMod:
						e.Sdiv(arm64.X17, lhsReg, rhsReg)
						e.Mul(arm64.X18, arm64.X17, rhsReg)
						e.Sub(dstReg, lhsReg, arm64.X18)
					case ir.OpAnd:
						e.And(dstReg, lhsReg, rhsReg)
					case ir.OpOr:
						e.Orr(dstReg, lhsReg, rhsReg)
					case ir.OpLt:
						e.Cmp(lhsReg, rhsReg)
						e.Cset(dstReg, arm64.CondLT)
					case ir.OpLe:
						e.Cmp(lhsReg, rhsReg)
						e.Cset(dstReg, arm64.CondLE)
					case ir.OpGt:
						e.Cmp(lhsReg, rhsReg)
						e.Cset(dstReg, arm64.CondGT)
					case ir.OpGe:
						e.Cmp(lhsReg, rhsReg)
						e.Cset(dstReg, arm64.CondGE)
					case ir.OpEq:
						e.Cmp(lhsReg, rhsReg)
						e.Cset(dstReg, arm64.CondEQ)
					case ir.OpNe:
						e.Cmp(lhsReg, rhsReg)
						e.Cset(dstReg, arm64.CondNE)
					}

				case *ir.UnaryInst:
					if bi.Op != "-" {
						continue
					}
					dstLoc := locs[bi.Res.ID]
					if !dstLoc.IsReg {
						continue
					}
					dstReg := arm64ScratchRegs[dstLoc.Reg]
					if v, ok := bi.Val.(*ir.Value); ok {
						srcLoc := locs[v.ID]
						if srcLoc.IsReg {
							e.Sub(dstReg, arm64.XZR, arm64ScratchRegs[srcLoc.Reg])
						}
					}

				case *ir.CallInst:
					for i, arg := range bi.Args {
						if i < len(arm64ParamRegs) {
							targetParam := arm64ParamRegs[i]
							if vArg, ok := arg.(*ir.Value); ok {
								argLoc := locs[vArg.ID]
								if argLoc.IsReg {
									e.MovReg(targetParam, arm64ScratchRegs[argLoc.Reg])
								}
							} else if cArg, ok := arg.(ir.ConstNumber); ok {
								if cArg.Value < 0 {
									e.Movz(targetParam, uint16(-cArg.Value))
									e.Sub(targetParam, arm64.XZR, targetParam)
								} else {
									e.Movz(targetParam, uint16(cArg.Value))
								}
							} else if sArg, ok := arg.(ir.ConstString); ok {
								strOffset := len(e.Code)
								e.Adr(targetParam, 0)
								strFixups = append(strFixups, stringFixupARM64{
									offset:    strOffset,
									targetReg: targetParam,
									str:       sArg.Value,
								})
							}
						}
					}

					callOffset := len(e.Code)
					e.Bl(0) // placeholder
					callFixups = append(callFixups, callFixup{offset: callOffset, callee: bi.Callee})

					if bi.Res != nil {
						dstLoc := locs[bi.Res.ID]
						if dstLoc.IsReg {
							dstReg := arm64ScratchRegs[dstLoc.Reg]
							e.MovReg(dstReg, arm64.X0)
						}
					}
				}
			}

			// Terminator
			if bb.Terminator != nil {
				switch term := bb.Terminator.(type) {
				case *ir.ReturnTerm:
					if term.Val != nil {
						if v, ok := term.Val.(*ir.Value); ok {
							loc := locs[v.ID]
							if loc.IsReg {
								srcReg := arm64ScratchRegs[loc.Reg]
								if srcReg != arm64.X0 {
									e.MovReg(arm64.X0, srcReg)
								}
							}
						} else if c, ok := term.Val.(ir.ConstNumber); ok {
							if c.Value < 0 {
								e.Movz(arm64.X0, uint16(-c.Value))
								e.Sub(arm64.X0, arm64.XZR, arm64.X0)
							} else {
								e.Movz(arm64.X0, uint16(c.Value))
							}
						} else if s, ok := term.Val.(ir.ConstString); ok {
							strOffset := len(e.Code)
							e.Adr(arm64.X0, 0)
							strFixups = append(strFixups, stringFixupARM64{
								offset:    strOffset,
								targetReg: arm64.X0,
								str:       s.Value,
							})
						}
					}

					if fn.Name == "@main" {
						callOffset := len(e.Code)
						e.Bl(0)
						callFixups = append(callFixups, callFixup{offset: callOffset, callee: "ts_sys_exit"})
					}

					// Epilogue: restore callee-saved registers and FP/LR
					e.Ldp(arm64.X23, arm64.X24, arm64.SP, 16)
					e.Ldp(arm64.X21, arm64.X22, arm64.SP, 32)
					e.Ldp(arm64.X19, arm64.X20, arm64.SP, 48)
					e.Ldp(arm64.X29, arm64.X30, arm64.SP, 64)
					e.AddImm(arm64.SP, arm64.SP, 80)
					e.Ret()

				case *ir.BranchTerm:
					var condReg arm64.Register = arm64.X8
					if vCond, ok := term.Cond.(*ir.Value); ok {
						loc := locs[vCond.ID]
						if loc.IsReg {
							condReg = arm64ScratchRegs[loc.Reg]
						}
					}
					// If cond != 0, branch to Then
					branchOffset := len(e.Code)
					e.Cbnz(condReg, 0)
					branchFixups = append(branchFixups, branchFixupARM64{
						offset:   branchOffset,
						targetBB: term.Then,
						isCond:   true,
						condReg:  condReg,
					})

					// Else branch
					jumpOffset := len(e.Code)
					e.B(0)
					branchFixups = append(branchFixups, branchFixupARM64{
						offset:   jumpOffset,
						targetBB: term.Else,
						isCond:   false,
					})

				case *ir.JumpTerm:
					// Emit phi incoming assignments for target basic block
					for _, phi := range term.Target.Phis {
						for _, inc := range phi.Incoming {
							if inc.Block == bb {
								dstLoc := locs[phi.Res.ID]
								if dstLoc.IsReg {
									dstReg := arm64ScratchRegs[dstLoc.Reg]
									if v, ok := inc.Value.(*ir.Value); ok {
										srcLoc := locs[v.ID]
										if srcLoc.IsReg {
											srcReg := arm64ScratchRegs[srcLoc.Reg]
											if dstReg != srcReg {
												e.MovReg(dstReg, srcReg)
											}
										}
									} else if c, ok := inc.Value.(ir.ConstNumber); ok {
										if c.Value < 0 {
											e.Movz(dstReg, uint16(-c.Value))
											e.Sub(dstReg, arm64.XZR, dstReg)
										} else {
											e.Movz(dstReg, uint16(c.Value))
										}
									} else if s, ok := inc.Value.(ir.ConstString); ok {
										strOffset := len(e.Code)
										e.Adr(dstReg, 0)
										strFixups = append(strFixups, stringFixupARM64{
											offset:    strOffset,
											targetReg: dstReg,
											str:       s.Value,
										})
									}
								}
							}
						}
					}

					jumpOffset := len(e.Code)
					e.B(0)
					branchFixups = append(branchFixups, branchFixupARM64{
						offset:   jumpOffset,
						targetBB: term.Target,
						isCond:   false,
					})
				}
			}
		}
	}

	// Emit ts_print_val (decimal printer)
	fnOffsets["ts_print_val"] = len(e.Code)
	emitARM64PrintVal(e)
	fnOffsets["ts_print_bool"] = fnOffsets["ts_print_val"]

	// Emit ts_print_str (string printer)
	fnOffsets["ts_print_str"] = len(e.Code)
	emitARM64PrintStr(e)

	// Emit ts_alloc (heap bump allocator)
	fnOffsets["ts_alloc"] = len(e.Code)
	emitARM64Alloc(e)

	// Emit ts_string_concat (string concatenator)
	fnOffsets["ts_string_concat"] = len(e.Code)
	emitARM64StringConcat(e, fnOffsets["ts_alloc"])

	// Emit ts_sys_exit
	fnOffsets["ts_sys_exit"] = len(e.Code)
	emitARM64SysExit(e)

	// Emit String Constants Table
	strOffsets := make(map[string]int)
	for _, sf := range strFixups {
		if _, exists := strOffsets[sf.str]; !exists {
			for len(e.Code)%8 != 0 {
				e.Code = append(e.Code, 0)
			}
			strOffsets[sf.str] = len(e.Code)

			var lenBuf [8]byte
			binary.LittleEndian.PutUint64(lenBuf[:], uint64(len(sf.str)))
			e.Code = append(e.Code, lenBuf[:]...)
			e.Code = append(e.Code, []byte(sf.str)...)
			e.Code = append(e.Code, 0)
		}
	}

	// Fix up string ADR instructions
	for _, sf := range strFixups {
		targetAddr := strOffsets[sf.str]
		disp := int32(targetAddr - sf.offset)
		imm21 := uint32(disp) & 0x1FFFFF
		immlo := imm21 & 0x3
		immhi := (imm21 >> 2) & 0x7FFFF
		inst := 0x10000000 | (immlo << 29) | (immhi << 5) | uint32(sf.targetReg)
		binary.LittleEndian.PutUint32(e.Code[sf.offset:], inst)
	}

	// Fix up function calls
	for _, cf := range callFixups {
		targetAddr, exists := fnOffsets[cf.callee]
		if !exists {
			return nil, fmt.Errorf("unresolved call target %q", cf.callee)
		}
		wordOffset := int32((targetAddr - cf.offset) / 4)
		binary.LittleEndian.PutUint32(e.Code[cf.offset:], 0x94000000|(uint32(wordOffset)&0x03FFFFFF))
	}

	// Fix up local branches
	for _, bf := range branchFixups {
		targetAddr, exists := bbOffsets[bf.targetBB]
		if !exists {
			return nil, fmt.Errorf("unresolved basic block target %p", bf.targetBB)
		}
		wordOffset := int32((targetAddr - bf.offset) / 4)
		if bf.isCond {
			// CBNZ condReg, wordOffset
			binary.LittleEndian.PutUint32(e.Code[bf.offset:], 0xB5000000|((uint32(wordOffset)&0x7FFFF)<<5)|uint32(bf.condReg))
		} else {
			// B wordOffset
			binary.LittleEndian.PutUint32(e.Code[bf.offset:], 0x14000000|(uint32(wordOffset)&0x03FFFFFF))
		}
	}

	return e.Code, nil
}

func emitARM64PrintVal(e *arm64.Emitter) {
	// Frame setup:
	// SUB SP, SP, #48
	// STP X29, X30, [SP, #32]
	// ADD X29, SP, #32
	e.SubImm(arm64.SP, arm64.SP, 48)
	e.Stp(arm64.X29, arm64.X30, arm64.SP, 32)
	e.AddImm(arm64.X29, arm64.SP, 32)

	// Buffer ends at SP+31. Put newline '\n' at SP+30
	e.AddImm(arm64.X1, arm64.SP, 30)
	e.Movz(arm64.X2, 10) // '\n'
	e.Strb(arm64.X2, arm64.X1, 0)

	// Sign handling: X5 = 1 if negative, 0 otherwise
	e.Movz(arm64.X5, 0)
	e.Cmp(arm64.X0, arm64.XZR)
	geOffset := len(e.Code)
	e.BCond(arm64.CondGE, 0)
	e.Movz(arm64.X5, 1)
	e.Sub(arm64.X0, arm64.XZR, arm64.X0) // X0 = -X0
	notNegOffset := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[geOffset:], 0x54000000|((uint32(int32((notNegOffset-geOffset)/4)&0x7FFFF)<<5)|uint32(arm64.CondGE)))

	// Check if X0 == 0
	cbnzOffset := len(e.Code)
	e.Cbnz(arm64.X0, 0) // placeholder fixup to loop

	// if X0 == 0: write '0'
	e.SubImm(arm64.X1, arm64.X1, 1)
	e.Movz(arm64.X2, 48) // '0'
	e.Strb(arm64.X2, arm64.X1, 0)

	jumpToPrintOffset := len(e.Code)
	e.B(0) // placeholder fixup to print

	// Loop for non-zero digits:
	loopOffset := len(e.Code)
	// Patch cbnzOffset to jump here:
	binary.LittleEndian.PutUint32(e.Code[cbnzOffset:], 0xB5000000|(uint32(int32((loopOffset-cbnzOffset)/4)&0x7FFFF)<<5)|uint32(arm64.X0))

	cbzOffset := len(e.Code)
	e.Cbz(arm64.X0, 0) // placeholder fixup to print when X0 == 0

	e.Movz(arm64.X2, 10)
	e.Sdiv(arm64.X3, arm64.X0, arm64.X2) // X3 = X0 / 10
	e.Mul(arm64.X4, arm64.X3, arm64.X2)  // X4 = X3 * 10
	e.Sub(arm64.X4, arm64.X0, arm64.X4)  // X4 = remainder
	e.AddImm(arm64.X4, arm64.X4, 48)     // digit char
	e.SubImm(arm64.X1, arm64.X1, 1)
	e.Strb(arm64.X4, arm64.X1, 0)
	e.MovReg(arm64.X0, arm64.X3)

	loopBackOffset := len(e.Code)
	e.B(int32((loopOffset - loopBackOffset) / 4))

	printOffset := len(e.Code)
	// Patch jumpToPrintOffset:
	binary.LittleEndian.PutUint32(e.Code[jumpToPrintOffset:], 0x14000000|(uint32(int32((printOffset-jumpToPrintOffset)/4))&0x03FFFFFF))
	// Patch cbzOffset:
	binary.LittleEndian.PutUint32(e.Code[cbzOffset:], 0xB4000000|(uint32(int32((printOffset-cbzOffset)/4)&0x7FFFF)<<5)|uint32(arm64.X0))

	// Prepend '-' if X5 != 0
	cbzSignOffset := len(e.Code)
	e.Cbz(arm64.X5, 0)
	e.SubImm(arm64.X1, arm64.X1, 1)
	e.Movz(arm64.X2, 45) // '-'
	e.Strb(arm64.X2, arm64.X1, 0)
	afterSignOffset := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[cbzSignOffset:], 0xB4000000|(uint32(int32((afterSignOffset-cbzSignOffset)/4)&0x7FFFF)<<5)|uint32(arm64.X5))

	// Print syscall:
	// Length = (SP + 31) - X1
	e.AddImm(arm64.X2, arm64.SP, 31)
	e.Sub(arm64.X2, arm64.X2, arm64.X1) // X2 = len
	e.Movz(arm64.X0, 1)                 // X0 = stdout
	// X1 is already buffer pointer
	e.Movz(arm64.X16, 4) // Darwin sys_write (4)
	e.Svc(0x80)

	// Epilogue:
	e.Ldp(arm64.X29, arm64.X30, arm64.SP, 32)
	e.AddImm(arm64.SP, arm64.SP, 48)
	e.Ret()
}

func emitARM64SysExit(e *arm64.Emitter) {
	e.Movz(arm64.X0, 0)  // exit code 0
	e.Movz(arm64.X16, 1) // Darwin sys_exit (1)
	e.Svc(0x80)
	e.Ret()
}

func emitARM64PrintStr(e *arm64.Emitter) {
	// Frame: 48 bytes (16-byte aligned)
	e.SubImm(arm64.SP, arm64.SP, 48)
	e.Stp(arm64.X29, arm64.X30, arm64.SP, 32)
	e.AddImm(arm64.X29, arm64.SP, 32)

	e.MovReg(arm64.X9, arm64.X0)
	e.Ldr(arm64.X2, arm64.X9, 0)    // len
	e.AddImm(arm64.X1, arm64.X9, 8) // data ptr
	e.Movz(arm64.X0, 1)             // stdout
	e.Movz(arm64.X16, 4)            // Darwin sys_write
	e.Svc(0x80)

	// Write newline
	e.AddImm(arm64.X1, arm64.SP, 16)
	e.Movz(arm64.X2, 10)
	e.Strb(arm64.X2, arm64.X1, 0)
	e.Movz(arm64.X2, 1)
	e.Movz(arm64.X0, 1)
	e.Movz(arm64.X16, 4)
	e.Svc(0x80)

	e.Ldp(arm64.X29, arm64.X30, arm64.SP, 32)
	e.AddImm(arm64.SP, arm64.SP, 48)
	e.Ret()
}

func emitARM64Alloc(e *arm64.Emitter) {
	pcInText := 1024 + len(e.Code)
	adrOffset := int32(0x4000 - pcInText)
	e.Adr(arm64.X1, adrOffset) // X1 points to 0x100004000 (__DATA)

	e.Ldr(arm64.X2, arm64.X1, 0) // current offset
	cbnzOffset := len(e.Code)
	e.Cbnz(arm64.X2, 0)
	e.Movz(arm64.X2, 16)

	hasOffset := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[cbnzOffset:], 0xB5000000|(uint32(int32((hasOffset-cbnzOffset)/4)&0x7FFFF)<<5)|uint32(arm64.X2))

	e.Add(arm64.X3, arm64.X1, arm64.X2) // return ptr
	e.Add(arm64.X2, arm64.X2, arm64.X0) // new offset
	e.AddImm(arm64.X2, arm64.X2, 15)
	e.Movz(arm64.X4, 15)
	e.Bic(arm64.X2, arm64.X2, arm64.X4)
	e.Str(arm64.X2, arm64.X1, 0)
	e.MovReg(arm64.X0, arm64.X3)
	e.Ret()
}

func emitARM64StringConcat(e *arm64.Emitter, allocOffset int) {
	e.SubImm(arm64.SP, arm64.SP, 80)
	e.Stp(arm64.X29, arm64.X30, arm64.SP, 64)
	e.AddImm(arm64.X29, arm64.SP, 64)
	e.Stp(arm64.X19, arm64.X20, arm64.SP, 48)
	e.Stp(arm64.X21, arm64.X22, arm64.SP, 32)
	e.Stp(arm64.X23, arm64.X24, arm64.SP, 16)

	e.MovReg(arm64.X19, arm64.X0) // a
	e.MovReg(arm64.X20, arm64.X1) // b

	e.Ldr(arm64.X21, arm64.X19, 0) // len_a
	e.Ldr(arm64.X22, arm64.X20, 0) // len_b

	e.Add(arm64.X23, arm64.X21, arm64.X22) // total_len

	e.AddImm(arm64.X0, arm64.X23, 16) // alloc size
	callAllocOffset := len(e.Code)
	e.Bl(int32((allocOffset - callAllocOffset) / 4))

	e.MovReg(arm64.X24, arm64.X0) // new_str

	e.Str(arm64.X23, arm64.X24, 0) // store total_len

	// Copy a
	e.AddImm(arm64.X9, arm64.X19, 8)  // src_a
	e.AddImm(arm64.X10, arm64.X24, 8) // dst
	e.Movz(arm64.X11, 0)

	cbzCopyA := len(e.Code)
	e.Cbz(arm64.X21, 0)

	loopCopyA := len(e.Code)
	e.Ldrb(arm64.X12, arm64.X9, 0)
	e.Strb(arm64.X12, arm64.X10, 0)
	e.AddImm(arm64.X9, arm64.X9, 1)
	e.AddImm(arm64.X10, arm64.X10, 1)
	e.AddImm(arm64.X11, arm64.X11, 1)
	e.Cmp(arm64.X11, arm64.X21)
	loopBackA := len(e.Code)
	e.BCond(arm64.CondLT, int32((loopCopyA-loopBackA)/4))

	afterCopyA := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[cbzCopyA:], 0xB4000000|(uint32(int32((afterCopyA-cbzCopyA)/4)&0x7FFFF)<<5)|uint32(arm64.X21))

	// Copy b
	e.AddImm(arm64.X9, arm64.X20, 8) // src_b
	e.Movz(arm64.X11, 0)

	cbzCopyB := len(e.Code)
	e.Cbz(arm64.X22, 0)

	loopCopyB := len(e.Code)
	e.Ldrb(arm64.X12, arm64.X9, 0)
	e.Strb(arm64.X12, arm64.X10, 0)
	e.AddImm(arm64.X9, arm64.X9, 1)
	e.AddImm(arm64.X10, arm64.X10, 1)
	e.AddImm(arm64.X11, arm64.X11, 1)
	e.Cmp(arm64.X11, arm64.X22)
	loopBackB := len(e.Code)
	e.BCond(arm64.CondLT, int32((loopCopyB-loopBackB)/4))

	afterCopyB := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[cbzCopyB:], 0xB4000000|(uint32(int32((afterCopyB-cbzCopyB)/4)&0x7FFFF)<<5)|uint32(arm64.X22))

	// Null-terminate
	e.Movz(arm64.X12, 0)
	e.Strb(arm64.X12, arm64.X10, 0)

	e.MovReg(arm64.X0, arm64.X24)

	e.Ldp(arm64.X23, arm64.X24, arm64.SP, 16)
	e.Ldp(arm64.X21, arm64.X22, arm64.SP, 32)
	e.Ldp(arm64.X19, arm64.X20, arm64.SP, 48)
	e.Ldp(arm64.X29, arm64.X30, arm64.SP, 64)
	e.AddImm(arm64.SP, arm64.SP, 80)
	e.Ret()
}

func lowerAMD64(prog *ir.Program) ([]byte, error) {
	e := amd64.NewEmitter()

	fnOffsets := make(map[string]int)
	var callFixups []callFixup
	var branchFixups []branchFixupAMD64
	var strFixups []stringFixupAMD64
	var closureCodeFixups []closureCodeFixupAMD64
	bbOffsets := make(map[*ir.BasicBlock]int)

	// Linux process entry is not a normal function call. Emit an explicit
	// startup stub so generated functions can use ordinary SysV call/return.
	hasMain := false
	for _, fn := range prog.Functions {
		if fn.Name == "@main" {
			hasMain = true
			break
		}
	}
	fnOffsets["_start"] = len(e.Code)
	// Reserve a small runtime context on the process stack. R15 is callee-saved
	// by SysV and deliberately excluded from the program register allocator.
	e.SubRegImm32(amd64.RSP, 144)
	e.MovRegReg(amd64.R15, amd64.RSP)
	initOffset := len(e.Code)
	e.CallRel32(0)
	callFixups = append(callFixups, callFixup{offset: initOffset, callee: "ts_runtime_init"})
	if hasMain {
		callOffset := len(e.Code)
		e.CallRel32(0)
		callFixups = append(callFixups, callFixup{offset: callOffset, callee: "@main"})
	}
	exitOffset := len(e.Code)
	e.CallRel32(0)
	callFixups = append(callFixups, callFixup{offset: exitOffset, callee: "ts_sys_exit"})

	for _, fn := range prog.Functions {
		fnOffsets[fn.Name] = len(e.Code)

		ra := regalloc.New(len(amd64ScratchRegs))
		locs := ra.Allocate(fn)
		spillBytes := ra.StackFrameSlots() * 8
		rootSlots := amd64RootSlots(fn)
		rootFrameBytes := 0
		rootFrameBaseOffset := int32(0)
		if len(rootSlots) != 0 {
			rootFrameBytes = 16 + len(rootSlots)*8
			rootFrameBaseOffset = -int32(40 + spillBytes + rootFrameBytes)
		}
		localBytes := spillBytes + rootFrameBytes
		// After CALL, push RBP + five callee-saved registers leaves RSP at 8 mod 16.
		// Choose a frame size that is 8 mod 16 so call sites remain 16-byte aligned.
		frameSize := int32(((localBytes + 23) &^ 15) - 8)
		spillOffset := func(loc regalloc.Location) int32 {
			return int32(-48 - loc.StackSlot*8)
		}
		loadValue := func(v *ir.Value, scratch amd64.Register) amd64.Register {
			loc := locs[v.ID]
			if loc.IsReg {
				return amd64ScratchRegs[loc.Reg]
			}
			e.MovRegDeref(scratch, amd64.RBP, spillOffset(loc))
			return scratch
		}
		storeValue := func(loc regalloc.Location, src amd64.Register) {
			if loc.IsReg {
				dst := amd64ScratchRegs[loc.Reg]
				if dst != src {
					e.MovRegReg(dst, src)
				}
				return
			}
			e.MovDerefReg(amd64.RBP, spillOffset(loc), src)
		}
		storeSSAValue := func(v *ir.Value, src amd64.Register) {
			storeValue(locs[v.ID], src)
			if slot, ok := rootSlots[v.ID]; ok {
				e.MovDerefReg(amd64.RBP, rootFrameBaseOffset+16+int32(slot*8), src)
			}
		}
		leaveRootFrame := func() {
			if len(rootSlots) == 0 {
				return
			}
			e.MovRegDeref(amd64.R10, amd64.RBP, rootFrameBaseOffset)
			e.MovDerefReg(amd64.R15, 16, amd64.R10)
		}

		loadOperand := func(op ir.Operand, scratch amd64.Register) (amd64.Register, error) {
			switch v := op.(type) {
			case *ir.Value:
				return loadValue(v, scratch), nil
			case ir.ConstNumber:
				e.MovRegImm64(scratch, numberBits(v.Value))
				return scratch, nil
			case ir.ConstBool:
				if v.Value {
					e.MovRegImm64(scratch, 1)
				} else {
					e.MovRegImm64(scratch, 0)
				}
				return scratch, nil
			case ir.ConstUndefined:
				e.MovRegImm64(scratch, amd64UndefinedBits)
				return scratch, nil
			case ir.ConstNull:
				e.MovRegImm64(scratch, amd64NullBits)
				return scratch, nil
			default:
				return scratch, fmt.Errorf("unsupported AMD64 operand %T", op)
			}
		}
		loadRawValue := func(op ir.Operand, dst amd64.Register) (amd64.Register, error) {
			if str, ok := op.(ir.ConstString); ok {
				at := len(e.Code)
				e.LeaRipRel32(dst, 0)
				strFixups = append(strFixups, stringFixupAMD64{offset: at + 3, targetReg: dst, str: str.Value})
				return dst, nil
			}
			return loadOperand(op, dst)
		}
		loadArrayIndex := func(op ir.Operand, dst amd64.Register) error {
			src, err := loadOperand(op, amd64.R10)
			if err != nil {
				return err
			}
			e.MovQXMMReg(amd64.XMM0, src)
			e.Cvttsd2si(dst, amd64.XMM0)
			return nil
		}
		emitRuntimeCall := func(callee string) {
			at := len(e.Code)
			e.CallRel32(0)
			callFixups = append(callFixups, callFixup{offset: at, callee: callee})
		}

		// Prologue: save RBP and callee-saved registers RBX, R12-R15.
		e.Push(amd64.RBP)
		e.MovRegReg(amd64.RBP, amd64.RSP)
		e.Push(amd64.RBX)
		e.Push(amd64.R12)
		e.Push(amd64.R13)
		e.Push(amd64.R14)
		e.Push(amd64.R15)
		e.SubRegImm32(amd64.RSP, frameSize)

		if len(rootSlots) != 0 {
			e.MovRegReg(amd64.R10, amd64.RBP)
			e.SubRegImm32(amd64.R10, -rootFrameBaseOffset)
			e.MovRegDeref(amd64.R11, amd64.R15, 16)
			e.MovDerefReg(amd64.R10, 0, amd64.R11)
			e.MovRegImm64(amd64.R11, int64(len(rootSlots)))
			e.MovDerefReg(amd64.R10, 8, amd64.R11)
			e.MovRegImm64(amd64.R11, 0)
			for i := 0; i < len(rootSlots); i++ {
				e.MovDerefReg(amd64.R10, int32(16+i*8), amd64.R11)
			}
			e.MovDerefReg(amd64.R15, 16, amd64.R10)
		}

		// Classify incoming SysV parameters. Number values use the SSE class;
		// references/booleans use the integer class. Overflow arguments are read
		// from the caller stack in source order.
		gprParam, xmmParam, stackParam := 0, 0, 0
		for _, param := range fn.Params {
			if isNumberType(param.Type()) {
				if xmmParam < len(amd64NumberParamRegs) {
					e.MovQRegXMM(amd64.R10, amd64NumberParamRegs[xmmParam])
					storeSSAValue(param, amd64.R10)
					xmmParam++
				} else {
					e.MovRegDeref(amd64.R10, amd64.RBP, int32(16+stackParam*8))
					storeSSAValue(param, amd64.R10)
					stackParam++
				}
				continue
			}
			if gprParam < len(amd64ParamRegs) {
				storeSSAValue(param, amd64ParamRegs[gprParam])
				gprParam++
			} else {
				e.MovRegDeref(amd64.R10, amd64.RBP, int32(16+stackParam*8))
				storeSSAValue(param, amd64.R10)
				stackParam++
			}
		}

		for _, bb := range fn.Blocks {
			bbOffsets[bb] = len(e.Code)

			for _, inst := range bb.Instructions {
				switch bi := inst.(type) {
				case *ir.BinaryInst:
					dstLoc := locs[bi.Res.ID]
					_, lhsNull := bi.LHS.(ir.ConstNull)
					_, lhsUndef := bi.LHS.(ir.ConstUndefined)
					_, rhsNull := bi.RHS.(ir.ConstNull)
					_, rhsUndef := bi.RHS.(ir.ConstUndefined)
					if (bi.Op == ir.OpEq || bi.Op == ir.OpNe) && (lhsNull || lhsUndef || rhsNull || rhsUndef) {
						lhs, err := loadRawValue(bi.LHS, amd64.R10)
						if err != nil {
							return nil, err
						}
						rhs, err := loadRawValue(bi.RHS, amd64.R11)
						if err != nil {
							return nil, err
						}
						e.CmpRegReg(lhs, rhs)
						cond := amd64.CondE
						if bi.Op == ir.OpNe {
							cond = amd64.CondNE
						}
						e.Setcc(cond, amd64.R10)
						storeSSAValue(bi.Res, amd64.R10)
						continue
					}
					if bi.LHS.Type() == types.TypeString && bi.RHS.Type() == types.TypeString && (bi.Op == ir.OpEq || bi.Op == ir.OpNe) {
						lhs, err := loadRawValue(bi.LHS, amd64.RDI)
						if err != nil {
							return nil, err
						}
						if lhs != amd64.RDI {
							e.MovRegReg(amd64.RDI, lhs)
						}
						rhs, err := loadRawValue(bi.RHS, amd64.RSI)
						if err != nil {
							return nil, err
						}
						if rhs != amd64.RSI {
							e.MovRegReg(amd64.RSI, rhs)
						}
						emitRuntimeCall("ts_string_eq")
						if bi.Op == ir.OpNe {
							e.TestRegReg(amd64.RAX, amd64.RAX)
							e.Setcc(amd64.CondE, amd64.RAX)
						}
						storeSSAValue(bi.Res, amd64.RAX)
						continue
					}
					if isNumberType(bi.LHS.Type()) || isNumberType(bi.RHS.Type()) {
						lhsReg, err := loadOperand(bi.LHS, amd64.R10)
						if err != nil {
							return nil, err
						}
						rhsReg, err := loadOperand(bi.RHS, amd64.R11)
						if err != nil {
							return nil, err
						}
						e.MovQXMMReg(amd64.XMM0, lhsReg)
						e.MovQXMMReg(amd64.XMM1, rhsReg)
						switch bi.Op {
						case ir.OpAdd:
							e.AddSD(amd64.XMM0, amd64.XMM1)
							e.MovQRegXMM(amd64.R10, amd64.XMM0)
						case ir.OpSub:
							e.SubSD(amd64.XMM0, amd64.XMM1)
							e.MovQRegXMM(amd64.R10, amd64.XMM0)
						case ir.OpMul:
							e.MulSD(amd64.XMM0, amd64.XMM1)
							e.MovQRegXMM(amd64.R10, amd64.XMM0)
						case ir.OpDiv:
							e.DivSD(amd64.XMM0, amd64.XMM1)
							e.MovQRegXMM(amd64.R10, amd64.XMM0)
						case ir.OpMod:
							e.MovSDRegReg(amd64.XMM2, amd64.XMM0)
							e.DivSD(amd64.XMM2, amd64.XMM1)
							e.Cvttsd2si(amd64.RAX, amd64.XMM2)
							e.Cvtsi2sd(amd64.XMM2, amd64.RAX)
							e.MulSD(amd64.XMM2, amd64.XMM1)
							e.SubSD(amd64.XMM0, amd64.XMM2)
							e.MovQRegXMM(amd64.R10, amd64.XMM0)
						case ir.OpEq, ir.OpNe, ir.OpLt, ir.OpLe, ir.OpGt, ir.OpGe:
							e.Ucomisd(amd64.XMM0, amd64.XMM1)
							switch bi.Op {
							case ir.OpEq:
								e.Setcc(amd64.CondE, amd64.R10)
								e.Setcc(amd64.CondNP, amd64.R11)
								e.AndRegReg(amd64.R10, amd64.R11)
							case ir.OpNe:
								e.Setcc(amd64.CondNE, amd64.R10)
								e.Setcc(amd64.CondP, amd64.R11)
								e.OrRegReg(amd64.R10, amd64.R11)
							case ir.OpLt:
								e.Setcc(amd64.CondB, amd64.R10)
								e.Setcc(amd64.CondNP, amd64.R11)
								e.AndRegReg(amd64.R10, amd64.R11)
							case ir.OpLe:
								e.Setcc(amd64.CondBE, amd64.R10)
								e.Setcc(amd64.CondNP, amd64.R11)
								e.AndRegReg(amd64.R10, amd64.R11)
							case ir.OpGt:
								e.Setcc(amd64.CondA, amd64.R10)
							case ir.OpGe:
								e.Setcc(amd64.CondAE, amd64.R10)
							}
						default:
							return nil, fmt.Errorf("unsupported AMD64 number op %v", bi.Op)
						}
						storeValue(dstLoc, amd64.R10)
						continue
					}

					lhsReg, err := loadOperand(bi.LHS, amd64.R10)
					if err != nil {
						return nil, err
					}
					rhsReg, err := loadOperand(bi.RHS, amd64.R11)
					if err != nil {
						return nil, err
					}
					e.MovRegReg(amd64.R10, lhsReg)
					switch bi.Op {
					case ir.OpAnd:
						e.AndRegReg(amd64.R10, rhsReg)
					case ir.OpOr:
						e.OrRegReg(amd64.R10, rhsReg)
					case ir.OpEq:
						e.CmpRegReg(amd64.R10, rhsReg)
						e.Setcc(amd64.CondE, amd64.R10)
					case ir.OpNe:
						e.CmpRegReg(amd64.R10, rhsReg)
						e.Setcc(amd64.CondNE, amd64.R10)
					default:
						return nil, fmt.Errorf("unsupported AMD64 non-number op %v", bi.Op)
					}
					storeValue(dstLoc, amd64.R10)

				case *ir.UnaryInst:
					if bi.Op != "-" || !isNumberType(bi.Val.Type()) {
						return nil, fmt.Errorf("unsupported AMD64 unary op %q", bi.Op)
					}
					src, err := loadOperand(bi.Val, amd64.R10)
					if err != nil {
						return nil, err
					}
					if src != amd64.R10 {
						e.MovRegReg(amd64.R10, src)
					}
					e.MovRegImm64(amd64.R11, int64(-0x8000000000000000))
					e.XorRegReg(amd64.R10, amd64.R11)
					storeValue(locs[bi.Res.ID], amd64.R10)

				case *ir.AllocObjectInst:
					e.MovRegImm64(amd64.RDI, int64(bi.FieldCount))
					e.MovRegImm64(amd64.RSI, int64(bi.RefMask))
					emitRuntimeCall("ts_object_new")
					storeSSAValue(bi.Res, amd64.RAX)

				case *ir.GetFieldInst:
					obj, err := loadOperand(bi.Obj, amd64.R10)
					if err != nil {
						return nil, err
					}
					e.MovRegDeref(amd64.R11, obj, int32(bi.Offset))
					storeSSAValue(bi.Res, amd64.R11)

				case *ir.SetFieldInst:
					obj, err := loadOperand(bi.Obj, amd64.R10)
					if err != nil {
						return nil, err
					}
					if obj != amd64.R10 {
						e.MovRegReg(amd64.R10, obj)
					}
					val, err := loadRawValue(bi.Val, amd64.R11)
					if err != nil {
						return nil, err
					}
					if val != amd64.R11 {
						e.MovRegReg(amd64.R11, val)
					}
					e.MovDerefReg(amd64.R10, int32(bi.Offset), amd64.R11)

				case *ir.AllocArrayInst:
					if err := loadArrayIndex(bi.Length, amd64.RDI); err != nil {
						return nil, err
					}
					e.MovRegImm64(amd64.RSI, amd64ArrayElementClass(bi.ElemType))
					emitRuntimeCall("ts_array_new")
					storeSSAValue(bi.Res, amd64.RAX)

				case *ir.GetElementInst:
					arr, err := loadOperand(bi.Array, amd64.RDI)
					if err != nil {
						return nil, err
					}
					if arr != amd64.RDI {
						e.MovRegReg(amd64.RDI, arr)
					}
					if err := loadArrayIndex(bi.Index, amd64.RSI); err != nil {
						return nil, err
					}
					emitRuntimeCall("ts_array_get")
					storeSSAValue(bi.Res, amd64.RAX)

				case *ir.SetElementInst:
					arr, err := loadOperand(bi.Array, amd64.RDI)
					if err != nil {
						return nil, err
					}
					if arr != amd64.RDI {
						e.MovRegReg(amd64.RDI, arr)
					}
					if err := loadArrayIndex(bi.Index, amd64.RSI); err != nil {
						return nil, err
					}
					val, err := loadRawValue(bi.Val, amd64.RDX)
					if err != nil {
						return nil, err
					}
					if val != amd64.RDX {
						e.MovRegReg(amd64.RDX, val)
					}
					emitRuntimeCall("ts_array_set")

				case *ir.ArrayLengthInst:
					arr, err := loadOperand(bi.Array, amd64.RDI)
					if err != nil {
						return nil, err
					}
					if arr != amd64.RDI {
						e.MovRegReg(amd64.RDI, arr)
					}
					emitRuntimeCall("ts_array_len")
					e.MovQRegXMM(amd64.R10, amd64.XMM0)
					storeSSAValue(bi.Res, amd64.R10)

				case *ir.ArrayPushInst:
					arr, err := loadOperand(bi.Array, amd64.RDI)
					if err != nil {
						return nil, err
					}
					if arr != amd64.RDI {
						e.MovRegReg(amd64.RDI, arr)
					}
					val, err := loadRawValue(bi.Val, amd64.RSI)
					if err != nil {
						return nil, err
					}
					if val != amd64.RSI {
						e.MovRegReg(amd64.RSI, val)
					}
					emitRuntimeCall("ts_array_push")
					e.MovQRegXMM(amd64.R10, amd64.XMM0)
					storeSSAValue(bi.Res, amd64.R10)

				case *ir.ArrayPopInst:
					arr, err := loadOperand(bi.Array, amd64.RDI)
					if err != nil {
						return nil, err
					}
					if arr != amd64.RDI {
						e.MovRegReg(amd64.RDI, arr)
					}
					emitRuntimeCall("ts_array_pop")
					storeSSAValue(bi.Res, amd64.RAX)

				case *ir.MakeClosureInst:
					// Closure payload: [code ptr, capture count, ref mask, captures...].
					e.MovRegImm64(amd64.RDI, int64(24+len(bi.Captures)*8))
					emitRuntimeCall("ts_alloc")
					emitAMD64SetObjectType(e, amd64.RAX, amd64ObjectTypeClosure)
					codeAt := len(e.Code)
					e.LeaRipRel32(amd64.R10, 0)
					closureCodeFixups = append(closureCodeFixups, closureCodeFixupAMD64{offset: codeAt + 3, function: bi.Function})
					e.MovDerefReg(amd64.RAX, 0, amd64.R10)
					e.MovRegImm64(amd64.R10, int64(len(bi.Captures)))
					e.MovDerefReg(amd64.RAX, 8, amd64.R10)
					e.MovRegImm64(amd64.R10, int64(bi.RefMask))
					e.MovDerefReg(amd64.RAX, 16, amd64.R10)
					for i, capture := range bi.Captures {
						v, err := loadRawValue(capture, amd64.R11)
						if err != nil {
							return nil, err
						}
						if v != amd64.R11 {
							e.MovRegReg(amd64.R11, v)
						}
						e.MovDerefReg(amd64.RAX, int32(24+i*8), amd64.R11)
					}
					storeSSAValue(bi.Res, amd64.RAX)

				case *ir.ClosureGetInst:
					closure, err := loadOperand(bi.Closure, amd64.R10)
					if err != nil {
						return nil, err
					}
					e.MovRegDeref(amd64.R11, closure, int32(24+bi.Index*8))
					storeSSAValue(bi.Res, amd64.R11)

				case *ir.IndirectCallInst:
					closure, err := loadOperand(bi.Closure, amd64.R10)
					if err != nil {
						return nil, err
					}
					if closure != amd64.RAX {
						e.MovRegReg(amd64.RAX, closure)
					}
					// Hidden closure environment occupies RDI. Method-style structural
					// calls optionally place their receiver in RSI; user integer-class
					// arguments then begin at RDX. SSE arguments still begin at XMM0.
					e.MovRegReg(amd64.RDI, amd64.RAX)
					userGPRs := []amd64.Register{amd64.RSI, amd64.RDX, amd64.RCX, amd64.R8, amd64.R9}
					if bi.ThisArg != nil {
						thisReg, err := loadRawValue(bi.ThisArg, amd64.R10)
						if err != nil {
							return nil, err
						}
						if thisReg != amd64.RSI {
							e.MovRegReg(amd64.RSI, thisReg)
						}
						userGPRs = []amd64.Register{amd64.RDX, amd64.RCX, amd64.R8, amd64.R9}
					}
					gprArg, xmmArg := 0, 0
					stackArgs := make([]ir.Operand, 0)
					emitIndirectGPRArg := func(dst amd64.Register, arg ir.Operand) error {
						v, err := loadRawValue(arg, amd64.R10)
						if err != nil {
							return err
						}
						if v != dst {
							e.MovRegReg(dst, v)
						}
						return nil
					}
					for i, arg := range bi.Args {
						argType := arg.Type()
						if i < len(bi.ParamTypes) && bi.ParamTypes[i] != nil {
							argType = bi.ParamTypes[i]
						}
						if isNumberType(argType) {
							if xmmArg < len(amd64NumberParamRegs) {
								v, err := loadOperand(arg, amd64.R10)
								if err != nil {
									return nil, err
								}
								e.MovQXMMReg(amd64NumberParamRegs[xmmArg], v)
								xmmArg++
							} else {
								stackArgs = append(stackArgs, arg)
							}
							continue
						}
						if gprArg < len(userGPRs) {
							if err := emitIndirectGPRArg(userGPRs[gprArg], arg); err != nil {
								return nil, err
							}
							gprArg++
						} else {
							stackArgs = append(stackArgs, arg)
						}
					}
					stackBytes := len(stackArgs) * 8
					padBytes := 0
					if stackBytes%16 != 0 {
						padBytes = 8
						e.SubRegImm32(amd64.RSP, 8)
					}
					for i := len(stackArgs) - 1; i >= 0; i-- {
						if err := emitIndirectGPRArg(amd64.R10, stackArgs[i]); err != nil {
							return nil, err
						}
						e.Push(amd64.R10)
					}
					e.MovRegDeref(amd64.R11, amd64.RAX, 0)
					e.CallReg(amd64.R11)
					if cleanup := stackBytes + padBytes; cleanup != 0 {
						e.AddRegImm32(amd64.RSP, int32(cleanup))
					}
					if bi.Res != nil {
						if isNumberType(bi.Res.Type()) {
							e.MovQRegXMM(amd64.R10, amd64.XMM0)
							storeSSAValue(bi.Res, amd64.R10)
						} else {
							storeSSAValue(bi.Res, amd64.RAX)
						}
					}

				case *ir.CallInst:
					gprArg, xmmArg := 0, 0
					stackArgs := make([]ir.Operand, 0)
					emitGPRArg := func(dst amd64.Register, arg ir.Operand) error {
						switch v := arg.(type) {
						case ir.ConstString:
							strOffset := len(e.Code)
							e.LeaRipRel32(dst, 0)
							strFixups = append(strFixups, stringFixupAMD64{offset: strOffset + 3, targetReg: dst, str: v.Value})
							return nil
						default:
							src, err := loadOperand(arg, amd64.R10)
							if err != nil {
								return err
							}
							if src != dst {
								e.MovRegReg(dst, src)
							}
							return nil
						}
					}
					for i, arg := range bi.Args {
						argType := arg.Type()
						if i < len(bi.ParamTypes) && bi.ParamTypes[i] != nil {
							argType = bi.ParamTypes[i]
						}
						if isNumberType(argType) {
							if xmmArg < len(amd64NumberParamRegs) {
								src, err := loadOperand(arg, amd64.R10)
								if err != nil {
									return nil, err
								}
								e.MovQXMMReg(amd64NumberParamRegs[xmmArg], src)
								xmmArg++
							} else {
								stackArgs = append(stackArgs, arg)
							}
							continue
						}
						if gprArg < len(amd64ParamRegs) {
							if err := emitGPRArg(amd64ParamRegs[gprArg], arg); err != nil {
								return nil, err
							}
							gprArg++
						} else {
							stackArgs = append(stackArgs, arg)
						}
					}

					stackBytes := len(stackArgs) * 8
					padBytes := 0
					if stackBytes%16 != 0 {
						padBytes = 8
						e.SubRegImm32(amd64.RSP, 8)
					}
					for i := len(stackArgs) - 1; i >= 0; i-- {
						if err := emitGPRArg(amd64.R10, stackArgs[i]); err != nil {
							return nil, err
						}
						e.Push(amd64.R10)
					}

					callOffset := len(e.Code)
					e.CallRel32(0)
					callFixups = append(callFixups, callFixup{offset: callOffset, callee: bi.Callee})
					if cleanup := stackBytes + padBytes; cleanup != 0 {
						e.AddRegImm32(amd64.RSP, int32(cleanup))
					}

					if bi.Res != nil {
						if isNumberType(bi.Res.Type()) {
							e.MovQRegXMM(amd64.R10, amd64.XMM0)
							storeSSAValue(bi.Res, amd64.R10)
						} else {
							storeSSAValue(bi.Res, amd64.RAX)
						}
					}

				default:
					return nil, fmt.Errorf("unsupported AMD64 instruction %T", inst)
				}
			}

			if bb.Terminator != nil {
				switch term := bb.Terminator.(type) {
				case *ir.ReturnTerm:
					if term.Val != nil {
						if isNumberType(fn.ReturnType) {
							src, err := loadOperand(term.Val, amd64.R10)
							if err != nil {
								return nil, err
							}
							e.MovQXMMReg(amd64.XMM0, src)
						} else if str, ok := term.Val.(ir.ConstString); ok {
							strOffset := len(e.Code)
							e.LeaRipRel32(amd64.RAX, 0)
							strFixups = append(strFixups, stringFixupAMD64{offset: strOffset + 3, targetReg: amd64.RAX, str: str.Value})
						} else {
							src, err := loadOperand(term.Val, amd64.RAX)
							if err != nil {
								return nil, err
							}
							if src != amd64.RAX {
								e.MovRegReg(amd64.RAX, src)
							}
						}
					}

					// Unlink the precise root frame before restoring the machine frame.
					leaveRootFrame()
					e.AddRegImm32(amd64.RSP, frameSize)
					e.Pop(amd64.R15)
					e.Pop(amd64.R14)
					e.Pop(amd64.R13)
					e.Pop(amd64.R12)
					e.Pop(amd64.RBX)
					e.Pop(amd64.RBP)
					e.Ret()

				case *ir.BranchTerm:
					if c, ok := term.Cond.(ir.ConstBool); ok {
						target := term.Else
						if c.Value {
							target = term.Then
						}
						jumpOffset := len(e.Code)
						e.JmpRel32(0)
						branchFixups = append(branchFixups, branchFixupAMD64{offset: jumpOffset, targetBB: target})
						break
					}
					if c, ok := term.Cond.(ir.ConstNumber); ok {
						target := term.Else
						if c.Value != 0 {
							target = term.Then
						}
						jumpOffset := len(e.Code)
						e.JmpRel32(0)
						branchFixups = append(branchFixups, branchFixupAMD64{offset: jumpOffset, targetBB: target})
						break
					}

					if vCond, ok := term.Cond.(*ir.Value); ok && isNumberType(vCond.Type()) {
						condReg := loadValue(vCond, amd64.R10)
						e.MovQXMMReg(amd64.XMM0, condReg)
						e.XorPD(amd64.XMM1, amd64.XMM1)
						e.Ucomisd(amd64.XMM0, amd64.XMM1)
						for _, cond := range []amd64.Cond{amd64.CondNE, amd64.CondP} {
							at := len(e.Code)
							e.JccRel32(cond, 0)
							branchFixups = append(branchFixups, branchFixupAMD64{offset: at, targetBB: term.Then, isCond: true})
						}
						jumpOffset := len(e.Code)
						e.JmpRel32(0)
						branchFixups = append(branchFixups, branchFixupAMD64{offset: jumpOffset, targetBB: term.Else})
						break
					}

					condReg := amd64.RAX
					if vCond, ok := term.Cond.(*ir.Value); ok {
						condReg = loadValue(vCond, amd64.R10)
					}
					e.TestRegReg(condReg, condReg)
					branchOffset := len(e.Code)
					e.JccRel32(amd64.CondNE, 0)
					branchFixups = append(branchFixups, branchFixupAMD64{offset: branchOffset, targetBB: term.Then, isCond: true})
					jumpOffset := len(e.Code)
					e.JmpRel32(0)
					branchFixups = append(branchFixups, branchFixupAMD64{offset: jumpOffset, targetBB: term.Else})

				case *ir.JumpTerm:
					for _, phi := range term.Target.Phis {
						for _, inc := range phi.Incoming {
							if inc.Block == bb {
								if v, ok := inc.Value.(*ir.Value); ok {
									srcReg := loadValue(v, amd64.R10)
									storeSSAValue(phi.Res, srcReg)
								} else if c, ok := inc.Value.(ir.ConstNumber); ok {
									e.MovRegImm64(amd64.R10, numberBits(c.Value))
									storeSSAValue(phi.Res, amd64.R10)
								} else if c, ok := inc.Value.(ir.ConstBool); ok {
									if c.Value {
										e.MovRegImm64(amd64.R10, 1)
									} else {
										e.MovRegImm64(amd64.R10, 0)
									}
									storeSSAValue(phi.Res, amd64.R10)
								} else if _, ok := inc.Value.(ir.ConstUndefined); ok {
									e.MovRegImm64(amd64.R10, amd64UndefinedBits)
									storeSSAValue(phi.Res, amd64.R10)
								} else if _, ok := inc.Value.(ir.ConstNull); ok {
									e.MovRegImm64(amd64.R10, amd64NullBits)
									storeSSAValue(phi.Res, amd64.R10)
								} else if c, ok := inc.Value.(ir.ConstString); ok {
									strOffset := len(e.Code)
									e.LeaRipRel32(amd64.R10, 0)
									strFixups = append(strFixups, stringFixupAMD64{offset: strOffset + 3, targetReg: amd64.R10, str: c.Value})
									storeSSAValue(phi.Res, amd64.R10)
								}
							}
						}
					}

					jumpOffset := len(e.Code)
					e.JmpRel32(0)
					branchFixups = append(branchFixups, branchFixupAMD64{
						offset:   jumpOffset,
						targetBB: term.Target,
						isCond:   false,
					})
				}
			}
		}
	}

	// Emit ts_print_val for Linux AMD64
	fnOffsets["ts_print_val"] = len(e.Code)
	emitAMD64PrintValV2(e)

	fnOffsets["ts_print_bool"] = len(e.Code)
	emitAMD64PrintBool(e)
	fnOffsets["ts_print_undefined"] = len(e.Code)
	emitAMD64PrintLiteral(e, "undefined\n")
	fnOffsets["ts_print_null"] = len(e.Code)
	emitAMD64PrintLiteral(e, "null\n")

	// Emit ts_print_str for Linux AMD64
	fnOffsets["ts_print_str"] = len(e.Code)
	emitAMD64PrintStr(e)
	fnOffsets["ts_print_object"] = len(e.Code)
	emitAMD64PrintLiteral(e, "[object Object]\n")
	fnOffsets["ts_print_true"] = len(e.Code)
	emitAMD64PrintLiteral(e, "true\n")
	fnOffsets["ts_print_false"] = len(e.Code)
	emitAMD64PrintLiteral(e, "false\n")

	fnOffsets["ts_js_box_number"] = len(e.Code)
	emitAMD64JSBoxNumber(e)
	fnOffsets["ts_js_box_bool"] = len(e.Code)
	emitAMD64JSBoxBool(e)
	fnOffsets["ts_js_box_string"] = len(e.Code)
	emitAMD64JSBoxString(e)
	fnOffsets["ts_js_box_ref"] = len(e.Code)
	emitAMD64JSBoxRef(e)
	fnOffsets["ts_js_unbox_number"] = len(e.Code)
	emitAMD64JSUnboxNumber(e)
	fnOffsets["ts_js_unbox_bool"] = len(e.Code)
	emitAMD64JSUnboxBool(e)
	fnOffsets["ts_js_unbox_string"] = len(e.Code)
	emitAMD64JSUnboxString(e)
	fnOffsets["ts_js_unbox_ref"] = len(e.Code)
	emitAMD64JSUnboxRef(e)
	fnOffsets["ts_js_print"] = len(e.Code)
	emitAMD64JSPrint(e, fnOffsets["ts_print_val"], fnOffsets["ts_print_str"], fnOffsets["ts_print_undefined"], fnOffsets["ts_print_null"], fnOffsets["ts_print_object"], fnOffsets["ts_print_true"], fnOffsets["ts_print_false"])
	fnOffsets["ts_json_parse_scalar"] = len(e.Code)
	emitAMD64JSONParseScalar(e)
	fnOffsets["ts_js_string_to_number"] = len(e.Code)
	emitAMD64JSStringToNumber(e, fnOffsets["ts_json_parse_scalar"])

	fnOffsets["ts_runtime_init"] = len(e.Code)
	emitAMD64RuntimeInit(e)

	fnOffsets["ts_gc_mark_payload"] = len(e.Code)
	emitAMD64GCMarkPayload(e)
	fnOffsets["ts_gc_collect"] = len(e.Code)
	emitAMD64GCCollect(e, fnOffsets["ts_gc_mark_payload"])
	fnOffsets["ts_gc_collections"] = len(e.Code)
	emitAMD64GCMetricNumber(e, amd64RTCollections)
	fnOffsets["ts_gc_reclaimed"] = len(e.Code)
	emitAMD64GCMetricNumber(e, amd64RTReclaimed)
	fnOffsets["ts_gc_mapped_bytes"] = len(e.Code)
	emitAMD64GCMetricNumber(e, amd64RTMappedBytes)

	// Allocation reuses swept blocks first, then bumps in the current chunk,
	// collecting before a new mmap chunk is added.
	fnOffsets["ts_alloc"] = len(e.Code)
	emitAMD64Alloc(e, fnOffsets["ts_gc_collect"])
	fnOffsets["ts_task_trampoline"] = len(e.Code)
	emitAMD64TaskTrampoline(e)
	fnOffsets["ts_task_resume"] = len(e.Code)
	emitAMD64TaskResume(e, fnOffsets["ts_task_trampoline"])
	fnOffsets["ts_task_suspend"] = len(e.Code)
	emitAMD64TaskSuspend(e)
	fnOffsets["ts_task_spawn"] = len(e.Code)
	emitAMD64TaskSpawn(e, fnOffsets["ts_alloc"])
	fnOffsets["ts_task_run_one"] = len(e.Code)
	emitAMD64TaskRunOne(e, fnOffsets["ts_task_resume"])
	fnOffsets["ts_task_join"] = len(e.Code)
	emitAMD64TaskJoin(e, fnOffsets["ts_task_run_one"])
	fnOffsets["ts_task_yield"] = len(e.Code)
	emitAMD64TaskYield(e, fnOffsets["ts_task_run_one"], fnOffsets["ts_task_suspend"])
	fnOffsets["ts_task_sleep"] = len(e.Code)
	emitAMD64TaskSleep(e)
	fnOffsets["ts_channel_new"] = len(e.Code)
	emitAMD64ChannelNew(e, fnOffsets["ts_alloc"])
	fnOffsets["ts_channel_try_send"] = len(e.Code)
	emitAMD64ChannelTrySend(e)
	fnOffsets["ts_channel_try_recv_or"] = len(e.Code)
	emitAMD64ChannelTryRecvOr(e)
	fnOffsets["ts_channel_send"] = len(e.Code)
	emitAMD64ChannelSend(e, fnOffsets["ts_channel_try_send"], fnOffsets["ts_task_yield"])
	fnOffsets["ts_channel_recv"] = len(e.Code)
	emitAMD64ChannelRecv(e, fnOffsets["ts_task_yield"])

	fnOffsets["ts_number_to_string"] = len(e.Code)
	emitAMD64NumberToString(e, fnOffsets["ts_alloc"])
	fnOffsets["ts_bool_to_string"] = len(e.Code)
	emitAMD64BoolToString(e, fnOffsets["ts_alloc"])

	fnOffsets["ts_date_from_number"] = len(e.Code)
	emitAMD64DateFromNumber(e)
	fnOffsets["ts_date_now"] = len(e.Code)
	emitAMD64DateNow(e)
	dateGetters := [6]int{}
	for i, name := range []string{"ts_date_get_year", "ts_date_get_month", "ts_date_get_date", "ts_date_get_hours", "ts_date_get_minutes", "ts_date_get_seconds"} {
		fnOffsets[name] = len(e.Code)
		dateGetters[i] = len(e.Code)
		emitAMD64DateGetPart(e, i)
	}
	fnOffsets["ts_date_to_iso"] = len(e.Code)
	emitAMD64DateToISO(e, fnOffsets["ts_alloc"], dateGetters)

	fnOffsets["ts_object_new"] = len(e.Code)
	emitAMD64ObjectNew(e, fnOffsets["ts_alloc"])

	fnOffsets["ts_array_new"] = len(e.Code)
	emitAMD64ArrayNew(e, fnOffsets["ts_alloc"])
	fnOffsets["ts_array_get"] = len(e.Code)
	emitAMD64ArrayGet(e)
	fnOffsets["ts_array_set"] = len(e.Code)
	emitAMD64ArraySet(e, fnOffsets["ts_alloc"])
	fnOffsets["ts_array_len"] = len(e.Code)
	emitAMD64ArrayLength(e)
	fnOffsets["ts_array_push"] = len(e.Code)
	emitAMD64ArrayPush(e, fnOffsets["ts_alloc"])
	fnOffsets["ts_array_pop"] = len(e.Code)
	emitAMD64ArrayPop(e)

	fnOffsets["ts_string_eq"] = len(e.Code)
	emitAMD64StringEq(e)
	fnOffsets["ts_regexp_test"] = len(e.Code)
	emitAMD64RegExpTest(e)

	fnOffsets["ts_js_key_eq"] = len(e.Code)
	emitAMD64JSKeyEq(e, fnOffsets["ts_string_eq"])
	fnOffsets["ts_collection_new"] = len(e.Code)
	emitAMD64CollectionNew(e, fnOffsets["ts_alloc"])
	fnOffsets["ts_collection_find"] = len(e.Code)
	emitAMD64CollectionFind(e, fnOffsets["ts_js_key_eq"])
	fnOffsets["ts_collection_set"] = len(e.Code)
	emitAMD64CollectionSet(e, fnOffsets["ts_alloc"], fnOffsets["ts_collection_find"])
	fnOffsets["ts_collection_get"] = len(e.Code)
	emitAMD64CollectionGet(e, fnOffsets["ts_collection_find"])
	fnOffsets["ts_collection_has"] = len(e.Code)
	emitAMD64CollectionHas(e, fnOffsets["ts_collection_find"])
	fnOffsets["ts_collection_delete"] = len(e.Code)
	emitAMD64CollectionDelete(e, fnOffsets["ts_collection_find"])
	fnOffsets["ts_collection_clear"] = len(e.Code)
	emitAMD64CollectionClear(e)
	fnOffsets["ts_collection_size"] = len(e.Code)
	emitAMD64CollectionSize(e)

	fnOffsets["ts_dynamic_object_new"] = len(e.Code)
	emitAMD64DynamicObjectNew(e, fnOffsets["ts_alloc"])
	fnOffsets["ts_dynamic_get"] = len(e.Code)
	emitAMD64DynamicGet(e, fnOffsets["ts_string_eq"])
	fnOffsets["ts_dynamic_set"] = len(e.Code)
	emitAMD64DynamicSet(e, fnOffsets["ts_alloc"], fnOffsets["ts_string_eq"])

	fnOffsets["ts_string_concat"] = len(e.Code)
	emitAMD64StringConcat(e, fnOffsets["ts_alloc"])
	fnOffsets["ts_js_array_to_string"] = len(e.Code)
	emitAMD64JSArrayToString(e, fnOffsets["ts_js_array_to_string"], fnOffsets["ts_alloc"], fnOffsets["ts_number_to_string"], fnOffsets["ts_bool_to_string"], fnOffsets["ts_string_concat"])
	fnOffsets["ts_js_to_string"] = len(e.Code)
	emitAMD64JSToString(e, fnOffsets["ts_alloc"], fnOffsets["ts_number_to_string"], fnOffsets["ts_bool_to_string"], fnOffsets["ts_js_array_to_string"])
	fnOffsets["ts_js_add"] = len(e.Code)
	emitAMD64JSAdd(e, fnOffsets["ts_js_to_string"], fnOffsets["ts_string_concat"], fnOffsets["ts_js_box_number"])
	fnOffsets["ts_js_to_number"] = len(e.Code)
	emitAMD64JSToNumber(e, fnOffsets["ts_js_string_to_number"], fnOffsets["ts_js_array_to_string"])
	for _, spec := range []struct{ name, op string }{
		{"ts_js_sub", "sub"}, {"ts_js_mul", "mul"}, {"ts_js_div", "div"}, {"ts_js_mod", "mod"},
	} {
		fnOffsets[spec.name] = len(e.Code)
		emitAMD64JSNumericBinary(e, fnOffsets["ts_js_to_number"], spec.op)
	}
	fnOffsets["ts_js_strict_eq"] = len(e.Code)
	emitAMD64JSStrictEqual(e, fnOffsets["ts_string_eq"])
	fnOffsets["ts_js_loose_eq"] = len(e.Code)
	emitAMD64JSLooseEqual(e, fnOffsets["ts_js_loose_eq"], fnOffsets["ts_js_strict_eq"], fnOffsets["ts_js_to_number"], fnOffsets["ts_js_to_string"])
	fnOffsets["ts_js_string_compare"] = len(e.Code)
	emitAMD64JSStringCompare(e)
	for _, spec := range []struct{ name, op string }{
		{"ts_js_lt", "lt"}, {"ts_js_le", "le"}, {"ts_js_gt", "gt"}, {"ts_js_ge", "ge"},
	} {
		fnOffsets[spec.name] = len(e.Code)
		emitAMD64JSRelational(e, fnOffsets["ts_js_to_number"], fnOffsets["ts_js_to_string"], fnOffsets["ts_js_string_compare"], spec.op)
	}

	// Emit ts_sys_exit
	fnOffsets["ts_sys_exit"] = len(e.Code)
	emitAMD64SysExit(e)

	// Emit String Constants Table
	strOffsets := make(map[string]int)
	for _, sf := range strFixups {
		if _, exists := strOffsets[sf.str]; !exists {
			for len(e.Code)%8 != 0 {
				e.Code = append(e.Code, 0)
			}
			strOffsets[sf.str] = len(e.Code)

			var lenBuf [8]byte
			binary.LittleEndian.PutUint64(lenBuf[:], uint64(len(sf.str)))
			e.Code = append(e.Code, lenBuf[:]...)
			e.Code = append(e.Code, []byte(sf.str)...)
			e.Code = append(e.Code, 0)
		}
	}

	// Fix up string LEA instructions
	for _, sf := range strFixups {
		targetAddr := strOffsets[sf.str]
		disp := int32(targetAddr - (sf.offset + 4))
		binary.LittleEndian.PutUint32(e.Code[sf.offset:], uint32(disp))
	}

	// Fix up closure code pointers encoded as RIP-relative LEA instructions.
	for _, cf := range closureCodeFixups {
		targetAddr, exists := fnOffsets[cf.function]
		if !exists {
			return nil, fmt.Errorf("unresolved closure function %q", cf.function)
		}
		disp := int32(targetAddr - (cf.offset + 4))
		binary.LittleEndian.PutUint32(e.Code[cf.offset:], uint32(disp))
	}

	// Fix up function calls
	for _, cf := range callFixups {
		targetAddr, exists := fnOffsets[cf.callee]
		if !exists {
			return nil, fmt.Errorf("unresolved call target %q", cf.callee)
		}
		rel32 := int32(targetAddr - (cf.offset + 5))
		binary.LittleEndian.PutUint32(e.Code[cf.offset+1:], uint32(rel32))
	}

	// Fix up local branches
	for _, bf := range branchFixups {
		targetAddr, exists := bbOffsets[bf.targetBB]
		if !exists {
			return nil, fmt.Errorf("unresolved basic block target %p", bf.targetBB)
		}
		if bf.isCond {
			rel32 := int32(targetAddr - (bf.offset + 6))
			binary.LittleEndian.PutUint32(e.Code[bf.offset+2:], uint32(rel32))
		} else {
			rel32 := int32(targetAddr - (bf.offset + 5))
			binary.LittleEndian.PutUint32(e.Code[bf.offset+1:], uint32(rel32))
		}
	}

	return e.Code, nil
}

func emitAMD64PrintVal(e *amd64.Emitter) {
	patchJcc := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6))))
	}
	patchJmp := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5))))
	}
	emitByte := func(ptr amd64.Register, ch byte) {
		e.MovRegImm64(amd64.RDX, int64(ch))
		e.MovDerefReg8(ptr, 0, amd64.RDX)
		e.AddRegImm32(ptr, 1)
	}
	emitWriteAndReturn := func() {
		e.MovRegReg(amd64.RDX, amd64.R8)
		e.SubRegReg(amd64.RDX, amd64.R9)
		e.MovRegImm64(amd64.RDI, 1)
		e.MovRegReg(amd64.RSI, amd64.R9)
		e.MovRegImm64(amd64.RAX, 1)
		e.Syscall()
		e.MovRegReg(amd64.RSP, amd64.RBP)
		e.Pop(amd64.RBP)
		e.Ret()
	}

	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.SubRegImm32(amd64.RSP, 160)

	// Inspect the raw IEEE-754 payload before formatting.
	e.MovQRegXMM(amd64.RAX, amd64.XMM0)
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.MovRegImm64(amd64.R11, int64(0x7ff0000000000000))
	e.AndRegReg(amd64.R10, amd64.R11)
	e.CmpRegReg(amd64.R10, amd64.R11)
	specialJump := len(e.Code)
	e.JccRel32(amd64.CondE, 0)

	// Output buffer starts at RBP-128.
	e.MovRegReg(amd64.R8, amd64.RBP)
	e.SubRegImm32(amd64.R8, 128)
	e.MovRegReg(amd64.R9, amd64.R8)

	// Preserve and print the sign, then clear it in the working F64 payload.
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.MovRegImm64(amd64.R11, int64(-0x8000000000000000))
	e.AndRegReg(amd64.R10, amd64.R11)
	e.TestRegReg(amd64.R10, amd64.R10)
	noSign := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	emitByte(amd64.R8, '-')
	afterSign := len(e.Code)
	patchJcc(noSign, afterSign)

	e.MovRegImm64(amd64.R11, int64(0x7fffffffffffffff))
	e.AndRegReg(amd64.RAX, amd64.R11)
	e.MovQXMMReg(amd64.XMM0, amd64.RAX)

	// Split the finite value into integer and fractional parts.
	e.Cvttsd2si(amd64.RAX, amd64.XMM0)
	e.MovSDRegReg(amd64.XMM1, amd64.XMM0)
	e.Cvtsi2sd(amd64.XMM2, amd64.RAX)
	e.SubSD(amd64.XMM1, amd64.XMM2)

	// Build integer digits backwards in the upper end of the stack frame.
	e.MovRegReg(amd64.RSI, amd64.RBP)
	e.SubRegImm32(amd64.RSI, 1)
	e.MovRegImm64(amd64.RCX, 0)
	e.TestRegReg(amd64.RAX, amd64.RAX)
	integerNonZero := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.SubRegImm32(amd64.RSI, 1)
	e.MovRegImm64(amd64.RDX, '0')
	e.MovDerefReg8(amd64.RSI, 0, amd64.RDX)
	e.MovRegImm64(amd64.RCX, 1)
	integerReadyJump := len(e.Code)
	e.JmpRel32(0)

	integerLoop := len(e.Code)
	patchJcc(integerNonZero, integerLoop)
	e.MovRegImm64(amd64.R10, 10)
	e.Cqo()
	e.IdivReg(amd64.R10)
	e.AddRegImm32(amd64.RDX, '0')
	e.SubRegImm32(amd64.RSI, 1)
	e.MovDerefReg8(amd64.RSI, 0, amd64.RDX)
	e.AddRegImm32(amd64.RCX, 1)
	e.TestRegReg(amd64.RAX, amd64.RAX)
	integerLoopBack := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	patchJcc(integerLoopBack, integerLoop)

	integerReady := len(e.Code)
	patchJmp(integerReadyJump, integerReady)

	copyLoop := len(e.Code)
	e.MovzxRegDeref8(amd64.RDX, amd64.RSI, 0)
	e.MovDerefReg8(amd64.R8, 0, amd64.RDX)
	e.AddRegImm32(amd64.RSI, 1)
	e.AddRegImm32(amd64.R8, 1)
	e.SubRegImm32(amd64.RCX, 1)
	copyLoopBack := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	patchJcc(copyLoopBack, copyLoop)

	// Emit up to 12 fractional digits, then trim trailing zeroes. This keeps
	// the runtime compact while preserving ordinary binary64 arithmetic.
	e.XorPD(amd64.XMM3, amd64.XMM3)
	e.Ucomisd(amd64.XMM1, amd64.XMM3)
	noFraction := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	emitByte(amd64.R8, '.')
	e.MovRegImm64(amd64.R11, int64(0x4024000000000000)) // 10.0
	e.MovQXMMReg(amd64.XMM2, amd64.R11)
	e.MovRegImm64(amd64.RCX, 12)

	fractionLoop := len(e.Code)
	e.MulSD(amd64.XMM1, amd64.XMM2)
	e.Cvttsd2si(amd64.RAX, amd64.XMM1)
	e.MovRegReg(amd64.RDX, amd64.RAX)
	e.AddRegImm32(amd64.RDX, '0')
	e.MovDerefReg8(amd64.R8, 0, amd64.RDX)
	e.AddRegImm32(amd64.R8, 1)
	e.Cvtsi2sd(amd64.XMM3, amd64.RAX)
	e.SubSD(amd64.XMM1, amd64.XMM3)
	e.SubRegImm32(amd64.RCX, 1)
	e.XorPD(amd64.XMM3, amd64.XMM3)
	e.Ucomisd(amd64.XMM1, amd64.XMM3)
	fractionDone := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	e.TestRegReg(amd64.RCX, amd64.RCX)
	fractionBack := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	patchJcc(fractionBack, fractionLoop)

	fractionDigitsDone := len(e.Code)
	patchJcc(fractionDone, fractionDigitsDone)

	trimLoop := len(e.Code)
	e.MovRegReg(amd64.R10, amd64.R8)
	e.SubRegImm32(amd64.R10, 1)
	e.MovzxRegDeref8(amd64.RAX, amd64.R10, 0)
	e.CmpRegImm32(amd64.RAX, '0')
	trimDone := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)
	e.MovRegReg(amd64.R8, amd64.R10)
	trimBack := len(e.Code)
	e.JmpRel32(0)
	patchJmp(trimBack, trimLoop)
	afterTrim := len(e.Code)
	patchJcc(trimDone, afterTrim)

	afterFraction := len(e.Code)
	patchJcc(noFraction, afterFraction)
	emitByte(amd64.R8, '\n')
	emitWriteAndReturn()

	// Special IEEE-754 values.
	special := len(e.Code)
	patchJcc(specialJump, special)
	e.MovRegReg(amd64.R8, amd64.RBP)
	e.SubRegImm32(amd64.R8, 128)
	e.MovRegReg(amd64.R9, amd64.R8)

	// Mantissa != 0 means NaN. The sign of NaN is not printed.
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.MovRegImm64(amd64.R11, int64(0x000fffffffffffff))
	e.AndRegReg(amd64.R10, amd64.R11)
	e.TestRegReg(amd64.R10, amd64.R10)
	isNaN := len(e.Code)
	e.JccRel32(amd64.CondNE, 0)

	e.MovRegReg(amd64.R10, amd64.RAX)
	e.MovRegImm64(amd64.R11, int64(-0x8000000000000000))
	e.AndRegReg(amd64.R10, amd64.R11)
	e.TestRegReg(amd64.R10, amd64.R10)
	infNoSign := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	emitByte(amd64.R8, '-')
	infAfterSign := len(e.Code)
	patchJcc(infNoSign, infAfterSign)
	for _, ch := range []byte("Infinity") {
		emitByte(amd64.R8, ch)
	}
	emitByte(amd64.R8, '\n')
	emitWriteAndReturn()

	nanLabel := len(e.Code)
	patchJcc(isNaN, nanLabel)
	for _, ch := range []byte("NaN") {
		emitByte(amd64.R8, ch)
	}
	emitByte(amd64.R8, '\n')
	emitWriteAndReturn()
}
func emitAMD64PrintBool(e *amd64.Emitter) {
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.SubRegImm32(amd64.RSP, 16)
	e.MovRegReg(amd64.RAX, amd64.RDI)
	e.AddRegImm32(amd64.RAX, 48)
	e.MovDerefReg8(amd64.RSP, 0, amd64.RAX)
	e.MovRegImm64(amd64.RAX, 10)
	e.MovDerefReg8(amd64.RSP, 1, amd64.RAX)
	e.MovRegImm64(amd64.RDI, 1)
	e.MovRegReg(amd64.RSI, amd64.RSP)
	e.MovRegImm64(amd64.RDX, 2)
	e.MovRegImm64(amd64.RAX, 1)
	e.Syscall()
	e.MovRegReg(amd64.RSP, amd64.RBP)
	e.Pop(amd64.RBP)
	e.Ret()
}

func emitAMD64PrintLiteral(e *amd64.Emitter, text string) {
	size := ((len(text) + 15) / 16) * 16
	if size == 0 {
		size = 16
	}
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
	for _, off := range []int32{amd64RTCurrentTask, amd64RTSchedRsp, amd64RTSchedRbp, amd64RTSchedRbx, amd64RTSchedR12, amd64RTSchedR13, amd64RTSchedR14, amd64RTSchedRoot} {
		e.MovDerefReg(amd64.R15, off, amd64.R11)
	}
	e.Ret()
}

func emitAMD64Alloc(e *amd64.Emitter, gcOffset int) {
	patchJcc := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+2:], uint32(int32(target-(at+6))))
	}
	patchJmp := func(at, target int) {
		binary.LittleEndian.PutUint32(e.Code[at+1:], uint32(int32(target-(at+5))))
	}
	emitReturn := func() {
		e.Pop(amd64.R14)
		e.Pop(amd64.R13)
		e.Pop(amd64.R12)
		e.Pop(amd64.RBX)
		e.Pop(amd64.RBP)
		e.Ret()
	}
	emitFreeSearch := func() {
		e.MovRegImm64(amd64.R12, 0) // previous header
		e.MovRegDeref(amd64.R13, amd64.R15, amd64RTFreeList)
		loop := len(e.Code)
		e.TestRegReg(amd64.R13, amd64.R13)
		miss := len(e.Code)
		e.JccRel32(amd64.CondE, 0)
		e.MovRegDeref(amd64.R10, amd64.R13, amd64ObjectSize)
		e.CmpRegReg(amd64.R10, amd64.RBX)
		found := len(e.Code)
		e.JccRel32(amd64.CondAE, 0)
		e.MovRegReg(amd64.R12, amd64.R13)
		e.MovRegDeref(amd64.R13, amd64.R13, amd64ObjectNextFree)
		back := len(e.Code)
		e.JmpRel32(0)
		patchJmp(back, loop)

		foundLabel := len(e.Code)
		patchJcc(found, foundLabel)
		e.MovRegDeref(amd64.R14, amd64.R13, amd64ObjectNextFree)
		e.TestRegReg(amd64.R12, amd64.R12)
		hasPrev := len(e.Code)
		e.JccRel32(amd64.CondNE, 0)
		e.MovDerefReg(amd64.R15, amd64RTFreeList, amd64.R14)
		unlinked := len(e.Code)
		e.JmpRel32(0)
		hasPrevLabel := len(e.Code)
		patchJcc(hasPrev, hasPrevLabel)
		e.MovDerefReg(amd64.R12, amd64ObjectNextFree, amd64.R14)
		unlinkDone := len(e.Code)
		patchJmp(unlinked, unlinkDone)
		e.MovRegImm64(amd64.R10, 0)
		e.MovDerefReg(amd64.R13, amd64ObjectFlags, amd64.R10)
		e.MovDerefReg(amd64.R13, amd64ObjectNextFree, amd64.R10)
		e.MovDerefReg(amd64.R13, amd64ObjectType, amd64.R10)
		e.MovRegReg(amd64.RAX, amd64.R13)
		e.AddRegImm32(amd64.RAX, amd64ObjectHeaderSize)
		emitReturn()

		missLabel := len(e.Code)
		patchJcc(miss, missLabel)
	}

	// Preserve callee-saved temporaries and keep call sites 16-byte aligned.
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.Push(amd64.RBX)
	e.Push(amd64.R12)
	e.Push(amd64.R13)
	e.Push(amd64.R14)

	// RBX is the aligned total object size, including its 32-byte header.
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.AddRegImm32(amd64.RBX, amd64ObjectHeaderSize+15)
	e.MovRegImm64(amd64.R11, -16)
	e.AndRegReg(amd64.RBX, amd64.R11)

	// Reclaimed blocks are preferred over fresh bump space.
	emitFreeSearch()

	// Fast bump allocation from the current chunk.
	e.MovRegDeref(amd64.RAX, amd64.R15, amd64RTCursor)
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.AddRegReg(amd64.R10, amd64.RBX)
	e.MovRegDeref(amd64.R11, amd64.R15, amd64RTEnd)
	e.CmpRegReg(amd64.R10, amd64.R11)
	collect := len(e.Code)
	e.JccRel32(amd64.CondA, 0)
	emitAMD64InitObjectHeader(e, amd64.RAX, amd64.RBX)
	e.MovDerefReg(amd64.R15, amd64RTCursor, amd64.R10)
	e.MovRegDeref(amd64.R11, amd64.R15, amd64RTChunkHead)
	e.MovDerefReg(amd64.R11, amd64ChunkUsed, amd64.R10)
	e.AddRegImm32(amd64.RAX, amd64ObjectHeaderSize)
	emitReturn()

	// On pressure, collect before mapping another chunk.
	collectLabel := len(e.Code)
	patchJcc(collect, collectLabel)
	callAt := len(e.Code)
	e.CallRel32(int32(gcOffset - (callAt + 5)))
	emitFreeSearch()

	// No reusable block fits. Refill with max(1 MiB, object + chunk header).
	e.MovRegReg(amd64.RSI, amd64.RBX)
	e.AddRegImm32(amd64.RSI, amd64ChunkSize)
	e.CmpRegImm32(amd64.RSI, 1<<20)
	large := len(e.Code)
	e.JccRel32(amd64.CondGE, 0)
	e.MovRegImm64(amd64.RSI, 1<<20)
	mapChunk := len(e.Code)
	patchJcc(large, mapChunk)
	e.MovRegImm64(amd64.RDI, 0)
	e.MovRegImm64(amd64.RDX, 3)
	e.MovRegImm64(amd64.R10, 0x22)
	e.MovRegImm64(amd64.R8, -1)
	e.MovRegImm64(amd64.R9, 0)
	e.MovRegImm64(amd64.RAX, 9)
	e.Syscall()

	// Link and initialize the new chunk.
	e.MovRegDeref(amd64.R11, amd64.R15, amd64RTChunkHead)
	e.MovDerefReg(amd64.RAX, amd64ChunkNext, amd64.R11)
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.AddRegReg(amd64.R10, amd64.RSI)
	e.MovDerefReg(amd64.RAX, amd64ChunkEnd, amd64.R10)
	e.MovRegReg(amd64.R11, amd64.RAX)
	e.AddRegImm32(amd64.R11, amd64ChunkSize)
	e.MovRegReg(amd64.R10, amd64.R11)
	e.AddRegReg(amd64.R10, amd64.RBX)
	e.MovDerefReg(amd64.RAX, amd64ChunkUsed, amd64.R10)
	e.MovDerefReg(amd64.R15, amd64RTChunkHead, amd64.RAX)
	e.MovDerefReg(amd64.R15, amd64RTCursor, amd64.R10)
	e.MovRegDeref(amd64.R10, amd64.RAX, amd64ChunkEnd)
	e.MovDerefReg(amd64.R15, amd64RTEnd, amd64.R10)
	e.MovRegDeref(amd64.R10, amd64.R15, amd64RTMappedBytes)
	e.AddRegReg(amd64.R10, amd64.RSI)
	e.MovDerefReg(amd64.R15, amd64RTMappedBytes, amd64.R10)

	// First object in the new chunk.
	e.MovRegReg(amd64.RAX, amd64.R11)
	emitAMD64InitObjectHeader(e, amd64.RAX, amd64.RBX)
	e.AddRegImm32(amd64.RAX, amd64ObjectHeaderSize)
	emitReturn()
}

func emitAMD64InitObjectHeader(e *amd64.Emitter, header, total amd64.Register) {
	e.MovDerefReg(header, amd64ObjectSize, total)
	e.MovRegImm64(amd64.R11, 0)
	e.MovDerefReg(header, amd64ObjectFlags, amd64.R11)
	e.MovDerefReg(header, amd64ObjectNextFree, amd64.R11)
	e.MovDerefReg(header, amd64ObjectType, amd64.R11)
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

	// dst = result + 8, src = a + 8
	e.AddRegImm32(amd64.R10, 8)
	e.MovRegReg(amd64.R8, amd64.RBX)
	e.AddRegImm32(amd64.R8, 8)
	e.MovRegImm64(amd64.R9, 0)
	e.TestRegReg(amd64.R13, amd64.R13)
	skipA := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	loopA := len(e.Code)
	e.MovzxRegDeref8(amd64.RAX, amd64.R8, 0)
	e.MovDerefReg8(amd64.R10, 0, amd64.RAX)
	e.AddRegImm32(amd64.R8, 1)
	e.AddRegImm32(amd64.R10, 1)
	e.AddRegImm32(amd64.R9, 1)
	e.CmpRegReg(amd64.R9, amd64.R13)
	backA := len(e.Code)
	e.JccRel32(amd64.CondL, 0)
	binary.LittleEndian.PutUint32(e.Code[backA+2:], uint32(int32(loopA-(backA+6))))
	afterA := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[skipA+2:], uint32(int32(afterA-(skipA+6))))

	// src = b + 8; dst already follows a.
	e.MovRegReg(amd64.R8, amd64.R12)
	e.AddRegImm32(amd64.R8, 8)
	e.MovRegImm64(amd64.R9, 0)
	e.TestRegReg(amd64.R14, amd64.R14)
	skipB := len(e.Code)
	e.JccRel32(amd64.CondE, 0)
	loopB := len(e.Code)
	e.MovzxRegDeref8(amd64.RAX, amd64.R8, 0)
	e.MovDerefReg8(amd64.R10, 0, amd64.RAX)
	e.AddRegImm32(amd64.R8, 1)
	e.AddRegImm32(amd64.R10, 1)
	e.AddRegImm32(amd64.R9, 1)
	e.CmpRegReg(amd64.R9, amd64.R14)
	backB := len(e.Code)
	e.JccRel32(amd64.CondL, 0)
	binary.LittleEndian.PutUint32(e.Code[backB+2:], uint32(int32(loopB-(backB+6))))
	afterB := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[skipB+2:], uint32(int32(afterB-(skipB+6))))

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
