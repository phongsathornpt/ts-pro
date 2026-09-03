package string

import (
	"bytes"
)

// TSString represents the runtime layout of a native string.
type TSString struct {
	Length int
	Data   []byte
}

// New creates a new runtime string from a Go string.
func New(s string) *TSString {
	b := []byte(s)
	return &TSString{
		Length: len(b),
		Data:   b,
	}
}

// Concat concatenates two runtime strings.
func Concat(a, b *TSString) *TSString {
	if a == nil && b == nil {
		return &TSString{}
	}
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	res := make([]byte, a.Length+b.Length)
	copy(res, a.Data)
	copy(res[a.Length:], b.Data)
	return &TSString{
		Length: len(res),
		Data:   res,
	}
}

// Equals checks if two runtime strings are identical.
func Equals(a, b *TSString) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Length != b.Length {
		return false
	}
	return bytes.Equal(a.Data, b.Data)
}

// Slice returns a substring as a new TSString.
func Slice(s *TSString, start, end int) *TSString {
	if s == nil {
		return &TSString{}
	}
	if start < 0 {
		start = 0
	}
	if end > s.Length {
		end = s.Length
	}
	if start >= end {
		return &TSString{}
	}
	sub := s.Data[start:end]
	res := make([]byte, len(sub))
	copy(res, sub)
	return &TSString{
		Length: len(res),
		Data:   res,
	}
}

func (s *TSString) String() string {
	if s == nil {
		return ""
	}
	return string(s.Data)
}
