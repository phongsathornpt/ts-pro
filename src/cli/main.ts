import { CompilerDriver } from "../compiler/build.ts";

interface NodeJSProcess {
  readonly argv: readonly string[];
  exit(code?: number): never;
}
declare const process: NodeJSProcess;

function main() {
  const args = process.argv.slice(2);
  if (args.length === 0 || args.includes("--help") || args.includes("-h")) {
    console.log("Usage: tspro-ts <command> [options] <file.ts>");
    console.log("Commands:");
    console.log("  build    Compile TypeScript file to native executable or LLVM IR");
    console.log("  version  Show version");
    process.exit(0);
  }

  const cmd = args[0];
  if (cmd === "version" || args.includes("--version")) {
    console.log("tspro-ts 0.1.0 (TypeScript 7 frontend)");
    process.exit(0);
  }

  if (cmd === "build") {
    let input = "";
    let output = "";
    let thinLTO = false;

    for (let i = 1; i < args.length; i++) {
      const arg = args[i];
      if (arg === undefined) {
        continue;
      }
      if (arg === "--thin-lto") {
        thinLTO = true;
      } else if (arg === "-o" && i + 1 < args.length) {
        output = args[++i] ?? "";
      } else if (!arg.startsWith("-")) {
        input = arg;
      }
    }

    if (!input) {
      console.log("error: no input file specified");
      process.exit(1);
    }

    const driver = new CompilerDriver({ input, output, thinLTO });
    void driver;
    console.log(`[tspro-ts] Compiling ${input} (thinLTO: ${thinLTO})...`);
  }
}

main();
