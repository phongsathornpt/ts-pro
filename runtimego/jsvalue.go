package main

/*
#include <stdint.h>
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"math"
	"strconv"
	"unsafe"
)

const (
	nativeJSTagNumber uint32 = 1
	nativeJSTagString uint32 = 2
)

type nativeJSValue struct {
	tag      uint32
	reserved uint32
	payload  uint64
}

func newNativeJSValue(tag uint32) *nativeJSValue {
	raw := tsnative_heap_alloc(C.size_t(unsafe.Sizeof(nativeJSValue{})))
	value := (*nativeJSValue)(raw)
	value.tag = tag
	value.reserved = 0
	value.payload = 0
	return value
}
func nativeJSNumber(value *nativeJSValue) float64 {
	return math.Float64frombits(value.payload)
}

func nativeJSRef(value *nativeJSValue) unsafe.Pointer {
	return unsafe.Pointer(uintptr(value.payload))
}

//export tsnative_jsvalue_box_f64
func tsnative_jsvalue_box_f64(number C.double) unsafe.Pointer {
	value := newNativeJSValue(nativeJSTagNumber)
	value.payload = math.Float64bits(float64(number))
	return unsafe.Pointer(value)
}

//export tsnative_jsvalue_box_string
func tsnative_jsvalue_box_string(raw unsafe.Pointer) unsafe.Pointer {
	value := newNativeJSValue(nativeJSTagString)
	value.payload = uint64(uintptr(raw))
	return unsafe.Pointer(value)
}

func nativeJSNumberToString(number float64) unsafe.Pointer {
	text := strconv.FormatFloat(number, 'g', 17, 64)
	if len(text) == 0 {
		return tsnative_string_new(nil, 0)
	}
	bytes := []byte(text)
	return tsnative_string_new(unsafe.Pointer(&bytes[0]), C.uint64_t(len(bytes)))
}
func nativeJSToString(value *nativeJSValue) unsafe.Pointer {
	if value == nil {
		C.abort()
	}
	switch value.tag {
	case nativeJSTagString:
		return nativeJSRef(value)
	case nativeJSTagNumber:
		return nativeJSNumberToString(nativeJSNumber(value))
	default:
		C.abort()
		return nil
	}
}

//export tsnative_jsvalue_add
func tsnative_jsvalue_add(leftRaw, rightRaw unsafe.Pointer) unsafe.Pointer {
	left := (*nativeJSValue)(leftRaw)
	right := (*nativeJSValue)(rightRaw)
	if left == nil || right == nil {
		C.abort()
	}
	if left.tag == nativeJSTagNumber && right.tag == nativeJSTagNumber {
		return tsnative_jsvalue_box_f64(C.double(nativeJSNumber(left) + nativeJSNumber(right)))
	}
	combined := tsnative_string_concat(nativeJSToString(left), nativeJSToString(right))
	return tsnative_jsvalue_box_string(combined)
}

//export tsnative_console_log_jsvalue
func tsnative_console_log_jsvalue(raw unsafe.Pointer) {
	value := (*nativeJSValue)(raw)
	if value == nil {
		C.abort()
	}
	switch value.tag {
	case nativeJSTagNumber:
		fmt.Printf("%.17g\n", nativeJSNumber(value))
	case nativeJSTagString:
		tsnative_console_log_string(nativeJSRef(value))
	default:
		C.abort()
	}
}
