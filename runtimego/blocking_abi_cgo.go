//go:build cgo

package main

/*
#include <stdint.h>
*/
import "C"

import "unsafe"

// The implementation in blocking.go is cgo-free. These wrappers are the
// temporary native ABI surface used by the c-archive build.

//export tsnative_blocking_bind_scheduler
func tsnative_blocking_bind_scheduler(current, prepare, cancel, wake, help C.uintptr_t) {
	nativeBlockingBindScheduler(uintptr(current), uintptr(prepare), uintptr(cancel), uintptr(wake), uintptr(help))
}

//export tsnative_blocking_submit
func tsnative_blocking_submit(entry, state unsafe.Pointer) unsafe.Pointer {
	return nativeBlockingSubmit(entry, state)
}

//export tsnative_blocking_job_wait_task
func tsnative_blocking_job_wait_task(raw unsafe.Pointer) C.int {
	return C.int(nativeBlockingJobWaitTask(raw))
}

//export tsnative_blocking_job_wait_cooperative
func tsnative_blocking_job_wait_cooperative(raw unsafe.Pointer) {
	nativeBlockingJobWaitCooperative(raw)
}

//export tsnative_blocking_job_release
func tsnative_blocking_job_release(raw unsafe.Pointer) {
	nativeBlockingJobRelease(raw)
}

//export tsnative_blocking_pool_shutdown
func tsnative_blocking_pool_shutdown() {
	nativeBlockingPoolShutdown()
}

//export tsnative_blocking_worker_count
func tsnative_blocking_worker_count() C.size_t {
	return C.size_t(nativeBlockingWorkerCount())
}

//export tsnative_blocking_active_jobs
func tsnative_blocking_active_jobs() C.size_t {
	return C.size_t(nativeBlockingActiveJobs())
}

//export tsnative_blocking_peak_active_jobs
func tsnative_blocking_peak_active_jobs() C.size_t {
	return C.size_t(nativeBlockingPeakActiveJobs())
}

//export tsnative_blocking_submitted_jobs
func tsnative_blocking_submitted_jobs() C.uint64_t {
	return C.uint64_t(nativeBlockingSubmittedJobs())
}

//export tsnative_blocking_completed_jobs
func tsnative_blocking_completed_jobs() C.uint64_t {
	return C.uint64_t(nativeBlockingCompletedJobs())
}
