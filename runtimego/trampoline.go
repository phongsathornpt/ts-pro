//go:build !cgo

package main

import "unsafe"

// callNativeEntry1 invokes a SysV function pointer with 1 pointer argument: fn(arg0)
func callNativeEntry1(entry uintptr, arg0 unsafe.Pointer)

// callNativeEntry2 invokes a SysV function pointer with 2 pointer arguments: fn(arg0, arg1)
func callNativeEntry2(entry uintptr, arg0, arg1 unsafe.Pointer)
