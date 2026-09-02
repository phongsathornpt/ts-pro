package main

import (
	"runtime"
	"sync"
	"unsafe"
)

const (
	nativeGCMarkMaxWorkers        = 8
	nativeGCParallelMarkMinBlocks = 512
)

type nativeGCMarkWorkerRole uint8

const (
	nativeGCMarkWorkerCollector nativeGCMarkWorkerRole = iota
	nativeGCMarkWorkerHelper
	nativeGCMarkWorkerSchedulerDonor
)

type nativeGCMarkPage struct {
	owner  int
	blocks []*nativeHeapBlock
	queued bool
	active bool
}

type nativeGCMarkState struct {
	mu sync.Mutex

	cond           *sync.Cond
	collectorOwner int
	pages          map[uintptr]*nativeGCMarkPage
	ownerQueues    map[int][]uintptr
	ownerOrder     []int
	ownerSeen      map[int]bool
	activePages    int

	work            uint64
	pagesScanned    uint64
	queueSwitches   uint64
	assistPages     uint64
	idleAssistPages uint64
}

func newNativeGCMarkState(owner int) *nativeGCMarkState {
	state := &nativeGCMarkState{
		collectorOwner: owner,
		pages:          make(map[uintptr]*nativeGCMarkPage),
		ownerQueues:    make(map[int][]uintptr),
		ownerSeen:      make(map[int]bool),
	}
	state.cond = sync.NewCond(&state.mu)
	return state
}

func configuredNativeGCMarkWorkers(blocks int) int {
	if blocks < nativeGCParallelMarkMinBlocks {
		return 1
	}
	limit := runtime.GOMAXPROCS(0)
	if schedulerWorkers := configuredSchedulerWorkers(); schedulerWorkers < limit {
		limit = schedulerWorkers
	}
	if limit > nativeGCMarkMaxWorkers {
		limit = nativeGCMarkMaxWorkers
	}
	if limit < 1 {
		limit = 1
	}
	return schedulerLimit("TSNATIVE_GC_MARK_WORKERS", limit, limit)
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

func (state *nativeGCMarkState) queuePageLocked(pageKey uintptr, page *nativeGCMarkPage) {
	if page == nil || page.queued || page.active || len(page.blocks) == 0 {
		return
	}
	if !state.ownerSeen[page.owner] {
		state.ownerSeen[page.owner] = true
		state.ownerOrder = append(state.ownerOrder, page.owner)
	}
	state.ownerQueues[page.owner] = append(state.ownerQueues[page.owner], pageKey)
	page.queued = true
	state.cond.Signal()
}

func (state *nativeGCMarkState) enqueue(candidate uintptr) {
	if candidate == 0 {
		return
	}
	state.mu.Lock()
	block := resolveNativeHeapBlockLocked(candidate)
	if block == nil || block.marked {
		state.mu.Unlock()
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
	state.queuePageLocked(pageKey, page)
	state.mu.Unlock()
}

func (state *nativeGCMarkState) popOwnerPageLocked(owner int) *nativeGCMarkPage {
	queue := state.ownerQueues[owner]
	for len(queue) != 0 {
		last := len(queue) - 1
		pageKey := queue[last]
		queue[last] = 0
		queue = queue[:last]
		state.ownerQueues[owner] = queue
		page := state.pages[pageKey]
		if page == nil || page.active || len(page.blocks) == 0 {
			if page != nil {
				page.queued = false
			}
			continue
		}
		page.queued = false
		page.active = true
		state.activePages++
		state.pagesScanned++
		return page
	}
	return nil
}

func (state *nativeGCMarkState) popPageLocked(preferredOwner int, role nativeGCMarkWorkerRole) *nativeGCMarkPage {
	if page := state.popOwnerPageLocked(preferredOwner); page != nil {
		if role != nativeGCMarkWorkerCollector {
			state.assistPages++
		}
		if role == nativeGCMarkWorkerSchedulerDonor {
			state.idleAssistPages++
		}
		return page
	}
	for _, owner := range state.ownerOrder {
		if owner == preferredOwner {
			continue
		}
		page := state.popOwnerPageLocked(owner)
		if page == nil {
			continue
		}
		state.queueSwitches++
		if role != nativeGCMarkWorkerCollector {
			state.assistPages++
		}
		if role == nativeGCMarkWorkerSchedulerDonor {
			state.idleAssistPages++
		}
		return page
	}
	return nil
}

func (state *nativeGCMarkState) drainPage(page *nativeGCMarkPage) {
	wordSize := uintptr(unsafe.Sizeof(uintptr(0)))
	for {
		state.mu.Lock()
		if len(page.blocks) == 0 {
			page.active = false
			state.activePages--
			state.cond.Broadcast()
			state.mu.Unlock()
			return
		}
		last := len(page.blocks) - 1
		block := page.blocks[last]
		page.blocks[last] = nil
		page.blocks = page.blocks[:last]
		state.work++
		state.mu.Unlock()

		count := block.size / wordSize
		for i := uintptr(0); i < count; i++ {
			word := *(*uintptr)(unsafe.Add(block.raw, i*wordSize))
			state.enqueue(word)
		}
	}
}

func (state *nativeGCMarkState) worker(role nativeGCMarkWorkerRole, preferredOwner int) {
	for {
		state.mu.Lock()
		page := state.popPageLocked(preferredOwner, role)
		for page == nil && state.activePages != 0 {
			state.cond.Wait()
			page = state.popPageLocked(preferredOwner, role)
		}
		if page == nil {
			state.mu.Unlock()
			return
		}
		state.mu.Unlock()
		state.drainPage(page)
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

	workerCount := configuredNativeGCMarkWorkers(len(nativeHeap.blocks))
	state.mu.Lock()
	hasWork := len(state.pages) != 0
	state.mu.Unlock()
	if !hasWork {
		workerCount = 1
	}

	if workerCount == 1 {
		state.worker(nativeGCMarkWorkerCollector, state.collectorOwner)
	} else {
		helperCount := workerCount - 1
		idleDonors := schedulerStartGCIdleAssist(state, helperCount)
		fallbackHelpers := helperCount - idleDonors

		var ready sync.WaitGroup
		var workers sync.WaitGroup
		start := make(chan struct{})
		ready.Add(fallbackHelpers)
		workers.Add(fallbackHelpers)
		for workerID := 0; workerID < fallbackHelpers; workerID++ {
			go func(id int) {
				defer workers.Done()
				ready.Done()
				<-start
				state.worker(nativeGCMarkWorkerHelper, id)
			}(workerID)
		}
		ready.Wait()
		close(start)
		state.worker(nativeGCMarkWorkerCollector, state.collectorOwner)
		workers.Wait()
		schedulerStopGCIdleAssist(state)
		nativeHeap.markAssistWorkers += uint64(helperCount)
		nativeHeap.markIdleAssistWorkers += uint64(idleDonors)
	}

	nativeHeap.markWork += state.work
	nativeHeap.markPages += state.pagesScanned
	nativeHeap.markQueueSwitches += state.queueSwitches
	nativeHeap.markAssistPages += state.assistPages
	nativeHeap.markIdleAssistPages += state.idleAssistPages
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

func nativeGCMarkAssist() (uint64, uint64) {
	nativeHeap.Lock()
	workers := nativeHeap.markAssistWorkers
	pages := nativeHeap.markAssistPages
	nativeHeap.Unlock()
	return workers, pages
}

func nativeGCMarkIdleAssist() (uint64, uint64) {
	nativeHeap.Lock()
	workers := nativeHeap.markIdleAssistWorkers
	pages := nativeHeap.markIdleAssistPages
	nativeHeap.Unlock()
	return workers, pages
}
