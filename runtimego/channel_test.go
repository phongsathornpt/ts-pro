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
