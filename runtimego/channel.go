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
	if receiver := popChannelWaiter(&channel.recvQueue); receiver != nil {
		deliverNativeChannelValue(receiver, float64(value))
		channel.mu.Unlock()
		finishNativeChannelWaiter(receiver)
		return 1
	}
	if channel.capacity == 0 {
		if channel.hasValue {
			channel.mu.Unlock()
			return 0
		}
		channel.slot, channel.hasValue = float64(value), true
		channel.cond.Broadcast()
		channel.mu.Unlock()
		return 1
	}
	if channel.count == channel.capacity {
		channel.mu.Unlock()
		return 0
	}
	channel.buffer[channel.tail] = float64(value)
	channel.tail = (channel.tail + 1) % channel.capacity
	channel.count++
	channel.cond.Broadcast()
	channel.mu.Unlock()
	return 1
}

//export tsnative_channel_f64_try_recv
func tsnative_channel_f64_try_recv(raw, out unsafe.Pointer) C.int {
	channel := lookupNativeF64Channel(raw)
	if channel == nil || out == nil {
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
	if channel.capacity == 0 && channel.hasValue {
		*(*C.double)(out) = C.double(channel.slot)
		channel.hasValue = false
		channel.cond.Broadcast()
		channel.mu.Unlock()
		return 1
	}
	channel.mu.Unlock()
	return 0
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
	if receiver := popChannelWaiter(&channel.recvQueue); receiver != nil {
		deliverNativeChannelValue(receiver, float64(value))
		channel.mu.Unlock()
		finishNativeChannelWaiter(receiver)
		return
	}
	if channel.capacity == 0 {
		for channel.hasValue {
			channel.cond.Wait()
		}
		if receiver := popChannelWaiter(&channel.recvQueue); receiver != nil {
			deliverNativeChannelValue(receiver, float64(value))
			channel.mu.Unlock()
			finishNativeChannelWaiter(receiver)
			return
		}
		channel.slot, channel.hasValue = float64(value), true
		channel.cond.Broadcast()
		for channel.hasValue {
			channel.cond.Wait()
		}
		channel.mu.Unlock()
		return
	}
	for channel.count == channel.capacity {
		channel.cond.Wait()
	}
	if receiver := popChannelWaiter(&channel.recvQueue); receiver != nil {
		deliverNativeChannelValue(receiver, float64(value))
		channel.mu.Unlock()
		finishNativeChannelWaiter(receiver)
		return
	}
	channel.buffer[channel.tail] = float64(value)
	channel.tail = (channel.tail + 1) % channel.capacity
	channel.count++
	channel.cond.Broadcast()
	channel.mu.Unlock()
}

//export tsnative_channel_f64_recv
func tsnative_channel_f64_recv(raw unsafe.Pointer) C.double {
	channel := lookupNativeF64Channel(raw)
	if channel == nil {
		C.abort()
	}
	channel.mu.Lock()
	for {
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
		if channel.capacity == 0 && channel.hasValue {
			value := channel.slot
			channel.hasValue = false
			channel.cond.Broadcast()
			channel.mu.Unlock()
			return C.double(value)
		}
		channel.cond.Wait()
	}
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
	channel.sendQueue = append(channel.sendQueue, &nativeF64ChannelWaiter{task: uintptr(task), value: float64(value)})
	channel.cond.Broadcast()
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
	if channel.capacity == 0 && channel.hasValue {
		*(*C.double)(out) = C.double(channel.slot)
		channel.hasValue = false
		channel.cond.Broadcast()
		channel.mu.Unlock()
		return 1
	}
	if C.tsnative_channel_call_int0(C.uintptr_t(hooks.prepare)) != 0 {
		channel.mu.Unlock()
		return -1
	}
	channel.recvQueue = append(channel.recvQueue, &nativeF64ChannelWaiter{task: uintptr(task), out: uintptr(out)})
	channel.cond.Broadcast()
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

type nativeRefRoot struct {
	slot  unsafe.Pointer
	token unsafe.Pointer
}

func newNativeRefRoot(value unsafe.Pointer) *nativeRefRoot {
	slot := C.malloc(C.size_t(unsafe.Sizeof(uintptr(0))))
	if slot == nil {
		nativeAbort("reference channel root slot allocation failed")
	}
	*(*unsafe.Pointer)(slot) = value
	token := tsnative_gc_root_register(slot)
	if token == nil {
		C.free(slot)
		nativeAbort("reference channel root registration failed")
	}
	return &nativeRefRoot{slot: slot, token: token}
}

func (root *nativeRefRoot) get() unsafe.Pointer {
	if root == nil || root.slot == nil {
		return nil
	}
	return *(*unsafe.Pointer)(root.slot)
}

func (root *nativeRefRoot) release() {
	if root == nil {
		return
	}
	if root.token != nil {
		tsnative_gc_root_unregister(root.token)
		root.token = nil
	}
	if root.slot != nil {
		C.free(root.slot)
		root.slot = nil
	}
}

type nativeRefChannelWaiter struct {
	task        uintptr
	value       *nativeRefRoot
	out         uintptr
	cooperative bool
	done        chan struct{}
	once        sync.Once
}

type nativeRefChannel struct {
	mu        sync.Mutex
	cond      *sync.Cond
	capacity  int
	buffer    []*nativeRefRoot
	head      int
	tail      int
	count     int
	slot      *nativeRefRoot
	sendQueue []*nativeRefChannelWaiter
	recvQueue []*nativeRefChannelWaiter
}

var nativeRefChannels = struct {
	sync.Mutex
	byHandle map[uintptr]*nativeRefChannel
}{byHandle: map[uintptr]*nativeRefChannel{}}

func lookupNativeRefChannel(raw unsafe.Pointer) *nativeRefChannel {
	if raw == nil {
		return nil
	}
	nativeRefChannels.Lock()
	channel := nativeRefChannels.byHandle[uintptr(raw)]
	nativeRefChannels.Unlock()
	return channel
}

func releaseNativeRefChannel(channel *nativeRefChannel) {
	if channel == nil {
		return
	}
	channel.mu.Lock()
	if channel.slot != nil {
		channel.slot.release()
		channel.slot = nil
	}
	for i := range channel.buffer {
		if channel.buffer[i] != nil {
			channel.buffer[i].release()
			channel.buffer[i] = nil
		}
	}
	for _, waiter := range channel.sendQueue {
		if waiter != nil && waiter.value != nil {
			waiter.value.release()
			waiter.value = nil
		}
	}
	channel.sendQueue = nil
	channel.recvQueue = nil
	channel.mu.Unlock()
}

func newNativeRefChannel(capacity int) unsafe.Pointer {
	if capacity < 0 {
		C.abort()
	}
	raw := tsnative_heap_alloc(1)
	channel := &nativeRefChannel{capacity: capacity}
	if capacity > 0 {
		channel.buffer = make([]*nativeRefRoot, capacity)
	}
	channel.cond = sync.NewCond(&channel.mu)
	nativeRefChannels.Lock()
	nativeRefChannels.byHandle[uintptr(raw)] = channel
	nativeRefChannels.Unlock()
	registerNativeHeapFinalizer(raw, func() {
		nativeRefChannels.Lock()
		state := nativeRefChannels.byHandle[uintptr(raw)]
		delete(nativeRefChannels.byHandle, uintptr(raw))
		nativeRefChannels.Unlock()
		releaseNativeRefChannel(state)
	})
	return raw
}

func popRefChannelWaiter(queue *[]*nativeRefChannelWaiter) *nativeRefChannelWaiter {
	if len(*queue) == 0 {
		return nil
	}
	waiter := (*queue)[0]
	copy((*queue)[0:], (*queue)[1:])
	*queue = (*queue)[:len(*queue)-1]
	return waiter
}

func finishNativeRefWaiter(waiter *nativeRefChannelWaiter) {
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

func deliverNativeRefRoot(waiter *nativeRefChannelWaiter, root *nativeRefRoot) {
	if waiter == nil || root == nil {
		return
	}
	if waiter.cooperative {
		waiter.value = root
		return
	}
	*(*unsafe.Pointer)(unsafe.Pointer(waiter.out)) = root.get()
	root.release()
}

func enqueueNativeRefRoot(channel *nativeRefChannel, root *nativeRefRoot) {
	channel.buffer[channel.tail] = root
	channel.tail = (channel.tail + 1) % channel.capacity
	channel.count++
}

func dequeueNativeRefRoot(channel *nativeRefChannel) *nativeRefRoot {
	root := channel.buffer[channel.head]
	channel.buffer[channel.head] = nil
	channel.head = (channel.head + 1) % channel.capacity
	channel.count--
	return root
}

//export tsnative_channel_ref_new_checked
func tsnative_channel_ref_new_checked(capacity C.double) unsafe.Pointer {
	value := float64(capacity)
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > float64(math.MaxInt) || math.Trunc(value) != value {
		C.abort()
	}
	return newNativeRefChannel(int(value))
}

//export tsnative_channel_ref_try_send
func tsnative_channel_ref_try_send(raw, value unsafe.Pointer) C.int {
	channel := lookupNativeRefChannel(raw)
	if channel == nil {
		return -1
	}
	root := newNativeRefRoot(value)
	channel.mu.Lock()
	if receiver := popRefChannelWaiter(&channel.recvQueue); receiver != nil {
		deliverNativeRefRoot(receiver, root)
		channel.mu.Unlock()
		finishNativeRefWaiter(receiver)
		return 1
	}
	if channel.capacity == 0 {
		if channel.slot != nil {
			channel.mu.Unlock()
			root.release()
			return 0
		}
		channel.slot = root
		channel.cond.Broadcast()
		channel.mu.Unlock()
		return 1
	}
	if channel.count == channel.capacity {
		channel.mu.Unlock()
		root.release()
		return 0
	}
	enqueueNativeRefRoot(channel, root)
	channel.cond.Broadcast()
	channel.mu.Unlock()
	return 1
}

func nativeRefTryRecv(raw unsafe.Pointer) (unsafe.Pointer, bool) {
	channel := lookupNativeRefChannel(raw)
	if channel == nil {
		return nil, false
	}
	channel.mu.Lock()
	if channel.capacity != 0 && channel.count != 0 {
		root := dequeueNativeRefRoot(channel)
		sender := popRefChannelWaiter(&channel.sendQueue)
		if sender != nil {
			enqueueNativeRefRoot(channel, sender.value)
			sender.value = nil
		}
		channel.cond.Broadcast()
		channel.mu.Unlock()
		finishNativeRefWaiter(sender)
		value := root.get()
		root.release()
		return value, true
	}
	if sender := popRefChannelWaiter(&channel.sendQueue); sender != nil {
		root := sender.value
		sender.value = nil
		channel.mu.Unlock()
		finishNativeRefWaiter(sender)
		value := root.get()
		root.release()
		return value, true
	}
	if channel.capacity == 0 && channel.slot != nil {
		root := channel.slot
		channel.slot = nil
		channel.cond.Broadcast()
		channel.mu.Unlock()
		value := root.get()
		root.release()
		return value, true
	}
	channel.mu.Unlock()
	return nil, false
}

//export tsnative_channel_ref_try_recv_or
func tsnative_channel_ref_try_recv_or(raw, fallback unsafe.Pointer) unsafe.Pointer {
	if value, ok := nativeRefTryRecv(raw); ok {
		return value
	}
	return fallback
}

func nativeRefSend(raw, value unsafe.Pointer) {
	channel := lookupNativeRefChannel(raw)
	if channel == nil {
		C.abort()
	}
	root := newNativeRefRoot(value)
	channel.mu.Lock()
	if receiver := popRefChannelWaiter(&channel.recvQueue); receiver != nil {
		deliverNativeRefRoot(receiver, root)
		channel.mu.Unlock()
		finishNativeRefWaiter(receiver)
		return
	}
	if channel.capacity == 0 {
		for channel.slot != nil {
			channel.cond.Wait()
		}
		if receiver := popRefChannelWaiter(&channel.recvQueue); receiver != nil {
			deliverNativeRefRoot(receiver, root)
			channel.mu.Unlock()
			finishNativeRefWaiter(receiver)
			return
		}
		channel.slot = root
		channel.cond.Broadcast()
		for channel.slot != nil {
			channel.cond.Wait()
		}
		channel.mu.Unlock()
		return
	}
	for channel.count == channel.capacity {
		channel.cond.Wait()
	}
	if receiver := popRefChannelWaiter(&channel.recvQueue); receiver != nil {
		deliverNativeRefRoot(receiver, root)
		channel.mu.Unlock()
		finishNativeRefWaiter(receiver)
		return
	}
	enqueueNativeRefRoot(channel, root)
	channel.cond.Broadcast()
	channel.mu.Unlock()
}

func nativeRefRecv(raw unsafe.Pointer) unsafe.Pointer {
	channel := lookupNativeRefChannel(raw)
	if channel == nil {
		C.abort()
	}
	channel.mu.Lock()
	for {
		if channel.capacity != 0 && channel.count != 0 {
			root := dequeueNativeRefRoot(channel)
			sender := popRefChannelWaiter(&channel.sendQueue)
			if sender != nil {
				enqueueNativeRefRoot(channel, sender.value)
				sender.value = nil
			}
			channel.cond.Broadcast()
			channel.mu.Unlock()
			finishNativeRefWaiter(sender)
			value := root.get()
			root.release()
			return value
		}
		if sender := popRefChannelWaiter(&channel.sendQueue); sender != nil {
			root := sender.value
			sender.value = nil
			channel.mu.Unlock()
			finishNativeRefWaiter(sender)
			value := root.get()
			root.release()
			return value
		}
		if channel.capacity == 0 && channel.slot != nil {
			root := channel.slot
			channel.slot = nil
			channel.cond.Broadcast()
			channel.mu.Unlock()
			value := root.get()
			root.release()
			return value
		}
		channel.cond.Wait()
	}
}

//export tsnative_channel_ref_send_task
func tsnative_channel_ref_send_task(raw, value unsafe.Pointer) C.int {
	channel := lookupNativeRefChannel(raw)
	if channel == nil {
		return -1
	}
	hooks := channelSchedulerHooks()
	task := C.tsnative_channel_call_current(C.uintptr_t(hooks.current))
	if task == nil {
		return -1
	}
	root := newNativeRefRoot(value)
	channel.mu.Lock()
	if receiver := popRefChannelWaiter(&channel.recvQueue); receiver != nil {
		deliverNativeRefRoot(receiver, root)
		channel.mu.Unlock()
		finishNativeRefWaiter(receiver)
		return 1
	}
	if channel.capacity != 0 && channel.count < channel.capacity {
		enqueueNativeRefRoot(channel, root)
		channel.cond.Broadcast()
		channel.mu.Unlock()
		return 1
	}
	if C.tsnative_channel_call_int0(C.uintptr_t(hooks.prepare)) != 0 {
		channel.mu.Unlock()
		root.release()
		return -1
	}
	channel.sendQueue = append(channel.sendQueue, &nativeRefChannelWaiter{task: uintptr(task), value: root})
	channel.cond.Broadcast()
	channel.mu.Unlock()
	return 0
}

//export tsnative_channel_ref_recv_task
func tsnative_channel_ref_recv_task(raw, out unsafe.Pointer) C.int {
	channel := lookupNativeRefChannel(raw)
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
		root := dequeueNativeRefRoot(channel)
		*(*unsafe.Pointer)(out) = root.get()
		root.release()
		sender := popRefChannelWaiter(&channel.sendQueue)
		if sender != nil {
			enqueueNativeRefRoot(channel, sender.value)
			sender.value = nil
		}
		channel.cond.Broadcast()
		channel.mu.Unlock()
		finishNativeRefWaiter(sender)
		return 1
	}
	if sender := popRefChannelWaiter(&channel.sendQueue); sender != nil {
		*(*unsafe.Pointer)(out) = sender.value.get()
		sender.value.release()
		sender.value = nil
		channel.mu.Unlock()
		finishNativeRefWaiter(sender)
		return 1
	}
	if channel.capacity == 0 && channel.slot != nil {
		root := channel.slot
		channel.slot = nil
		*(*unsafe.Pointer)(out) = root.get()
		root.release()
		channel.cond.Broadcast()
		channel.mu.Unlock()
		return 1
	}
	if C.tsnative_channel_call_int0(C.uintptr_t(hooks.prepare)) != 0 {
		channel.mu.Unlock()
		return -1
	}
	channel.recvQueue = append(channel.recvQueue, &nativeRefChannelWaiter{task: uintptr(task), out: uintptr(out)})
	channel.cond.Broadcast()
	channel.mu.Unlock()
	return 0
}

func waitNativeRefChannelCooperatively(waiter *nativeRefChannelWaiter) {
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

//export tsnative_channel_ref_send_cooperative
func tsnative_channel_ref_send_cooperative(raw, value unsafe.Pointer) {
	channel := lookupNativeRefChannel(raw)
	if channel == nil {
		C.abort()
	}
	hooks := channelSchedulerHooks()
	if C.tsnative_channel_call_current(C.uintptr_t(hooks.current)) == nil {
		nativeRefSend(raw, value)
		return
	}
	root := newNativeRefRoot(value)
	channel.mu.Lock()
	if receiver := popRefChannelWaiter(&channel.recvQueue); receiver != nil {
		deliverNativeRefRoot(receiver, root)
		channel.mu.Unlock()
		finishNativeRefWaiter(receiver)
		return
	}
	if channel.capacity != 0 && channel.count < channel.capacity {
		enqueueNativeRefRoot(channel, root)
		channel.cond.Broadcast()
		channel.mu.Unlock()
		return
	}
	waiter := &nativeRefChannelWaiter{task: uintptr(C.tsnative_channel_call_current(C.uintptr_t(hooks.current))), value: root, cooperative: true, done: make(chan struct{})}
	channel.sendQueue = append(channel.sendQueue, waiter)
	channel.mu.Unlock()
	waitNativeRefChannelCooperatively(waiter)
}

//export tsnative_channel_ref_recv_cooperative
func tsnative_channel_ref_recv_cooperative(raw unsafe.Pointer) unsafe.Pointer {
	channel := lookupNativeRefChannel(raw)
	if channel == nil {
		C.abort()
	}
	hooks := channelSchedulerHooks()
	if C.tsnative_channel_call_current(C.uintptr_t(hooks.current)) == nil {
		return nativeRefRecv(raw)
	}
	channel.mu.Lock()
	if channel.capacity != 0 && channel.count != 0 {
		root := dequeueNativeRefRoot(channel)
		sender := popRefChannelWaiter(&channel.sendQueue)
		if sender != nil {
			enqueueNativeRefRoot(channel, sender.value)
			sender.value = nil
		}
		channel.cond.Broadcast()
		channel.mu.Unlock()
		finishNativeRefWaiter(sender)
		value := root.get()
		root.release()
		return value
	}
	if sender := popRefChannelWaiter(&channel.sendQueue); sender != nil {
		value := sender.value.get()
		sender.value.release()
		sender.value = nil
		channel.mu.Unlock()
		finishNativeRefWaiter(sender)
		return value
	}
	if channel.capacity == 0 && channel.slot != nil {
		root := channel.slot
		channel.slot = nil
		channel.cond.Broadcast()
		channel.mu.Unlock()
		value := root.get()
		root.release()
		return value
	}
	waiter := &nativeRefChannelWaiter{task: uintptr(C.tsnative_channel_call_current(C.uintptr_t(hooks.current))), cooperative: true, done: make(chan struct{})}
	channel.recvQueue = append(channel.recvQueue, waiter)
	channel.mu.Unlock()
	waitNativeRefChannelCooperatively(waiter)
	value := waiter.value.get()
	waiter.value.release()
	waiter.value = nil
	return value
}

type nativeBoolChannelWaiter struct {
	task        uintptr
	value       uint8
	out         uintptr
	cooperative bool
	done        chan struct{}
	once        sync.Once
}

type nativeBoolChannel struct {
	mu        sync.Mutex
	cond      *sync.Cond
	capacity  int
	buffer    []uint8
	head      int
	tail      int
	count     int
	slot      uint8
	hasValue  bool
	sendQueue []*nativeBoolChannelWaiter
	recvQueue []*nativeBoolChannelWaiter
}

var nativeBoolChannels = struct {
	sync.Mutex
	byHandle map[uintptr]*nativeBoolChannel
}{byHandle: map[uintptr]*nativeBoolChannel{}}

func lookupNativeBoolChannel(raw unsafe.Pointer) *nativeBoolChannel {
	if raw == nil {
		return nil
	}
	nativeBoolChannels.Lock()
	channel := nativeBoolChannels.byHandle[uintptr(raw)]
	nativeBoolChannels.Unlock()
	return channel
}

func newNativeBoolChannel(capacity int) unsafe.Pointer {
	if capacity < 0 {
		C.abort()
	}
	raw := tsnative_heap_alloc(1)
	channel := &nativeBoolChannel{capacity: capacity}
	if capacity > 0 {
		channel.buffer = make([]uint8, capacity)
	}
	channel.cond = sync.NewCond(&channel.mu)
	nativeBoolChannels.Lock()
	nativeBoolChannels.byHandle[uintptr(raw)] = channel
	nativeBoolChannels.Unlock()
	registerNativeHeapFinalizer(raw, func() {
		nativeBoolChannels.Lock()
		delete(nativeBoolChannels.byHandle, uintptr(raw))
		nativeBoolChannels.Unlock()
	})
	return raw
}

func popBoolChannelWaiter(queue *[]*nativeBoolChannelWaiter) *nativeBoolChannelWaiter {
	if len(*queue) == 0 {
		return nil
	}
	waiter := (*queue)[0]
	copy((*queue)[0:], (*queue)[1:])
	*queue = (*queue)[:len(*queue)-1]
	return waiter
}

func deliverNativeBoolValue(waiter *nativeBoolChannelWaiter, value uint8) {
	if waiter.cooperative {
		waiter.value = value
		return
	}
	*(*C.uint8_t)(unsafe.Pointer(waiter.out)) = C.uint8_t(value)
}

func finishNativeBoolWaiter(waiter *nativeBoolChannelWaiter) {
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

//export tsnative_channel_bool_new_checked
func tsnative_channel_bool_new_checked(capacity C.double) unsafe.Pointer {
	value := float64(capacity)
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > float64(math.MaxInt) || math.Trunc(value) != value {
		C.abort()
	}
	return newNativeBoolChannel(int(value))
}

//export tsnative_channel_bool_try_send
func tsnative_channel_bool_try_send(raw unsafe.Pointer, value C.uint8_t) C.int {
	channel := lookupNativeBoolChannel(raw)
	if channel == nil {
		return -1
	}
	v := uint8(value)
	channel.mu.Lock()
	if receiver := popBoolChannelWaiter(&channel.recvQueue); receiver != nil {
		deliverNativeBoolValue(receiver, v)
		channel.mu.Unlock()
		finishNativeBoolWaiter(receiver)
		return 1
	}
	if channel.capacity == 0 {
		if channel.hasValue {
			channel.mu.Unlock()
			return 0
		}
		channel.slot, channel.hasValue = v, true
		channel.cond.Broadcast()
		channel.mu.Unlock()
		return 1
	}
	if channel.count == channel.capacity {
		channel.mu.Unlock()
		return 0
	}
	channel.buffer[channel.tail] = v
	channel.tail = (channel.tail + 1) % channel.capacity
	channel.count++
	channel.cond.Broadcast()
	channel.mu.Unlock()
	return 1
}

//export tsnative_channel_bool_try_recv_or
func tsnative_channel_bool_try_recv_or(raw unsafe.Pointer, fallback C.uint8_t) C.uint8_t {
	channel := lookupNativeBoolChannel(raw)
	if channel == nil {
		return fallback
	}
	channel.mu.Lock()
	if channel.capacity != 0 && channel.count != 0 {
		value := channel.buffer[channel.head]
		channel.head = (channel.head + 1) % channel.capacity
		channel.count--
		sender := popBoolChannelWaiter(&channel.sendQueue)
		if sender != nil {
			channel.buffer[channel.tail] = sender.value
			channel.tail = (channel.tail + 1) % channel.capacity
			channel.count++
		}
		channel.cond.Broadcast()
		channel.mu.Unlock()
		finishNativeBoolWaiter(sender)
		return C.uint8_t(value)
	}
	if sender := popBoolChannelWaiter(&channel.sendQueue); sender != nil {
		value := sender.value
		channel.mu.Unlock()
		finishNativeBoolWaiter(sender)
		return C.uint8_t(value)
	}
	if channel.capacity == 0 && channel.hasValue {
		value := channel.slot
		channel.hasValue = false
		channel.cond.Broadcast()
		channel.mu.Unlock()
		return C.uint8_t(value)
	}
	channel.mu.Unlock()
	return fallback
}

//export tsnative_channel_bool_send_task
func tsnative_channel_bool_send_task(raw unsafe.Pointer, value C.uint8_t) C.int {
	channel := lookupNativeBoolChannel(raw)
	if channel == nil {
		return -1
	}
	hooks := channelSchedulerHooks()
	task := C.tsnative_channel_call_current(C.uintptr_t(hooks.current))
	if task == nil {
		return -1
	}
	v := uint8(value)
	channel.mu.Lock()
	if receiver := popBoolChannelWaiter(&channel.recvQueue); receiver != nil {
		deliverNativeBoolValue(receiver, v)
		channel.mu.Unlock()
		finishNativeBoolWaiter(receiver)
		return 1
	}
	if channel.capacity != 0 && channel.count < channel.capacity {
		channel.buffer[channel.tail] = v
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
	channel.sendQueue = append(channel.sendQueue, &nativeBoolChannelWaiter{task: uintptr(task), value: v})
	channel.cond.Broadcast()
	channel.mu.Unlock()
	return 0
}

//export tsnative_channel_bool_recv_task
func tsnative_channel_bool_recv_task(raw, out unsafe.Pointer) C.int {
	channel := lookupNativeBoolChannel(raw)
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
		*(*C.uint8_t)(out) = C.uint8_t(channel.buffer[channel.head])
		channel.head = (channel.head + 1) % channel.capacity
		channel.count--
		sender := popBoolChannelWaiter(&channel.sendQueue)
		if sender != nil {
			channel.buffer[channel.tail] = sender.value
			channel.tail = (channel.tail + 1) % channel.capacity
			channel.count++
		}
		channel.cond.Broadcast()
		channel.mu.Unlock()
		finishNativeBoolWaiter(sender)
		return 1
	}
	if sender := popBoolChannelWaiter(&channel.sendQueue); sender != nil {
		*(*C.uint8_t)(out) = C.uint8_t(sender.value)
		channel.mu.Unlock()
		finishNativeBoolWaiter(sender)
		return 1
	}
	if channel.capacity == 0 && channel.hasValue {
		*(*C.uint8_t)(out) = C.uint8_t(channel.slot)
		channel.hasValue = false
		channel.cond.Broadcast()
		channel.mu.Unlock()
		return 1
	}
	if C.tsnative_channel_call_int0(C.uintptr_t(hooks.prepare)) != 0 {
		channel.mu.Unlock()
		return -1
	}
	channel.recvQueue = append(channel.recvQueue, &nativeBoolChannelWaiter{task: uintptr(task), out: uintptr(out)})
	channel.cond.Broadcast()
	channel.mu.Unlock()
	return 0
}

func waitNativeBoolChannelCooperatively(waiter *nativeBoolChannelWaiter) {
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

//export tsnative_channel_bool_send_cooperative
func tsnative_channel_bool_send_cooperative(raw unsafe.Pointer, value C.uint8_t) {
	channel := lookupNativeBoolChannel(raw)
	if channel == nil {
		C.abort()
	}
	hooks := channelSchedulerHooks()
	if C.tsnative_channel_call_current(C.uintptr_t(hooks.current)) == nil {
		v := uint8(value)
		channel.mu.Lock()
		if receiver := popBoolChannelWaiter(&channel.recvQueue); receiver != nil {
			deliverNativeBoolValue(receiver, v)
			channel.mu.Unlock()
			finishNativeBoolWaiter(receiver)
			return
		}
		if channel.capacity == 0 {
			for channel.hasValue {
				channel.cond.Wait()
			}
			if receiver := popBoolChannelWaiter(&channel.recvQueue); receiver != nil {
				deliverNativeBoolValue(receiver, v)
				channel.mu.Unlock()
				finishNativeBoolWaiter(receiver)
				return
			}
			channel.slot, channel.hasValue = v, true
			channel.cond.Broadcast()
			for channel.hasValue {
				channel.cond.Wait()
			}
			channel.mu.Unlock()
			return
		}
		for channel.count == channel.capacity {
			channel.cond.Wait()
		}
		if receiver := popBoolChannelWaiter(&channel.recvQueue); receiver != nil {
			deliverNativeBoolValue(receiver, v)
			channel.mu.Unlock()
			finishNativeBoolWaiter(receiver)
			return
		}
		channel.buffer[channel.tail] = v
		channel.tail = (channel.tail + 1) % channel.capacity
		channel.count++
		channel.cond.Broadcast()
		channel.mu.Unlock()
		return
	}
	channel.mu.Lock()
	if receiver := popBoolChannelWaiter(&channel.recvQueue); receiver != nil {
		deliverNativeBoolValue(receiver, uint8(value))
		channel.mu.Unlock()
		finishNativeBoolWaiter(receiver)
		return
	}
	if channel.capacity != 0 && channel.count < channel.capacity {
		channel.buffer[channel.tail] = uint8(value)
		channel.tail = (channel.tail + 1) % channel.capacity
		channel.count++
		channel.cond.Broadcast()
		channel.mu.Unlock()
		return
	}
	waiter := &nativeBoolChannelWaiter{value: uint8(value), cooperative: true, done: make(chan struct{})}
	channel.sendQueue = append(channel.sendQueue, waiter)
	channel.mu.Unlock()
	waitNativeBoolChannelCooperatively(waiter)
}

//export tsnative_channel_bool_recv_cooperative
func tsnative_channel_bool_recv_cooperative(raw unsafe.Pointer) C.uint8_t {
	channel := lookupNativeBoolChannel(raw)
	if channel == nil {
		C.abort()
	}
	hooks := channelSchedulerHooks()
	if C.tsnative_channel_call_current(C.uintptr_t(hooks.current)) == nil {
		channel.mu.Lock()
		for {
			if channel.capacity != 0 && channel.count != 0 {
				value := channel.buffer[channel.head]
				channel.head = (channel.head + 1) % channel.capacity
				channel.count--
				sender := popBoolChannelWaiter(&channel.sendQueue)
				if sender != nil {
					channel.buffer[channel.tail] = sender.value
					channel.tail = (channel.tail + 1) % channel.capacity
					channel.count++
				}
				channel.cond.Broadcast()
				channel.mu.Unlock()
				finishNativeBoolWaiter(sender)
				return C.uint8_t(value)
			}
			if sender := popBoolChannelWaiter(&channel.sendQueue); sender != nil {
				value := sender.value
				channel.mu.Unlock()
				finishNativeBoolWaiter(sender)
				return C.uint8_t(value)
			}
			if channel.capacity == 0 && channel.hasValue {
				value := channel.slot
				channel.hasValue = false
				channel.cond.Broadcast()
				channel.mu.Unlock()
				return C.uint8_t(value)
			}
			channel.cond.Wait()
		}
	}
	channel.mu.Lock()
	if channel.capacity != 0 && channel.count != 0 {
		value := channel.buffer[channel.head]
		channel.head = (channel.head + 1) % channel.capacity
		channel.count--
		sender := popBoolChannelWaiter(&channel.sendQueue)
		if sender != nil {
			channel.buffer[channel.tail] = sender.value
			channel.tail = (channel.tail + 1) % channel.capacity
			channel.count++
		}
		channel.cond.Broadcast()
		channel.mu.Unlock()
		finishNativeBoolWaiter(sender)
		return C.uint8_t(value)
	}
	if sender := popBoolChannelWaiter(&channel.sendQueue); sender != nil {
		value := sender.value
		channel.mu.Unlock()
		finishNativeBoolWaiter(sender)
		return C.uint8_t(value)
	}
	waiter := &nativeBoolChannelWaiter{cooperative: true, done: make(chan struct{})}
	channel.recvQueue = append(channel.recvQueue, waiter)
	channel.mu.Unlock()
	waitNativeBoolChannelCooperatively(waiter)
	return C.uint8_t(waiter.value)
}
