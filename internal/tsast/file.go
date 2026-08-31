package tsast

import (
	"encoding/binary"
	"fmt"
)

type File struct {
	data          []byte
	stringOffsets uint32
	stringTable   uint32
	extendedData  uint32
	structured    uint32
	nodesOffset   uint32
	nodeCount     uint32
}

type Node struct {
	file  *File
	index uint32
}

func Decode(data []byte) (*File, error) {
	if len(data) < headerSize {
		return nil, fmt.Errorf("TypeScript AST payload too small: %d", len(data))
	}
	metadata := binary.LittleEndian.Uint32(data[headerOffsetMetadata:])
	version := metadata >> 24
	if version != ProtocolVersion {
		return nil, fmt.Errorf("unsupported TypeScript AST protocol %d; expected %d", version, ProtocolVersion)
	}
	f := &File{data: data}
	f.stringOffsets = f.u32(headerOffsetStringOffsets)
	f.stringTable = f.u32(headerOffsetStringTable)
	f.extendedData = f.u32(headerOffsetExtended)
	f.structured = f.u32(headerOffsetStructured)
	f.nodesOffset = f.u32(headerOffsetNodes)
	if err := f.validateSections(); err != nil {
		return nil, err
	}
	f.nodeCount = uint32((len(data) - int(f.nodesOffset)) / nodeLen)
	if f.nodeCount <= 1 {
		return nil, fmt.Errorf("TypeScript AST has no source-file node")
	}
	return f, nil
}

func (f *File) validateSections() error {
	offsets := []uint32{headerSize, f.stringOffsets, f.stringTable, f.extendedData, f.structured, f.nodesOffset, uint32(len(f.data))}
	for i := 1; i < len(offsets); i++ {
		if offsets[i] < offsets[i-1] || offsets[i] > uint32(len(f.data)) {
			return fmt.Errorf("invalid TypeScript AST section offsets: %v", offsets)
		}
	}
	if (len(f.data)-int(f.nodesOffset))%nodeLen != 0 {
		return fmt.Errorf("TypeScript AST node section is misaligned")
	}
	return nil
}

func (f *File) Root() Node { return Node{file: f, index: 1} }
func (f *File) Node(index uint32) (Node, bool) {
	if index == 0 || index >= f.nodeCount {
		return Node{}, false
	}
	return Node{file: f, index: index}, true
}
func (f *File) NodeCount() uint32 { return f.nodeCount }

func (f *File) u32(offset uint32) uint32 {
	return binary.LittleEndian.Uint32(f.data[offset : offset+4])
}
func (f *File) i32(offset uint32) int32 {
	return int32(binary.LittleEndian.Uint32(f.data[offset : offset+4]))
}
func (f *File) nodeOffset(index uint32) uint32 { return f.nodesOffset + index*nodeLen }

func (n Node) Index() uint32       { return n.index }
func (n Node) Kind() uint32        { return n.file.u32(n.file.nodeOffset(n.index) + nodeOffsetKind) }
func (n Node) Pos() int32          { return n.file.i32(n.file.nodeOffset(n.index) + nodeOffsetPos) }
func (n Node) End() int32          { return n.file.i32(n.file.nodeOffset(n.index) + nodeOffsetEnd) }
func (n Node) Next() uint32        { return n.file.u32(n.file.nodeOffset(n.index) + nodeOffsetNext) }
func (n Node) ParentIndex() uint32 { return n.file.u32(n.file.nodeOffset(n.index) + nodeOffsetParent) }
func (n Node) Data() uint32        { return n.file.u32(n.file.nodeOffset(n.index) + nodeOffsetData) }
func (n Node) Flags() uint32       { return n.file.u32(n.file.nodeOffset(n.index) + nodeOffsetFlags) }
func (n Node) IsList() bool        { return n.Kind() == KindNodeList }

func (n Node) dataType() uint32 { return n.Data() & dataTypeMask }
func (n Node) ChildMask() uint8 {
	if n.dataType() != dataTypeChildren {
		return 0xff
	}
	return uint8(n.Data() & childMask)
}
