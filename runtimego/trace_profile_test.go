package runtimego

import (
	"runtime"
	"testing"
	"unsafe"
)

func benchmarkTraceLayout(b *testing.B, atomicLayout bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	heapShutdown()
	defer heapShutdown()

	const blocks = 1024
	roots := make([]unsafe.Pointer, blocks)
	for i := range roots {
		if atomicLayout {
			roots[i] = heapAllocAtomic(2048)
		} else {
			roots[i] = heapAlloc(2048)
		}
	}
	frame := gcEnter(unsafe.Pointer(&roots[0]), uintptr(len(roots)))
	defer gcLeave(frame)
	gcCollect()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gcCollect()
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
