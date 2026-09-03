package lower

import (
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

// Allocatable scratch registers for regalloc
var amd64ScratchRegs = []amd64.Register{
	amd64.RAX, amd64.RBX, amd64.R10, amd64.R11, amd64.R12, amd64.R13, amd64.R14, amd64.R15,
}

func lowerAMD64(prog *ir.Program) ([]byte, error) {
	e := amd64.NewEmitter()

	for _, fn := range prog.Functions {
		// Run register allocation
		ra := regalloc.New(len(amd64ScratchRegs))
		locs := ra.Allocate(fn)

		// Prologue:
		// push rbp
		// mov rbp, rsp
		e.Push(amd64.RBP)
		e.MovRegReg(amd64.RBP, amd64.RSP)

		// Copy incoming parameters from ABI registers into allocated locations
		for i, param := range fn.Params {
			if i < len(amd64ParamRegs) {
				loc := locs[param.ID]
				if loc.IsReg {
					targetReg := amd64ScratchRegs[loc.Reg]
					e.MovRegReg(targetReg, amd64ParamRegs[i])
				}
			}
		}

		// Emit instructions
		for _, bb := range fn.Blocks {
			for _, inst := range bb.Instructions {
				switch bi := inst.(type) {
				case *ir.BinaryInst:
					dstLoc := locs[bi.Res.ID]
					if !dstLoc.IsReg {
						continue
					}
					dstReg := amd64ScratchRegs[dstLoc.Reg]

					// Move LHS to dstReg if needed
					if vLHS, ok := bi.LHS.(*ir.Value); ok {
						srcLoc := locs[vLHS.ID]
						if srcLoc.IsReg {
							e.MovRegReg(dstReg, amd64ScratchRegs[srcLoc.Reg])
						}
					} else if cLHS, ok := bi.LHS.(ir.ConstNumber); ok {
						e.MovRegImm64(dstReg, int64(cLHS.Value))
					}

					// Op with RHS
					if vRHS, ok := bi.RHS.(*ir.Value); ok {
						rhsLoc := locs[vRHS.ID]
						if rhsLoc.IsReg {
							rhsReg := amd64ScratchRegs[rhsLoc.Reg]
							switch bi.Op {
							case ir.OpAdd:
								e.AddRegReg(dstReg, rhsReg)
							case ir.OpSub:
								e.SubRegReg(dstReg, rhsReg)
							}
						}
					}
				}
			}

			if bb.Terminator != nil {
				if ret, ok := bb.Terminator.(*ir.ReturnTerm); ok {
					if ret.Val != nil {
						if v, ok := ret.Val.(*ir.Value); ok {
							loc := locs[v.ID]
							if loc.IsReg {
								srcReg := amd64ScratchRegs[loc.Reg]
								if srcReg != amd64.RAX {
									e.MovRegReg(amd64.RAX, srcReg)
								}
							}
						} else if c, ok := ret.Val.(ir.ConstNumber); ok {
							e.MovRegImm64(amd64.RAX, int64(c.Value))
						}
					}
					// Epilogue:
					// mov rsp, rbp
					// pop rbp
					// ret
					e.MovRegReg(amd64.RSP, amd64.RBP)
					e.Pop(amd64.RBP)
					e.Ret()
				}
			}
		}
	}

	return e.Code, nil
}

func lowerARM64(prog *ir.Program) ([]byte, error) {
	e := arm64.NewEmitter()

	for _, fn := range prog.Functions {
		// Simple prologue for leaf function
		for _, bb := range fn.Blocks {
			for _, inst := range bb.Instructions {
				switch bi := inst.(type) {
				case *ir.BinaryInst:
					switch bi.Op {
					case ir.OpAdd:
						e.Add(arm64.X0, arm64.X0, arm64.X1)
					case ir.OpSub:
						e.Sub(arm64.X0, arm64.X0, arm64.X1)
					}
				}
			}
			if bb.Terminator != nil {
				if _, ok := bb.Terminator.(*ir.ReturnTerm); ok {
					e.Ret()
				}
			}
		}
	}

	return e.Code, nil
}
