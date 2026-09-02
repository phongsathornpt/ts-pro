package runtimego

import (
	"math"
	"runtime"
	"sync"
	"time"
	"unsafe"
)

type nativeF64ChannelWaiter struct {
	task        uintptr
	value       float64
	out         unsafe.Pointer
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

var nativeChannels = struct {
	sync.Mutex
	byHandle map[uintptr]*nativeF64Channel
}{byHandle: map[uintptr]*nativeF64Channel{}}

func channelBindScheduler(current, prepare, cancel, wake, help uintptr) {}

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
		nativeAbort("negative channel capacity")
	}
	raw := heapAlloc(1)
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
	*(*float64)(waiter.out) = value
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
		_ = schedulerWakeTask(waiter.task)
	})
}

func channelF64New(capacity uintptr) unsafe.Pointer {
	if uint64(capacity) > uint64(math.MaxInt) {
		nativeAbort("channel capacity overflow")
	}
	return newNativeF64Channel(int(capacity))
}

func channelF64NewChecked(capacity float64) unsafe.Pointer {
	value := capacity
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > float64(math.MaxInt) || math.Trunc(value) != value {
		nativeAbort("invalid channel capacity")
	}
	return newNativeF64Channel(int(value))
}

func channelF64TrySend(raw unsafe.Pointer, value float64) int32 {
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

func channelF64TryRecv(raw, out unsafe.Pointer) int32 {
	channel := lookupNativeF64Channel(raw)
	if channel == nil || out == nil {
		return -1
	}
	channel.mu.Lock()
	if channel.capacity != 0 && channel.count != 0 {
		*(*float64)(out) = channel.buffer[channel.head]
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
		*(*float64)(out) = sender.value
		channel.mu.Unlock()
		finishNativeChannelWaiter(sender)
		return 1
	}
	if channel.capacity == 0 && channel.hasValue {
		*(*float64)(out) = channel.slot
		channel.hasValue = false
		channel.cond.Broadcast()
		channel.mu.Unlock()
		return 1
	}
	channel.mu.Unlock()
	return 0
}

func channelF64TryRecvOr(raw unsafe.Pointer, fallback float64) float64 {
	value := fallback
	if channelF64TryRecv(raw, unsafe.Pointer(&value)) == 1 {
		return value
	}
	return fallback
}

func channelF64Send(raw unsafe.Pointer, value float64) {
	channel := lookupNativeF64Channel(raw)
	if channel == nil {
		nativeAbort("send to nil f64 channel")
	}
	channel.mu.Lock()
	if receiver := popChannelWaiter(&channel.recvQueue); receiver != nil {
		deliverNativeChannelValue(receiver, value)
		channel.mu.Unlock()
		finishNativeChannelWaiter(receiver)
		return
	}
	if channel.capacity == 0 {
		for channel.hasValue {
			channel.cond.Wait()
		}
		if receiver := popChannelWaiter(&channel.recvQueue); receiver != nil {
			deliverNativeChannelValue(receiver, value)
			channel.mu.Unlock()
			finishNativeChannelWaiter(receiver)
			return
		}
		channel.slot, channel.hasValue = value, true
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
		deliverNativeChannelValue(receiver, value)
		channel.mu.Unlock()
		finishNativeChannelWaiter(receiver)
		return
	}
	channel.buffer[channel.tail] = value
	channel.tail = (channel.tail + 1) % channel.capacity
	channel.count++
	channel.cond.Broadcast()
	channel.mu.Unlock()
}

func channelF64Recv(raw unsafe.Pointer) float64 {
	channel := lookupNativeF64Channel(raw)
	if channel == nil {
		nativeAbort("recv from nil f64 channel")
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
			return value
		}
		if sender := popChannelWaiter(&channel.sendQueue); sender != nil {
			value := sender.value
			channel.mu.Unlock()
			finishNativeChannelWaiter(sender)
			return value
		}
		if channel.capacity == 0 && channel.hasValue {
			value := channel.slot
			channel.hasValue = false
			channel.cond.Broadcast()
			channel.mu.Unlock()
			return value
		}
		channel.cond.Wait()
	}
}

func channelF64SendTask(raw unsafe.Pointer, value float64) int32 {
	channel := lookupNativeF64Channel(raw)
	if channel == nil {
		return -1
	}
	task := schedulerCurrentTaskPtr()
	if task == 0 {
		return -1
	}
	channel.mu.Lock()
	if receiver := popChannelWaiter(&channel.recvQueue); receiver != nil {
		deliverNativeChannelValue(receiver, value)
		channel.mu.Unlock()
		finishNativeChannelWaiter(receiver)
		return 1
	}
	if channel.capacity != 0 && channel.count < channel.capacity {
		channel.buffer[channel.tail] = value
		channel.tail = (channel.tail + 1) % channel.capacity
		channel.count++
		channel.cond.Broadcast()
		channel.mu.Unlock()
		return 1
	}
	if schedulerPreparePark() != 0 {
		channel.mu.Unlock()
		return -1
	}
	channel.sendQueue = append(channel.sendQueue, &nativeF64ChannelWaiter{task: task, value: value})
	channel.cond.Broadcast()
	channel.mu.Unlock()
	return 0
}

func channelF64RecvTask(raw, out unsafe.Pointer) int32 {
	channel := lookupNativeF64Channel(raw)
	if channel == nil || out == nil {
		return -1
	}
	task := schedulerCurrentTaskPtr()
	if task == 0 {
		return -1
	}
	channel.mu.Lock()
	if channel.capacity != 0 && channel.count != 0 {
		*(*float64)(out) = channel.buffer[channel.head]
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
		*(*float64)(out) = sender.value
		channel.mu.Unlock()
		finishNativeChannelWaiter(sender)
		return 1
	}
	if channel.capacity == 0 && channel.hasValue {
		*(*float64)(out) = channel.slot
		channel.hasValue = false
		channel.cond.Broadcast()
		channel.mu.Unlock()
		return 1
	}
	if schedulerPreparePark() != 0 {
		channel.mu.Unlock()
		return -1
	}
	channel.recvQueue = append(channel.recvQueue, &nativeF64ChannelWaiter{task: task, out: out})
	channel.cond.Broadcast()
	channel.mu.Unlock()
	return 0
}

func waitNativeChannelCooperatively(waiter *nativeF64ChannelWaiter) {
	for {
		select {
		case <-waiter.done:
			return
		default:
		}
		if schedulerHelpOnce() != 0 {
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

func channelF64SendCooperative(raw unsafe.Pointer, value float64) {
	channel := lookupNativeF64Channel(raw)
	if channel == nil {
		nativeAbort("send cooperative to nil f64 channel")
	}
	currentTask := schedulerCurrentTaskPtr()
	if currentTask == 0 {
		channelF64Send(raw, value)
		return
	}
	channel.mu.Lock()
	if receiver := popChannelWaiter(&channel.recvQueue); receiver != nil {
		deliverNativeChannelValue(receiver, value)
		channel.mu.Unlock()
		finishNativeChannelWaiter(receiver)
		return
	}
	if channel.capacity != 0 && channel.count < channel.capacity {
		channel.buffer[channel.tail] = value
		channel.tail = (channel.tail + 1) % channel.capacity
		channel.count++
		channel.cond.Broadcast()
		channel.mu.Unlock()
		return
	}
	waiter := &nativeF64ChannelWaiter{
		task:  currentTask,
		value: value, cooperative: true, done: make(chan struct{}),
	}
	channel.sendQueue = append(channel.sendQueue, waiter)
	channel.mu.Unlock()
	waitNativeChannelCooperatively(waiter)
}

func channelF64RecvCooperative(raw unsafe.Pointer) float64 {
	channel := lookupNativeF64Channel(raw)
	if channel == nil {
		nativeAbort("recv cooperative from nil f64 channel")
	}
	currentTask := schedulerCurrentTaskPtr()
	if currentTask == 0 {
		return channelF64Recv(raw)
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
		return value
	}
	if sender := popChannelWaiter(&channel.sendQueue); sender != nil {
		value := sender.value
		channel.mu.Unlock()
		finishNativeChannelWaiter(sender)
		return value
	}
	waiter := &nativeF64ChannelWaiter{
		task:        currentTask,
		cooperative: true, done: make(chan struct{}),
	}
	channel.recvQueue = append(channel.recvQueue, waiter)
	channel.mu.Unlock()
	waitNativeChannelCooperatively(waiter)
	return waiter.value
}

type nativeRefRoot struct {
	slot  unsafe.Pointer
	token unsafe.Pointer
}

func newNativeRefRoot(value unsafe.Pointer) *nativeRefRoot {
	slot := unsafe.Pointer(new(unsafe.Pointer))
	*(*unsafe.Pointer)(slot) = value
	token := gcRootRegister(slot)
	if token == nil {
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
		gcRootUnregister(root.token)
		root.token = nil
	}
	root.slot = nil
}

type nativeRefChannelWaiter struct {
	task        uintptr
	value       *nativeRefRoot
	out         unsafe.Pointer
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
		nativeAbort("negative ref channel capacity")
	}
	raw := heapAlloc(1)
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
		_ = schedulerWakeTask(waiter.task)
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
	nativeGCStoreRefSlot(waiter.out, root.get())
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

func channelRefNewChecked(capacity float64) unsafe.Pointer {
	value := capacity
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > float64(math.MaxInt) || math.Trunc(value) != value {
		nativeAbort("invalid ref channel capacity")
	}
	return newNativeRefChannel(int(value))
}

func channelRefTrySend(raw, value unsafe.Pointer) int32 {
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

func channelRefTryRecvOr(raw, fallback unsafe.Pointer) unsafe.Pointer {
	if value, ok := nativeRefTryRecv(raw); ok {
		return value
	}
	return fallback
}

func nativeRefSend(raw, value unsafe.Pointer) {
	channel := lookupNativeRefChannel(raw)
	if channel == nil {
		nativeAbort("send to nil ref channel")
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
		nativeAbort("recv from nil ref channel")
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

func channelRefSendTask(raw, value unsafe.Pointer) int32 {
	channel := lookupNativeRefChannel(raw)
	if channel == nil {
		return -1
	}
	task := schedulerCurrentTaskPtr()
	if task == 0 {
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
	if schedulerPreparePark() != 0 {
		channel.mu.Unlock()
		root.release()
		return -1
	}
	channel.sendQueue = append(channel.sendQueue, &nativeRefChannelWaiter{task: task, value: root})
	channel.cond.Broadcast()
	channel.mu.Unlock()
	return 0
}

func channelRefRecvTask(raw, out unsafe.Pointer) int32 {
	channel := lookupNativeRefChannel(raw)
	if channel == nil || out == nil {
		return -1
	}
	task := schedulerCurrentTaskPtr()
	if task == 0 {
		return -1
	}
	channel.mu.Lock()
	if channel.capacity != 0 && channel.count != 0 {
		root := dequeueNativeRefRoot(channel)
		nativeGCStoreRefSlot(out, root.get())
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
		nativeGCStoreRefSlot(out, sender.value.get())
		sender.value.release()
		sender.value = nil
		channel.mu.Unlock()
		finishNativeRefWaiter(sender)
		return 1
	}
	if channel.capacity == 0 && channel.slot != nil {
		root := channel.slot
		channel.slot = nil
		nativeGCStoreRefSlot(out, root.get())
		root.release()
		channel.cond.Broadcast()
		channel.mu.Unlock()
		return 1
	}
	if schedulerPreparePark() != 0 {
		channel.mu.Unlock()
		return -1
	}
	channel.recvQueue = append(channel.recvQueue, &nativeRefChannelWaiter{task: task, out: out})
	channel.cond.Broadcast()
	channel.mu.Unlock()
	return 0
}

func waitNativeRefChannelCooperatively(waiter *nativeRefChannelWaiter) {
	for {
		select {
		case <-waiter.done:
			return
		default:
		}
		if schedulerHelpOnce() != 0 {
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

func channelRefSendCooperative(raw, value unsafe.Pointer) {
	channel := lookupNativeRefChannel(raw)
	if channel == nil {
		nativeAbort("send cooperative to nil ref channel")
	}
	currentTask := schedulerCurrentTaskPtr()
	if currentTask == 0 {
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
	waiter := &nativeRefChannelWaiter{task: currentTask, value: root, cooperative: true, done: make(chan struct{})}
	channel.sendQueue = append(channel.sendQueue, waiter)
	channel.mu.Unlock()
	waitNativeRefChannelCooperatively(waiter)
}

func channelRefRecvCooperative(raw unsafe.Pointer) unsafe.Pointer {
	channel := lookupNativeRefChannel(raw)
	if channel == nil {
		nativeAbort("recv cooperative from nil ref channel")
	}
	currentTask := schedulerCurrentTaskPtr()
	if currentTask == 0 {
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
	waiter := &nativeRefChannelWaiter{task: currentTask, cooperative: true, done: make(chan struct{})}
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
	out         unsafe.Pointer
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
		nativeAbort("negative bool channel capacity")
	}
	raw := heapAlloc(1)
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
	*(*uint8)(waiter.out) = value
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
		_ = schedulerWakeTask(waiter.task)
	})
}

func channelBoolNewChecked(capacity float64) unsafe.Pointer {
	value := capacity
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > float64(math.MaxInt) || math.Trunc(value) != value {
		nativeAbort("invalid bool channel capacity")
	}
	return newNativeBoolChannel(int(value))
}

func channelBoolTrySend(raw unsafe.Pointer, value uint8) int32 {
	channel := lookupNativeBoolChannel(raw)
	if channel == nil {
		return -1
	}
	v := value
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

func channelBoolTryRecvOr(raw unsafe.Pointer, fallback uint8) uint8 {
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
		return value
	}
	if sender := popBoolChannelWaiter(&channel.sendQueue); sender != nil {
		value := sender.value
		channel.mu.Unlock()
		finishNativeBoolWaiter(sender)
		return value
	}
	if channel.capacity == 0 && channel.hasValue {
		value := channel.slot
		channel.hasValue = false
		channel.cond.Broadcast()
		channel.mu.Unlock()
		return value
	}
	channel.mu.Unlock()
	return fallback
}

func channelBoolSendTask(raw unsafe.Pointer, value uint8) int32 {
	channel := lookupNativeBoolChannel(raw)
	if channel == nil {
		return -1
	}
	task := schedulerCurrentTaskPtr()
	if task == 0 {
		return -1
	}
	v := value
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
	if schedulerPreparePark() != 0 {
		channel.mu.Unlock()
		return -1
	}
	channel.sendQueue = append(channel.sendQueue, &nativeBoolChannelWaiter{task: task, value: v})
	channel.cond.Broadcast()
	channel.mu.Unlock()
	return 0
}

func channelBoolRecvTask(raw, out unsafe.Pointer) int32 {
	channel := lookupNativeBoolChannel(raw)
	if channel == nil || out == nil {
		return -1
	}
	task := schedulerCurrentTaskPtr()
	if task == 0 {
		return -1
	}
	channel.mu.Lock()
	if channel.capacity != 0 && channel.count != 0 {
		*(*uint8)(out) = channel.buffer[channel.head]
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
		*(*uint8)(out) = sender.value
		channel.mu.Unlock()
		finishNativeBoolWaiter(sender)
		return 1
	}
	if channel.capacity == 0 && channel.hasValue {
		*(*uint8)(out) = channel.slot
		channel.hasValue = false
		channel.cond.Broadcast()
		channel.mu.Unlock()
		return 1
	}
	if schedulerPreparePark() != 0 {
		channel.mu.Unlock()
		return -1
	}
	channel.recvQueue = append(channel.recvQueue, &nativeBoolChannelWaiter{task: task, out: out})
	channel.cond.Broadcast()
	channel.mu.Unlock()
	return 0
}

func waitNativeBoolChannelCooperatively(waiter *nativeBoolChannelWaiter) {
	for {
		select {
		case <-waiter.done:
			return
		default:
		}
		if schedulerHelpOnce() != 0 {
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

func channelBoolSendCooperative(raw unsafe.Pointer, value uint8) {
	channel := lookupNativeBoolChannel(raw)
	if channel == nil {
		nativeAbort("send cooperative to nil bool channel")
	}
	currentTask := schedulerCurrentTaskPtr()
	if currentTask == 0 {
		v := value
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
		deliverNativeBoolValue(receiver, value)
		channel.mu.Unlock()
		finishNativeBoolWaiter(receiver)
		return
	}
	if channel.capacity != 0 && channel.count < channel.capacity {
		channel.buffer[channel.tail] = value
		channel.tail = (channel.tail + 1) % channel.capacity
		channel.count++
		channel.cond.Broadcast()
		channel.mu.Unlock()
		return
	}
	waiter := &nativeBoolChannelWaiter{value: value, cooperative: true, done: make(chan struct{})}
	channel.sendQueue = append(channel.sendQueue, waiter)
	channel.mu.Unlock()
	waitNativeBoolChannelCooperatively(waiter)
}

func channelBoolRecvCooperative(raw unsafe.Pointer) uint8 {
	channel := lookupNativeBoolChannel(raw)
	if channel == nil {
		nativeAbort("recv cooperative from nil bool channel")
	}
	currentTask := schedulerCurrentTaskPtr()
	if currentTask == 0 {
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
				return value
			}
			if sender := popBoolChannelWaiter(&channel.sendQueue); sender != nil {
				value := sender.value
				channel.mu.Unlock()
				finishNativeBoolWaiter(sender)
				return value
			}
			if channel.capacity == 0 && channel.hasValue {
				value := channel.slot
				channel.hasValue = false
				channel.cond.Broadcast()
				channel.mu.Unlock()
				return value
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
		return value
	}
	if sender := popBoolChannelWaiter(&channel.sendQueue); sender != nil {
		value := sender.value
		channel.mu.Unlock()
		finishNativeBoolWaiter(sender)
		return value
	}
	waiter := &nativeBoolChannelWaiter{cooperative: true, done: make(chan struct{})}
	channel.recvQueue = append(channel.recvQueue, waiter)
	channel.mu.Unlock()
	waitNativeBoolChannelCooperatively(waiter)
	return waiter.value
}
