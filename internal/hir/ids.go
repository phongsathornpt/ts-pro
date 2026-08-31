package hir

type ModuleID uint32
type FunctionID uint32
type BlockID uint32
type ValueID uint32
type TypeID uint32
type ShapeID uint32

func NewModuleID(value uint32) ModuleID     { return ModuleID(value) }
func NewFunctionID(value uint32) FunctionID { return FunctionID(value) }
func NewBlockID(value uint32) BlockID       { return BlockID(value) }
func NewValueID(value uint32) ValueID       { return ValueID(value) }
func NewTypeID(value uint32) TypeID         { return TypeID(value) }
func NewShapeID(value uint32) ShapeID       { return ShapeID(value) }
