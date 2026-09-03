import { Repr } from "../mir/mir.ts";
import type { Module } from "../mir/mir.ts";
import { BinaryOp } from "../frontend/model.ts";
import { CompilerDriver } from "./build.ts";

function testBasicFibModule(): void {
  const module: Module = {
    name: "fib_test",
    functions: [
      {
        id: 0,
        name: "fib",
        params: [{ value: 0, repr: Repr.F64, name: "n" }],
        returnRepr: Repr.F64,
        entry: 0,
        blocks: [
          {
            id: 0,
            instructions: [
              { result: 1, repr: Repr.F64, op: { kind: "ConstF64", value: 2.0 } },
              { result: 2, repr: Repr.Bool, op: { kind: "Binary", op: BinaryOp.LessThan, left: 0, right: 1 } },
            ],
            terminator: { kind: "Branch", condition: 2, thenTarget: 1, elseTarget: 2 },
          },
          {
            id: 1,
            instructions: [],
            terminator: { kind: "Return", value: 0 },
          },
          {
            id: 2,
            instructions: [
              { result: 3, repr: Repr.F64, op: { kind: "ConstF64", value: 1.0 } },
              { result: 4, repr: Repr.F64, op: { kind: "Binary", op: BinaryOp.Sub, left: 0, right: 3 } },
              { result: 5, repr: Repr.F64, op: { kind: "Call", callee: 0, args: [4] } },
              { result: 6, repr: Repr.F64, op: { kind: "ConstF64", value: 2.0 } },
              { result: 7, repr: Repr.F64, op: { kind: "Binary", op: BinaryOp.Sub, left: 0, right: 6 } },
              { result: 8, repr: Repr.F64, op: { kind: "Call", callee: 0, args: [7] } },
              { result: 9, repr: Repr.F64, op: { kind: "Binary", op: BinaryOp.Add, left: 5, right: 8 } },
            ],
            terminator: { kind: "Return", value: 9 },
          },
        ],
      },
    ],
  };

  const driver = new CompilerDriver({ input: "fib.ts", thinLTO: true });
  const ir = driver.compileMIRToLLVM(module);

  if (!ir.includes('define double @fib(double %v0)')) {
    throw new Error("Missing fib definition in emitted LLVM IR");
  }
  if (!ir.includes('fcmp olt double %v0, %v1')) {
    throw new Error("Missing comparison in emitted LLVM IR");
  }
  if (!ir.includes('br i1 %v2, label %b1, label %b2')) {
    throw new Error("Missing branch in emitted LLVM IR");
  }
  console.log("testBasicFibModule passed!");
}

testBasicFibModule();
