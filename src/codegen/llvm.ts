import { Repr } from "../mir/mir.ts";
import type { Module, Function, Instruction, Operation, Terminator } from "../mir/mir.ts";
import { BinaryOp, UnaryOp } from "../frontend/model.ts";

export function emitLLVM(module: Module): string {
  const out: string[] = [];

  // Header & Target declarations
  out.push('; ModuleID = "' + module.name + '"');
  out.push('source_filename = "' + module.name + '"');
  out.push("");

  // Common declarations
  out.push("declare void @tsnative_console_log_f64(double)");
  out.push("declare void @tsnative_console_log_string(ptr)");
  out.push("declare void @tsnative_console_log_bool(i1)");
  out.push("declare void @tsnative_console_log_ref(ptr)");
  out.push("declare ptr @tsnative_string_new(ptr, i64)");
  out.push("declare void @tsnative_task_yield()");
  out.push("");

  // Functions
  for (const fn of module.functions) {
    emitFunction(fn, out);
    out.push("");
  }

  return out.join("\n");
}

function emitFunction(fn: Function, out: string[]): void {
  const retType = llvmType(fn.returnRepr);
  const paramDecls = fn.params.map(p => `${llvmType(p.repr)} %v${p.value}`).join(", ");
  out.push(`define ${retType} @${fn.name}(${paramDecls}) {`);

  for (const block of fn.blocks) {
    out.push(`b${block.id}:`);
    for (const inst of block.instructions) {
      emitInstruction(inst, out);
    }
    emitTerminator(block.terminator, out);
  }

  out.push("}");
}

function emitInstruction(inst: Instruction, out: string[]): void {
  const res = `%v${inst.result}`;
  const repr = inst.repr;
  const op: Operation = inst.op;

  switch (op.kind) {
    case "ConstF64":
      out.push(`  ${res} = fadd double 0.0, ${formatF64(op.value)}`);
      break;
    case "ConstBool":
      out.push(`  ${res} = ${op.value ? "true" : "false"}`);
      break;
    case "ConstString": {
      out.push(`  ; string literal ${JSON.stringify(op.value)}`);
      out.push(`  ${res} = call ptr @tsnative_string_new(ptr null, i64 ${op.value.length})`);
      break;
    }
    case "Binary":
      emitBinary(res, repr, op.op, op.left, op.right, out);
      break;
    case "Unary":
      emitUnary(res, repr, op.op, op.operand, out);
      break;
    case "Call": {
      const args = op.args.map(a => `double %v${a}`).join(", ");
      out.push(`  ${res} = call ${llvmType(repr)} @f${op.callee}(${args})`);
      break;
    }
    case "Intrinsic":
      if (op.name === "console.log") {
        if (op.args.length > 0) {
          out.push(`  call void @tsnative_console_log_f64(double %v${op.args[0]})`);
        }
      }
      break;
    case "Phi": {
      const incoming = op.incoming.map(inc => `[ %v${inc.value}, %b${inc.block} ]`).join(", ");
      out.push(`  ${res} = phi ${llvmType(repr)} ${incoming}`);
      break;
    }
    default: {
      const unhandledOp: { readonly kind: string } = op;
      out.push(`  ; op ${unhandledOp.kind}`);
      break;
    }
  }
}

function emitBinary(res: string, _repr: Repr, op: BinaryOp, left: number, right: number, out: string[]): void {
  const l = `%v${left}`;
  const r = `%v${right}`;
  switch (op) {
    case BinaryOp.Add:
      out.push(`  ${res} = fadd double ${l}, ${r}`);
      break;
    case BinaryOp.Sub:
      out.push(`  ${res} = fsub double ${l}, ${r}`);
      break;
    case BinaryOp.Mul:
      out.push(`  ${res} = fmul double ${l}, ${r}`);
      break;
    case BinaryOp.Div:
      out.push(`  ${res} = fdiv double ${l}, ${r}`);
      break;
    case BinaryOp.Equal:
    case BinaryOp.StrictEqual:
      out.push(`  ${res} = fcmp oeq double ${l}, ${r}`);
      break;
    case BinaryOp.NotEqual:
    case BinaryOp.StrictNotEqual:
      out.push(`  ${res} = fcmp one double ${l}, ${r}`);
      break;
    case BinaryOp.LessThan:
      out.push(`  ${res} = fcmp olt double ${l}, ${r}`);
      break;
    case BinaryOp.LessThanOrEqual:
      out.push(`  ${res} = fcmp ole double ${l}, ${r}`);
      break;
    case BinaryOp.GreaterThan:
      out.push(`  ${res} = fcmp ogt double ${l}, ${r}`);
      break;
    case BinaryOp.GreaterThanOrEqual:
      out.push(`  ${res} = fcmp oge double ${l}, ${r}`);
      break;
    default:
      out.push(`  ${res} = fadd double ${l}, ${r}`);
      break;
  }
}

function emitUnary(res: string, _repr: Repr, op: UnaryOp, operand: number, out: string[]): void {
  const v = `%v${operand}`;
  switch (op) {
    case UnaryOp.Negate:
      out.push(`  ${res} = fneg double ${v}`);
      break;
    case UnaryOp.Not:
      out.push(`  ${res} = xor i1 ${v}, true`);
      break;
    default:
      break;
  }
}

function emitTerminator(term: Terminator, out: string[]): void {
  switch (term.kind) {
    case "Return":
      if (term.value !== undefined) {
        out.push(`  ret double %v${term.value}`);
      } else {
        out.push("  ret void");
      }
      break;
    case "Jump":
      out.push(`  br label %b${term.target}`);
      break;
    case "Branch":
      out.push(`  br i1 %v${term.condition}, label %b${term.thenTarget}, label %b${term.elseTarget}`);
      break;
  }
}

function llvmType(repr: Repr): string {
  switch (repr) {
    case Repr.Void:
      return "void";
    case Repr.Bool:
      return "i1";
    case Repr.F64:
      return "double";
    case Repr.StringRef:
    case Repr.ArrayRef:
    case Repr.ObjectRef:
    case Repr.FunctionRef:
    case Repr.TaskRef:
    case Repr.TaskGroupRef:
    case Repr.ChannelRef:
    case Repr.JSValue:
      return "ptr";
    default:
      return "ptr";
  }
}

function formatF64(v: number): string {
  if (Number.isInteger(v)) {
    return v.toFixed(1);
  }
  return v.toString();
}
