package main

/*
#include <stdint.h>
#include <stdlib.h>
typedef void *(*tsnative_channel_current_fn)(void);
typedef int (*tsnative_channel_int0_fn)(void);
typedef void (*tsnative_channel_void0_fn)(void);
typedef int (*tsnative_channel_wake_fn)(void *);
static void *tsnative_channel_call_current(uintptr_t fn) { return fn ? ((tsnative_channel_current_fn)fn)() : NULL; }
static int tsnative_channel_call_int0(uintptr_t fn) { return fn ? ((tsnative_channel_int0_fn)fn)() : -1; }
static void tsnative_channel_call_void0(uintptr_t fn) { if (fn) ((tsnative_channel_void0_fn)fn)(); }
static int tsnative_channel_call_wake(uintptr_t fn, void *task) { return fn ? ((tsnative_channel_wake_fn)fn)(task) : -1; }
*/
import "C"

import (
	"math"
	"runtime"
	"sync"
	"time"
	"unsafe"
)

type nativeChannelSchedulerHooks struct {
	current uintptr
	prepare uintptr
	cancel  uintptr
	wake    uintptr
	help    uintptr
}

type nativeF64ChannelWaiter struct {
	task        uintptr
	value       float64
	out         uintptr
	cooperative bool
	done        chan struct{}
	once        sync.Once
}

type nativeF64Channel struct {
	mu        sync.Mutex
	cond      *sync.Cond
	capacity  int
	buffer    []float64
	head      int
	tail      int
	count     int
	hasValue  bool
	slot      float64
	sendQueue []*nativeF64ChannelWaiter
	recvQueue []*nativeF64ChannelWaiter
}

var nativeChannelScheduler struct {
	sync.RWMutex
	hooks nativeChannelSchedulerHooks
}

var nativeChannels = struct {
	sync.Mutex
	byHandle map[uintptr]*nativeF64Channel
}{byHandle: map[uintptr]*nativeF64Channel{}}

//export tsnative_channel_bind_scheduler
func tsnative_channel_bind_scheduler(current, prepare, cancel, wake, help C.uintptr_t) {
	nativeChannelScheduler.Lock()
	nativeChannelScheduler.hooks = nativeChannelSchedulerHooks{uintptr(current), uintptr(prepare), uintptr(cancel), uintptr(wake), uintptr(help)}
	nativeChannelScheduler.Unlock()
}

func channelSchedulerHooks() nativeChannelSchedulerHooks {
	nativeChannelScheduler.RLock()
	hooks := nativeChannelScheduler.hooks
	nativeChannelScheduler.RUnlock()
	return hooks
}

func lookupNativeF64Channel(raw unsafe.Pointer) *nativeF64Channel {
	if raw == nil {
		return nil
	}
	nativeChannels.Lock()
	channel := nativeChannels.byHandle[uintptr(raw)]
	nativeChannels.Unlock()
	return channel
}
func newNativeF64Channel(capacity int) unsafe.Pointer {
	if capacity < 0 {
		C.abort()
	}
	raw := tsnative_heap_alloc(1)
	channel := &nativeF64Channel{capacity: capacity}
	if capacity > 0 {
		channel.buffer = make([]float64, capacity)
	}
	channel.cond = sync.NewCond(&channel.mu)
	nativeChannels.Lock()
	nativeChannels.byHandle[uintptr(raw)] = channel
	nativeChannels.Unlock()
	registerNativeHeapFinalizer(raw, func() {
		nativeChannels.Lock()
		delete(nativeChannels.byHandle, uintptr(raw))
		nativeChannels.Unlock()
	})
	return raw
}

func popChannelWaiter(queue *[]*nativeF64ChannelWaiter) *nativeF64ChannelWaiter {
	if len(*queue) == 0 {
		return nil
	}
	waiter := (*queue)[0]
	copy((*queue)[0:], (*queue)[1:])
	*queue = (*queue)[:len(*queue)-1]
	return waiter
}
func deliverNativeChannelValue(waiter *nativeF64ChannelWaiter, value float64) {
	if waiter.cooperative {
		waiter.value = value
		return
	}
	*(*C.double)(unsafe.Pointer(waiter.out)) = C.double(value)
}

func finishNativeChannelWaiter(waiter *nativeF64ChannelWaiter) {
	if waiter == nil {
		return
	}
	waiter.once.Do(func() {
		if waiter.cooperative {
			close(waiter.done)
			return
		}
		hooks := channelSchedulerHooks()
		_ = C.tsnative_channel_call_wake(C.uintptr_t(hooks.wake), unsafe.Pointer(waiter.task))
	})
}

//export tsnative_channel_f64_new
func tsnative_channel_f64_new(capacity C.size_t) unsafe.Pointer {
	if uint64(capacity) > uint64(math.MaxInt) {
		C.abort()
	}
	return newNativeF64Channel(int(capacity))
}

//export tsnative_channel_f64_new_checked
func tsnative_channel_f64_new_checked(capacity C.double) unsafe.Pointer {
	value := float64(capacity)
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > float64(math.MaxInt) || math.Trunc(value) != value {
		C.abort()
	}
	return newNativeF64Channel(int(value))
}

//export tsnative_channel_f64_try_send
func tsnative_channel_f64_try_send(raw unsafe.Pointer, value C.double) C.int {
	channel := lookupNativeF64Channel(raw)
	if channel == nil {
		return -1
	}
	channel.mu.Lock()
	defer channel.mu.Unlock()
	if channel.capacity == 0 {
		if channel.hasValue {
			return 0
		}
		channel.slot = float64(value)
		channel.hasValue = true
		channel.cond.Broadcast()
		return 1
	}
	if channel.count == channel.capacity {
		return 0
	}
	channel.buffer[channel.tail] = float64(value)
	channel.tail = (channel.tail + 1) % channel.capacity
	channel.count++
	channel.cond.Broadcast()
	return 1
}

//export tsnative_channel_f64_try_recv
func tsnative_channel_f64_try_recv(raw, out unsafe.Pointer) C.int {
	channel := lookupNativeF64Channel(raw)
	if channel == nil || out == nil {
		return -1
	}
	channel.mu.Lock()
	defer channel.mu.Unlock()
	if channel.capacity == 0 {
		if !channel.hasValue {
			return 0
		}
		*(*C.double)(out) = C.double(channel.slot)
		channel.hasValue = false
		channel.cond.Broadcast()
		return 1
	}
	if channel.count == 0 {
		return 0
	}
	*(*C.double)(out) = C.double(channel.buffer[channel.head])
	channel.head = (channel.head + 1) % channel.capacity
	channel.count--
	channel.cond.Broadcast()
	return 1
}

//export tsnative_channel_f64_try_recv_or
func tsnative_channel_f64_try_recv_or(raw unsafe.Pointer, fallback C.double) C.double {
	value := fallback
	if tsnative_channel_f64_try_recv(raw, unsafe.Pointer(&value)) == 1 {
		return value
	}
	return fallback
}

//export tsnative_channel_f64_send
func tsnative_channel_f64_send(raw unsafe.Pointer, value C.double) {
	channel := lookupNativeF64Channel(raw)
	if channel == nil {
		C.abort()
	}
	channel.mu.Lock()
	defer channel.mu.Unlock()
	if channel.capacity == 0 {
		for channel.hasValue {
			channel.cond.Wait()
		}
		channel.slot = float64(value)
		channel.hasValue = true
		channel.cond.Broadcast()
		for channel.hasValue {
			channel.cond.Wait()
		}
		return
	}
	for channel.count == channel.capacity {
		channel.cond.Wait()
	}
	channel.buffer[channel.tail] = float64(value)
	channel.tail = (channel.tail + 1) % channel.capacity
	channel.count++
	channel.cond.Broadcast()
}

//export tsnative_channel_f64_recv
func tsnative_channel_f64_recv(raw unsafe.Pointer) C.double {
	channel := lookupNativeF64Channel(raw)
	if channel == nil {
		C.abort()
	}
	channel.mu.Lock()
	defer channel.mu.Unlock()
	if channel.capacity == 0 {
		for !channel.hasValue {
			channel.cond.Wait()
		}
		value := channel.slot
		channel.hasValue = false
		channel.cond.Broadcast()
		return C.double(value)
	}
	for channel.count == 0 {
		channel.cond.Wait()
	}
	value := channel.buffer[channel.head]
	channel.head = (channel.head + 1) % channel.capacity
	channel.count--
	channel.cond.Broadcast()
	return C.double(value)
}

//export tsnative_channel_f64_send_task
func tsnative_channel_f64_send_task(raw unsafe.Pointer, value C.double) C.int {
	channel := lookupNativeF64Channel(raw)
	if channel == nil {
		return -1
	}
	hooks := channelSchedulerHooks()
	task := C.tsnative_channel_call_current(C.uintptr_t(hooks.current))
	if task == nil {
		return -1
	}
	channel.mu.Lock()
	if receiver := popChannelWaiter(&channel.recvQueue); receiver != nil {
		deliverNativeChannelValue(receiver, float64(value))
		channel.mu.Unlock()
		finishNativeChannelWaiter(receiver)
		return 1
	}
	if channel.capacity != 0 && channel.count < channel.capacity {
		channel.buffer[channel.tail] = float64(value)
		channel.tail = (channel.tail + 1) % channel.capacity
		channel.count++
		channel.cond.Broadcast()
		channel.mu.Unlock()
		return 1
	}
	if C.tsnative_channel_call_int0(C.uintptr_t(hooks.prepare)) != 0 {
		channel.mu.Unlock()
		return -1
	}
	waiter := &nativeF64ChannelWaiter{task: uintptr(task), value: float64(value)}
	channel.sendQueue = append(channel.sendQueue, waiter)
	channel.mu.Unlock()
	return 0
}

//export tsnative_channel_f64_recv_task
func tsnative_channel_f64_recv_task(raw, out unsafe.Pointer) C.int {
	channel := lookupNativeF64Channel(raw)
	if channel == nil || out == nil {
		return -1
	}
	hooks := channelSchedulerHooks()
	task := C.tsnative_channel_call_current(C.uintptr_t(hooks.current))
	if task == nil {
		return -1
	}
	channel.mu.Lock()
	if channel.capacity != 0 && channel.count != 0 {
		*(*C.double)(out) = C.double(channel.buffer[channel.head])
		channel.head = (channel.head + 1) % channel.capacity
		channel.count--
		sender := popChannelWaiter(&channel.sendQueue)
		if sender != nil {
			channel.buffer[channel.tail] = sender.value
			channel.tail = (channel.tail + 1) % channel.capacity
			channel.count++
		}
		channel.cond.Broadcast()
		channel.mu.Unlock()
		finishNativeChannelWaiter(sender)
		return 1
	}
	if sender := popChannelWaiter(&channel.sendQueue); sender != nil {
		*(*C.double)(out) = C.double(sender.value)
		channel.mu.Unlock()
		finishNativeChannelWaiter(sender)
		return 1
	}
	if C.tsnative_channel_call_int0(C.uintptr_t(hooks.prepare)) != 0 {
		channel.mu.Unlock()
		return -1
	}
	waiter := &nativeF64ChannelWaiter{task: uintptr(task), out: uintptr(out)}
	channel.recvQueue = append(channel.recvQueue, waiter)
	channel.mu.Unlock()
	return 0
}

func waitNativeChannelCooperatively(waiter *nativeF64ChannelWaiter) {
	hooks := channelSchedulerHooks()
	for {
		select {
		case <-waiter.done:
			return
		default:
		}
		if hooks.help != 0 && C.tsnative_channel_call_int0(C.uintptr_t(hooks.help)) != 0 {
			runtime.Gosched()
			continue
		}
		select {
		case <-waiter.done:
			return
		case <-time.After(100 * time.Microsecond):
		}
	}
}

//export tsnative_channel_f64_send_cooperative
func tsnative_channel_f64_send_cooperative(raw unsafe.Pointer, value C.double) {
	channel := lookupNativeF64Channel(raw)
	if channel == nil {
		C.abort()
	}
	hooks := channelSchedulerHooks()
	if C.tsnative_channel_call_current(C.uintptr_t(hooks.current)) == nil {
		tsnative_channel_f64_send(raw, value)
		return
	}
	channel.mu.Lock()
	if receiver := popChannelWaiter(&channel.recvQueue); receiver != nil {
		deliverNativeChannelValue(receiver, float64(value))
		channel.mu.Unlock()
		finishNativeChannelWaiter(receiver)
		return
	}
	if channel.capacity != 0 && channel.count < channel.capacity {
		channel.buffer[channel.tail] = float64(value)
		channel.tail = (channel.tail + 1) % channel.capacity
		channel.count++
		channel.cond.Broadcast()
		channel.mu.Unlock()
		return
	}
	waiter := &nativeF64ChannelWaiter{
		task:  uintptr(C.tsnative_channel_call_current(C.uintptr_t(hooks.current))),
		value: float64(value), cooperative: true, done: make(chan struct{}),
	}
	channel.sendQueue = append(channel.sendQueue, waiter)
	channel.mu.Unlock()
	waitNativeChannelCooperatively(waiter)
}

//export tsnative_channel_f64_recv_cooperative
func tsnative_channel_f64_recv_cooperative(raw unsafe.Pointer) C.double {
	channel := lookupNativeF64Channel(raw)
	if channel == nil {
		C.abort()
	}
	hooks := channelSchedulerHooks()
	if C.tsnative_channel_call_current(C.uintptr_t(hooks.current)) == nil {
		return tsnative_channel_f64_recv(raw)
	}
	channel.mu.Lock()
	if channel.capacity != 0 && channel.count != 0 {
		value := channel.buffer[channel.head]
		channel.head = (channel.head + 1) % channel.capacity
		channel.count--
		sender := popChannelWaiter(&channel.sendQueue)
		if sender != nil {
			channel.buffer[channel.tail] = sender.value
			channel.tail = (channel.tail + 1) % channel.capacity
			channel.count++
		}
		channel.cond.Broadcast()
		channel.mu.Unlock()
		finishNativeChannelWaiter(sender)
		return C.double(value)
	}
	if sender := popChannelWaiter(&channel.sendQueue); sender != nil {
		value := sender.value
		channel.mu.Unlock()
		finishNativeChannelWaiter(sender)
		return C.double(value)
	}
	waiter := &nativeF64ChannelWaiter{
		task:        uintptr(C.tsnative_channel_call_current(C.uintptr_t(hooks.current))),
		cooperative: true, done: make(chan struct{}),
	}
	channel.recvQueue = append(channel.recvQueue, waiter)
	channel.mu.Unlock()
	waitNativeChannelCooperatively(waiter)
	return C.double(waiter.value)
}
