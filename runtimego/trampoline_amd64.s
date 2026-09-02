//go:build !cgo

#include "textflag.h"

// func callNativeEntry1(entry uintptr, arg0 unsafe.Pointer)
TEXT ·callNativeEntry1(SB), NOSPLIT, $8-16
	MOVQ entry+0(FP), AX
	MOVQ arg0+8(FP), DI
	CALL AX
	RET

// func callNativeEntry2(entry uintptr, arg0, arg1 unsafe.Pointer)
TEXT ·callNativeEntry2(SB), NOSPLIT, $8-24
	MOVQ entry+0(FP), AX
	MOVQ arg0+8(FP), DI
	MOVQ arg1+16(FP), SI
	CALL AX
	RET
