package runtimego

import (
	"sync"
	"sync/atomic"
)

const (
	nativeNurseryDefaultBytes = initialGCThreshold
	nativeNurseryMaxBytes     = 64 * 1024 * 1024
	nativeNurseryBucketCount  = nativeSchedulerMaxWorkers + 1
)

type nativeHeapGeneration uint8

const (
	nativeHeapGenerationNursery nativeHeapGeneration = iota
	nativeHeapGenerationOld
)

type nativeNurseryBlockBucket struct {
	sync.Mutex
	blocks []*nativeHeapBlock
}

var nativeNurseryBytes [nativeNurseryBucketCount]atomic.Uint64
var nativeNurseryBlocks [nativeNurseryBucketCount]nativeNurseryBlockBucket
var nativeNurseryLimitBytes atomic.Uint64
var nativeGCMinorCollections atomic.Uint64
var nativeGCMajorCollections atomic.Uint64
var nativeGCPromotedBlocks atomic.Uint64
var nativeGCPromotedBytes atomic.Uint64
var nativeGCMinorOldScans atomic.Uint64
var nativeGCMinorNurseryScans atomic.Uint64

func nativeNurseryBucket(owner int) int {
	if owner >= 0 && owner < nativeSchedulerMaxWorkers {
		return owner
	}
	return nativeSchedulerMaxWorkers
}

func configuredNativeNurseryBytes() uint64 {
	return uint64(schedulerLimit(
		"TSNATIVE_GC_NURSERY_BYTES",
		nativeNurseryDefaultBytes,
		nativeNurseryMaxBytes,
	))
}

func nativeNurseryLimit() uint64 {
	if value := nativeNurseryLimitBytes.Load(); value != 0 {
		return value
	}
	value := configuredNativeNurseryBytes()
	if nativeNurseryLimitBytes.CompareAndSwap(0, value) {
		return value
	}
	return nativeNurseryLimitBytes.Load()
}

func nativeNurseryAdd(owner int, size uintptr) uint64 {
	return nativeNurseryBytes[nativeNurseryBucket(owner)].Add(uint64(size))
}
func nativeNurseryTrack(owner int, block *nativeHeapBlock) {
	if block == nil {
		return
	}
	bucket := &nativeNurseryBlocks[nativeNurseryBucket(owner)]
	bucket.Lock()
	bucket.blocks = append(bucket.blocks, block)
	bucket.Unlock()
}

func takeNativeNurseryBlocks() []*nativeHeapBlock {
	blocks := make([]*nativeHeapBlock, 0)
	for i := range nativeNurseryBlocks {
		bucket := &nativeNurseryBlocks[i]
		bucket.Lock()
		blocks = append(blocks, bucket.blocks...)
		clear(bucket.blocks)
		bucket.blocks = nil
		bucket.Unlock()
	}
	return blocks
}

func clearNativeNurseryBlocks() {
	for i := range nativeNurseryBlocks {
		bucket := &nativeNurseryBlocks[i]
		bucket.Lock()
		clear(bucket.blocks)
		bucket.blocks = nil
		bucket.Unlock()
	}
}

func nativeNurseryBlockCount() int {
	count := 0
	for i := range nativeNurseryBlocks {
		bucket := &nativeNurseryBlocks[i]
		bucket.Lock()
		count += len(bucket.blocks)
		bucket.Unlock()
	}
	return count
}

func nativeNurseryLiveBytes() uint64 {
	var total uint64
	for i := range nativeNurseryBytes {
		total += nativeNurseryBytes[i].Load()
	}
	return total
}

func clearNativeNurseryBytes() {
	for i := range nativeNurseryBytes {
		nativeNurseryBytes[i].Store(0)
	}
}

func resetNativeNurseryState() {
	clearNativeNurseryBytes()
	clearNativeNurseryBlocks()
	nativeNurseryLimitBytes.Store(0)
}

func resetNativeGenerationalMetrics() {
	nativeGCMinorCollections.Store(0)
	nativeGCMajorCollections.Store(0)
	nativeGCPromotedBlocks.Store(0)
	nativeGCPromotedBytes.Store(0)
	nativeGCMinorOldScans.Store(0)
	nativeGCMinorNurseryScans.Store(0)
}

func gcMinorCollections() uint64 {
	return nativeGCMinorCollections.Load()
}

func gcMajorCollections() uint64 {
	return nativeGCMajorCollections.Load()
}

func gcPromotedBlocks() uint64 {
	return nativeGCPromotedBlocks.Load()
}

func gcPromotedBytes() uint64 {
	return nativeGCPromotedBytes.Load()
}

func gcOldBytes() uint64 {
	return nativeHeapOldBytes.Load()
}

func gcMinorOldScans() uint64 {
	return nativeGCMinorOldScans.Load()
}

func gcNurseryBytes() uint64 {
	return nativeNurseryLiveBytes()
}

func gcNurseryBlocks() uint64 {
	return uint64(nativeNurseryBlockCount())
}

func gcMinorNurseryScans() uint64 {
	return nativeGCMinorNurseryScans.Load()
}
