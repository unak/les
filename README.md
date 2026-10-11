# Les Language Specification & Plan

## Overview
Les (Lazy Evaluation Script) is a statically-typed, aggressively lazy-evaluated scripting language implemented in Go.

## Built-in Types

| Type | Syntax / Examples | Description |
|---|---|---|
| `Int` | `42`, `-10`, `0` | 64-bit signed integer. |
| `Float` | `3.14`, `-0.5`, `2.0` | 64-bit IEEE 754 floating-point number. |
| `String` | `"hello"`, `'world'` | UTF-8 encoded string. |
| `Bool` | `True`, `False` | Boolean value. |
| `Tuple` | `()`, `(1,)`, `(1, 2, 3)` | Immutable, lazily evaluated sequence. Replaces array concept. Indexed by integer `t[0]`. |
| `Dict` | `{"a": 1, "b": 2}`, `{}` | Immutable persistent map based on HAMT (Hash Array Mapped Trie). Updates use structural sharing with O(log n) overhead, preserving referential transparency without full deep copies. Indexed by key `d["a"]`. |

Type annotations are optional (full HM-style type inference):
```python
x: Int = 10
p: (Int, String) = (1, "ok")
d: Dict[String, Int] = {"count": 42}
```

## Advanced Type System & Polymorphism

### 1. Union Types (`A | B`)
- Types can be composed as unions:
  ```python
  x: Int | String = cond ? 42 : "error"
  ```
- Exhaustive pattern matching or type narrowing resolves concrete types safely.

### 2. Structural Typing & Interfaces (Go-style Duck Typing)
- Types implicitly satisfy interfaces if they possess matching method/protocol signatures (no explicit `implements` keyword needed).
- Ad-hoc polymorphism: Built-in functions like `print` accept any type implementing the `Show` / stringifiable interface:
  ```python
  interface Show:
      to_string(): String
  ```
- All primitive types, Tuples, and Dicts implicitly satisfy `Show`.
- Future user-defined structs will automatically satisfy any matching interface structurally.

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
