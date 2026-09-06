package irgen

import (
	"fmt"
	"strings"

	"github.com/phongsathornpt/ts-pro/internal/core/ast"
	"github.com/phongsathornpt/ts-pro/internal/core/types"
)

func genericSpecializationKey(name string, fn *types.FunctionType) string {
	parts := make([]string, len(fn.Params))
	for i, p := range fn.Params {
		parts[i] = p.Type.String()
	}
	return name + "(" + strings.Join(parts, ",") + ")->" + fn.Return.String()
}

func (g *generator) ensureGenericSpecialization(decl *ast.FunctionDecl, concrete *types.FunctionType) string {
	generic := g.semaResult.Types[decl].(*types.FunctionType)
	bindings, _ := types.FunctionBindings(generic, concrete)
	key := genericSpecializationKey(decl.Name, concrete)
	if name, ok := g.genericSpecs[key]; ok {
		return name
	}
	name := fmt.Sprintf("%s$spec%d", decl.Name, g.genericSpecCount)
	g.genericSpecCount++
	// Register before lowering so recursive calls reuse this specialization.
	g.genericSpecs[key] = name

	outerFn, outerBB, outerLocals, outerProvenance, outerBindings := g.currentFn, g.currentBB, g.locals, g.localProvenance, g.typeBindings
	g.typeBindings = bindings
	fn, _ := g.lowerFunctionAs(decl, concrete, name)
	g.currentFn, g.currentBB, g.locals, g.localProvenance, g.typeBindings = outerFn, outerBB, outerLocals, outerProvenance, outerBindings
	g.prog.Functions = append(g.prog.Functions, fn)
	return name
}
