# Les Language Specification & Plan

## Overview
Les (Lazy Evaluation Script) is a statically-typed, aggressively lazy-evaluated scripting language implemented in Go.

## Core Language Semantics

### 1. Function Application and Tuple Semantics
- Semantically, all function calls take a single **Tuple** argument.
- `f(a, b)` passes a 2-tuple `(a, b)`.
- Function parameter definitions `def f(a, b): ...` pattern-match / destructure the incoming argument tuple.
- Single argument `f(a)` or `f a`: passes `(a,)` (or 1-element tuple view).

### 2. Parenthesis Omission & Precedence
- Omission of parentheses in function application is permitted.
- **Precedence**: Function call without parentheses has lowest precedence among expression operators, binding greedily:
  ```python
  f a + b    # equivalent to f(a + b)
  print x * 2 + 1  # equivalent to print((x * 2) + 1)
  ```
- To call `f(a) + b`, explicit parentheses are required: `f(a) + b`.

### 3. Block Arguments (Option A: Ruby-like `do` blocks)
- Trailing block syntax:
  ```python
  each items do |x|:
      print x

  measure do:
      heavy_computation()

  map(items) do |x|:
      x * 2
  ```
- Semantics: The block is desugared into a closure / anonymous function passed as the final argument (or appended to the argument tuple).

### 4. Future Lambda Syntax Harmony
- Block: `do |x|: ...`
- Inline lambda (harmonious design): `|x|: expr` or `|x, y|: expr`.
  ```python
  # Future lambda compatibility:
  doubled = map items, |x|: x * 2
  ```

### 5. Control Flow & Operators
- Multi-line block `if / else`:
  ```python
  if x > 0:
      1
  else:
      0
  ```
- Inline ternary: C-style only (`cond ? then_expr : else_expr`).
- Tuples: `()`, `(1,)`, `(1, 2)`.
- Dicts: `{"a": 1}`.
- Indexing: `[]` for both tuples and dicts (`t[0]`, `d["a"]`).

### 6. Side Effects
- Demand-driven strict evaluation at built-in effect points (e.g. `print`).
- User-defined functions cannot create raw side-effect primitives.
