package sema

import (
	"fmt"
	"strings"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
	"github.com/phongsathornpt/ts-pro/internal/support/diag"
	"github.com/phongsathornpt/ts-pro/internal/support/source"
)

type SymbolKind uint8

const (
	SymVar SymbolKind = iota
	SymParam
	SymFunc
	SymClass
	SymEnum
	SymInterface
	SymTypeAlias
)

type Symbol struct {
	Name string
	Kind SymbolKind
	Type types.Type
	Node ast.Node
}

type ClassInfo struct {
	Name         string
	Decl         *ast.ClassDecl
	Instance     *types.ObjectType
	Constructor  *types.FunctionType
	Methods      map[string]*types.FunctionType
	MethodOwners map[string]string
	BaseName     string
	TypeParams   []*types.TypeVar
	TypeBindings map[*types.TypeVar]types.Type
	GenericBase  string
	Resolved     bool
	Resolving    bool
}

type BuiltinCollectionInfo struct {
	Kind     string
	Instance *types.ObjectType
	Key      types.Type
	Value    types.Type
}

type Scope struct {
	Parent  *Scope
	Symbols map[string]*Symbol
}

func NewScope(parent *Scope) *Scope {
	return &Scope{
		Parent:  parent,
		Symbols: make(map[string]*Symbol),
	}
}

func (s *Scope) Define(sym *Symbol) error {
	if _, exists := s.Symbols[sym.Name]; exists {
		return fmt.Errorf("duplicate symbol %q", sym.Name)
	}
	s.Symbols[sym.Name] = sym
	return nil
}

func (s *Scope) Resolve(name string) *Symbol {
	if sym, exists := s.Symbols[name]; exists {
		return sym
	}
	if s.Parent != nil {
		return s.Parent.Resolve(name)
	}
	return nil
}

// Result holds the analyzed types and symbols for an AST.
type Result struct {
	Types               map[ast.Node]types.Type
	Symbols             map[ast.Node]*Symbol
	GenericCalls        map[*ast.CallExpr]*types.FunctionType
	GenericClasses      map[*ast.NewExpr]*ClassInfo
	Classes             map[string]*ClassInfo
	Enums               map[string]map[string]float64
	ImportAliases       map[string]string
	BuiltinCollections  map[string]*BuiltinCollectionInfo
	TaskResults         map[string]types.Type
	ChannelElements     map[string]types.Type
	TaskGroupType       *types.ObjectType
	AsyncResults        map[*ast.FunctionDecl]types.Type
	DateType            *types.ObjectType
	RegExpType          *types.ObjectType
	DOMExceptionType    *types.ObjectType
	EventType           *types.ObjectType
	CustomEventType     *types.ObjectType
	MessageEventType    *types.ObjectType
	ErrorEventType      *types.ObjectType
	EventTargetType     *types.ObjectType
	AbortSignalType     *types.ObjectType
	AbortControllerType *types.ObjectType
	ByteBufferType      *types.ObjectType
	ArrayBufferType     *types.ObjectType
	Uint8ArrayType      *types.ObjectType
	VarTypes            map[*ast.VarDeclStmt][]types.Type
	RootScope           *Scope
	Diagnostics         diag.DiagnosticList
}

type Checker struct {
	currentScope      *Scope
	result            *Result
	currentFnRet      types.Type
	currentClass      *ClassInfo
	currentThisType   types.Type
	typeParamEnvs     []map[string]*types.TypeVar
	genericClassSpecs map[string]*ClassInfo
	classSpecCount    int
	taskTypeCount     int
	channelTypeCount  int
}

func NewChecker() *Checker {
	root := NewScope(nil)
	return &Checker{
		currentScope: root,
		result: &Result{
			Types:              make(map[ast.Node]types.Type),
			Symbols:            make(map[ast.Node]*Symbol),
			GenericCalls:       make(map[*ast.CallExpr]*types.FunctionType),
			GenericClasses:     make(map[*ast.NewExpr]*ClassInfo),
			Classes:            make(map[string]*ClassInfo),
			Enums:              make(map[string]map[string]float64),
			ImportAliases:      make(map[string]string),
			VarTypes:           make(map[*ast.VarDeclStmt][]types.Type),
			BuiltinCollections: make(map[string]*BuiltinCollectionInfo),
			TaskResults:        make(map[string]types.Type),
			ChannelElements:    make(map[string]types.Type),
			AsyncResults:       make(map[*ast.FunctionDecl]types.Type),
			RootScope:          root,
			Diagnostics:        make(diag.DiagnosticList, 0),
		},
		genericClassSpecs: make(map[string]*ClassInfo),
	}
}

func (c *Checker) pushTypeParams(vars []*types.TypeVar) func() {
	env := make(map[string]*types.TypeVar, len(vars))
	for _, tv := range vars {
		env[tv.Name] = tv
	}
	c.typeParamEnvs = append(c.typeParamEnvs, env)
	return func() { c.typeParamEnvs = c.typeParamEnvs[:len(c.typeParamEnvs)-1] }
}

func (c *Checker) resolveTypeParam(name string) *types.TypeVar {
	for i := len(c.typeParamEnvs) - 1; i >= 0; i-- {
		if tv := c.typeParamEnvs[i][name]; tv != nil {
			return tv
		}
	}
	return nil
}

func newTypeParams(names []string) []*types.TypeVar {
	vars := make([]*types.TypeVar, len(names))
	for i, name := range names {
		vars[i] = types.NewTypeVar(name, nil)
	}
	return vars
}

func (c *Checker) error(span source.Span, code string, msg string) {
	c.result.Diagnostics = append(c.result.Diagnostics, diag.Diagnostic{
		Span:     span,
		Code:     code,
		Message:  msg,
		Severity: diag.SeverityError,
	})
}

// Check performs two-pass semantic analysis on the program.
func Check(prog *ast.Program) *Result {
	checker := NewChecker()
	checker.declareTopLevel(prog)
	checker.checkProgram(prog)
	return checker.result
}

func (c *Checker) declareTopLevel(prog *ast.Program) {
	// Predeclare class identities so fields/functions may reference classes that
	// appear later in the source file.
	for _, stmt := range prog.Statements {
		if enumDecl, ok := stmt.(*ast.EnumDecl); ok {
			c.result.Enums[enumDecl.Name] = make(map[string]float64)
			sym := &Symbol{Name: enumDecl.Name, Kind: SymEnum, Type: types.TypeNumber, Node: enumDecl}
			if err := c.currentScope.Define(sym); err != nil {
				c.error(enumDecl.Span(), "TS2300", err.Error())
			}
			c.result.Symbols[enumDecl] = sym
			c.result.Types[enumDecl] = types.TypeNumber
			continue
		}
		cls, ok := stmt.(*ast.ClassDecl)
		if !ok {
			continue
		}
		instance := types.NewObject(cls.Name)
		info := &ClassInfo{
			Name: cls.Name, Decl: cls, Instance: instance,
			Methods: make(map[string]*types.FunctionType), MethodOwners: make(map[string]string), BaseName: cls.Extends,
			TypeParams: newTypeParams(cls.TypeParams),
		}
		c.result.Classes[cls.Name] = info
		sym := &Symbol{Name: cls.Name, Kind: SymClass, Type: instance, Node: cls}
		if err := c.currentScope.Define(sym); err != nil {
			c.error(cls.Span(), "TS2300", err.Error())
		}
		c.result.Symbols[cls] = sym
		c.result.Types[cls] = instance
	}

	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.ImportDecl:
			for _, spec := range s.Specifiers {
				target := c.currentScope.Resolve(spec.Imported)
				if target == nil {
					c.error(s.Span(), "TS2305", fmt.Sprintf("Module %q has no exported member %q.", s.Module, spec.Imported))
					continue
				}
				if spec.Local == spec.Imported {
					continue
				}
				alias := &Symbol{Name: spec.Local, Kind: target.Kind, Type: target.Type, Node: s}
				if err := c.currentScope.Define(alias); err != nil {
					c.error(s.Span(), "TS2300", err.Error())
					continue
				}
				c.result.ImportAliases[spec.Local] = spec.Imported
			}
		case *ast.EnumDecl:
			value := float64(0)
			for _, member := range s.Members {
				if member.Value != nil {
					lit, ok := member.Value.(*ast.NumberLit)
					if !ok {
						c.error(member.Value.Span(), "TS1061", "Native enum member initializer must be a numeric literal.")
						continue
					}
					value = lit.Value
				}
				c.result.Enums[s.Name][member.Name] = value
				value++
			}
		case *ast.FunctionDecl:
			fnType := c.resolveFunctionType(s)
			sym := &Symbol{Name: s.Name, Kind: SymFunc, Type: fnType, Node: s}
			if err := c.currentScope.Define(sym); err != nil {
				c.error(s.Span(), "TS2300", err.Error())
			}
			c.result.Symbols[s] = sym
			c.result.Types[s] = fnType
		case *ast.InterfaceDecl:
			objType := types.NewObject(s.Name)
			for _, f := range s.Fields {
				objType.AddField(f.Name, c.resolveTypeNode(f.Type), f.Optional)
			}
			sym := &Symbol{Name: s.Name, Kind: SymInterface, Type: objType, Node: s}
			if err := c.currentScope.Define(sym); err != nil {
				c.error(s.Span(), "TS2300", err.Error())
			}
			c.result.Symbols[s] = sym
			c.result.Types[s] = objType
		case *ast.TypeAliasDecl:
			aliasType := c.resolveTypeNode(s.Type)
			sym := &Symbol{Name: s.Name, Kind: SymTypeAlias, Type: aliasType, Node: s}
			if err := c.currentScope.Define(sym); err != nil {
				c.error(s.Span(), "TS2300", err.Error())
			}
			c.result.Symbols[s] = sym
			c.result.Types[s] = aliasType
		case *ast.ClassDecl:
			c.resolveClassInfo(s)
		}
	}
}

func (c *Checker) resolveClassInfo(cls *ast.ClassDecl) {
	info := c.result.Classes[cls.Name]
	if info == nil || info.Resolved {
		return
	}
	if info.Resolving {
		c.error(cls.Span(), "TS2506", fmt.Sprintf("Class '%s' is referenced directly or indirectly in its own base expression.", cls.Name))
		return
	}
	info.Resolving = true
	defer func() { info.Resolving = false }()
	popTypeParams := c.pushTypeParams(info.TypeParams)
	defer popTypeParams()

	if info.BaseName != "" {
		base := c.result.Classes[info.BaseName]
		if base == nil {
			c.error(cls.Span(), "TS2304", fmt.Sprintf("Cannot find base class '%s'.", info.BaseName))
		} else {
			c.resolveClassInfo(base.Decl)
			for _, name := range base.Instance.FieldOrder {
				field := base.Instance.Fields[name]
				info.Instance.AddField(name, field.Type, field.Optional)
			}
			for name, method := range base.Methods {
				info.Methods[name] = method
				info.MethodOwners[name] = base.MethodOwners[name]
			}
		}
	}

	for _, field := range cls.Fields {
		if field.IsStatic {
			continue
		}
		ft := c.resolveTypeNode(field.Type)
		if ft == nil {
			ft = types.TypeAny
		}
		info.Instance.AddField(field.Name, ft, false)
	}
	for _, method := range cls.Methods {
		params := make([]types.Param, len(method.Params))
		for i, p := range method.Params {
			pt := c.resolveTypeNode(p.Type)
			if pt == nil {
				pt = types.TypeAny
			}
			params[i] = types.Param{Name: p.Name, Type: pt, Optional: p.Optional, Rest: p.Rest}
			if method.Name == "constructor" && p.IsParameterProperty {
				info.Instance.AddField(p.Name, pt, p.Optional)
			}
		}
		ret := c.resolveTypeNode(method.ReturnType)
		if ret == nil {
			ret = types.TypeVoid
		}
		ft := types.NewFunction(params, ret)
		if method.Name == "constructor" {
			info.Constructor = ft
		} else {
			info.Methods[method.Name] = ft
			info.MethodOwners[method.Name] = cls.Name
		}
	}
	if info.Constructor == nil {
		info.Constructor = types.NewFunction(nil, types.TypeVoid)
	}
	info.Resolved = true
}

func genericClassKey(info *ClassInfo, args []types.Type) string {
	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = arg.String()
	}
	return info.Name + "<" + strings.Join(parts, ",") + ">"
}

func (c *Checker) specializeClass(info *ClassInfo, args []types.Type) *ClassInfo {
	key := genericClassKey(info, args)
	if spec := c.genericClassSpecs[key]; spec != nil {
		return spec
	}
	bindings := make(map[*types.TypeVar]types.Type, len(args))
	for i, tp := range info.TypeParams {
		bindings[tp] = args[i]
	}
	instance := types.Substitute(info.Instance, bindings).(*types.ObjectType)
	name := fmt.Sprintf("%s$spec%d", info.Name, c.classSpecCount)
	c.classSpecCount++
	instance.Name = name
	ctor, _ := types.Substitute(info.Constructor, bindings).(*types.FunctionType)
	spec := &ClassInfo{
		Name: name, Decl: info.Decl, Instance: instance, Constructor: ctor,
		Methods: make(map[string]*types.FunctionType, len(info.Methods)), MethodOwners: make(map[string]string, len(info.Methods)),
		BaseName: info.BaseName, TypeBindings: bindings, GenericBase: info.Name, Resolved: true,
	}
	for method, fn := range info.Methods {
		concrete := types.Substitute(fn, bindings).(*types.FunctionType)
		spec.Methods[method] = concrete
		owner := info.MethodOwners[method]
		if owner == info.Name || owner == "" {
			owner = name
		}
		spec.MethodOwners[method] = owner
	}
	c.genericClassSpecs[key] = spec
	c.result.Classes[name] = spec
	return spec
}

func (c *Checker) checkProgram(prog *ast.Program) {
	for _, stmt := range prog.Statements {
		c.checkStatement(stmt)
	}
}

func (c *Checker) lookupMemberType(objType types.Type, property string) (types.Type, bool) {
	if objType == types.TypeAny || objType == types.TypeUnknown {
		return types.TypeAny, true
	}
	switch t := objType.(type) {
	case *types.TupleType:
		if property == "length" {
			return types.TypeNumber, true
		}
	case *types.ArrayType:
		switch property {
		case "length":
			return types.TypeNumber, true
		case "push":
			return types.NewFunction([]types.Param{{Name: "value", Type: t.Elem}}, types.TypeNumber), true
		case "pop":
			return types.NewFunction(nil, t.Elem), true
		}
	case *types.ObjectType:
		if t.Name == "$ArrayBuffer" {
			return c.builtinArrayBufferMember(property)
		}
		if t.Name == "$Uint8Array" {
			return c.builtinUint8ArrayMember(property)
		}
		if t.Name == "$Event" {
			return c.builtinEventMember(property)
		}
		if t.Name == "$CustomEvent" {
			if property == "detail" {
				return types.TypeAny, true
			}
			return c.builtinEventMember(property)
		}
		if t.Name == "$MessageEvent" {
			switch property {
			case "data", "source", "ports":
				return types.TypeAny, true
			case "origin", "lastEventId":
				return types.TypeString, true
			}
			return c.builtinEventMember(property)
		}
		if t.Name == "$ErrorEvent" {
			switch property {
			case "message", "filename":
				return types.TypeString, true
			case "lineno", "colno":
				return types.TypeNumber, true
			case "error":
				return types.TypeAny, true
			}
			return c.builtinEventMember(property)
		}
		if t.Name == "$EventTarget" {
			return c.builtinEventTargetMember(property)
		}
		if t.Name == "$AbortSignal" {
			return c.builtinAbortSignalMember(property)
		}
		if t.Name == "$AbortController" {
			return c.builtinAbortControllerMember(property)
		}
		if t.Name == "$AbortSignalConstructor" {
			return c.builtinAbortSignalStaticMember(property)
		}
		if t.Name == "$Date" {
			if member, ok := c.builtinDateMember(property); ok {
				return member, true
			}
		}
		if t.Name == "$RegExp" {
			switch property {
			case "test":
				return types.NewFunction([]types.Param{{Name: "text", Type: types.TypeString}}, types.TypeBoolean), true
			case "source":
				return types.TypeString, true
			}
		}
		if t.Name == "$DateConstructor" && property == "now" {
			return types.NewFunction(nil, types.TypeNumber), true
		}
		if t.Name == "$JSON" {
			switch property {
			case "parse":
				return types.NewFunction([]types.Param{{Name: "text", Type: types.TypeString}}, types.TypeAny), true
			case "stringify":
				return types.NewFunction([]types.Param{{Name: "value", Type: types.TypeAny}}, types.TypeString), true
			}
		}
		if builtin := c.result.BuiltinCollections[t.Name]; builtin != nil {
			if member, ok := c.builtinCollectionMember(builtin, property); ok {
				return member, true
			}
		}
		if info := c.result.Classes[t.Name]; info != nil {
			if method := info.Methods[property]; method != nil {
				return method, true
			}
		}
		if field, ok := t.Fields[property]; ok {
			if field.Optional {
				return types.NewUnion(field.Type, types.TypeUndefined), true
			}
			return field.Type, true
		}
	case *types.UnionType:
		members := make([]types.Type, 0, len(t.Members))
		for _, member := range t.Members {
			mt, ok := c.lookupMemberType(member, property)
			if !ok {
				return nil, false
			}
			members = append(members, mt)
		}
		if len(members) > 0 {
			return types.NewUnion(members...), true
		}
	}
	return nil, false
}

func (c *Checker) resolveFunctionType(fn *ast.FunctionDecl) *types.FunctionType {
	typeParams := newTypeParams(fn.TypeParams)
	pop := c.pushTypeParams(typeParams)
	defer pop()
	var params []types.Param
	for _, p := range fn.Params {
		pType := c.resolveTypeNode(p.Type)
		if pType == nil {
			pType = types.TypeAny
		}
		if p.Optional {
			pType = types.NewUnion(pType, types.TypeUndefined)
		}
		if p.Rest {
			if _, ok := pType.(*types.ArrayType); !ok {
				c.error(p.SourceSpan, "TS2370", "A rest parameter must be of an array type.")
			}
		}
		params = append(params, types.Param{
			Name:     p.Name,
			Type:     pType,
			Optional: p.Optional,
			Rest:     p.Rest,
		})
	}
	var retType types.Type
	if fn.IsAsync {
		inner := types.TypeAny
		if ref, ok := fn.ReturnType.(*ast.TypeRefNode); ok && ref.Name == "Promise" && len(ref.TypeArgs) == 1 {
			inner = c.resolveTypeNode(ref.TypeArgs[0])
		} else {
			c.error(fn.Span(), "TS1064", "An async function return type must be Promise<T>.")
		}
		name := fmt.Sprintf("$Task$async$%d", c.taskTypeCount)
		c.taskTypeCount++
		taskType := types.NewObject(name)
		c.result.TaskResults[name] = inner
		c.result.AsyncResults[fn] = inner
		retType = taskType
	} else {
		retType = c.resolveTypeNode(fn.ReturnType)
		if retType == nil {
			retType = types.TypeVoid
		}
	}
	return types.NewGenericFunction(typeParams, params, retType)
}

func (c *Checker) resolveTypeNode(node ast.TypeNode) types.Type {
	if node == nil {
		return nil
	}
	switch t := node.(type) {
	case *ast.PrimitiveTypeNode:
		switch t.Kind {
		case "number":
			return types.TypeNumber
		case "string":
			return types.TypeString
		case "boolean":
			return types.TypeBoolean
		case "void":
			return types.TypeVoid
		case "any":
			return types.TypeAny
		case "never":
			return types.TypeNever
		case "unknown":
			return types.TypeUnknown
		case "undefined":
			return types.TypeUndefined
		default:
			return types.TypeNull
		}
	case *ast.TypeRefNode:
		if t.Name == "ArrayBuffer" {
			return c.builtinArrayBufferType()
		}
		if t.Name == "Uint8Array" {
			return c.builtinUint8ArrayType()
		}
		if t.Name == "Event" {
			return c.builtinEventType()
		}
		if t.Name == "CustomEvent" {
			return c.builtinCustomEventType()
		}
		if t.Name == "MessageEvent" {
			return c.builtinMessageEventType()
		}
		if t.Name == "ErrorEvent" {
			return c.builtinErrorEventType()
		}
		if t.Name == "EventTarget" {
			return c.builtinEventTargetType()
		}
		if t.Name == "AbortSignal" {
			return c.builtinAbortSignalType()
		}
		if t.Name == "AbortController" {
			return c.builtinAbortControllerType()
		}
		if t.Name == "DOMException" {
			return c.builtinDOMExceptionType()
		}
		if t.Name == "Promise" || t.Name == "PromiseLike" {
			return c.newPromiseType(c.resolveTypeNode(t.TypeArgs[0]))
		}
		if tv := c.resolveTypeParam(t.Name); tv != nil {
			return tv
		}
		if t.Name == "Array" && len(t.TypeArgs) == 1 {
			return types.NewArray(c.resolveTypeNode(t.TypeArgs[0]))
		}
		sym := c.currentScope.Resolve(t.Name)
		if sym != nil && (sym.Kind == SymInterface || sym.Kind == SymClass || sym.Kind == SymTypeAlias || sym.Kind == SymEnum) {
			return sym.Type
		}
		return types.TypeAny
	case *ast.ArrayTypeNode:
		elem := c.resolveTypeNode(t.ElemType)
		return types.NewArray(elem)
	case *ast.TupleTypeNode:
		elems := make([]types.Type, len(t.Elements))
		for i, elem := range t.Elements {
			elems[i] = c.resolveTypeNode(elem)
		}
		return types.NewTuple(elems...)
	case *ast.ObjectTypeNode:
		obj := types.NewObject("")
		for _, field := range t.Fields {
			obj.AddField(field.Name, c.resolveTypeNode(field.Type), field.Optional)
		}
		return obj
	case *ast.UnionTypeNode:
		var members []types.Type
		for _, m := range t.Types {
			members = append(members, c.resolveTypeNode(m))
		}
		return types.NewUnion(members...)
	default:
		ftn := node.(*ast.FunctionTypeNode)
		params := make([]types.Param, 0, len(ftn.Params))
		var thisType types.Type
		for _, p := range ftn.Params {
			pt := c.resolveTypeNode(p.Type)
			if pt == nil {
				pt = types.TypeAny
			}
			if p.IsThis {
				thisType = pt
				continue
			}
			params = append(params, types.Param{Name: p.Name, Type: pt, Optional: p.Optional, Rest: p.Rest})
		}
		ret := c.resolveTypeNode(ftn.ReturnType)
		fn := types.NewFunction(params, ret)
		fn.This = thisType
		return fn
	}
}
