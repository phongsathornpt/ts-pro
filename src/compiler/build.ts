import { emitLLVM } from "../codegen/llvm.ts";
import type { Module } from "../mir/mir.ts";

export interface BuildOptions {
  input: string;
  output?: string;
  optimization?: string;
  pureGo?: boolean;
  thinLTO?: boolean;
  pgoProfile?: string;
  pgoGenerate?: string;
  target?: string;
}

export interface BuildResult {
  output: string;
  llvmIR?: string;
}

export class CompilerDriver {
  private options: BuildOptions;

  constructor(options: BuildOptions) {
    this.options = options;
  }

  public compileMIRToLLVM(module: Module): string {
    return emitLLVM(module);
  }
}
