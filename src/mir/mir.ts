import { BinaryOp, UnaryOp } from "../frontend/model.ts";
import type { FunctionID, BlockID, ValueID, ShapeID } from "../hir/hir.ts";

export const Repr = {
  Void: 0,
  Bool: 1,
  F64: 2,
  StringRef: 3,
  ArrayRef: 4,
  ObjectRef: 5,
  FunctionRef: 6,
  TaskRef: 7,
  TaskGroupRef: 8,
  ChannelRef: 9,
  JSValue: 10,
} as const;
export type Repr = (typeof Repr)[keyof typeof Repr];

export interface Param {
  value: ValueID;
  repr: Repr;
  name: string;
}

export type Operation =
  | { kind: "ConstF64"; value: number }
  | { kind: "ConstString"; value: string }
  | { kind: "ConstBool"; value: boolean }
  | { kind: "ConstNull" }
  | { kind: "ConstUndefined" }
  | { kind: "Binary"; op: BinaryOp; left: ValueID; right: ValueID }
  | { kind: "Unary"; op: UnaryOp; operand: ValueID }
  | { kind: "Call"; callee: FunctionID; args: ValueID[] }
  | { kind: "Intrinsic"; name: string; args: ValueID[] }
  | { kind: "ObjectAlloc"; shape: ShapeID }
  | { kind: "FieldGet"; object: ValueID; shape: ShapeID; field: number }
  | { kind: "FieldSet"; object: ValueID; shape: ShapeID; field: number; value: ValueID }
  | { kind: "ArrayAlloc"; length: ValueID }
  | { kind: "ArrayGet"; array: ValueID; index: ValueID }
  | { kind: "ArraySet"; array: ValueID; index: ValueID; value: ValueID }
  | { kind: "Phi"; incoming: { block: BlockID; value: ValueID }[] }
  | { kind: "Box"; value: ValueID; sourceRepr: Repr }
  | { kind: "Unbox"; value: ValueID; targetRepr: Repr };

export type Terminator =
  | { kind: "Return"; value?: ValueID }
  | { kind: "Jump"; target: BlockID }
  | { kind: "Branch"; condition: ValueID; thenTarget: BlockID; elseTarget: BlockID };

export interface Instruction {
  result: ValueID;
  repr: Repr;
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
  returnRepr: Repr;
  entry: BlockID;
  blocks: Block[];
}

export interface Module {
  name: string;
  functions: Function[];
}
