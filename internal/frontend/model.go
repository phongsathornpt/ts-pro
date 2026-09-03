package frontend

type SourceID uint32
type SymbolID uint32
type TypeID uint32
type FunctionID uint32
type ShapeID uint32

type Position struct {
	Line      uint32
	Character uint32
}

type Span struct {
	Source SourceID
	Start  Position
	End    Position
}

type Source struct {
	ID      SourceID
	URI     string
	Path    string
	Version int32
}

type SymbolKind uint8

const (
	SymbolInvalid SymbolKind = iota
	SymbolVariable
	SymbolParameter
	SymbolFunction
	SymbolMethod
	SymbolProperty
	SymbolClass
	SymbolInterface
	SymbolTypeAlias
)

type Symbol struct {
	ID       SymbolID
	Name     string
	Kind     SymbolKind
	Type     TypeID
	Decl     Span
	Exported bool
}

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
	TypeMap
	TypeSet
	TypeDate
	TypeRegExp
)

type Type struct {
	ID         TypeID
	Kind       TypeKind
	Name       string
	Element    TypeID
	Members    []TypeID
	Params     []TypeID
	ReturnType TypeID
	Properties []SymbolID
	Shape      ShapeID
}

type ShapeField struct {
	Name string
	Type TypeID
}

type Shape struct {
	ID       ShapeID
	Name     string
	ClassTag uint32
	Fields   []ShapeField
}

type Parameter struct {
	Symbol      SymbolID
	Name        string
	Type        TypeID
	Span        Span
	Optional    bool
	Rest        bool
	Initializer *Expr
}

type Function struct {
	ID              FunctionID
	Symbol          SymbolID
	Name            string
	Source          SourceID
	Span            Span
	Params          []Parameter
	ReturnType      TypeID
	Async           bool
	Generator       bool
	Exported        bool
	HasExplicitThis bool
	Body            []Statement
}

type Snapshot struct {
	ProjectVersion string
	Sources        []Source
	Symbols        []Symbol
	Types          []Type
	Functions      []Function
	Shapes         []Shape
	Entry          []Statement
}

func (s Snapshot) Source(id SourceID) (Source, bool) {
	if int(id) >= len(s.Sources) {
		return Source{}, false
	}
	return s.Sources[id], true
}

func (s Snapshot) Type(id TypeID) (Type, bool) {
	if int(id) >= len(s.Types) {
		return Type{}, false
	}
	return s.Types[id], true
}
