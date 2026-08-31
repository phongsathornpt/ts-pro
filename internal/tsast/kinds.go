package tsast

const (
	KindEndOfFile                    uint32 = 1
	KindNumericLiteral               uint32 = 8
	KindStringLiteral                uint32 = 10
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
	KindSuperKeyword                 uint32 = 107
	KindThisKeyword                  uint32 = 109
	KindPrivateKeyword               uint32 = 122
	KindProtectedKeyword             uint32 = 123
	KindPublicKeyword                uint32 = 124
	KindStaticKeyword                uint32 = 125
	KindReadonlyKeyword              uint32 = 148
	KindNumberKeyword                uint32 = 150
	KindParameter                    uint32 = 170
	KindPropertyDeclaration          uint32 = 173
	KindMethodDeclaration            uint32 = 175
	KindConstructor                  uint32 = 177
	KindArrayLiteralExpression       uint32 = 210
	KindObjectLiteralExpression      uint32 = 211
	KindPropertyAccessExpression     uint32 = 212
	KindElementAccessExpression      uint32 = 213
	KindCallExpression               uint32 = 214
	KindNewExpression                uint32 = 215
	KindPrefixUnaryExpression        uint32 = 225
	KindPostfixUnaryExpression       uint32 = 226
	KindNonNullExpression            uint32 = 236
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
	KindClassDeclaration             uint32 = 264
	KindInterfaceDeclaration         uint32 = 265
	KindTypeAliasDeclaration         uint32 = 266
	KindPropertyAssignment           uint32 = 303
	KindShorthandPropertyAssignment  uint32 = 304
	KindSourceFile                   uint32 = 307
	KindIdentifier                   uint32 = 79
)

var kindNames = map[uint32]string{
	KindEndOfFile: "EndOfFile", KindNumericLiteral: "NumericLiteral", KindStringLiteral: "StringLiteral",
	KindLessThanToken: "LessThanToken", KindGreaterThanToken: "GreaterThanToken", KindLessThanEqualsToken: "LessThanEqualsToken",
	KindGreaterThanEqualsToken: "GreaterThanEqualsToken", KindEqualsEqualsToken: "EqualsEqualsToken", KindExclamationEqualsToken: "ExclamationEqualsToken",
	KindEqualsEqualsEqualsToken: "EqualsEqualsEqualsToken", KindExclamationEqualsEqualsToken: "ExclamationEqualsEqualsToken",
	KindPlusToken: "PlusToken", KindMinusToken: "MinusToken", KindAsteriskToken: "AsteriskToken", KindSlashToken: "SlashToken",
	KindPlusPlusToken: "PlusPlusToken", KindMinusMinusToken: "MinusMinusToken", KindEqualsToken: "EqualsToken",
	KindExportKeyword: "ExportKeyword", KindSuperKeyword: "SuperKeyword", KindThisKeyword: "ThisKeyword",
	KindPrivateKeyword: "PrivateKeyword", KindProtectedKeyword: "ProtectedKeyword", KindPublicKeyword: "PublicKeyword", KindStaticKeyword: "StaticKeyword", KindReadonlyKeyword: "ReadonlyKeyword",
	KindIdentifier: "Identifier", KindNumberKeyword: "NumberKeyword",
	KindParameter: "Parameter", KindPropertyDeclaration: "PropertyDeclaration", KindMethodDeclaration: "MethodDeclaration", KindConstructor: "Constructor",
	KindArrayLiteralExpression: "ArrayLiteralExpression", KindObjectLiteralExpression: "ObjectLiteralExpression", KindPropertyAccessExpression: "PropertyAccessExpression",
	KindElementAccessExpression: "ElementAccessExpression", KindCallExpression: "CallExpression", KindNewExpression: "NewExpression",
	KindPrefixUnaryExpression: "PrefixUnaryExpression", KindPostfixUnaryExpression: "PostfixUnaryExpression", KindNonNullExpression: "NonNullExpression",
	KindBinaryExpression: "BinaryExpression", KindBlock: "Block", KindVariableStatement: "VariableStatement", KindExpressionStatement: "ExpressionStatement",
	KindIfStatement: "IfStatement", KindWhileStatement: "WhileStatement", KindForStatement: "ForStatement", KindReturnStatement: "ReturnStatement",
	KindVariableDeclaration: "VariableDeclaration", KindVariableDeclarationList: "VariableDeclarationList", KindFunctionDeclaration: "FunctionDeclaration", KindClassDeclaration: "ClassDeclaration",
	KindInterfaceDeclaration: "InterfaceDeclaration", KindTypeAliasDeclaration: "TypeAliasDeclaration",
	KindPropertyAssignment: "PropertyAssignment", KindShorthandPropertyAssignment: "ShorthandPropertyAssignment",
	KindSourceFile: "SourceFile", KindNodeList: "NodeList",
}

func KindName(kind uint32) string {
	if name, ok := kindNames[kind]; ok {
		return name
	}
	return "Unknown"
}
