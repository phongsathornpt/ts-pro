package main

/*
#include <stdint.h>
*/
import "C"

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

//export tsnative_gc_minor_collections
func tsnative_gc_minor_collections() C.uint64_t {
	return C.uint64_t(nativeGCMinorCollections.Load())
}

//export tsnative_gc_major_collections
func tsnative_gc_major_collections() C.uint64_t {
	return C.uint64_t(nativeGCMajorCollections.Load())
}

//export tsnative_gc_promoted_blocks
func tsnative_gc_promoted_blocks() C.uint64_t {
	return C.uint64_t(nativeGCPromotedBlocks.Load())
}

//export tsnative_gc_promoted_bytes
func tsnative_gc_promoted_bytes() C.uint64_t {
	return C.uint64_t(nativeGCPromotedBytes.Load())
}

//export tsnative_gc_old_bytes
func tsnative_gc_old_bytes() C.uint64_t {
	return C.uint64_t(nativeHeapOldBytes.Load())
}

//export tsnative_gc_minor_old_scans
func tsnative_gc_minor_old_scans() C.uint64_t {
	return C.uint64_t(nativeGCMinorOldScans.Load())
}

//export tsnative_gc_nursery_bytes
func tsnative_gc_nursery_bytes() C.uint64_t {
	return C.uint64_t(nativeNurseryLiveBytes())
}

//export tsnative_gc_nursery_blocks
func tsnative_gc_nursery_blocks() C.uint64_t {
	return C.uint64_t(nativeNurseryBlockCount())
}

//export tsnative_gc_minor_nursery_scans
func tsnative_gc_minor_nursery_scans() C.uint64_t {
	return C.uint64_t(nativeGCMinorNurseryScans.Load())
}
