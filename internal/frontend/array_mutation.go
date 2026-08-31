package frontend

import (
	"fmt"

	"github.com/projectthorn/tsv7-bin/internal/tsast"
)

func (e *extractor) buildArrayAssignment(node, target, rhs tsast.Node) (Statement, bool, error) {
	arrayNode, ok := target.NamedChild("expression")
	if !ok {
		return Statement{}, true, fmt.Errorf("array assignment at %d has no receiver", target.Pos())
	}
	indexNode, ok := target.NamedChild("argumentExpression")
	if !ok {
		return Statement{}, true, fmt.Errorf("array assignment at %d has no index", target.Pos())
	}
	array, err := e.extractExpr(arrayNode)
	if err != nil {
		return Statement{}, true, err
	}
	index, err := e.extractExpr(indexNode)
	if err != nil {
		return Statement{}, true, err
	}
	value, err := e.extractExpr(rhs)
	if err != nil {
		return Statement{}, true, err
	}
	if int(array.Type) >= len(e.result.Types) {
		return Statement{}, true, fmt.Errorf("array assignment has invalid receiver type t%d", array.Type)
	}
	arrayType := e.result.Types[array.Type]
	if arrayType.Kind != TypeArray || int(arrayType.Element) >= len(e.result.Types) || e.result.Types[arrayType.Element].Kind != TypeNumber {
		return Statement{}, true, fmt.Errorf("native indexed assignment currently requires number[]")
	}
	if int(index.Type) >= len(e.result.Types) || e.result.Types[index.Type].Kind != TypeNumber {
		return Statement{}, true, fmt.Errorf("native array index must be number")
	}
	if int(value.Type) >= len(e.result.Types) || e.result.Types[value.Type].Kind != TypeNumber {
		return Statement{}, true, fmt.Errorf("native number[] assignment requires number value")
	}
	return Statement{
		Kind: StmtArrayAssign, Span: e.span(node), Type: value.Type,
		Object: array, Index: index, Value: value,
	}, true, nil
}
