export const TypeKind = {
  TypeInvalid: 0,
  TypeVoid: 1,
  TypeBoolean: 2,
  TypeNumber: 3,
  TypeString: 4,
  TypeArray: 5,
  TypeObject: 6,
  TypeFunction: 7,
  TypeTask: 8,
  TypeTaskGroup: 9,
  TypeChannel: 10,
  TypePromise: 11,
  TypeAny: 12,
} as const;
export type TypeKind = (typeof TypeKind)[keyof typeof TypeKind];

export interface SemanticType {
  id: number;
  kind: TypeKind;
  name: string;
  element?: number;
  shape?: number;
  returnType?: number;
  paramTypes?: number[];
  optional?: boolean;
}

export interface ShapeField {
  name: string;
  type: number;
  offset: number;
  readonly: boolean;
}

export interface Shape {
  id: number;
  name: string;
  classTag: number;
  fields: ShapeField[];
}

export interface Param {
  name: string;
  type: number;
}

export const BinaryOp = {
  Add: "Add",
  Sub: "Sub",
  Mul: "Mul",
  Div: "Div",
  Mod: "Mod",
  Equal: "Equal",
  NotEqual: "NotEqual",
  StrictEqual: "StrictEqual",
  StrictNotEqual: "StrictNotEqual",
  LessThan: "LessThan",
  LessThanOrEqual: "LessThanOrEqual",
  GreaterThan: "GreaterThan",
  GreaterThanOrEqual: "GreaterThanOrEqual",
  BitwiseAnd: "BitwiseAnd",
  BitwiseOr: "BitwiseOr",
  BitwiseXor: "BitwiseXor",
  ShiftLeft: "ShiftLeft",
  ShiftRight: "ShiftRight",
  UnsignedShiftRight: "UnsignedShiftRight",
} as const;
export type BinaryOp = (typeof BinaryOp)[keyof typeof BinaryOp];

export const UnaryOp = {
  Negate: "Negate",
  Not: "Not",
  BitwiseNot: "BitwiseNot",
} as const;
export type UnaryOp = (typeof UnaryOp)[keyof typeof UnaryOp];

export interface SemanticFunction {
  id: number;
  name: string;
  params: Param[];
  returnType: number;
  body: SemanticBlock;
}

export interface SemanticBlock {
  statements: SemanticStmt[];
}

export type SemanticStmt =
  | { kind: "Expr"; expr: SemanticExpr }
  | { kind: "Return"; value?: SemanticExpr }
  | { kind: "Let"; name: string; type: number; init: SemanticExpr }
  | { kind: "Assign"; target: SemanticExpr; value: SemanticExpr }
  | { kind: "If"; condition: SemanticExpr; thenBranch: SemanticBlock; elseBranch?: SemanticBlock }
  | { kind: "While"; condition: SemanticExpr; body: SemanticBlock }
  | { kind: "For"; init?: SemanticStmt; condition?: SemanticExpr; post?: SemanticStmt; body: SemanticBlock };

export type SemanticExpr =
  | { kind: "ConstNumber"; value: number; type: number }
  | { kind: "ConstString"; value: string; type: number }
  | { kind: "ConstBoolean"; value: boolean; type: number }
  | { kind: "Variable"; name: string; type: number }
  | { kind: "Binary"; op: BinaryOp; left: SemanticExpr; right: SemanticExpr; type: number }
  | { kind: "Unary"; op: UnaryOp; operand: SemanticExpr; type: number }
  | { kind: "Call"; callee: string; args: SemanticExpr[]; type: number }
  | { kind: "MethodCall"; object: SemanticExpr; method: string; args: SemanticExpr[]; type: number }
  | { kind: "FieldGet"; object: SemanticExpr; field: string; type: number }
  | { kind: "FieldSet"; object: SemanticExpr; field: string; value: SemanticExpr; type: number }
  | { kind: "ArrayNew"; elements: SemanticExpr[]; type: number }
  | { kind: "ArrayGet"; array: SemanticExpr; index: SemanticExpr; type: number }
  | { kind: "ArraySet"; array: SemanticExpr; index: SemanticExpr; value: SemanticExpr; type: number };

export interface SemanticModule {
  name: string;
  types: SemanticType[];
  shapes: Shape[];
  functions: SemanticFunction[];
}
