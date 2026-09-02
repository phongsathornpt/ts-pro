package main

import "sync"

const nativeHeapBlockShardCount = 64

type nativeHeapBlockShard struct {
	sync.RWMutex
	blocks map[uintptr]*nativeHeapBlock
}

type nativeHeapBlockTable struct {
	shards [nativeHeapBlockShardCount]nativeHeapBlockShard
}

var nativeBlocks = newNativeHeapBlockTable()

func newNativeHeapBlockTable() *nativeHeapBlockTable {
	table := &nativeHeapBlockTable{}
	for i := range table.shards {
		table.shards[i].blocks = make(map[uintptr]*nativeHeapBlock)
	}
	return table
}

func nativeHeapBlockShardIndex(key uintptr) uintptr {
	x := uint64(key >> 4)
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return uintptr(x & (nativeHeapBlockShardCount - 1))
}
func (table *nativeHeapBlockTable) shard(key uintptr) *nativeHeapBlockShard {
	return &table.shards[nativeHeapBlockShardIndex(key)]
}

func (table *nativeHeapBlockTable) get(key uintptr) *nativeHeapBlock {
	shard := table.shard(key)
	shard.RLock()
	block := shard.blocks[key]
	shard.RUnlock()
	return block
}

func (table *nativeHeapBlockTable) set(key uintptr, block *nativeHeapBlock) {
	shard := table.shard(key)
	shard.Lock()
	shard.blocks[key] = block
	shard.Unlock()
}

func (table *nativeHeapBlockTable) rangeBlocks(visit func(uintptr, *nativeHeapBlock)) {
	for i := range table.shards {
		shard := &table.shards[i]
		shard.RLock()
		for key, block := range shard.blocks {
			visit(key, block)
		}
		shard.RUnlock()
	}
}
func (table *nativeHeapBlockTable) sweep(remove func(uintptr, *nativeHeapBlock) bool) {
	for i := range table.shards {
		shard := &table.shards[i]
		shard.Lock()
		for key, block := range shard.blocks {
			if remove(key, block) {
				delete(shard.blocks, key)
			}
		}
		shard.Unlock()
	}
}

func (table *nativeHeapBlockTable) count() int {
	count := 0
	table.rangeBlocks(func(_ uintptr, _ *nativeHeapBlock) { count++ })
	return count
}

func (table *nativeHeapBlockTable) drain() []*nativeHeapBlock {
	blocks := make([]*nativeHeapBlock, 0, table.count())
	for i := range table.shards {
		shard := &table.shards[i]
		shard.Lock()
		for key, block := range shard.blocks {
			blocks = append(blocks, block)
			delete(shard.blocks, key)
		}
		shard.Unlock()
	}
	return blocks
}
func (table *nativeHeapBlockTable) update(key uintptr, update func(*nativeHeapBlock)) bool {
	shard := table.shard(key)
	shard.Lock()
	block := shard.blocks[key]
	if block != nil {
		update(block)
	}
	shard.Unlock()
	return block != nil
}
