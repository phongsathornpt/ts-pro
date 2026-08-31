package tsast

import "math/bits"

var childProperties = map[uint32][]string{
	KindSourceFile:               {"statements", "endOfFileToken"},
	KindFunctionDeclaration:      {"modifiers", "asteriskToken", "name", "typeParameters", "parameters", "type", "body"},
	KindParameter:                {"modifiers", "dotDotDotToken", "name", "questionToken", "type", "initializer"},
	KindBlock:                    {"statements"},
	KindIfStatement:              {"expression", "thenStatement", "elseStatement"},
	KindWhileStatement:           {"expression", "statement"},
	KindForStatement:             {"initializer", "condition", "incrementor", "statement"},
	KindReturnStatement:          {"expression"},
	KindBinaryExpression:         {"modifiers", "left", "type", "operatorToken", "right"},
	KindCallExpression:           {"expression", "questionDotToken", "typeArguments", "arguments"},
	KindVariableStatement:        {"modifiers", "declarationList"},
	KindVariableDeclarationList:  {"declarations"},
	KindVariableDeclaration:      {"name", "exclamationToken", "type", "initializer"},
	KindPrefixUnaryExpression:    {"operand"},
	KindPostfixUnaryExpression:   {"operand"},
	KindExpressionStatement:      {"expression"},
	KindPropertyAccessExpression: {"expression", "questionDotToken", "name"},
}

func (n Node) RawChildren() []Node {
	first := n.index + 1
	if first >= n.file.nodeCount {
		return nil
	}
	child, ok := n.file.Node(first)
	if !ok || child.ParentIndex() != n.index {
		return nil
	}
	var result []Node
	for {
		result = append(result, child)
		next := child.Next()
		if next == 0 {
			break
		}
		child, ok = n.file.Node(next)
		if !ok {
			break
		}
	}
	return result
}

func (n Node) Children() []Node {
	var result []Node
	for _, child := range n.RawChildren() {
		if child.IsList() {
			result = append(result, child.ListElements()...)
		} else {
			result = append(result, child)
		}
	}
	return result
}

func (n Node) ListElements() []Node {
	if !n.IsList() || n.Data() == 0 {
		return nil
	}
	var result []Node
	index := n.index + 1
	for index != 0 {
		child, ok := n.file.Node(index)
		if !ok {
			break
		}
		result = append(result, child)
		index = child.Next()
	}
	return result
}

func (n Node) NamedChild(name string) (Node, bool) {
	props, ok := childProperties[n.Kind()]
	if !ok {
		return Node{}, false
	}
	order := -1
	for i, prop := range props {
		if prop == name {
			order = i
			break
		}
	}
	if order < 0 || order >= 8 {
		return Node{}, false
	}
	mask := n.ChildMask()
	bit := uint8(1 << order)
	if mask&bit == 0 {
		return Node{}, false
	}
	lowerMask := mask & (bit - 1)
	physical := bits.OnesCount8(lowerMask)
	children := n.RawChildren()
	if physical >= len(children) {
		return Node{}, false
	}
	return children[physical], true
}
