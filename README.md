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
### 4. Unified Namespace (Lisp-1) & Bare Callable Statements
- **Lisp-1 Namespace**: Functions and variables share the same unified lexical namespace. Functions are first-class citizens and can be assigned to variables or passed to higher-order functions directly:
  ```python
  def double(x): x * 2
  f = double
  f(21)            # 42

  def apply(fn, val): fn(val)
  apply(double, 10) # 20
  ```
- **Bare Callable Statements**: When an expression statement consists solely of a bare identifier or module member that resolves to a function, it is automatically invoked with 0 arguments. Inside other expressions (such as assignments or arguments), it evaluates to the function value itself:
  ```python
  puts             # Statement level: auto-invoked as puts() (prints newline)
  f = puts         # Expression level: assigned as a function object
  f "hello"        # Invoked with argument
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

### 4. Lambda Expressions
Anonymous functions (lambdas) support both single-line expressions and multi-line indented blocks:

- **Single-Line Inline Lambda**:
  ```python
  inc = |x|: x + 1
  add = |x, y|: x + y
  get_const = ||: 42
  doubled = map(items, |x|: x * 2)
  ```
- **Multi-Line Indented Lambda Block**:
  ```python
  complex_fn = |x|:
      y = x * 2
      y + 10
  ```
- **Semantics**: Evaluates to a first-class `Closure` capturing lexical scope. Returns the evaluated expression or the last statement of the block.

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
- **Union Type Syntax**: `A | B` (e.g., `Int | String`, `Int | Nil`).
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
          case Nil:
              print "nil value"
          case _:
              print "other"

  classify 100
  classify "les"
  classify Nil
  ```

### 8. Built-in Functions

#### `def print(*args: Any): ()`
Outputs string representations of all arguments directly without inserting spaces or appending a trailing newline.

- **Effect**: Built-in demand boundary; forces evaluation of lazy arguments (`Thunk`s).
- **Arguments**: `*args: Any` — variable number of values to print.
- **Returns**: `()` (empty tuple).
- **Usage**:
  ```python
  print "Hello, world!\n"
  print "a", "b", 42  # outputs "ab42" without space or newline
  ```

#### `def len(collection: Any): Int`
Returns the length of a `String` (rune count), `Tuple`, or `Dict`.

- **Arguments**: `collection: Any`
- **Returns**: `Int`
- **Indexing Support**: `String` and `Tuple` support 0-based indexing as well as negative indices (`[-1]` for last element).

#### `def type(*args: Any): String`
Returns the string representation of the runtime type of the given value or argument tuple.

- **Arguments**: `*args: Any` — single value or multiple arguments.
- **Returns**: `String` — type name (e.g. `"Int"`, `"(Int, Int)"`, `"(Int)"`).
- **Behavior**:
  - `type(42)` -> `"Int"`
  - `type((1,))` -> `"(Int)"`
  - `type(1, 2)` -> `"(Int, Int)"`
  - `type("hello")` -> `"String"`
  - `type(Nil)` -> `"Nil"`
  - `type()` -> `"()"`
- **Usage**:
  ```python
  t = type(42)       # "Int"
  t2 = type(1, 2)    # "(Int, Int)"
  ```

#### `def exit(code: Int | Bool = 0)`
Terminates the program execution immediately with the given exit status code.

- **Arguments**: Optional 0 or 1 argument (`Int` or `Bool`).
  - No arguments (`exit` or `exit()`): exits with code `0`.
  - `Int`: exits with the given integer code (e.g. `exit 0`, `exit 42`).
  - `Bool`: `True` exits with code `0`, `False` exits with code `1`.
- **Usage**:
  ```python
  exit           # Bare statement auto-call: exits with 0
  exit 0
  exit True      # exits with 0
  exit False     # exits with 1
  exit 1
  ```


### 9. Special Types: `Nil` and `Any`
- **`Nil`**:
  - Represents the absence of a value (both as the literal `Nil` and the type `Nil`).
  - Useful in union types such as `Int | Nil` (nullable/optional semantics).
- **`Any`**:
  - Safe top type, analogous to Go's `any` or TypeScript's `unknown`.
  - Any value can be assigned to `Any`, but values typed as `Any` cannot be used directly in operations without type refinement via `is` or `match`.

### 10. Variadic Arguments & Tuple Unpacking

#### Variadic Parameters (`*args`)
- Function parameters can accept 0 or more arguments by prefixing with `*`:
  ```python
  def log(prefix: String, *args: Any):
      print prefix, args
  ```
- **Type Representation**: A variadic parameter `*args: T` corresponds to the union type `() | (T, ...)`.

#### Tuple Destructuring in Pattern Matching
- Extract head and lazy tail stream elements via `(h, ...t)`:
  ```python
  def fib(a: Int, b: Int):
      (a, fib(b, a + b))

  stream = fib(0, 1)

  match stream:
      case (first, rest):
          print "first:", first
          match rest:
              case (second, ...):
                  print "second:", second
  ```

#### Unpack Assignment
- Unpack fixed-length or stream tuples using either parenthesized or splat syntax:
  ```python
  # Cons-cell stream unpacking
  (f0, s1) = fib(0, 1)
  (f1, s2) = s1

  # Splat unpack assignment
  head, ...tail = (1, 2, 3, 4)
  print head  # 1
  print tail  # (2, 3, 4)
  ```




