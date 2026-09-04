package string

import (
	"testing"
)

func TestStringOperations(t *testing.T) {
	s1 := New("hello ")
	s2 := New("world")
	s3 := Concat(s1, s2)

	if s3.String() != "hello world" {
		t.Errorf("Concat result = %q, want 'hello world'", s3.String())
	}
	if !Equals(s3, New("hello world")) {
		t.Errorf("Equals should return true for identical content")
	}

	sub := Slice(s3, 6, 11)
	if sub.String() != "world" {
		t.Errorf("Slice result = %q, want 'world'", sub.String())
	}

	// Nil and edge test cases
	if Concat(nil, nil).String() != "" {
		t.Errorf("Concat(nil, nil) should be empty")
	}
	if Concat(nil, s1) != s1 {
		t.Errorf("Concat(nil, s1) should return s1")
	}
	if Concat(s1, nil) != s1 {
		t.Errorf("Concat(s1, nil) should return s1")
	}

	if !Equals(nil, nil) {
		t.Errorf("Equals(nil, nil) should be true")
	}
	if Equals(s1, nil) || Equals(nil, s1) {
		t.Errorf("Equals with single nil should be false")
	}
	if Equals(s1, s2) {
		t.Errorf("different strings should not be equal")
	}
	diffLen := New("hello")
	if Equals(s1, diffLen) {
		t.Errorf("different length strings should not be equal")
	}

	if Slice(nil, 0, 5).String() != "" {
		t.Errorf("Slice(nil) should be empty")
	}
	if Slice(s1, -5, 100).String() != "hello " {
		t.Errorf("Slice with out-of-bound indexes failed")
	}
	if Slice(s1, 4, 2).String() != "" {
		t.Errorf("Slice with start >= end should be empty")
	}

	var nilStr *TSString
	if nilStr.String() != "" {
		t.Errorf("nilStr.String() should be empty")
	}
}
