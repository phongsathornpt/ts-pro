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
}
