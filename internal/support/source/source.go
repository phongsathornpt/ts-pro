package source

import (
	"fmt"
	"sort"
	"sync"
)

// Pos represents a compact byte offset within a source FileSet.
// A Pos value of 0 indicates an unknown or invalid position.
type Pos uint32

const NoPos Pos = 0

// Span represents a contiguous range of source code [Start, End).
type Span struct {
	Start Pos
	End   Pos
}

// IsValid reports whether the span has valid start and end positions.
func (s Span) IsValid() bool {
	return s.Start > 0 && s.End >= s.Start
}

// Location represents a decoded human-readable file position.
type Location struct {
	Filename string
	Line     int
	Column   int
}

func (l Location) String() string {
	if l.Filename == "" {
		return fmt.Sprintf("%d:%d", l.Line, l.Column)
	}
	return fmt.Sprintf("%s:%d:%d", l.Filename, l.Line, l.Column)
}

// File represents a single source file in the FileSet.
type File struct {
	Name  string
	Base  Pos
	Size  int
	Src   []byte
	lines []int // Byte offsets within File of each line start (0-indexed relative to File)
}

// LineCount returns the total number of lines in the file.
func (f *File) LineCount() int {
	return len(f.lines)
}

// LineContent returns the content of the given 1-based line number without trailing newline.
func (f *File) LineContent(line int) string {
	if line < 1 || line > len(f.lines) {
		return ""
	}
	start := f.lines[line-1]
	var end int
	if line < len(f.lines) {
		end = f.lines[line]
		if end > start && f.Src[end-1] == '\n' {
			end--
			if end > start && f.Src[end-1] == '\r' {
				end--
			}
		}
	} else {
		end = len(f.Src)
	}
	if start > len(f.Src) {
		return ""
	}
	if end > len(f.Src) {
		end = len(f.Src)
	}
	return string(f.Src[start:end])
}

// Location translates a Pos within this file into a 1-based Line and Column.
func (f *File) Location(pos Pos) Location {
	if pos < f.Base || int(pos-f.Base) > f.Size {
		return Location{Filename: f.Name}
	}
	offset := int(pos - f.Base)
	// Binary search lines for the largest offset <= pos
	idx := sort.Search(len(f.lines), func(i int) bool {
		return f.lines[i] > offset
	}) - 1
	if idx < 0 {
		idx = 0
	}
	line := idx + 1
	col := offset - f.lines[idx] + 1
	return Location{
		Filename: f.Name,
		Line:     line,
		Column:   col,
	}
}

// FileSet maintains a set of source files and maps global Pos values to files.
type FileSet struct {
	mu    sync.RWMutex
	base  Pos
	files []*File
}

// NewFileSet creates an empty FileSet.
func NewFileSet() *FileSet {
	return &FileSet{
		base:  1, // 1-based base so NoPos (0) remains invalid
		files: make([]*File, 0),
	}
}

// AddFile registers a new file with the given name and source bytes.
func (fs *FileSet) AddFile(name string, src []byte) *File {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	base := fs.base
	size := len(src)
	fs.base += Pos(size + 1) // +1 separator padding between files

	lines := []int{0}
	for i, b := range src {
		if b == '\n' && i+1 < size {
			lines = append(lines, i+1)
		}
	}

	f := &File{
		Name:  name,
		Base:  base,
		Size:  size,
		Src:   src,
		lines: lines,
	}
	fs.files = append(fs.files, f)
	return f
}

// File returns the file containing the given Pos, or nil if not found.
func (fs *FileSet) File(pos Pos) *File {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	idx := sort.Search(len(fs.files), func(i int) bool {
		return fs.files[i].Base > pos
	}) - 1
	if idx < 0 || idx >= len(fs.files) {
		return nil
	}
	f := fs.files[idx]
	if pos <= f.Base+Pos(f.Size) {
		return f
	}
	return nil
}

// Location maps a Pos to its decoded Location.
func (fs *FileSet) Location(pos Pos) Location {
	f := fs.File(pos)
	if f == nil {
		return Location{}
	}
	return f.Location(pos)
}
