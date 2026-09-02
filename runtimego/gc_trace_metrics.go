package main

/*
#include <stdint.h>
*/
import "C"

type nativeGCTraceMetrics struct {
	words        uint64
	atomic       uint64
	precise      uint64
	conservative uint64
}

func nativeGCTraceStats() nativeGCTraceMetrics {
	nativeHeap.Lock()
	metrics := nativeGCTraceMetrics{
		words:        nativeHeap.markTraceWords,
		atomic:       nativeHeap.markAtomicBlocks,
		precise:      nativeHeap.markPreciseBlocks,
		conservative: nativeHeap.markConservativeBlocks,
	}
	nativeHeap.Unlock()
	return metrics
}

//export tsnative_gc_trace_words
func tsnative_gc_trace_words() C.uint64_t {
	return C.uint64_t(nativeGCTraceStats().words)
}

//export tsnative_gc_trace_atomic_blocks
func tsnative_gc_trace_atomic_blocks() C.uint64_t {
	return C.uint64_t(nativeGCTraceStats().atomic)
}

//export tsnative_gc_trace_precise_blocks
func tsnative_gc_trace_precise_blocks() C.uint64_t {
	return C.uint64_t(nativeGCTraceStats().precise)
}

//export tsnative_gc_trace_conservative_blocks
func tsnative_gc_trace_conservative_blocks() C.uint64_t {
	return C.uint64_t(nativeGCTraceStats().conservative)
}
