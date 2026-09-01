package main

/*
#include <stdint.h>
#include <stdlib.h>
typedef void *(*tsnative_timer_current_fn)(void);
typedef int (*tsnative_timer_int0_fn)(void);
typedef void (*tsnative_timer_void0_fn)(void);
typedef int (*tsnative_timer_wake_fn)(void *);
static void *tsnative_timer_call_current(uintptr_t fn) { return fn ? ((tsnative_timer_current_fn)fn)() : NULL; }
static int tsnative_timer_call_int0(uintptr_t fn) { return fn ? ((tsnative_timer_int0_fn)fn)() : -1; }
static void tsnative_timer_call_void0(uintptr_t fn) { if (fn) ((tsnative_timer_void0_fn)fn)(); }
static int tsnative_timer_call_wake(uintptr_t fn, void *task) { return fn ? ((tsnative_timer_wake_fn)fn)(task) : -1; }
*/
import "C"

import (
	"math"
	"sync"
	"time"
	"unsafe"
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

type nativeTimerSchedulerHooks struct {
	current uintptr
	prepare uintptr
	cancel  uintptr
	wake    uintptr
	help    uintptr
}

var nativeTimerScheduler struct {
	sync.RWMutex
	hooks nativeTimerSchedulerHooks
}

//export tsnative_timer_bind_scheduler
func tsnative_timer_bind_scheduler(current, prepare, cancel, wake, help C.uintptr_t) {
	nativeTimerScheduler.Lock()
	nativeTimerScheduler.hooks = nativeTimerSchedulerHooks{uintptr(current), uintptr(prepare), uintptr(cancel), uintptr(wake), uintptr(help)}
	nativeTimerScheduler.Unlock()
}

func timerSchedulerHooks() nativeTimerSchedulerHooks {
	nativeTimerScheduler.RLock()
	hooks := nativeTimerScheduler.hooks
	nativeTimerScheduler.RUnlock()
	return hooks
}

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
		hooks := timerSchedulerHooks()
		_ = C.tsnative_timer_call_wake(C.uintptr_t(hooks.wake), unsafe.Pointer(waiter.task))
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
	hooks := timerSchedulerHooks()
	task := C.tsnative_timer_call_current(C.uintptr_t(hooks.current))
	if task == nil || C.tsnative_timer_call_int0(C.uintptr_t(hooks.prepare)) != 0 {
		return -1
	}
	if scheduleNativeTimer(duration, uintptr(unsafe.Pointer(task)), false) == nil {
		C.tsnative_timer_call_void0(C.uintptr_t(hooks.cancel))
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
	hooks := timerSchedulerHooks()
	if C.tsnative_timer_call_current(C.uintptr_t(hooks.current)) == nil {
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
		if hooks.help != 0 && C.tsnative_timer_call_int0(C.uintptr_t(hooks.help)) != 0 {
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
