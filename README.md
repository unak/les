# Les Language Specification & Plan

## Overview
Les (Lazy Evaluation Script) is a statically-typed, aggressively lazy-evaluated scripting language implemented in Go.

## Core Language Features
- **Syntax**: Python-style indentation (off-side rule).
- **Evaluation Strategy**: Aggressive lazy evaluation (memoized thunks / graph reduction).
- **Side Effects**: Implicit evaluation demanded at effect boundaries (e.g. `print()`). Raw effect primitives are built-in only.
- **Type System**: Static typing with full Hindley-Milner style type inference. Type annotations optional.
- **Primitive Types**: `Int`, `Float`, `String`, `Bool`, `Tuple`, `Dict`. No dedicated Array type.

## MVP Objectives
1. Basic expressions and `print()` with strict evaluation demand at side-effect points.
2. Lazy Fibonacci and infinite tuple streams taking head `n` elements.
3. Function definitions (`def f(...)`) with lazy arguments (unused arguments are never computed).

