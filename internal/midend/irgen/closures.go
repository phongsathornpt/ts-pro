package irgen

import (
	"fmt"
	"sort"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/ir"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func captureCellRuntimeType() *types.ObjectType {
	return types.NewObject("$JSValueCell")
}

func (g *generator) captureCell(name string) (ir.Operand, types.Type, bool) {
	if g.currentFn == nil {
		return nil, nil, false
	}
	cells := g.captureCells[g.currentFn]
	if cells == nil {
		return nil, nil, false
	}
	cell, ok := cells[name]
	if !ok {
		return nil, nil, false
	}
	return cell, g.captureCellTypes[g.currentFn][name], true
}

func (g *generator) bindCaptureCell(fn *ir.Function, name string, cell ir.Operand, valueType types.Type) {
	if g.captureCells[fn] == nil {
		g.captureCells[fn] = make(map[string]ir.Operand)
		g.captureCellTypes[fn] = make(map[string]types.Type)
	}
	g.captureCells[fn][name] = cell
	g.captureCellTypes[fn][name] = valueType
}

func (g *generator) ensureCaptureCell(name string) (ir.Operand, types.Type) {
	if cell, valueType, ok := g.captureCell(name); ok {
		return cell, valueType
	}
	value := g.locals[name]
	valueType := value.Type()
	cellType := captureCellRuntimeType()
	cell := g.currentFn.NewValue(name+"_cell", cellType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: cell, Callee: "ts_jsvalue_cell_new"})
	boxed := value
	if !irJSValueType(valueType) {
		boxed = g.boxJSValue(value, valueType)
	}
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_jsvalue_cell_set", Args: []ir.Operand{cell, boxed}, ParamTypes: []types.Type{cellType, types.TypeAny}})
	g.bindCaptureCell(g.currentFn, name, cell, valueType)
	return cell, valueType
}

func (g *generator) readLocal(name string) ir.Operand {
	cell, valueType, ok := g.captureCell(name)
	if !ok {
		return g.locals[name]
	}
	boxed := g.currentFn.NewValue(name+"_cell_value", types.TypeAny)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Res: boxed, Callee: "ts_jsvalue_cell_get", Args: []ir.Operand{cell}})
	return g.coerceJSValueBoundary(boxed, types.TypeAny, valueType)
}

func (g *generator) writeCapturedLocal(name string, value ir.Operand) bool {
	cell, _, ok := g.captureCell(name)
	if !ok {
		return false
	}
	boxed := value
	if !irJSValueType(value.Type()) {
		boxed = g.boxJSValue(value, value.Type())
	}
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.CallInst{Callee: "ts_jsvalue_cell_set", Args: []ir.Operand{cell, boxed}, ParamTypes: []types.Type{cell.Type(), types.TypeAny}})
	g.locals[name] = value
	return true
}

func (g *generator) collectArrowCaptures(expr ast.Expr, params map[string]struct{}) []string {
	found := make(map[string]struct{})
	var walk func(ast.Expr)
	walk = func(e ast.Expr) {
		switch n := e.(type) {
		case *ast.IdentExpr:
			if _, isParam := params[n.Name]; isParam {
				return
			}
			if _, ok := g.locals[n.Name]; ok {
				found[n.Name] = struct{}{}
			}
		case *ast.BinaryExpr:
			walk(n.Left)
			walk(n.Right)
		case *ast.UnaryExpr:
			walk(n.Target)
		case *ast.CallExpr:
			walk(n.Callee)
			for _, a := range n.Args {
				walk(a)
			}
		case *ast.MemberExpr:
			walk(n.Object)
		case *ast.IndexExpr:
			walk(n.Target)
			walk(n.Index)
		case *ast.ArrayLit:
			for _, el := range n.Elements {
				walk(el)
			}
		case *ast.SpreadExpr:
			walk(n.Value)
		case *ast.ObjectLit:
			for _, prop := range n.Properties {
				walk(prop.Value)
			}
		case *ast.AssignExpr:
			walk(n.Left)
			walk(n.Right)
		case *ast.TernaryExpr:
			walk(n.Cond)
			walk(n.Then)
			walk(n.Else)
		case *ast.ArrowFuncExpr:
			nestedParams := make(map[string]struct{}, len(n.Params))
			for _, p := range n.Params {
				nestedParams[p.Name] = struct{}{}
			}
			var nested []string
			if n.IsExprBody {
				nested = g.collectArrowCaptures(n.Body.(ast.Expr), nestedParams)
			} else {
				nested = g.collectBlockClosureCaptures(n.Body.(*ast.BlockStmt), nestedParams)
			}
			for _, name := range nested {
				if _, isParam := params[name]; !isParam {
					found[name] = struct{}{}
				}
			}
		case *ast.FunctionExpr:
			nestedParams := make(map[string]struct{}, len(n.Params))
			for _, p := range n.Params {
				if !p.IsThis {
					nestedParams[p.Name] = struct{}{}
				}
			}
			for _, name := range g.collectBlockClosureCaptures(n.Body, nestedParams) {
				if _, isParam := params[name]; !isParam {
					found[name] = struct{}{}
				}
			}
		}
	}
	walk(expr)
	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (g *generator) collectBlockClosureCaptures(block *ast.BlockStmt, params map[string]struct{}) []string {
	locals := make(map[string]struct{}, len(params))
	for name := range params {
		locals[name] = struct{}{}
	}
	var collectDecls func(ast.Stmt)
	collectDecls = func(stmt ast.Stmt) {
		if stmt == nil {
			return
		}
		switch n := stmt.(type) {
		case *ast.VarDeclStmt:
			for _, d := range n.Declarations {
				locals[d.Name] = struct{}{}
			}
		case *ast.ForOfStmt:
			locals[n.Name] = struct{}{}
			collectDecls(n.Body)
		case *ast.ForStmt:
			collectDecls(n.Init)
			collectDecls(n.Body)
		case *ast.BlockStmt:
			for _, child := range n.Statements {
				collectDecls(child)
			}
		case *ast.IfStmt:
			collectDecls(n.Then)
			collectDecls(n.Else)
		case *ast.WhileStmt:
			collectDecls(n.Body)
		case *ast.DoWhileStmt:
			collectDecls(n.Body)
		case *ast.SwitchStmt:
			for _, c := range n.Cases {
				for _, child := range c.Statements {
					collectDecls(child)
				}
			}
		case *ast.FunctionDecl:
			locals[n.Name] = struct{}{}
		}
	}
	collectDecls(block)

	found := make(map[string]struct{})
	var walkExpr func(ast.Expr)
	var walkStmt func(ast.Stmt)
	walkExpr = func(e ast.Expr) {
		if e == nil {
			return
		}
		switch n := e.(type) {
		case *ast.IdentExpr:
			if _, local := locals[n.Name]; local {
				return
			}
			if _, outer := g.locals[n.Name]; outer {
				found[n.Name] = struct{}{}
			}
		case *ast.BinaryExpr:
			walkExpr(n.Left)
			walkExpr(n.Right)
		case *ast.UnaryExpr:
			walkExpr(n.Target)
		case *ast.CallExpr:
			walkExpr(n.Callee)
			for _, a := range n.Args {
				walkExpr(a)
			}
		case *ast.MemberExpr:
			walkExpr(n.Object)
		case *ast.IndexExpr:
			walkExpr(n.Target)
			walkExpr(n.Index)
		case *ast.ArrayLit:
			for _, el := range n.Elements {
				walkExpr(el)
			}
		case *ast.SpreadExpr:
			walkExpr(n.Value)
		case *ast.ObjectLit:
			for _, prop := range n.Properties {
				walkExpr(prop.Value)
			}
		case *ast.AssignExpr:
			walkExpr(n.Left)
			walkExpr(n.Right)
		case *ast.TernaryExpr:
			walkExpr(n.Cond)
			walkExpr(n.Then)
			walkExpr(n.Else)
		case *ast.ArrowFuncExpr:
			nestedParams := make(map[string]struct{}, len(n.Params))
			for _, p := range n.Params {
				nestedParams[p.Name] = struct{}{}
			}
			var nested []string
			if n.IsExprBody {
				nested = g.collectArrowCaptures(n.Body.(ast.Expr), nestedParams)
			} else {
				nested = g.collectBlockClosureCaptures(n.Body.(*ast.BlockStmt), nestedParams)
			}
			for _, name := range nested {
				if _, shadowed := locals[name]; !shadowed {
					found[name] = struct{}{}
				}
			}
		case *ast.FunctionExpr:
			nestedParams := make(map[string]struct{}, len(n.Params))
			for _, p := range n.Params {
				if !p.IsThis {
					nestedParams[p.Name] = struct{}{}
				}
			}
			for _, name := range g.collectBlockClosureCaptures(n.Body, nestedParams) {
				if _, shadowed := locals[name]; !shadowed {
					found[name] = struct{}{}
				}
			}
		}
	}
	walkStmt = func(stmt ast.Stmt) {
		if stmt == nil {
			return
		}
		switch n := stmt.(type) {
		case *ast.BlockStmt:
			for _, child := range n.Statements {
				walkStmt(child)
			}
		case *ast.VarDeclStmt:
			for _, d := range n.Declarations {
				walkExpr(d.Init)
			}
		case *ast.ExprStmt:
			walkExpr(n.Expr)
		case *ast.ReturnStmt:
			walkExpr(n.Value)
		case *ast.IfStmt:
			walkExpr(n.Cond)
			walkStmt(n.Then)
			walkStmt(n.Else)
		case *ast.WhileStmt:
			walkExpr(n.Cond)
			walkStmt(n.Body)
		case *ast.DoWhileStmt:
			walkStmt(n.Body)
			walkExpr(n.Cond)
		case *ast.ForStmt:
			walkStmt(n.Init)
			walkExpr(n.Cond)
			walkExpr(n.Post)
			walkStmt(n.Body)
		case *ast.ForOfStmt:
			walkExpr(n.Iterable)
			walkStmt(n.Body)
		case *ast.SwitchStmt:
			walkExpr(n.Expr)
			for _, c := range n.Cases {
				walkExpr(c.Test)
				for _, child := range c.Statements {
					walkStmt(child)
				}
			}
		case *ast.FunctionDecl:
			// Nested declarations own their body.
		}
	}
	walkStmt(block)
	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (g *generator) lowerArrowExpr(e *ast.ArrowFuncExpr) ir.Operand {
	fnType := g.semanticType(e).(*types.FunctionType)
	paramSet := make(map[string]struct{}, len(e.Params))
	for _, p := range e.Params {
		paramSet[p.Name] = struct{}{}
	}
	var captureNames []string
	if e.IsExprBody {
		body := e.Body.(ast.Expr)
		captureNames = g.collectArrowCaptures(body, paramSet)
	} else {
		body := e.Body.(*ast.BlockStmt)
		captureNames = g.collectBlockClosureCaptures(body, paramSet)
	}
	captureOps := make([]ir.Operand, 0, len(captureNames))
	captureTypes := make([]types.Type, 0, len(captureNames))
	var refMask uint64
	for i, name := range captureNames {
		cell, valueType := g.ensureCaptureCell(name)
		captureOps = append(captureOps, cell)
		captureTypes = append(captureTypes, valueType)
		// Capture cells are always GC-managed references.
		refMask |= uint64(1) << i
	}

	outerFn, outerBB, outerLocals, outerProvenance, outerDirectCallees := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	name := fmt.Sprintf("$arrow%d", g.arrowCounter)
	g.arrowCounter++
	lifted := ir.NewFunction(name, fnType.Return)
	g.currentFn = lifted
	g.currentBB = lifted.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)

	env := lifted.NewValue("$env", fnType)
	lifted.Params = append(lifted.Params, env)
	for i, captureName := range captureNames {
		cellType := captureOps[i].Type()
		cell := lifted.NewValue(captureName+"_cell_capture", cellType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ClosureGetInst{Res: cell, Closure: env, Index: i})
		// Keep the name visible to nested capture analysis while all reads/writes
		// route through the shared cell.
		g.locals[captureName] = cell
		g.bindCaptureCell(lifted, captureName, cell, captureTypes[i])
	}
	savedStreamKind := g.activeStreamControllerKind
	g.activeStreamControllerKind = ""
	for i, p := range e.Params {
		pt := types.TypeAny
		if i < len(fnType.Params) {
			pt = fnType.Params[i].Type
		}
		v := lifted.NewValue(p.Name, pt)
		lifted.Params = append(lifted.Params, v)
		g.locals[p.Name] = v
		if savedStreamKind != "" && g.semaResult != nil {
			switch savedStreamKind {
			case "readable":
				if i == 0 && g.semaResult.ReadableStreamDefaultControllerType != nil {
					g.localProvenance[p.Name] = g.semaResult.ReadableStreamDefaultControllerType
				}
			case "transform":
				if (i == 1 || (i == 0 && len(e.Params) == 1)) && g.semaResult.TransformStreamDefaultControllerType != nil {
					g.localProvenance[p.Name] = g.semaResult.TransformStreamDefaultControllerType
				}
			case "writable":
				if (i == 1 || (i == 0 && len(e.Params) == 1)) && g.semaResult.WritableStreamDefaultControllerType != nil {
					g.localProvenance[p.Name] = g.semaResult.WritableStreamDefaultControllerType
				}
			}
		}
	}
	if e.IsExprBody {
		body := e.Body.(ast.Expr)
		ret := g.lowerExpr(body)
		if fnType.Return == types.TypeVoid {
			if g.currentBB.Terminator == nil {
				g.currentBB.Terminator = &ir.ReturnTerm{}
			}
		} else {
			ret = g.coerceJSValueBoundary(ret, g.semanticType(body), fnType.Return)
			if g.currentBB.Terminator == nil {
				g.currentBB.Terminator = &ir.ReturnTerm{Val: ret}
			}
		}
	} else {
		body := e.Body.(*ast.BlockStmt)
		for _, stmt := range body.Statements {
			g.lowerStatement(stmt)
		}
		if g.currentBB.Terminator == nil {
			g.currentBB.Terminator = &ir.ReturnTerm{}
		}
	}
	g.prog.Functions = append(g.prog.Functions, lifted)

	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProvenance, outerDirectCallees
	g.activeStreamControllerKind = savedStreamKind
	res := g.currentFn.NewValue("closure", fnType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{
		Res: res, Function: name, Captures: captureOps, RefMask: refMask,
	})
	return res
}

func (g *generator) lowerFunctionExpr(e *ast.FunctionExpr) ir.Operand {
	fnType := g.semanticType(e).(*types.FunctionType)
	paramSet := make(map[string]struct{}, len(e.Params))
	for _, p := range e.Params {
		if !p.IsThis {
			paramSet[p.Name] = struct{}{}
		}
	}
	captureNames := g.collectBlockClosureCaptures(e.Body, paramSet)
	captureOps := make([]ir.Operand, 0, len(captureNames))
	captureTypes := make([]types.Type, 0, len(captureNames))
	var refMask uint64
	for i, captureName := range captureNames {
		cell, valueType := g.ensureCaptureCell(captureName)
		captureOps = append(captureOps, cell)
		captureTypes = append(captureTypes, valueType)
		refMask |= uint64(1) << i
	}
	outerFn, outerBB, outerLocals, outerProvenance, outerDirectCallees := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee
	name := fmt.Sprintf("$function%d", g.arrowCounter)
	g.arrowCounter++
	lifted := ir.NewFunction(name, fnType.Return)
	g.currentFn = lifted
	g.currentBB = lifted.NewBlock("entry")
	g.locals = make(map[string]ir.Operand)
	g.localProvenance = make(map[string]types.Type)
	g.localDirectCallee = make(map[string]string)

	env := lifted.NewValue("$env", fnType)
	lifted.Params = append(lifted.Params, env)
	for i, captureName := range captureNames {
		cellType := captureOps[i].Type()
		cell := lifted.NewValue(captureName+"_cell_capture", cellType)
		g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.ClosureGetInst{Res: cell, Closure: env, Index: i})
		g.locals[captureName] = cell
		g.bindCaptureCell(lifted, captureName, cell, captureTypes[i])
	}
	if fnType.This != nil {
		thisVal := lifted.NewValue("$this", fnType.This)
		lifted.Params = append(lifted.Params, thisVal)
		g.locals["$this"] = thisVal
	}
	savedStreamKind := g.activeStreamControllerKind
	g.activeStreamControllerKind = ""
	runtimeIndex := 0
	for _, p := range e.Params {
		if p.IsThis {
			continue
		}
		pt := types.TypeAny
		if runtimeIndex < len(fnType.Params) {
			pt = fnType.Params[runtimeIndex].Type
		}
		v := lifted.NewValue(p.Name, pt)
		lifted.Params = append(lifted.Params, v)
		g.locals[p.Name] = v
		if savedStreamKind != "" && g.semaResult != nil {
			switch savedStreamKind {
			case "readable":
				if runtimeIndex == 0 && g.semaResult.ReadableStreamDefaultControllerType != nil {
					g.localProvenance[p.Name] = g.semaResult.ReadableStreamDefaultControllerType
				}
			case "transform":
				if (runtimeIndex == 1 || (runtimeIndex == 0 && len(e.Params) == 1)) && g.semaResult.TransformStreamDefaultControllerType != nil {
					g.localProvenance[p.Name] = g.semaResult.TransformStreamDefaultControllerType
				}
			case "writable":
				if (runtimeIndex == 1 || (runtimeIndex == 0 && len(e.Params) == 1)) && g.semaResult.WritableStreamDefaultControllerType != nil {
					g.localProvenance[p.Name] = g.semaResult.WritableStreamDefaultControllerType
				}
			}
		}
		runtimeIndex++
	}
	for _, stmt := range e.Body.Statements {
		g.lowerStatement(stmt)
	}
	if g.currentBB.Terminator == nil {
		g.currentBB.Terminator = &ir.ReturnTerm{}
	}
	g.prog.Functions = append(g.prog.Functions, lifted)
	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.localDirectCallee = outerFn, outerBB, outerLocals, outerProvenance, outerDirectCallees
	g.activeStreamControllerKind = savedStreamKind
	res := g.currentFn.NewValue("closure", fnType)
	g.currentBB.Instructions = append(g.currentBB.Instructions, &ir.MakeClosureInst{Res: res, Function: name, Captures: captureOps, RefMask: refMask})
	return res
}
