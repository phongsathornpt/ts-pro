package runtime

import (
	"testing"
	"unsafe"
)

func TestNativeF64ChannelHeapFinalizerTracksRoots(t *testing.T) {
	heapShutdown()
	defer heapShutdown()

	raw := newNativeF64Channel(2)
	if lookupNativeF64Channel(raw) == nil {
		t.Fatal("channel state missing after allocation")
	}
	gcCollect()
	if lookupNativeF64Channel(raw) != nil {
		t.Fatal("unrooted channel state survived collection")
	}

	raw = newNativeF64Channel(2)
	slot := raw
	root := gcRootRegister(unsafe.Pointer(&slot))
	if root == nil {
		t.Fatal("channel root registration failed")
	}
	gcCollect()
	if lookupNativeF64Channel(raw) == nil {
		t.Fatal("rooted channel state was finalized")
	}

	gcRootUnregister(root)
	gcCollect()
	if lookupNativeF64Channel(raw) != nil {
		t.Fatal("unrooted channel state survived collection after root release")
	}
}

func TestNativeRefChannelBuffersKeepPayloadRooted(t *testing.T) {
	heapShutdown()
	defer heapShutdown()

	channel := newNativeRefChannel(1)
	channelSlot := channel
	channelRoot := gcRootRegister(unsafe.Pointer(&channelSlot))
	if channelRoot == nil {
		t.Fatal("reference channel root registration failed")
	}
	payload := heapAlloc(16)
	if channelRefTrySend(channel, payload) != 1 {
		t.Fatal("reference channel try send failed")
	}
	gcCollect()
	if !nativeHeapContains(payload) {
		t.Fatal("reference channel buffered payload was collected")
	}
	got := channelRefTryRecvOr(channel, nil)
	if got != payload {
		t.Fatal("reference channel receive did not preserve payload")
	}
	gcRootUnregister(channelRoot)
	gcCollect()
	if lookupNativeRefChannel(channel) != nil {
		t.Fatal("unrooted reference channel state survived collection")
	}
	if nativeHeapContains(payload) {
		t.Fatal("reference channel payload survived after receive and channel root release")
	}
}

func TestNativeBoolChannelTryOperations(t *testing.T) {
	heapShutdown()
	defer heapShutdown()
	raw := newNativeBoolChannel(1)
	if channelBoolTrySend(raw, 1) != 1 {
		t.Fatal("boolean channel try send failed")
	}
	if got := channelBoolTryRecvOr(raw, 0); got != 1 {
		t.Fatalf("boolean channel recv = %d", got)
	}
	if got := channelBoolTryRecvOr(raw, 0); got != 0 {
		t.Fatalf("boolean channel fallback = %d", got)
	}
	gcCollect()
	if lookupNativeBoolChannel(raw) != nil {
		t.Fatal("unrooted boolean channel survived collection")
	}
}
