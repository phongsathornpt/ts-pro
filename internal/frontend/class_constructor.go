package frontend

import (
	"fmt"

	"github.com/projectthorn/tsv7-bin/internal/tsast"
)

func (e *extractor) finalizeConstructorSignature(info *classInfo) error {
	voidType := e.ensureSemanticType(TypeVoid, "void")
	functionID := FunctionID(len(e.result.Functions))
	thisSymbol := e.newSyntheticSymbol("this", SymbolParameter, info.Type, info.Node)
	fn := Function{
		ID: functionID, Name: info.Name + ".constructor", Source: 0,
		Span: e.span(info.Node), ReturnType: voidType,
	}
	fn.Params = append(fn.Params, Parameter{
		Symbol: thisSymbol, Name: "this", Type: info.Type, Span: e.span(info.Node),
	})
	fn.Params = append(fn.Params, info.ConstructorParams...)
	e.result.Functions = append(e.result.Functions, fn)
	info.Constructor = functionID
	info.ConstructorThis = thisSymbol
	thisCopy := thisSymbol
	e.pending = append(e.pending, pendingFunctionBody{
		Function: functionID, Node: info.ConstructorNode,
		This: &thisCopy, Constructor: info,
	})
	return nil
}

func (e *extractor) extractNativeConstructorBody(info *classInfo) ([]Statement, error) {
	previousAliases := e.parameterAliases
	e.parameterAliases = make(map[string]SymbolID, len(info.ConstructorParams))
	for _, param := range info.ConstructorParams {
		e.parameterAliases[param.Name] = param.Symbol
	}
	defer func() { e.parameterAliases = previousAliases }()

	var sourceStatements []tsast.Node
	if info.HasConstructor {
		block, ok := info.ConstructorNode.NamedChild("body")
		if !ok {
			return nil, fmt.Errorf("constructor %s has no body", info.Name)
		}
		if statements, ok := block.NamedChild("statements"); ok && statements.IsList() {
			sourceStatements = statements.ListElements()
		}
	}

	body := make([]Statement, 0, len(info.Initializers)+len(info.ConstructorParams)+len(sourceStatements)+1)
	if info.Base != nil {
		superStmt, remaining, err := e.extractSuperConstructorStatement(info, sourceStatements)
		if err != nil {
			return nil, err
		}
		body = append(body, superStmt)
		sourceStatements = remaining
	}
	for _, initializer := range info.Initializers {
		value, err := e.extractExpr(initializer.Node)
		if err != nil {
			return nil, err
		}
		field := e.result.Shapes[info.Shape].Fields[initializer.Field]
		body = append(body, e.constructorFieldStore(info, initializer.Field, field.Name, field.Type, value))
	}
	for fieldIndex, paramIndex := range info.FieldParam {
		if paramIndex < 0 {
			continue
		}
		param := info.ConstructorParams[paramIndex]
		value := &Expr{Kind: ExprIdentifier, Type: param.Type, Symbol: param.Symbol, Name: param.Name, Span: param.Span}
		field := e.result.Shapes[info.Shape].Fields[fieldIndex]
		body = append(body, e.constructorFieldStore(info, fieldIndex, field.Name, field.Type, value))
	}
	for _, node := range sourceStatements {
		stmt, err := e.extractStatement(node)
		if err != nil {
			return nil, fmt.Errorf("constructor %s: %w", info.Name, err)
		}
		body = append(body, stmt)
	}
	return body, nil
}

func (e *extractor) constructorFieldStore(info *classInfo, fieldIndex int, name string, typeID TypeID, value *Expr) Statement {
	object := &Expr{
		Kind: ExprIdentifier, Type: info.Type, Symbol: info.ConstructorThis,
		Name: "this", Span: e.span(info.Node),
	}
	return Statement{
		Kind: StmtFieldAssign, Span: e.span(info.Node), Type: typeID,
		Object: object, Field: name, FieldIndex: uint32(fieldIndex), Value: value,
	}
}

func (e *extractor) buildFieldAssignment(node, target, rhs tsast.Node) (Statement, bool, error) {
	objectNode, ok := target.NamedChild("expression")
	if !ok {
		return Statement{}, true, fmt.Errorf("field assignment at %d has no receiver", target.Pos())
	}
	nameNode, ok := target.NamedChild("name")
	if !ok || nameNode.Kind() != tsast.KindIdentifier {
		return Statement{}, true, fmt.Errorf("field assignment at %d has invalid field name", target.Pos())
	}
	name, _ := nameNode.Text()
	object, err := e.extractExpr(objectNode)
	if err != nil {
		return Statement{}, true, err
	}
	if int(object.Type) >= len(e.result.Types) {
		return Statement{}, true, fmt.Errorf("field assignment %s has invalid receiver type", name)
	}
	typ := e.result.Types[object.Type]
	value, err := e.extractExpr(rhs)
	if err != nil {
		return Statement{}, true, err
	}
	if typ.Kind == TypeAny || typ.Kind == TypeUnion {
		return Statement{
			Kind: StmtDynamicFieldAssign, Span: e.span(node), Type: e.ensureSemanticType(TypeAny, "any"),
			Object: object, Field: name, Value: value,
		}, true, nil
	}
	if typ.Kind != TypeObject || int(typ.Shape) >= len(e.result.Shapes) {
		return Statement{}, true, fmt.Errorf("field assignment %s requires a closed object", name)
	}
	shape := e.result.Shapes[typ.Shape]
	for fieldIndex, field := range shape.Fields {
		if field.Name != name {
			continue
		}
		return Statement{
			Kind: StmtFieldAssign, Span: e.span(node), Type: field.Type,
			Object: object, Field: name, FieldIndex: uint32(fieldIndex), Value: value,
		}, true, nil
	}
	return Statement{}, true, fmt.Errorf("shape %s has no mutable field %s", shape.Name, name)
}
