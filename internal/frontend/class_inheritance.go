package frontend

import (
	"fmt"

	"github.com/phongsathornpt/ts-pro/internal/tsast"
)

func (e *extractor) resolveBaseClass(node tsast.Node, info *classInfo) error {
	clauses, ok := node.NamedChild("heritageClauses")
	if !ok || !clauses.IsList() {
		return nil
	}
	for _, clause := range clauses.ListElements() {
		if clause.Kind() != tsast.KindHeritageClause {
			continue
		}
		if clause.HeritageIsImplements() {
			continue
		}
		types, ok := clause.NamedChild("types")
		if !ok || !types.IsList() || len(types.ListElements()) != 1 {
			return fmt.Errorf("class %s requires exactly one native base class", info.Name)
		}
		typeNode := types.ListElements()[0]
		expression, ok := typeNode.NamedChild("expression")
		if !ok || expression.Kind() != tsast.KindIdentifier {
			return fmt.Errorf("class %s extends a non-identifier base", info.Name)
		}
		symbol, err := e.client.GetSymbolAtLocation(e.ctx, e.snapshot, e.project, expression.Handle(e.currentFile()))
		if err != nil || symbol == nil {
			if err == nil {
				err = fmt.Errorf("base class of %s has no TypeScript symbol", info.Name)
			}
			return err
		}
		base, ok := e.classes[symbol.ID]
		if !ok {
			return fmt.Errorf("base class %s of %s must be declared earlier in the native module", symbol.Name, info.Name)
		}
		if info.Base != nil {
			return fmt.Errorf("class %s has multiple native base classes", info.Name)
		}
		info.Base = base
		e.relayoutDerivedShape(info, base)
	}
	return nil
}

func (e *extractor) relayoutDerivedShape(info, base *classInfo) {
	shape := &e.result.Shapes[info.Shape]
	baseFields := e.result.Shapes[base.Shape].Fields
	seen := make(map[string]struct{}, len(baseFields))
	fields := append([]ShapeField(nil), baseFields...)
	for _, field := range baseFields {
		seen[field.Name] = struct{}{}
	}
	for _, field := range shape.Fields {
		if _, inherited := seen[field.Name]; inherited {
			continue
		}
		fields = append(fields, field)
	}
	shape.Fields = fields
}

func isSuperCallStatement(node tsast.Node) (tsast.Node, bool) {
	if node.Kind() != tsast.KindExpressionStatement {
		return tsast.Node{}, false
	}
	expr, ok := node.NamedChild("expression")
	if !ok || expr.Kind() != tsast.KindCallExpression {
		return tsast.Node{}, false
	}
	callee, ok := expr.NamedChild("expression")
	if !ok || callee.Kind() != tsast.KindSuperKeyword {
		return tsast.Node{}, false
	}
	return expr, true
}

func (e *extractor) extractSuperConstructorStatement(info *classInfo, statements []tsast.Node) (Statement, []tsast.Node, error) {
	if info.Base == nil {
		return Statement{}, statements, fmt.Errorf("class %s has no native base constructor", info.Name)
	}
	var call tsast.Node
	if info.HasConstructor {
		if len(statements) == 0 {
			return Statement{}, nil, fmt.Errorf("derived constructor %s requires super(...) as its first statement", info.Name)
		}
		var ok bool
		call, ok = isSuperCallStatement(statements[0])
		if !ok {
			return Statement{}, nil, fmt.Errorf("derived constructor %s requires super(...) as its first statement", info.Name)
		}
		statements = statements[1:]
	} else if len(info.Base.ConstructorParams) != 0 {
		return Statement{}, nil, fmt.Errorf("implicit constructor for %s cannot satisfy base constructor arguments", info.Name)
	}

	voidType := e.ensureSemanticType(TypeVoid, "void")
	thisExpr := &Expr{Kind: ExprIdentifier, Type: info.Type, Symbol: info.ConstructorThis, Name: "this", Span: e.span(info.Node)}
	args := []*Expr{thisExpr}
	if info.HasConstructor {
		if arguments, ok := call.NamedChild("arguments"); ok {
			for _, node := range arguments.ListElements() {
				arg, err := e.extractExpr(node)
				if err != nil {
					return Statement{}, nil, err
				}
				args = append(args, arg)
			}
		}
	}
	if len(args)-1 != len(info.Base.ConstructorParams) {
		return Statement{}, nil, fmt.Errorf("super constructor for %s expects %d arguments; got %d", info.Name, len(info.Base.ConstructorParams), len(args)-1)
	}
	target := info.Base.Constructor
	expr := &Expr{
		Kind: ExprCall, Type: voidType, Args: args,
		CallTarget: &target, Span: e.span(info.Node),
	}
	return Statement{Kind: StmtExpr, Span: e.span(info.Node), Expr: expr}, statements, nil
}
