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
