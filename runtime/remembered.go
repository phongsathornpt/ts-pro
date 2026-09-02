package runtime

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

const nativeRememberedShardCount = 64

type nativeRememberedShard struct {
	sync.Mutex
	parents map[uintptr]struct{}
}

type nativeRememberedState struct {
	shards  [nativeRememberedShardCount]nativeRememberedShard
	stores  atomic.Uint64
	records atomic.Uint64
}

var nativeRemembered = newNativeRememberedState()

func newNativeRememberedState() *nativeRememberedState {
	state := &nativeRememberedState{}
	for i := range state.shards {
		state.shards[i].parents = make(map[uintptr]struct{})
	}
	return state
}

func nativeRememberedShardIndex(parent uintptr) uintptr {
	x := uint64(parent >> 4)
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return uintptr(x & (nativeRememberedShardCount - 1))
}

func (state *nativeRememberedState) remember(parent uintptr) {
	shard := &state.shards[nativeRememberedShardIndex(parent)]
	shard.Lock()
	if _, exists := shard.parents[parent]; !exists {
		shard.parents[parent] = struct{}{}
		state.records.Add(1)
	}
	shard.Unlock()
}
func (state *nativeRememberedState) rangeParents(visit func(uintptr)) {
	if visit == nil {
		return
	}
	for i := range state.shards {
		shard := &state.shards[i]
		shard.Lock()
		parents := make([]uintptr, 0, len(shard.parents))
		for parent := range shard.parents {
			parents = append(parents, parent)
		}
		shard.Unlock()
		for _, parent := range parents {
			visit(parent)
		}
	}
}

func (state *nativeRememberedState) clear() {
	for i := range state.shards {
		shard := &state.shards[i]
		shard.Lock()
		clear(shard.parents)
		shard.Unlock()
	}
}

func (state *nativeRememberedState) count() int {
	count := 0
	state.rangeParents(func(uintptr) { count++ })
	return count
}

func nativeGCRememberPair(parentBlock *nativeHeapBlock, child unsafe.Pointer) {
	if parentBlock == nil || child == nil || parentBlock.generation != nativeHeapGenerationOld {
		return
	}
	childBlock := nativeBlocks.get(uintptr(child))
	if childBlock == nil {
		nativeHeap.Lock()
		childBlock = resolveNativeHeapBlockLocked(uintptr(child))
		nativeHeap.Unlock()
	}
	if childBlock != nil && childBlock.generation == nativeHeapGenerationNursery {
		nativeRemembered.remember(parentBlock.ptr)
	}
}

func nativeGCStoreRefSlot(slot, child unsafe.Pointer) {
	if slot == nil {
		nativeAbort("native GC reference store slot is nil")
		return
	}
	nativeHeapWorld.RLock()
	nativeRemembered.stores.Add(1)
	parentBlock := nativeBlocks.get(uintptr(slot))
	if parentBlock == nil {
		nativeHeap.Lock()
		parentBlock = resolveNativeHeapBlockLocked(uintptr(slot))
		nativeHeap.Unlock()
	}
	nativeGCRememberPair(parentBlock, child)
	*(*unsafe.Pointer)(slot) = child
	nativeHeapWorld.RUnlock()
}

func nativeGCStoreRef(parent, slot, child unsafe.Pointer) {
	if slot == nil {
		nativeAbort("native GC reference store slot is nil")
		return
	}
	nativeHeapWorld.RLock()
	nativeRemembered.stores.Add(1)
	if parent != nil {
		nativeGCRememberPair(nativeBlocks.get(uintptr(parent)), child)
	}
	*(*unsafe.Pointer)(slot) = child
	nativeHeapWorld.RUnlock()
}

func gcStoreRef(parent, slot, child unsafe.Pointer) {
	nativeGCStoreRef(parent, slot, child)
}

func gcStoreRefSlot(slot, child unsafe.Pointer) {
	nativeGCStoreRefSlot(slot, child)
}

func gcRememberedParents() uintptr {
	return uintptr(nativeRemembered.count())
}
func resetNativeRememberedState() {
	nativeRemembered.clear()
	nativeRemembered.stores.Store(0)
	nativeRemembered.records.Store(0)
}

func gcBarrierStores() uint64 {
	return nativeRemembered.stores.Load()
}

func gcRememberedRecords() uint64 {
	return nativeRemembered.records.Load()
}
