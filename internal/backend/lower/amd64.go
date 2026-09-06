package lower

import (
	"fmt"

	"github.com/phongsathornpt/ts-pro/internal/backend/asm/amd64"
	"github.com/phongsathornpt/ts-pro/internal/backend/regalloc"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

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
	e.SubRegImm32(amd64.RSP, 256)
	e.MovRegReg(amd64.R15, amd64.RSP)
	initOffset := len(e.Code)
	e.CallRel32(0)
	callFixups = append(callFixups, callFixup{offset: initOffset, callee: "ts_runtime_init"})
	if hasMain {
		callOffset := len(e.Code)
		e.CallRel32(0)
		callFixups = append(callFixups, callFixup{offset: callOffset, callee: "@main"})
	}
	drainOffset := len(e.Code)
	e.CallRel32(0)
	callFixups = append(callFixups, callFixup{offset: drainOffset, callee: "ts_task_drain"})
	exitOffset := len(e.Code)
	e.CallRel32(0)
	callFixups = append(callFixups, callFixup{offset: exitOffset, callee: "ts_sys_exit"})

	for _, fn := range prog.Functions {
		fnOffsets[fn.Name] = len(e.Code)

		ra := regalloc.New(len(amd64ScratchRegs))
		locs := ra.Allocate(fn)
		spillBytes := ra.StackFrameSlots() * 8
		rootLiveOut := amd64RootLiveOut(fn)
		rootSlots := amd64RootSlotsWithLiveOut(fn, rootLiveOut)
		rootSlotCount := amd64RootSlotCount(rootSlots)
		rootFrameBytes := 0
		rootFrameBaseOffset := int32(0)
		if rootSlotCount != 0 {
			rootFrameBytes = 16 + rootSlotCount*8
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
			if rootSlotCount == 0 {
				return
			}
			e.MovRegDeref(amd64.R10, amd64.RBP, rootFrameBaseOffset)
			e.MovDerefReg(amd64.R15, 16, amd64.R10)
		}
		clearDeadRoots := func(bb *ir.BasicBlock) {
			if rootSlotCount == 0 {
				return
			}
			keepSlots := make(map[int]struct{}, len(rootLiveOut[bb])+1)
			for id := range rootLiveOut[bb] {
				if slot, ok := rootSlots[id]; ok {
					keepSlots[slot] = struct{}{}
				}
			}
			for _, v := range amd64RootTerminatorUses(bb.Terminator) {
				if slot, ok := rootSlots[v.ID]; ok {
					keepSlots[slot] = struct{}{}
				}
			}
			zeroLoaded := false
			for slot := 0; slot < rootSlotCount; slot++ {
				if _, ok := keepSlots[slot]; ok {
					continue
				}
				if !zeroLoaded {
					e.MovRegImm64(amd64.R11, 0)
					zeroLoaded = true
				}
				e.MovDerefReg(amd64.RBP, rootFrameBaseOffset+16+int32(slot*8), amd64.R11)
			}
		}

		loadOperand := func(op ir.Operand, scratch amd64.Register) amd64.Register {
			switch v := op.(type) {
			case *ir.Value:
				return loadValue(v, scratch)
			case ir.ConstNumber:
				e.MovRegImm64(scratch, numberBits(v.Value))
				return scratch
			case ir.ConstBool:
				if v.Value {
					e.MovRegImm64(scratch, 1)
				} else {
					e.MovRegImm64(scratch, 0)
				}
				return scratch
			case ir.ConstUndefined:
				e.MovRegImm64(scratch, amd64UndefinedBits)
				return scratch
			case ir.ConstNull:
				e.MovRegImm64(scratch, amd64NullBits)
				return scratch
			default:
				at := len(e.Code)
				e.LeaRipRel32(scratch, 0)
				str := ""
				if cs, ok := op.(ir.ConstString); ok {
					str = cs.Value
				}
				strFixups = append(strFixups, stringFixupAMD64{offset: at + 3, targetReg: scratch, str: str})
				return scratch
			}
		}
		loadRawValue := loadOperand
		loadArrayIndex := func(op ir.Operand, dst amd64.Register) {
			src := loadOperand(op, amd64.R10)
			e.MovQXMMReg(amd64.XMM0, src)
			e.Cvttsd2si(dst, amd64.XMM0)
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

		if rootSlotCount != 0 {
			e.MovRegReg(amd64.R10, amd64.RBP)
			e.SubRegImm32(amd64.R10, -rootFrameBaseOffset)
			e.MovRegDeref(amd64.R11, amd64.R15, 16)
			e.MovDerefReg(amd64.R10, 0, amd64.R11)
			e.MovRegImm64(amd64.R11, int64(rootSlotCount))
			e.MovDerefReg(amd64.R10, 8, amd64.R11)
			e.MovRegImm64(amd64.R11, 0)
			for i := 0; i < rootSlotCount; i++ {
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
						lhs := loadRawValue(bi.LHS, amd64.R10)
						rhs := loadRawValue(bi.RHS, amd64.R11)
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
						lhs := loadRawValue(bi.LHS, amd64.RDI)
						if lhs != amd64.RDI {
							e.MovRegReg(amd64.RDI, lhs)
						}
						rhs := loadRawValue(bi.RHS, amd64.RSI)
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
						lhsReg := loadOperand(bi.LHS, amd64.R10)
						rhsReg := loadOperand(bi.RHS, amd64.R11)
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

					lhsReg := loadOperand(bi.LHS, amd64.R10)
					rhsReg := loadOperand(bi.RHS, amd64.R11)
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
					src := loadOperand(bi.Val, amd64.R10)
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
					obj := loadOperand(bi.Obj, amd64.R10)
					e.MovRegDeref(amd64.R11, obj, int32(bi.Offset))
					storeSSAValue(bi.Res, amd64.R11)

				case *ir.SetFieldInst:
					obj := loadOperand(bi.Obj, amd64.R10)
					if obj != amd64.R10 {
						e.MovRegReg(amd64.R10, obj)
					}
					val := loadRawValue(bi.Val, amd64.R11)
					if val != amd64.R11 {
						e.MovRegReg(amd64.R11, val)
					}
					e.MovDerefReg(amd64.R10, int32(bi.Offset), amd64.R11)

				case *ir.AllocArrayInst:
					loadArrayIndex(bi.Length, amd64.RDI)
					e.MovRegImm64(amd64.RSI, amd64ArrayElementClass(bi.ElemType))
					emitRuntimeCall("ts_array_new")
					storeSSAValue(bi.Res, amd64.RAX)

				case *ir.GetElementInst:
					arr := loadOperand(bi.Array, amd64.RDI)
					if arr != amd64.RDI {
						e.MovRegReg(amd64.RDI, arr)
					}
					loadArrayIndex(bi.Index, amd64.RSI)
					emitRuntimeCall("ts_array_get")
					storeSSAValue(bi.Res, amd64.RAX)

				case *ir.SetElementInst:
					arr := loadOperand(bi.Array, amd64.RDI)
					if arr != amd64.RDI {
						e.MovRegReg(amd64.RDI, arr)
					}
					loadArrayIndex(bi.Index, amd64.RSI)
					val := loadRawValue(bi.Val, amd64.RDX)
					if val != amd64.RDX {
						e.MovRegReg(amd64.RDX, val)
					}
					emitRuntimeCall("ts_array_set")

				case *ir.ArrayLengthInst:
					arr := loadOperand(bi.Array, amd64.RDI)
					if arr != amd64.RDI {
						e.MovRegReg(amd64.RDI, arr)
					}
					emitRuntimeCall("ts_array_len")
					e.MovQRegXMM(amd64.R10, amd64.XMM0)
					storeSSAValue(bi.Res, amd64.R10)

				case *ir.ArrayPushInst:
					arr := loadOperand(bi.Array, amd64.RDI)
					if arr != amd64.RDI {
						e.MovRegReg(amd64.RDI, arr)
					}
					val := loadRawValue(bi.Val, amd64.RSI)
					if val != amd64.RSI {
						e.MovRegReg(amd64.RSI, val)
					}
					emitRuntimeCall("ts_array_push")
					e.MovQRegXMM(amd64.R10, amd64.XMM0)
					storeSSAValue(bi.Res, amd64.R10)

				case *ir.ArrayPopInst:
					arr := loadOperand(bi.Array, amd64.RDI)
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
						v := loadRawValue(capture, amd64.R11)
						if v != amd64.R11 {
							e.MovRegReg(amd64.R11, v)
						}
						e.MovDerefReg(amd64.RAX, int32(24+i*8), amd64.R11)
					}
					storeSSAValue(bi.Res, amd64.RAX)

				case *ir.ClosureGetInst:
					closure := loadOperand(bi.Closure, amd64.R10)
					e.MovRegDeref(amd64.R11, closure, int32(24+bi.Index*8))
					storeSSAValue(bi.Res, amd64.R11)

				case *ir.IndirectCallInst:
					closure := loadOperand(bi.Closure, amd64.R10)
					if closure != amd64.RAX {
						e.MovRegReg(amd64.RAX, closure)
					}
					// Hidden closure environment occupies RDI. Method-style structural
					// calls optionally place their receiver in RSI; user integer-class
					// arguments then begin at RDX. SSE arguments still begin at XMM0.
					e.MovRegReg(amd64.RDI, amd64.RAX)
					userGPRs := []amd64.Register{amd64.RSI, amd64.RDX, amd64.RCX, amd64.R8, amd64.R9}
					if bi.ThisArg != nil {
						thisReg := loadRawValue(bi.ThisArg, amd64.R10)
						if thisReg != amd64.RSI {
							e.MovRegReg(amd64.RSI, thisReg)
						}
						userGPRs = []amd64.Register{amd64.RDX, amd64.RCX, amd64.R8, amd64.R9}
					}
					gprArg, xmmArg := 0, 0
					stackArgs := make([]ir.Operand, 0)
					emitIndirectGPRArg := func(dst amd64.Register, arg ir.Operand) {
						v := loadRawValue(arg, amd64.R10)
						if v != dst {
							e.MovRegReg(dst, v)
						}
					}
					for i, arg := range bi.Args {
						argType := arg.Type()
						if i < len(bi.ParamTypes) && bi.ParamTypes[i] != nil {
							argType = bi.ParamTypes[i]
						}
						if isNumberType(argType) {
							if xmmArg < len(amd64NumberParamRegs) {
								v := loadOperand(arg, amd64.R10)
								e.MovQXMMReg(amd64NumberParamRegs[xmmArg], v)
								xmmArg++
							} else {
								stackArgs = append(stackArgs, arg)
							}
							continue
						}
						if gprArg < len(userGPRs) {
							emitIndirectGPRArg(userGPRs[gprArg], arg)
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
						emitIndirectGPRArg(amd64.R10, stackArgs[i])
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
					emitGPRArg := func(dst amd64.Register, arg ir.Operand) {
						switch v := arg.(type) {
						case ir.ConstString:
							strOffset := len(e.Code)
							e.LeaRipRel32(dst, 0)
							strFixups = append(strFixups, stringFixupAMD64{offset: strOffset + 3, targetReg: dst, str: v.Value})
						default:
							src := loadOperand(arg, amd64.R10)
							if src != dst {
								e.MovRegReg(dst, src)
							}
						}
					}
					for i, arg := range bi.Args {
						argType := arg.Type()
						if i < len(bi.ParamTypes) && bi.ParamTypes[i] != nil {
							argType = bi.ParamTypes[i]
						}
						if isNumberType(argType) {
							if xmmArg < len(amd64NumberParamRegs) {
								src := loadOperand(arg, amd64.R10)
								e.MovQXMMReg(amd64NumberParamRegs[xmmArg], src)
								xmmArg++
							} else {
								stackArgs = append(stackArgs, arg)
							}
							continue
						}
						if gprArg < len(amd64ParamRegs) {
							emitGPRArg(amd64ParamRegs[gprArg], arg)
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
						emitGPRArg(amd64.R10, stackArgs[i])
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

				}
			}

			clearDeadRoots(bb)
			if bb.Terminator != nil {
				switch term := bb.Terminator.(type) {
				case *ir.ReturnTerm:
					if term.Val != nil {
						if isNumberType(fn.ReturnType) {
							src := loadOperand(term.Val, amd64.R10)
							e.MovQXMMReg(amd64.XMM0, src)
						} else if str, ok := term.Val.(ir.ConstString); ok {
							strOffset := len(e.Code)
							e.LeaRipRel32(amd64.RAX, 0)
							strFixups = append(strFixups, stringFixupAMD64{offset: strOffset + 3, targetReg: amd64.RAX, str: str.Value})
						} else {
							src := loadOperand(term.Val, amd64.RAX)
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

	fnOffsets["ts_print_true"] = len(e.Code)
	emitAMD64PrintLiteral(e, "true\n")
	fnOffsets["ts_print_false"] = len(e.Code)
	emitAMD64PrintLiteral(e, "false\n")
	fnOffsets["ts_print_bool"] = len(e.Code)
	emitAMD64PrintBool(e, fnOffsets["ts_print_true"], fnOffsets["ts_print_false"])
	fnOffsets["ts_print_undefined"] = len(e.Code)
	emitAMD64PrintLiteral(e, "undefined\n")
	fnOffsets["ts_print_null"] = len(e.Code)
	emitAMD64PrintLiteral(e, "null\n")

	// Emit ts_print_str for Linux AMD64
	fnOffsets["ts_print_str"] = len(e.Code)
	emitAMD64PrintStr(e)
	fnOffsets["ts_print_object"] = len(e.Code)
	emitAMD64PrintLiteral(e, "[object Object]\n")

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
	fnOffsets["ts_js_to_bool"] = len(e.Code)
	emitAMD64JSToBool(e)
	fnOffsets["ts_js_print"] = len(e.Code)
	emitAMD64JSPrint(e, fnOffsets["ts_print_val"], fnOffsets["ts_print_str"], fnOffsets["ts_print_undefined"], fnOffsets["ts_print_null"], fnOffsets["ts_print_object"], fnOffsets["ts_print_true"], fnOffsets["ts_print_false"])
	fnOffsets["ts_json_parse_scalar"] = len(e.Code)
	emitAMD64JSONParseScalar(e)
	fnOffsets["ts_js_string_to_number"] = len(e.Code)
	emitAMD64JSStringToNumber(e, fnOffsets["ts_json_parse_scalar"])

	emitAMD64RuntimeSymbols(e, fnOffsets)

	return finalizeAMD64(e, fnOffsets, strFixups, closureCodeFixups, callFixups, branchFixups, bbOffsets)
}
