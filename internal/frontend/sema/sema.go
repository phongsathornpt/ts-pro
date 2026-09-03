package sema

import (
	"fmt"

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
	SymInterface
	SymTypeAlias
)

type Symbol struct {
	Name string
	Kind SymbolKind
	Type types.Type
	Node ast.Node
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
	Types       map[ast.Node]types.Type
	Symbols     map[ast.Node]*Symbol
	RootScope   *Scope
	Diagnostics diag.DiagnosticList
}

type Checker struct {
	currentScope *Scope
	result       *Result
	currentFnRet types.Type
}

func NewChecker() *Checker {
	root := NewScope(nil)
	return &Checker{
		currentScope: root,
		result: &Result{
			Types:       make(map[ast.Node]types.Type),
			Symbols:     make(map[ast.Node]*Symbol),
			RootScope:   root,
			Diagnostics: make(diag.DiagnosticList, 0),
		},
	}
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
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.FunctionDecl:
			fnType := c.resolveFunctionType(s)
			sym := &Symbol{
				Name: s.Name,
				Kind: SymFunc,
				Type: fnType,
				Node: s,
			}
			if err := c.currentScope.Define(sym); err != nil {
				c.error(s.Span(), "TS2300", err.Error())
			}
			c.result.Symbols[s] = sym
			c.result.Types[s] = fnType
		case *ast.InterfaceDecl:
			objType := types.NewObject(s.Name)
			for _, f := range s.Fields {
				fieldType := c.resolveTypeNode(f.Type)
				objType.AddField(f.Name, fieldType, f.Optional)
			}
			sym := &Symbol{
				Name: s.Name,
				Kind: SymInterface,
				Type: objType,
				Node: s,
			}
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
		}
	}
}

func (c *Checker) checkProgram(prog *ast.Program) {
	for _, stmt := range prog.Statements {
		c.checkStatement(stmt)
	}
}

func (c *Checker) checkStatement(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.VarDeclStmt:
		c.checkVarDecl(s)
	case *ast.FunctionDecl:
		c.checkFunctionDecl(s)
	case *ast.BlockStmt:
		c.checkBlock(s)
	case *ast.IfStmt:
		c.checkIf(s)
	case *ast.WhileStmt:
		c.checkWhile(s)
	case *ast.ForStmt:
		c.checkFor(s)
	case *ast.ReturnStmt:
		c.checkReturn(s)
	case *ast.ExprStmt:
		c.checkExpr(s.Expr)
	}
}

func (c *Checker) checkVarDecl(stmt *ast.VarDeclStmt) {
	for _, decl := range stmt.Declarations {
		var declaredType types.Type
		if decl.Type != nil {
			declaredType = c.resolveTypeNode(decl.Type)
		}

		var initType types.Type
		if decl.Init != nil {
			initType = c.checkExpr(decl.Init)
		}

		finalType := declaredType
		if finalType == nil {
			finalType = initType
		}
		if finalType == nil {
			finalType = types.TypeAny
		}

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
}

func (c *Checker) checkFunctionDecl(fn *ast.FunctionDecl) {
	parentFnRet := c.currentFnRet
	defer func() { c.currentFnRet = parentFnRet }()

	fnType := c.resolveFunctionType(fn)
	c.currentFnRet = fnType.Return

	// Function scope
	c.currentScope = NewScope(c.currentScope)
	defer func() { c.currentScope = c.currentScope.Parent }()

	for _, p := range fn.Params {
		pType := c.resolveTypeNode(p.Type)
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
		retType = c.checkExpr(s.Value)
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
	case *ast.BoolLit:
		c.result.Types[e] = types.TypeBoolean
		return types.TypeBoolean
	case *ast.NullLit:
		c.result.Types[e] = types.TypeNull
		return types.TypeNull
	case *ast.UndefinedLit:
		c.result.Types[e] = types.TypeUndefined
		return types.TypeUndefined
	case *ast.IdentExpr:
		sym := c.currentScope.Resolve(e.Name)
		if sym == nil {
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
		for _, param := range e.Params {
			pt := c.resolveTypeNode(param.Type)
			if pt == nil {
				pt = types.TypeAny
			}
			params = append(params, types.Param{Name: param.Name, Type: pt, Optional: param.Optional})
		}

		parentScope := c.currentScope
		parentFnRet := c.currentFnRet
		c.currentScope = NewScope(parentScope)
		defer func() {
			c.currentScope = parentScope
			c.currentFnRet = parentFnRet
		}()
		for i, param := range e.Params {
			_ = c.currentScope.Define(&Symbol{Name: param.Name, Kind: SymParam, Type: params[i].Type, Node: e})
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
		c.result.Types[e] = fnType
		return fnType
	case *ast.CallExpr:
		calleeType := c.checkExpr(e.Callee)
		fnType, ok := calleeType.(*types.FunctionType)
		if !ok {
			if calleeType.Kind() != types.KindAny {
				c.error(e.Callee.Span(), "TS2349", "This expression is not callable.")
			}
			c.result.Types[e] = types.TypeAny
			return types.TypeAny
		}

		for i, arg := range e.Args {
			argType := c.checkExpr(arg)
			if i < len(fnType.Params) {
				expected := fnType.Params[i].Type
				if !argType.AssignableTo(expected) {
					c.error(arg.Span(), "TS2345", fmt.Sprintf("Argument of type '%s' is not assignable to parameter of type '%s'.", argType, expected))
				}
			}
		}
		c.result.Types[e] = fnType.Return
		return fnType.Return
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
	case *ast.ArrayLit:
		var elemType types.Type = types.TypeNever
		for _, el := range e.Elements {
			t := c.checkExpr(el)
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
			obj.AddField(prop.Key, pType, false)
		}
		c.result.Types[e] = obj
		return obj
	case *ast.IndexExpr:
		targetType := c.checkExpr(e.Target)
		indexType := c.checkExpr(e.Index)
		if indexType != types.TypeNumber && indexType != types.TypeAny {
			c.error(e.Index.Span(), "TS7015", "Array index expression must be a number.")
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
		objType := c.checkExpr(e.Object)
		if arr, ok := objType.(*types.ArrayType); ok {
			switch e.Property {
			case "length":
				c.result.Types[e] = types.TypeNumber
				return types.TypeNumber
			case "push":
				t := types.NewFunction([]types.Param{{Name: "value", Type: arr.Elem}}, types.TypeNumber)
				c.result.Types[e] = t
				return t
			case "pop":
				t := types.NewFunction(nil, arr.Elem)
				c.result.Types[e] = t
				return t
			}
			c.error(e.Span(), "TS2339", fmt.Sprintf("Property '%s' does not exist on type '%s'.", e.Property, objType))
			c.result.Types[e] = types.TypeAny
			return types.TypeAny
		}
		if o, ok := objType.(*types.ObjectType); ok {
			if f, exists := o.Fields[e.Property]; exists {
				c.result.Types[e] = f.Type
				return f.Type
			}
			c.error(e.Span(), "TS2339", fmt.Sprintf("Property '%s' does not exist on type '%s'.", e.Property, objType))
			c.result.Types[e] = types.TypeAny
			return types.TypeAny
		}
		if objType != nil && objType != types.TypeAny {
			c.error(e.Span(), "TS2339", fmt.Sprintf("Property '%s' does not exist on type '%s'.", e.Property, objType))
		}
		c.result.Types[e] = types.TypeAny
		return types.TypeAny
	default:
		c.result.Types[expr] = types.TypeAny
		return types.TypeAny
	}
}

func (c *Checker) resolveFunctionType(fn *ast.FunctionDecl) *types.FunctionType {
	var params []types.Param
	for _, p := range fn.Params {
		pType := c.resolveTypeNode(p.Type)
		if pType == nil {
			pType = types.TypeAny
		}
		params = append(params, types.Param{
			Name:     p.Name,
			Type:     pType,
			Optional: p.Optional,
		})
	}
	retType := c.resolveTypeNode(fn.ReturnType)
	if retType == nil {
		retType = types.TypeVoid
	}
	return types.NewFunction(params, retType)
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
		default:
			return types.TypeAny
		}
	case *ast.TypeRefNode:
		if t.Name == "Array" && len(t.TypeArgs) == 1 {
			return types.NewArray(c.resolveTypeNode(t.TypeArgs[0]))
		}
		sym := c.currentScope.Resolve(t.Name)
		if sym != nil && (sym.Kind == SymInterface || sym.Kind == SymClass || sym.Kind == SymTypeAlias) {
			return sym.Type
		}
		return types.TypeAny
	case *ast.ArrayTypeNode:
		elem := c.resolveTypeNode(t.ElemType)
		return types.NewArray(elem)
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
		for _, p := range t.Params {
			pt := c.resolveTypeNode(p.Type)
			if pt == nil {
				pt = types.TypeAny
			}
			params = append(params, types.Param{Name: p.Name, Type: pt, Optional: p.Optional})
		}
		ret := c.resolveTypeNode(t.ReturnType)
		if ret == nil {
			ret = types.TypeVoid
		}
		return types.NewFunction(params, ret)
	default:
		return types.TypeAny
	}
}
