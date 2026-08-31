package tsast

const (
	KindEndOfFile                    uint32 = 1
	KindNumericLiteral               uint32 = 8
	KindLessThanToken                uint32 = 29
	KindGreaterThanToken             uint32 = 31
	KindLessThanEqualsToken          uint32 = 32
	KindGreaterThanEqualsToken       uint32 = 33
	KindEqualsEqualsToken            uint32 = 34
	KindExclamationEqualsToken       uint32 = 35
	KindEqualsEqualsEqualsToken      uint32 = 36
	KindExclamationEqualsEqualsToken uint32 = 37
	KindPlusToken                    uint32 = 39
	KindMinusToken                   uint32 = 40
	KindAsteriskToken                uint32 = 41
	KindSlashToken                   uint32 = 43
	KindPlusPlusToken                uint32 = 45
	KindMinusMinusToken              uint32 = 46
	KindEqualsToken                  uint32 = 63
	KindExportKeyword                uint32 = 94
	KindNumberKeyword                uint32 = 150
	KindParameter                    uint32 = 170
	KindPropertyAccessExpression     uint32 = 212
	KindCallExpression               uint32 = 214
	KindPrefixUnaryExpression        uint32 = 225
	KindPostfixUnaryExpression       uint32 = 226
	KindBinaryExpression             uint32 = 227
	KindBlock                        uint32 = 242
	KindVariableStatement            uint32 = 244
	KindExpressionStatement          uint32 = 245
	KindIfStatement                  uint32 = 246
	KindWhileStatement               uint32 = 248
	KindForStatement                 uint32 = 249
	KindReturnStatement              uint32 = 254
	KindVariableDeclaration          uint32 = 261
	KindVariableDeclarationList      uint32 = 262
	KindFunctionDeclaration          uint32 = 263
	KindSourceFile                   uint32 = 307
	KindIdentifier                   uint32 = 79
)

var kindNames = map[uint32]string{
	KindEndOfFile: "EndOfFile", KindNumericLiteral: "NumericLiteral",
	KindLessThanToken: "LessThanToken", KindGreaterThanToken: "GreaterThanToken", KindLessThanEqualsToken: "LessThanEqualsToken",
	KindGreaterThanEqualsToken: "GreaterThanEqualsToken", KindEqualsEqualsToken: "EqualsEqualsToken", KindExclamationEqualsToken: "ExclamationEqualsToken",
	KindEqualsEqualsEqualsToken: "EqualsEqualsEqualsToken", KindExclamationEqualsEqualsToken: "ExclamationEqualsEqualsToken",
	KindPlusToken: "PlusToken", KindMinusToken: "MinusToken", KindAsteriskToken: "AsteriskToken", KindSlashToken: "SlashToken",
	KindPlusPlusToken: "PlusPlusToken", KindMinusMinusToken: "MinusMinusToken", KindEqualsToken: "EqualsToken",
	KindExportKeyword: "ExportKeyword", KindIdentifier: "Identifier", KindNumberKeyword: "NumberKeyword",
	KindParameter: "Parameter", KindPropertyAccessExpression: "PropertyAccessExpression", KindCallExpression: "CallExpression",
	KindPrefixUnaryExpression: "PrefixUnaryExpression", KindPostfixUnaryExpression: "PostfixUnaryExpression",
	KindBinaryExpression: "BinaryExpression", KindBlock: "Block", KindVariableStatement: "VariableStatement", KindExpressionStatement: "ExpressionStatement",
	KindIfStatement: "IfStatement", KindWhileStatement: "WhileStatement", KindForStatement: "ForStatement", KindReturnStatement: "ReturnStatement",
	KindVariableDeclaration: "VariableDeclaration", KindVariableDeclarationList: "VariableDeclarationList", KindFunctionDeclaration: "FunctionDeclaration",
	KindSourceFile: "SourceFile", KindNodeList: "NodeList",
}

func KindName(kind uint32) string {
	if name, ok := kindNames[kind]; ok {
		return name
	}
	return "Unknown"
}
