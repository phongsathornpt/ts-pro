package main

import (
	"runtime"
	"testing"
	"unsafe"
)

func benchmarkTraceLayout(b *testing.B, atomicLayout bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	const blocks = 1024
	roots := make([]unsafe.Pointer, blocks)
	for i := range roots {
		if atomicLayout {
			roots[i] = tsnative_heap_alloc_atomic(2048)
		} else {
			roots[i] = tsnative_heap_alloc(2048)
		}
	}
	frame := tsnative_gc_enter(unsafe.Pointer(&roots[0]), uintptr(len(roots)))
	defer tsnative_gc_leave(frame)
	tsnative_gc_collect()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tsnative_gc_collect()
	}
	b.StopTimer()
	runtime.KeepAlive(roots)
}

func BenchmarkTraceConservative2MiB(b *testing.B) {
	benchmarkTraceLayout(b, false)
}

func BenchmarkTraceAtomic2MiB(b *testing.B) {
	benchmarkTraceLayout(b, true)
}
