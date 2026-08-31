# Native TypeScript Profile

Recommended baseline for native builds:

```json
{
  "compilerOptions": {
    "target": "esnext",
    "module": "esnext",
    "moduleResolution": "bundler",
    "strict": true,
    "noImplicitAny": true,
    "strictNullChecks": true,
    "strictPropertyInitialization": true,
    "exactOptionalPropertyTypes": true,
    "noUncheckedIndexedAccess": true,
    "noPropertyAccessFromIndexSignature": true,
    "noImplicitOverride": true,
    "noImplicitReturns": true,
    "noFallthroughCasesInSwitch": true,
    "useUnknownInCatchVariables": true,
    "verbatimModuleSyntax": true,
    "noEmit": true,
    "lib": ["ESNext"],
    "types": []
  }
}
```

## Native compiler policy

The TypeScript config improves semantic precision but is not a representation proof. Native lowering still performs its own safety analysis for assertions, ambient declarations, dynamic indexing, `any`, `unknown`, and runtime-created values.

The compiler will eventually support native-specific policy outside `compilerOptions`, for example:

```json
{
  "tsnative": {
    "safety": "strict",
    "wholeProgram": true,
    "closedWorld": true,
    "preferUnboxed": true,
    "dynamicFallback": "error",
    "lto": "thin"
  }
}
```

## TypeScript-LS contract

The official TypeScript 7 language server and native compiler must resolve the same project and the same `tsconfig.json`. The Go frontend must not maintain a second parser-specific config or SWC config.

Native-only settings such as representation policy, LLVM optimization, LTO, and dynamic fallback remain outside `compilerOptions`; TypeScript-LS owns TypeScript semantics while the Go compiler owns native policy.
