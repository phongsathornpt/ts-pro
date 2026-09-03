package lower

import (
	"encoding/binary"
	"fmt"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
	"github.com/phongsathornpt/ts-pro/internal/backend/asm/arm64"
	"github.com/phongsathornpt/ts-pro/internal/backend/regalloc"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
)

type Arch string

const (
	ArchAMD64 Arch = "amd64"
	ArchARM64 Arch = "arm64"
)

// Lower lowers an IR program into target machine code.
func Lower(prog *ir.Program, target Arch) ([]byte, error) {
	switch target {
	case ArchAMD64:
		return lowerAMD64(prog)
	case ArchARM64:
		return lowerARM64(prog)
	default:
		return nil, fmt.Errorf("unsupported architecture: %s", target)
	}
}

// System V AMD64 parameter registers
var amd64ParamRegs = []amd64.Register{
	amd64.RDI, amd64.RSI, amd64.RDX, amd64.RCX, amd64.R8, amd64.R9,
}

// Callee-saved scratch registers for AMD64 regalloc (preserved across calls)
var amd64ScratchRegs = []amd64.Register{
	amd64.RBX, amd64.R12, amd64.R13, amd64.R14, amd64.R15,
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
	targetBB string
	isCond   bool
	condReg  arm64.Register
}

type branchFixupAMD64 struct {
	offset   int
	targetBB string
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
	bbOffsets := make(map[string]int)

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
			bbOffsets[bb.Name] = len(e.Code)

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
						e.Movz(arm64.X8, uint16(cLHS.Value))
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
						e.Movz(arm64.X16, uint16(cRHS.Value))
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
								e.Movz(targetParam, uint16(cArg.Value))
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
							e.Movz(arm64.X0, uint16(c.Value))
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
						targetBB: term.Then.Name,
						isCond:   true,
						condReg:  condReg,
					})

					// Else branch
					jumpOffset := len(e.Code)
					e.B(0)
					branchFixups = append(branchFixups, branchFixupARM64{
						offset:   jumpOffset,
						targetBB: term.Else.Name,
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
										e.Movz(dstReg, uint16(c.Value))
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
						targetBB: term.Target.Name,
						isCond:   false,
					})
				}
			}
		}
	}

	// Emit ts_print_val (decimal printer)
	fnOffsets["ts_print_val"] = len(e.Code)
	emitARM64PrintVal(e)

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
			continue
		}
		wordOffset := int32((targetAddr - cf.offset) / 4)
		binary.LittleEndian.PutUint32(e.Code[cf.offset:], 0x94000000|(uint32(wordOffset)&0x03FFFFFF))
	}

	// Fix up local branches
	for _, bf := range branchFixups {
		targetAddr, exists := bbOffsets[bf.targetBB]
		if !exists {
			continue
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
	bbOffsets := make(map[string]int)

	for _, fn := range prog.Functions {
		fnOffsets[fn.Name] = len(e.Code)

		ra := regalloc.New(len(amd64ScratchRegs))
		locs := ra.Allocate(fn)

		// Prologue: save RBP and callee-saved registers RBX, R12-R15
		e.Push(amd64.RBP)
		e.MovRegReg(amd64.RBP, amd64.RSP)
		e.Push(amd64.RBX)
		e.Push(amd64.R12)
		e.Push(amd64.R13)
		e.Push(amd64.R14)
		e.Push(amd64.R15)
		e.SubRegImm32(amd64.RSP, 8)

		// Copy incoming parameters
		for i, param := range fn.Params {
			if i < len(amd64ParamRegs) {
				loc := locs[param.ID]
				if loc.IsReg {
					targetReg := amd64ScratchRegs[loc.Reg]
					e.MovRegReg(targetReg, amd64ParamRegs[i])
				}
			}
		}

		for _, bb := range fn.Blocks {
			bbOffsets[bb.Name] = len(e.Code)

			for _, inst := range bb.Instructions {
				switch bi := inst.(type) {
				case *ir.BinaryInst:
					dstLoc := locs[bi.Res.ID]
					if !dstLoc.IsReg {
						continue
					}
					dstReg := amd64ScratchRegs[dstLoc.Reg]

					if vLHS, ok := bi.LHS.(*ir.Value); ok {
						srcLoc := locs[vLHS.ID]
						if srcLoc.IsReg {
							e.MovRegReg(dstReg, amd64ScratchRegs[srcLoc.Reg])
						}
					} else if cLHS, ok := bi.LHS.(ir.ConstNumber); ok {
						e.MovRegImm64(dstReg, int64(cLHS.Value))
					}

					var rhsReg amd64.Register = amd64.RAX
					if vRHS, ok := bi.RHS.(*ir.Value); ok {
						rhsLoc := locs[vRHS.ID]
						if rhsLoc.IsReg {
							rhsReg = amd64ScratchRegs[rhsLoc.Reg]
						}
					} else if cRHS, ok := bi.RHS.(ir.ConstNumber); ok {
						e.MovRegImm64(amd64.RAX, int64(cRHS.Value))
						rhsReg = amd64.RAX
					}

					switch bi.Op {
					case ir.OpAdd:
						e.AddRegReg(dstReg, rhsReg)
					case ir.OpSub:
						e.SubRegReg(dstReg, rhsReg)
					case ir.OpMul:
						e.ImulRegReg(dstReg, rhsReg)
					case ir.OpLt:
						e.CmpRegReg(dstReg, rhsReg)
						e.Setcc(amd64.CondL, dstReg)
					case ir.OpLe:
						e.CmpRegReg(dstReg, rhsReg)
						e.Setcc(amd64.CondLE, dstReg)
					case ir.OpGt:
						e.CmpRegReg(dstReg, rhsReg)
						e.Setcc(amd64.CondG, dstReg)
					case ir.OpGe:
						e.CmpRegReg(dstReg, rhsReg)
						e.Setcc(amd64.CondGE, dstReg)
					case ir.OpEq:
						e.CmpRegReg(dstReg, rhsReg)
						e.Setcc(amd64.CondE, dstReg)
					case ir.OpNe:
						e.CmpRegReg(dstReg, rhsReg)
						e.Setcc(amd64.CondNE, dstReg)
					}

				case *ir.CallInst:
					for i, arg := range bi.Args {
						if i < len(amd64ParamRegs) {
							targetParam := amd64ParamRegs[i]
							if vArg, ok := arg.(*ir.Value); ok {
								argLoc := locs[vArg.ID]
								if argLoc.IsReg {
									e.MovRegReg(targetParam, amd64ScratchRegs[argLoc.Reg])
								}
							} else if cArg, ok := arg.(ir.ConstNumber); ok {
								e.MovRegImm64(targetParam, int64(cArg.Value))
							} else if sArg, ok := arg.(ir.ConstString); ok {
								strOffset := len(e.Code)
								e.LeaRipRel32(targetParam, 0)
								strFixups = append(strFixups, stringFixupAMD64{
									offset:    strOffset + 3,
									targetReg: targetParam,
									str:       sArg.Value,
								})
							}
						}
					}

					callOffset := len(e.Code)
					e.CallRel32(0)
					callFixups = append(callFixups, callFixup{offset: callOffset, callee: bi.Callee})

					if bi.Res != nil {
						dstLoc := locs[bi.Res.ID]
						if dstLoc.IsReg {
							dstReg := amd64ScratchRegs[dstLoc.Reg]
							e.MovRegReg(dstReg, amd64.RAX)
						}
					}
				}
			}

			if bb.Terminator != nil {
				switch term := bb.Terminator.(type) {
				case *ir.ReturnTerm:
					if term.Val != nil {
						if v, ok := term.Val.(*ir.Value); ok {
							loc := locs[v.ID]
							if loc.IsReg {
								srcReg := amd64ScratchRegs[loc.Reg]
								if srcReg != amd64.RAX {
									e.MovRegReg(amd64.RAX, srcReg)
								}
							}
						} else if c, ok := term.Val.(ir.ConstNumber); ok {
							e.MovRegImm64(amd64.RAX, int64(c.Value))
						} else if s, ok := term.Val.(ir.ConstString); ok {
							strOffset := len(e.Code)
							e.LeaRipRel32(amd64.RAX, 0)
							strFixups = append(strFixups, stringFixupAMD64{
								offset:    strOffset + 3,
								targetReg: amd64.RAX,
								str:       s.Value,
							})
						}
					}

					if fn.Name == "@main" {
						callOffset := len(e.Code)
						e.CallRel32(0)
						callFixups = append(callFixups, callFixup{offset: callOffset, callee: "ts_sys_exit"})
					}

					// Epilogue: restore callee-saved registers and RBP
					e.AddRegImm32(amd64.RSP, 8)
					e.Pop(amd64.R15)
					e.Pop(amd64.R14)
					e.Pop(amd64.R13)
					e.Pop(amd64.R12)
					e.Pop(amd64.RBX)
					e.Pop(amd64.RBP)
					e.Ret()

				case *ir.BranchTerm:
					var condReg amd64.Register = amd64.RAX
					if vCond, ok := term.Cond.(*ir.Value); ok {
						loc := locs[vCond.ID]
						if loc.IsReg {
							condReg = amd64ScratchRegs[loc.Reg]
						}
					}
					// cmp condReg, 0
					e.CmpRegReg(condReg, condReg)
					branchOffset := len(e.Code)
					e.JccRel32(amd64.CondNE, 0)
					branchFixups = append(branchFixups, branchFixupAMD64{
						offset:   branchOffset,
						targetBB: term.Then.Name,
						isCond:   true,
						condReg:  condReg,
					})

					jumpOffset := len(e.Code)
					e.JmpRel32(0)
					branchFixups = append(branchFixups, branchFixupAMD64{
						offset:   jumpOffset,
						targetBB: term.Else.Name,
						isCond:   false,
					})

				case *ir.JumpTerm:
					for _, phi := range term.Target.Phis {
						for _, inc := range phi.Incoming {
							if inc.Block == bb {
								dstLoc := locs[phi.Res.ID]
								if dstLoc.IsReg {
									dstReg := amd64ScratchRegs[dstLoc.Reg]
									if v, ok := inc.Value.(*ir.Value); ok {
										srcLoc := locs[v.ID]
										if srcLoc.IsReg {
											srcReg := amd64ScratchRegs[srcLoc.Reg]
											if dstReg != srcReg {
												e.MovRegReg(dstReg, srcReg)
											}
										}
									} else if c, ok := inc.Value.(ir.ConstNumber); ok {
										e.MovRegImm64(dstReg, int64(c.Value))
									}
								}
							}
						}
					}

					jumpOffset := len(e.Code)
					e.JmpRel32(0)
					branchFixups = append(branchFixups, branchFixupAMD64{
						offset:   jumpOffset,
						targetBB: term.Target.Name,
						isCond:   false,
					})
				}
			}
		}
	}

	// Emit ts_print_val for Linux AMD64
	fnOffsets["ts_print_val"] = len(e.Code)
	emitAMD64PrintVal(e)

	// Emit ts_print_str for Linux AMD64
	fnOffsets["ts_print_str"] = len(e.Code)
	emitAMD64PrintStr(e)

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
			continue
		}
		rel32 := int32(targetAddr - (cf.offset + 5))
		binary.LittleEndian.PutUint32(e.Code[cf.offset+1:], uint32(rel32))
	}

	// Fix up local branches
	for _, bf := range branchFixups {
		targetAddr, exists := bbOffsets[bf.targetBB]
		if !exists {
			continue
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
	// Push RBP, Mov RBP, RSP
	e.Push(amd64.RBP)
	e.MovRegReg(amd64.RBP, amd64.RSP)
	e.SubRegImm32(amd64.RSP, 48)

	// RDI has the value to print.
	// We'll write to buffer at RBP - 32 ... RBP - 1
	// Set RBP-1 to '\n'
	e.MovDerefReg(amd64.RBP, -1, amd64.RAX) // placeholder

	// Epilogue
	e.MovRegReg(amd64.RSP, amd64.RBP)
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
