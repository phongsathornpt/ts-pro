package regalloc

import (
	"sort"

	"github.com/phongsathornpt/ts-pro/internal/core/ir"
)

// PhysReg represents a physical CPU register.
type PhysReg int

const NoReg PhysReg = -1

// Location represents where a value lives (physical register or stack frame slot).
type Location struct {
	IsReg     bool
	Reg       PhysReg
	StackSlot int // Byte offset or slot index in stack frame
}

func InReg(r PhysReg) Location {
	return Location{IsReg: true, Reg: r}
}

func InStack(slot int) Location {
	return Location{IsReg: false, StackSlot: slot}
}

// Interval represents the liveness range of an SSA virtual register.
type Interval struct {
	ValID int
	Start int
	End   int
}

// Allocator allocates physical registers to virtual registers using Linear Scan.
type Allocator struct {
	numPhysRegs int
	stackSlots  int
}

func New(numPhysRegs int) *Allocator {
	return &Allocator{
		numPhysRegs: numPhysRegs,
		stackSlots:  0,
	}
}

// StackFrameSlots returns the number of spill slots used.
func (a *Allocator) StackFrameSlots() int {
	return a.stackSlots
}

// Allocate assigns registers and stack slots for a function.
func (a *Allocator) Allocate(fn *ir.Function) map[int]Location {
	intervals := a.computeIntervals(fn)
	// Sort intervals by start point
	sort.Slice(intervals, func(i, j int) bool {
		return intervals[i].Start < intervals[j].Start
	})

	assignment := make(map[int]Location, len(intervals))
	type activeItem struct {
		interval Interval
		reg      PhysReg
	}
	active := make([]activeItem, 0, a.numPhysRegs)
	insertActive := func(item activeItem) {
		active = append(active, item)
		for i := len(active) - 1; i > 0 && active[i].interval.End < active[i-1].interval.End; i-- {
			active[i], active[i-1] = active[i-1], active[i]
		}
	}
	freeRegs := make([]bool, a.numPhysRegs)
	for i := range freeRegs {
		freeRegs[i] = true
	}

	for _, curr := range intervals {
		// Expire old intervals while compacting the active set in place.
		kept := 0
		for _, act := range active {
			if act.interval.End < curr.Start || (act.interval.End == curr.Start && act.interval.Start < curr.Start) {
				freeRegs[act.reg] = true
				continue
			}
			active[kept] = act
			kept++
		}
		active = active[:kept]

		// Find free register
		allocatedReg := NoReg
		for r := 0; r < a.numPhysRegs; r++ {
			if freeRegs[r] {
				allocatedReg = PhysReg(r)
				freeRegs[r] = false
				break
			}
		}

		if allocatedReg != NoReg {
			assignment[curr.ValID] = InReg(allocatedReg)
			insertActive(activeItem{interval: curr, reg: allocatedReg})
		} else {
			// Spill: spill the one with the furthest end
			last := len(active) - 1
			if len(active) > 0 && active[last].interval.End > curr.End {
				spilled := active[last]
				assignment[spilled.interval.ValID] = InStack(a.stackSlots)
				a.stackSlots++

				assignment[curr.ValID] = InReg(spilled.reg)
				active = active[:last]
				insertActive(activeItem{interval: curr, reg: spilled.reg})
			} else {
				assignment[curr.ValID] = InStack(a.stackSlots)
				a.stackSlots++
			}
		}
	}

	return assignment
}

func operandValue(op ir.Operand) *ir.Value {
	v, _ := op.(*ir.Value)
	return v
}

func instructionValues(inst ir.Instruction) []*ir.Value {
	values := make([]*ir.Value, 0, 4)
	add := func(op ir.Operand) {
		if v := operandValue(op); v != nil {
			values = append(values, v)
		}
	}
	switch i := inst.(type) {
	case *ir.BinaryInst:
		add(i.LHS)
		add(i.RHS)
	case *ir.UnaryInst:
		add(i.Val)
	case *ir.CallInst:
		for _, arg := range i.Args {
			add(arg)
		}
	case *ir.MakeClosureInst:
		for _, capture := range i.Captures {
			add(capture)
		}
	case *ir.ClosureGetInst:
		add(i.Closure)
	case *ir.IndirectCallInst:
		add(i.Closure)
		add(i.ThisArg)
		for _, arg := range i.Args {
			add(arg)
		}
	case *ir.GetFieldInst:
		add(i.Obj)
	case *ir.SetFieldInst:
		add(i.Obj)
		add(i.Val)
	case *ir.AllocArrayInst:
		add(i.Length)
	case *ir.GetElementInst:
		add(i.Array)
		add(i.Index)
	case *ir.SetElementInst:
		add(i.Array)
		add(i.Index)
		add(i.Val)
	case *ir.ArrayLengthInst:
		add(i.Array)
	case *ir.ArrayPushInst:
		add(i.Array)
		add(i.Val)
	case *ir.ArrayPopInst:
		add(i.Array)
	}
	return values
}

func terminatorValues(term ir.Terminator) []*ir.Value {
	if term == nil {
		return nil
	}
	switch t := term.(type) {
	case *ir.ReturnTerm:
		if v := operandValue(t.Val); v != nil {
			return []*ir.Value{v}
		}
	case *ir.BranchTerm:
		if v := operandValue(t.Cond); v != nil {
			return []*ir.Value{v}
		}
	}
	return nil
}

func copySet(src map[int]struct{}) map[int]struct{} {
	out := make(map[int]struct{}, len(src))
	for id := range src {
		out[id] = struct{}{}
	}
	return out
}

func setsEqual(a, b map[int]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for id := range a {
		if _, ok := b[id]; !ok {
			return false
		}
	}
	return true
}

func (a *Allocator) computeIntervals(fn *ir.Function) []Interval {
	startMap := make(map[int]int)
	endMap := make(map[int]int)
	blockStart := make(map[*ir.BasicBlock]int, len(fn.Blocks))
	blockEnd := make(map[*ir.BasicBlock]int, len(fn.Blocks))
	defs := make(map[*ir.BasicBlock]map[int]struct{}, len(fn.Blocks))
	uses := make(map[*ir.BasicBlock]map[int]struct{}, len(fn.Blocks))
	liveIn := make(map[*ir.BasicBlock]map[int]struct{}, len(fn.Blocks))
	liveOut := make(map[*ir.BasicBlock]map[int]struct{}, len(fn.Blocks))
	edgeUses := make(map[*ir.BasicBlock]map[*ir.BasicBlock]map[int]struct{})

	step := 0
	for _, param := range fn.Params {
		startMap[param.ID] = 0
		endMap[param.ID] = 0
	}

	for blockIndex, bb := range fn.Blocks {
		defs[bb] = map[int]struct{}{}
		uses[bb] = map[int]struct{}{}
		liveIn[bb] = map[int]struct{}{}
		liveOut[bb] = map[int]struct{}{}
		blockStart[bb] = step + 1
		if blockIndex == 0 {
			for _, param := range fn.Params {
				defs[bb][param.ID] = struct{}{}
			}
		}

		for _, phi := range bb.Phis {
			step++
			if phi.Res != nil {
				if _, exists := startMap[phi.Res.ID]; !exists {
					startMap[phi.Res.ID] = step
				}
				if endMap[phi.Res.ID] < step {
					endMap[phi.Res.ID] = step
				}
				defs[bb][phi.Res.ID] = struct{}{}
			}
		}

		for _, inst := range bb.Instructions {
			step++
			for _, value := range instructionValues(inst) {
				if _, defined := defs[bb][value.ID]; !defined {
					uses[bb][value.ID] = struct{}{}
				}
				if endMap[value.ID] < step {
					endMap[value.ID] = step
				}
			}
			if res := inst.Result(); res != nil {
				if _, exists := startMap[res.ID]; !exists {
					startMap[res.ID] = step
				}
				if endMap[res.ID] < step {
					endMap[res.ID] = step
				}
				defs[bb][res.ID] = struct{}{}
			}
		}

		if bb.Terminator != nil {
			step++
			for _, value := range terminatorValues(bb.Terminator) {
				if _, defined := defs[bb][value.ID]; !defined {
					uses[bb][value.ID] = struct{}{}
				}
				if endMap[value.ID] < step {
					endMap[value.ID] = step
				}
			}
		}
		blockEnd[bb] = step
	}

	// Phi incoming values are used on predecessor edges, not at the target
	// block's phi position. Recording them here is what makes arbitrary block
	// layout safe instead of accidentally depending on creation order.
	for _, pred := range fn.Blocks {
		if pred.Terminator == nil {
			continue
		}
		for _, succ := range pred.Terminator.Successors() {
			if edgeUses[pred] == nil {
				edgeUses[pred] = map[*ir.BasicBlock]map[int]struct{}{}
			}
			set := map[int]struct{}{}
			for _, phi := range succ.Phis {
				for _, incoming := range phi.Incoming {
					if incoming.Block != pred {
						continue
					}
					if value := operandValue(incoming.Value); value != nil {
						set[value.ID] = struct{}{}
						if endMap[value.ID] < blockEnd[pred] {
							endMap[value.ID] = blockEnd[pred]
						}
					}
				}
			}
			edgeUses[pred][succ] = set
		}
	}

	for changed := true; changed; {
		changed = false
		for i := len(fn.Blocks) - 1; i >= 0; i-- {
			bb := fn.Blocks[i]
			newOut := map[int]struct{}{}
			if bb.Terminator != nil {
				for _, succ := range bb.Terminator.Successors() {
					for id := range liveIn[succ] {
						newOut[id] = struct{}{}
					}
					for id := range edgeUses[bb][succ] {
						newOut[id] = struct{}{}
					}
				}
			}
			newIn := copySet(uses[bb])
			for id := range newOut {
				if _, defined := defs[bb][id]; !defined {
					newIn[id] = struct{}{}
				}
			}
			if !setsEqual(liveOut[bb], newOut) || !setsEqual(liveIn[bb], newIn) {
				liveOut[bb], liveIn[bb] = newOut, newIn
				changed = true
			}
		}
	}

	// Linear scan still needs one contiguous interval. Conservatively span every
	// block where a value is live, even when physical block order differs from
	// CFG execution order. This may increase pressure slightly, but cannot
	// miscompile a live-through value by reusing its register early.
	for _, bb := range fn.Blocks {
		for id := range liveIn[bb] {
			if start, ok := startMap[id]; ok && blockStart[bb] < start {
				startMap[id] = blockStart[bb]
			}
			if endMap[id] < blockEnd[bb] {
				endMap[id] = blockEnd[bb]
			}
		}
		for id := range liveOut[bb] {
			if start, ok := startMap[id]; ok && blockStart[bb] < start {
				startMap[id] = blockStart[bb]
			}
			if endMap[id] < blockEnd[bb] {
				endMap[id] = blockEnd[bb]
			}
		}
	}

	intervals := make([]Interval, 0, len(startMap))
	for valID, start := range startMap {
		intervals = append(intervals, Interval{ValID: valID, Start: start, End: endMap[valID]})
	}
	return intervals
}
