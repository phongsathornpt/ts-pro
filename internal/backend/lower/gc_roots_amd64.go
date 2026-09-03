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
)
