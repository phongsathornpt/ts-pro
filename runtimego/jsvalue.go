package runtimego

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
		raw = heapAllocRefs(size, unsafe.Pointer(&offset), 1)
	default:
		raw = heapAllocAtomic(size)
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

func jsValueBoxF64(number float64) unsafe.Pointer {
	value := newNativeJSValue(nativeJSTagNumber)
	value.payload = math.Float64bits(number)
	return unsafe.Pointer(value)
}

func jsValueBoxString(raw unsafe.Pointer) unsafe.Pointer {
	value := newNativeJSValue(nativeJSTagString)
	nativeJSSetRef(value, raw)
	return unsafe.Pointer(value)
}

func jsValueBoxBool(raw uint8) unsafe.Pointer {
	value := newNativeJSValue(nativeJSTagBoolean)
	if raw != 0 {
		value.payload = 1
	}
	return unsafe.Pointer(value)
}

func jsValueBoxObject(raw unsafe.Pointer) unsafe.Pointer {
	return jsValueBoxObjectShape(raw, 0)
}

func jsValueBoxObjectShape(raw unsafe.Pointer, shape uint32) unsafe.Pointer {
	value := newNativeJSValue(nativeJSTagObject)
	value.reserved = shape + 1
	nativeJSSetRef(value, raw)
	return unsafe.Pointer(value)
}

func jsValueObjectShape(raw unsafe.Pointer) uint32 {
	value := (*nativeJSValue)(raw)
	if value == nil || value.tag != nativeJSTagObject || value.reserved == 0 {
		return ^uint32(0)
	}
	return value.reserved - 1
}

func jsValueBoxFunction(raw unsafe.Pointer) unsafe.Pointer {
	value := newNativeJSValue(nativeJSTagFunction)
	nativeJSSetRef(value, raw)
	return unsafe.Pointer(value)
}

func jsValueBoxFunctionTarget(raw unsafe.Pointer, target uint32) unsafe.Pointer {
	value := newNativeJSValue(nativeJSTagFunction)
	value.reserved = target + 1
	nativeJSSetRef(value, raw)
	return unsafe.Pointer(value)
}

func jsValueFunctionTarget(raw unsafe.Pointer) uint32 {
	value := (*nativeJSValue)(raw)
	if value == nil || value.tag != nativeJSTagFunction || value.reserved == 0 {
		return ^uint32(0)
	}
	return value.reserved - 1
}

func jsValueBoxArray(raw unsafe.Pointer) unsafe.Pointer {
	value := newNativeJSValue(nativeJSTagArray)
	nativeJSSetRef(value, raw)
	return unsafe.Pointer(value)
}

func jsValueUnboxObject(raw unsafe.Pointer) unsafe.Pointer {
	value := (*nativeJSValue)(raw)
	if value == nil || value.tag != nativeJSTagObject {
		nativeAbort("unbox object failed")
	}
	return nativeJSRef(value)
}

func jsValueUnboxObjectShape(raw unsafe.Pointer, expected uint32) unsafe.Pointer {
	value := (*nativeJSValue)(raw)
	if value == nil || value.tag != nativeJSTagObject || value.reserved == 0 || value.reserved-1 != expected {
		nativeAbort("unbox object shape failed")
	}
	return nativeJSRef(value)
}

func jsValueUnboxFunction(raw unsafe.Pointer) unsafe.Pointer {
	value := (*nativeJSValue)(raw)
	if value == nil || value.tag != nativeJSTagFunction {
		nativeAbort("unbox function failed")
	}
	return nativeJSRef(value)
}

func jsValueDynamicSetMissing() {
	nativeAbort("dynamic property write requires an existing closed-shape field")
}

func jsValueDynamicCallInvalid() {
	nativeAbort("dynamic call target or argument ABI is invalid")
}

func jsValueUnboxF64(raw unsafe.Pointer) float64 {
	value := (*nativeJSValue)(raw)
	if value == nil || value.tag != nativeJSTagNumber {
		nativeAbort("unbox f64 failed")
	}
	return nativeJSNumber(value)
}

func jsValueUnboxString(raw unsafe.Pointer) unsafe.Pointer {
	value := (*nativeJSValue)(raw)
	if value == nil || value.tag != nativeJSTagString {
		nativeAbort("unbox string failed")
	}
	return nativeJSRef(value)
}

func jsValueUnboxBool(raw unsafe.Pointer) uint8 {
	value := (*nativeJSValue)(raw)
	if value == nil || value.tag != nativeJSTagBoolean {
		nativeAbort("unbox bool failed")
	}
	return nativeJSBoolResult(nativeJSBool(value))
}

func jsValueUnboxArray(raw unsafe.Pointer) unsafe.Pointer {
	value := (*nativeJSValue)(raw)
	if value == nil || value.tag != nativeJSTagArray {
		nativeAbort("unbox array failed")
	}
	return nativeJSRef(value)
}

func jsValueNull() unsafe.Pointer {
	return unsafe.Pointer(newNativeJSValue(nativeJSTagNull))
}

func jsValueUndefined() unsafe.Pointer {
	return unsafe.Pointer(newNativeJSValue(nativeJSTagUndefined))
}

func nativeJSStringLiteral(text string) unsafe.Pointer {
	if len(text) == 0 {
		return stringNew(nil, 0)
	}
	bytes := []byte(text)
	return stringNew(unsafe.Pointer(&bytes[0]), uint64(len(bytes)))
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
		nativeAbort("jsvalue to string on nil")
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
		nativeAbort("invalid jsvalue tag in to string")
		return nil
	}
}

func nativeJSToPrimitive(value *nativeJSValue) *nativeJSValue {
	if value == nil {
		nativeAbort("jsvalue to primitive on nil")
	}
	switch value.tag {
	case nativeJSTagObject, nativeJSTagArray, nativeJSTagFunction:
		return (*nativeJSValue)(jsValueBoxString(nativeJSToString(value)))
	default:
		return value
	}
}

func nativeJSToNumber(value *nativeJSValue) float64 {
	if value == nil {
		nativeAbort("jsvalue to number on nil")
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
		nativeAbort("invalid jsvalue tag in to number")
		return 0
	}
}

func nativeJSRelationalCompare(left, right *nativeJSValue) (int, bool) {
	if left == nil || right == nil {
		nativeAbort("jsvalue relational compare on nil")
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
		nativeAbort("relational compare with object/array/function")
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

func jsValueAdd(leftRaw, rightRaw unsafe.Pointer) unsafe.Pointer {
	left := (*nativeJSValue)(leftRaw)
	right := (*nativeJSValue)(rightRaw)
	if left == nil || right == nil {
		nativeAbort("jsvalue add on nil")
	}
	left, right = nativeJSToPrimitive(left), nativeJSToPrimitive(right)
	if left.tag == nativeJSTagString || right.tag == nativeJSTagString {
		combined := stringConcat(nativeJSToString(left), nativeJSToString(right))
		return jsValueBoxString(combined)
	}
	return jsValueBoxF64(nativeJSToNumber(left) + nativeJSToNumber(right))
}

func jsValueSub(leftRaw, rightRaw unsafe.Pointer) float64 {
	return nativeJSToNumber((*nativeJSValue)(leftRaw)) - nativeJSToNumber((*nativeJSValue)(rightRaw))
}

func jsValueMul(leftRaw, rightRaw unsafe.Pointer) float64 {
	return nativeJSToNumber((*nativeJSValue)(leftRaw)) * nativeJSToNumber((*nativeJSValue)(rightRaw))
}

func jsValueDiv(leftRaw, rightRaw unsafe.Pointer) float64 {
	return nativeJSToNumber((*nativeJSValue)(leftRaw)) / nativeJSToNumber((*nativeJSValue)(rightRaw))
}

func nativeJSBoolResult(value bool) uint8 {
	if value {
		return 1
	}
	return 0
}

func jsValueLt(leftRaw, rightRaw unsafe.Pointer) uint8 {
	cmp, ok := nativeJSRelationalCompare((*nativeJSValue)(leftRaw), (*nativeJSValue)(rightRaw))
	return nativeJSBoolResult(ok && cmp < 0)
}

func jsValueLe(leftRaw, rightRaw unsafe.Pointer) uint8 {
	cmp, ok := nativeJSRelationalCompare((*nativeJSValue)(leftRaw), (*nativeJSValue)(rightRaw))
	return nativeJSBoolResult(ok && cmp <= 0)
}

func jsValueGt(leftRaw, rightRaw unsafe.Pointer) uint8 {
	cmp, ok := nativeJSRelationalCompare((*nativeJSValue)(leftRaw), (*nativeJSValue)(rightRaw))
	return nativeJSBoolResult(ok && cmp > 0)
}

func jsValueGe(leftRaw, rightRaw unsafe.Pointer) uint8 {
	cmp, ok := nativeJSRelationalCompare((*nativeJSValue)(leftRaw), (*nativeJSValue)(rightRaw))
	return nativeJSBoolResult(ok && cmp >= 0)
}

func jsValueEq(leftRaw, rightRaw unsafe.Pointer) uint8 {
	return nativeJSBoolResult(nativeJSLooseEqual((*nativeJSValue)(leftRaw), (*nativeJSValue)(rightRaw)))
}

func jsValueNe(leftRaw, rightRaw unsafe.Pointer) uint8 {
	return nativeJSBoolResult(!nativeJSLooseEqual((*nativeJSValue)(leftRaw), (*nativeJSValue)(rightRaw)))
}

func jsValueStrictEq(leftRaw, rightRaw unsafe.Pointer) uint8 {
	return nativeJSBoolResult(nativeJSStrictEqual((*nativeJSValue)(leftRaw), (*nativeJSValue)(rightRaw)))
}

func jsValueStrictNe(leftRaw, rightRaw unsafe.Pointer) uint8 {
	return nativeJSBoolResult(!nativeJSStrictEqual((*nativeJSValue)(leftRaw), (*nativeJSValue)(rightRaw)))
}

func consoleLogJSValue(raw unsafe.Pointer) {
	value := (*nativeJSValue)(raw)
	if value == nil {
		nativeAbort("log nil jsvalue")
	}
	switch value.tag {
	case nativeJSTagNumber:
		fmt.Printf("%.17g\n", nativeJSNumber(value))
	case nativeJSTagString:
		consoleLogString(nativeJSRef(value))
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
		nativeAbort("log unknown jsvalue tag")
	}
}
