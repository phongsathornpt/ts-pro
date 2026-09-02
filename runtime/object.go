package runtime

import "unsafe"

func objectAlloc(size uintptr) unsafe.Pointer {
	return heapAlloc(size)
}

func objectAllocAtomic(size uintptr) unsafe.Pointer {
	return heapAllocAtomic(size)
}

func objectAllocRefs(size uintptr, offsets unsafe.Pointer, count uintptr) unsafe.Pointer {
	return heapAllocRefs(size, offsets, count)
}
