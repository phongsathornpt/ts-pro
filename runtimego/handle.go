package main

import (
	"os"
	"sync"
	"unsafe"
)

const nativeHandleSize = uintptr(unsafe.Sizeof(uintptr(0)))

var nativeHandles = struct {
	sync.Mutex
	pages [][]byte
	free  []unsafe.Pointer
}{}

func allocNativeHandle() unsafe.Pointer {
	nativeHandles.Lock()
	defer nativeHandles.Unlock()
	if count := len(nativeHandles.free); count != 0 {
		handle := nativeHandles.free[count-1]
		nativeHandles.free[count-1] = nil
		nativeHandles.free = nativeHandles.free[:count-1]
		return handle
	}
	_, page := nativeMap(uintptr(os.Getpagesize()))
	base := unsafe.Pointer(&page[0])
	for offset := nativeHandleSize; offset+nativeHandleSize <= uintptr(len(page)); offset += nativeHandleSize {
		nativeHandles.free = append(nativeHandles.free, unsafe.Add(base, offset))
	}
	nativeHandles.pages = append(nativeHandles.pages, page)
	return base
}

func freeNativeHandle(handle unsafe.Pointer) {
	if handle == nil {
		return
	}
	nativeHandles.Lock()
	nativeHandles.free = append(nativeHandles.free, handle)
	nativeHandles.Unlock()
}
