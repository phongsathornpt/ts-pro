package array

import (
	"math"
	"testing"
)

func TestArrayOperations(t *testing.T) {
	arr := New(2)
	// Store float64 3.14 bitwise
	fBits := math.Float64bits(3.14)
	Push(arr, fBits)
	Push(arr, 42)
	Push(arr, 99) // triggers growth

	if arr.Length != 3 {
		t.Errorf("arr.Length = %d, want 3", arr.Length)
	}

	val, ok := Get(arr, 0)
	if !ok || math.Float64frombits(val) != 3.14 {
		t.Errorf("Get(0) = %v, want 3.14", math.Float64frombits(val))
	}

	if _, ok := Get(arr, -1); ok {
		t.Errorf("expected Get(-1) to fail")
	}
	if _, ok := Get(arr, 10); ok {
		t.Errorf("expected Get(10) to fail")
	}

	if !Set(arr, 1, 100) {
		t.Errorf("expected Set(1) to succeed")
	}
	if Set(arr, -1, 100) {
		t.Errorf("expected Set(-1) to fail")
	}
	if Set(arr, 10, 100) {
		t.Errorf("expected Set(10) to fail")
	}

	popVal, ok := Pop(arr)
	if !ok || popVal != 99 {
		t.Errorf("Pop() = %v, want 99", popVal)
	}
	if arr.Length != 2 {
		t.Errorf("arr.Length after pop = %d, want 2", arr.Length)
	}

	// Pop remaining
	Pop(arr)
	Pop(arr)
	if _, ok := Pop(arr); ok {
		t.Errorf("expected Pop on empty array to fail")
	}
}
