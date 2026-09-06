package lower

import (
	"sort"

	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func isAMD64HeapRefType(t types.Type) bool {
	if t == nil {
		return false
	}
	if amd64JSValueType(t) {
		return true
	}
	switch t.Kind() {
	case types.KindString, types.KindArray, types.KindTuple, types.KindObject, types.KindFunction:
		return true
	case types.KindUnion:
		if u, ok := t.(*types.UnionType); ok {
			for _, member := range u.Members {
				if isAMD64HeapRefType(member) {
					return true
				}
			}
		}
	}
	return false
}

func amd64ArrayElementClass(t types.Type) int64 {
	if amd64JSValueType(t) {
		return 2
	}
	if isAMD64HeapRefType(t) {
		return 1
	}
	return 0
}

func amd64RootSlots(fn *ir.Function) map[int]int {
	ids := make(map[int]struct{})
	add := func(v *ir.Value) {
		if v != nil && isAMD64HeapRefType(v.Type()) {
			ids[v.ID] = struct{}{}
		}
	}
	for _, param := range fn.Params {
		add(param)
	}
	for _, bb := range fn.Blocks {
		for _, phi := range bb.Phis {
			add(phi.Res)
		}
		for _, inst := range bb.Instructions {
			add(inst.Result())
		}
	}
	ordered := make([]int, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Ints(ordered)
	slots := make(map[int]int, len(ordered))
	for i, id := range ordered {
		slots[id] = i
	}
	return slots
}

func amd64RootOperandValue(op ir.Operand) *ir.Value {
	v, _ := op.(*ir.Value)
	if v != nil && isAMD64HeapRefType(v.Type()) {
		return v
	}
	return nil
}

func amd64RootInstructionUses(inst ir.Instruction) []*ir.Value {
	uses := make([]*ir.Value, 0, 4)
	add := func(op ir.Operand) {
		if v := amd64RootOperandValue(op); v != nil {
			uses = append(uses, v)
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
	return uses
}

func amd64RootTerminatorUses(term ir.Terminator) []*ir.Value {
	if term == nil {
		return nil
	}
	switch t := term.(type) {
	case *ir.ReturnTerm:
		if v := amd64RootOperandValue(t.Val); v != nil {
			return []*ir.Value{v}
		}
	case *ir.BranchTerm:
		if v := amd64RootOperandValue(t.Cond); v != nil {
			return []*ir.Value{v}
		}
	}
	return nil
}

func amd64RootLiveOut(fn *ir.Function) map[*ir.BasicBlock]map[int]struct{} {
	defs := make(map[*ir.BasicBlock]map[int]struct{}, len(fn.Blocks))
	uses := make(map[*ir.BasicBlock]map[int]struct{}, len(fn.Blocks))
	liveIn := make(map[*ir.BasicBlock]map[int]struct{}, len(fn.Blocks))
	liveOut := make(map[*ir.BasicBlock]map[int]struct{}, len(fn.Blocks))
	edgeUses := make(map[*ir.BasicBlock]map[*ir.BasicBlock]map[int]struct{})
	for _, bb := range fn.Blocks {
		defs[bb], uses[bb], liveIn[bb], liveOut[bb] = map[int]struct{}{}, map[int]struct{}{}, map[int]struct{}{}, map[int]struct{}{}
		if len(fn.Blocks) != 0 && bb == fn.Blocks[0] {
			for _, param := range fn.Params {
				if isAMD64HeapRefType(param.Type()) {
					defs[bb][param.ID] = struct{}{}
				}
			}
		}
		for _, phi := range bb.Phis {
			if phi.Res != nil && isAMD64HeapRefType(phi.Res.Type()) {
				defs[bb][phi.Res.ID] = struct{}{}
			}
		}
		for _, inst := range bb.Instructions {
			for _, v := range amd64RootInstructionUses(inst) {
				if _, defined := defs[bb][v.ID]; !defined {
					uses[bb][v.ID] = struct{}{}
				}
			}
			if res := inst.Result(); res != nil && isAMD64HeapRefType(res.Type()) {
				defs[bb][res.ID] = struct{}{}
			}
		}
		for _, v := range amd64RootTerminatorUses(bb.Terminator) {
			if _, defined := defs[bb][v.ID]; !defined {
				uses[bb][v.ID] = struct{}{}
			}
		}
	}
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
				for _, inc := range phi.Incoming {
					if inc.Block == pred {
						if v := amd64RootOperandValue(inc.Value); v != nil {
							set[v.ID] = struct{}{}
						}
					}
				}
			}
			edgeUses[pred][succ] = set
		}
	}
	equal := func(a, b map[int]struct{}) bool {
		if len(a) != len(b) {
			return false
		}
		for k := range a {
			if _, ok := b[k]; !ok {
				return false
			}
		}
		return true
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
			newIn := map[int]struct{}{}
			for id := range uses[bb] {
				newIn[id] = struct{}{}
			}
			for id := range newOut {
				if _, defined := defs[bb][id]; !defined {
					newIn[id] = struct{}{}
				}
			}
			if !equal(newOut, liveOut[bb]) || !equal(newIn, liveIn[bb]) {
				liveOut[bb], liveIn[bb], changed = newOut, newIn, true
			}
		}
	}
	return liveOut
}

const (
	amd64RTCursor      int32 = 0
	amd64RTEnd         int32 = 8
	amd64RTRootHead    int32 = 16
	amd64RTChunkHead   int32 = 24
	amd64RTFreeList    int32 = 32
	amd64RTCollections int32 = 40
	amd64RTReclaimed   int32 = 48
	amd64RTMappedBytes int32 = 56
	amd64RTTaskHead    int32 = 64
	amd64RTTaskTail    int32 = 72
	amd64RTCurrentTask int32 = 80
	amd64RTSchedRsp    int32 = 88
	amd64RTSchedRbp    int32 = 96
	amd64RTSchedRbx    int32 = 104
	amd64RTSchedR12    int32 = 112
	amd64RTSchedR13    int32 = 120
	amd64RTSchedR14    int32 = 128
	amd64RTSchedRoot   int32 = 136
	amd64RTTimerHead   int32 = 144
	amd64RTMarkChunk   int32 = 152

	amd64ChunkNext int32 = 0
	amd64ChunkEnd  int32 = 8
	amd64ChunkUsed int32 = 16
	amd64ChunkSize int32 = 32

	amd64ObjectSize       int32 = 0
	amd64ObjectFlags      int32 = 8
	amd64ObjectNextFree   int32 = 16
	amd64ObjectType       int32 = 24
	amd64ObjectHeaderSize int32 = 32

	amd64ObjectTypeAtomic         int64 = 0
	amd64ObjectTypeArray          int64 = 1
	amd64ObjectTypeArrayData      int64 = 2
	amd64ObjectTypeRefData        int64 = 3
	amd64ObjectTypeObject         int64 = 4
	amd64ObjectTypeClosure        int64 = 5
	amd64ObjectTypeJSValueData    int64 = 6
	amd64ObjectTypeDynamicObject  int64 = 7
	amd64ObjectTypeDynamicEntries int64 = 8
	amd64ObjectTypeTask           int64 = 9
	amd64ObjectTypeChannel        int64 = 10
	amd64ObjectTypeTaskGroup      int64 = 11
	amd64ObjectTypeCollection     int64 = 12
)
