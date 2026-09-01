package main

import (
	"testing"
	"unsafe"
)

func TestNativeF64ChannelHeapFinalizerTracksRoots(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	raw := newNativeF64Channel(2)
	if lookupNativeF64Channel(raw) == nil {
		t.Fatal("channel state missing after allocation")
	}
	tsnative_gc_collect()
	if lookupNativeF64Channel(raw) != nil {
		t.Fatal("unrooted channel state survived collection")
	}

	raw = newNativeF64Channel(2)
	slot := raw
	root := tsnative_gc_root_register(unsafe.Pointer(&slot))
	if root == nil {
		t.Fatal("channel root registration failed")
	}
	tsnative_gc_collect()
	if lookupNativeF64Channel(raw) == nil {
		t.Fatal("rooted channel state was finalized")
	}

	tsnative_gc_root_unregister(root)
	tsnative_gc_collect()
	if lookupNativeF64Channel(raw) != nil {
		t.Fatal("unrooted channel state survived collection after root release")
	}
}

func TestNativeRefChannelBuffersKeepPayloadRooted(t *testing.T) {
	tsnative_heap_shutdown()
	defer tsnative_heap_shutdown()

	channel := newNativeRefChannel(1)
	channelSlot := channel
	channelRoot := tsnative_gc_root_register(unsafe.Pointer(&channelSlot))
	if channelRoot == nil {
		t.Fatal("reference channel root registration failed")
	}
	payload := tsnative_heap_alloc(16)
	if tsnative_channel_ref_try_send(channel, payload) != 1 {
		t.Fatal("reference channel try send failed")
	}
	tsnative_gc_collect()
	if !nativeHeapContains(payload) {
		t.Fatal("reference channel buffered payload was collected")
	}
	got := tsnative_channel_ref_try_recv_or(channel, nil)
	if got != payload {
		t.Fatal("reference channel receive did not preserve payload")
	}
	tsnative_gc_root_unregister(channelRoot)
	tsnative_gc_collect()
	if lookupNativeRefChannel(channel) != nil {
		t.Fatal("unrooted reference channel state survived collection")
	}
	if nativeHeapContains(payload) {
		t.Fatal("reference channel payload survived after receive and channel root release")
	}
}
