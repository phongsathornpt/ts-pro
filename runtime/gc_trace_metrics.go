package runtime

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

func gcTraceWords() uint64 {
	return nativeGCTraceStats().words
}

func gcTraceAtomicBlocks() uint64 {
	return nativeGCTraceStats().atomic
}

func gcTracePreciseBlocks() uint64 {
	return nativeGCTraceStats().precise
}

func gcTraceConservativeBlocks() uint64 {
	return nativeGCTraceStats().conservative
}
