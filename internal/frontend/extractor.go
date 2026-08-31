package frontend

import (
	"context"
	"fmt"
	"path/filepath"
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
	default:
		return Statement{}, fmt.Errorf("unsupported native statement %s at %d", tsast.KindName(node.Kind()), node.Pos())
	}
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
	case tsast.KindBinaryExpression:
		return e.extractBinary(node, expr)
	case tsast.KindCallExpression:
		return e.extractCall(node, expr)
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
	case tsast.KindLessThanEqualsToken:
		op = BinaryLessEqual
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
	if calleeNode.Kind() != tsast.KindIdentifier {
		return nil, fmt.Errorf("non-identifier callee at %d is not supported by the native MVP", calleeNode.Pos())
	}
	calleeName, _ := calleeNode.Text()
	expr.Kind = ExprCall
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
	if args, ok := node.NamedChild("arguments"); ok {
		for _, argNode := range args.ListElements() {
			arg, err := e.extractExpr(argNode)
			if err != nil {
				return nil, err
			}
			expr.Args = append(expr.Args, arg)
		}
	}
	if expr.CallTarget == nil {
		return nil, fmt.Errorf("dynamic call at %d is not supported by the native MVP", node.Pos())
	}
	return expr, nil
}

func (e *extractor) typeAt(node tsast.Node) (TypeID, error) {
	info, err := e.client.GetTypeAtLocation(e.ctx, e.snapshot, e.project, node.Handle(e.fileName))
	if err != nil {
		return 0, err
	}
	if info == nil {
		return 0, fmt.Errorf("node %s at %d has no TypeScript type", tsast.KindName(node.Kind()), node.Pos())
	}
	if id, ok := e.types[info.ID]; ok {
		return id, nil
	}
	text, err := e.client.TypeToString(e.ctx, e.snapshot, e.project, info.ID)
	if err != nil {
		return 0, err
	}
	id := TypeID(len(e.result.Types))
	e.result.Types = append(e.result.Types, Type{ID: id, Kind: classifyType(text), Name: text})
	e.types[info.ID] = id
	return id, nil
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
