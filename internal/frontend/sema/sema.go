package sema

import (
	"fmt"
	"strings"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/token"
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
	Types              map[ast.Node]types.Type
	Symbols            map[ast.Node]*Symbol
	GenericCalls       map[*ast.CallExpr]*types.FunctionType
	GenericClasses     map[*ast.NewExpr]*ClassInfo
	Classes            map[string]*ClassInfo
	Enums              map[string]map[string]float64
	ImportAliases      map[string]string
	BuiltinCollections map[string]*BuiltinCollectionInfo
	TaskResults        map[string]types.Type
	ChannelElements    map[string]types.Type
	TaskGroupType      *types.ObjectType
	DateType           *types.ObjectType
	RegExpType         *types.ObjectType
	VarTypes           map[*ast.VarDeclStmt][]types.Type
	RootScope          *Scope
	Diagnostics        diag.DiagnosticList
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
				owner := base.MethodOwners[name]
				if owner == "" {
					owner = base.Name
				}
				info.MethodOwners[name] = owner
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

func (c *Checker) specializeClass(info *ClassInfo, args []types.Type) (*ClassInfo, error) {
	if info == nil {
		return nil, fmt.Errorf("cannot specialize nil class")
	}
	if len(info.TypeParams) != len(args) {
		return nil, fmt.Errorf("generic class '%s' expects %d type arguments, got %d", info.Name, len(info.TypeParams), len(args))
	}
	key := genericClassKey(info, args)
	if spec := c.genericClassSpecs[key]; spec != nil {
		return spec, nil
	}
	bindings := make(map[*types.TypeVar]types.Type, len(args))
	for i, tp := range info.TypeParams {
		bindings[tp] = args[i]
	}
	instance, ok := types.Substitute(info.Instance, bindings).(*types.ObjectType)
	if !ok {
		return nil, fmt.Errorf("generic class %s instance substitution produced %T", info.Name, instance)
	}
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
		concrete, ok := types.Substitute(fn, bindings).(*types.FunctionType)
		if !ok {
			return nil, fmt.Errorf("generic class %s method %s substitution produced %T", info.Name, method, concrete)
		}
		spec.Methods[method] = concrete
		owner := info.MethodOwners[method]
		if owner == info.Name || owner == "" {
			owner = name
		}
		spec.MethodOwners[method] = owner
	}
	c.genericClassSpecs[key] = spec
	c.result.Classes[name] = spec
	return spec, nil
}

func (c *Checker) checkProgram(prog *ast.Program) {
	for _, stmt := range prog.Statements {
		c.checkStatement(stmt)
	}
}

func (c *Checker) checkStatement(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.ImportDecl:
		// Resolved during top-level declaration after dependency flattening.
	case *ast.VarDeclStmt:
		c.checkVarDecl(s)
	case *ast.FunctionDecl:
		c.checkFunctionDecl(s)
	case *ast.ClassDecl:
		c.checkClassDecl(s)
	case *ast.EnumDecl:
		// Numeric enum members are resolved during declaration.
	case *ast.BlockStmt:
		c.checkBlock(s)
	case *ast.IfStmt:
		c.checkIf(s)
	case *ast.WhileStmt:
		c.checkWhile(s)
	case *ast.DoWhileStmt:
		c.checkDoWhile(s)
	case *ast.ForStmt:
		c.checkFor(s)
	case *ast.ForOfStmt:
		c.checkForOf(s)
	case *ast.SwitchStmt:
		c.checkSwitch(s)
	case *ast.BreakStmt, *ast.ContinueStmt:
		// Control-flow legality is enforced during native lowering for now.
	case *ast.ReturnStmt:
		c.checkReturn(s)
	case *ast.ExprStmt:
		c.checkExpr(s.Expr)
	}
}

func removeNullishType(t types.Type) types.Type {
	if t == nil {
		return nil
	}
	if t == types.TypeNull || t == types.TypeUndefined {
		return types.TypeNever
	}
	union, ok := t.(*types.UnionType)
	if !ok {
		return t
	}
	members := make([]types.Type, 0, len(union.Members))
	for _, member := range union.Members {
		if member == types.TypeNull || member == types.TypeUndefined {
			continue
		}
		members = append(members, member)
	}
	switch len(members) {
	case 0:
		return types.TypeNever
	case 1:
		return members[0]
	default:
		return types.NewUnion(members...)
	}
}

func (c *Checker) builtinRegExpType() *types.ObjectType {
	if c.result.RegExpType == nil {
		c.result.RegExpType = types.NewObject("$RegExp")
	}
	return c.result.RegExpType
}

func (c *Checker) builtinDateType() *types.ObjectType {
	if c.result.DateType == nil {
		c.result.DateType = types.NewObject("$Date")
	}
	return c.result.DateType
}

func (c *Checker) builtinDateMember(property string) (types.Type, bool) {
	switch property {
	case "toISOString":
		return types.NewFunction(nil, types.TypeString), true
	case "getUTCFullYear", "getUTCMonth", "getUTCDate", "getUTCHours", "getUTCMinutes", "getUTCSeconds":
		return types.NewFunction(nil, types.TypeNumber), true
	}
	return nil, false
}

func (c *Checker) builtinCollection(kind string, key, value types.Type) *BuiltinCollectionInfo {
	name := "$" + kind + "<" + key.String()
	if kind == "Map" {
		name += "," + value.String()
	}
	name += ">"
	if info := c.result.BuiltinCollections[name]; info != nil {
		return info
	}
	info := &BuiltinCollectionInfo{Kind: kind, Key: key, Value: value}
	info.Instance = types.NewObject(name)
	c.result.BuiltinCollections[name] = info
	return info
}

func (c *Checker) builtinCollectionMember(info *BuiltinCollectionInfo, property string) (types.Type, bool) {
	if info == nil {
		return nil, false
	}
	if property == "size" {
		return types.TypeNumber, true
	}
	keyParam := types.Param{Name: "key", Type: info.Key}
	switch info.Kind {
	case "Map":
		switch property {
		case "set":
			return types.NewFunction([]types.Param{keyParam, types.Param{Name: "value", Type: info.Value}}, info.Instance), true
		case "get":
			return types.NewFunction([]types.Param{keyParam}, types.NewUnion(info.Value, types.TypeUndefined)), true
		case "has", "delete":
			return types.NewFunction([]types.Param{keyParam}, types.TypeBoolean), true
		case "clear":
			return types.NewFunction(nil, types.TypeVoid), true
		}
	case "Set":
		switch property {
		case "add":
			return types.NewFunction([]types.Param{keyParam}, info.Instance), true
		case "has", "delete":
			return types.NewFunction([]types.Param{keyParam}, types.TypeBoolean), true
		case "clear":
			return types.NewFunction(nil, types.TypeVoid), true
		}
	}
	return nil, false
}

func (c *Checker) lookupMemberType(objType types.Type, property string) (types.Type, bool) {
	if objType == nil {
		return nil, false
	}
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
		if len(members) == 1 {
			return members[0], true
		}
		if len(members) > 1 {
			return types.NewUnion(members...), true
		}
	}
	return nil, false
}

func (c *Checker) checkExprWithExpected(expr ast.Expr, expected types.Type) types.Type {
	if expr == nil || expected == nil {
		return c.checkExpr(expr)
	}
	if tuple, ok := expected.(*types.TupleType); ok {
		if lit, ok := expr.(*ast.ArrayLit); ok {
			actual := make([]types.Type, len(lit.Elements))
			for i, elem := range lit.Elements {
				actual[i] = c.checkExpr(elem)
			}
			actualTuple := types.NewTuple(actual...)
			if len(actual) == len(tuple.Elements) && actualTuple.AssignableTo(tuple) {
				c.result.Types[lit] = tuple
				return tuple
			}
			c.result.Types[lit] = actualTuple
			return actualTuple
		}
	}
	if array, ok := expected.(*types.ArrayType); ok {
		if lit, ok := expr.(*ast.ArrayLit); ok {
			compatible := true
			for _, elem := range lit.Elements {
				if !c.checkExpr(elem).AssignableTo(array.Elem) {
					compatible = false
				}
			}
			if compatible {
				c.result.Types[lit] = array
				return array
			}
		}
	}
	if object, ok := expected.(*types.ObjectType); ok {
		if lit, ok := expr.(*ast.ObjectLit); ok {
			actual := c.checkExpr(lit)
			if actual.AssignableTo(object) {
				c.result.Types[lit] = object
				return object
			}
			return actual
		}
	}
	return c.checkExpr(expr)
}

func (c *Checker) checkVarDecl(stmt *ast.VarDeclStmt) {
	resolved := make([]types.Type, len(stmt.Declarations))
	for declIndex, decl := range stmt.Declarations {
		var declaredType types.Type
		if decl.Type != nil {
			declaredType = c.resolveTypeNode(decl.Type)
		}

		var initType types.Type
		if decl.Init != nil {
			if declaredType != nil {
				initType = c.checkExprWithExpected(decl.Init, declaredType)
			} else {
				initType = c.checkExpr(decl.Init)
			}
		}

		finalType := declaredType
		if finalType == nil {
			finalType = initType
		}
		if finalType == nil {
			finalType = types.TypeAny
		}
		resolved[declIndex] = finalType

		if declaredType != nil && initType != nil {
			if !initType.AssignableTo(declaredType) {
				c.error(decl.Init.Span(), "TS2322", fmt.Sprintf("Type '%s' is not assignable to type '%s'.", initType, declaredType))
			}
		}

		sym := &Symbol{
			Name: decl.Name,
			Kind: SymVar,
			Type: finalType,
			Node: stmt,
		}
		if err := c.currentScope.Define(sym); err != nil {
			c.error(decl.SourceSpan, "TS2300", err.Error())
		}
	}
	c.result.VarTypes[stmt] = resolved
}

func (c *Checker) checkFunctionDecl(fn *ast.FunctionDecl) {
	parentFnRet := c.currentFnRet
	defer func() { c.currentFnRet = parentFnRet }()

	fnType, _ := c.result.Types[fn].(*types.FunctionType)
	if fnType == nil {
		fnType = c.resolveFunctionType(fn)
		c.result.Types[fn] = fnType
	}
	c.currentFnRet = fnType.Return
	popTypeParams := c.pushTypeParams(fnType.TypeParams)
	defer popTypeParams()

	// Function scope
	c.currentScope = NewScope(c.currentScope)
	defer func() { c.currentScope = c.currentScope.Parent }()

	for i, p := range fn.Params {
		pType := types.TypeAny
		if i < len(fnType.Params) {
			pType = fnType.Params[i].Type
		} else if resolved := c.resolveTypeNode(p.Type); resolved != nil {
			pType = resolved
		}
		if pType == nil {
			pType = types.TypeAny
		}
		sym := &Symbol{
			Name: p.Name,
			Kind: SymParam,
			Type: pType,
			Node: fn,
		}
		_ = c.currentScope.Define(sym)
	}

	if fn.Body != nil {
		for _, s := range fn.Body.Statements {
			c.checkStatement(s)
		}
	}
}

func (c *Checker) checkClassDecl(cls *ast.ClassDecl) {
	info := c.result.Classes[cls.Name]
	if info == nil {
		return
	}
	parentClass := c.currentClass
	parentRet := c.currentFnRet
	c.currentClass = info
	popTypeParams := c.pushTypeParams(info.TypeParams)
	defer func() {
		popTypeParams()
		c.currentClass = parentClass
		c.currentFnRet = parentRet
	}()

	for _, field := range cls.Fields {
		if field.IsStatic || field.Init == nil {
			continue
		}
		expected := info.Instance.Fields[field.Name].Type
		actual := c.checkExprWithExpected(field.Init, expected)
		if !actual.AssignableTo(expected) {
			c.error(field.Init.Span(), "TS2322", fmt.Sprintf("Type '%s' is not assignable to field '%s: %s'.", actual, field.Name, expected))
		}
	}

	for i := range cls.Methods {
		method := &cls.Methods[i]
		var fnType *types.FunctionType
		if method.Name == "constructor" {
			fnType = info.Constructor
		} else {
			fnType = info.Methods[method.Name]
		}
		if fnType == nil {
			continue
		}
		c.currentFnRet = fnType.Return
		parentScope := c.currentScope
		c.currentScope = NewScope(parentScope)
		for j, p := range method.Params {
			pt := types.TypeAny
			if j < len(fnType.Params) {
				pt = fnType.Params[j].Type
			}
			_ = c.currentScope.Define(&Symbol{Name: p.Name, Kind: SymParam, Type: pt, Node: cls})
		}
		for _, stmt := range method.Body.Statements {
			c.checkStatement(stmt)
		}
		c.currentScope = parentScope
	}
}

func (c *Checker) checkBlock(b *ast.BlockStmt) {
	c.currentScope = NewScope(c.currentScope)
	defer func() { c.currentScope = c.currentScope.Parent }()

	for _, s := range b.Statements {
		c.checkStatement(s)
	}
}

func (c *Checker) checkIf(s *ast.IfStmt) {
	condType := c.checkExpr(s.Cond)
	_ = condType
	c.checkStatement(s.Then)
	if s.Else != nil {
		c.checkStatement(s.Else)
	}
}

func (c *Checker) checkWhile(s *ast.WhileStmt) {
	c.checkExpr(s.Cond)
	c.checkStatement(s.Body)
}

func (c *Checker) checkDoWhile(s *ast.DoWhileStmt) {
	c.checkStatement(s.Body)
	c.checkExpr(s.Cond)
}

func (c *Checker) checkSwitch(s *ast.SwitchStmt) {
	c.checkExpr(s.Expr)
	for _, clause := range s.Cases {
		if clause.Test != nil {
			c.checkExpr(clause.Test)
		}
		for _, stmt := range clause.Statements {
			c.checkStatement(stmt)
		}
	}
}

func (c *Checker) checkForOf(s *ast.ForOfStmt) {
	iterableType := c.checkExpr(s.Iterable)
	arr, ok := iterableType.(*types.ArrayType)
	if !ok {
		c.error(s.Iterable.Span(), "TS2488", fmt.Sprintf("Type '%s' is not iterable by the native array for-of lowering.", iterableType))
		return
	}
	elemType := arr.Elem
	if s.Type != nil {
		declared := c.resolveTypeNode(s.Type)
		if !elemType.AssignableTo(declared) {
			c.error(s.Span(), "TS2322", fmt.Sprintf("Type '%s' is not assignable to for-of variable type '%s'.", elemType, declared))
		}
		elemType = declared
	}
	c.currentScope = NewScope(c.currentScope)
	defer func() { c.currentScope = c.currentScope.Parent }()
	_ = c.currentScope.Define(&Symbol{Name: s.Name, Kind: SymVar, Type: elemType, Node: s})
	c.checkStatement(s.Body)
}

func (c *Checker) checkFor(s *ast.ForStmt) {
	c.currentScope = NewScope(c.currentScope)
	defer func() { c.currentScope = c.currentScope.Parent }()

	if s.Init != nil {
		c.checkStatement(s.Init)
	}
	if s.Cond != nil {
		c.checkExpr(s.Cond)
	}
	if s.Post != nil {
		c.checkExpr(s.Post)
	}
	c.checkStatement(s.Body)
}

func (c *Checker) checkReturn(s *ast.ReturnStmt) {
	var retType types.Type = types.TypeVoid
	if s.Value != nil {
		if c.currentFnRet != nil {
			retType = c.checkExprWithExpected(s.Value, c.currentFnRet)
		} else {
			retType = c.checkExpr(s.Value)
		}
	}

	if c.currentFnRet != nil {
		if !retType.AssignableTo(c.currentFnRet) {
			span := s.Span()
			if s.Value != nil {
				span = s.Value.Span()
			}
			c.error(span, "TS2322", fmt.Sprintf("Type '%s' is not assignable to return type '%s'.", retType, c.currentFnRet))
		}
	}
}

func (c *Checker) checkExpr(expr ast.Expr) types.Type {
	if expr == nil {
		return types.TypeVoid
	}

	switch e := expr.(type) {
	case *ast.NumberLit:
		c.result.Types[e] = types.TypeNumber
		return types.TypeNumber
	case *ast.StringLit:
		c.result.Types[e] = types.TypeString
		return types.TypeString
	case *ast.RegexLit:
		t := c.builtinRegExpType()
		c.result.Types[e] = t
		return t
	case *ast.BoolLit:
		c.result.Types[e] = types.TypeBoolean
		return types.TypeBoolean
	case *ast.NullLit:
		c.result.Types[e] = types.TypeNull
		return types.TypeNull
	case *ast.UndefinedLit:
		c.result.Types[e] = types.TypeUndefined
		return types.TypeUndefined
	case *ast.ThisExpr:
		if c.currentThisType != nil {
			c.result.Types[e] = c.currentThisType
			return c.currentThisType
		}
		if c.currentClass == nil {
			c.error(e.Span(), "TS2335", "'this' can only be referenced in a class body or a function with a this parameter.")
			c.result.Types[e] = types.TypeAny
			return types.TypeAny
		}
		c.result.Types[e] = c.currentClass.Instance
		return c.currentClass.Instance
	case *ast.SuperExpr:
		if c.currentClass == nil || c.currentClass.BaseName == "" {
			c.error(e.Span(), "TS2335", "'super' can only be referenced in a derived class.")
			c.result.Types[e] = types.TypeAny
			return types.TypeAny
		}
		if base := c.result.Classes[c.currentClass.BaseName]; base != nil {
			c.result.Types[e] = base.Constructor
			return base.Constructor
		}
		c.result.Types[e] = types.TypeAny
		return types.TypeAny
	case *ast.NewExpr:
		if e.ClassName == "RegExp" {
			if len(e.Args) < 1 || len(e.Args) > 2 {
				c.error(e.Span(), "TS2554", "RegExp expects a pattern and optional flags.")
			}
			for _, arg := range e.Args {
				if c.checkExpr(arg) != types.TypeString {
					c.error(arg.Span(), "TS2345", "Native RegExp pattern and flags must be strings.")
				}
			}
			t := c.builtinRegExpType()
			c.result.Types[e] = t
			return t
		}
		if e.ClassName == "Date" {
			if len(e.Args) != 1 {
				c.error(e.Span(), "TS2554", "Native Date constructor currently expects exactly one number or ISO string argument.")
			} else {
				at := c.checkExpr(e.Args[0])
				if at != types.TypeNumber && at != types.TypeString {
					c.error(e.Args[0].Span(), "TS2345", fmt.Sprintf("Date constructor argument must be number or string, got '%s'.", at))
				}
			}
			date := c.builtinDateType()
			c.result.Types[e] = date
			return date
		}
		if e.ClassName == "Map" || e.ClassName == "Set" {
			want := 1
			if e.ClassName == "Map" {
				want = 2
			}
			if len(e.TypeArgs) != want {
				c.error(e.Span(), "TS2558", fmt.Sprintf("%s expects %d type arguments, got %d.", e.ClassName, want, len(e.TypeArgs)))
				c.result.Types[e] = types.TypeAny
				return types.TypeAny
			}
			args := make([]types.Type, want)
			for i, node := range e.TypeArgs {
				args[i] = c.resolveTypeNode(node)
			}
			key, value := args[0], types.TypeUndefined
			if e.ClassName == "Map" {
				value = args[1]
			}
			info := c.builtinCollection(e.ClassName, key, value)
			c.result.Types[e] = info.Instance
			return info.Instance
		}
		info := c.result.Classes[e.ClassName]
		if info == nil {
			c.error(e.Span(), "TS2304", fmt.Sprintf("Cannot find class '%s'.", e.ClassName))
			c.result.Types[e] = types.TypeAny
			return types.TypeAny
		}
		if len(info.TypeParams) > 0 {
			if len(e.TypeArgs) != len(info.TypeParams) {
				c.error(e.Span(), "TS2558", fmt.Sprintf("Generic class '%s' expects %d type arguments, got %d.", e.ClassName, len(info.TypeParams), len(e.TypeArgs)))
			} else {
				typeArgs := make([]types.Type, len(e.TypeArgs))
				for i, node := range e.TypeArgs {
					typeArgs[i] = c.resolveTypeNode(node)
				}
				spec, err := c.specializeClass(info, typeArgs)
				if err != nil {
					c.error(e.Span(), "TS2314", err.Error())
				} else {
					info = spec
					c.result.GenericClasses[e] = spec
				}
			}
		} else if len(e.TypeArgs) > 0 {
			c.error(e.Span(), "TS2558", fmt.Sprintf("Class '%s' is not generic.", e.ClassName))
		}
		ctor := info.Constructor
		for i, arg := range e.Args {
			at := c.checkExpr(arg)
			if ctor != nil && i < len(ctor.Params) && !at.AssignableTo(ctor.Params[i].Type) {
				c.error(arg.Span(), "TS2345", fmt.Sprintf("Argument of type '%s' is not assignable to constructor parameter '%s'.", at, ctor.Params[i].Type))
			}
		}
		c.result.Types[e] = info.Instance
		return info.Instance
	case *ast.IdentExpr:
		sym := c.currentScope.Resolve(e.Name)
		if sym == nil {
			if e.Name == "Date" {
				ctor := types.NewObject("$DateConstructor")
				c.result.Types[e] = ctor
				return ctor
			}
			if e.Name == "JSON" {
				jsonType := types.NewObject("$JSON")
				c.result.Types[e] = jsonType
				return jsonType
			}
			if e.Name == "console" {
				obj := types.NewObject("console")
				obj.AddField("log", types.NewFunction([]types.Param{{Name: "value", Type: types.TypeAny}}, types.TypeVoid), false)
				c.result.Types[e] = obj
				return obj
			}
			c.error(e.Span(), "TS2304", fmt.Sprintf("Cannot find name '%s'.", e.Name))
			c.result.Types[e] = types.TypeAny
			return types.TypeAny
		}
		c.result.Symbols[e] = sym
		c.result.Types[e] = sym.Type
		return sym.Type
	case *ast.BinaryExpr:
		lType := c.checkExpr(e.Left)
		rType := c.checkExpr(e.Right)

		switch e.Op {
		case token.Plus:
			if lType == types.TypeAny || rType == types.TypeAny || lType == types.TypeUnknown || rType == types.TypeUnknown {
				c.result.Types[e] = types.TypeAny
				return types.TypeAny
			}
			if lType == types.TypeString || rType == types.TypeString {
				c.result.Types[e] = types.TypeString
				return types.TypeString
			}
			c.result.Types[e] = types.TypeNumber
			return types.TypeNumber
		case token.Minus, token.Star, token.Slash, token.Percent:
			c.result.Types[e] = types.TypeNumber
			return types.TypeNumber
		case token.EqEq, token.EqEqEq, token.BangEq, token.BangEqEq,
			token.Lt, token.LtEq, token.Gt, token.GtEq:
			c.result.Types[e] = types.TypeBoolean
			return types.TypeBoolean
		case token.AmpAmp, token.PipePipe:
			c.result.Types[e] = rType
			return rType
		case token.QuestionQuestion:
			left := removeNullishType(lType)
			if left == types.TypeNever {
				c.result.Types[e] = rType
				return rType
			}
			result := types.NewUnion(left, rType)
			c.result.Types[e] = result
			return result
		default:
			c.result.Types[e] = lType
			return lType
		}
	case *ast.UnaryExpr:
		targetType := c.checkExpr(e.Target)
		if e.Op == token.Bang {
			c.result.Types[e] = types.TypeBoolean
			return types.TypeBoolean
		}
		if e.Op == token.PlusPlus || e.Op == token.MinusMinus {
			c.result.Types[e] = types.TypeNumber
			return types.TypeNumber
		}
		c.result.Types[e] = targetType
		return targetType
	case *ast.ArrowFuncExpr:
		params := make([]types.Param, 0, len(e.Params))
		var thisType types.Type
		for _, param := range e.Params {
			pt := c.resolveTypeNode(param.Type)
			if pt == nil {
				pt = types.TypeAny
			}
			if param.IsThis {
				thisType = pt
				continue
			}
			params = append(params, types.Param{Name: param.Name, Type: pt, Optional: param.Optional, Rest: param.Rest})
		}

		parentScope := c.currentScope
		parentFnRet := c.currentFnRet
		parentThis := c.currentThisType
		c.currentScope = NewScope(parentScope)
		if thisType != nil {
			c.currentThisType = thisType
		}
		defer func() {
			c.currentScope = parentScope
			c.currentFnRet = parentFnRet
			c.currentThisType = parentThis
		}()
		runtimeIndex := 0
		for _, param := range e.Params {
			if param.IsThis {
				continue
			}
			_ = c.currentScope.Define(&Symbol{Name: param.Name, Kind: SymParam, Type: params[runtimeIndex].Type, Node: e})
			runtimeIndex++
		}

		declaredReturn := c.resolveTypeNode(e.ReturnType)
		var returnType types.Type
		if e.IsExprBody {
			bodyExpr, ok := e.Body.(ast.Expr)
			if !ok {
				returnType = types.TypeAny
			} else {
				bodyType := c.checkExpr(bodyExpr)
				returnType = bodyType
				if declaredReturn != nil {
					if !bodyType.AssignableTo(declaredReturn) {
						c.error(bodyExpr.Span(), "TS2322", fmt.Sprintf("Type '%s' is not assignable to return type '%s'.", bodyType, declaredReturn))
					}
					returnType = declaredReturn
				}
			}
		} else {
			if declaredReturn == nil {
				declaredReturn = types.TypeVoid
			}
			c.currentFnRet = declaredReturn
			if block, ok := e.Body.(*ast.BlockStmt); ok {
				for _, stmt := range block.Statements {
					c.checkStatement(stmt)
				}
			}
			returnType = declaredReturn
		}
		fnType := types.NewFunction(params, returnType)
		fnType.This = thisType
		c.result.Types[e] = fnType
		return fnType
	case *ast.FunctionExpr:
		params := make([]types.Param, 0, len(e.Params))
		var thisType types.Type
		for _, param := range e.Params {
			pt := c.resolveTypeNode(param.Type)
			if pt == nil {
				pt = types.TypeAny
			}
			if param.IsThis {
				thisType = pt
				continue
			}
			params = append(params, types.Param{Name: param.Name, Type: pt, Optional: param.Optional, Rest: param.Rest})
		}
		declaredReturn := c.resolveTypeNode(e.ReturnType)
		if declaredReturn == nil {
			declaredReturn = types.TypeVoid
		}
		parentScope, parentFnRet, parentThis := c.currentScope, c.currentFnRet, c.currentThisType
		c.currentScope = NewScope(parentScope)
		c.currentFnRet = declaredReturn
		c.currentThisType = thisType
		runtimeIndex := 0
		for _, param := range e.Params {
			if param.IsThis {
				continue
			}
			_ = c.currentScope.Define(&Symbol{Name: param.Name, Kind: SymParam, Type: params[runtimeIndex].Type, Node: e})
			runtimeIndex++
		}
		for _, stmt := range e.Body.Statements {
			c.checkStatement(stmt)
		}
		c.currentScope, c.currentFnRet, c.currentThisType = parentScope, parentFnRet, parentThis
		fnType := types.NewFunction(params, declaredReturn)
		fnType.This = thisType
		c.result.Types[e] = fnType
		return fnType
	case *ast.CallExpr:
		if ident, ok := e.Callee.(*ast.IdentExpr); ok {
			switch ident.Name {
			case "taskGroup":
				if len(e.Args) != 0 {
					c.error(e.Span(), "TS2554", "taskGroup expects no arguments.")
				}
				if c.result.TaskGroupType == nil {
					c.result.TaskGroupType = types.NewObject("$TaskGroup")
				}
				c.result.Types[e.Callee] = types.TypeAny
				c.result.Types[e] = c.result.TaskGroupType
				return c.result.TaskGroupType
			case "groupSpawn":
				if len(e.Args) != 2 {
					c.error(e.Span(), "TS2554", "groupSpawn expects group and zero-argument function.")
					c.result.Types[e] = types.TypeAny
					return types.TypeAny
				}
				groupType := c.checkExpr(e.Args[0])
				if c.result.TaskGroupType == nil || !groupType.Equals(c.result.TaskGroupType) {
					c.error(e.Args[0].Span(), "TS2345", "groupSpawn expects a task group.")
				}
				fnType, ok := c.checkExpr(e.Args[1]).(*types.FunctionType)
				if !ok || len(fnType.Params) != 0 {
					c.error(e.Args[1].Span(), "TS2345", "groupSpawn expects a zero-argument function.")
					c.result.Types[e] = types.TypeAny
					return types.TypeAny
				}
				name := fmt.Sprintf("$Task$%d", c.taskTypeCount)
				c.taskTypeCount++
				taskType := types.NewObject(name)
				c.result.TaskResults[name] = fnType.Return
				c.result.Types[e.Callee] = types.TypeAny
				c.result.Types[e] = taskType
				return taskType
			case "groupJoin", "groupCancel":
				if len(e.Args) != 1 {
					c.error(e.Span(), "TS2554", ident.Name+" expects one task group.")
				} else {
					groupType := c.checkExpr(e.Args[0])
					if c.result.TaskGroupType == nil || !groupType.Equals(c.result.TaskGroupType) {
						c.error(e.Args[0].Span(), "TS2345", ident.Name+" expects a task group.")
					}
				}
				c.result.Types[e.Callee] = types.TypeAny
				c.result.Types[e] = types.TypeVoid
				return types.TypeVoid
			case "channel":
				if len(e.TypeArgs) != 1 {
					c.error(e.Span(), "TS2558", "channel expects exactly one type argument.")
					c.result.Types[e] = types.TypeAny
					return types.TypeAny
				}
				if len(e.Args) != 1 {
					c.error(e.Span(), "TS2554", "channel expects exactly one capacity argument.")
				} else if capType := c.checkExpr(e.Args[0]); !capType.AssignableTo(types.TypeNumber) {
					c.error(e.Args[0].Span(), "TS2345", "channel capacity must be a number.")
				}
				elem := c.resolveTypeNode(e.TypeArgs[0])
				name := fmt.Sprintf("$Channel$%d", c.channelTypeCount)
				c.channelTypeCount++
				channelType := types.NewObject(name)
				c.result.ChannelElements[name] = elem
				c.result.Types[e.Callee] = types.TypeAny
				c.result.Types[e] = channelType
				return channelType
			case "channelSend":
				if len(e.Args) != 2 {
					c.error(e.Span(), "TS2554", "channelSend expects channel and value.")
					c.result.Types[e] = types.TypeVoid
					return types.TypeVoid
				}
				chType := c.checkExpr(e.Args[0])
				obj, ok := chType.(*types.ObjectType)
				elem, known := types.Type(nil), false
				if ok {
					elem, known = c.result.ChannelElements[obj.Name]
				}
				valueType := c.checkExpr(e.Args[1])
				if !known {
					c.error(e.Args[0].Span(), "TS2345", "channelSend expects a channel handle.")
				} else if !valueType.AssignableTo(elem) {
					c.error(e.Args[1].Span(), "TS2345", fmt.Sprintf("Type '%s' is not assignable to channel element type '%s'.", valueType, elem))
				}
				c.result.Types[e.Callee] = types.TypeAny
				c.result.Types[e] = types.TypeVoid
				return types.TypeVoid
			case "channelRecv":
				if len(e.Args) != 1 {
					c.error(e.Span(), "TS2554", "channelRecv expects one channel.")
					c.result.Types[e] = types.TypeAny
					return types.TypeAny
				}
				chType := c.checkExpr(e.Args[0])
				obj, ok := chType.(*types.ObjectType)
				elem, known := types.Type(nil), false
				if ok {
					elem, known = c.result.ChannelElements[obj.Name]
				}
				if !known {
					c.error(e.Args[0].Span(), "TS2345", "channelRecv expects a channel handle.")
					c.result.Types[e] = types.TypeAny
					return types.TypeAny
				}
				c.result.Types[e.Callee] = types.TypeAny
				c.result.Types[e] = elem
				return elem
			case "channelTrySend":
				if len(e.Args) != 2 {
					c.error(e.Span(), "TS2554", "channelTrySend expects channel and value.")
					c.result.Types[e] = types.TypeBoolean
					return types.TypeBoolean
				}
				chType := c.checkExpr(e.Args[0])
				obj, ok := chType.(*types.ObjectType)
				elem, known := types.Type(nil), false
				if ok {
					elem, known = c.result.ChannelElements[obj.Name]
				}
				valueType := c.checkExpr(e.Args[1])
				if !known {
					c.error(e.Args[0].Span(), "TS2345", "channelTrySend expects a channel handle.")
				} else if !valueType.AssignableTo(elem) {
					c.error(e.Args[1].Span(), "TS2345", fmt.Sprintf("Type '%s' is not assignable to channel element type '%s'.", valueType, elem))
				}
				c.result.Types[e.Callee] = types.TypeAny
				c.result.Types[e] = types.TypeBoolean
				return types.TypeBoolean
			case "channelTryRecvOr":
				if len(e.Args) != 2 {
					c.error(e.Span(), "TS2554", "channelTryRecvOr expects channel and fallback.")
					c.result.Types[e] = types.TypeAny
					return types.TypeAny
				}
				chType := c.checkExpr(e.Args[0])
				obj, ok := chType.(*types.ObjectType)
				elem, known := types.Type(nil), false
				if ok {
					elem, known = c.result.ChannelElements[obj.Name]
				}
				fallbackType := c.checkExpr(e.Args[1])
				if !known {
					c.error(e.Args[0].Span(), "TS2345", "channelTryRecvOr expects a channel handle.")
					c.result.Types[e] = types.TypeAny
					return types.TypeAny
				}
				if !fallbackType.AssignableTo(elem) {
					c.error(e.Args[1].Span(), "TS2345", fmt.Sprintf("Fallback type '%s' is not assignable to channel element type '%s'.", fallbackType, elem))
				}
				c.result.Types[e.Callee] = types.TypeAny
				c.result.Types[e] = elem
				return elem
			case "spawn":
				if len(e.Args) != 1 {
					c.error(e.Span(), "TS2554", "spawn expects exactly one zero-argument function.")
					c.result.Types[e] = types.TypeAny
					return types.TypeAny
				}
				fnType, ok := c.checkExpr(e.Args[0]).(*types.FunctionType)
				if !ok {
					c.error(e.Args[0].Span(), "TS2345", "spawn expects a function value.")
					c.result.Types[e] = types.TypeAny
					return types.TypeAny
				}
				if len(fnType.Params) != 0 {
					c.error(e.Args[0].Span(), "TS2345", "spawn currently requires a zero-argument function.")
				}
				name := fmt.Sprintf("$Task$%d", c.taskTypeCount)
				c.taskTypeCount++
				taskType := types.NewObject(name)
				c.result.TaskResults[name] = fnType.Return
				c.result.Types[e.Callee] = types.TypeAny
				c.result.Types[e] = taskType
				return taskType
			case "yieldNow":
				if len(e.Args) != 0 {
					c.error(e.Span(), "TS2554", "yieldNow expects no arguments.")
				}
				c.result.Types[e.Callee] = types.TypeAny
				c.result.Types[e] = types.TypeVoid
				return types.TypeVoid
			case "sleep":
				if len(e.Args) != 1 {
					c.error(e.Span(), "TS2554", "sleep expects exactly one millisecond argument.")
				} else if argType := c.checkExpr(e.Args[0]); !argType.AssignableTo(types.TypeNumber) {
					c.error(e.Args[0].Span(), "TS2345", "sleep expects a number of milliseconds.")
				}
				c.result.Types[e.Callee] = types.TypeAny
				c.result.Types[e] = types.TypeVoid
				return types.TypeVoid
			case "setTaskContext":
				if len(e.Args) != 1 {
					c.error(e.Span(), "TS2554", "setTaskContext expects one string argument.")
				} else if argType := c.checkExpr(e.Args[0]); !argType.AssignableTo(types.TypeString) {
					c.error(e.Args[0].Span(), "TS2345", "setTaskContext expects a string.")
				}
				c.result.Types[e.Callee] = types.TypeAny
				c.result.Types[e] = types.TypeVoid
				return types.TypeVoid
			case "taskContext":
				if len(e.Args) != 0 {
					c.error(e.Span(), "TS2554", "taskContext expects no arguments.")
				}
				c.result.Types[e.Callee] = types.TypeAny
				c.result.Types[e] = types.TypeString
				return types.TypeString
			case "cancelTask":
				if len(e.Args) != 1 {
					c.error(e.Span(), "TS2554", "cancelTask expects exactly one task.")
				} else {
					taskType := c.checkExpr(e.Args[0])
					obj, ok := taskType.(*types.ObjectType)
					if !ok {
						c.error(e.Args[0].Span(), "TS2345", "cancelTask expects a task handle.")
					} else if _, known := c.result.TaskResults[obj.Name]; !known {
						c.error(e.Args[0].Span(), "TS2345", "cancelTask received an unknown task handle type.")
					}
				}
				c.result.Types[e.Callee] = types.TypeAny
				c.result.Types[e] = types.TypeVoid
				return types.TypeVoid
			case "taskCancelled":
				if len(e.Args) != 0 {
					c.error(e.Span(), "TS2554", "taskCancelled expects no arguments.")
				}
				c.result.Types[e.Callee] = types.TypeAny
				c.result.Types[e] = types.TypeBoolean
				return types.TypeBoolean
			case "join":
				if len(e.Args) != 1 {
					c.error(e.Span(), "TS2554", "join expects exactly one task.")
					c.result.Types[e] = types.TypeAny
					return types.TypeAny
				}
				taskType := c.checkExpr(e.Args[0])
				obj, ok := taskType.(*types.ObjectType)
				if !ok {
					c.error(e.Args[0].Span(), "TS2345", "join expects a task handle.")
					c.result.Types[e] = types.TypeAny
					return types.TypeAny
				}
				resultType, ok := c.result.TaskResults[obj.Name]
				if !ok {
					c.error(e.Args[0].Span(), "TS2345", "join received an unknown task handle type.")
					c.result.Types[e] = types.TypeAny
					return types.TypeAny
				}
				c.result.Types[e.Callee] = types.TypeAny
				c.result.Types[e] = resultType
				return resultType
			}
		}
		calleeType := c.checkExpr(e.Callee)
		fnType, ok := calleeType.(*types.FunctionType)
		if !ok {
			if calleeType.Kind() != types.KindAny {
				c.error(e.Callee.Span(), "TS2349", "This expression is not callable.")
			}
			c.result.Types[e] = types.TypeAny
			return types.TypeAny
		}

		argTypes := make([]types.Type, len(e.Args))
		for i, arg := range e.Args {
			argTypes[i] = c.checkExpr(arg)
		}

		effective := fnType
		if len(e.TypeArgs) > 0 {
			if len(fnType.TypeParams) == 0 {
				c.error(e.Span(), "TS2558", fmt.Sprintf("Expected 0 type arguments, but got %d.", len(e.TypeArgs)))
			} else {
				args := make([]types.Type, len(e.TypeArgs))
				for i, node := range e.TypeArgs {
					args[i] = c.resolveTypeNode(node)
				}
				instantiated, err := types.InstantiateFunction(fnType, args)
				if err != nil {
					c.error(e.Span(), "TS2558", err.Error())
					c.result.Types[e] = types.TypeAny
					return types.TypeAny
				}
				effective = instantiated
			}
		} else if len(fnType.TypeParams) > 0 {
			instantiated, err := types.InferFunction(fnType, argTypes)
			if err != nil {
				c.error(e.Span(), "TS2684", err.Error())
				c.result.Types[e] = types.TypeAny
				return types.TypeAny
			}
			effective = instantiated
		}

		for i, argType := range argTypes {
			var expected types.Type
			if i < len(effective.Params) {
				param := effective.Params[i]
				if param.Rest {
					if arr, ok := param.Type.(*types.ArrayType); ok {
						expected = arr.Elem
					}
				} else {
					expected = param.Type
				}
			} else if len(effective.Params) > 0 && effective.Params[len(effective.Params)-1].Rest {
				if arr, ok := effective.Params[len(effective.Params)-1].Type.(*types.ArrayType); ok {
					expected = arr.Elem
				}
			}
			if expected != nil && !argType.AssignableTo(expected) {
				c.error(e.Args[i].Span(), "TS2345", fmt.Sprintf("Argument of type '%s' is not assignable to parameter of type '%s'.", argType, expected))
			}
		}
		if len(fnType.TypeParams) > 0 {
			c.result.GenericCalls[e] = effective
		}
		c.result.Types[e] = effective.Return
		return effective.Return
	case *ast.AssignExpr:
		targetType := c.checkExpr(e.Left)
		valType := c.checkExpr(e.Right)
		if !valType.AssignableTo(targetType) {
			c.error(e.Right.Span(), "TS2322", fmt.Sprintf("Type '%s' is not assignable to type '%s'.", valType, targetType))
		}
		c.result.Types[e] = targetType
		return targetType
	case *ast.TernaryExpr:
		c.checkExpr(e.Cond)
		t1 := c.checkExpr(e.Then)
		t2 := c.checkExpr(e.Else)
		resType := types.NewUnion(t1, t2)
		c.result.Types[e] = resType
		return resType
	case *ast.SpreadExpr:
		t := c.checkExpr(e.Value)
		c.result.Types[e] = t
		return t
	case *ast.ArrayLit:
		var elemType types.Type = types.TypeNever
		for _, el := range e.Elements {
			t := c.checkExpr(el)
			if spread, ok := el.(*ast.SpreadExpr); ok {
				arr, ok := t.(*types.ArrayType)
				if !ok {
					c.error(spread.Span(), "TS2488", fmt.Sprintf("Type '%s' is not spreadable by native array spread lowering.", t))
					t = types.TypeAny
				} else {
					t = arr.Elem
				}
			}
			if elemType == types.TypeNever {
				elemType = t
			} else {
				elemType = types.NewUnion(elemType, t)
			}
		}
		if elemType == types.TypeNever {
			elemType = types.TypeAny
		}
		arrType := types.NewArray(elemType)
		c.result.Types[e] = arrType
		return arrType
	case *ast.ObjectLit:
		obj := types.NewObject("")
		for _, prop := range e.Properties {
			pType := c.checkExpr(prop.Value)
			if prop.Spread {
				source, ok := pType.(*types.ObjectType)
				if !ok {
					c.error(prop.SourceSpan, "TS2698", fmt.Sprintf("Spread types may only be created from closed object types, got '%s'.", pType))
					continue
				}
				for _, name := range source.FieldOrder {
					field := source.Fields[name]
					obj.AddField(name, field.Type, field.Optional)
				}
				continue
			}
			obj.AddField(prop.Key, pType, false)
		}
		c.result.Types[e] = obj
		return obj
	case *ast.IndexExpr:
		targetType := c.checkExpr(e.Target)
		indexType := c.checkExpr(e.Index)
		if object, ok := targetType.(*types.ObjectType); ok {
			if key, ok := e.Index.(*ast.StringLit); ok {
				if field, exists := object.Fields[key.Value]; exists {
					fieldType := field.Type
					if field.Optional {
						fieldType = types.NewUnion(fieldType, types.TypeUndefined)
					}
					c.result.Types[e] = fieldType
					return fieldType
				}
				c.error(e.Span(), "TS7053", fmt.Sprintf("Property '%s' does not exist on type '%s'.", key.Value, targetType))
				c.result.Types[e] = types.TypeAny
				return types.TypeAny
			}
		}
		if targetType != types.TypeAny && indexType != types.TypeNumber && indexType != types.TypeAny {
			c.error(e.Index.Span(), "TS7015", "Array index expression must be a number.")
		}
		if tuple, ok := targetType.(*types.TupleType); ok {
			if lit, ok := e.Index.(*ast.NumberLit); ok {
				idx := int(lit.Value)
				if float64(idx) == lit.Value && idx >= 0 && idx < len(tuple.Elements) {
					c.result.Types[e] = tuple.Elements[idx]
					return tuple.Elements[idx]
				}
			}
			if len(tuple.Elements) > 0 {
				u := types.NewUnion(tuple.Elements...)
				c.result.Types[e] = u
				return u
			}
			c.result.Types[e] = types.TypeNever
			return types.TypeNever
		}
		if arr, ok := targetType.(*types.ArrayType); ok {
			c.result.Types[e] = arr.Elem
			return arr.Elem
		}
		if targetType != types.TypeAny {
			c.error(e.Span(), "TS7053", fmt.Sprintf("Element implicitly has an 'any' type because type '%s' has no numeric index signature.", targetType))
		}
		c.result.Types[e] = types.TypeAny
		return types.TypeAny
	case *ast.MemberExpr:
		if ident, ok := e.Object.(*ast.IdentExpr); ok {
			if members := c.result.Enums[ident.Name]; members != nil {
				if _, exists := members[e.Property]; !exists {
					c.error(e.Span(), "TS2339", fmt.Sprintf("Enum '%s' has no member '%s'.", ident.Name, e.Property))
				}
				c.result.Types[ident] = types.TypeNumber
				c.result.Types[e] = types.TypeNumber
				return types.TypeNumber
			}
		}
		objType := c.checkExpr(e.Object)
		lookupType := objType
		if e.Optional {
			lookupType = removeNullishType(objType)
		}
		memberType, ok := c.lookupMemberType(lookupType, e.Property)
		if !ok {
			c.error(e.Span(), "TS2339", fmt.Sprintf("Property '%s' does not exist on type '%s'.", e.Property, objType))
			c.result.Types[e] = types.TypeAny
			return types.TypeAny
		}
		if e.Optional {
			memberType = types.NewUnion(memberType, types.TypeUndefined)
		}
		c.result.Types[e] = memberType
		return memberType
	default:
		c.result.Types[expr] = types.TypeAny
		return types.TypeAny
	}
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
	retType := c.resolveTypeNode(fn.ReturnType)
	if retType == nil {
		retType = types.TypeVoid
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
		case "null":
			return types.TypeNull
		default:
			return types.TypeAny
		}
	case *ast.TypeRefNode:
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
	case *ast.FunctionTypeNode:
		params := make([]types.Param, 0, len(t.Params))
		var thisType types.Type
		for _, p := range t.Params {
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
		ret := c.resolveTypeNode(t.ReturnType)
		if ret == nil {
			ret = types.TypeVoid
		}
		fn := types.NewFunction(params, ret)
		fn.This = thisType
		return fn
	default:
		return types.TypeAny
	}
}
