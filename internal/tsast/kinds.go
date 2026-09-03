package tsast

const (
	KindEndOfFile                    uint32 = 1
	KindNumericLiteral               uint32 = 8
	KindStringLiteral                uint32 = 10
	KindRegularExpressionLiteral     uint32 = 13
	KindNoSubstitutionTemplateLiteral uint32 = 14
	KindTemplateHead                  uint32 = 15
	KindTemplateMiddle                uint32 = 16
	KindTemplateTail                  uint32 = 17
	KindLessThanToken                uint32 = 29
	KindGreaterThanToken             uint32 = 31
	KindLessThanEqualsToken          uint32 = 32
	KindGreaterThanEqualsToken       uint32 = 33
	KindEqualsEqualsToken            uint32 = 34
	KindExclamationEqualsToken       uint32 = 35
	KindEqualsEqualsEqualsToken      uint32 = 36
	KindExclamationEqualsEqualsToken uint32 = 37
	KindDotDotDotToken               uint32 = 25
	KindPlusToken                    uint32 = 39
	KindMinusToken                   uint32 = 40
	KindAsteriskToken                uint32 = 41
	KindSlashToken                   uint32 = 43
	KindPlusPlusToken                uint32 = 45
	KindMinusMinusToken              uint32 = 46
	KindAmpersandAmpersandToken      uint32 = 55
	KindBarBarToken                  uint32 = 56
	KindQuestionToken                uint32 = 57
	KindQuestionQuestionToken        uint32 = 60
	KindEqualsToken                  uint32 = 63
	KindExportKeyword                uint32 = 94
	KindExtendsKeyword               uint32 = 95
	KindFalseKeyword                 uint32 = 96
	KindNullKeyword                  uint32 = 105
	KindSuperKeyword                 uint32 = 107
	KindThisKeyword                  uint32 = 109
	KindTrueKeyword                  uint32 = 111
	KindPrivateKeyword               uint32 = 122
	KindProtectedKeyword             uint32 = 123
	KindPublicKeyword                uint32 = 124
	KindStaticKeyword                uint32 = 125
	KindAsyncKeyword                 uint32 = 133
	KindReadonlyKeyword              uint32 = 148
	KindNumberKeyword                uint32 = 150
	KindParameter                    uint32 = 170
	KindPropertyDeclaration          uint32 = 173
	KindMethodDeclaration            uint32 = 175
	KindConstructor                  uint32 = 177
	KindObjectBindingPattern         uint32 = 207
	KindArrayBindingPattern          uint32 = 208
	KindBindingElement               uint32 = 209
	KindArrayLiteralExpression       uint32 = 210
	KindObjectLiteralExpression      uint32 = 211
	KindPropertyAccessExpression     uint32 = 212
	KindElementAccessExpression      uint32 = 213
	KindCallExpression               uint32 = 214
	KindParenthesizedExpression      uint32 = 218
	KindAwaitExpression              uint32 = 224
	KindFunctionExpression           uint32 = 219
	KindArrowFunction                uint32 = 220
	KindNewExpression                uint32 = 215
	KindPrefixUnaryExpression        uint32 = 225
	KindPostfixUnaryExpression       uint32 = 226
	KindConditionalExpression        uint32 = 228
	KindTemplateExpression           uint32 = 229
	KindSpreadElement                uint32 = 231
	KindExpressionWithTypeArguments  uint32 = 234
	KindNonNullExpression            uint32 = 236
	KindBinaryExpression             uint32 = 227
	KindTemplateSpan                 uint32 = 240
	KindBlock                        uint32 = 242
	KindVariableStatement            uint32 = 244
	KindExpressionStatement          uint32 = 245
	KindIfStatement                  uint32 = 246
	KindDoStatement                  uint32 = 247
	KindWhileStatement               uint32 = 248
	KindForStatement                 uint32 = 249
	KindForOfStatement               uint32 = 251
	KindBreakStatement               uint32 = 253
	KindReturnStatement              uint32 = 254
	KindSwitchStatement              uint32 = 256
	KindThrowStatement               uint32 = 258
	KindTryStatement                 uint32 = 259
	KindVariableDeclaration          uint32 = 261
	KindVariableDeclarationList      uint32 = 262
	KindFunctionDeclaration          uint32 = 263
	KindClassDeclaration             uint32 = 264
	KindInterfaceDeclaration         uint32 = 265
	KindTypeAliasDeclaration         uint32 = 266
	KindEnumDeclaration              uint32 = 267
	KindCaseBlock                    uint32 = 270
	KindImportDeclaration            uint32 = 273
	KindImportClause                 uint32 = 274
	KindNamespaceImport              uint32 = 275
	KindNamedImports                 uint32 = 276
	KindImportSpecifier              uint32 = 277
	KindExportAssignment             uint32 = 278
	KindExportDeclaration            uint32 = 279
	KindNamedExports                 uint32 = 280
	KindExportSpecifier              uint32 = 281
	KindCaseClause                   uint32 = 297
	KindDefaultClause                uint32 = 298
	KindHeritageClause               uint32 = 299
	KindCatchClause                  uint32 = 300
	KindPropertyAssignment           uint32 = 303
	KindShorthandPropertyAssignment  uint32 = 304
	KindSpreadAssignment             uint32 = 305
	KindEnumMember                   uint32 = 306
	KindSourceFile                   uint32 = 307
	KindIdentifier                   uint32 = 79
)

var kindNames = map[uint32]string{
	KindEndOfFile: "EndOfFile", KindNumericLiteral: "NumericLiteral", KindStringLiteral: "StringLiteral", KindRegularExpressionLiteral: "RegularExpressionLiteral",
	KindLessThanToken: "LessThanToken", KindGreaterThanToken: "GreaterThanToken", KindLessThanEqualsToken: "LessThanEqualsToken",
	KindGreaterThanEqualsToken: "GreaterThanEqualsToken", KindEqualsEqualsToken: "EqualsEqualsToken", KindExclamationEqualsToken: "ExclamationEqualsToken",
	KindEqualsEqualsEqualsToken: "EqualsEqualsEqualsToken", KindExclamationEqualsEqualsToken: "ExclamationEqualsEqualsToken",
	KindPlusToken: "PlusToken", KindMinusToken: "MinusToken", KindAsteriskToken: "AsteriskToken", KindSlashToken: "SlashToken",
	KindPlusPlusToken: "PlusPlusToken", KindMinusMinusToken: "MinusMinusToken", KindEqualsToken: "EqualsToken",
	KindAmpersandAmpersandToken: "AmpersandAmpersandToken", KindBarBarToken: "BarBarToken", KindQuestionToken: "QuestionToken", KindQuestionQuestionToken: "QuestionQuestionToken",
	KindExportKeyword: "ExportKeyword", KindExtendsKeyword: "ExtendsKeyword", KindFalseKeyword: "FalseKeyword", KindNullKeyword: "NullKeyword", KindSuperKeyword: "SuperKeyword", KindThisKeyword: "ThisKeyword", KindTrueKeyword: "TrueKeyword",
	KindAsyncKeyword: "AsyncKeyword", KindPrivateKeyword: "PrivateKeyword", KindProtectedKeyword: "ProtectedKeyword", KindPublicKeyword: "PublicKeyword", KindStaticKeyword: "StaticKeyword", KindReadonlyKeyword: "ReadonlyKeyword",
	KindIdentifier: "Identifier", KindNumberKeyword: "NumberKeyword",
	KindParameter: "Parameter", KindPropertyDeclaration: "PropertyDeclaration", KindMethodDeclaration: "MethodDeclaration", KindConstructor: "Constructor",
	KindObjectBindingPattern: "ObjectBindingPattern", KindArrayBindingPattern: "ArrayBindingPattern", KindBindingElement: "BindingElement",
	KindArrayLiteralExpression: "ArrayLiteralExpression", KindObjectLiteralExpression: "ObjectLiteralExpression", KindPropertyAccessExpression: "PropertyAccessExpression",
	KindElementAccessExpression: "ElementAccessExpression", KindCallExpression: "CallExpression",
	KindParenthesizedExpression: "ParenthesizedExpression", KindAwaitExpression: "AwaitExpression", KindFunctionExpression: "FunctionExpression", KindArrowFunction: "ArrowFunction", KindNewExpression: "NewExpression",
	KindExpressionWithTypeArguments: "ExpressionWithTypeArguments", KindPrefixUnaryExpression: "PrefixUnaryExpression", KindPostfixUnaryExpression: "PostfixUnaryExpression", KindNonNullExpression: "NonNullExpression",
	KindNoSubstitutionTemplateLiteral: "NoSubstitutionTemplateLiteral", KindTemplateHead: "TemplateHead", KindTemplateMiddle: "TemplateMiddle", KindTemplateTail: "TemplateTail",
	KindBinaryExpression: "BinaryExpression", KindConditionalExpression: "ConditionalExpression", KindTemplateExpression: "TemplateExpression", KindTemplateSpan: "TemplateSpan", KindBlock: "Block", KindVariableStatement: "VariableStatement", KindExpressionStatement: "ExpressionStatement",
	KindIfStatement: "IfStatement", KindDoStatement: "DoStatement", KindWhileStatement: "WhileStatement", KindForStatement: "ForStatement", KindForOfStatement: "ForOfStatement", KindBreakStatement: "BreakStatement", KindReturnStatement: "ReturnStatement", KindSwitchStatement: "SwitchStatement", KindThrowStatement: "ThrowStatement", KindTryStatement: "TryStatement",
	KindVariableDeclaration: "VariableDeclaration", KindVariableDeclarationList: "VariableDeclarationList", KindFunctionDeclaration: "FunctionDeclaration", KindClassDeclaration: "ClassDeclaration",
	KindInterfaceDeclaration: "InterfaceDeclaration", KindTypeAliasDeclaration: "TypeAliasDeclaration", KindEnumDeclaration: "EnumDeclaration",
	KindCaseBlock: "CaseBlock", KindCaseClause: "CaseClause", KindDefaultClause: "DefaultClause",
	KindImportDeclaration: "ImportDeclaration", KindImportClause: "ImportClause", KindNamespaceImport: "NamespaceImport",
	KindNamedImports: "NamedImports", KindImportSpecifier: "ImportSpecifier", KindExportAssignment: "ExportAssignment",
	KindExportDeclaration: "ExportDeclaration", KindNamedExports: "NamedExports", KindExportSpecifier: "ExportSpecifier",
	KindHeritageClause: "HeritageClause", KindCatchClause: "CatchClause", KindPropertyAssignment: "PropertyAssignment", KindShorthandPropertyAssignment: "ShorthandPropertyAssignment",
	KindSpreadAssignment: "SpreadAssignment", KindSpreadElement: "SpreadElement", KindDotDotDotToken: "DotDotDotToken",
	KindEnumMember: "EnumMember", KindSourceFile: "SourceFile", KindNodeList: "NodeList",
}

func KindName(kind uint32) string {
	if name, ok := kindNames[kind]; ok {
		return name
	}
	return "Unknown"
}
