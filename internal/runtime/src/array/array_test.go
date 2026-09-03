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

	popVal, ok := Pop(arr)
	if !ok || popVal != 99 {
		t.Errorf("Pop() = %v, want 99", popVal)
	}
	if arr.Length != 2 {
		t.Errorf("arr.Length after pop = %d, want 2", arr.Length)
	}
}
