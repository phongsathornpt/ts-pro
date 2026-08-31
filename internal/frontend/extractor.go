package frontend

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/projectthorn/tsv7-bin/internal/tsast"
	"github.com/projectthorn/tsv7-bin/internal/tsls"
)

type extractor struct {
	ctx        context.Context
	client     *tsls.APIClient
	snapshot   uint64
	project    string
	fileName   string
	sourceText string
	result     Snapshot
	types      map[uint64]TypeID
	symbols    map[uint64]SymbolID
	functions  map[uint64]FunctionID
	shapes     map[uint64]ShapeID
}

func ExtractFile(ctx context.Context, client *tsls.APIClient, snapshot uint64, project, fileName string) (Snapshot, error) {
	payload, err := client.GetSourceFile(ctx, snapshot, project, fileName)
	if err != nil {
		return Snapshot{}, err
	}
	file, err := tsast.Decode(payload)
	if err != nil {
		return Snapshot{}, err
	}
	text, ok := file.Root().Text()
	if !ok {
		return Snapshot{}, fmt.Errorf("decode source text for %s", fileName)
	}
	abs, err := filepath.Abs(fileName)
	if err != nil {
		return Snapshot{}, fmt.Errorf("resolve source path: %w", err)
	}
	e := &extractor{
		ctx:        ctx,
		client:     client,
		snapshot:   snapshot,
		project:    project,
		fileName:   abs,
		sourceText: text,
		types:      map[uint64]TypeID{},
		symbols:    map[uint64]SymbolID{},
		functions:  map[uint64]FunctionID{},
		shapes:     map[uint64]ShapeID{},
	}
	e.result.Sources = append(e.result.Sources, Source{ID: 0, URI: "file://" + filepath.ToSlash(abs), Path: abs})

	var declarations []tsast.Node
	for _, node := range file.Root().Children() {
		if node.Kind() == tsast.KindFunctionDeclaration {
			declarations = append(declarations, node)
		}
	}
	for _, node := range declarations {
		if err := e.extractFunctionSignature(node); err != nil {
			return Snapshot{}, err
		}
	}
	for i, node := range declarations {
		body, err := e.extractFunctionBody(node)
		if err != nil {
			return Snapshot{}, err
		}
		e.result.Functions[i].Body = body
	}
	for _, node := range file.Root().Children() {
		switch node.Kind() {
		case tsast.KindFunctionDeclaration, tsast.KindInterfaceDeclaration, tsast.KindTypeAliasDeclaration, tsast.KindEndOfFile:
			continue
		case tsast.KindExpressionStatement:
			stmt, err := e.extractStatement(node)
			if err != nil {
				return Snapshot{}, err
			}
			e.result.Entry = append(e.result.Entry, stmt)
		default:
			return Snapshot{}, fmt.Errorf("unsupported top-level native statement %s at %d", tsast.KindName(node.Kind()), node.Pos())
		}
	}
	return e.result, nil
}

func (e *extractor) extractFunctionSignature(node tsast.Node) error {
	nameNode, ok := node.NamedChild("name")
	if !ok {
		return fmt.Errorf("function declaration at %d has no name", node.Pos())
	}
	name, _ := nameNode.Text()
	symbol, err := e.client.GetSymbolAtLocation(e.ctx, e.snapshot, e.project, nameNode.Handle(e.fileName))
	if err != nil {
		return err
	}
	if symbol == nil {
		return fmt.Errorf("function %s has no TypeScript symbol", name)
	}
	functionID := FunctionID(len(e.result.Functions))
	functionSymbol := e.internSymbol(symbol, SymbolFunction, nameNode)
	e.functions[symbol.ID] = functionID

	fn := Function{
		ID:       functionID,
		Symbol:   functionSymbol,
		Name:     name,
		Source:   0,
		Span:     e.span(node),
		Exported: hasModifier(node, tsast.KindExportKeyword),
	}
	if params, ok := node.NamedChild("parameters"); ok {
		for _, paramNode := range params.ListElements() {
			param, err := e.extractParameter(paramNode)
			if err != nil {
				return err
			}
			fn.Params = append(fn.Params, param)
		}
	}
	returnTypeNode, ok := node.NamedChild("type")
	if !ok {
		return fmt.Errorf("function %s requires an explicit return type for native lowering", name)
	}
	returnType, err := e.typeAt(returnTypeNode)
	if err != nil {
		return err
	}
	fn.ReturnType = returnType
	e.result.Functions = append(e.result.Functions, fn)
	return nil
}

func (e *extractor) extractParameter(node tsast.Node) (Parameter, error) {
	nameNode, ok := node.NamedChild("name")
	if !ok {
		return Parameter{}, fmt.Errorf("parameter at %d has no name", node.Pos())
	}
	name, _ := nameNode.Text()
	symbol, err := e.client.GetSymbolAtLocation(e.ctx, e.snapshot, e.project, nameNode.Handle(e.fileName))
	if err != nil {
		return Parameter{}, err
	}
	if symbol == nil {
		return Parameter{}, fmt.Errorf("parameter %s has no TypeScript symbol", name)
	}
	typeID, err := e.typeAt(nameNode)
	if err != nil {
		return Parameter{}, err
	}
	symbolID := e.internSymbol(symbol, SymbolParameter, nameNode)
	e.result.Symbols[symbolID].Type = typeID
	return Parameter{Symbol: symbolID, Name: name, Type: typeID, Span: e.span(node)}, nil
}

func (e *extractor) extractFunctionBody(node tsast.Node) ([]Statement, error) {
	body, ok := node.NamedChild("body")
	if !ok {
		return nil, fmt.Errorf("function at %d has no body", node.Pos())
	}
	return e.extractBlock(body)
}

func (e *extractor) extractBlock(block tsast.Node) ([]Statement, error) {
	statements, ok := block.NamedChild("statements")
	if !ok {
		return nil, nil
	}
	var result []Statement
	for _, node := range statements.ListElements() {
		stmt, err := e.extractStatement(node)
		if err != nil {
			return nil, err
		}
		result = append(result, stmt)
	}
	return result, nil
}

func (e *extractor) extractStatement(node tsast.Node) (Statement, error) {
	switch node.Kind() {
	case tsast.KindReturnStatement:
		stmt := Statement{Kind: StmtReturn, Span: e.span(node)}
		if exprNode, ok := node.NamedChild("expression"); ok {
			expr, err := e.extractExpr(exprNode)
			if err != nil {
				return Statement{}, err
			}
			stmt.Return = expr
		}
		return stmt, nil
	case tsast.KindIfStatement:
		conditionNode, _ := node.NamedChild("expression")
		condition, err := e.extractExpr(conditionNode)
		if err != nil {
			return Statement{}, err
		}
		thenNode, ok := node.NamedChild("thenStatement")
		if !ok {
			return Statement{}, fmt.Errorf("if statement at %d has no then branch", node.Pos())
		}
		thenBody, err := e.extractStatementBody(thenNode)
		if err != nil {
			return Statement{}, err
		}
		stmt := Statement{Kind: StmtIf, Span: e.span(node), Expr: condition, Then: thenBody}
		if elseNode, ok := node.NamedChild("elseStatement"); ok {
			stmt.Else, err = e.extractStatementBody(elseNode)
			if err != nil {
				return Statement{}, err
			}
		}
		return stmt, nil
	case tsast.KindBlock:
		body, err := e.extractBlock(node)
		return Statement{Kind: StmtBlock, Span: e.span(node), Then: body}, err
	case tsast.KindVariableStatement:
		items, err := e.extractVariableStatement(node)
		if err != nil {
			return Statement{}, err
		}
		if len(items) == 1 {
			return items[0], nil
		}
		return Statement{Kind: StmtBlock, Span: e.span(node), Then: items}, nil
	case tsast.KindWhileStatement:
		return e.extractWhile(node)
	case tsast.KindForStatement:
		return e.extractFor(node)
	case tsast.KindExpressionStatement:
		exprNode, ok := node.NamedChild("expression")
		if !ok {
			return Statement{}, fmt.Errorf("expression statement at %d has no expression", node.Pos())
		}
		if mutation, ok, err := e.extractMutation(exprNode); ok || err != nil {
			return mutation, err
		}
		expr, err := e.extractExpr(exprNode)
		if err != nil {
			return Statement{}, err
		}
		return Statement{Kind: StmtExpr, Span: e.span(node), Expr: expr}, nil
	default:
		return Statement{}, fmt.Errorf("unsupported native statement %s at %d", tsast.KindName(node.Kind()), node.Pos())
	}
}

func (e *extractor) extractVariableStatement(node tsast.Node) ([]Statement, error) {
	list, ok := node.NamedChild("declarationList")
	if !ok {
		return nil, fmt.Errorf("variable statement at %d has no declaration list", node.Pos())
	}
	return e.extractVariableDeclarationList(list)
}

func (e *extractor) extractVariableDeclarationList(node tsast.Node) ([]Statement, error) {
	declarations, ok := node.NamedChild("declarations")
	if !ok || !declarations.IsList() {
		return nil, fmt.Errorf("variable declaration list at %d has no declarations", node.Pos())
	}
	result := make([]Statement, 0, len(declarations.ListElements()))
	for _, declaration := range declarations.ListElements() {
		stmt, err := e.extractVariableDeclaration(declaration)
		if err != nil {
			return nil, err
		}
		result = append(result, stmt)
	}
	return result, nil
}

func (e *extractor) extractVariableDeclaration(node tsast.Node) (Statement, error) {
	nameNode, ok := node.NamedChild("name")
	if !ok || nameNode.Kind() != tsast.KindIdentifier {
		return Statement{}, fmt.Errorf("native variable at %d requires an identifier name", node.Pos())
	}
	name, _ := nameNode.Text()
	symbol, err := e.client.GetSymbolAtLocation(e.ctx, e.snapshot, e.project, nameNode.Handle(e.fileName))
	if err != nil || symbol == nil {
		if err == nil {
			err = fmt.Errorf("variable %s has no TypeScript symbol", name)
		}
		return Statement{}, err
	}
	typeID, err := e.typeAt(nameNode)
	if err != nil {
		return Statement{}, err
	}
	symbolID := e.internSymbol(symbol, SymbolVariable, nameNode)
	e.result.Symbols[symbolID].Type = typeID
	initializer, ok := node.NamedChild("initializer")
	if !ok {
		return Statement{}, fmt.Errorf("native variable %s requires an initializer", name)
	}
	value, err := e.extractExpr(initializer)
	if err != nil {
		return Statement{}, err
	}
	return Statement{Kind: StmtVar, Span: e.span(node), Symbol: symbolID, Name: name, Type: typeID, Value: value}, nil
}

func (e *extractor) extractWhile(node tsast.Node) (Statement, error) {
	conditionNode, ok := node.NamedChild("expression")
	if !ok {
		return Statement{}, fmt.Errorf("while at %d has no condition", node.Pos())
	}
	condition, err := e.extractExpr(conditionNode)
	if err != nil {
		return Statement{}, err
	}
	bodyNode, ok := node.NamedChild("statement")
	if !ok {
		return Statement{}, fmt.Errorf("while at %d has no body", node.Pos())
	}
	body, err := e.extractStatementBody(bodyNode)
	if err != nil {
		return Statement{}, err
	}
	return Statement{Kind: StmtWhile, Span: e.span(node), Expr: condition, Then: body}, nil
}

func (e *extractor) extractFor(node tsast.Node) (Statement, error) {
	stmt := Statement{Kind: StmtFor, Span: e.span(node)}
	if initializer, ok := node.NamedChild("initializer"); ok {
		var err error
		switch initializer.Kind() {
		case tsast.KindVariableDeclarationList:
			stmt.Init, err = e.extractVariableDeclarationList(initializer)
		default:
			var mutation Statement
			var matched bool
			mutation, matched, err = e.extractMutation(initializer)
			if err == nil && matched {
				stmt.Init = []Statement{mutation}
			}
			if err == nil && !matched {
				err = fmt.Errorf("unsupported for initializer %s at %d", tsast.KindName(initializer.Kind()), initializer.Pos())
			}
		}
		if err != nil {
			return Statement{}, err
		}
	}
	conditionNode, ok := node.NamedChild("condition")
	if !ok {
		return Statement{}, fmt.Errorf("native for at %d requires a condition", node.Pos())
	}
	condition, err := e.extractExpr(conditionNode)
	if err != nil {
		return Statement{}, err
	}
	stmt.Expr = condition
	if incrementor, ok := node.NamedChild("incrementor"); ok {
		mutation, matched, err := e.extractMutation(incrementor)
		if err != nil {
			return Statement{}, err
		}
		if !matched {
			return Statement{}, fmt.Errorf("unsupported for incrementor %s at %d", tsast.KindName(incrementor.Kind()), incrementor.Pos())
		}
		stmt.Update = []Statement{mutation}
	}
	bodyNode, ok := node.NamedChild("statement")
	if !ok {
		return Statement{}, fmt.Errorf("for at %d has no body", node.Pos())
	}
	stmt.Then, err = e.extractStatementBody(bodyNode)
	return stmt, err
}

func (e *extractor) extractMutation(node tsast.Node) (Statement, bool, error) {
	if node.Kind() == tsast.KindBinaryExpression {
		opNode, ok := node.NamedChild("operatorToken")
		if !ok || opNode.Kind() != tsast.KindEqualsToken {
			return Statement{}, false, nil
		}
		left, ok := node.NamedChild("left")
		if !ok || left.Kind() != tsast.KindIdentifier {
			return Statement{}, true, fmt.Errorf("native assignment at %d requires identifier target", node.Pos())
		}
		right, ok := node.NamedChild("right")
		if !ok {
			return Statement{}, true, fmt.Errorf("assignment at %d has no value", node.Pos())
		}
		return e.buildAssignment(node, left, right, 0)
	}
	if node.Kind() == tsast.KindPrefixUnaryExpression || node.Kind() == tsast.KindPostfixUnaryExpression {
		op, ok := node.UnaryOperatorKind()
		if !ok || (op != tsast.KindPlusPlusToken && op != tsast.KindMinusMinusToken) {
			return Statement{}, false, nil
		}
		operand, ok := node.NamedChild("operand")
		if !ok || operand.Kind() != tsast.KindIdentifier {
			return Statement{}, true, fmt.Errorf("increment at %d requires identifier operand", node.Pos())
		}
		return e.buildAssignment(node, operand, tsast.Node{}, op)
	}
	return Statement{}, false, nil
}

func (e *extractor) buildAssignment(node, target, rhs tsast.Node, unary uint32) (Statement, bool, error) {
	name, _ := target.Text()
	symbol, err := e.client.GetSymbolAtLocation(e.ctx, e.snapshot, e.project, target.Handle(e.fileName))
	if err != nil || symbol == nil {
		if err == nil {
			err = fmt.Errorf("assignment target %s has no TypeScript symbol", name)
		}
		return Statement{}, true, err
	}
	typeID, err := e.typeAt(target)
	if err != nil {
		return Statement{}, true, err
	}
	symbolID := e.internSymbol(symbol, SymbolVariable, target)
	e.result.Symbols[symbolID].Type = typeID
	var value *Expr
	if unary == 0 {
		value, err = e.extractExpr(rhs)
	} else {
		left := &Expr{Kind: ExprIdentifier, Type: typeID, Symbol: symbolID, Name: name, Span: e.span(target)}
		right := &Expr{Kind: ExprNumber, Type: typeID, Number: 1, Span: e.span(node)}
		op := BinaryAdd
		if unary == tsast.KindMinusMinusToken {
			op = BinarySub
		}
		value = &Expr{Kind: ExprBinary, Type: typeID, Operator: op, Left: left, Right: right, Span: e.span(node)}
	}
	if err != nil {
		return Statement{}, true, err
	}
	return Statement{Kind: StmtAssign, Span: e.span(node), Symbol: symbolID, Name: name, Type: typeID, Value: value}, true, nil
}

func (e *extractor) extractStatementBody(node tsast.Node) ([]Statement, error) {
	if node.Kind() == tsast.KindBlock {
		return e.extractBlock(node)
	}
	stmt, err := e.extractStatement(node)
	if err != nil {
		return nil, err
	}
	return []Statement{stmt}, nil
}

func (e *extractor) extractExpr(node tsast.Node) (*Expr, error) {
	typeID, err := e.typeAt(node)
	if err != nil {
		return nil, err
	}
	expr := &Expr{Type: typeID, Span: e.span(node)}
	switch node.Kind() {
	case tsast.KindIdentifier:
		expr.Kind = ExprIdentifier
		expr.Name, _ = node.Text()
		symbol, err := e.client.GetSymbolAtLocation(e.ctx, e.snapshot, e.project, node.Handle(e.fileName))
		if err != nil {
			return nil, err
		}
		if symbol != nil {
			kind := SymbolVariable
			if _, ok := e.functions[symbol.ID]; ok {
				kind = SymbolFunction
			}
			expr.Symbol = e.internSymbol(symbol, kind, node)
			e.result.Symbols[expr.Symbol].Type = typeID
		}
		return expr, nil
	case tsast.KindNumericLiteral:
		expr.Kind = ExprNumber
		text, ok := node.Text()
		if !ok {
			return nil, fmt.Errorf("numeric literal at %d has no text", node.Pos())
		}
		expr.Number, err = strconv.ParseFloat(text, 64)
		if err != nil {
			return nil, fmt.Errorf("parse numeric literal %q: %w", text, err)
		}
		return expr, nil
	case tsast.KindStringLiteral:
		expr.Kind = ExprString
		text, ok := node.Text()
		if !ok {
			return nil, fmt.Errorf("string literal at %d has no text", node.Pos())
		}
		expr.String = text
		return expr, nil
	case tsast.KindBinaryExpression:
		return e.extractBinary(node, expr)
	case tsast.KindCallExpression:
		return e.extractCall(node, expr)
	case tsast.KindArrayLiteralExpression:
		expr.Kind = ExprArray
		elements, ok := node.NamedChild("elements")
		if !ok || !elements.IsList() {
			return nil, fmt.Errorf("array literal at %d has no element list", node.Pos())
		}
		for _, elementNode := range elements.ListElements() {
			element, err := e.extractExpr(elementNode)
			if err != nil {
				return nil, err
			}
			if int(element.Type) >= len(e.result.Types) || e.result.Types[element.Type].Kind != TypeNumber {
				return nil, fmt.Errorf("native array at %d currently supports number elements only", node.Pos())
			}
			expr.Elements = append(expr.Elements, element)
		}
		return expr, nil
	case tsast.KindObjectLiteralExpression:
		if int(typeID) >= len(e.result.Types) || e.result.Types[typeID].Kind != TypeObject {
			return nil, fmt.Errorf("object literal at %d has non-object type", node.Pos())
		}
		shapeID := e.result.Types[typeID].Shape
		if int(shapeID) >= len(e.result.Shapes) {
			return nil, fmt.Errorf("object literal at %d has invalid shape s%d", node.Pos(), shapeID)
		}
		propertiesNode, ok := node.NamedChild("properties")
		if !ok || !propertiesNode.IsList() {
			return nil, fmt.Errorf("object literal at %d has no properties", node.Pos())
		}
		values := map[string]*Expr{}
		for _, property := range propertiesNode.ListElements() {
			nameNode, ok := property.NamedChild("name")
			if !ok || nameNode.Kind() != tsast.KindIdentifier {
				return nil, fmt.Errorf("native object property at %d requires identifier name", property.Pos())
			}
			name, _ := nameNode.Text()
			var value *Expr
			var err error
			switch property.Kind() {
			case tsast.KindPropertyAssignment:
				initializer, ok := property.NamedChild("initializer")
				if !ok {
					return nil, fmt.Errorf("object property %s has no initializer", name)
				}
				value, err = e.extractExpr(initializer)
			case tsast.KindShorthandPropertyAssignment:
				value, err = e.extractExpr(nameNode)
			default:
				return nil, fmt.Errorf("unsupported object property %s at %d", tsast.KindName(property.Kind()), property.Pos())
			}
			if err != nil {
				return nil, err
			}
			values[name] = value
		}
		expr.Kind = ExprObject
		for i, field := range e.result.Shapes[shapeID].Fields {
			value, ok := values[field.Name]
			if !ok {
				return nil, fmt.Errorf("object literal at %d is missing shape field %s", node.Pos(), field.Name)
			}
			expr.Fields = append(expr.Fields, ObjectFieldExpr{Name: field.Name, Index: uint32(i), Value: value})
		}
		return expr, nil
	case tsast.KindElementAccessExpression:
		objectNode, ok := node.NamedChild("expression")
		if !ok {
			return nil, fmt.Errorf("element access at %d has no object", node.Pos())
		}
		indexNode, ok := node.NamedChild("argumentExpression")
		if !ok {
			return nil, fmt.Errorf("element access at %d has no index", node.Pos())
		}
		object, err := e.extractExpr(objectNode)
		if err != nil {
			return nil, err
		}
		index, err := e.extractExpr(indexNode)
		if err != nil {
			return nil, err
		}
		expr.Kind, expr.Object, expr.Index = ExprIndex, object, index
		return expr, nil
	case tsast.KindPropertyAccessExpression:
		objectNode, ok := node.NamedChild("expression")
		if !ok {
			return nil, fmt.Errorf("property access at %d has no object", node.Pos())
		}
		nameNode, ok := node.NamedChild("name")
		if !ok {
			return nil, fmt.Errorf("property access at %d has no name", node.Pos())
		}
		name, _ := nameNode.Text()
		object, err := e.extractExpr(objectNode)
		if err != nil {
			return nil, err
		}
		if int(object.Type) >= len(e.result.Types) {
			return nil, fmt.Errorf("property access at %d has invalid object type", node.Pos())
		}
		objectType := e.result.Types[object.Type]
		if objectType.Kind == TypeArray && name == "length" {
			expr.Kind, expr.Object = ExprArrayLength, object
			return expr, nil
		}
		if objectType.Kind != TypeObject || int(objectType.Shape) >= len(e.result.Shapes) {
			return nil, fmt.Errorf("native property %q at %d requires a closed object shape", name, node.Pos())
		}
		shape := e.result.Shapes[objectType.Shape]
		for i, field := range shape.Fields {
			if field.Name == name {
				expr.Kind, expr.Object, expr.Field, expr.FieldIndex = ExprFieldGet, object, name, uint32(i)
				return expr, nil
			}
		}
		return nil, fmt.Errorf("shape %s has no field %q", shape.Name, name)
	case tsast.KindNonNullExpression:
		innerNode, ok := node.NamedChild("expression")
		if !ok {
			return nil, fmt.Errorf("non-null expression at %d has no operand", node.Pos())
		}
		inner, err := e.extractExpr(innerNode)
		if err != nil {
			return nil, err
		}
		inner.Type, inner.Span = typeID, e.span(node)
		return inner, nil
	default:
		return nil, fmt.Errorf("unsupported native expression %s at %d", tsast.KindName(node.Kind()), node.Pos())
	}
}

func (e *extractor) extractBinary(node tsast.Node, expr *Expr) (*Expr, error) {
	leftNode, _ := node.NamedChild("left")
	rightNode, _ := node.NamedChild("right")
	opNode, _ := node.NamedChild("operatorToken")
	left, err := e.extractExpr(leftNode)
	if err != nil {
		return nil, err
	}
	right, err := e.extractExpr(rightNode)
	if err != nil {
		return nil, err
	}
	op := BinaryInvalid
	switch opNode.Kind() {
	case tsast.KindPlusToken:
		op = BinaryAdd
	case tsast.KindMinusToken:
		op = BinarySub
	case tsast.KindAsteriskToken:
		op = BinaryMul
	case tsast.KindSlashToken:
		op = BinaryDiv
	case tsast.KindLessThanToken:
		op = BinaryLessThan
	case tsast.KindLessThanEqualsToken:
		op = BinaryLessEqual
	case tsast.KindGreaterThanToken:
		op = BinaryGreaterThan
	case tsast.KindGreaterThanEqualsToken:
		op = BinaryGreaterEqual
	case tsast.KindEqualsEqualsToken, tsast.KindEqualsEqualsEqualsToken:
		op = BinaryEqual
	case tsast.KindExclamationEqualsToken, tsast.KindExclamationEqualsEqualsToken:
		op = BinaryNotEqual
	default:
		return nil, fmt.Errorf("unsupported binary operator %s at %d", tsast.KindName(opNode.Kind()), opNode.Pos())
	}
	expr.Kind = ExprBinary
	expr.Operator = op
	expr.Left = left
	expr.Right = right
	return expr, nil
}

func (e *extractor) extractCall(node tsast.Node, expr *Expr) (*Expr, error) {
	calleeNode, ok := node.NamedChild("expression")
	if !ok {
		return nil, fmt.Errorf("call at %d has no callee", node.Pos())
	}
	expr.Kind = ExprCall
	consoleCall := false
	switch calleeNode.Kind() {
	case tsast.KindIdentifier:
		calleeName, _ := calleeNode.Text()
		expr.Callee = &Expr{Kind: ExprIdentifier, Name: calleeName, Span: e.span(calleeNode)}
		symbol, err := e.client.GetSymbolAtLocation(e.ctx, e.snapshot, e.project, calleeNode.Handle(e.fileName))
		if err != nil {
			return nil, err
		}
		if symbol != nil {
			if target, ok := e.functions[symbol.ID]; ok {
				targetCopy := target
				expr.CallTarget = &targetCopy
			}
		}
	case tsast.KindPropertyAccessExpression:
		if !isConsoleLog(calleeNode) {
			return nil, fmt.Errorf("property call at %d is not a supported native intrinsic", calleeNode.Pos())
		}
		expr.Callee = &Expr{Kind: ExprIdentifier, Name: "console.log", Span: e.span(calleeNode)}
		consoleCall = true
	default:
		return nil, fmt.Errorf("callee %s at %d is not supported by the native MVP", tsast.KindName(calleeNode.Kind()), calleeNode.Pos())
	}
	if args, ok := node.NamedChild("arguments"); ok {
		for _, argNode := range args.ListElements() {
			arg, err := e.extractExpr(argNode)
			if err != nil {
				return nil, err
			}
			expr.Args = append(expr.Args, arg)
		}
	}
	if consoleCall {
		if len(expr.Args) != 1 || int(expr.Args[0].Type) >= len(e.result.Types) {
			return nil, fmt.Errorf("console.log native MVP requires exactly one supported argument at %d", node.Pos())
		}
		switch e.result.Types[expr.Args[0].Type].Kind {
		case TypeNumber:
			expr.Intrinsic = IntrinsicConsoleLogF64
		case TypeString:
			expr.Intrinsic = IntrinsicConsoleLogString
		default:
			return nil, fmt.Errorf("console.log native MVP does not support argument type %q at %d", e.result.Types[expr.Args[0].Type].Name, node.Pos())
		}
		return expr, nil
	}
	if expr.CallTarget == nil {
		return nil, fmt.Errorf("dynamic call at %d is not supported by the native MVP", node.Pos())
	}
	return expr, nil
}

func isConsoleLog(node tsast.Node) bool {
	object, ok := node.NamedChild("expression")
	if !ok || object.Kind() != tsast.KindIdentifier {
		return false
	}
	name, ok := node.NamedChild("name")
	if !ok || name.Kind() != tsast.KindIdentifier {
		return false
	}
	objectText, _ := object.Text()
	nameText, _ := name.Text()
	return objectText == "console" && nameText == "log"
}

func (e *extractor) typeAt(node tsast.Node) (TypeID, error) {
	info, err := e.client.GetTypeAtLocation(e.ctx, e.snapshot, e.project, node.Handle(e.fileName))
	if err != nil {
		return 0, err
	}
	if info == nil {
		return 0, fmt.Errorf("node %s at %d has no TypeScript type", tsast.KindName(node.Kind()), node.Pos())
	}
	return e.internAPIType(info)
}

func (e *extractor) internAPIType(info *tsls.APIType) (TypeID, error) {
	if info == nil {
		return 0, fmt.Errorf("nil TypeScript type")
	}
	if id, ok := e.types[info.ID]; ok {
		return id, nil
	}
	text, err := e.client.TypeToString(e.ctx, e.snapshot, e.project, info.ID)
	if err != nil {
		return 0, err
	}
	kind := classifyType(text)
	if kind == TypeNumber && text == "number" {
		id := e.ensureSemanticType(TypeNumber, "number")
		e.types[info.ID] = id
		return id, nil
	}
	if kind == TypeString && text == "string" {
		id := e.ensureSemanticType(TypeString, "string")
		e.types[info.ID] = id
		return id, nil
	}
	typ := Type{Kind: kind, Name: text}
	if kind == TypeArray {
		base := strings.TrimSpace(strings.TrimPrefix(text, "readonly "))
		if base != "number[]" {
			return 0, fmt.Errorf("native array type %q is not supported", text)
		}
		typ.Element = e.ensureSemanticType(TypeNumber, "number")
	}
	id := TypeID(len(e.result.Types))
	typ.ID = id
	e.result.Types = append(e.result.Types, typ)
	e.types[info.ID] = id
	if kind == TypeObject {
		shape, err := e.internObjectShape(info, text)
		if err != nil {
			return 0, err
		}
		e.result.Types[id].Shape = shape
	}
	return id, nil
}

func (e *extractor) internObjectShape(info *tsls.APIType, name string) (ShapeID, error) {
	if id, ok := e.shapes[info.ID]; ok {
		return id, nil
	}
	id := ShapeID(len(e.result.Shapes))
	e.shapes[info.ID] = id
	e.result.Shapes = append(e.result.Shapes, Shape{ID: id, Name: name})
	properties, err := e.client.GetPropertiesOfType(e.ctx, e.snapshot, e.project, info.ID)
	if err != nil {
		return 0, err
	}
	sort.Slice(properties, func(i, j int) bool { return properties[i].Name < properties[j].Name })
	fields := make([]ShapeField, 0, len(properties))
	for _, property := range properties {
		propertyType, err := e.client.GetTypeOfSymbol(e.ctx, e.snapshot, e.project, property.ID)
		if err != nil {
			return 0, err
		}
		if propertyType == nil {
			return 0, fmt.Errorf("property %s on %s has no TypeScript type", property.Name, name)
		}
		fieldType, err := e.internAPIType(propertyType)
		if err != nil {
			return 0, err
		}
		fields = append(fields, ShapeField{Name: property.Name, Type: fieldType})
	}
	e.result.Shapes[id].Fields = fields
	return id, nil
}

func (e *extractor) ensureSemanticType(kind TypeKind, name string) TypeID {
	for _, typ := range e.result.Types {
		if typ.Kind == kind && typ.Name == name {
			return typ.ID
		}
	}
	id := TypeID(len(e.result.Types))
	e.result.Types = append(e.result.Types, Type{ID: id, Kind: kind, Name: name})
	return id
}

func classifyType(text string) TypeKind {
	switch text {
	case "any":
		return TypeAny
	case "unknown":
		return TypeUnknown
	case "never":
		return TypeNever
	case "void":
		return TypeVoid
	case "undefined":
		return TypeUndefined
	case "null":
		return TypeNull
	case "boolean", "true", "false":
		return TypeBoolean
	case "number":
		return TypeNumber
	case "string":
		return TypeString
	}
	if _, err := strconv.ParseFloat(text, 64); err == nil {
		return TypeNumber
	}
	if len(text) >= 2 && ((strings.HasPrefix(text, "\"") && strings.HasSuffix(text, "\"")) || (strings.HasPrefix(text, "'") && strings.HasSuffix(text, "'"))) {
		return TypeString
	}
	arrayText := strings.TrimSpace(strings.TrimPrefix(text, "readonly "))
	if strings.HasSuffix(arrayText, "[]") {
		return TypeArray
	}
	if strings.Contains(text, "=>") {
		return TypeFunction
	}
	if strings.Contains(text, " | ") {
		return TypeUnion
	}
	return TypeObject
}

func (e *extractor) internSymbol(symbol *tsls.APISymbol, kind SymbolKind, node tsast.Node) SymbolID {
	if id, ok := e.symbols[symbol.ID]; ok {
		return id
	}
	id := SymbolID(len(e.result.Symbols))
	e.symbols[symbol.ID] = id
	e.result.Symbols = append(e.result.Symbols, Symbol{
		ID: id, Name: symbol.Name, Kind: kind, Decl: e.span(node),
	})
	return id
}

func (e *extractor) span(node tsast.Node) Span {
	return Span{Source: 0, Start: e.position(node.Pos()), End: e.position(node.End())}
}

func (e *extractor) position(offset int32) Position {
	if offset < 0 {
		offset = 0
	}
	if int(offset) > len(e.sourceText) {
		offset = int32(len(e.sourceText))
	}
	prefix := e.sourceText[:offset]
	line := uint32(strings.Count(prefix, "\n"))
	lastNewline := strings.LastIndex(prefix, "\n")
	columnStart := 0
	if lastNewline >= 0 {
		columnStart = lastNewline + 1
	}
	return Position{Line: line, Character: uint32(len(prefix[columnStart:]))}
}

func hasModifier(node tsast.Node, kind uint32) bool {
	mods, ok := node.NamedChild("modifiers")
	if !ok || !mods.IsList() {
		return false
	}
	for _, modifier := range mods.ListElements() {
		if modifier.Kind() == kind {
			return true
		}
	}
	return false
}
