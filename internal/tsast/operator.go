package tsast

// UnaryOperatorKind decodes the compact TS7 binary-AST operator field.
func (n Node) UnaryOperatorKind() (uint32, bool) {
	switch n.Kind() {
	case KindPostfixUnaryExpression:
		if (n.Data()>>24)&1 != 0 {
			return KindMinusMinusToken, true
		}
		return KindPlusPlusToken, true
	case KindPrefixUnaryExpression:
		idx := (n.Data() >> 24) & 7
		switch idx {
		case 4:
			return KindPlusPlusToken, true
		case 5:
			return KindMinusMinusToken, true
		}
	}
	return 0, false
}
