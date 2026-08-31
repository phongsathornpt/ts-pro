package tsast

import "fmt"

func (f *File) stringAt(index uint32) (string, error) {
	count := (f.stringTable - f.stringOffsets) / 4
	if index+1 >= count {
		return "", fmt.Errorf("TypeScript AST string index %d out of range", index)
	}
	start := f.u32(f.stringOffsets + index*4)
	end := f.u32(f.stringOffsets + (index+1)*4)
	if end < start || f.stringTable+end > uint32(len(f.data)) {
		return "", fmt.Errorf("invalid TypeScript AST string span %d:%d", start, end)
	}
	return string(f.data[f.stringTable+start : f.stringTable+end]), nil
}

func (n Node) Text() (string, bool) {
	var index uint32
	switch n.Kind() {
	case KindIdentifier:
		if n.dataType() != dataTypeString {
			return "", false
		}
		index = n.Data() & stringIndexMask
	case KindNumericLiteral, KindSourceFile:
		if n.dataType() != dataTypeExtended {
			return "", false
		}
		offset := n.file.extendedData + (n.Data() & extendedDataIndexMask)
		if offset+4 > uint32(len(n.file.data)) {
			return "", false
		}
		index = n.file.u32(offset)
	default:
		return "", false
	}
	text, err := n.file.stringAt(index)
	return text, err == nil
}
