package source

import (
	"testing"
)

func TestFileSet(t *testing.T) {
	fs := NewFileSet()

	src1 := []byte("hello\nworld\nfoo")
	f1 := fs.AddFile("test1.ts", src1)

	if f1.LineCount() != 3 {
		t.Fatalf("expected 3 lines, got %d", f1.LineCount())
	}

	if f1.LineContent(1) != "hello" {
		t.Errorf("line 1: got %q, want %q", f1.LineContent(1), "hello")
	}
	if f1.LineContent(2) != "world" {
		t.Errorf("line 2: got %q, want %q", f1.LineContent(2), "world")
	}
	if f1.LineContent(3) != "foo" {
		t.Errorf("line 3: got %q, want %q", f1.LineContent(3), "foo")
	}

	// Pos resolution
	// 'w' in "world" is offset 6 (0-based) in f1
	wPos := f1.Base + 6
	loc := fs.Location(wPos)
	if loc.Line != 2 || loc.Column != 1 || loc.Filename != "test1.ts" {
		t.Errorf("unexpected loc: %+v", loc)
	}

	// Add second file
	src2 := []byte("let x = 42;")
	f2 := fs.AddFile("test2.ts", src2)
	xPos := f2.Base + 4 // 'x'
	loc2 := fs.Location(xPos)
	if loc2.Line != 1 || loc2.Column != 5 || loc2.Filename != "test2.ts" {
		t.Errorf("unexpected loc2: %+v", loc2)
	}
}
