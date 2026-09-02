package main

/*
#include <stdint.h>
*/
import "C"

import (
	"sync"
	"sync/atomic"
	"time"
)

type nativeMeasuredMutex struct {
	mu           sync.Mutex
	acquisitions atomic.Uint64
	contended    atomic.Uint64
	waitNanos    atomic.Uint64
}

type nativeLockMetrics struct {
	acquisitions uint64
	contended    uint64
	waitNanos    uint64
}

func (m *nativeMeasuredMutex) Lock() {
	if m.mu.TryLock() {
		m.acquisitions.Add(1)
		return
	}
	m.contended.Add(1)
	started := time.Now()
	m.mu.Lock()
	m.waitNanos.Add(uint64(time.Since(started)))
	m.acquisitions.Add(1)
}

func (m *nativeMeasuredMutex) Unlock() { m.mu.Unlock() }

func (m *nativeMeasuredMutex) snapshot() nativeLockMetrics {
	return nativeLockMetrics{
		acquisitions: m.acquisitions.Load(),
		contended:    m.contended.Load(),
		waitNanos:    m.waitNanos.Load(),
	}
}

func (m *nativeMeasuredMutex) resetMetrics() {
	m.acquisitions.Store(0)
	m.contended.Store(0)
	m.waitNanos.Store(0)
}

func nativeHeapLockMetrics() nativeLockMetrics { return nativeHeap.nativeMeasuredMutex.snapshot() }
func nativeRootLockMetrics() nativeLockMetrics { return nativeRoots.metrics() }

//export tsnative_heap_lock_acquisitions
func tsnative_heap_lock_acquisitions() C.uint64_t {
	return C.uint64_t(nativeHeapLockMetrics().acquisitions)
}

//export tsnative_heap_lock_contentions
func tsnative_heap_lock_contentions() C.uint64_t {
	return C.uint64_t(nativeHeapLockMetrics().contended)
}

//export tsnative_heap_lock_wait_ns
func tsnative_heap_lock_wait_ns() C.uint64_t { return C.uint64_t(nativeHeapLockMetrics().waitNanos) }

//export tsnative_root_lock_acquisitions
func tsnative_root_lock_acquisitions() C.uint64_t {
	return C.uint64_t(nativeRootLockMetrics().acquisitions)
}

//export tsnative_root_lock_contentions
func tsnative_root_lock_contentions() C.uint64_t {
	return C.uint64_t(nativeRootLockMetrics().contended)
}

//export tsnative_root_lock_wait_ns
func tsnative_root_lock_wait_ns() C.uint64_t { return C.uint64_t(nativeRootLockMetrics().waitNanos) }
