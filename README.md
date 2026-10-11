# Les Language Specification & Plan

## Overview
Les (Lazy Evaluation Script) is a statically-typed, aggressively lazy-evaluated scripting language implemented in Go.

## Built-in Types

| Type | Syntax / Examples | Description |
|---|---|---|
| `Int` | `42`, `-10`, `0` | 64-bit signed integer. |
| `Float` | `3.14`, `-0.5`, `2.0` | 64-bit IEEE 754 floating-point number. |
| `String` | `"hello"`, `'world'` | Immutable UTF-8 encoded byte sequence (treated purely as raw UTF-8 byte stream for now). |
| `Bool` | `True`, `False` | Boolean value. |
| `Tuple` | `()`, `(1,)`, `(1, 2, 3)` | Immutable, lazily evaluated sequence. Replaces array concept. Indexed by integer `t[0]`. |
| `Dict` | `{"a": 1, "b": 2}`, `{}` | Immutable persistent map based on HAMT (Hash Array Mapped Trie). Updates use structural sharing with O(log n) overhead, preserving referential transparency without full deep copies. Indexed by key `d["a"]`. |

Type annotations are optional (full HM-style type inference):
```python
x: Int = 10
p: (Int, String) = (1, "ok")
d: Dict[String, Int] = {"count": 42}

# Infinite / stream Tuple: ellipsis '...' repeats the last specified element type indefinitely
fib_stream: (Int, ...) = fib(0, 1)

# Leading distinct types followed by infinite stream:
header_stream: (String, Int, ...) = ("data", 1, 2, 3, ...)
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

## Module System & Variable Scope

### 1. Identifier Visibility (Go-style)
- **Public (Exported)**: Starts with uppercase letter (`[A-Z]`). Accessible outside the module.
  ```python
  def Add(a, b):  # Exported
      a + b

  MaxVal = 100    # Exported
  ```
- **Private (Module-Internal)**: Starts with lowercase letter or underscore (`[a-z_]`). Accessible only within the module.
  ```python
  def helper(x):  # Private
      x * 2
  ```

### 2. Module Organization
- Terminology: Called **Module** in Les.
- **Default Module**: Directory name becomes module name implicitly.
- **Explicit Declaration**: Optional `module <name>` statement at file top overrides directory-based default:
  ```python
  module math
  ```
- **Importing & Module References**:
  - `import` declaration without `@`: `import math`
  - Referencing module members uses `@` prefix to clearly distinguish from variables:
    ```python
    import math

    result = @math.Add(1, 2)
    val = user.name  # Cleanly distinguished: variable/struct access does NOT use '@'
    ```

### 3. Scope & Rebinding Rules
- **Same Scope Rebinding**: **Forbidden (Error)**. A variable cannot be redeclared or rebound within the same lexical scope:
  ```python
  x = 10
  x = 20  # Compile / Type Error: duplicate definition of 'x' in same scope
  ```
- **Nested / Closure Scope Shadowing**: **Allowed**. Inner scopes (such as inside a function, block, or closure) may define a variable that shadows an outer scope variable:
  ```python
  x = 10
  def f():
      x = 20  # OK: shadows outer 'x' within f's local scope
      x
  ```

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

### 7. Union Types & Pattern Matching
- **Union Type Syntax**: `A | B` (e.g., `Int | String`, `Int | None`).
- **Branch Type Inference**: When branches (such as `? :`) return different types, the expression is inferred as a union type (`Int | String`).
- **Type Test Operator `is`**:
  - Binary operator testing dynamic value type: `expr is Type`.
  - Evaluates to `Bool` (`True` or `False`).
  ```python
  x = 10
  if x is Int:
      print "an integer"
  ```
- **Pattern Match Expression `match ... case`**:
  ```python
  def classify(v):
      match v:
          case Int:
              print "number:", v
          case String:
              print "string:", v
          case _:
              print "other"

  classify 100
  classify "les"
  ```

