package array

// TSArray represents a heap-allocated dynamic array of 64-bit slots (numbers, pointers, booleans).
type TSArray struct {
	Length   int
	Capacity int
	Data     []uint64
}

// New creates a new array with initial capacity.
func New(capacity int) *TSArray {
	if capacity < 4 {
		capacity = 4
	}
	return &TSArray{
		Length:   0,
		Capacity: capacity,
		Data:     make([]uint64, 0, capacity),
	}
}

// Push appends an element to the array, growing backing storage if necessary.
func Push(arr *TSArray, val uint64) {
	arr.Data = append(arr.Data, val)
	arr.Length = len(arr.Data)
	arr.Capacity = cap(arr.Data)
}

// Pop removes and returns the last element, or 0 if empty.
func Pop(arr *TSArray) (uint64, bool) {
	if arr.Length == 0 {
		return 0, false
	}
	val := arr.Data[arr.Length-1]
	arr.Data = arr.Data[:arr.Length-1]
	arr.Length--
	return val, true
}

// Get reads the element at index, returning false if out of bounds.
func Get(arr *TSArray, index int) (uint64, bool) {
	if index < 0 || index >= arr.Length {
		return 0, false
	}
	return arr.Data[index], true
}

// Set writes the element at index, returning false if out of bounds.
func Set(arr *TSArray, index int, val uint64) bool {
	if index < 0 || index >= arr.Length {
		return false
	}
	arr.Data[index] = val
	return true
}
