package frontend

import (
	"fmt"
	"sort"

	"github.com/projectthorn/tsv7-bin/internal/tsast"
	"github.com/projectthorn/tsv7-bin/internal/tsls"
)

type closureInfo struct {
	Function FunctionID
	Captures []SymbolID
}

func (e *extractor) extractLocalClosure(name string, variable *tsls.APISymbol, node tsast.Node) (closureInfo, error) {
	fnID := FunctionID(len(e.result.Functions))
	fn := Function{ID: fnID, Name: fmt.Sprintf("closure$%s$%d", name, fnID), Source: 0, Span: e.span(node)}
	if variable != nil {
		fn.Symbol = e.internSymbol(variable, SymbolVariable, node)
	}

	parameterAPISymbols := map[uint64]struct{}{}
	var explicit []Parameter
	if params, ok := node.NamedChild("parameters"); ok && params.IsList() {
		for _, paramNode := range params.ListElements() {
			param, err := e.extractParameter(paramNode)
			if err != nil {
				return closureInfo{}, err
			}
			explicit = append(explicit, param)
			nameNode, ok := paramNode.NamedChild("name")
			if !ok {
				return closureInfo{}, fmt.Errorf("closure parameter at %d has no name", paramNode.Pos())
			}
			symbol, err := e.client.GetSymbolAtLocation(e.ctx, e.snapshot, e.project, nameNode.Handle(e.fileName))
			if err != nil {
				return closureInfo{}, err
			}
			if symbol != nil {
				parameterAPISymbols[symbol.ID] = struct{}{}
			}
		}
	}
	returnTypeNode, ok := node.NamedChild("type")
	if !ok {
		return closureInfo{}, fmt.Errorf("closure %s requires an explicit return type", name)
	}
	returnType, err := e.typeAt(returnTypeNode)
	if err != nil {
		return closureInfo{}, err
	}
	fn.ReturnType = returnType
	bodyNode, ok := node.NamedChild("body")
	if !ok {
		return closureInfo{}, fmt.Errorf("closure %s has no body", name)
	}
	captures, err := e.collectClosureCaptures(bodyNode, parameterAPISymbols)
	if err != nil {
		return closureInfo{}, err
	}
	for _, symbolID := range captures {
		symbol := e.result.Symbols[symbolID]
		fn.Params = append(fn.Params, Parameter{Symbol: symbolID, Name: symbol.Name, Type: symbol.Type, Span: symbol.Decl})
	}
	fn.Params = append(fn.Params, explicit...)
	e.result.Functions = append(e.result.Functions, fn)

	var body []Statement
	if bodyNode.Kind() == tsast.KindBlock {
		body, err = e.extractBlock(bodyNode)
	} else {
		value, bodyErr := e.extractExpr(bodyNode)
		if bodyErr != nil {
			err = bodyErr
		} else {
			body = []Statement{{Kind: StmtReturn, Span: e.span(bodyNode), Return: value}}
		}
	}
	if err != nil {
		return closureInfo{}, err
	}
	e.result.Functions[fnID].Body = body
	return closureInfo{Function: fnID, Captures: captures}, nil
}

func (e *extractor) collectClosureCaptures(body tsast.Node, excluded map[uint64]struct{}) ([]SymbolID, error) {
	captured := map[SymbolID]struct{}{}
	var visit func(tsast.Node) error
	visit = func(node tsast.Node) error {
		if node.Kind() == tsast.KindThisKeyword && e.currentThis != nil {
			captured[*e.currentThis] = struct{}{}
		}
		if node.Kind() == tsast.KindIdentifier {
			symbol, err := e.client.GetSymbolAtLocation(e.ctx, e.snapshot, e.project, node.Handle(e.fileName))
			if err != nil {
				return err
			}
			if symbol != nil {
				if _, skip := excluded[symbol.ID]; !skip {
					if id, ok := e.symbols[symbol.ID]; ok {
						kind := e.result.Symbols[id].Kind
						if kind == SymbolVariable || kind == SymbolParameter {
							captured[id] = struct{}{}
						}
					}
				}
			}
		}
		for _, child := range node.Children() {
			if err := visit(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(body); err != nil {
		return nil, err
	}
	result := make([]SymbolID, 0, len(captured))
	for id := range captured {
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}

func (e *extractor) closureCaptureArgs(info closureInfo, span Span) []*Expr {
	args := make([]*Expr, 0, len(info.Captures))
	for _, id := range info.Captures {
		symbol := e.result.Symbols[id]
		args = append(args, &Expr{Kind: ExprIdentifier, Type: symbol.Type, Symbol: id, Name: symbol.Name, Span: span})
	}
	return args
}
