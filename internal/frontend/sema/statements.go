package sema

import (
	"fmt"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/token"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

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
	case *ast.ThrowStmt:
		c.checkExpr(s.Value)
	case *ast.TryStmt:
		c.checkTry(s)
	case *ast.ExprStmt:
		c.checkExpr(s.Expr)
	}
}

func statementReturns(stmt ast.Stmt) bool {
	switch s := stmt.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.BlockStmt:
		if len(s.Statements) == 0 {
			return false
		}
		return statementReturns(s.Statements[len(s.Statements)-1])
	default:
		return false
	}
}

func removeExactType(t, excluded types.Type) types.Type {
	if t.Equals(excluded) {
		return types.TypeNever
	}
	union := t.(*types.UnionType)
	members := make([]types.Type, 0, len(union.Members))
	for _, member := range union.Members {
		if !member.Equals(excluded) {
			members = append(members, member)
		}
	}
	if len(members) == 1 {
		return members[0]
	}
	return types.NewUnion(members...)
}

func strictNullishGuard(stmt ast.Stmt) (string, types.Type, bool) {
	ifStmt, ok := stmt.(*ast.IfStmt)
	if !ok || ifStmt.Else != nil || !statementReturns(ifStmt.Then) {
		return "", nil, false
	}
	binary, ok := ifStmt.Cond.(*ast.BinaryExpr)
	if !ok || binary.Op != token.EqEqEq {
		return "", nil, false
	}
	match := func(identExpr ast.Expr, literal ast.Expr) (string, types.Type, bool) {
		ident, ok := identExpr.(*ast.IdentExpr)
		if !ok {
			return "", nil, false
		}
		switch literal.(type) {
		case *ast.NullLit:
			return ident.Name, types.TypeNull, true
		case *ast.UndefinedLit:
			return ident.Name, types.TypeUndefined, true
		default:
			return "", nil, false
		}
	}
	if name, excluded, ok := match(binary.Left, binary.Right); ok {
		return name, excluded, true
	}
	return match(binary.Right, binary.Left)
}

func (c *Checker) applyGuardReturnNarrowing(stmt ast.Stmt) {
	name, excluded, ok := strictNullishGuard(stmt)
	if !ok {
		return
	}
	sym := c.currentScope.Resolve(name)
	if sym == nil {
		return
	}
	narrowed := removeExactType(sym.Type, excluded)
	if narrowed.Equals(sym.Type) {
		return
	}
	if local := c.currentScope.Symbols[name]; local != nil {
		local.Type = narrowed
		return
	}
	_ = c.currentScope.Define(&Symbol{Name: sym.Name, Kind: sym.Kind, Type: narrowed, Node: sym.Node})
}

func (c *Checker) checkStatementList(statements []ast.Stmt) {
	for _, stmt := range statements {
		c.checkStatement(stmt)
		c.applyGuardReturnNarrowing(stmt)
	}
}

func removeNullishType(t types.Type) types.Type {
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
	if fn.IsAsync {
		if inner := c.result.AsyncResults[fn]; inner != nil {
			c.currentFnRet = inner
		}
	}
	popTypeParams := c.pushTypeParams(fnType.TypeParams)
	defer popTypeParams()

	// Function scope
	c.currentScope = NewScope(c.currentScope)
	defer func() { c.currentScope = c.currentScope.Parent }()

	for i, p := range fn.Params {
		pType := fnType.Params[i].Type
		sym := &Symbol{
			Name: p.Name,
			Kind: SymParam,
			Type: pType,
			Node: fn,
		}
		_ = c.currentScope.Define(sym)
	}

	if fn.Body != nil {
		c.checkStatementList(fn.Body.Statements)
	}
}

func (c *Checker) checkClassDecl(cls *ast.ClassDecl) {
	info := c.result.Classes[cls.Name]
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
		c.checkStatementList(method.Body.Statements)
		c.currentScope = parentScope
	}
}

func (c *Checker) checkTry(s *ast.TryStmt) {
	c.checkBlock(s.Try)
	if s.Catch != nil {
		catchType := types.TypeAny
		if s.CatchType != nil {
			if resolved := c.resolveTypeNode(s.CatchType); resolved != nil {
				catchType = resolved
			}
		}
		parent := c.currentScope
		c.currentScope = NewScope(parent)
		_ = c.currentScope.Define(&Symbol{Name: s.CatchName, Kind: SymVar, Type: catchType, Node: s})
		for _, stmt := range s.Catch.Statements {
			c.checkStatement(stmt)
		}
		c.currentScope = parent
	}
	if s.Finally != nil {
		c.checkBlock(s.Finally)
	}
}

func (c *Checker) checkBlock(b *ast.BlockStmt) {
	c.currentScope = NewScope(c.currentScope)
	defer func() { c.currentScope = c.currentScope.Parent }()

	c.checkStatementList(b.Statements)
}

func (c *Checker) checkIf(s *ast.IfStmt) {
	c.checkExpr(s.Cond)
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
	var elemType types.Type
	if arr, ok := iterableType.(*types.ArrayType); ok {
		elemType = arr.Elem
	} else if obj, ok := iterableType.(*types.ObjectType); ok && obj.Name == "$Headers" {
		elemType = types.NewArray(types.TypeString)
	} else {
		c.error(s.Iterable.Span(), "TS2488", fmt.Sprintf("Type '%s' is not iterable by the native array for-of lowering.", iterableType))
		return
	}
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
