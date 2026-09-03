package types

import (
	"fmt"
	"strings"
	"sync/atomic"
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

// --- Type Variables ---

var nextTypeVarID atomic.Uint64

type TypeVar struct {
	ID         uint64
	Name       string
	Constraint Type
}

func NewTypeVar(name string, constraint Type) *TypeVar {
	return &TypeVar{ID: nextTypeVarID.Add(1), Name: name, Constraint: constraint}
}

func (t *TypeVar) Kind() TypeKind { return KindTypeVar }
func (t *TypeVar) String() string { return t.Name }
func (t *TypeVar) Equals(other Type) bool {
	o, ok := other.(*TypeVar)
	return ok && t.ID == o.ID
}
func (t *TypeVar) AssignableTo(target Type) bool {
	if target == nil {
		return false
	}
	if target.Kind() == KindAny || target.Kind() == KindUnknown || t.Equals(target) {
		return true
	}
	if u, ok := target.(*UnionType); ok {
		return u.ContainsAssignable(t)
	}
	if t.Constraint != nil {
		return t.Constraint.AssignableTo(target)
	}
	return false
}

// --- Tuple Type ---

type TupleType struct {
	Elements []Type
}

func NewTuple(elements ...Type) *TupleType {
	return &TupleType{Elements: append([]Type(nil), elements...)}
}

func (t *TupleType) Kind() TypeKind { return KindTuple }
func (t *TupleType) String() string {
	parts := make([]string, len(t.Elements))
	for i, elem := range t.Elements {
		parts[i] = elem.String()
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
func (t *TupleType) Equals(other Type) bool {
	o, ok := other.(*TupleType)
	if !ok || len(t.Elements) != len(o.Elements) {
		return false
	}
	for i := range t.Elements {
		if !t.Elements[i].Equals(o.Elements[i]) {
			return false
		}
	}
	return true
}
func (t *TupleType) AssignableTo(target Type) bool {
	if target == nil {
		return false
	}
	if target.Kind() == KindAny || target.Kind() == KindUnknown {
		return true
	}
	if o, ok := target.(*TupleType); ok {
		if len(t.Elements) != len(o.Elements) {
			return false
		}
		for i := range t.Elements {
			if !t.Elements[i].AssignableTo(o.Elements[i]) {
				return false
			}
		}
		return true
	}
	if a, ok := target.(*ArrayType); ok {
		for _, elem := range t.Elements {
			if !elem.AssignableTo(a.Elem) {
				return false
			}
		}
		return true
	}
	if u, ok := target.(*UnionType); ok {
		return u.ContainsAssignable(t)
	}
	return false
}

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
	Rest     bool
}

type FunctionType struct {
	TypeParams []*TypeVar
	This       Type
	Params     []Param
	Return     Type
}

func NewFunction(params []Param, ret Type) *FunctionType {
	return &FunctionType{Params: params, Return: ret}
}

func NewGenericFunction(typeParams []*TypeVar, params []Param, ret Type) *FunctionType {
	return &FunctionType{TypeParams: append([]*TypeVar(nil), typeParams...), Params: params, Return: ret}
}

func (f *FunctionType) Kind() TypeKind { return KindFunction }
func (f *FunctionType) String() string {
	var parts []string
	if f.This != nil {
		parts = append(parts, fmt.Sprintf("this: %s", f.This.String()))
	}
	for _, p := range f.Params {
		opt := ""
		if p.Optional {
			opt = "?"
		}
		rest := ""
		if p.Rest {
			rest = "..."
		}
		parts = append(parts, fmt.Sprintf("%s%s%s: %s", rest, p.Name, opt, p.Type.String()))
	}
	typeParams := ""
	if len(f.TypeParams) > 0 {
		names := make([]string, len(f.TypeParams))
		for i, tp := range f.TypeParams {
			names[i] = tp.String()
		}
		typeParams = "<" + strings.Join(names, ", ") + ">"
	}
	return fmt.Sprintf("%s(%s) => %s", typeParams, strings.Join(parts, ", "), f.Return.String())
}
func (f *FunctionType) Equals(other Type) bool {
	o, ok := other.(*FunctionType)
	if !ok || len(f.TypeParams) != len(o.TypeParams) || len(f.Params) != len(o.Params) || !f.Return.Equals(o.Return) {
		return false
	}
	if (f.This == nil) != (o.This == nil) {
		return false
	}
	if f.This != nil && !f.This.Equals(o.This) {
		return false
	}
	for i := range f.TypeParams {
		if !f.TypeParams[i].Equals(o.TypeParams[i]) {
			return false
		}
	}
	for i := range f.Params {
		if f.Params[i].Rest != o.Params[i].Rest || !f.Params[i].Type.Equals(o.Params[i].Type) {
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
		if o.This != nil {
			if f.This == nil || !o.This.AssignableTo(f.This) {
				return false
			}
		}
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

// Substitute recursively replaces type variables using declaration-identity bindings.
// Types without bound variables are returned unchanged where practical.
func Substitute(t Type, bindings map[*TypeVar]Type) Type {
	if t == nil || len(bindings) == 0 {
		return t
	}
	switch v := t.(type) {
	case *TypeVar:
		if replacement, ok := bindings[v]; ok {
			return replacement
		}
		return v
	case *ArrayType:
		return NewArray(Substitute(v.Elem, bindings))
	case *TupleType:
		elems := make([]Type, len(v.Elements))
		for i, elem := range v.Elements {
			elems[i] = Substitute(elem, bindings)
		}
		return NewTuple(elems...)
	case *UnionType:
		members := make([]Type, len(v.Members))
		for i, member := range v.Members {
			members[i] = Substitute(member, bindings)
		}
		return NewUnion(members...)
	case *ObjectType:
		out := NewObject(v.Name)
		for _, name := range v.FieldOrder {
			field := v.Fields[name]
			out.AddField(name, Substitute(field.Type, bindings), field.Optional)
		}
		return out
	case *FunctionType:
		params := make([]Param, len(v.Params))
		for i, param := range v.Params {
			params[i] = param
			params[i].Type = Substitute(param.Type, bindings)
		}
		ret := Substitute(v.Return, bindings)
		thisType := Substitute(v.This, bindings)
		remaining := make([]*TypeVar, 0, len(v.TypeParams))
		for _, tp := range v.TypeParams {
			if _, bound := bindings[tp]; !bound {
				remaining = append(remaining, tp)
			}
		}
		out := NewGenericFunction(remaining, params, ret)
		out.This = thisType
		return out
	default:
		return t
	}
}

func InstantiateFunction(fn *FunctionType, args []Type) (*FunctionType, error) {
	if fn == nil {
		return nil, fmt.Errorf("cannot instantiate nil function type")
	}
	if len(fn.TypeParams) != len(args) {
		return nil, fmt.Errorf("generic function expects %d type arguments, got %d", len(fn.TypeParams), len(args))
	}
	bindings := make(map[*TypeVar]Type, len(args))
	for i, tp := range fn.TypeParams {
		arg := args[i]
		if arg == nil {
			return nil, fmt.Errorf("type argument %d is nil", i)
		}
		if tp.Constraint != nil && !arg.AssignableTo(tp.Constraint) {
			return nil, fmt.Errorf("type argument %s does not satisfy constraint %s", arg, tp.Constraint)
		}
		bindings[tp] = arg
	}
	instantiated, ok := Substitute(fn, bindings).(*FunctionType)
	if !ok {
		return nil, fmt.Errorf("generic function substitution produced %T", instantiated)
	}
	instantiated.TypeParams = nil
	return instantiated, nil
}

func inferTypeBindings(pattern, actual Type, bindings map[*TypeVar]Type) error {
	if pattern == nil || actual == nil {
		return fmt.Errorf("cannot infer from nil type")
	}
	switch p := pattern.(type) {
	case *TypeVar:
		if existing, ok := bindings[p]; ok {
			if existing.Equals(actual) {
				return nil
			}
			return fmt.Errorf("conflicting inferences for %s: %s and %s", p, existing, actual)
		}
		if p.Constraint != nil && !actual.AssignableTo(p.Constraint) {
			return fmt.Errorf("inferred type %s does not satisfy constraint %s for %s", actual, p.Constraint, p)
		}
		bindings[p] = actual
		return nil
	case *ArrayType:
		a, ok := actual.(*ArrayType)
		if !ok {
			return nil
		}
		return inferTypeBindings(p.Elem, a.Elem, bindings)
	case *TupleType:
		a, ok := actual.(*TupleType)
		if !ok || len(p.Elements) != len(a.Elements) {
			return nil
		}
		for i := range p.Elements {
			if err := inferTypeBindings(p.Elements[i], a.Elements[i], bindings); err != nil {
				return err
			}
		}
		return nil
	case *FunctionType:
		a, ok := actual.(*FunctionType)
		if !ok || len(p.Params) != len(a.Params) {
			return nil
		}
		for i := range p.Params {
			if err := inferTypeBindings(p.Params[i].Type, a.Params[i].Type, bindings); err != nil {
				return err
			}
		}
		return inferTypeBindings(p.Return, a.Return, bindings)
	case *ObjectType:
		a, ok := actual.(*ObjectType)
		if !ok {
			return nil
		}
		for name, field := range p.Fields {
			actualField, exists := a.Fields[name]
			if !exists {
				continue
			}
			if err := inferTypeBindings(field.Type, actualField.Type, bindings); err != nil {
				return err
			}
		}
		return nil
	default:
		return nil
	}
}

// InferFunction infers a generic function's type arguments from actual argument
// types and returns a concrete instantiation. It deliberately rejects unresolved
// or conflicting bindings rather than silently widening them to any.
func InferFunction(fn *FunctionType, actualArgs []Type) (*FunctionType, error) {
	if fn == nil {
		return nil, fmt.Errorf("cannot infer nil function type")
	}
	if len(fn.TypeParams) == 0 {
		return fn, nil
	}
	bindings := make(map[*TypeVar]Type, len(fn.TypeParams))
	limit := len(actualArgs)
	if len(fn.Params) < limit {
		limit = len(fn.Params)
	}
	for i := 0; i < limit; i++ {
		if err := inferTypeBindings(fn.Params[i].Type, actualArgs[i], bindings); err != nil {
			return nil, err
		}
	}
	args := make([]Type, len(fn.TypeParams))
	for i, tp := range fn.TypeParams {
		arg, ok := bindings[tp]
		if !ok {
			return nil, fmt.Errorf("could not infer type argument for %s", tp)
		}
		args[i] = arg
	}
	return InstantiateFunction(fn, args)
}

// FunctionBindings reconstructs declaration type-variable bindings from a
// concrete function instantiation. It is used by monomorphizing backends to
// substitute semantic types throughout the generic function body.
func FunctionBindings(generic, concrete *FunctionType) (map[*TypeVar]Type, error) {
	if generic == nil || concrete == nil {
		return nil, fmt.Errorf("cannot bind nil function types")
	}
	if len(generic.TypeParams) == 0 {
		return map[*TypeVar]Type{}, nil
	}
	if len(generic.Params) != len(concrete.Params) {
		return nil, fmt.Errorf("generic/concrete parameter count mismatch: %d != %d", len(generic.Params), len(concrete.Params))
	}
	bindings := make(map[*TypeVar]Type, len(generic.TypeParams))
	for i := range generic.Params {
		if err := inferTypeBindings(generic.Params[i].Type, concrete.Params[i].Type, bindings); err != nil {
			return nil, err
		}
	}
	if err := inferTypeBindings(generic.Return, concrete.Return, bindings); err != nil {
		return nil, err
	}
	for _, tp := range generic.TypeParams {
		if _, ok := bindings[tp]; !ok {
			return nil, fmt.Errorf("concrete function does not bind type parameter %s", tp)
		}
	}
	return bindings, nil
}
