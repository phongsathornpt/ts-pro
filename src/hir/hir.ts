import { BinaryOp, UnaryOp } from "../frontend/model.ts";

export type FunctionID = number;
export type BlockID = number;
export type ValueID = number;
export type TypeID = number;
export type ShapeID = number;

export interface Param {
  value: ValueID;
  type: TypeID;
  name: string;
}

export type Operation =
  | { kind: "ConstNumber"; value: number }
  | { kind: "ConstString"; value: string }
  | { kind: "ConstBoolean"; value: boolean }
  | { kind: "Binary"; op: BinaryOp; left: ValueID; right: ValueID }
  | { kind: "Unary"; op: UnaryOp; operand: ValueID }
  | { kind: "Call"; callee: FunctionID; args: ValueID[] }
  | { kind: "Intrinsic"; name: string; args: ValueID[] }
  | { kind: "Alloc"; shape: ShapeID }
  | { kind: "FieldGet"; object: ValueID; shape: ShapeID; field: number }
  | { kind: "FieldSet"; object: ValueID; shape: ShapeID; field: number; value: ValueID }
  | { kind: "ArrayNew"; elements: ValueID[] }
  | { kind: "ArrayGet"; array: ValueID; index: ValueID }
  | { kind: "ArraySet"; array: ValueID; index: ValueID; value: ValueID }
  | { kind: "Phi"; incoming: { block: BlockID; value: ValueID }[] };

export type Terminator =
  | { kind: "Return"; value?: ValueID }
  | { kind: "Jump"; target: BlockID }
  | { kind: "Branch"; condition: ValueID; thenTarget: BlockID; elseTarget: BlockID };

export interface Instruction {
  result: ValueID;
  type: TypeID;
  op: Operation;
}

export interface Block {
  id: BlockID;
  instructions: Instruction[];
  terminator: Terminator;
}

export interface Function {
  id: FunctionID;
  name: string;
  params: Param[];
  returnType: TypeID;
  entry: BlockID;
  blocks: Block[];
}

export interface Module {
  name: string;
  functions: Function[];
}
