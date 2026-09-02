package runtimego

import (
	"os"
	"sync"
	"sync/atomic"
	"unsafe"
)

const nativeRootShardCount = 16

const nativeRootTokenSize = uintptr(unsafe.Sizeof(uintptr(0)))

type nativeRootFrame struct {
	token      unsafe.Pointer
	slots      unsafe.Pointer
	count      uintptr
	persistent bool
	tid        int
}

type nativeRootShard struct {
	nativeMeasuredMutex
	roots        map[uintptr]*nativeRootFrame
	threadStacks map[int][]uintptr
	tokenFree    []unsafe.Pointer
}
type nativeRootState struct {
	shards [nativeRootShardCount]nativeRootShard
	pageMu sync.Mutex
	pages  [][]byte

	handoffs atomic.Uint64
}

var nativeRoots = newNativeRootState()

func newNativeRootState() *nativeRootState {
	state := &nativeRootState{}
	for i := range state.shards {
		state.shards[i].roots = make(map[uintptr]*nativeRootFrame)
		state.shards[i].threadStacks = make(map[int][]uintptr)
	}
	return state
}

func nativeRootShardIndexForThread(tid int) int {
	return int(uintptr(tid) & (nativeRootShardCount - 1))
}

func nativeRootShardIndexForToken(raw unsafe.Pointer) int {
	page := uintptr(raw) / uintptr(os.Getpagesize())
	return int(page & (nativeRootShardCount - 1))
}

func (state *nativeRootState) shardForToken(raw unsafe.Pointer) *nativeRootShard {
	return &state.shards[nativeRootShardIndexForToken(raw)]
}
func (state *nativeRootState) refillTokens(target int) {
	state.pageMu.Lock()
	defer state.pageMu.Unlock()

	targetShard := &state.shards[target]
	targetShard.Lock()
	hasToken := len(targetShard.tokenFree) != 0
	targetShard.Unlock()
	if hasToken {
		return
	}

	pageSize := uintptr(os.Getpagesize())
	_, region := nativeMap(pageSize * nativeRootShardCount)
	for pageOffset := uintptr(0); pageOffset < uintptr(len(region)); pageOffset += pageSize {
		base := unsafe.Pointer(&region[pageOffset])
		index := nativeRootShardIndexForToken(base)
		shard := &state.shards[index]
		shard.Lock()
		for offset := uintptr(0); offset+nativeRootTokenSize <= pageSize; offset += nativeRootTokenSize {
			shard.tokenFree = append(shard.tokenFree, unsafe.Add(base, offset))
		}
		shard.Unlock()
	}
	state.pages = append(state.pages, region)
}

func (state *nativeRootState) lockShardWithToken(index int) (*nativeRootShard, unsafe.Pointer) {
	for {
		shard := &state.shards[index]
		shard.Lock()
		if count := len(shard.tokenFree); count != 0 {
			token := shard.tokenFree[count-1]
			shard.tokenFree[count-1] = nil
			shard.tokenFree = shard.tokenFree[:count-1]
			return shard, token
		}
		shard.Unlock()
		state.refillTokens(index)
	}
}
func (state *nativeRootState) freeTokenLocked(shard *nativeRootShard, token unsafe.Pointer) {
	if token != nil {
		shard.tokenFree = append(shard.tokenFree, token)
	}
}

func (state *nativeRootState) canCollect(tid int) bool {
	if state.handoffs.Load() != 0 {
		return false
	}
	activeThreads := 0
	ownsActiveStack := false
	for i := range state.shards {
		shard := &state.shards[i]
		shard.Lock()
		for threadID, stack := range shard.threadStacks {
			if len(stack) == 0 {
				continue
			}
			activeThreads++
			if threadID == tid {
				ownsActiveStack = true
			}
		}
		shard.Unlock()
		if activeThreads > 1 {
			return false
		}
	}
	return activeThreads == 0 || ownsActiveStack
}

func (state *nativeRootState) rangeFrames(visit func(*nativeRootFrame)) {
	for i := range state.shards {
		shard := &state.shards[i]
		shard.Lock()
		for _, frame := range shard.roots {
			visit(frame)
		}
		shard.Unlock()
	}
}
func (state *nativeRootState) metrics() nativeLockMetrics {
	var total nativeLockMetrics
	for i := range state.shards {
		metrics := state.shards[i].snapshot()
		total.acquisitions += metrics.acquisitions
		total.contended += metrics.contended
		total.waitNanos += metrics.waitNanos
	}
	return total
}

func (state *nativeRootState) resetMetrics() {
	for i := range state.shards {
		state.shards[i].nativeMeasuredMutex.resetMetrics()
	}
}

func (state *nativeRootState) rootCount() int {
	count := 0
	for i := range state.shards {
		shard := &state.shards[i]
		shard.Lock()
		count += len(shard.roots)
		shard.Unlock()
	}
	return count
}

func (state *nativeRootState) shutdown() [][]byte {
	state.pageMu.Lock()
	defer state.pageMu.Unlock()
	for i := range state.shards {
		shard := &state.shards[i]
		shard.Lock()
		shard.roots = make(map[uintptr]*nativeRootFrame)
		shard.threadStacks = make(map[int][]uintptr)
		shard.tokenFree = nil
		shard.Unlock()
	}
	state.handoffs.Store(0)
	pages := state.pages
	state.pages = nil
	return pages
}

func tsnative_gc_enter(slots unsafe.Pointer, count uintptr) unsafe.Pointer {
	nativeHeapWorld.RLock()
	tid := nativeCurrentThreadID()
	index := nativeRootShardIndexForThread(tid)
	shard, token := nativeRoots.lockShardWithToken(index)
	key := uintptr(token)
	shard.roots[key] = &nativeRootFrame{token: token, slots: slots, count: count, tid: tid}
	shard.threadStacks[tid] = append(shard.threadStacks[tid], key)
	shard.Unlock()
	nativeHeapWorld.RUnlock()
	return token
}

func tsnative_gc_leave(raw unsafe.Pointer) {
	nativeHeapWorld.RLock()
	token := uintptr(raw)
	tid := nativeCurrentThreadID()
	shard := nativeRoots.shardForToken(raw)
	shard.Lock()
	frame := shard.roots[token]
	stack := shard.threadStacks[tid]
	if frame == nil || frame.persistent || frame.tid != tid || len(stack) == 0 || stack[len(stack)-1] != token {
		shard.Unlock()
		nativeHeapWorld.RUnlock()
		nativeAbort("invalid GC root frame discipline")
		return
	}
	delete(shard.roots, token)
	nativeRoots.freeTokenLocked(shard, frame.token)
	stack = stack[:len(stack)-1]
	if len(stack) == 0 {
		delete(shard.threadStacks, tid)
	} else {
		shard.threadStacks[tid] = stack
	}
	shard.Unlock()
	nativeHeapWorld.RUnlock()
}

func tsnative_gc_root_register(slot unsafe.Pointer) unsafe.Pointer {
	if slot == nil {
		return nil
	}
	nativeHeapWorld.RLock()
	index := nativeRootShardIndexForThread(nativeCurrentThreadID())
	shard, token := nativeRoots.lockShardWithToken(index)
	shard.roots[uintptr(token)] = &nativeRootFrame{token: token, slots: slot, count: 1, persistent: true}
	shard.Unlock()
	nativeHeapWorld.RUnlock()
	return token
}

func tsnative_gc_root_unregister(raw unsafe.Pointer) {
	if raw == nil {
		return
	}
	nativeHeapWorld.RLock()
	key := uintptr(raw)
	shard := nativeRoots.shardForToken(raw)
	shard.Lock()
	frame := shard.roots[key]
	if frame == nil || !frame.persistent {
		shard.Unlock()
		nativeHeapWorld.RUnlock()
		nativeAbort("invalid persistent GC root")
		return
	}
	delete(shard.roots, key)
	nativeRoots.freeTokenLocked(shard, frame.token)
	shard.Unlock()
	nativeHeapWorld.RUnlock()
}

func tsnative_gc_handoff_begin() {
	nativeHeapWorld.RLock()
	nativeRoots.handoffs.Add(1)
	nativeHeapWorld.RUnlock()
}

func tsnative_gc_handoff_end() {
	nativeHeapWorld.RLock()
	for {
		count := nativeRoots.handoffs.Load()
		if count == 0 {
			nativeHeapWorld.RUnlock()
			nativeAbort("invalid GC handoff discipline")
			return
		}
		if nativeRoots.handoffs.CompareAndSwap(count, count-1) {
			break
		}
	}
	nativeHeapWorld.RUnlock()
}
