package main

/*
#include <stddef.h>
*/
import "C"

import "unsafe"

//export tsnative_object_alloc
func tsnative_object_alloc(size C.size_t) unsafe.Pointer {
	return tsnative_heap_alloc(uintptr(size))
}

//export tsnative_object_alloc_atomic
func tsnative_object_alloc_atomic(size C.size_t) unsafe.Pointer {
	return tsnative_heap_alloc_atomic(uintptr(size))
}

//export tsnative_object_alloc_refs
func tsnative_object_alloc_refs(size C.size_t, offsets unsafe.Pointer, count C.size_t) unsafe.Pointer {
	return tsnative_heap_alloc_refs(uintptr(size), offsets, uintptr(count))
}
