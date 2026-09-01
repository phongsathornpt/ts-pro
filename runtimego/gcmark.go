package main

import "unsafe"

type nativeGCMarkState struct {
	collectorOwner int
	currentOwner   int
	queues         map[int][]*nativeHeapBlock
	pendingOwners  []int
}

func newNativeGCMarkState(owner int) *nativeGCMarkState {
	return &nativeGCMarkState{
		collectorOwner: owner,
		currentOwner:   owner,
		queues:         make(map[int][]*nativeHeapBlock),
	}
}

func nativeHeapBlockOwner(block *nativeHeapBlock) int {
	if block != nil && block.span != nil {
		return block.span.owner
	}
	return -1
}

func resolveNativeHeapBlockLocked(candidate uintptr) *nativeHeapBlock {
	if candidate == 0 {
		return nil
	}
	if block := nativeHeap.blocks[candidate]; block != nil {
		return block
	}
	span := nativeHeapSpanForPointerLocked(candidate)
	if span == nil {
		return nil
	}
	base := uintptr(span.base)
	end := base + uintptr(len(span.data))
	if candidate < base || candidate >= end {
		return nil
	}
	offset := candidate - base
	slotBase := base + (offset/span.classSize)*span.classSize
	return nativeHeap.blocks[slotBase]
}

func (state *nativeGCMarkState) enqueue(candidate uintptr) {
	block := resolveNativeHeapBlockLocked(candidate)
	if block == nil || block.marked {
		return
	}
	block.marked = true
	owner := nativeHeapBlockOwner(block)
	queue := state.queues[owner]
	wasEmpty := len(queue) == 0
	state.queues[owner] = append(queue, block)
	if wasEmpty && owner != state.currentOwner {
		state.pendingOwners = append(state.pendingOwners, owner)
	}
}

func (state *nativeGCMarkState) pop() *nativeHeapBlock {
	for {
		queue := state.queues[state.currentOwner]
		if count := len(queue); count != 0 {
			block := queue[count-1]
			queue[count-1] = nil
			state.queues[state.currentOwner] = queue[:count-1]
			return block
		}
		if len(state.pendingOwners) == 0 {
			return nil
		}
		last := len(state.pendingOwners) - 1
		owner := state.pendingOwners[last]
		state.pendingOwners = state.pendingOwners[:last]
		if len(state.queues[owner]) == 0 {
			continue
		}
		state.currentOwner = owner
		if owner != state.collectorOwner {
			nativeHeap.markQueueSwitches++
		}
	}
}

func markNativeHeapRootsLocked() {
	state := newNativeGCMarkState(nativeAllocatorOwner())
	wordSize := uintptr(unsafe.Sizeof(uintptr(0)))
	for _, frame := range nativeHeap.roots {
		for i := uintptr(0); i < frame.count; i++ {
			slotAddr := unsafe.Add(frame.slots, i*wordSize)
			state.enqueue(*(*uintptr)(slotAddr))
		}
	}
	for {
		block := state.pop()
		if block == nil {
			return
		}
		nativeHeap.markWork++
		count := block.size / wordSize
		for i := uintptr(0); i < count; i++ {
			word := *(*uintptr)(unsafe.Add(block.raw, i*wordSize))
			state.enqueue(word)
		}
	}
}

func nativeGCMarkWork() (uint64, uint64) {
	nativeHeap.Lock()
	work := nativeHeap.markWork
	switches := nativeHeap.markQueueSwitches
	nativeHeap.Unlock()
	return work, switches
}
