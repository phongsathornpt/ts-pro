package main

import "unsafe"

type nativeGCMarkPage struct {
	owner  int
	blocks []*nativeHeapBlock
	queued bool
}

type nativeGCMarkState struct {
	collectorOwner int
	currentOwner   int
	currentPage    uintptr
	pages          map[uintptr]*nativeGCMarkPage
	ownerQueues    map[int][]uintptr
	pendingOwners  []int
}

func newNativeGCMarkState(owner int) *nativeGCMarkState {
	return &nativeGCMarkState{
		collectorOwner: owner,
		currentOwner:   owner,
		pages:          make(map[uintptr]*nativeGCMarkPage),
		ownerQueues:    make(map[int][]uintptr),
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

func nativeGCMarkPageKey(block *nativeHeapBlock) uintptr {
	if block == nil {
		return 0
	}
	return nativeAllocatorPageBase(block.ptr)
}

func (state *nativeGCMarkState) enqueue(candidate uintptr) {
	block := resolveNativeHeapBlockLocked(candidate)
	if block == nil || block.marked {
		return
	}
	block.marked = true
	pageKey := nativeGCMarkPageKey(block)
	page := state.pages[pageKey]
	if page == nil {
		page = &nativeGCMarkPage{owner: nativeHeapBlockOwner(block)}
		state.pages[pageKey] = page
	}
	page.blocks = append(page.blocks, block)
	if page.queued || pageKey == state.currentPage {
		return
	}
	ownerQueue := state.ownerQueues[page.owner]
	wasEmpty := len(ownerQueue) == 0
	state.ownerQueues[page.owner] = append(ownerQueue, pageKey)
	page.queued = true
	if wasEmpty && page.owner != state.currentOwner {
		state.pendingOwners = append(state.pendingOwners, page.owner)
	}
}

func (state *nativeGCMarkState) popPage() *nativeGCMarkPage {
	for {
		queue := state.ownerQueues[state.currentOwner]
		if count := len(queue); count != 0 {
			pageKey := queue[count-1]
			state.ownerQueues[state.currentOwner] = queue[:count-1]
			page := state.pages[pageKey]
			if page == nil || len(page.blocks) == 0 {
				if page != nil {
					page.queued = false
				}
				continue
			}
			page.queued = false
			state.currentPage = pageKey
			nativeHeap.markPages++
			return page
		}
		if len(state.pendingOwners) == 0 {
			return nil
		}
		last := len(state.pendingOwners) - 1
		owner := state.pendingOwners[last]
		state.pendingOwners = state.pendingOwners[:last]
		if len(state.ownerQueues[owner]) == 0 {
			continue
		}
		state.currentOwner = owner
		if owner != state.collectorOwner {
			nativeHeap.markQueueSwitches++
		}
	}
}

func (state *nativeGCMarkState) drainPage(page *nativeGCMarkPage) {
	wordSize := uintptr(unsafe.Sizeof(uintptr(0)))
	for len(page.blocks) != 0 {
		last := len(page.blocks) - 1
		block := page.blocks[last]
		page.blocks[last] = nil
		page.blocks = page.blocks[:last]
		nativeHeap.markWork++
		count := block.size / wordSize
		for i := uintptr(0); i < count; i++ {
			word := *(*uintptr)(unsafe.Add(block.raw, i*wordSize))
			state.enqueue(word)
		}
	}
	state.currentPage = 0
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
		page := state.popPage()
		if page == nil {
			return
		}
		state.drainPage(page)
	}
}

func nativeGCMarkWork() (uint64, uint64) {
	nativeHeap.Lock()
	work := nativeHeap.markWork
	switches := nativeHeap.markQueueSwitches
	nativeHeap.Unlock()
	return work, switches
}

func nativeGCMarkPages() uint64 {
	nativeHeap.Lock()
	pages := nativeHeap.markPages
	nativeHeap.Unlock()
	return pages
}
