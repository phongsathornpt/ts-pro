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

func isNumberType(t types.Type) bool { return t != nil && t.Kind() == types.KindNumber }
func numberBits(v float64) int64     { return int64(math.Float64bits(v)) }

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
	e.SubRegImm32(amd64.RSP, 32)
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
		// After CALL, push RBP + five callee-saved registers leaves RSP at 8 mod 16.
		// Choose a frame size that is 8 mod 16 so call sites remain 16-byte aligned.
		frameSize := int32(((spillBytes + 23) &^ 15) - 8)
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
			default:
				return scratch, fmt.Errorf("unsupported AMD64 operand %T", op)
			}
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

		// Classify incoming SysV parameters. Number values use the SSE class;
		// references/booleans use the integer class. Overflow arguments are read
		// from the caller stack in source order.
		gprParam, xmmParam, stackParam := 0, 0, 0
		for _, param := range fn.Params {
			if isNumberType(param.Type()) {
				if xmmParam < len(amd64NumberParamRegs) {
					e.MovQRegXMM(amd64.R10, amd64NumberParamRegs[xmmParam])
					storeValue(locs[param.ID], amd64.R10)
					xmmParam++
				} else {
					e.MovRegDeref(amd64.R10, amd64.RBP, int32(16+stackParam*8))
					storeValue(locs[param.ID], amd64.R10)
					stackParam++
				}
				continue
			}
			if gprParam < len(amd64ParamRegs) {
				storeValue(locs[param.ID], amd64ParamRegs[gprParam])
				gprParam++
			} else {
				e.MovRegDeref(amd64.R10, amd64.RBP, int32(16+stackParam*8))
				storeValue(locs[param.ID], amd64.R10)
				stackParam++
			}
		}

		for _, bb := range fn.Blocks {
			bbOffsets[bb] = len(e.Code)

			for _, inst := range bb.Instructions {
				switch bi := inst.(type) {
				case *ir.BinaryInst:
					dstLoc := locs[bi.Res.ID]
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
					for _, arg := range bi.Args {
						if isNumberType(arg.Type()) {
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
							storeValue(locs[bi.Res.ID], amd64.R10)
						} else {
							storeValue(locs[bi.Res.ID], amd64.RAX)
						}
					}

				}
			}

			if bb.Terminator != nil {
				switch term := bb.Terminator.(type) {
				case *ir.ReturnTerm:
					if term.Val != nil {
						if isNumberType(term.Val.Type()) {
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

					// Epilogue: restore callee-saved registers and RBP
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
								dstLoc := locs[phi.Res.ID]
								if v, ok := inc.Value.(*ir.Value); ok {
									srcReg := loadValue(v, amd64.R10)
									storeValue(dstLoc, srcReg)
								} else if c, ok := inc.Value.(ir.ConstNumber); ok {
									e.MovRegImm64(amd64.R10, numberBits(c.Value))
									storeValue(dstLoc, amd64.R10)
								} else if c, ok := inc.Value.(ir.ConstBool); ok {
									if c.Value {
										e.MovRegImm64(amd64.R10, 1)
									} else {
										e.MovRegImm64(amd64.R10, 0)
									}
									storeValue(dstLoc, amd64.R10)
								} else if c, ok := inc.Value.(ir.ConstString); ok {
									strOffset := len(e.Code)
									e.LeaRipRel32(amd64.R10, 0)
									strFixups = append(strFixups, stringFixupAMD64{offset: strOffset + 3, targetReg: amd64.R10, str: c.Value})
									storeValue(dstLoc, amd64.R10)
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

	// Emit ts_print_str for Linux AMD64
	fnOffsets["ts_print_str"] = len(e.Code)
	emitAMD64PrintStr(e)

	fnOffsets["ts_runtime_init"] = len(e.Code)
	emitAMD64RuntimeInit(e)

	// Runtime allocations use a 16-byte-aligned bump arena and mmap only when a
	// region must be refilled, rather than mapping once per allocation.
	fnOffsets["ts_alloc"] = len(e.Code)
	emitAMD64Alloc(e)

	fnOffsets["ts_string_concat"] = len(e.Code)
	emitAMD64StringConcat(e, fnOffsets["ts_alloc"])

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

func emitAMD64RuntimeInit(e *amd64.Emitter) {
	// R15 points at a 32-byte process-lifetime runtime context. Seed it with a
	// 1 MiB RW arena: [0]=cursor, [8]=end.
	e.MovRegImm64(amd64.RDI, 0)
	e.MovRegImm64(amd64.RSI, 1<<20)
	e.MovRegImm64(amd64.RDX, 3)
	e.MovRegImm64(amd64.R10, 0x22)
	e.MovRegImm64(amd64.R8, -1)
	e.MovRegImm64(amd64.R9, 0)
	e.MovRegImm64(amd64.RAX, 9)
	e.Syscall()
	e.MovDerefReg(amd64.R15, 0, amd64.RAX)
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.AddRegImm32(amd64.R10, 1<<20)
	e.MovDerefReg(amd64.R15, 8, amd64.R10)
	e.Ret()
}

func emitAMD64Alloc(e *amd64.Emitter) {
	// Keep the aligned request in callee-saved RBX across a potential mmap.
	e.Push(amd64.RBX)
	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.AddRegImm32(amd64.RBX, 15)
	e.MovRegImm64(amd64.R11, -16)
	e.AndRegReg(amd64.RBX, amd64.R11)

	// Fast bump allocation from the current arena.
	e.MovRegDeref(amd64.RAX, amd64.R15, 0)
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.AddRegReg(amd64.R10, amd64.RBX)
	e.MovRegDeref(amd64.R11, amd64.R15, 8)
	e.CmpRegReg(amd64.R10, amd64.R11)
	fast := len(e.Code)
	e.JccRel32(amd64.CondBE, 0)

	// Refill with max(request, 1 MiB).
	e.MovRegReg(amd64.RSI, amd64.RBX)
	e.CmpRegImm32(amd64.RSI, 1<<20)
	large := len(e.Code)
	e.JccRel32(amd64.CondGE, 0)
	e.MovRegImm64(amd64.RSI, 1<<20)
	mapChunk := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[large+2:], uint32(int32(mapChunk-(large+6))))
	e.MovRegImm64(amd64.RDI, 0)
	e.MovRegImm64(amd64.RDX, 3)
	e.MovRegImm64(amd64.R10, 0x22)
	e.MovRegImm64(amd64.R8, -1)
	e.MovRegImm64(amd64.R9, 0)
	e.MovRegImm64(amd64.RAX, 9)
	e.Syscall()

	// First object starts at the new mapping base; publish the remaining arena.
	e.MovRegReg(amd64.R10, amd64.RAX)
	e.AddRegReg(amd64.R10, amd64.RBX)
	e.MovDerefReg(amd64.R15, 0, amd64.R10)
	e.MovRegReg(amd64.R11, amd64.RAX)
	e.AddRegReg(amd64.R11, amd64.RSI)
	e.MovDerefReg(amd64.R15, 8, amd64.R11)
	e.Pop(amd64.RBX)
	e.Ret()

	fastPath := len(e.Code)
	binary.LittleEndian.PutUint32(e.Code[fast+2:], uint32(int32(fastPath-(fast+6))))
	e.MovDerefReg(amd64.R15, 0, amd64.R10)
	e.Pop(amd64.RBX)
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
	e.SubRegImm32(amd64.RSP, 8)

	e.MovRegReg(amd64.RBX, amd64.RDI)
	e.MovRegReg(amd64.R12, amd64.RSI)
	e.MovRegDeref(amd64.R13, amd64.RBX, 0)
	e.MovRegDeref(amd64.R14, amd64.R12, 0)
	e.MovRegReg(amd64.RDI, amd64.R13)
	e.AddRegReg(amd64.RDI, amd64.R14)
	e.AddRegImm32(amd64.RDI, 8)
	callAt := len(e.Code)
	e.CallRel32(int32(allocOffset - (callAt + 5)))
	e.MovRegReg(amd64.R15, amd64.RAX)

	e.MovRegReg(amd64.R11, amd64.R13)
	e.AddRegReg(amd64.R11, amd64.R14)
	e.MovDerefReg(amd64.R15, 0, amd64.R11)

	// dst = result + 8, src = a + 8
	e.MovRegReg(amd64.R10, amd64.R15)
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

	e.MovRegReg(amd64.RAX, amd64.R15)
	e.AddRegImm32(amd64.RSP, 8)
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
