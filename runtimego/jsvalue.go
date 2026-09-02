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
	"strings"
	"unicode/utf16"
	"unsafe"
)

const (
	nativeJSTagNumber    uint32 = 1
	nativeJSTagString    uint32 = 2
	nativeJSTagBoolean   uint32 = 3
	nativeJSTagNull      uint32 = 4
	nativeJSTagUndefined uint32 = 5
	nativeJSTagObject    uint32 = 6
	nativeJSTagFunction  uint32 = 7
	nativeJSTagArray     uint32 = 8
)

type nativeJSValue struct {
	tag      uint32
	reserved uint32
	payload  uint64
}

func newNativeJSValue(tag uint32) *nativeJSValue {
	size := unsafe.Sizeof(nativeJSValue{})
	var raw unsafe.Pointer
	switch tag {
	case nativeJSTagString, nativeJSTagObject, nativeJSTagFunction, nativeJSTagArray:
		offset := unsafe.Offsetof(nativeJSValue{}.payload)
		raw = tsnative_heap_alloc_refs(size, unsafe.Pointer(&offset), 1)
	default:
		raw = tsnative_heap_alloc_atomic(size)
	}
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
	return *(*unsafe.Pointer)(unsafe.Pointer(&value.payload))
}

func nativeJSSetRef(value *nativeJSValue, raw unsafe.Pointer) {
	*(*unsafe.Pointer)(unsafe.Pointer(&value.payload)) = raw
}

func nativeJSBool(value *nativeJSValue) bool {
	return value.payload != 0
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
	nativeJSSetRef(value, raw)
	return unsafe.Pointer(value)
}

//export tsnative_jsvalue_box_bool
func tsnative_jsvalue_box_bool(raw C.uint8_t) unsafe.Pointer {
	value := newNativeJSValue(nativeJSTagBoolean)
	if raw != 0 {
		value.payload = 1
	}
	return unsafe.Pointer(value)
}

//export tsnative_jsvalue_box_object
func tsnative_jsvalue_box_object(raw unsafe.Pointer) unsafe.Pointer {
	return tsnative_jsvalue_box_object_shape(raw, 0)
}

//export tsnative_jsvalue_box_object_shape
func tsnative_jsvalue_box_object_shape(raw unsafe.Pointer, shape C.uint32_t) unsafe.Pointer {
	value := newNativeJSValue(nativeJSTagObject)
	value.reserved = uint32(shape) + 1
	nativeJSSetRef(value, raw)
	return unsafe.Pointer(value)
}

//export tsnative_jsvalue_object_shape
func tsnative_jsvalue_object_shape(raw unsafe.Pointer) C.uint32_t {
	value := (*nativeJSValue)(raw)
	if value == nil || value.tag != nativeJSTagObject || value.reserved == 0 {
		return C.uint32_t(^uint32(0))
	}
	return C.uint32_t(value.reserved - 1)
}

//export tsnative_jsvalue_box_function
func tsnative_jsvalue_box_function(raw unsafe.Pointer) unsafe.Pointer {
	value := newNativeJSValue(nativeJSTagFunction)
	nativeJSSetRef(value, raw)
	return unsafe.Pointer(value)
}

//export tsnative_jsvalue_box_array
func tsnative_jsvalue_box_array(raw unsafe.Pointer) unsafe.Pointer {
	value := newNativeJSValue(nativeJSTagArray)
	nativeJSSetRef(value, raw)
	return unsafe.Pointer(value)
}

//export tsnative_jsvalue_unbox_object
func tsnative_jsvalue_unbox_object(raw unsafe.Pointer) unsafe.Pointer {
	value := (*nativeJSValue)(raw)
	if value == nil || value.tag != nativeJSTagObject {
		C.abort()
	}
	return nativeJSRef(value)
}

//export tsnative_jsvalue_unbox_object_shape
func tsnative_jsvalue_unbox_object_shape(raw unsafe.Pointer, expected C.uint32_t) unsafe.Pointer {
	value := (*nativeJSValue)(raw)
	if value == nil || value.tag != nativeJSTagObject || value.reserved == 0 || value.reserved-1 != uint32(expected) {
		C.abort()
	}
	return nativeJSRef(value)
}

//export tsnative_jsvalue_unbox_function
func tsnative_jsvalue_unbox_function(raw unsafe.Pointer) unsafe.Pointer {
	value := (*nativeJSValue)(raw)
	if value == nil || value.tag != nativeJSTagFunction {
		C.abort()
	}
	return nativeJSRef(value)
}

//export tsnative_jsvalue_dynamic_set_missing
func tsnative_jsvalue_dynamic_set_missing() {
	nativeAbort("dynamic property write requires an existing closed-shape field")
}

//export tsnative_jsvalue_unbox_f64
func tsnative_jsvalue_unbox_f64(raw unsafe.Pointer) C.double {
	value := (*nativeJSValue)(raw)
	if value == nil || value.tag != nativeJSTagNumber {
		C.abort()
	}
	return C.double(nativeJSNumber(value))
}

//export tsnative_jsvalue_unbox_string
func tsnative_jsvalue_unbox_string(raw unsafe.Pointer) unsafe.Pointer {
	value := (*nativeJSValue)(raw)
	if value == nil || value.tag != nativeJSTagString {
		C.abort()
	}
	return nativeJSRef(value)
}

//export tsnative_jsvalue_unbox_bool
func tsnative_jsvalue_unbox_bool(raw unsafe.Pointer) C.uint8_t {
	value := (*nativeJSValue)(raw)
	if value == nil || value.tag != nativeJSTagBoolean {
		C.abort()
	}
	return nativeJSBoolResult(nativeJSBool(value))
}

//export tsnative_jsvalue_unbox_array
func tsnative_jsvalue_unbox_array(raw unsafe.Pointer) unsafe.Pointer {
	value := (*nativeJSValue)(raw)
	if value == nil || value.tag != nativeJSTagArray {
		C.abort()
	}
	return nativeJSRef(value)
}

//export tsnative_jsvalue_null
func tsnative_jsvalue_null() unsafe.Pointer {
	return unsafe.Pointer(newNativeJSValue(nativeJSTagNull))
}

//export tsnative_jsvalue_undefined
func tsnative_jsvalue_undefined() unsafe.Pointer {
	return unsafe.Pointer(newNativeJSValue(nativeJSTagUndefined))
}

func nativeJSStringLiteral(text string) unsafe.Pointer {
	if len(text) == 0 {
		return tsnative_string_new(nil, 0)
	}
	bytes := []byte(text)
	return tsnative_string_new(unsafe.Pointer(&bytes[0]), C.uint64_t(len(bytes)))
}

func nativeJSStringToNumber(raw unsafe.Pointer) float64 {
	text := strings.TrimSpace(string(nativeStringBytes(raw)))
	if text == "" {
		return 0
	}
	switch text {
	case "Infinity", "+Infinity":
		return math.Inf(1)
	case "-Infinity":
		return math.Inf(-1)
	}
	if len(text) > 2 && text[0] == '0' {
		base := 0
		switch text[1] {
		case 'x', 'X':
			base = 16
		case 'b', 'B':
			base = 2
		case 'o', 'O':
			base = 8
		}
		if base != 0 {
			n, err := strconv.ParseUint(text[2:], base, 64)
			if err != nil {
				return math.NaN()
			}
			return float64(n)
		}
	}
	number, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return math.NaN()
	}
	return number
}

func nativeJSCompareUTF16(left, right unsafe.Pointer) int {
	a := utf16.Encode([]rune(string(nativeStringBytes(left))))
	b := utf16.Encode([]rune(string(nativeStringBytes(right))))
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	for i := 0; i < limit; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return 0
}

func nativeJSNumberText(number float64) string {
	switch {
	case math.IsNaN(number):
		return "NaN"
	case math.IsInf(number, 1):
		return "Infinity"
	case math.IsInf(number, -1):
		return "-Infinity"
	case number == 0:
		return "0"
	default:
		return strconv.FormatFloat(number, 'g', -1, 64)
	}
}

func nativeJSNumberToString(number float64) unsafe.Pointer {
	return nativeJSStringLiteral(nativeJSNumberText(number))
}

func nativeJSArrayToString(raw unsafe.Pointer) unsafe.Pointer {
	length := nativeF64ArrayLen(raw)
	if length == 0 {
		return nativeJSStringLiteral("")
	}
	var text strings.Builder
	for i := uint64(0); i < length; i++ {
		if i != 0 {
			text.WriteByte(',')
		}
		text.WriteString(nativeJSNumberText(*nativeF64ArrayElement(raw, i)))
	}
	return nativeJSStringLiteral(text.String())
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
	case nativeJSTagBoolean:
		if nativeJSBool(value) {
			return nativeJSStringLiteral("true")
		}
		return nativeJSStringLiteral("false")
	case nativeJSTagNull:
		return nativeJSStringLiteral("null")
	case nativeJSTagUndefined:
		return nativeJSStringLiteral("undefined")
	case nativeJSTagObject:
		return nativeJSStringLiteral("[object Object]")
	case nativeJSTagArray:
		return nativeJSArrayToString(nativeJSRef(value))
	case nativeJSTagFunction:
		return nativeJSStringLiteral("function () { [native code] }")
	default:
		C.abort()
		return nil
	}
}

func nativeJSToPrimitive(value *nativeJSValue) *nativeJSValue {
	if value == nil {
		C.abort()
	}
	switch value.tag {
	case nativeJSTagObject, nativeJSTagArray, nativeJSTagFunction:
		return (*nativeJSValue)(tsnative_jsvalue_box_string(nativeJSToString(value)))
	default:
		return value
	}
}

func nativeJSToNumber(value *nativeJSValue) float64 {
	if value == nil {
		C.abort()
	}
	switch value.tag {
	case nativeJSTagNumber:
		return nativeJSNumber(value)
	case nativeJSTagString:
		return nativeJSStringToNumber(nativeJSRef(value))
	case nativeJSTagBoolean:
		if nativeJSBool(value) {
			return 1
		}
		return 0
	case nativeJSTagNull:
		return 0
	case nativeJSTagUndefined:
		return math.NaN()
	case nativeJSTagObject, nativeJSTagArray, nativeJSTagFunction:
		primitive := nativeJSToPrimitive(value)
		if primitive == value {
			return math.NaN()
		}
		return nativeJSToNumber(primitive)
	default:
		C.abort()
		return 0
	}
}

func nativeJSRelationalCompare(left, right *nativeJSValue) (int, bool) {
	if left == nil || right == nil {
		C.abort()
	}
	left, right = nativeJSToPrimitive(left), nativeJSToPrimitive(right)
	if left.tag == nativeJSTagString && right.tag == nativeJSTagString {
		return nativeJSCompareUTF16(nativeJSRef(left), nativeJSRef(right)), true
	}
	a, b := nativeJSToNumber(left), nativeJSToNumber(right)
	if math.IsNaN(a) || math.IsNaN(b) {
		return 0, false
	}
	if a < b {
		return -1, true
	}
	if a > b {
		return 1, true
	}
	return 0, true
}

func nativeJSStrictEqual(left, right *nativeJSValue) bool {
	if left == nil || right == nil || left.tag != right.tag {
		return false
	}
	switch left.tag {
	case nativeJSTagNumber:
		a, b := nativeJSNumber(left), nativeJSNumber(right)
		return !math.IsNaN(a) && !math.IsNaN(b) && a == b
	case nativeJSTagString:
		return string(nativeStringBytes(nativeJSRef(left))) == string(nativeStringBytes(nativeJSRef(right)))
	case nativeJSTagBoolean:
		return nativeJSBool(left) == nativeJSBool(right)
	case nativeJSTagNull, nativeJSTagUndefined:
		return true
	case nativeJSTagObject, nativeJSTagArray, nativeJSTagFunction:
		return nativeJSRef(left) == nativeJSRef(right)
	default:
		return false
	}
}

func nativeJSEqualNumber(number float64, other *nativeJSValue) bool {
	if math.IsNaN(number) || other == nil {
		return false
	}
	switch other.tag {
	case nativeJSTagNumber:
		value := nativeJSNumber(other)
		return !math.IsNaN(value) && number == value
	case nativeJSTagString:
		value := nativeJSStringToNumber(nativeJSRef(other))
		return !math.IsNaN(value) && number == value
	case nativeJSTagBoolean:
		if nativeJSBool(other) {
			return number == 1
		}
		return number == 0
	case nativeJSTagNull, nativeJSTagUndefined:
		return false
	case nativeJSTagObject, nativeJSTagArray, nativeJSTagFunction:
		C.abort()
	}
	return false
}

func nativeJSLooseEqual(left, right *nativeJSValue) bool {
	if left == nil || right == nil {
		return false
	}
	if left.tag == right.tag {
		return nativeJSStrictEqual(left, right)
	}
	if (left.tag == nativeJSTagNull && right.tag == nativeJSTagUndefined) || (left.tag == nativeJSTagUndefined && right.tag == nativeJSTagNull) {
		return true
	}
	if left.tag == nativeJSTagNumber {
		return nativeJSEqualNumber(nativeJSNumber(left), right)
	}
	if right.tag == nativeJSTagNumber {
		return nativeJSEqualNumber(nativeJSNumber(right), left)
	}
	if left.tag == nativeJSTagBoolean {
		n := 0.0
		if nativeJSBool(left) {
			n = 1
		}
		return nativeJSEqualNumber(n, right)
	}
	if right.tag == nativeJSTagBoolean {
		n := 0.0
		if nativeJSBool(right) {
			n = 1
		}
		return nativeJSEqualNumber(n, left)
	}
	if left.tag == nativeJSTagString && right.tag == nativeJSTagString {
		return nativeJSStrictEqual(left, right)
	}
	if left.tag == nativeJSTagObject || left.tag == nativeJSTagArray || left.tag == nativeJSTagFunction {
		return nativeJSLooseEqual(nativeJSToPrimitive(left), right)
	}
	if right.tag == nativeJSTagObject || right.tag == nativeJSTagArray || right.tag == nativeJSTagFunction {
		return nativeJSLooseEqual(left, nativeJSToPrimitive(right))
	}
	return false
}

//export tsnative_jsvalue_add
func tsnative_jsvalue_add(leftRaw, rightRaw unsafe.Pointer) unsafe.Pointer {
	left := (*nativeJSValue)(leftRaw)
	right := (*nativeJSValue)(rightRaw)
	if left == nil || right == nil {
		C.abort()
	}
	left, right = nativeJSToPrimitive(left), nativeJSToPrimitive(right)
	if left.tag == nativeJSTagString || right.tag == nativeJSTagString {
		combined := tsnative_string_concat(nativeJSToString(left), nativeJSToString(right))
		return tsnative_jsvalue_box_string(combined)
	}
	return tsnative_jsvalue_box_f64(C.double(nativeJSToNumber(left) + nativeJSToNumber(right)))
}

//export tsnative_jsvalue_sub
func tsnative_jsvalue_sub(leftRaw, rightRaw unsafe.Pointer) C.double {
	return C.double(nativeJSToNumber((*nativeJSValue)(leftRaw)) - nativeJSToNumber((*nativeJSValue)(rightRaw)))
}

//export tsnative_jsvalue_mul
func tsnative_jsvalue_mul(leftRaw, rightRaw unsafe.Pointer) C.double {
	return C.double(nativeJSToNumber((*nativeJSValue)(leftRaw)) * nativeJSToNumber((*nativeJSValue)(rightRaw)))
}

//export tsnative_jsvalue_div
func tsnative_jsvalue_div(leftRaw, rightRaw unsafe.Pointer) C.double {
	return C.double(nativeJSToNumber((*nativeJSValue)(leftRaw)) / nativeJSToNumber((*nativeJSValue)(rightRaw)))
}

func nativeJSBoolResult(value bool) C.uint8_t {
	if value {
		return 1
	}
	return 0
}

//export tsnative_jsvalue_lt
func tsnative_jsvalue_lt(leftRaw, rightRaw unsafe.Pointer) C.uint8_t {
	cmp, ok := nativeJSRelationalCompare((*nativeJSValue)(leftRaw), (*nativeJSValue)(rightRaw))
	return nativeJSBoolResult(ok && cmp < 0)
}

//export tsnative_jsvalue_le
func tsnative_jsvalue_le(leftRaw, rightRaw unsafe.Pointer) C.uint8_t {
	cmp, ok := nativeJSRelationalCompare((*nativeJSValue)(leftRaw), (*nativeJSValue)(rightRaw))
	return nativeJSBoolResult(ok && cmp <= 0)
}

//export tsnative_jsvalue_gt
func tsnative_jsvalue_gt(leftRaw, rightRaw unsafe.Pointer) C.uint8_t {
	cmp, ok := nativeJSRelationalCompare((*nativeJSValue)(leftRaw), (*nativeJSValue)(rightRaw))
	return nativeJSBoolResult(ok && cmp > 0)
}

//export tsnative_jsvalue_ge
func tsnative_jsvalue_ge(leftRaw, rightRaw unsafe.Pointer) C.uint8_t {
	cmp, ok := nativeJSRelationalCompare((*nativeJSValue)(leftRaw), (*nativeJSValue)(rightRaw))
	return nativeJSBoolResult(ok && cmp >= 0)
}

//export tsnative_jsvalue_eq
func tsnative_jsvalue_eq(leftRaw, rightRaw unsafe.Pointer) C.uint8_t {
	return nativeJSBoolResult(nativeJSLooseEqual((*nativeJSValue)(leftRaw), (*nativeJSValue)(rightRaw)))
}

//export tsnative_jsvalue_ne
func tsnative_jsvalue_ne(leftRaw, rightRaw unsafe.Pointer) C.uint8_t {
	return nativeJSBoolResult(!nativeJSLooseEqual((*nativeJSValue)(leftRaw), (*nativeJSValue)(rightRaw)))
}

//export tsnative_jsvalue_strict_eq
func tsnative_jsvalue_strict_eq(leftRaw, rightRaw unsafe.Pointer) C.uint8_t {
	return nativeJSBoolResult(nativeJSStrictEqual((*nativeJSValue)(leftRaw), (*nativeJSValue)(rightRaw)))
}

//export tsnative_jsvalue_strict_ne
func tsnative_jsvalue_strict_ne(leftRaw, rightRaw unsafe.Pointer) C.uint8_t {
	return nativeJSBoolResult(!nativeJSStrictEqual((*nativeJSValue)(leftRaw), (*nativeJSValue)(rightRaw)))
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
	case nativeJSTagBoolean:
		if nativeJSBool(value) {
			fmt.Println("true")
		} else {
			fmt.Println("false")
		}
	case nativeJSTagNull:
		fmt.Println("null")
	case nativeJSTagUndefined:
		fmt.Println("undefined")
	default:
		C.abort()
	}
}
