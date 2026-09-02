//go:build cgo

package main

/*
#include <stdint.h>
typedef void (*tsnative_entry1_fn)(void *);
typedef void (*tsnative_entry2_fn)(void *, void *);

static void tsnative_call_entry1(uintptr_t entry, void *arg0) {
	((tsnative_entry1_fn)entry)(arg0);
}

static void tsnative_call_entry2(uintptr_t entry, void *arg0, void *arg1) {
	((tsnative_entry2_fn)entry)(arg0, arg1);
}
*/
import "C"

import "unsafe"

// callNativeEntry1 is the temporary cgo implementation used while the native
// ABI is still built as a Go c-archive. The cgo-free build uses trampoline_amd64.s.
func callNativeEntry1(entry uintptr, arg0 unsafe.Pointer) {
	C.tsnative_call_entry1(C.uintptr_t(entry), arg0)
}

// callNativeEntry2 is the temporary cgo implementation used while the native
// ABI is still built as a Go c-archive. The cgo-free build uses trampoline_amd64.s.
func callNativeEntry2(entry uintptr, arg0, arg1 unsafe.Pointer) {
	C.tsnative_call_entry2(C.uintptr_t(entry), arg0, arg1)
}
