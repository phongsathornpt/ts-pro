package hir

type TypeKind uint8

const (
	TypeInvalid TypeKind = iota
	TypeAny
	TypeUnknown
	TypeNever
	TypeVoid
	TypeUndefined
	TypeNull
	TypeBoolean
	TypeNumber
	TypeString
	TypeObject
	TypeArray
	TypeUnion
	TypeFunction
	TypeTask
	TypePromise
	TypeChannel
	TypeTaskGroup
	TypeParameter
)

type SemanticType struct {
	Kind       TypeKind
	Shape      ShapeID
	Element    TypeID
	Members    []TypeID
	Params     []TypeID
	ReturnType TypeID
}
type ShapeField struct {
	Name         string
	SemanticType TypeID
	Repr         Repr
}

type Shape struct {
	ID       ShapeID
	Name     string
	ClassTag uint32
	Fields   []ShapeField
}

type ReprKind uint8

const (
	ReprUnproven ReprKind = iota
	ReprVoid
	ReprBool
	ReprI32
	ReprI64
	ReprF64
	ReprStringRef
	ReprArrayRef
	ReprObjectRef
	ReprFunctionRef
	ReprTaskRef
	ReprChannelRef
	ReprTaskGroupRef
	ReprTaggedUnion
	ReprJSValue
)

type Repr struct {
	Kind  ReprKind
	Shape ShapeID
}

func (r Repr) Proven() bool {
	return r.Kind != ReprUnproven
}
