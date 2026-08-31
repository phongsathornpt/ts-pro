package tsast

const ProtocolVersion uint32 = 5

const (
	headerOffsetMetadata      = 0
	headerOffsetStringOffsets = 24
	headerOffsetStringTable   = 28
	headerOffsetExtended      = 32
	headerOffsetStructured    = 36
	headerOffsetNodes         = 40
	headerSize                = 44
	nodeLen                   = 28
)

const (
	nodeOffsetKind   = 0
	nodeOffsetPos    = 4
	nodeOffsetEnd    = 8
	nodeOffsetNext   = 12
	nodeOffsetParent = 16
	nodeOffsetData   = 20
	nodeOffsetFlags  = 24
)

const (
	KindNodeList          uint32 = 0xffffffff
	dataTypeChildren      uint32 = 0x00000000
	dataTypeString        uint32 = 0x40000000
	dataTypeExtended      uint32 = 0x80000000
	dataTypeMask          uint32 = 0xc0000000
	childMask             uint32 = 0x000000ff
	stringIndexMask       uint32 = 0x00ffffff
	extendedDataIndexMask uint32 = 0x00ffffff
)
