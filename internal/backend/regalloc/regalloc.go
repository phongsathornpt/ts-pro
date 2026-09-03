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

	assignment := make(map[int]Location)
	type activeItem struct {
		interval Interval
		reg      PhysReg
	}
	var active []activeItem
	freeRegs := make([]bool, a.numPhysRegs)
	for i := range freeRegs {
		freeRegs[i] = true
	}

	for _, curr := range intervals {
		// Expire old intervals
		var stillActive []activeItem
		for _, act := range active {
			if act.interval.End <= curr.Start {
				freeRegs[act.reg] = true
			} else {
				stillActive = append(stillActive, act)
			}
		}
		active = stillActive

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
			active = append(active, activeItem{interval: curr, reg: allocatedReg})
			// Sort active by end position
			sort.Slice(active, func(i, j int) bool {
				return active[i].interval.End < active[j].interval.End
			})
		} else {
			// Spill: spill the one with the furthest end
			last := len(active) - 1
			if len(active) > 0 && active[last].interval.End > curr.End {
				spilled := active[last]
				assignment[spilled.interval.ValID] = InStack(a.stackSlots)
				a.stackSlots++

				assignment[curr.ValID] = InReg(spilled.reg)
				active[last] = activeItem{interval: curr, reg: spilled.reg}
				sort.Slice(active, func(i, j int) bool {
					return active[i].interval.End < active[j].interval.End
				})
			} else {
				assignment[curr.ValID] = InStack(a.stackSlots)
				a.stackSlots++
			}
		}
	}

	return assignment
}

func (a *Allocator) computeIntervals(fn *ir.Function) []Interval {
	startMap := make(map[int]int)
	endMap := make(map[int]int)
	bbStart := make(map[string]int)

	step := 0
	for _, p := range fn.Params {
		startMap[p.ID] = step
		endMap[p.ID] = step
	}

	for _, bb := range fn.Blocks {
		bbStart[bb.Name] = step + 1
		for _, phi := range bb.Phis {
			step++
			if phi.Res != nil {
				startMap[phi.Res.ID] = step
				endMap[phi.Res.ID] = step
			}
			for _, inc := range phi.Incoming {
				if v, ok := inc.Value.(*ir.Value); ok {
					endMap[v.ID] = step
				}
			}
		}

		for _, inst := range bb.Instructions {
			step++
			res := inst.Result()
			if res != nil {
				if _, exists := startMap[res.ID]; !exists {
					startMap[res.ID] = step
				}
				endMap[res.ID] = step
			}
			switch i := inst.(type) {
			case *ir.BinaryInst:
				if v, ok := i.LHS.(*ir.Value); ok {
					endMap[v.ID] = step
				}
				if v, ok := i.RHS.(*ir.Value); ok {
					endMap[v.ID] = step
				}
			case *ir.UnaryInst:
				if v, ok := i.Val.(*ir.Value); ok {
					endMap[v.ID] = step
				}
			case *ir.GetFieldInst:
				if v, ok := i.Obj.(*ir.Value); ok {
					endMap[v.ID] = step
				}
			case *ir.SetFieldInst:
				if v, ok := i.Obj.(*ir.Value); ok {
					endMap[v.ID] = step
				}
				if v, ok := i.Val.(*ir.Value); ok {
					endMap[v.ID] = step
				}
			case *ir.CallInst:
				for _, arg := range i.Args {
					if v, ok := arg.(*ir.Value); ok {
						endMap[v.ID] = step
					}
				}
			case *ir.AllocArrayInst:
				if v, ok := i.Length.(*ir.Value); ok {
					endMap[v.ID] = step
				}
			case *ir.GetElementInst:
				if v, ok := i.Array.(*ir.Value); ok {
					endMap[v.ID] = step
				}
				if v, ok := i.Index.(*ir.Value); ok {
					endMap[v.ID] = step
				}
			case *ir.SetElementInst:
				if v, ok := i.Array.(*ir.Value); ok {
					endMap[v.ID] = step
				}
				if v, ok := i.Index.(*ir.Value); ok {
					endMap[v.ID] = step
				}
				if v, ok := i.Val.(*ir.Value); ok {
					endMap[v.ID] = step
				}
			case *ir.ArrayLengthInst:
				if v, ok := i.Array.(*ir.Value); ok {
					endMap[v.ID] = step
				}
			case *ir.ArrayPushInst:
				if v, ok := i.Array.(*ir.Value); ok {
					endMap[v.ID] = step
				}
				if v, ok := i.Val.(*ir.Value); ok {
					endMap[v.ID] = step
				}
			case *ir.ArrayPopInst:
				if v, ok := i.Array.(*ir.Value); ok {
					endMap[v.ID] = step
				}
			}
		}

		if bb.Terminator != nil {
			step++
			switch t := bb.Terminator.(type) {
			case *ir.ReturnTerm:
				if v, ok := t.Val.(*ir.Value); ok {
					endMap[v.ID] = step
				}
			case *ir.BranchTerm:
				if v, ok := t.Cond.(*ir.Value); ok {
					endMap[v.ID] = step
				}
				for _, target := range []*ir.BasicBlock{t.Then, t.Else} {
					for _, phi := range target.Phis {
						for _, inc := range phi.Incoming {
							if inc.Block == bb {
								if v, ok := inc.Value.(*ir.Value); ok {
									endMap[v.ID] = step
								}
							}
						}
					}
				}
			case *ir.JumpTerm:
				// If target was visited before bb, this is a loop backedge!
				if tStart, ok := bbStart[t.Target.Name]; ok && tStart < bbStart[bb.Name] {
					for valID, start := range startMap {
						if start <= tStart && endMap[valID] >= tStart {
							if endMap[valID] < step {
								endMap[valID] = step
							}
						}
					}
				}

				for _, phi := range t.Target.Phis {
					for _, inc := range phi.Incoming {
						if inc.Block == bb {
							if v, ok := inc.Value.(*ir.Value); ok {
								endMap[v.ID] = step
							}
						}
					}
				}
			}
		}
	}

	var intervals []Interval
	for valID, start := range startMap {
		end := endMap[valID]
		intervals = append(intervals, Interval{
			ValID: valID,
			Start: start,
			End:   end,
		})
	}
	return intervals
}
