package main

/*
#include <stdint.h>
#include <stdlib.h>
*/
import "C"

import (
	"math"
	"sync"
	"time"
)

type nativeTimerWaiter struct {
	once        sync.Once
	timer       *time.Timer
	task        uintptr
	cooperative bool
	done        chan struct{}
}

var nativeTimers = struct {
	sync.Mutex
	waiters  map[*nativeTimerWaiter]struct{}
	stopping bool
}{waiters: map[*nativeTimerWaiter]struct{}{}}

//export tsnative_timer_bind_scheduler
func tsnative_timer_bind_scheduler(current, prepare, cancel, wake, help C.uintptr_t) {}

func nativeTimerDuration(milliseconds float64) time.Duration {
	if math.IsNaN(milliseconds) || math.IsInf(milliseconds, 0) || milliseconds < 0 {
		C.abort()
	}
	ns := milliseconds * 1_000_000
	if ns > float64(math.MaxInt64) {
		C.abort()
	}
	return time.Duration(ns)
}

func completeNativeTimer(waiter *nativeTimerWaiter) {
	waiter.once.Do(func() {
		nativeTimers.Lock()
		delete(nativeTimers.waiters, waiter)
		nativeTimers.Unlock()
		if waiter.cooperative {
			close(waiter.done)
			return
		}
		_ = schedulerWakeTask(waiter.task)
	})
}

func scheduleNativeTimer(duration time.Duration, task uintptr, cooperative bool) *nativeTimerWaiter {
	waiter := &nativeTimerWaiter{task: task, cooperative: cooperative, done: make(chan struct{})}
	nativeTimers.Lock()
	if nativeTimers.stopping {
		nativeTimers.Unlock()
		return nil
	}
	nativeTimers.waiters[waiter] = struct{}{}
	nativeTimers.Unlock()
	waiter.timer = time.AfterFunc(duration, func() { completeNativeTimer(waiter) })
	return waiter
}

//export tsnative_sleep_task
func tsnative_sleep_task(milliseconds C.double) C.int {
	duration := nativeTimerDuration(float64(milliseconds))
	if duration == 0 {
		return 1
	}
	task := schedulerCurrentTaskPtr()
	if task == 0 || tsnative_scheduler_prepare_park() != 0 {
		return -1
	}
	if scheduleNativeTimer(duration, task, false) == nil {
		tsnative_scheduler_cancel_park()
		return -1
	}
	return 0
}

//export tsnative_sleep_cooperative
func tsnative_sleep_cooperative(milliseconds C.double) {
	duration := nativeTimerDuration(float64(milliseconds))
	if duration == 0 {
		return
	}
	if schedulerCurrentTaskPtr() == 0 {
		time.Sleep(duration)
		return
	}
	waiter := scheduleNativeTimer(duration, 0, true)
	if waiter == nil {
		C.abort()
	}
	for {
		select {
		case <-waiter.done:
			return
		default:
		}
		if tsnative_scheduler_help_once() != 0 {
			continue
		}
		select {
		case <-waiter.done:
			return
		case <-time.After(100 * time.Microsecond):
		}
	}
}

//export tsnative_timer_shutdown
func tsnative_timer_shutdown() {
	nativeTimers.Lock()
	nativeTimers.stopping = true
	waiters := make([]*nativeTimerWaiter, 0, len(nativeTimers.waiters))
	for waiter := range nativeTimers.waiters {
		waiters = append(waiters, waiter)
	}
	nativeTimers.Unlock()

	for _, waiter := range waiters {
		if waiter.timer != nil {
			waiter.timer.Stop()
		}
		completeNativeTimer(waiter)
	}

	nativeTimers.Lock()
	nativeTimers.stopping = false
	nativeTimers.Unlock()
}
