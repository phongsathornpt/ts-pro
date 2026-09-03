package opt

import (
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"math"
)

// Options controls optimization passes.
type Options struct {
	Level int // 0: none, 1: basic, 2: standard, 3: aggressive
}

// Optimize runs optimization passes on the IR program according to the given options.
func Optimize(prog *ir.Program, opts Options) {
	if opts.Level == 0 {
		return
	}

	for _, fn := range prog.Functions {
		optimizeFunction(fn, opts)
	}
}

func optimizeFunction(fn *ir.Function, opts Options) {
	changed := true
	for i := 0; i < 5 && changed; i++ {
		changed = false
		if constantFold(fn) {
			changed = true
		}
		if deadCodeElim(fn) {
			changed = true
		}
		if simplifyCFG(fn) {
			changed = true
		}
	}
}

func constantFold(fn *ir.Function) bool {
	changed := false
	constMap := make(map[int]ir.Operand)

	for _, bb := range fn.Blocks {
		var newInsts []ir.Instruction
		for _, inst := range bb.Instructions {
			switch bi := inst.(type) {
			case *ir.BinaryInst:
				lhs := resolveConst(bi.LHS, constMap)
				rhs := resolveConst(bi.RHS, constMap)

				c1, ok1 := lhs.(ir.ConstNumber)
				c2, ok2 := rhs.(ir.ConstNumber)
				if ok1 && ok2 {
					var res float64
					valid := true
					switch bi.Op {
					case ir.OpAdd:
						res = c1.Value + c2.Value
					case ir.OpSub:
						res = c1.Value - c2.Value
					case ir.OpMul:
						res = c1.Value * c2.Value
					case ir.OpDiv:
						res = c1.Value / c2.Value
					case ir.OpMod:
						res = math.Mod(c1.Value, c2.Value)
					case ir.OpAnd:
						res = float64(int64(c1.Value) & int64(c2.Value))
					case ir.OpOr:
						res = float64(int64(c1.Value) | int64(c2.Value))
					default:
						valid = false
					}
					if valid && bi.Res != nil {
						constMap[bi.Res.ID] = ir.ConstNumber{Value: res}
						changed = true
						continue // Folded! Do not keep the instruction
					}
				}
				bi.LHS = lhs
				bi.RHS = rhs
				newInsts = append(newInsts, bi)
			case *ir.UnaryInst:
				val := resolveConst(bi.Val, constMap)
				if c, ok := val.(ir.ConstNumber); ok && bi.Op == "-" && bi.Res != nil {
					constMap[bi.Res.ID] = ir.ConstNumber{Value: -c.Value}
					changed = true
					continue
				}
				bi.Val = val
				newInsts = append(newInsts, bi)
			case *ir.CallInst:
				for i, arg := range bi.Args {
					bi.Args[i] = resolveConst(arg, constMap)
				}
				newInsts = append(newInsts, bi)
			default:
				newInsts = append(newInsts, inst)
			}
		}
		bb.Instructions = newInsts

		for _, phi := range bb.Phis {
			for i, inc := range phi.Incoming {
				phi.Incoming[i].Value = resolveConst(inc.Value, constMap)
			}
		}

		// Fold branch if condition is constant
		if br, ok := bb.Terminator.(*ir.BranchTerm); ok {
			c := resolveConst(br.Cond, constMap)
			if cb, ok := c.(ir.ConstBool); ok {
				if cb.Value {
					bb.Terminator = &ir.JumpTerm{Target: br.Then}
				} else {
					bb.Terminator = &ir.JumpTerm{Target: br.Else}
				}
				changed = true
			}
		} else if ret, ok := bb.Terminator.(*ir.ReturnTerm); ok && ret.Val != nil {
			ret.Val = resolveConst(ret.Val, constMap)
		}
	}

	return changed
}

func resolveConst(op ir.Operand, constMap map[int]ir.Operand) ir.Operand {
	if v, ok := op.(*ir.Value); ok {
		if c, exists := constMap[v.ID]; exists {
			return c
		}
	}
	return op
}

func deadCodeElim(fn *ir.Function) bool {
	// Count uses of each value
	uses := make(map[int]int)

	for _, bb := range fn.Blocks {
		for _, phi := range bb.Phis {
			for _, inc := range phi.Incoming {
				if v, ok := inc.Value.(*ir.Value); ok {
					uses[v.ID]++
				}
			}
		}
		for _, inst := range bb.Instructions {
			switch i := inst.(type) {
			case *ir.BinaryInst:
				if v, ok := i.LHS.(*ir.Value); ok {
					uses[v.ID]++
				}
				if v, ok := i.RHS.(*ir.Value); ok {
					uses[v.ID]++
				}
			case *ir.UnaryInst:
				if v, ok := i.Val.(*ir.Value); ok {
					uses[v.ID]++
				}
			case *ir.CallInst:
				for _, arg := range i.Args {
					if v, ok := arg.(*ir.Value); ok {
						uses[v.ID]++
					}
				}
			case *ir.AllocArrayInst:
				if v, ok := i.Length.(*ir.Value); ok {
					uses[v.ID]++
				}
			case *ir.GetElementInst:
				if v, ok := i.Array.(*ir.Value); ok {
					uses[v.ID]++
				}
				if v, ok := i.Index.(*ir.Value); ok {
					uses[v.ID]++
				}
			case *ir.SetElementInst:
				if v, ok := i.Array.(*ir.Value); ok {
					uses[v.ID]++
				}
				if v, ok := i.Index.(*ir.Value); ok {
					uses[v.ID]++
				}
				if v, ok := i.Val.(*ir.Value); ok {
					uses[v.ID]++
				}
			case *ir.ArrayLengthInst:
				if v, ok := i.Array.(*ir.Value); ok {
					uses[v.ID]++
				}
			case *ir.ArrayPushInst:
				if v, ok := i.Array.(*ir.Value); ok {
					uses[v.ID]++
				}
				if v, ok := i.Val.(*ir.Value); ok {
					uses[v.ID]++
				}
			case *ir.ArrayPopInst:
				if v, ok := i.Array.(*ir.Value); ok {
					uses[v.ID]++
				}
			}
		}
		if bb.Terminator != nil {
			switch t := bb.Terminator.(type) {
			case *ir.ReturnTerm:
				if v, ok := t.Val.(*ir.Value); ok {
					uses[v.ID]++
				}
			case *ir.BranchTerm:
				if v, ok := t.Cond.(*ir.Value); ok {
					uses[v.ID]++
				}
			}
		}
	}

	changed := false
	for _, bb := range fn.Blocks {
		var retained []ir.Instruction
		for _, inst := range bb.Instructions {
			res := inst.Result()
			// Keep calls or instructions with side effects
			switch inst.(type) {
			case *ir.CallInst, *ir.SetElementInst, *ir.ArrayPushInst, *ir.ArrayPopInst:
				retained = append(retained, inst)
				continue
			}
			if res != nil && uses[res.ID] == 0 {
				changed = true
				continue // Dead code eliminated
			}
			retained = append(retained, inst)
		}
		bb.Instructions = retained
	}

	return changed
}

func simplifyCFG(fn *ir.Function) bool {
	if len(fn.Blocks) <= 1 {
		return false
	}
	// Reachability analysis
	reachable := make(map[*ir.BasicBlock]bool)
	var worklist []*ir.BasicBlock
	if len(fn.Blocks) > 0 {
		worklist = append(worklist, fn.Blocks[0])
		reachable[fn.Blocks[0]] = true
	}

	for len(worklist) > 0 {
		curr := worklist[0]
		worklist = worklist[1:]
		if curr.Terminator != nil {
			for _, succ := range curr.Terminator.Successors() {
				if succ != nil && !reachable[succ] {
					reachable[succ] = true
					worklist = append(worklist, succ)
				}
			}
		}
	}

	if len(reachable) == len(fn.Blocks) {
		return false
	}

	var liveBlocks []*ir.BasicBlock
	for _, bb := range fn.Blocks {
		if reachable[bb] {
			liveBlocks = append(liveBlocks, bb)
		}
	}
	fn.Blocks = liveBlocks
	return true
}
