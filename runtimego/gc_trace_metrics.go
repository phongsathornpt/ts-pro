package runtimego

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

func tsnative_gc_trace_words() uint64 {
	return nativeGCTraceStats().words
}

func tsnative_gc_trace_atomic_blocks() uint64 {
	return nativeGCTraceStats().atomic
}

func tsnative_gc_trace_precise_blocks() uint64 {
	return nativeGCTraceStats().precise
}

func tsnative_gc_trace_conservative_blocks() uint64 {
	return nativeGCTraceStats().conservative
}
