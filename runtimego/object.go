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
