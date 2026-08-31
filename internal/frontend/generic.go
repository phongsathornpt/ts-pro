package frontend

import (
	"fmt"
	"strings"

	"github.com/projectthorn/tsv7-bin/internal/tsast"
	"github.com/projectthorn/tsv7-bin/internal/tsls"
)

type genericInfo struct {
	Name   string
	Symbol *tsls.APISymbol
	Node   tsast.Node
}

func (e *extractor) registerGenericFunction(node tsast.Node) error {
	nameNode, ok := node.NamedChild("name")
	if !ok || nameNode.Kind() != tsast.KindIdentifier {
		return fmt.Errorf("generic function at %d requires a name", node.Pos())
	}
	name, _ := nameNode.Text()
	symbol, err := e.client.GetSymbolAtLocation(e.ctx, e.snapshot, e.project, nameNode.Handle(e.fileName))
	if err != nil || symbol == nil {
		if err == nil {
			err = fmt.Errorf("generic function %s has no TypeScript symbol", name)
		}
		return err
	}
	e.generics[symbol.ID] = genericInfo{Name: name, Symbol: symbol, Node: node}
	return nil
}

func hasTypeParameters(node tsast.Node) bool {
	params, ok := node.NamedChild("typeParameters")
	return ok && params.IsList() && len(params.ListElements()) != 0
}

func (e *extractor) specializeGenericCall(info genericInfo, args []*Expr, resultType TypeID) (FunctionID, error) {
	key := genericSpecializationKey(info.Symbol.ID, args, resultType)
	if id, ok := e.specializations[key]; ok {
		return id, nil
	}
	paramsNode, ok := info.Node.NamedChild("parameters")
	if !ok || !paramsNode.IsList() {
		return 0, fmt.Errorf("generic function %s has no parameter list", info.Name)
	}
	paramNodes := paramsNode.ListElements()
	if len(paramNodes) != len(args) {
		return 0, fmt.Errorf("generic function %s requires %d arguments; got %d", info.Name, len(paramNodes), len(args))
	}
	functionID := FunctionID(len(e.result.Functions))
	e.specializations[key] = functionID
	return e.buildGenericSpecialization(functionID, info, paramNodes, args, resultType)
}

const typeFlagTypeParameter uint32 = 524288

func (e *extractor) buildGenericSpecialization(id FunctionID, info genericInfo, paramNodes []tsast.Node, args []*Expr, resultType TypeID) (FunctionID, error) {
	nameNode, _ := info.Node.NamedChild("name")
	fn := Function{ID: id, Symbol: e.internSymbol(info.Symbol, SymbolFunction, nameNode),
		Name: genericSpecializationName(info.Name, args), Source: 0, Span: e.span(info.Node), ReturnType: resultType}
	typeSubs := map[uint64]TypeID{}
	symbolSubs := map[uint64]SymbolID{}
	for i, paramNode := range paramNodes {
		param, origType, origSymbol, err := e.specializeGenericParameter(paramNode, args[i].Type)
		if err != nil {
			return 0, err
		}
		fn.Params = append(fn.Params, param)
		symbolSubs[origSymbol.ID] = param.Symbol
		if origType.Flags&typeFlagTypeParameter != 0 {
			typeSubs[origType.ID] = args[i].Type
		}
	}
	e.result.Functions = append(e.result.Functions, fn)
	return e.extractGenericSpecializedBody(id, info.Node, typeSubs, symbolSubs)
}

func (e *extractor) specializeGenericParameter(node tsast.Node, concrete TypeID) (Parameter, *tsls.APIType, *tsls.APISymbol, error) {
	nameNode, ok := node.NamedChild("name")
	if !ok || nameNode.Kind() != tsast.KindIdentifier {
		return Parameter{}, nil, nil, fmt.Errorf("generic parameter at %d requires an identifier", node.Pos())
	}
	name, _ := nameNode.Text()
	symbol, err := e.client.GetSymbolAtLocation(e.ctx, e.snapshot, e.project, nameNode.Handle(e.fileName))
	if err != nil || symbol == nil {
		return Parameter{}, nil, nil, fmt.Errorf("generic parameter %s symbol: %w", name, err)
	}
	apiType, err := e.client.GetTypeAtLocation(e.ctx, e.snapshot, e.project, nameNode.Handle(e.fileName))
	if err != nil || apiType == nil {
		return Parameter{}, nil, nil, fmt.Errorf("generic parameter %s type: %w", name, err)
	}
	if apiType.Flags&typeFlagTypeParameter == 0 {
		declared, err := e.internAPIType(apiType)
		if err != nil || declared != concrete {
			return Parameter{}, nil, nil, fmt.Errorf("generic MVP parameter %s must be a direct type parameter", name)
		}
	}
	synthetic := e.newSyntheticSymbol(name, SymbolParameter, concrete, node)
	return Parameter{Symbol: synthetic, Name: name, Type: concrete, Span: e.span(node)}, apiType, symbol, nil
}

func (e *extractor) extractGenericSpecializedBody(id FunctionID, node tsast.Node, typeSubs map[uint64]TypeID, symbolSubs map[uint64]SymbolID) (FunctionID, error) {
	oldTypes, oldSymbols := e.typeSubstitutions, e.symbolSubstitutions
	e.typeSubstitutions, e.symbolSubstitutions = typeSubs, symbolSubs
	body, err := e.extractFunctionBody(node)
	e.typeSubstitutions, e.symbolSubstitutions = oldTypes, oldSymbols
	if err != nil {
		return 0, err
	}
	e.result.Functions[id].Body = body
	return id, nil
}

func genericSpecializationKey(symbol uint64, args []*Expr, result TypeID) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d", symbol)
	for _, arg := range args {
		fmt.Fprintf(&b, ":%d", arg.Type)
	}
	fmt.Fprintf(&b, "->%d", result)
	return b.String()
}

func genericSpecializationName(name string, args []*Expr) string {
	var b strings.Builder
	b.WriteString(name)
	for _, arg := range args {
		fmt.Fprintf(&b, "$t%d", arg.Type)
	}
	return b.String()
}
