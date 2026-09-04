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
	if loc.String() != "test1.ts:2:1" {
		t.Errorf("loc.String() = %q", loc.String())
	}

	noFileLoc := Location{Line: 5, Column: 10}
	if noFileLoc.String() != "5:10" {
		t.Errorf("noFileLoc.String() = %q", noFileLoc.String())
	}

	// Add second file
	src2 := []byte("let x = 42;")
	f2 := fs.AddFile("test2.ts", src2)
	xPos := f2.Base + 4 // 'x'
	loc2 := fs.Location(xPos)
	if loc2.Line != 1 || loc2.Column != 5 || loc2.Filename != "test2.ts" {
		t.Errorf("unexpected loc2: %+v", loc2)
	}

	// Edge cases
	if f1.LineContent(0) != "" || f1.LineContent(100) != "" {
		t.Errorf("out-of-range lines should be empty")
	}

	badLoc := f1.Location(0)
	if badLoc.Line != 0 {
		t.Errorf("bad pos should return zero line")
	}

	if fs.File(99999) != nil {
		t.Errorf("expected nil for non-existent pos")
	}
	if fs.Location(99999).Line != 0 {
		t.Errorf("expected empty location for non-existent pos")
	}
}
