package types

import (
	"fmt"
	"strings"
)

type TypeKind uint8

const (
	KindInvalid TypeKind = iota
	KindAny
	KindUnknown
	KindNever
	KindVoid
	KindUndefined
	KindNull
	KindBoolean
	KindNumber
	KindString
	KindObject
	KindArray
	KindTuple
	KindFunction
	KindUnion
	KindTypeVar
)

type Type interface {
	Kind() TypeKind
	String() string
	Equals(other Type) bool
	AssignableTo(target Type) bool
}

// --- Primitive Singletons ---

type primitiveType struct {
	kind TypeKind
	name string
}

func (p *primitiveType) Kind() TypeKind { return p.kind }
func (p *primitiveType) String() string { return p.name }
func (p *primitiveType) Equals(other Type) bool {
	if other == nil {
		return false
	}
	return p.kind == other.Kind()
}
func (p *primitiveType) AssignableTo(target Type) bool {
	if target == nil {
		return false
	}
	if target.Kind() == KindAny || target.Kind() == KindUnknown {
		return true
	}
	if p.kind == KindNever {
		return true
	}
	if p.kind == KindAny {
		return true
	}
	if u, ok := target.(*UnionType); ok {
		return u.ContainsAssignable(p)
	}
	return p.kind == target.Kind()
}

var (
	TypeAny       Type = &primitiveType{kind: KindAny, name: "any"}
	TypeUnknown   Type = &primitiveType{kind: KindUnknown, name: "unknown"}
	TypeNever     Type = &primitiveType{kind: KindNever, name: "never"}
	TypeVoid      Type = &primitiveType{kind: KindVoid, name: "void"}
	TypeUndefined Type = &primitiveType{kind: KindUndefined, name: "undefined"}
	TypeNull      Type = &primitiveType{kind: KindNull, name: "null"}
	TypeBoolean   Type = &primitiveType{kind: KindBoolean, name: "boolean"}
	TypeNumber    Type = &primitiveType{kind: KindNumber, name: "number"}
	TypeString    Type = &primitiveType{kind: KindString, name: "string"}
)

// --- Array Type ---

type ArrayType struct {
	Elem Type
}

func NewArray(elem Type) *ArrayType {
	return &ArrayType{Elem: elem}
}

func (a *ArrayType) Kind() TypeKind { return KindArray }
func (a *ArrayType) String() string {
	return fmt.Sprintf("%s[]", a.Elem.String())
}
func (a *ArrayType) Equals(other Type) bool {
	if o, ok := other.(*ArrayType); ok {
		return a.Elem.Equals(o.Elem)
	}
	return false
}
func (a *ArrayType) AssignableTo(target Type) bool {
	if target == nil {
		return false
	}
	if target.Kind() == KindAny || target.Kind() == KindUnknown {
		return true
	}
	if o, ok := target.(*ArrayType); ok {
		return a.Elem.AssignableTo(o.Elem)
	}
	if u, ok := target.(*UnionType); ok {
		return u.ContainsAssignable(a)
	}
	return false
}

// --- Object Type (Shapes) ---

type Field struct {
	Name     string
	Type     Type
	Optional bool
}

type ObjectType struct {
	Name       string // Optional shape name (e.g., interface/class name)
	Fields     map[string]Field
	FieldOrder []string
}

func NewObject(name string) *ObjectType {
	return &ObjectType{
		Name:       name,
		Fields:     make(map[string]Field),
		FieldOrder: make([]string, 0),
	}
}

func (o *ObjectType) AddField(name string, t Type, optional bool) {
	if _, exists := o.Fields[name]; !exists {
		o.FieldOrder = append(o.FieldOrder, name)
	}
	o.Fields[name] = Field{Name: name, Type: t, Optional: optional}
}

func (o *ObjectType) Kind() TypeKind { return KindObject }
func (o *ObjectType) String() string {
	if o.Name != "" {
		return o.Name
	}
	var sb strings.Builder
	sb.WriteString("{ ")
	for i, fName := range o.FieldOrder {
		if i > 0 {
			sb.WriteString("; ")
		}
		f := o.Fields[fName]
		opt := ""
		if f.Optional {
			opt = "?"
		}
		sb.WriteString(fmt.Sprintf("%s%s: %s", f.Name, opt, f.Type.String()))
	}
	sb.WriteString(" }")
	return sb.String()
}
func (o *ObjectType) Equals(other Type) bool {
	t, ok := other.(*ObjectType)
	if !ok || len(o.Fields) != len(t.Fields) {
		return false
	}
	for name, f1 := range o.Fields {
		f2, exists := t.Fields[name]
		if !exists || f1.Optional != f2.Optional || !f1.Type.Equals(f2.Type) {
			return false
		}
	}
	return true
}
func (o *ObjectType) AssignableTo(target Type) bool {
	if target == nil {
		return false
	}
	if target.Kind() == KindAny || target.Kind() == KindUnknown {
		return true
	}
	if t, ok := target.(*ObjectType); ok {
		// Structural subtyping: target must be subset of source
		for name, targetField := range t.Fields {
			sourceField, exists := o.Fields[name]
			if !exists {
				if !targetField.Optional {
					return false
				}
				continue
			}
			if !sourceField.Type.AssignableTo(targetField.Type) {
				return false
			}
		}
		return true
	}
	if u, ok := target.(*UnionType); ok {
		return u.ContainsAssignable(o)
	}
	return false
}

// --- Function Type ---

type Param struct {
	Name     string
	Type     Type
	Optional bool
}

type FunctionType struct {
	Params []Param
	Return Type
}

func NewFunction(params []Param, ret Type) *FunctionType {
	return &FunctionType{Params: params, Return: ret}
}

func (f *FunctionType) Kind() TypeKind { return KindFunction }
func (f *FunctionType) String() string {
	var parts []string
	for _, p := range f.Params {
		opt := ""
		if p.Optional {
			opt = "?"
		}
		parts = append(parts, fmt.Sprintf("%s%s: %s", p.Name, opt, p.Type.String()))
	}
	return fmt.Sprintf("(%s) => %s", strings.Join(parts, ", "), f.Return.String())
}
func (f *FunctionType) Equals(other Type) bool {
	o, ok := other.(*FunctionType)
	if !ok || len(f.Params) != len(o.Params) || !f.Return.Equals(o.Return) {
		return false
	}
	for i := range f.Params {
		if !f.Params[i].Type.Equals(o.Params[i].Type) {
			return false
		}
	}
	return true
}
func (f *FunctionType) AssignableTo(target Type) bool {
	if target == nil {
		return false
	}
	if target.Kind() == KindAny || target.Kind() == KindUnknown {
		return true
	}
	if o, ok := target.(*FunctionType); ok {
		if !f.Return.AssignableTo(o.Return) {
			return false
		}
		// Contravariant parameter check: target param assignable to source param
		if len(o.Params) < len(f.Params) {
			return false
		}
		for i := range f.Params {
			if !o.Params[i].Type.AssignableTo(f.Params[i].Type) {
				return false
			}
		}
		return true
	}
	if u, ok := target.(*UnionType); ok {
		return u.ContainsAssignable(f)
	}
	return false
}

// --- Union Type ---

type UnionType struct {
	Members []Type
}

func NewUnion(members ...Type) Type {
	var flat []Type
	for _, m := range members {
		if m.Kind() == KindNever {
			continue
		}
		if u, ok := m.(*UnionType); ok {
			flat = append(flat, u.Members...)
		} else {
			// Deduplicate
			duplicate := false
			for _, existing := range flat {
				if existing.Equals(m) {
					duplicate = true
					break
				}
			}
			if !duplicate {
				flat = append(flat, m)
			}
		}
	}
	if len(flat) == 0 {
		return TypeNever
	}
	if len(flat) == 1 {
		return flat[0]
	}
	return &UnionType{Members: flat}
}

func (u *UnionType) Kind() TypeKind { return KindUnion }
func (u *UnionType) String() string {
	var parts []string
	for _, m := range u.Members {
		parts = append(parts, m.String())
	}
	return strings.Join(parts, " | ")
}
func (u *UnionType) Equals(other Type) bool {
	o, ok := other.(*UnionType)
	if !ok || len(u.Members) != len(o.Members) {
		return false
	}
	for _, m1 := range u.Members {
		found := false
		for _, m2 := range o.Members {
			if m1.Equals(m2) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func (u *UnionType) AssignableTo(target Type) bool {
	if target == nil {
		return false
	}
	if target.Kind() == KindAny || target.Kind() == KindUnknown {
		return true
	}
	// Every member of union must be assignable to target
	for _, m := range u.Members {
		if !m.AssignableTo(target) {
			return false
		}
	}
	return true
}

func (u *UnionType) ContainsAssignable(src Type) bool {
	for _, m := range u.Members {
		if src.AssignableTo(m) {
			return true
		}
	}
	return false
}
