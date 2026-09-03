package frontend

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/phongsathornpt/ts-pro/internal/tsast"
	"github.com/phongsathornpt/ts-pro/internal/tsls"
)

type pendingFunctionBody struct {
	Function    FunctionID
	Node        tsast.Node
	This        *SymbolID
	Constructor *classInfo
	FileName    string
}

type classFieldInitializer struct {
	Field int
	Node  tsast.Node
}

type classInfo struct {
	Node              tsast.Node
	Name              string
	Type              TypeID
	Shape             ShapeID
	ConstructorParams []Parameter
	FieldParam        []int
	Initializers      []classFieldInitializer
	ConstructorNode   tsast.Node
	HasConstructor    bool
	Constructor       FunctionID
	ConstructorThis   SymbolID
	Base              *classInfo
	Methods           map[string]FunctionID
}

type extractor struct {
	ctx                 context.Context
	client              *tsls.APIClient
	snapshot            uint64
	project             string
	fileName            string
	currentFileName     string
	sourceText          string
	sources             map[string]SourceID
	result              Snapshot
	types               map[uint64]TypeID
	symbols             map[uint64]SymbolID
	functions           map[uint64]FunctionID
	shapes              map[uint64]ShapeID
	classes             map[uint64]*classInfo
	classesByType       map[TypeID]*classInfo
	concreteClasses     map[SymbolID]*classInfo
	closures            map[uint64]closureInfo
	generics            map[uint64]genericInfo
	specializations     map[string]FunctionID
	enums               map[string]*enumInfo
	importedFunctions   map[string]FunctionID
	importedClasses     map[string]*classInfo
	typeSubstitutions   map[uint64]TypeID
	symbolSubstitutions map[uint64]SymbolID
	pending             []pendingFunctionBody
	currentThis         *SymbolID
	currentFunction     *FunctionID
	parameterAliases    map[string]SymbolID
	arrayConstants      map[SymbolID]*Expr
}

type enumInfo struct {
	Name    string
	Members map[string]float64
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
		ctx:             ctx,
		client:          client,
		snapshot:        snapshot,
		project:         project,
		fileName:        abs,
		sourceText:      text,
		types:           map[uint64]TypeID{},
		symbols:         map[uint64]SymbolID{},
		functions:       map[uint64]FunctionID{},
		shapes:          map[uint64]ShapeID{},
		classes:         map[uint64]*classInfo{},
		classesByType:   map[TypeID]*classInfo{},
		concreteClasses: map[SymbolID]*classInfo{},
		closures:        map[uint64]closureInfo{},
		generics:        map[uint64]genericInfo{},
		specializations: map[string]FunctionID{},
		enums:               map[string]*enumInfo{},
		importedFunctions:   map[string]FunctionID{},
		importedClasses:     map[string]*classInfo{},
		arrayConstants:      map[SymbolID]*Expr{},
		sources:             map[string]SourceID{abs: 0},
	}
	e.result.Sources = append(e.result.Sources, Source{ID: 0, URI: "file://" + filepath.ToSlash(abs), Path: abs})
	e.ensureSemanticType(TypeVoid, "void")
	e.ensureSemanticType(TypeBoolean, "boolean")

	visitedModules := map[string]bool{abs: true}
	for _, node := range file.Root().Children() {
		if node.Kind() == tsast.KindImportDeclaration {
			if err := e.processImport(node, visitedModules); err != nil {
				return Snapshot{}, err
			}
		}
	}

	for _, node := range file.Root().Children() {
		switch node.Kind() {
		case tsast.KindFunctionDeclaration:
			if hasTypeParameters(node) {
				if err := e.registerGenericFunction(node); err != nil {
					return Snapshot{}, err
				}
				continue
			}
			functionID, err := e.extractFunctionSignature(node)
			if err != nil {
				return Snapshot{}, err
			}
			e.pending = append(e.pending, pendingFunctionBody{Function: functionID, Node: node, FileName: abs})
		case tsast.KindClassDeclaration:
			if err := e.extractClassSignatures(node); err != nil {
				return Snapshot{}, err
			}
		case tsast.KindEnumDeclaration:
			if err := e.extractEnum(node); err != nil {
				return Snapshot{}, err
			}
		}
	}
	for _, pending := range e.pending {
		e.currentFileName = pending.FileName
		e.currentThis = pending.This
		current := pending.Function
		e.currentFunction = &current
		e.concreteClasses = map[SymbolID]*classInfo{}
		var body []Statement
		var err error
		if pending.Constructor != nil {
			body, err = e.extractNativeConstructorBody(pending.Constructor)
		} else {
			body, err = e.extractFunctionBody(pending.Node)
		}
		if err != nil {
			return Snapshot{}, err
		}
		e.result.Functions[pending.Function].Body = body
	}
	e.currentFileName = abs
	e.currentThis = nil
	e.currentFunction = nil
	for _, node := range file.Root().Children() {
		switch node.Kind() {
		case tsast.KindFunctionDeclaration, tsast.KindClassDeclaration, tsast.KindInterfaceDeclaration, tsast.KindTypeAliasDeclaration, tsast.KindEnumDeclaration, tsast.KindEndOfFile, tsast.KindImportDeclaration, tsast.KindExportDeclaration, tsast.KindExportAssignment:
			continue
		case tsast.KindVariableStatement:
			items, err := e.extractVariableStatement(node)
			if err != nil {
				return Snapshot{}, err
			}
			e.result.Entry = append(e.result.Entry, items...)
		case tsast.KindExpressionStatement, tsast.KindIfStatement, tsast.KindDoStatement, tsast.KindWhileStatement, tsast.KindForStatement, tsast.KindForOfStatement, tsast.KindBlock, tsast.KindSwitchStatement:
			stmt, err := e.extractStatement(node)
			if err != nil {
				return Snapshot{}, err
			}
			e.result.Entry = append(e.result.Entry, stmt)
		default:
			return Snapshot{}, fmt.Errorf("unsupported top-level native statement %s (kind=%d) at %d", tsast.KindName(node.Kind()), node.Kind(), node.Pos())
		}
	}
	return e.result, nil
}

func (e *extractor) currentFile() string {
	if e.currentFileName != "" {
		return e.currentFileName
	}
	return e.fileName
}

func (e *extractor) processImport(node tsast.Node, visited map[string]bool) error {
	specifier := ""
	var clauseNode tsast.Node
	for _, child := range node.Children() {
		if child.Kind() == tsast.KindStringLiteral {
			if txt, ok := child.Text(); ok {
				specifier = strings.Trim(txt, `"'`+"`")
			}
		} else if child.Kind() == tsast.KindImportClause {
			clauseNode = child
		}
	}
	if specifier == "" {
		if specifierNode, ok := node.NamedChild("moduleSpecifier"); ok {
			if txt, ok := specifierNode.Text(); ok {
				specifier = strings.Trim(txt, `"'`+"`")
			}
		}
	}
	if specifier == "" || !strings.HasPrefix(specifier, ".") {
		return nil
	}
	targetDir := filepath.Dir(e.currentFile())
	resolved := filepath.Join(targetDir, specifier)
	if !strings.HasSuffix(resolved, ".ts") {
		if _, err := os.Stat(resolved + ".ts"); err == nil {
			resolved += ".ts"
		} else if _, err := os.Stat(filepath.Join(resolved, "index.ts")); err == nil {
			resolved = filepath.Join(resolved, "index.ts")
		}
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return err
	}
	if visited[abs] {
		return nil
	}
	visited[abs] = true

	sourceID := SourceID(len(e.result.Sources))
	e.sources[abs] = sourceID
	e.result.Sources = append(e.result.Sources, Source{ID: sourceID, URI: "file://" + filepath.ToSlash(abs), Path: abs})

	payload, err := e.client.GetSourceFile(e.ctx, e.snapshot, e.project, abs)
	if err != nil {
		return err
	}
	file, err := tsast.Decode(payload)
	if err != nil {
		return err
	}

	prevFile := e.currentFileName
	e.currentFileName = abs

	for _, child := range file.Root().Children() {
		if child.Kind() == tsast.KindImportDeclaration {
			if err := e.processImport(child, visited); err != nil {
				return err
			}
		}
	}

	fileFnIDs := make(map[string]FunctionID)
	fileClasses := make(map[string]*classInfo)

	for _, child := range file.Root().Children() {
		switch child.Kind() {
		case tsast.KindFunctionDeclaration:
			if hasTypeParameters(child) {
				if err := e.registerGenericFunction(child); err != nil {
					return err
				}
				continue
			}
			fnID, err := e.extractFunctionSignature(child)
			if err != nil {
				return err
			}
			if nameNode, ok := child.NamedChild("name"); ok {
				if name, ok := nameNode.Text(); ok {
					fileFnIDs[name] = fnID
				}
			} else {
				for _, n := range child.Children() {
					if n.Kind() == tsast.KindIdentifier {
						if name, ok := n.Text(); ok {
							fileFnIDs[name] = fnID
							break
						}
					}
				}
			}
			e.pending = append(e.pending, pendingFunctionBody{Function: fnID, Node: child, FileName: abs})
		case tsast.KindClassDeclaration:
			if err := e.extractClassSignatures(child); err != nil {
				return err
			}
			for _, c := range e.classes {
				fileClasses[c.Name] = c
			}
		case tsast.KindEnumDeclaration:
			if err := e.extractEnum(child); err != nil {
				return err
			}
		}
	}

	if clauseNode.Kind() == tsast.KindImportClause {
		for _, child := range clauseNode.Children() {
			if child.Kind() == tsast.KindIdentifier {
				localName, _ := child.Text()
				if localName != "" {
					if fnID, ok := fileFnIDs[localName]; ok {
						e.importedFunctions[localName] = fnID
					}
					if cls, ok := fileClasses[localName]; ok {
						e.importedClasses[localName] = cls
					}
				}
			} else if child.Kind() == tsast.KindNamedImports {
				for _, specChild := range child.Children() {
					if specChild.Kind() == tsast.KindImportSpecifier {
						var idents []string
						for _, idNode := range specChild.Children() {
							if idNode.Kind() == tsast.KindIdentifier {
								if txt, ok := idNode.Text(); ok {
									idents = append(idents, txt)
								}
							}
						}
						localName := ""
						remoteName := ""
						if len(idents) == 1 {
							localName = idents[0]
							remoteName = idents[0]
						} else if len(idents) >= 2 {
							remoteName = idents[0]
							localName = idents[1]
						}
						if localName != "" {
							if fnID, ok := fileFnIDs[remoteName]; ok {
								e.importedFunctions[localName] = fnID
							}
							if cls, ok := fileClasses[remoteName]; ok {
								e.importedClasses[localName] = cls
							}
						}
					}
				}
			}
		}
	}

	for _, child := range file.Root().Children() {
		switch child.Kind() {
		case tsast.KindVariableStatement:
			items, err := e.extractVariableStatement(child)
			if err != nil {
				return err
			}
			e.result.Entry = append(e.result.Entry, items...)
		case tsast.KindExpressionStatement, tsast.KindIfStatement, tsast.KindDoStatement, tsast.KindWhileStatement, tsast.KindForStatement, tsast.KindForOfStatement, tsast.KindBlock, tsast.KindSwitchStatement:
			stmt, err := e.extractStatement(child)
			if err != nil {
				return err
			}
			e.result.Entry = append(e.result.Entry, stmt)
		}
	}

	e.currentFileName = prevFile
	return nil
}

func (e *extractor) extractEnum(node tsast.Node) error {
	var nameNode tsast.Node
	for _, child := range node.Children() {
		if child.Kind() == tsast.KindIdentifier {
			nameNode = child
			break
		}
	}
	if nameNode.Kind() == 0 {
		return fmt.Errorf("enum at %d has no name", node.Pos())
	}
	enumName, _ := nameNode.Text()
	info := &enumInfo{Name: enumName, Members: map[string]float64{}}
	currentVal := float64(0)
	for _, child := range node.Children() {
		if child.Kind() != tsast.KindEnumMember {
			continue
		}
		subs := child.Children()
		if len(subs) == 0 {
			continue
		}
		memberName, _ := subs[0].Text()
		if len(subs) > 1 {
			txt, ok := subs[1].Text()
			if ok {
				if v, err := strconv.ParseFloat(txt, 64); err == nil {
					currentVal = v
				}
			}
		}
		info.Members[memberName] = currentVal
		currentVal++
	}
	e.enums[enumName] = info
	return nil
}

func (e *extractor) extractFunctionSignature(node tsast.Node) (FunctionID, error) {
	nameNode, ok := node.NamedChild("name")
	if !ok {
		return 0, fmt.Errorf("function declaration at %d has no name", node.Pos())
	}
	name, _ := nameNode.Text()
	symbol, err := e.client.GetSymbolAtLocation(e.ctx, e.snapshot, e.project, nameNode.Handle(e.currentFile()))
	if err != nil {
		return 0, err
	}
	if symbol == nil {
		return 0, fmt.Errorf("function %s has no TypeScript symbol", name)
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
		Async:    hasModifier(node, tsast.KindAsyncKeyword),
	}
	if params, ok := node.NamedChild("parameters"); ok {
		for _, paramNode := range params.ListElements() {
			param, err := e.extractParameter(paramNode)
			if err != nil {
				return 0, err
			}
			fn.Params = append(fn.Params, param)
		}
	}
	returnTypeNode, ok := node.NamedChild("type")
	if !ok {
		return 0, fmt.Errorf("function %s requires an explicit return type for native lowering", name)
	}
	returnType, err := e.typeAt(returnTypeNode)
	if err != nil {
		return 0, err
	}
	if fn.Async {
		if int(returnType) >= len(e.result.Types) || e.result.Types[returnType].Kind != TypePromise {
			return 0, fmt.Errorf("async function %s requires Promise<T> return type", name)
		}
		fn.ReturnType = e.result.Types[returnType].ReturnType
	} else {
		fn.ReturnType = returnType
	}
	e.result.Functions = append(e.result.Functions, fn)
	return functionID, nil
}

func (e *extractor) extractParameter(node tsast.Node) (Parameter, error) {
	nameNode, ok := node.NamedChild("name")
	if !ok {
		return Parameter{}, fmt.Errorf("parameter at %d has no name", node.Pos())
	}
	name, _ := nameNode.Text()
	symbol, err := e.client.GetSymbolAtLocation(e.ctx, e.snapshot, e.project, nameNode.Handle(e.currentFile()))
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
	param := Parameter{Symbol: symbolID, Name: name, Type: typeID, Span: e.span(node)}
	if _, ok := node.NamedChild("questionToken"); ok {
		param.Optional = true
	}
	if _, ok := node.NamedChild("dotDotDotToken"); ok {
		param.Rest = true
	} else {
		for _, child := range node.Children() {
			if child.Kind() == tsast.KindDotDotDotToken {
				param.Rest = true
				break
			}
		}
	}
	if initNode, ok := node.NamedChild("initializer"); ok {
		initExpr, err := e.extractExpr(initNode)
		if err != nil {
			return Parameter{}, err
		}
		param.Initializer = initExpr
	}
	return param, nil
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
	case tsast.KindTryStatement:
		return e.extractTryStatement(node)
	case tsast.KindThrowStatement:
		if e.currentFunction == nil || int(*e.currentFunction) >= len(e.result.Functions) || !e.result.Functions[*e.currentFunction].Async {
			return Statement{}, fmt.Errorf("throw at %d is currently supported only in native async functions", node.Pos())
		}
		exprNode, ok := node.NamedChild("expression")
		if !ok {
			return Statement{}, fmt.Errorf("throw at %d requires an expression", node.Pos())
		}
		expr, err := e.extractExpr(exprNode)
		if err != nil {
			return Statement{}, err
		}
		anyType := e.ensureSemanticType(TypeAny, "any")
		return Statement{Kind: StmtThrow, Span: e.span(node), Type: anyType, Value: expr}, nil
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
		before := cloneConcreteClasses(e.concreteClasses)
		thenNode, ok := node.NamedChild("thenStatement")
		if !ok {
			return Statement{}, fmt.Errorf("if statement at %d has no then branch", node.Pos())
		}
		var thenBody []Statement
		thenState, err := e.withConcreteSnapshot(func() error {
			var inner error
			thenBody, inner = e.extractStatementBody(thenNode)
			return inner
		})
		if err != nil {
			return Statement{}, err
		}
		stmt := Statement{Kind: StmtIf, Span: e.span(node), Expr: condition, Then: thenBody}
		elseState := before
		if elseNode, ok := node.NamedChild("elseStatement"); ok {
			elseState, err = e.withConcreteSnapshot(func() error {
				var inner error
				stmt.Else, inner = e.extractStatementBody(elseNode)
				return inner
			})
			if err != nil {
				return Statement{}, err
			}
		}
		e.concreteClasses = mergeConcreteClasses(thenState, elseState)
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
	case tsast.KindDoStatement:
		return e.extractDoWhile(node)
	case tsast.KindForStatement:
		return e.extractFor(node)
	case tsast.KindForOfStatement:
		return e.extractForOf(node)
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
	case tsast.KindSwitchStatement:
		return e.extractSwitch(node)
	default:
		return Statement{}, fmt.Errorf("unsupported native statement %s at %d", tsast.KindName(node.Kind()), node.Pos())
	}
}

func (e *extractor) extractSwitch(node tsast.Node) (Statement, error) {
	exprNode, ok := node.NamedChild("expression")
	if !ok {
		return Statement{}, fmt.Errorf("switch at %d has no expression", node.Pos())
	}
	subject, err := e.extractExpr(exprNode)
	if err != nil {
		return Statement{}, err
	}
	caseBlock, ok := node.NamedChild("caseBlock")
	if !ok {
		return Statement{}, fmt.Errorf("switch at %d has no caseBlock", node.Pos())
	}

	type clauseItem struct {
		isDefault bool
		matchExpr *Expr
		body      []Statement
	}
	var clauses []clauseItem
	for _, child := range caseBlock.Children() {
		switch child.Kind() {
		case tsast.KindCaseClause:
			subs := child.Children()
			if len(subs) == 0 {
				continue
			}
			matchExpr, err := e.extractExpr(subs[0])
			if err != nil {
				return Statement{}, err
			}
			var body []Statement
			for _, stmtNode := range subs[1:] {
				if stmtNode.Kind() == tsast.KindBreakStatement {
					continue
				}
				s, err := e.extractStatement(stmtNode)
				if err != nil {
					return Statement{}, err
				}
				body = append(body, s)
			}
			clauses = append(clauses, clauseItem{isDefault: false, matchExpr: matchExpr, body: body})
		case tsast.KindDefaultClause:
			subs := child.Children()
			var body []Statement
			for _, stmtNode := range subs {
				if stmtNode.Kind() == tsast.KindBreakStatement {
					continue
				}
				s, err := e.extractStatement(stmtNode)
				if err != nil {
					return Statement{}, err
				}
				body = append(body, s)
			}
			clauses = append(clauses, clauseItem{isDefault: true, body: body})
		}
	}

	var currentElse []Statement
	for _, cl := range clauses {
		if cl.isDefault {
			currentElse = cl.body
			break
		}
	}

	for i := len(clauses) - 1; i >= 0; i-- {
		cl := clauses[i]
		if cl.isDefault {
			continue
		}
		cond := &Expr{
			Kind:     ExprBinary,
			Operator: BinaryStrictEqual,
			Left:     subject,
			Right:    cl.matchExpr,
			Type:     1,
			Span:     e.span(node),
		}
		stmt := Statement{
			Kind: StmtIf,
			Span: e.span(node),
			Expr: cond,
			Then: cl.body,
			Else: currentElse,
		}
		currentElse = []Statement{stmt}
	}
	if len(currentElse) > 0 {
		return currentElse[0], nil
	}
	return Statement{Kind: StmtBlock, Span: e.span(node)}, nil
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
		stmts, err := e.extractVariableDeclaration(declaration)
		if err != nil {
			return nil, err
		}
		result = append(result, stmts...)
	}
	return result, nil
}

func (e *extractor) buildIndexRead(object *Expr, index int, span Span) (*Expr, error) {
	numberType := e.ensureSemanticType(TypeNumber, "number")
	elemType := numberType
	if int(object.Type) < len(e.result.Types) {
		objectType := e.result.Types[object.Type]
		if objectType.Kind == TypeArray && int(objectType.Element) < len(e.result.Types) {
			elemType = objectType.Element
		}
	}
	return &Expr{
		Kind:   ExprIndex,
		Type:   elemType,
		Object: object,
		Index: &Expr{
			Kind:   ExprNumber,
			Type:   numberType,
			Number: float64(index),
			Span:   span,
		},
		Span: span,
	}, nil
}

func (e *extractor) buildPropertyRead(object *Expr, name string, span Span) (*Expr, error) {
	if int(object.Type) < len(e.result.Types) {
		objectType := e.result.Types[object.Type]
		if objectType.Kind == TypeObject && int(objectType.Shape) < len(e.result.Shapes) {
			shape := e.result.Shapes[objectType.Shape]
			for i, field := range shape.Fields {
				if field.Name == name {
					return &Expr{
						Kind:       ExprFieldGet,
						Type:       field.Type,
						Object:     object,
						Field:      name,
						FieldIndex: uint32(i),
						Span:       span,
					}, nil
				}
			}
		}
	}
	return &Expr{
		Kind:   ExprDynamicFieldGet,
		Object: object,
		Field:  name,
		Span:   span,
	}, nil
}

func (e *extractor) extractVariableDeclaration(node tsast.Node) ([]Statement, error) {
	nameNode, ok := node.NamedChild("name")
	if !ok {
		return nil, fmt.Errorf("native variable at %d has no name", node.Pos())
	}
	initializer, ok := node.NamedChild("initializer")
	if !ok {
		return nil, fmt.Errorf("native variable at %d requires an initializer", node.Pos())
	}

	if nameNode.Kind() == tsast.KindIdentifier {
		name, _ := nameNode.Text()
		symbol, err := e.client.GetSymbolAtLocation(e.ctx, e.snapshot, e.project, nameNode.Handle(e.currentFile()))
		if err != nil || symbol == nil {
			if err == nil {
				err = fmt.Errorf("variable %s has no TypeScript symbol", name)
			}
			return nil, err
		}
		typeID, err := e.typeAt(nameNode)
		if err != nil {
			return nil, err
		}
		symbolID := e.internSymbol(symbol, SymbolVariable, nameNode)
		e.result.Symbols[symbolID].Type = typeID

		if initializer.Kind() == tsast.KindArrowFunction || initializer.Kind() == tsast.KindFunctionExpression {
			closure, err := e.extractLocalClosure(name, symbol, initializer)
			if err != nil {
				return nil, err
			}
			e.closures[symbol.ID] = closure
			return []Statement{{Kind: StmtClosureBind, Span: e.span(node), Symbol: symbolID, Name: name, Type: typeID}}, nil
		}
		value, err := e.extractExpr(initializer)
		if err != nil {
			return nil, err
		}
		if value.Kind == ExprArray {
			e.arrayConstants[symbolID] = value
		}
		e.recordConcreteClass(symbolID, value)
		return []Statement{{Kind: StmtVar, Span: e.span(node), Symbol: symbolID, Name: name, Type: typeID, Value: value}}, nil
	}

	if nameNode.Kind() == tsast.KindArrayBindingPattern || nameNode.Kind() == tsast.KindObjectBindingPattern {
		rhs, err := e.extractExpr(initializer)
		if err != nil {
			return nil, err
		}
		var statements []Statement
		rhsVar := rhs
		if rhs.Kind != ExprIdentifier {
			tempName := fmt.Sprintf("__destruct_tmp_%d", node.Pos())
			tempSym := SymbolID(len(e.result.Symbols))
			e.result.Symbols = append(e.result.Symbols, Symbol{
				ID: tempSym, Name: tempName, Kind: SymbolVariable, Type: rhs.Type, Decl: e.span(node),
			})
			statements = append(statements, Statement{
				Kind: StmtVar, Span: e.span(node), Symbol: tempSym, Name: tempName, Type: rhs.Type, Value: rhs,
			})
			rhsVar = &Expr{Kind: ExprIdentifier, Symbol: tempSym, Name: tempName, Type: rhs.Type, Span: e.span(node)}
		}

		if nameNode.Kind() == tsast.KindArrayBindingPattern {
			for i, elem := range nameNode.Children() {
				if elem.Kind() != tsast.KindBindingElement {
					continue
				}
				var elemNameNode tsast.Node
				if nameChild, ok := elem.NamedChild("name"); ok && nameChild.Kind() == tsast.KindIdentifier {
					elemNameNode = nameChild
				} else {
					subs := elem.Children()
					if len(subs) > 0 && subs[0].Kind() == tsast.KindIdentifier {
						elemNameNode = subs[0]
					} else {
						continue
					}
				}
				elemName, _ := elemNameNode.Text()
				elemSymbol, err := e.client.GetSymbolAtLocation(e.ctx, e.snapshot, e.project, elemNameNode.Handle(e.currentFile()))
				if err != nil || elemSymbol == nil {
					if err == nil {
						err = fmt.Errorf("destructured variable %s has no TypeScript symbol", elemName)
					}
					return nil, err
				}
				elemType, err := e.typeAt(elemNameNode)
				if err != nil {
					return nil, err
				}
				elemSymbolID := e.internSymbol(elemSymbol, SymbolVariable, elemNameNode)
				e.result.Symbols[elemSymbolID].Type = elemType

				readExpr, err := e.buildIndexRead(rhsVar, i, e.span(elem))
				if err != nil {
					return nil, err
				}
				if elemType != 0 {
					readExpr.Type = elemType
				}
				if initNode, ok := elem.NamedChild("initializer"); ok {
					defaultExpr, err := e.extractExpr(initNode)
					if err != nil {
						return nil, err
					}
					readExpr = &Expr{
						Kind:     ExprBinary,
						Operator: BinaryNullishCoalesce,
						Left:     readExpr,
						Right:    defaultExpr,
						Type:     elemType,
						Span:     e.span(elem),
					}
				}
				statements = append(statements, Statement{
					Kind:   StmtVar,
					Span:   e.span(elem),
					Symbol: elemSymbolID,
					Name:   elemName,
					Type:   elemType,
					Value:  readExpr,
				})
			}
			return statements, nil
		}

		if nameNode.Kind() == tsast.KindObjectBindingPattern {
			for _, elem := range nameNode.Children() {
				if elem.Kind() != tsast.KindBindingElement {
					continue
				}
				var elemNameNode tsast.Node
				var propName string
				if nameChild, ok := elem.NamedChild("name"); ok && nameChild.Kind() == tsast.KindIdentifier {
					elemNameNode = nameChild
					elemName, _ := elemNameNode.Text()
					propName = elemName
					if propChild, ok := elem.NamedChild("propertyName"); ok && propChild.Kind() == tsast.KindIdentifier {
						propName, _ = propChild.Text()
					}
				} else {
					subs := elem.Children()
					if len(subs) == 1 && subs[0].Kind() == tsast.KindIdentifier {
						elemNameNode = subs[0]
						elemName, _ := elemNameNode.Text()
						propName = elemName
					} else if len(subs) >= 2 && subs[0].Kind() == tsast.KindIdentifier && subs[1].Kind() == tsast.KindIdentifier {
						propName, _ = subs[0].Text()
						elemNameNode = subs[1]
					} else {
						continue
					}
				}
				elemName, _ := elemNameNode.Text()
				elemSymbol, err := e.client.GetSymbolAtLocation(e.ctx, e.snapshot, e.project, elemNameNode.Handle(e.currentFile()))
				if err != nil || elemSymbol == nil {
					if err == nil {
						err = fmt.Errorf("destructured variable %s has no TypeScript symbol", elemName)
					}
					return nil, err
				}
				elemType, err := e.typeAt(elemNameNode)
				if err != nil {
					return nil, err
				}
				elemSymbolID := e.internSymbol(elemSymbol, SymbolVariable, elemNameNode)
				e.result.Symbols[elemSymbolID].Type = elemType

				readExpr, err := e.buildPropertyRead(rhsVar, propName, e.span(elem))
				if err != nil {
					return nil, err
				}
				if elemType != 0 {
					readExpr.Type = elemType
				}
				if initNode, ok := elem.NamedChild("initializer"); ok {
					defaultExpr, err := e.extractExpr(initNode)
					if err != nil {
						return nil, err
					}
					readExpr = &Expr{
						Kind:     ExprBinary,
						Operator: BinaryNullishCoalesce,
						Left:     readExpr,
						Right:    defaultExpr,
						Type:     elemType,
						Span:     e.span(elem),
					}
				}
				statements = append(statements, Statement{
					Kind:   StmtVar,
					Span:   e.span(elem),
					Symbol: elemSymbolID,
					Name:   elemName,
					Type:   elemType,
					Value:  readExpr,
				})
			}
			return statements, nil
		}
	}

	return nil, fmt.Errorf("native variable at %d (name kind=%d %s) requires an identifier name", node.Pos(), nameNode.Kind(), tsast.KindName(nameNode.Kind()))
}

func (e *extractor) extractTryStatement(node tsast.Node) (Statement, error) {
	if e.currentFunction == nil || int(*e.currentFunction) >= len(e.result.Functions) || !e.result.Functions[*e.currentFunction].Async {
		return Statement{}, fmt.Errorf("try/catch at %d is currently supported only in native async functions", node.Pos())
	}
	tryBlock, ok := node.NamedChild("tryBlock")
	if !ok {
		return Statement{}, fmt.Errorf("try statement at %d has no try block", node.Pos())
	}
	catchClause, ok := node.NamedChild("catchClause")
	if !ok {
		return Statement{}, fmt.Errorf("try statement at %d requires a catch clause", node.Pos())
	}
	decl, ok := catchClause.NamedChild("variableDeclaration")
	if !ok {
		return Statement{}, fmt.Errorf("catch clause at %d requires a binding", catchClause.Pos())
	}
	nameNode, ok := decl.NamedChild("name")
	if !ok || nameNode.Kind() != tsast.KindIdentifier {
		return Statement{}, fmt.Errorf("catch clause at %d requires an identifier binding", catchClause.Pos())
	}
	symbol, err := e.client.GetSymbolAtLocation(e.ctx, e.snapshot, e.project, nameNode.Handle(e.currentFile()))
	if err != nil || symbol == nil {
		if err == nil {
			err = fmt.Errorf("catch binding has no TypeScript symbol")
		}
		return Statement{}, err
	}
	anyType := e.ensureSemanticType(TypeAny, "any")
	e.ensureSemanticType(TypeBoolean, "boolean")
	e.ensureSemanticType(TypeVoid, "void")
	catchSymbol := e.internSymbol(symbol, SymbolVariable, nameNode)
	e.result.Symbols[catchSymbol].Type = anyType
	tryBody, err := e.extractBlock(tryBlock)
	if err != nil {
		return Statement{}, err
	}
	catchBlock, ok := catchClause.NamedChild("block")
	if !ok {
		return Statement{}, fmt.Errorf("catch clause at %d has no block", catchClause.Pos())
	}
	catchBody, err := e.extractBlock(catchBlock)
	if err != nil {
		return Statement{}, err
	}
	var finallyBody []Statement
	if finallyBlock, ok := node.NamedChild("finallyBlock"); ok {
		finallyBody, err = e.extractBlock(finallyBlock)
		if err != nil {
			return Statement{}, err
		}
	}
	return Statement{Kind: StmtTry, Span: e.span(node), Then: tryBody, Catch: catchBody, Finally: finallyBody, CatchSymbol: catchSymbol, CatchType: anyType}, nil
}

func statementsContainReturn(statements []Statement) bool {
	for _, stmt := range statements {
		if stmt.Kind == StmtReturn {
			return true
		}
		if statementsContainReturn(stmt.Then) || statementsContainReturn(stmt.Else) || statementsContainReturn(stmt.Catch) || statementsContainReturn(stmt.Finally) {
			return true
		}
	}
	return false
}

func statementsContainThrow(statements []Statement) bool {
	for _, stmt := range statements {
		if stmt.Kind == StmtThrow {
			return true
		}
		if statementsContainThrow(stmt.Then) || statementsContainThrow(stmt.Else) || statementsContainThrow(stmt.Catch) || statementsContainThrow(stmt.Finally) {
			return true
		}
	}
	return false
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
	before := cloneConcreteClasses(e.concreteClasses)
	body, err := e.extractStatementBody(bodyNode)
	if err != nil {
		return Statement{}, err
	}
	e.concreteClasses = mergeConcreteClasses(before, e.concreteClasses)
	return Statement{Kind: StmtWhile, Span: e.span(node), Expr: condition, Then: body}, nil
}

func (e *extractor) extractDoWhile(node tsast.Node) (Statement, error) {
	bodyNode, ok := node.NamedChild("statement")
	if !ok {
		return Statement{}, fmt.Errorf("do/while at %d has no body", node.Pos())
	}
	body, err := e.extractStatementBody(bodyNode)
	if err != nil {
		return Statement{}, err
	}
	conditionNode, ok := node.NamedChild("expression")
	if !ok {
		return Statement{}, fmt.Errorf("do/while at %d has no condition", node.Pos())
	}
	condition, err := e.extractExpr(conditionNode)
	if err != nil {
		return Statement{}, err
	}
	return Statement{Kind: StmtDoWhile, Span: e.span(node), Expr: condition, Then: body}, nil
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
	loopEntry := cloneConcreteClasses(e.concreteClasses)
	bodyNode, ok := node.NamedChild("statement")
	if !ok {
		return Statement{}, fmt.Errorf("for at %d has no body", node.Pos())
	}
	stmt.Then, err = e.extractStatementBody(bodyNode)
	if err != nil {
		return Statement{}, err
	}
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
	e.concreteClasses = mergeConcreteClasses(loopEntry, e.concreteClasses)
	return stmt, nil
}

func (e *extractor) extractForOf(node tsast.Node) (Statement, error) {
	children := node.Children()
	var initNode, exprNode, bodyNode tsast.Node
	if in, ok := node.NamedChild("initializer"); ok {
		initNode = in
	}
	if ex, ok := node.NamedChild("expression"); ok {
		exprNode = ex
	}
	if st, ok := node.NamedChild("statement"); ok {
		bodyNode = st
	}
	if initNode.Kind() == 0 && len(children) >= 3 {
		initNode = children[len(children)-3]
	}
	if exprNode.Kind() == 0 && len(children) >= 2 {
		exprNode = children[len(children)-2]
	}
	if bodyNode.Kind() == 0 && len(children) >= 1 {
		bodyNode = children[len(children)-1]
	}
	if initNode.Kind() == 0 || exprNode.Kind() == 0 || bodyNode.Kind() == 0 {
		return Statement{}, fmt.Errorf("malformed for...of loop at %d", node.Pos())
	}

	iterable, err := e.extractExpr(exprNode)
	if err != nil {
		return Statement{}, err
	}

	numberType := e.ensureSemanticType(TypeNumber, "number")
	boolType := e.ensureSemanticType(TypeBoolean, "boolean")

	// 1. Assign iterable to temporary variable: const __for_of_arr_* = iterable;
	arrTempName := fmt.Sprintf("__for_of_arr_%d", node.Pos())
	arrTempSym := SymbolID(len(e.result.Symbols))
	e.result.Symbols = append(e.result.Symbols, Symbol{
		ID:   arrTempSym,
		Name: arrTempName,
		Kind: SymbolVariable,
		Type: iterable.Type,
		Decl: e.span(node),
	})
	arrVar := &Expr{Kind: ExprIdentifier, Symbol: arrTempSym, Name: arrTempName, Type: iterable.Type, Span: e.span(exprNode)}
	arrStmt := Statement{Kind: StmtVar, Span: e.span(exprNode), Symbol: arrTempSym, Name: arrTempName, Type: iterable.Type, Value: iterable}

	// 2. Loop index: let __for_of_idx_* = 0;
	idxTempName := fmt.Sprintf("__for_of_idx_%d", node.Pos())
	idxTempSym := SymbolID(len(e.result.Symbols))
	e.result.Symbols = append(e.result.Symbols, Symbol{
		ID:   idxTempSym,
		Name: idxTempName,
		Kind: SymbolVariable,
		Type: numberType,
		Decl: e.span(node),
	})
	idxVar := &Expr{Kind: ExprIdentifier, Symbol: idxTempSym, Name: idxTempName, Type: numberType, Span: e.span(node)}
	idxInit := Statement{Kind: StmtVar, Span: e.span(node), Symbol: idxTempSym, Name: idxTempName, Type: numberType, Value: &Expr{Kind: ExprNumber, Number: 0, Type: numberType, Span: e.span(node)}}

	// 3. Condition: __for_of_idx_* < __for_of_arr_*.length;
	lenExpr := &Expr{Kind: ExprArrayLength, Object: arrVar, Type: numberType, Span: e.span(node)}
	condExpr := &Expr{Kind: ExprBinary, Operator: BinaryLessThan, Left: idxVar, Right: lenExpr, Type: boolType, Span: e.span(node)}

	// 4. Update: __for_of_idx_* = __for_of_idx_* + 1;
	oneExpr := &Expr{Kind: ExprNumber, Number: 1, Type: numberType, Span: e.span(node)}
	addExpr := &Expr{Kind: ExprBinary, Operator: BinaryAdd, Left: idxVar, Right: oneExpr, Type: numberType, Span: e.span(node)}
	updateStmt := Statement{Kind: StmtAssign, Span: e.span(node), Symbol: idxTempSym, Name: idxTempName, Type: numberType, Value: addExpr}

	// 5. Item variable declaration at start of loop body
	var declNode tsast.Node
	if initNode.Kind() == tsast.KindVariableDeclarationList {
		for _, c := range initNode.Children() {
			if c.Kind() == tsast.KindVariableDeclaration {
				declNode = c
				break
			}
		}
	} else if initNode.Kind() == tsast.KindVariableDeclaration {
		declNode = initNode
	}
	if declNode.Kind() == 0 {
		return Statement{}, fmt.Errorf("unsupported for...of loop variable declaration at %d", initNode.Pos())
	}

	nameNode, ok := declNode.NamedChild("name")
	if !ok {
		subs := declNode.Children()
		if len(subs) > 0 {
			nameNode = subs[0]
		}
	}
	if nameNode.Kind() == 0 {
		return Statement{}, fmt.Errorf("for...of variable at %d has no name", declNode.Pos())
	}

	itemName, _ := nameNode.Text()
	itemSymbol, err := e.client.GetSymbolAtLocation(e.ctx, e.snapshot, e.project, nameNode.Handle(e.currentFile()))
	if err != nil || itemSymbol == nil {
		if err == nil {
			err = fmt.Errorf("for...of variable %s has no TypeScript symbol", itemName)
		}
		return Statement{}, err
	}
	itemType, err := e.typeAt(nameNode)
	if err != nil {
		return Statement{}, err
	}
	itemSymID := e.internSymbol(itemSymbol, SymbolVariable, nameNode)
	e.result.Symbols[itemSymID].Type = itemType

	readExpr := &Expr{Kind: ExprIndex, Object: arrVar, Index: idxVar, Type: itemType, Span: e.span(declNode)}
	itemStmt := Statement{Kind: StmtVar, Span: e.span(declNode), Symbol: itemSymID, Name: itemName, Type: itemType, Value: readExpr}

	loopEntry := cloneConcreteClasses(e.concreteClasses)
	bodyStmts, err := e.extractStatementBody(bodyNode)
	if err != nil {
		return Statement{}, err
	}
	loopBody := append([]Statement{itemStmt}, bodyStmts...)
	e.concreteClasses = mergeConcreteClasses(loopEntry, e.concreteClasses)

	forStmt := Statement{
		Kind:   StmtFor,
		Span:   e.span(node),
		Init:   []Statement{idxInit},
		Expr:   condExpr,
		Update: []Statement{updateStmt},
		Then:   loopBody,
	}

	return Statement{
		Kind: StmtBlock,
		Span: e.span(node),
		Then: []Statement{arrStmt, forStmt},
	}, nil
}

func (e *extractor) extractMutation(node tsast.Node) (Statement, bool, error) {
	if node.Kind() == tsast.KindBinaryExpression {
		opNode, ok := node.NamedChild("operatorToken")
		if !ok || opNode.Kind() != tsast.KindEqualsToken {
			return Statement{}, false, nil
		}
		left, ok := node.NamedChild("left")
		if !ok {
			return Statement{}, true, fmt.Errorf("assignment at %d has no target", node.Pos())
		}
		right, ok := node.NamedChild("right")
		if !ok {
			return Statement{}, true, fmt.Errorf("assignment at %d has no value", node.Pos())
		}
		switch left.Kind() {
		case tsast.KindIdentifier:
			return e.buildAssignment(node, left, right, 0)
		case tsast.KindPropertyAccessExpression:
			return e.buildFieldAssignment(node, left, right)
		case tsast.KindElementAccessExpression:
			return e.buildArrayAssignment(node, left, right)
		default:
			return Statement{}, true, fmt.Errorf("native assignment at %d does not support %s target", node.Pos(), tsast.KindName(left.Kind()))
		}
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
	symbol, err := e.client.GetSymbolAtLocation(e.ctx, e.snapshot, e.project, target.Handle(e.currentFile()))
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
	e.recordConcreteClass(symbolID, value)
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
	if node.Kind() == tsast.KindThisKeyword {
		if e.currentThis == nil {
			return nil, fmt.Errorf("this at %d is outside a native method", node.Pos())
		}
		typeID := e.result.Symbols[*e.currentThis].Type
		return &Expr{Kind: ExprIdentifier, Type: typeID, Name: "this", Symbol: *e.currentThis, Span: e.span(node)}, nil
	}
	typeID, err := e.typeAt(node)
	if err != nil {
		return nil, err
	}
	expr := &Expr{Type: typeID, Span: e.span(node)}
	switch node.Kind() {
	case tsast.KindTrueKeyword, tsast.KindFalseKeyword:
		expr.Kind = ExprBoolean
		expr.Boolean = node.Kind() == tsast.KindTrueKeyword
		return expr, nil
	case tsast.KindNullKeyword:
		expr.Kind = ExprNull
		return expr, nil
	case tsast.KindIdentifier:
		expr.Kind = ExprIdentifier
		expr.Name, _ = node.Text()
		if expr.Name == "undefined" && int(typeID) < len(e.result.Types) && e.result.Types[typeID].Kind == TypeUndefined {
			expr.Kind = ExprUndefined
			return expr, nil
		}
		if alias, ok := e.parameterAliases[expr.Name]; ok {
			expr.Symbol = alias
			return expr, nil
		}
		symbol, err := e.client.GetSymbolAtLocation(e.ctx, e.snapshot, e.project, node.Handle(e.currentFile()))
		if err != nil {
			return nil, err
		}
		if symbol != nil {
			if replacement, ok := e.symbolSubstitutions[symbol.ID]; ok {
				expr.Symbol = replacement
				return expr, nil
			}
			if closure, ok := e.closures[symbol.ID]; ok {
				target := closure.Function
				expr.Kind = ExprClosure
				expr.CallTarget = &target
				expr.Captures = e.closureCaptureArgs(closure, e.span(node))
				return expr, nil
			}
			if target, ok := e.functions[symbol.ID]; ok {
				targetCopy := target
				expr.Kind = ExprClosure
				expr.CallTarget = &targetCopy
				return expr, nil
			}
			expr.Symbol = e.internSymbol(symbol, SymbolVariable, node)
			e.result.Symbols[expr.Symbol].Type = typeID
			if concrete, ok := e.concreteClasses[expr.Symbol]; ok {
				expr.ConcreteType, expr.ConcreteKnown = concrete.Type, true
			}
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
	case tsast.KindRegularExpressionLiteral:
		text, ok := node.Text()
		if !ok {
			return nil, fmt.Errorf("regexp literal at %d has no text", node.Pos())
		}
		lastSlash := strings.LastIndex(text, "/")
		if lastSlash <= 0 {
			return nil, fmt.Errorf("invalid regular expression literal at %d: %s", node.Pos(), text)
		}
		pattern := text[1:lastSlash]
		flags := text[lastSlash+1:]
		stringType := e.ensureSemanticType(TypeString, "string")
		regExpType := e.ensureSemanticType(TypeRegExp, "RegExp")
		args := []*Expr{
			{Kind: ExprString, String: pattern, Type: stringType, Span: e.span(node)},
		}
		if flags != "" {
			args = append(args, &Expr{Kind: ExprString, String: flags, Type: stringType, Span: e.span(node)})
		}
		expr.Kind = ExprRegExpNew
		expr.Args = args
		expr.Type = regExpType
		return expr, nil
	case tsast.KindArrowFunction, tsast.KindFunctionExpression, tsast.KindMethodDeclaration:
		info, err := e.extractLocalClosure("inline", nil, node)
		if err != nil {
			return nil, err
		}
		target := info.Function
		expr.Kind = ExprClosure
		expr.CallTarget = &target
		expr.Captures = e.closureCaptureArgs(info, e.span(node))
		return expr, nil
	case tsast.KindBinaryExpression:
		return e.extractBinary(node, expr)
	case tsast.KindCallExpression:
		return e.extractCall(node, expr)
	case tsast.KindNewExpression:
		return e.extractNew(node, expr)
	case tsast.KindArrayLiteralExpression:
		expr.Kind = ExprArray
		elements, ok := node.NamedChild("elements")
		if !ok || !elements.IsList() {
			return nil, fmt.Errorf("array literal at %d has no element list", node.Pos())
		}
		if int(typeID) >= len(e.result.Types) || e.result.Types[typeID].Kind != TypeArray {
			t := Type{}
			if int(typeID) < len(e.result.Types) {
				t = e.result.Types[typeID]
			}
			return nil, fmt.Errorf("array literal at %d has non-array type: kind=%d name=%q", node.Pos(), t.Kind, t.Name)
		}
		arrayType := e.result.Types[typeID]
		if int(arrayType.Element) >= len(e.result.Types) {
			return nil, fmt.Errorf("array literal at %d has invalid element type", node.Pos())
		}
		elementKind := e.result.Types[arrayType.Element].Kind
		switch elementKind {
		case TypeNumber, TypeBoolean, TypeString, TypeObject, TypeArray, TypeFunction, TypeAny, TypeUnion, TypePromise, TypeParameter, TypeNever:
		default:
			return nil, fmt.Errorf("native array at %d does not support %s elements yet", node.Pos(), e.result.Types[arrayType.Element].Name)
		}
		hasSpread := false
		for _, elementNode := range elements.ListElements() {
			if elementNode.Kind() == tsast.KindSpreadElement {
				hasSpread = true
				break
			}
		}
		if hasSpread {
			var chunks []*Expr
			var currentLiterals []*Expr
			for _, elementNode := range elements.ListElements() {
				if elementNode.Kind() == tsast.KindSpreadElement {
					if len(currentLiterals) > 0 {
						chunks = append(chunks, &Expr{
							Kind:     ExprArray,
							Type:     typeID,
							Elements: currentLiterals,
							Span:     e.span(node),
						})
						currentLiterals = nil
					}
					exprNode, ok := elementNode.NamedChild("expression")
					if !ok {
						return nil, fmt.Errorf("spread element at %d has no expression", elementNode.Pos())
					}
					spreadArr, err := e.extractExpr(exprNode)
					if err != nil {
						return nil, err
					}
					chunks = append(chunks, spreadArr)
				} else {
					elem, err := e.extractExpr(elementNode)
					if err != nil {
						return nil, err
					}
					currentLiterals = append(currentLiterals, elem)
				}
			}
			if len(currentLiterals) > 0 {
				chunks = append(chunks, &Expr{
					Kind:     ExprArray,
					Type:     typeID,
					Elements: currentLiterals,
					Span:     e.span(node),
				})
			}
			return &Expr{
				Kind:     ExprArrayConcat,
				Type:     typeID,
				Elements: chunks,
				Span:     e.span(node),
			}, nil
		}
		for _, elementNode := range elements.ListElements() {
			element, err := e.extractExpr(elementNode)
			if err != nil {
				return nil, err
			}
			if int(element.Type) >= len(e.result.Types) || !e.compatibleArrayElement(arrayType.Element, element.Type) {
				return nil, fmt.Errorf("native array at %d requires homogeneous %s elements", node.Pos(), e.result.Types[arrayType.Element].Name)
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
			if property.Kind() == tsast.KindSpreadAssignment {
				exprNode, ok := property.NamedChild("expression")
				if !ok {
					return nil, fmt.Errorf("spread property at %d has no expression", property.Pos())
				}
				spreadObj, err := e.extractExpr(exprNode)
				if err != nil {
					return nil, err
				}
				if int(spreadObj.Type) < len(e.result.Types) && e.result.Types[spreadObj.Type].Kind == TypeObject {
					srcShapeID := e.result.Types[spreadObj.Type].Shape
					if int(srcShapeID) < len(e.result.Shapes) {
						srcShape := e.result.Shapes[srcShapeID]
						for fIdx, field := range srcShape.Fields {
							values[field.Name] = &Expr{
								Kind:       ExprFieldGet,
								Object:     spreadObj,
								Field:      field.Name,
								FieldIndex: uint32(fIdx),
								Type:       field.Type,
								Span:       e.span(property),
							}
						}
					}
				}
				continue
			}
			nameNode, ok := property.NamedChild("name")
			if !ok || (nameNode.Kind() != tsast.KindIdentifier && nameNode.Kind() != tsast.KindStringLiteral) {
				return nil, fmt.Errorf("native object property at %d requires identifier or string literal name", property.Pos())
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
			case tsast.KindMethodDeclaration:
				value, err = e.extractExpr(property)
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
		if int(object.Type) < len(e.result.Types) {
			objType := e.result.Types[object.Type]
			if objType.Kind == TypeArray {
				if int(objType.Element) >= len(e.result.Types) {
					return nil, fmt.Errorf("native element access at %d has invalid array element type", node.Pos())
				}
				if int(index.Type) < len(e.result.Types) && (e.result.Types[index.Type].Kind == TypeAny || e.result.Types[index.Type].Kind == TypeUnion) {
					expr.Kind, expr.Object, expr.Index = ExprDynamicIndexGet, object, index
					expr.Type = e.ensureSemanticType(TypeAny, "any")
					return expr, nil
				}
				expr.Type = objType.Element
				expr.Kind, expr.Object, expr.Index = ExprIndex, object, index
				return expr, nil
			}
			if objType.Kind == TypeObject && int(objType.Shape) < len(e.result.Shapes) && index.Kind == ExprString {
				shape := e.result.Shapes[objType.Shape]
				for i, field := range shape.Fields {
					if field.Name == index.String {
						expr.Kind, expr.Object, expr.Field, expr.FieldIndex = ExprFieldGet, object, field.Name, uint32(i)
						expr.Type = field.Type
						return expr, nil
					}
				}
			}
			if objType.Kind == TypeAny || objType.Kind == TypeUnion || objType.Kind == TypeObject {
				expr.Kind, expr.Object, expr.Index = ExprDynamicIndexGet, object, index
				expr.Type = e.ensureSemanticType(TypeAny, "any")
				return expr, nil
			}
		}
		return nil, fmt.Errorf("native element access at %d requires an array or dynamic receiver", node.Pos())
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
		if objectNode.Kind() == tsast.KindIdentifier {
			objName, _ := objectNode.Text()
			if enumInfo, ok := e.enums[objName]; ok {
				if val, ok := enumInfo.Members[name]; ok {
					numberType := e.ensureSemanticType(TypeNumber, "number")
					return &Expr{Kind: ExprNumber, Number: val, Type: numberType, Span: e.span(node)}, nil
				}
			}
		}
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
		if objectType.Kind == TypeMap && name == "size" {
			expr.Kind, expr.Object = ExprMapSize, object
			return expr, nil
		}
		if objectType.Kind == TypeSet && name == "size" {
			expr.Kind, expr.Object = ExprSetSize, object
			return expr, nil
		}
		if objectType.Kind == TypeRegExp && name == "source" {
			expr.Kind, expr.Object = ExprRegExpSource, object
			return expr, nil
		}
		if objectType.Kind == TypeAny || objectType.Kind == TypeUnion {
			expr.Kind, expr.Object, expr.Field = ExprDynamicFieldGet, object, name
			return expr, nil
		}
		if objectType.Kind != TypeObject || int(objectType.Shape) >= len(e.result.Shapes) {
			return nil, fmt.Errorf("native property %q at %d requires a closed object shape; receiver type=%q kind=%d", name, node.Pos(), objectType.Name, objectType.Kind)
		}
		shape := e.result.Shapes[objectType.Shape]
		for i, field := range shape.Fields {
			if field.Name == name {
				expr.Kind, expr.Object, expr.Field, expr.FieldIndex = ExprFieldGet, object, name, uint32(i)
				return expr, nil
			}
		}
		return nil, fmt.Errorf("shape %s has no field %q", shape.Name, name)
	case tsast.KindAwaitExpression:
		innerNode, ok := node.NamedChild("expression")
		if !ok {
			return nil, fmt.Errorf("await at %d has no expression", node.Pos())
		}
		inner, err := e.extractExpr(innerNode)
		if err != nil {
			return nil, err
		}
		if int(inner.Type) >= len(e.result.Types) || (e.result.Types[inner.Type].Kind != TypePromise && e.result.Types[inner.Type].Kind != TypeTask) {
			return nil, fmt.Errorf("await at %d currently requires a native Promise/Task", node.Pos())
		}
		expr.Kind = ExprTaskJoin
		expr.TaskShared = e.result.Types[inner.Type].Kind == TypePromise
		expr.Args = []*Expr{inner}
		return expr, nil
	case tsast.KindParenthesizedExpression:
		innerNode, ok := node.NamedChild("expression")
		if !ok {
			return nil, fmt.Errorf("parenthesized expression at %d has no operand", node.Pos())
		}
		inner, err := e.extractExpr(innerNode)
		if err != nil {
			return nil, err
		}
		inner.Type, inner.Span = typeID, e.span(node)
		return inner, nil
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
	case tsast.KindPrefixUnaryExpression:
		op, ok := node.UnaryOperatorKind()
		if !ok {
			return nil, fmt.Errorf("prefix unary expression at %d has unknown operator", node.Pos())
		}
		operandNode, ok := node.NamedChild("operand")
		if !ok {
			return nil, fmt.Errorf("prefix unary expression at %d has no operand", node.Pos())
		}
		operand, err := e.extractExpr(operandNode)
		if err != nil {
			return nil, err
		}
		if op == tsast.KindMinusToken {
			zero := &Expr{Kind: ExprNumber, Number: 0, Type: operand.Type, Span: e.span(node)}
			return &Expr{Kind: ExprBinary, Operator: BinarySub, Left: zero, Right: operand, Type: operand.Type, Span: e.span(node)}, nil
		}
		if op == tsast.KindPlusToken {
			return operand, nil
		}
		return nil, fmt.Errorf("unsupported prefix unary operator %s at %d", tsast.KindName(op), node.Pos())
	case tsast.KindNoSubstitutionTemplateLiteral:
		text, _ := node.Text()
		stringType := e.ensureSemanticType(TypeString, "string")
		expr.Kind = ExprString
		expr.Type = stringType
		expr.String = text
		return expr, nil
	case tsast.KindTemplateExpression:
		stringType := e.ensureSemanticType(TypeString, "string")
		expr.Kind = ExprTemplateLiteral
		expr.Type = stringType
		children := node.Children()
		if len(children) > 0 {
			headText, _ := children[0].Text()
			if len(headText) > 0 {
				expr.Elements = append(expr.Elements, &Expr{
					Kind:   ExprString,
					Type:   stringType,
					String: headText,
					Span:   e.span(children[0]),
				})
			}
			for _, spanNode := range children[1:] {
				subs := spanNode.Children()
				if len(subs) >= 1 {
					subExpr, err := e.extractExpr(subs[0])
					if err != nil {
						return nil, err
					}
					expr.Elements = append(expr.Elements, subExpr)
				}
				if len(subs) >= 2 {
					middleText, _ := subs[1].Text()
					if len(middleText) > 0 {
						expr.Elements = append(expr.Elements, &Expr{
							Kind:   ExprString,
							Type:   stringType,
							String: middleText,
							Span:   e.span(subs[1]),
						})
					}
				}
			}
		}
		return expr, nil
	default:
		return nil, fmt.Errorf("unsupported native expression %s (kind=%d) at %d", tsast.KindName(node.Kind()), node.Kind(), node.Pos())
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
	case tsast.KindEqualsEqualsToken:
		op = BinaryEqual
	case tsast.KindExclamationEqualsToken:
		op = BinaryNotEqual
	case tsast.KindEqualsEqualsEqualsToken:
		op = BinaryStrictEqual
	case tsast.KindExclamationEqualsEqualsToken:
		op = BinaryStrictNotEqual
	case tsast.KindQuestionQuestionToken:
		op = BinaryNullishCoalesce
	case tsast.KindBarBarToken:
		op = BinaryLogicalOr
	case tsast.KindAmpersandAmpersandToken:
		op = BinaryLogicalAnd
	default:
		return nil, fmt.Errorf("unsupported binary operator %s (kind=%d) at %d", tsast.KindName(opNode.Kind()), opNode.Kind(), opNode.Pos())
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
	calleeIdentifier := ""
	var generic *genericInfo
	switch calleeNode.Kind() {
	case tsast.KindIdentifier:
		calleeName, _ := calleeNode.Text()
		calleeIdentifier = calleeName
		expr.Callee = &Expr{Kind: ExprIdentifier, Name: calleeName, Span: e.span(calleeNode)}
		calleeType, err := e.typeAt(calleeNode)
		if err != nil {
			return nil, err
		}
		dynamicCallee := int(calleeType) < len(e.result.Types) && (e.result.Types[calleeType].Kind == TypeAny || e.result.Types[calleeType].Kind == TypeUnion)
		symbol, err := e.client.GetSymbolAtLocation(e.ctx, e.snapshot, e.project, calleeNode.Handle(e.currentFile()))
		if err != nil {
			return nil, err
		}
		if symbol != nil && !dynamicCallee {
			if info, ok := e.generics[symbol.ID]; ok {
				infoCopy := info
				generic = &infoCopy
			} else if closure, ok := e.closures[symbol.ID]; ok {
				targetCopy := closure.Function
				expr.CallTarget = &targetCopy
				expr.Args = append(expr.Args, e.closureCaptureArgs(closure, e.span(calleeNode))...)
			} else if target, ok := e.functions[symbol.ID]; ok {
				targetCopy := target
				expr.CallTarget = &targetCopy
			}
		}
		if expr.CallTarget == nil && generic == nil && !isConcurrencyIntrinsic(calleeIdentifier) {
			if target, ok := e.importedFunctions[calleeIdentifier]; ok {
				targetCopy := target
				expr.CallTarget = &targetCopy
			} else {
				callee, err := e.extractExpr(calleeNode)
				if err != nil {
					return nil, err
				}
				expr.Callee = callee
			}
		}
	case tsast.KindPropertyAccessExpression:
		if isConsoleLog(calleeNode) {
			expr.Callee = &Expr{Kind: ExprIdentifier, Name: "console.log", Span: e.span(calleeNode)}
			consoleCall = true
			break
		}
		receiverNode, ok := calleeNode.NamedChild("expression")
		if !ok {
			return nil, fmt.Errorf("method call at %d has no receiver", calleeNode.Pos())
		}
		nameNode, nameOK := calleeNode.NamedChild("name")
		if receiverNode.Kind() == tsast.KindIdentifier && nameOK && nameNode.Kind() == tsast.KindIdentifier {
			receiverName, _ := receiverNode.Text()
			methodName, _ := nameNode.Text()
			if receiverName == "Promise" && (methodName == "resolve" || methodName == "reject" || methodName == "all" || methodName == "race") {
				calleeIdentifier = "Promise." + methodName
				expr.Callee = &Expr{Kind: ExprIdentifier, Name: calleeIdentifier, Span: e.span(calleeNode)}
				break
			}
			if receiverName == "JSON" && (methodName == "stringify" || methodName == "parse") {
				calleeIdentifier = "JSON." + methodName
				expr.Callee = &Expr{Kind: ExprIdentifier, Name: calleeIdentifier, Span: e.span(calleeNode)}
				break
			}
			if receiverName == "Date" && methodName == "now" {
				calleeIdentifier = "Date.now"
				expr.Callee = &Expr{Kind: ExprIdentifier, Name: calleeIdentifier, Span: e.span(calleeNode)}
				break
			}
		}
		if !nameOK || nameNode.Kind() != tsast.KindIdentifier {
			return nil, fmt.Errorf("method call at %d has no method name", calleeNode.Pos())
		}
		receiver, err := e.extractExpr(receiverNode)
		if err != nil {
			return nil, err
		}
		name, _ := nameNode.Text()
		if int(receiver.Type) < len(e.result.Types) {
			kind := e.result.Types[receiver.Type].Kind
			if kind == TypeArray && (name == "push" || name == "pop") {
				if name == "push" {
					expr.Kind = ExprArrayPush
				} else {
					expr.Kind = ExprArrayPop
				}
				expr.Object = receiver
				break
			}
			if kind == TypeMap {
				switch name {
				case "get":
					expr.Kind = ExprMapGet
				case "set":
					expr.Kind = ExprMapSet
				case "has":
					expr.Kind = ExprMapHas
				case "delete":
					expr.Kind = ExprMapDelete
				case "clear":
					expr.Kind = ExprMapClear
				}
				expr.Object = receiver
				break
			}
			if kind == TypeSet {
				switch name {
				case "add":
					expr.Kind = ExprSetAdd
				case "has":
					expr.Kind = ExprSetHas
				case "delete":
					expr.Kind = ExprSetDelete
				case "clear":
					expr.Kind = ExprSetClear
				}
				expr.Object = receiver
				break
			}
			if kind == TypeDate {
				switch name {
				case "getTime":
					expr.Kind = ExprDateGetTime
				case "toISOString":
					expr.Kind = ExprDateToISOString
				case "getFullYear", "getUTCFullYear":
					expr.Kind = ExprDateGetFullYear
				case "getMonth", "getUTCMonth":
					expr.Kind = ExprDateGetMonth
				case "getDate", "getUTCDate":
					expr.Kind = ExprDateGetDate
				case "getHours", "getUTCHours":
					expr.Kind = ExprDateGetHours
				case "getMinutes", "getUTCMinutes":
					expr.Kind = ExprDateGetMinutes
				case "getSeconds", "getUTCSeconds":
					expr.Kind = ExprDateGetSeconds
				}
				expr.Object = receiver
				break
			}
			if kind == TypeRegExp {
				switch name {
				case "test":
					expr.Kind = ExprRegExpTest
				}
				expr.Object = receiver
				break
			}
			if kind == TypeAny || kind == TypeUnion {
				if targets := e.dynamicMethodTargets(name); len(targets) != 0 {
					expr.Kind = ExprDynamicMethodCall
					expr.Object = receiver
					expr.Field = name
					expr.Dispatch = targets
				} else {
					expr.Kind = ExprDynamicCall
					expr.Object = receiver
					expr.Field = name
				}
				break
			}
		}
		method, err := e.client.GetSymbolAtLocation(e.ctx, e.snapshot, e.project, nameNode.Handle(e.currentFile()))
		if err != nil {
			return nil, err
		}
		if method == nil {
			return nil, fmt.Errorf("method call at %d has no TypeScript symbol", calleeNode.Pos())
		}
		var target FunctionID
		var targetOK bool
		if receiver.ConcreteKnown {
			if concrete, ok := e.classesByType[receiver.ConcreteType]; ok {
				target, targetOK = e.findClassMethod(concrete, method.Name)
			}
		}
		if !targetOK {
			target, targetOK = e.functions[method.ID]
			if targetOK {
				if staticClass, ok := e.classesByType[receiver.Type]; ok && e.hasKnownOverride(staticClass, method.Name, target) {
					expr.Dispatch = e.dispatchTargets(staticClass, method.Name)
				}
			}
		}
		if !targetOK {
			return nil, fmt.Errorf("method %s is not a native target", method.Name)
		}
		expr.Callee = &Expr{Kind: ExprIdentifier, Name: name, Span: e.span(calleeNode)}
		if len(expr.Dispatch) == 0 {
			targetCopy := target
			expr.CallTarget = &targetCopy
		}
		expr.Args = append(expr.Args, receiver)
	default:
		callee, err := e.extractExpr(calleeNode)
		if err != nil {
			return nil, err
		}
		expr.Callee = callee
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
	if expr.CallTarget != nil && int(*expr.CallTarget) < len(e.result.Functions) {
		target := e.result.Functions[*expr.CallTarget]
		userArgCount := 0
		if args, ok := node.NamedChild("arguments"); ok {
			userArgCount = len(args.ListElements())
		}
		offset := len(expr.Args) - userArgCount
		hasRest := len(target.Params) > 0 && target.Params[len(target.Params)-1].Rest
		if hasRest {
			restIdx := len(target.Params) - 1
			restParam := target.Params[restIdx]
			actualRestStart := restIdx + offset
			if len(expr.Args) >= actualRestStart {
				restElements := append([]*Expr(nil), expr.Args[actualRestStart:]...)
				expr.Args = append(expr.Args[:actualRestStart], &Expr{
					Kind:     ExprArray,
					Elements: restElements,
					Type:     restParam.Type,
					Span:     expr.Span,
				})
			} else {
				for i := len(expr.Args); i < actualRestStart; i++ {
					p := target.Params[i-offset]
					if p.Initializer != nil {
						expr.Args = append(expr.Args, p.Initializer)
					} else {
						expr.Args = append(expr.Args, &Expr{Kind: ExprUndefined, Type: p.Type, Span: expr.Span})
					}
				}
				expr.Args = append(expr.Args, &Expr{
					Kind:     ExprArray,
					Elements: nil,
					Type:     restParam.Type,
					Span:     expr.Span,
				})
			}
		} else {
			for i := userArgCount + offset; i < len(target.Params); i++ {
				p := target.Params[i]
				if p.Initializer != nil {
					expr.Args = append(expr.Args, p.Initializer)
				} else if p.Optional {
					expr.Args = append(expr.Args, &Expr{Kind: ExprUndefined, Type: p.Type, Span: expr.Span})
				}
			}
		}
	}
	if calleeIdentifier == "Promise.resolve" || calleeIdentifier == "Promise.reject" || calleeIdentifier == "Promise.all" || calleeIdentifier == "Promise.race" {
		return e.extractPromiseStaticCall(node, expr, calleeIdentifier)
	}
	if calleeIdentifier == "JSON.stringify" || calleeIdentifier == "JSON.parse" {
		return e.extractJSONCall(node, expr, calleeIdentifier)
	}
	if calleeIdentifier == "Date.now" {
		expr.Kind = ExprDateNow
		return expr, nil
	}
	if isConcurrencyIntrinsic(calleeIdentifier) {
		return e.extractConcurrencyCall(node, expr, calleeIdentifier)
	}
	if expr.Kind == ExprArrayPush || expr.Kind == ExprArrayPop ||
		expr.Kind == ExprMapGet || expr.Kind == ExprMapSet || expr.Kind == ExprMapHas || expr.Kind == ExprMapDelete || expr.Kind == ExprMapClear ||
		expr.Kind == ExprSetAdd || expr.Kind == ExprSetHas || expr.Kind == ExprSetDelete || expr.Kind == ExprSetClear ||
		expr.Kind == ExprDateNow || expr.Kind == ExprDateGetTime || expr.Kind == ExprDateToISOString ||
		expr.Kind == ExprDateGetFullYear || expr.Kind == ExprDateGetMonth || expr.Kind == ExprDateGetDate ||
		expr.Kind == ExprDateGetHours || expr.Kind == ExprDateGetMinutes || expr.Kind == ExprDateGetSeconds ||
		expr.Kind == ExprRegExpNew || expr.Kind == ExprRegExpTest || expr.Kind == ExprRegExpSource {
		return expr, nil
	}
	if expr.Kind == ExprDynamicMethodCall || (expr.Kind == ExprDynamicCall && expr.Object != nil && expr.Field != "") {
		return expr, nil
	}
	if expr.CallTarget != nil {
		target := e.result.Functions[*expr.CallTarget]
		if target.Async {
			if int(expr.Type) >= len(e.result.Types) || e.result.Types[expr.Type].Kind != TypePromise || !e.compatibleTaskResult(e.result.Types[expr.Type].ReturnType, target.ReturnType) {
				return nil, fmt.Errorf("async call at %d has inconsistent Promise result type", node.Pos())
			}
			expr.Kind = ExprTaskSpawn
			expr.Captures = append([]*Expr(nil), expr.Args...)
			expr.Args = nil
			expr.Callee = nil
			return expr, nil
		}
	}
	if generic != nil {
		target, err := e.specializeGenericCall(*generic, expr.Args, expr.Type)
		if err != nil {
			return nil, err
		}
		targetCopy := target
		expr.CallTarget = &targetCopy
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
		case TypeBoolean, TypeAny, TypeUnion:
			expr.Intrinsic = IntrinsicConsoleLogJSValue
		default:
			return nil, fmt.Errorf("console.log native MVP does not support argument type %q at %d", e.result.Types[expr.Args[0].Type].Name, node.Pos())
		}
		return expr, nil
	}
	if expr.CallTarget == nil && len(expr.Dispatch) == 0 {
		if expr.Callee == nil || int(expr.Callee.Type) >= len(e.result.Types) {
			return nil, fmt.Errorf("dynamic call at %d has invalid callee type", node.Pos())
		}
		calleeKind := e.result.Types[expr.Callee.Type].Kind
		if calleeKind == TypeAny || calleeKind == TypeUnion {
			expr.Kind = ExprDynamicCall
			return expr, nil
		}
		if calleeKind != TypeFunction {
			return nil, fmt.Errorf("dynamic call at %d is not a proven native function value", node.Pos())
		}
	}
	return expr, nil
}

func (e *extractor) extractJSONCall(node tsast.Node, expr *Expr, callee string) (*Expr, error) {
	if len(expr.Args) < 1 {
		return nil, fmt.Errorf("%s requires at least one argument at %d", callee, node.Pos())
	}
	if callee == "JSON.stringify" {
		expr.Kind = ExprJSONStringify
		expr.Type = e.ensureSemanticType(TypeString, "string")
		return expr, nil
	}
	if callee == "JSON.parse" {
		expr.Kind = ExprJSONParse
		if expr.Type == 0 {
			expr.Type = e.ensureSemanticType(TypeAny, "any")
		}
		return expr, nil
	}
	return nil, fmt.Errorf("unsupported JSON API %s at %d", callee, node.Pos())
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
	info, err := e.client.GetTypeAtLocation(e.ctx, e.snapshot, e.project, node.Handle(e.currentFile()))
	if err != nil {
		return 0, err
	}
	if info == nil {
		return 0, fmt.Errorf("node %s at %d has no TypeScript type", tsast.KindName(node.Kind()), node.Pos())
	}
	if replacement, ok := e.typeSubstitutions[info.ID]; ok {
		return replacement, nil
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
	if info.Flags&typeFlagTypeParameter != 0 {
		kind = TypeParameter
	} else if info.Flags&typeFlagUnion != 0 && !(kind == TypeBoolean && text == "boolean") {
		kind = TypeUnion
	}
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
	if kind == TypeTask || kind == TypePromise {
		args, argsErr := e.client.GetTypeArguments(e.ctx, e.snapshot, e.project, info.ID)
		if argsErr == nil && len(args) == 1 {
			resultID, resultErr := e.internAPIType(&args[0])
			if resultErr != nil {
				return 0, resultErr
			}
			typ.ReturnType = resultID
		} else {
			resultText := "void"
			if strings.HasPrefix(text, "TsnativeTask<") && strings.HasSuffix(text, ">") {
				resultText = strings.TrimSpace(text[len("TsnativeTask<") : len(text)-1])
			} else if (strings.HasPrefix(text, "Promise<") || strings.HasPrefix(text, "PromiseLike<")) && strings.HasSuffix(text, ">") {
				prefix := "Promise<"
				if strings.HasPrefix(text, "PromiseLike<") {
					prefix = "PromiseLike<"
				}
				resultText = strings.TrimSpace(text[len(prefix) : len(text)-1])
			}
			switch resultText {
			case "void":
				typ.ReturnType = e.ensureSemanticType(TypeVoid, "void")
			case "number":
				typ.ReturnType = e.ensureSemanticType(TypeNumber, "number")
			case "string":
				typ.ReturnType = e.ensureSemanticType(TypeString, "string")
			case "T":
				typ.ReturnType = e.ensureSemanticType(TypeParameter, "T")
			default:
				return 0, fmt.Errorf("native task/promise result type %q could not be resolved from TypeScript type arguments", resultText)
			}
		}
	}
	if kind == TypeChannel {
		args, argsErr := e.client.GetTypeArguments(e.ctx, e.snapshot, e.project, info.ID)
		if argsErr != nil || len(args) != 1 {
			return 0, fmt.Errorf("native channel type %q requires one resolved type argument", text)
		}
		elementID, elementErr := e.internAPIType(&args[0])
		if elementErr != nil {
			return 0, elementErr
		}
		switch e.result.Types[elementID].Kind {
		case TypeNumber, TypeBoolean, TypeString, TypeObject, TypeArray, TypeFunction, TypeAny, TypeUnion, TypeNull, TypeUndefined, TypeParameter:
		default:
			return 0, fmt.Errorf("native channel element type %q is not supported yet", e.result.Types[elementID].Name)
		}
		typ.Element = elementID
	}
	if kind == TypeArray {
		args, argsErr := e.client.GetTypeArguments(e.ctx, e.snapshot, e.project, info.ID)
		if argsErr == nil && len(args) == 1 {
			elementID, elementErr := e.internAPIType(&args[0])
			if elementErr != nil {
				return 0, elementErr
			}
			switch e.result.Types[elementID].Kind {
			case TypeNumber, TypeBoolean, TypeString, TypeObject, TypeArray, TypeFunction, TypeAny, TypeUnion, TypePromise, TypeParameter, TypeNever:
				typ.Element = elementID
			default:
				return 0, fmt.Errorf("native array element type %q is not supported", e.result.Types[elementID].Name)
			}
		} else {
			base := strings.TrimSpace(strings.TrimPrefix(text, "readonly "))
			switch base {
			case "number[]":
				typ.Element = e.ensureSemanticType(TypeNumber, "number")
			case "string[]":
				typ.Element = e.ensureSemanticType(TypeString, "string")
			case "boolean[]":
				typ.Element = e.ensureSemanticType(TypeBoolean, "boolean")
			default:
				if strings.HasPrefix(base, "[") && strings.HasSuffix(base, "]") {
					typ.Element = e.ensureSemanticType(TypeAny, "any")
				} else {
					return 0, fmt.Errorf("native array type %q is not supported", text)
				}
			}
		}
	}
	if kind == TypeUnion {
		members, membersErr := e.client.GetTypesOfType(e.ctx, e.snapshot, e.project, info.ID)
		if membersErr != nil || len(members) == 0 {
			if membersErr == nil {
				membersErr = fmt.Errorf("union has no constituent types")
			}
			return 0, fmt.Errorf("resolve native union type %q: %w", text, membersErr)
		}
		for i := range members {
			memberID, memberErr := e.internAPIType(&members[i])
			if memberErr != nil {
				return 0, memberErr
			}
			typ.Members = append(typ.Members, memberID)
		}
	}
	id := TypeID(len(e.result.Types))
	typ.ID = id
	e.result.Types = append(e.result.Types, typ)
	e.types[info.ID] = id
	if kind == TypeFunction {
		signatures, err := e.client.GetSignaturesOfType(e.ctx, e.snapshot, e.project, info.ID, 0)
		if err != nil {
			return 0, err
		}
		if len(signatures) != 1 {
			return 0, fmt.Errorf("native function type %q requires exactly one call signature; got %d", text, len(signatures))
		}
		params, err := e.client.GetParametersOfSignature(e.ctx, e.snapshot, e.project, signatures[0].ID)
		if err != nil {
			return 0, err
		}
		for _, param := range params {
			paramType, err := e.client.GetTypeOfSymbol(e.ctx, e.snapshot, e.project, param.ID)
			if err != nil || paramType == nil {
				if err == nil {
					err = fmt.Errorf("function parameter %s has no type", param.Name)
				}
				return 0, err
			}
			paramID, err := e.internAPIType(paramType)
			if err != nil {
				return 0, err
			}
			e.result.Types[id].Params = append(e.result.Types[id].Params, paramID)
		}
		returnType, err := e.client.GetReturnTypeOfSignature(e.ctx, e.snapshot, e.project, signatures[0].ID)
		if err != nil || returnType == nil {
			if err == nil {
				err = fmt.Errorf("function type %q has no return type", text)
			}
			return 0, err
		}
		returnID, err := e.internAPIType(returnType)
		if err != nil {
			return 0, err
		}
		e.result.Types[id].ReturnType = returnID
	}
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
		if property.Flags&(4|8192) == 0 {
			continue
		}
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
	case "TsnativeTask":
		return TypeTask
	case "Promise", "PromiseLike":
		return TypePromise
	case "TsnativeChannel":
		return TypeChannel
	case "TsnativeTaskGroup":
		return TypeTaskGroup
	case "Map":
		return TypeMap
	case "Set":
		return TypeSet
	case "Date":
		return TypeDate
	case "RegExp":
		return TypeRegExp
	}
	if strings.HasPrefix(text, "Map<") && strings.HasSuffix(text, ">") {
		return TypeMap
	}
	if strings.HasPrefix(text, "Set<") && strings.HasSuffix(text, ">") {
		return TypeSet
	}
	if strings.HasPrefix(text, "TsnativeTask<") && strings.HasSuffix(text, ">") {
		return TypeTask
	}
	if (strings.HasPrefix(text, "Promise<") || strings.HasPrefix(text, "PromiseLike<")) && strings.HasSuffix(text, ">") {
		return TypePromise
	}
	if strings.HasPrefix(text, "TsnativeChannel<") && strings.HasSuffix(text, ">") {
		return TypeChannel
	}
	if _, err := strconv.ParseFloat(text, 64); err == nil {
		return TypeNumber
	}
	if len(text) >= 2 && ((strings.HasPrefix(text, "\"") && strings.HasSuffix(text, "\"")) || (strings.HasPrefix(text, "'") && strings.HasSuffix(text, "'"))) {
		return TypeString
	}
	arrayText := strings.TrimSpace(strings.TrimPrefix(text, "readonly "))
	if strings.HasPrefix(arrayText, "Array<") && strings.HasSuffix(arrayText, ">") {
		return TypeArray
	}
	if strings.HasPrefix(strings.TrimSpace(text), "{") && strings.HasSuffix(strings.TrimSpace(text), "}") {
		return TypeObject
	}
	if strings.Contains(text, "=>") {
		if strings.HasSuffix(arrayText, "[]") {
			elementText := strings.TrimSpace(strings.TrimSuffix(arrayText, "[]"))
			if strings.HasPrefix(elementText, "(") && strings.HasSuffix(elementText, ")") {
				return TypeArray
			}
		}
		return TypeFunction
	}
	if strings.HasSuffix(arrayText, "[]") {
		return TypeArray
	}
	if strings.HasPrefix(arrayText, "[") && strings.HasSuffix(arrayText, "]") {
		return TypeArray
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

func (e *extractor) extractClassSignatures(node tsast.Node) error {
	nameNode, ok := node.NamedChild("name")
	if !ok || nameNode.Kind() != tsast.KindIdentifier {
		return fmt.Errorf("native class at %d requires an identifier name", node.Pos())
	}
	name, _ := nameNode.Text()
	symbol, err := e.client.GetSymbolAtLocation(e.ctx, e.snapshot, e.project, nameNode.Handle(e.currentFile()))
	if err != nil || symbol == nil {
		if err == nil {
			err = fmt.Errorf("class %s has no TypeScript symbol", name)
		}
		return err
	}
	declared, err := e.client.GetDeclaredTypeOfSymbol(e.ctx, e.snapshot, e.project, symbol.ID)
	if err != nil || declared == nil {
		if err == nil {
			err = fmt.Errorf("class %s has no declared instance type", name)
		}
		return err
	}
	typeID, err := e.internAPIType(declared)
	if err != nil {
		return err
	}
	if int(typeID) >= len(e.result.Types) || e.result.Types[typeID].Kind != TypeObject {
		return fmt.Errorf("class %s does not have a closed object type", name)
	}
	shapeID := e.result.Types[typeID].Shape
	e.result.Shapes[shapeID].ClassTag = uint32(shapeID) + 1
	info := &classInfo{Node: node, Name: name, Type: typeID, Shape: shapeID, Methods: map[string]FunctionID{}}
	e.classes[symbol.ID] = info
	e.classesByType[typeID] = info
	if err := e.resolveBaseClass(node, info); err != nil {
		return err
	}
	info.FieldParam = make([]int, len(e.result.Shapes[shapeID].Fields))
	for i := range info.FieldParam {
		info.FieldParam[i] = -1
	}
	classSymbol := e.internSymbol(symbol, SymbolClass, nameNode)
	e.result.Symbols[classSymbol].Type = typeID
	return e.extractClassMembers(node, info)
}

func (e *extractor) extractClassMembers(node tsast.Node, info *classInfo) error {
	members, ok := node.NamedChild("members")
	if !ok || !members.IsList() {
		return e.finalizeConstructorSignature(info)
	}
	for _, member := range members.ListElements() {
		switch member.Kind() {
		case tsast.KindConstructor:
			if info.HasConstructor {
				return fmt.Errorf("class %s has multiple constructors", info.Name)
			}
			if err := e.extractConstructor(member, info); err != nil {
				return err
			}
		case tsast.KindMethodDeclaration:
			if hasModifier(member, tsast.KindStaticKeyword) {
				return fmt.Errorf("static method in class %s is not supported yet", info.Name)
			}
			if err := e.extractMethodSignature(member, info); err != nil {
				return err
			}
		case tsast.KindPropertyDeclaration:
			if err := e.validateClassProperty(member, info); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported class member %s in %s", tsast.KindName(member.Kind()), info.Name)
		}
	}
	return e.finalizeConstructorSignature(info)
}

func (e *extractor) extractConstructor(node tsast.Node, info *classInfo) error {
	info.HasConstructor = true
	info.ConstructorNode = node
	params, _ := node.NamedChild("parameters")
	if params.IsList() {
		for index, paramNode := range params.ListElements() {
			param, err := e.extractParameter(paramNode)
			if err != nil {
				return err
			}
			info.ConstructorParams = append(info.ConstructorParams, param)
			if !isParameterProperty(paramNode) {
				continue
			}
			for fieldIndex, field := range e.result.Shapes[info.Shape].Fields {
				if field.Name == param.Name {
					info.FieldParam[fieldIndex] = index
					break
				}
			}
		}
	}
	if _, ok := node.NamedChild("body"); !ok {
		return fmt.Errorf("constructor for %s has no body", info.Name)
	}
	return nil
}

func (e *extractor) extractMethodSignature(node tsast.Node, info *classInfo) error {
	nameNode, ok := node.NamedChild("name")
	if !ok || nameNode.Kind() != tsast.KindIdentifier {
		return fmt.Errorf("method in %s requires an identifier name", info.Name)
	}
	name, _ := nameNode.Text()
	symbol, err := e.client.GetSymbolAtLocation(e.ctx, e.snapshot, e.project, nameNode.Handle(e.currentFile()))
	if err != nil || symbol == nil {
		if err == nil {
			err = fmt.Errorf("method %s.%s has no TypeScript symbol", info.Name, name)
		}
		return err
	}
	functionID := FunctionID(len(e.result.Functions))
	methodSymbol := e.internSymbol(symbol, SymbolMethod, nameNode)
	e.functions[symbol.ID] = functionID
	info.Methods[name] = functionID
	thisSymbol := e.newSyntheticSymbol("this", SymbolParameter, info.Type, node)
	fn := Function{ID: functionID, Symbol: methodSymbol, Name: info.Name + "." + name, Source: 0, Span: e.span(node)}
	fn.Params = append(fn.Params, Parameter{Symbol: thisSymbol, Name: "this", Type: info.Type, Span: e.span(node)})
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
		return fmt.Errorf("method %s.%s requires an explicit return type", info.Name, name)
	}
	fn.ReturnType, err = e.typeAt(returnTypeNode)
	if err != nil {
		return err
	}
	e.result.Functions = append(e.result.Functions, fn)
	thisCopy := thisSymbol
	e.pending = append(e.pending, pendingFunctionBody{Function: functionID, Node: node, This: &thisCopy, FileName: e.currentFile()})
	return nil
}

func (e *extractor) newSyntheticSymbol(name string, kind SymbolKind, typeID TypeID, node tsast.Node) SymbolID {
	id := SymbolID(len(e.result.Symbols))
	e.result.Symbols = append(e.result.Symbols, Symbol{ID: id, Name: name, Kind: kind, Type: typeID, Decl: e.span(node)})
	return id
}

func isParameterProperty(node tsast.Node) bool {
	return hasModifier(node, tsast.KindPublicKeyword) || hasModifier(node, tsast.KindPrivateKeyword) ||
		hasModifier(node, tsast.KindProtectedKeyword) || hasModifier(node, tsast.KindReadonlyKeyword)
}

func (e *extractor) extractNew(node tsast.Node, expr *Expr) (*Expr, error) {
	callee, ok := node.NamedChild("expression")
	if !ok || callee.Kind() != tsast.KindIdentifier {
		return nil, fmt.Errorf("native new at %d requires a class identifier", node.Pos())
	}
	calleeText, _ := callee.Text()
	if calleeText == "Map" {
		expr.Kind = ExprMapNew
		return expr, nil
	}
	if calleeText == "Set" {
		expr.Kind = ExprSetNew
		return expr, nil
	}
	if calleeText == "Date" {
		var args []*Expr
		if arguments, ok := node.NamedChild("arguments"); ok {
			for _, argNode := range arguments.ListElements() {
				arg, err := e.extractExpr(argNode)
				if err != nil {
					return nil, err
				}
				args = append(args, arg)
			}
		}
		expr.Kind = ExprDateNew
		expr.Args = args
		return expr, nil
	}
	if calleeText == "RegExp" {
		var args []*Expr
		if arguments, ok := node.NamedChild("arguments"); ok {
			for _, argNode := range arguments.ListElements() {
				arg, err := e.extractExpr(argNode)
				if err != nil {
					return nil, err
				}
				args = append(args, arg)
			}
		}
		regExpType := e.ensureSemanticType(TypeRegExp, "RegExp")
		expr.Kind = ExprRegExpNew
		expr.Args = args
		expr.Type = regExpType
		return expr, nil
	}
	symbol, err := e.client.GetSymbolAtLocation(e.ctx, e.snapshot, e.project, callee.Handle(e.currentFile()))
	if err != nil || symbol == nil {
		if err == nil {
			err = fmt.Errorf("new target at %d has no TypeScript symbol", node.Pos())
		}
		return nil, err
	}
	class, ok := e.classes[symbol.ID]
	if !ok {
		if cls, ok := e.importedClasses[calleeText]; ok {
			class = cls
		} else {
			return nil, fmt.Errorf("new target %s is not a native class", symbol.Name)
		}
	}
	var args []*Expr
	if arguments, ok := node.NamedChild("arguments"); ok {
		for _, argNode := range arguments.ListElements() {
			arg, err := e.extractExpr(argNode)
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
		}
	}
	if len(args) != len(class.ConstructorParams) {
		return nil, fmt.Errorf("constructor %s expects %d arguments; got %d", class.Name, len(class.ConstructorParams), len(args))
	}
	constructor := class.Constructor
	expr.Kind, expr.Type, expr.Args, expr.Constructor = ExprNewClass, class.Type, args, &constructor
	expr.ConcreteType, expr.ConcreteKnown = class.Type, true
	return expr, nil
}

func (e *extractor) validateClassProperty(node tsast.Node, info *classInfo) error {
	if hasModifier(node, tsast.KindStaticKeyword) {
		return fmt.Errorf("static property in class %s is not supported yet", info.Name)
	}
	nameNode, ok := node.NamedChild("name")
	if !ok || nameNode.Kind() != tsast.KindIdentifier {
		return fmt.Errorf("class %s property at %d requires an identifier name", info.Name, node.Pos())
	}
	name, _ := nameNode.Text()
	for fieldIndex, field := range e.result.Shapes[info.Shape].Fields {
		if field.Name == name {
			if initializer, ok := node.NamedChild("initializer"); ok {
				info.Initializers = append(info.Initializers, classFieldInitializer{Field: fieldIndex, Node: initializer})
			}
			return nil
		}
	}
	return fmt.Errorf("class %s property %s is missing from checker-derived shape", info.Name, name)
}
