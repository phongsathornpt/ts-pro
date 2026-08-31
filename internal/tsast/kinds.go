package tsast

const (
	KindEndOfFile                uint32 = 1
	KindNumericLiteral           uint32 = 8
	KindLessThanEqualsToken      uint32 = 32
	KindPlusToken                uint32 = 39
	KindMinusToken               uint32 = 40
	KindExportKeyword            uint32 = 94
	KindNumberKeyword            uint32 = 150
	KindParameter                uint32 = 170
	KindPropertyAccessExpression uint32 = 212
	KindCallExpression           uint32 = 214
	KindBinaryExpression         uint32 = 227
	KindBlock                    uint32 = 242
	KindExpressionStatement      uint32 = 245
	KindIfStatement              uint32 = 246
	KindReturnStatement          uint32 = 254
	KindFunctionDeclaration      uint32 = 263
	KindSourceFile               uint32 = 307
	KindIdentifier               uint32 = 79
)

var kindNames = map[uint32]string{
	KindEndOfFile: "EndOfFile", KindNumericLiteral: "NumericLiteral",
	KindLessThanEqualsToken: "LessThanEqualsToken", KindPlusToken: "PlusToken", KindMinusToken: "MinusToken",
	KindExportKeyword: "ExportKeyword", KindIdentifier: "Identifier", KindNumberKeyword: "NumberKeyword",
	KindParameter: "Parameter", KindPropertyAccessExpression: "PropertyAccessExpression", KindCallExpression: "CallExpression",
	KindBinaryExpression: "BinaryExpression", KindBlock: "Block", KindExpressionStatement: "ExpressionStatement",
	KindIfStatement: "IfStatement", KindReturnStatement: "ReturnStatement", KindFunctionDeclaration: "FunctionDeclaration",
	KindSourceFile: "SourceFile", KindNodeList: "NodeList",
}

func KindName(kind uint32) string {
	if name, ok := kindNames[kind]; ok {
		return name
	}
	return "Unknown"
}
