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

type nativeRWLockMetrics struct {
	read  nativeLockMetrics
	write nativeLockMetrics
}

type nativeMeasuredRWMutex struct {
	mu sync.RWMutex

	readAcquisitions  atomic.Uint64
	readContended     atomic.Uint64
	readWaitNanos     atomic.Uint64
	writeAcquisitions atomic.Uint64
	writeContended    atomic.Uint64
	writeWaitNanos    atomic.Uint64
}

func (m *nativeMeasuredRWMutex) RLock() {
	if m.mu.TryRLock() {
		m.readAcquisitions.Add(1)
		return
	}
	m.readContended.Add(1)
	started := time.Now()
	m.mu.RLock()
	m.readWaitNanos.Add(uint64(time.Since(started)))
	m.readAcquisitions.Add(1)
}

func (m *nativeMeasuredRWMutex) RUnlock() { m.mu.RUnlock() }

func (m *nativeMeasuredRWMutex) Lock() {
	if m.mu.TryLock() {
		m.writeAcquisitions.Add(1)
		return
	}
	m.writeContended.Add(1)
	started := time.Now()
	m.mu.Lock()
	m.writeWaitNanos.Add(uint64(time.Since(started)))
	m.writeAcquisitions.Add(1)
}

func (m *nativeMeasuredRWMutex) Unlock() { m.mu.Unlock() }

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

func (m *nativeMeasuredRWMutex) snapshot() nativeRWLockMetrics {
	return nativeRWLockMetrics{
		read: nativeLockMetrics{
			acquisitions: m.readAcquisitions.Load(),
			contended:    m.readContended.Load(),
			waitNanos:    m.readWaitNanos.Load(),
		},
		write: nativeLockMetrics{
			acquisitions: m.writeAcquisitions.Load(),
			contended:    m.writeContended.Load(),
			waitNanos:    m.writeWaitNanos.Load(),
		},
	}
}

func (m *nativeMeasuredRWMutex) resetMetrics() {
	m.readAcquisitions.Store(0)
	m.readContended.Store(0)
	m.readWaitNanos.Store(0)
	m.writeAcquisitions.Store(0)
	m.writeContended.Store(0)
	m.writeWaitNanos.Store(0)
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

func nativeWorldLockMetrics() nativeRWLockMetrics { return nativeHeapWorld.snapshot() }
func nativeBlockLockMetrics() nativeRWLockMetrics { return nativeBlocks.metrics() }

//export tsnative_world_read_lock_acquisitions
func tsnative_world_read_lock_acquisitions() C.uint64_t {
	return C.uint64_t(nativeWorldLockMetrics().read.acquisitions)
}

//export tsnative_world_read_lock_contentions
func tsnative_world_read_lock_contentions() C.uint64_t {
	return C.uint64_t(nativeWorldLockMetrics().read.contended)
}

//export tsnative_world_read_lock_wait_ns
func tsnative_world_read_lock_wait_ns() C.uint64_t {
	return C.uint64_t(nativeWorldLockMetrics().read.waitNanos)
}

//export tsnative_world_write_lock_acquisitions
func tsnative_world_write_lock_acquisitions() C.uint64_t {
	return C.uint64_t(nativeWorldLockMetrics().write.acquisitions)
}

//export tsnative_world_write_lock_contentions
func tsnative_world_write_lock_contentions() C.uint64_t {
	return C.uint64_t(nativeWorldLockMetrics().write.contended)
}

//export tsnative_world_write_lock_wait_ns
func tsnative_world_write_lock_wait_ns() C.uint64_t {
	return C.uint64_t(nativeWorldLockMetrics().write.waitNanos)
}

//export tsnative_block_read_lock_acquisitions
func tsnative_block_read_lock_acquisitions() C.uint64_t {
	return C.uint64_t(nativeBlockLockMetrics().read.acquisitions)
}

//export tsnative_block_read_lock_contentions
func tsnative_block_read_lock_contentions() C.uint64_t {
	return C.uint64_t(nativeBlockLockMetrics().read.contended)
}

//export tsnative_block_read_lock_wait_ns
func tsnative_block_read_lock_wait_ns() C.uint64_t {
	return C.uint64_t(nativeBlockLockMetrics().read.waitNanos)
}

//export tsnative_block_write_lock_acquisitions
func tsnative_block_write_lock_acquisitions() C.uint64_t {
	return C.uint64_t(nativeBlockLockMetrics().write.acquisitions)
}

//export tsnative_block_write_lock_contentions
func tsnative_block_write_lock_contentions() C.uint64_t {
	return C.uint64_t(nativeBlockLockMetrics().write.contended)
}

//export tsnative_block_write_lock_wait_ns
func tsnative_block_write_lock_wait_ns() C.uint64_t {
	return C.uint64_t(nativeBlockLockMetrics().write.waitNanos)
}
