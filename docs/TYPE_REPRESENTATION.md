# Type and Representation Model

TypeScript types and native runtime representations are separate layers.

```text
TypeScript semantic type
        |
        v
native safety / representation proof
        |
        +--> proven static representation
        |      F64, I32/I64, I1, StringRef, StructRef, Array<T>
        |
        +--> partially proven
        |      tagged unions, guards, checked references
        |
        +--> dynamic
               JSValue / runtime dispatch
```

## Core rule

Never lower `number`, a type assertion, ambient declaration, or `any` directly to a native LLVM type without representation proof.

Explicit `any` is allowed only as a dynamic boundary. Implicit `any` is rejected by the native profile.

## Initial representation set

- `Void`
- `Bool` -> LLVM `i1`
- `I32` / `I64` when integer range is proven
- `F64` for general JavaScript numbers
- `StringRef`
- `ArrayRef<T>` for homogeneous arrays
- `ObjectRef<Shape>` for closed object/class layouts
- `FunctionRef`
- `TaggedUnion`
- `JSValue` for truly dynamic values

## Optimization direction

Prefer known field offsets, direct calls, typed arrays, scalar replacement, and monomorphized generics. Runtime helpers are slow paths, not the default lowering strategy.
