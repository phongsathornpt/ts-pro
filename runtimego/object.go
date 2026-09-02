package runtimego

import "unsafe"

func tsnative_object_alloc(size uintptr) unsafe.Pointer {
	return tsnative_heap_alloc(size)
}

func tsnative_object_alloc_atomic(size uintptr) unsafe.Pointer {
	return tsnative_heap_alloc_atomic(size)
}

func tsnative_object_alloc_refs(size uintptr, offsets unsafe.Pointer, count uintptr) unsafe.Pointer {
	return tsnative_heap_alloc_refs(size, offsets, count)
}
