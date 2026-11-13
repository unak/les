# Les Language Syntax & Semantics Guide

This document provides a comprehensive specification and guide to the syntax, type system, semantics, and object-oriented features of the Les (Lazy Evaluation Script) programming language.

---

## Table of Contents
1. [Primitive Types](#1-primitive-types)
   - [Numeric Literal Syntax Rules & Disambiguation](#numeric-literal-syntax-rules--disambiguation)
   - [Tuple Disambiguation](#tuple-disambiguation)
2. [Advanced Type System & Polymorphism](#2-advanced-type-system--polymorphism)
   - [Union Types (`A | B`)](#union-types-a--b)
   - [Structural Typing & Interfaces](#structural-typing--interfaces)
   - [User-Defined Generics & Type Constraints](#user-defined-generics--type-constraints)
3. [Module System & Variable Scope](#3-module-system--variable-scope)
   - [Identifier Visibility (Go-style)](#identifier-visibility-go-style)
   - [Module Organization](#module-organization)
   - [Scope & Rebinding Rules](#scope--rebinding-rules)
   - [Unified Namespace (Lisp-1) & Bare Callable Statements](#unified-namespace-lisp-1--bare-callable-statements)
4. [Core Language Semantics](#4-core-language-semantics)
   - [Function Application and Tuple Semantics](#function-application-and-tuple-semantics)
   - [Parenthesis Omission & Precedence Table](#parenthesis-omission--precedence-table)
   - [Trailing Default Parameters & Block Parameters](#trailing-default-parameters--block-parameters)
   - [Lambda Expressions](#lambda-expressions)
   - [Control Flow & Operators](#control-flow--operators)
   - [Statement Separator (`;`) & Line Continuation (`\`)](#statement-separator---line-continuation-)
   - [Side Effects & I/O Semantics](#side-effects--io-semantics)
   - [Type System, Type Inference & Pattern Matching](#type-system-type-inference--pattern-matching)
   - [Special Types: `Nil` and `Unknown`](#special-types-nil-and-unknown)
   - [Variadic Arguments & Tuple Unpacking](#variadic-arguments--tuple-unpacking)
5. [Object-Oriented Programming (Structs & Interfaces)](#5-object-oriented-programming-structs--interfaces)
   - [Structs (`struct`)](#structs-struct)
   - [Dot Access & `@` Syntactic Sugar for `self.`](#dot-access---syntactic-sugar-for-self)
   - [Go-style Struct Embedding & Promotion](#go-style-struct-embedding--promotion)
   - [Interfaces (`interface`) & Structural Subtyping](#interfaces-interface--structural-subtyping)
6. [Error Handling](#6-error-handling)

---

## 1. Primitive Types

| Type | Syntax / Examples | Description |
|---|---|---|
| `Int` | `42`, `-10`, `0`, `0b1010`, `0o755`, `0d100`, `0xdead_beef`, `1_000_000` | Arbitrary-precision signed integer (transparently expanding from 64-bit int to multi-precision). Supports binary (`0b`/`0B`), octal (`0o`/`0O`), explicit decimal (`0d`/`0D`), hexadecimal (`0x`/`0X`), and visual digit separators (`_`). |
| `Float` | `3.14`, `-0.5`, `2.0`, `1_000.5`, `3.14159_26535` | 64-bit IEEE 754 floating-point number. Supports visual digit separators (`_`) in both integer and fractional parts. |
| `String` | `"hello"`, `'world'` | Immutable UTF-8 encoded string. Indexed by 0-based rune (Unicode code point) index `s[0]`, returning a 1-rune `String`. Supports negative index `s[-1]` (last rune). |
| `Bytes` | `Bytes("hello")`, `"hello".Bytes()`, `Bytes()` | Immutable raw byte sequence representing binary data. Indexed by 0-based byte offset `b[0]`, returning an `Int` (`0`–`255`). Supports negative index `b[-1]` (last byte). |
| `Bool` | `true`, `false` | Boolean value. |
| `Nil` | `nil` | Represents the absence of a value (singleton literal `nil`). Frequently used in union types (e.g., `String | Nil`). |
| `Tuple` | `()`, `(1,)`, `(1, 2, 3)` | Immutable, lazily evaluated sequence. Replaces array concept. Indexed by integer `t[0]`. Multiline literals and trailing commas are fully supported: `(\n    1,\n    2,\n)`. |
| `Dict` | `{"a": 1, "b": 2}`, `{}` | Immutable persistent map based on HAMT (Hash Array Mapped Trie). Updates use structural sharing with O(log n) overhead, preserving referential transparency without full deep copies. Multiline literals and trailing commas are fully supported: `{\n    "a": 1,\n    "b": 2,\n}`. Indexed by key `d["a"]`. Keys are strictly typed (e.g. `1` and `"1"` are distinct keys without coercion). |
| `Regex` | `/pattern/flags`, `Regex("pattern", "flags")` | Immutable compiled regular expression object. Supports flags `i` (case-insensitive), `m` (multi-line), `s` (dot matches all), and `U` (ungreedy). Disambiguated from division `/` based on preceding token context. |
| `Match` | Result of `s =~ r` or `r.Match(s)` | Successful regex match result. Indexed by 0-based group index `m[0]` (full match, `String`), `m[1..n]` (captures, `String | Nil`), negative index `m[-1]`, or group name `m["name"]`. Implements `Indexable[Int, String | Nil]` and `Indexable[String, String | Nil]`. |

> [!NOTE]
> **Numeric Literal Syntax Rules & Disambiguation**:
> - **Radix Prefixes**: Binary (`0b`/`0B`), Octal (`0o`/`0O`), Explicit Decimal (`0d`/`0D`), and Hexadecimal (`0x`/`0X`). The prefix must be followed immediately by at least one valid digit for that radix (e.g. `0b`, `0x`, or `0x_12` are syntax errors).
> - **Digit Separators (`_`)**:
>   - Underscores can only be placed strictly between two valid digits (e.g. `1_000`, `0xCAFE_BABE`, `1_000.5`, `3.14159_26535`).
>   - **Consecutive underscores (`__`) are forbidden**: e.g. `123__456` is a syntax error.
>   - **Trailing underscores are forbidden**: e.g. `1_` and `3.14_` are syntax errors.
>   - **Underscores directly adjacent to decimal points are forbidden**: e.g. `3._14` is invalid.
> - **Floating-Point Notation**:
>   - **Leading zero required**: `.1` is forbidden as a float literal; write `0.1`.
>   - **Trailing zero required**: `1.` is not a float literal; it is interpreted as integer `1` followed by member access dot `.` (to unambiguously support method calls like `1.String()`). Write `1.0` for a float.
>   - Multiple decimal points (e.g. `1.2.3`) are syntax errors.
> - **Invalid Suffixes**: Numbers cannot be immediately followed by letters without whitespace or operators (e.g. `0a0` or `123abc` are syntax errors).

> [!NOTE]
> **Tuple Disambiguation (Grouping Parentheses vs Tuple Literals and Types)**:
> - **Literals**:
>   - `(expr)` is treated as **expression grouping** and evaluates to the result of `expr` (e.g. `(1 + 2)` evaluates to `3`, not a tuple).
>   - `(expr,)` with a trailing comma creates a **1-element `Tuple`** (e.g. `(42,)` creates a `Tuple` containing `42`).
>   - `()` creates a **0-element (empty) `Tuple`**.
>   - `(expr1, expr2)` creates a **multi-element `Tuple`** (trailing comma is optional: `(1, 2)` or `(1, 2,)`).
> - **Type Annotations**:
>   - `(T)` is grouping for a type expression and resolves to `T` itself.
>   - `(T,)` with a trailing comma represents the type of a **1-element `Tuple`** holding `T`.
>   - `()` represents the type of an **empty `Tuple`**.
>   - `() | (T,)` represents a `Tuple` that contains **either 0 or 1 element** of type `T`.

Type annotations are optional (full HM-style type inference):
```python
x: Int = 10
p: (Int, String) = (1, "ok")
d: {String: Int} = {"count": 42}
opt: String | Nil = nil

# Single-element tuple literal and type annotation (trailing comma required):
single: (Int,) = (42,)

# Zero-or-one element tuple (e.g. optional single element):
zero_or_one: () | (String,) = ("hello",)
empty_or_one: () | (String,) = ()

# Infinite / stream Tuple: ellipsis '...' repeats the last specified element type indefinitely
fib_stream: (Int, ...) = fib(0, 1)

# Leading distinct types followed by infinite stream:
header_stream: (String, Int, ...) = ("data", 1, 2, 3, ...)
```

---

## 2. Advanced Type System & Polymorphism

### Union Types (`A | B`)
- Types can be composed as unions:
  ```python
  x: Int | String = cond ? 42 : "error"
  ```
- Exhaustive pattern matching or type narrowing resolves concrete types safely.

### Structural Typing & Interfaces (Go-style Duck Typing)
- Types implicitly satisfy interfaces if they possess matching method/protocol signatures (no explicit `implements` keyword needed).
- Ad-hoc polymorphism: Built-in functions like `print` accept any type implementing the `Show` / stringifiable interface:
  ```python
  interface Show:
      Inspect(): String
  ```
- All primitive types, Tuples, and Dicts implicitly satisfy `Show`.
- User-defined structs automatically satisfy any matching interface structurally.

### User-Defined Generics & Type Constraints
- **Type Parameters (`[T]`)**: Full generic support across functions/methods (`def`), structs (`struct`), and interfaces (`interface`):
  ```python
  struct Box[T]:
      val: T

  interface Container[T]:
      Get(): T

  def identity[T](val: T) -> T:
      val
  ```
- **Type Constraints (`[T: Constraint]`)**: Constrain generic type variables by interfaces or union constraints:
  ```python
  def display[T: Show](item: T):
      print item.Inspect()

  def sum[N: Numeric](a: N, b: N) -> N:
      a + b
  ```
- **`Self` Pseudo-Type & Default Type Parameters (`[T = Default]`)**:
  - Within `interface` and `struct` definitions, `Self` dynamically represents the receiver type in method parameter and return type annotations:
    ```python
    interface Cloneable:
        Clone(): Self

    struct Vec2:
        x: Int
        y: Int
        def Clone() -> Self:
            Vec2(@x, @y)
    ```
  - Type parameters can declare a default type using `[T = Default]` (or `[T: Constraint = Default]`), commonly `[T = Self]` for symmetric operations:
    ```python
    interface Addable[T = Self]:
        +(other: T): Self
    ```
  - When an interface with default type parameters is used without explicit type arguments (e.g. `v is Addable` or `[N: Numeric]`), its type parameter automatically defaults to `Self` (the receiver type). Explicit specialization (e.g. `v is Addable[Int]`) overrides the default.
  - Using `Self` outside `struct` or `interface` definitions (such as in standalone top-level functions) is a compile-time error.

---

## 3. Module System & Variable Scope

### Identifier Visibility (Go-style)
- Les follows Go-style visibility based on the casing of the first character of the identifier:
  - **Public (Exported)**: Starts with an uppercase Unicode letter (`[A-Z]`). Accessible when imported by other modules.
  - **Private (Module-local)**: Starts with a lowercase letter or underscore (`[a-z_]`). Inaccessible outside the declaring module; external access attempts raise a compile/eval error.
```python
module math

# Exported (Public)
def Add(a: Int, b: Int) -> Int:
    a + b

Pi: Float = 3.1415926535

# Unexported (Private)
def helper(x: Int) -> Int:
    x * 2
```

### Module Organization
- **File-level `module` declaration**: Placed at the top of a file to declare its module namespace:
  ```python
  module my_math
  ```
- **Module Imports (`import`)**:
  ```python
  import math
  import my_math as m
  import "path/to/module" as custom
  ```
- **Module Member Access (`::`)**: Access exported members using the double-colon scope resolution operator:
  ```python
  import math

  val = math::Sin(math::Pi / 2.0)
  print math::Pi
  ```

### Scope & Rebinding Rules
- Lexical scoping: Variables declared in inner blocks shadow outer scope variables.
- Immutable top-level bindings: Variables declared at module or global scope cannot be re-declared in the same scope.

### Unified Namespace (Lisp-1) & Bare Callable Statements
- Les employs a unified namespace (Lisp-1): functions, variables, and modules share the same symbol table.
- **Bare Callable Auto-Invocation**: Writing a 0-argument callable expression (function, method, lambda) as a standalone statement without parentheses automatically executes it:
  ```python
  exit       # Automatically invokes exit(0)
  math::Run  # Automatically invokes math::Run()
  ```

---

## 4. Core Language Semantics

### Function Application and Tuple Semantics
- Semantically, all function calls take a single **Tuple** argument.
- `f(a, b)` passes a 2-tuple `(a, b)`.
- Function parameter definitions `def f(a, b): ...` pattern-match / destructure the incoming argument tuple.
- Single argument `f(a)` or `f a`: passes `(a,)` (or 1-element tuple view).

### Parenthesis Omission & Precedence Table
- Omission of parentheses in function application is permitted.
- **Precedence**: Function call without parentheses has lowest precedence among expression operators, binding greedily:
  ```python
  f a + b          # equivalent to f(a + b)
  print x * 2 + 1  # equivalent to print((x * 2) + 1)
  print x |> f     # equivalent to print(f(x)) -- pipeline is consumed greedily as argument
  ```
- To call `f(a) + b`, explicit parentheses are required: `f(a) + b`.
- Similarly, to pass a function call result as the input to a pipeline, use explicit parentheses on the call: `f(x) |> g` (evaluates as `g(f(x))`).

#### Operators and Precedence
Listed from highest to lowest precedence:

| Precedence | Operator | Syntax / Example | Types | Description |
|---|---|---|---|---|
| 1 (Highest) | `.` | `p.x`, `p.dist()` | `StructInstance` | Member / method access (with promotion) |
| 1 | `[]` | `s[0]`, `t[1]`, `d["k"]` | `String`, `Tuple`, `Dict` | Indexing (0-based rune for String, negative indices supported) |
| 1 | `()` | `f(a, b)` | Function / Struct | Function call or struct constructor |
| 1 | `!` (postfix) | `expr!` | | Explicit demand point (postfix sugar for `force expr`) |
| 1 | `force` (prefix) | `force expr` | | Explicit demand point (strict evaluation) |
| 2 | `+`, `-`, `!`, `~` | `+x`, `-x`, `!flag`, `~mask` | `Int`/`Float` (`+`, `-`), `Bool` (`!`), `Int` (`~`) | Unary plus, negation, logical NOT, and bitwise NOT |
| 3 | `*`, `/`, `%` | `a * b`, `a / b`, `a % b` | `Int`, `Float` (`*`, `/`) | Multiplication, division, modulo |
| 4 | `+`, `-` | `a + b`, `a - b` | `Int`, `Float`, `String` (`+`), `Bytes` (`+`) | Addition, subtraction, string/bytes concatenation |
| 5 | `<<`, `>>` | `x << n`, `x >> n` | `Int` | Arithmetic bitwise shifts with Ruby semantics (`x << -n` acts as `x >> n`) |
| 6 | `&` | `a & b` | `Int` | Bitwise AND |
| 7 | `^` | `a ^ b` | `Int` | Bitwise XOR |
| 8 | `\|` | `a \| b` | `Int` | Bitwise OR |
| 9 | `<`, `<=`, `>`, `>=` | `a < b`, `a >= b` | `Int`, `Float`, `String`, `Bytes` | Relational comparisons |
| 10 | `is` | `x is Int`, `p is Reader` | | Dynamic type / interface satisfaction test |
| 11 | `==`, `!=`, `=~` | `a == b`, `s =~ /pat/` | Any, `String`/`Regex` | Equality / inequality comparison, pattern match |
| 12 | `&&` | `cond1 && cond2` | `Bool` | Short-circuit logical AND |
| 13 | `\|\|` | `cond1 \|\| cond2` | `Bool` | Short-circuit logical OR |
| 14 | `? :` | `cond ? a : b` | `Bool` (cond) | Inline ternary conditional |
| 15 | `\|>` | `x \|> f` | Any | Pipeline operator |
| 16 (Lowest) | *(space)* | `print x * 2 + 1` | Callable | Paren-omitted function call (weakest binding) |

#### Bitwise Operators (`~`, `&`, `^`, `|`, `<<`, `>>`)
Les provides comprehensive bitwise operators operating on arbitrary-precision integers (`Int`):

- **Bitwise NOT (`~x`)**: Inverts all bits (two's complement complement, equivalent to `-x - 1`). E.g. `~0 == -1`, `~(-1) == 0`.
- **Bitwise AND (`a & b`)**: Bitwise AND of two integers.
- **Bitwise XOR (`a ^ b`)**: Bitwise exclusive OR.
- **Bitwise OR (`a | b`)**: Bitwise inclusive OR. Syntactically unambiguous: expression bitwise OR is separated from union types (`A | B`) and lambda parameters (`|x|:`).
- **Arithmetic Shift (`<<`, `>>`) with Ruby Semantics**:
  - `x << n`: Left shift by `n` bits (multiplication by $2^n$).
  - `x >> n`: Arithmetic right shift by `n` bits (floor division by $2^n$). Preserves signs (e.g. `-8 >> 1 == -4`, `-1 >> 1 == -1`).
  - Supports arbitrary-precision expansion without overflow or loss of precision.
  - Negative shift count reverses the shift direction: `x << -n` is equivalent to `x >> n`, and `x >> -n` is equivalent to `x << n`.
  - Shift count safeguard: shifts exceeding $2^{26}$ bits raise a runtime error (`shift count too large: n`) to prevent memory exhaustion.
- **Operator Overloading & Pipeline Integration**:
  - Structs and interfaces can define methods for bitwise operators (`def ~()`, `def &(other)`, `def <<(n)`, `def >>(n)`).
  - Built-in `Int` exposes bound operator methods (e.g. `(12).&(10)`, `(1).<<(4)`).
  - Seamlessly supported on RHS of the pipeline operator: `x |> ~`, `x |> &(0xff)`, `x |> >>(2)`.

#### Indexing Details (`[]`)
- `String`: `s[i]` accesses rune (Unicode code point) at 0-based index `i`, returning a 1-rune `String`. Supports negative index `s[-1]` (last rune). Out-of-bounds index raises a runtime error.
- `Tuple`: `t[i]` accesses element at 0-based index `i`. Supports negative index `t[-1]` on finite tuples. Negative indexing is not supported on infinite tuples because an infinite sequence has no end, and attempting it raises a runtime error (`cannot use negative index on infinite tuple`). Out-of-bounds index raises a runtime error.
- `Dict`: `d[key]` accesses value by key (`d["a"]`). Keys are strictly typed (e.g. `1` and `"1"` do not conflict). Non-existent key raises a runtime error.

### Trailing Default Parameters & Block Parameters
- **Trailing Default Parameters**:
  Functions, methods, and struct definitions can define default values for trailing positional parameters. Once a parameter has a default value, all subsequent positional parameters must also provide defaults:
  ```python
  def greet(name: String, prefix: String = "Hello"):
      prefix + " " + name

  greet("Alice")           # "Hello Alice"
  greet("Bob", "Hi")       # "Hi Bob"
  ```
  - Default expressions are lazily evaluated within the call environment when omitted, allowing defaults to reference preceding parameters (e.g., `def clamp(x, min_val = 0, max_val = min_val + 100)`).
  - Struct fields also inherit default parameter initialization during positional instantiation:
    ```python
    struct Server:
        host: String
        port: Int = 8080

    s = Server("localhost")  # port automatically defaults to 8080
    ```

- **Explicit Block Parameters (`&block` / `&block = nil`)**:
  To cleanly distinguish regular arguments from trailing `do` blocks and avoid arity ambiguity when defaults are present, functions may declare an explicit block parameter at the end of the parameter list:
  ```python
  def each_step(items, step = 1, &block):
      i = 0
      for i < len(items):
          block(items[i])
          i = i + step

  # Invocation with default step and trailing block:
  each_step(numbers) do |n|:
      print n

  # Optional block:
  def run_job(name: String, &block = nil):
      if block != nil:
          block(name)
      else:
          print "Running default job: ", name
  ```
- **Backward Compatibility**:
  When a function does not define a `&block` parameter, trailing `do` blocks continue to be desugared and appended as the final positional argument.

### Lambda Expressions
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

### Control Flow & Operators
- Multi-line block `if / else`:
  ```python
  if x > 0:
      1
  else:
      0
  ```
- **`for` Loops & Loop Control**:
  Go-style `for` loops support both condition-controlled loops (`for <cond>:`) and infinite loops (`for:`), with `break` and `continue`:
  ```python
  # Infinite loop with break and continue
  for:
      line = gets()
      if line is Nil:
          break
      print line, "\n"

  # Condition-based loop
  i = 0
  for i < 10:
      print i, "\n"
      i = i + 1
  ```
- **Same-Type Variable Rebinding**:
  Re-binding an existing variable in the current scope is allowed as long as the new value has the same (or compatible) type. Type-mismatched assignments (e.g. `x = 10` followed by `x = "hello"`) are caught as static type errors. When rebinding an existing variable, the right-hand side is strictly evaluated (forced) immediately to prevent lazy evaluation cycles.
- **Pipeline Operator (`|>`)**:
  Functional pipelining to cleanly express transformation chains without nested parentheses.
  `expr |> fn` evaluates as `fn(expr)`, and `expr |> fn(arg2)` evaluates as `fn(expr, arg2)`:
  ```python
  result = raw_input |> clean |> parse |> format
  # Equivalent to: format(parse(clean(raw_input)))

  val = 10 |> add(20) # Equivalent to: add(10, 20)
  ```
- **Interaction with Parenthesis-Omission Calls**:
  Parenthesis-omitted call statements (`print expr`) bind greedily, so `print x |> f` is parsed as `print(f(x))`. If you want to pipeline into print, write `x |> f |> print`. If you want to pipe the result of a call, use explicit parentheses: `compute(x) |> format`.
- Tuples: `()`, `(1,)`, `(1, 2)`.
- Dicts: `{"a": 1}`.

### Statement Separator (`;`), Explicit Indentation (`+;`, `-;`, `;;`), & Line Continuation (`\`)
- **Statement Separator (`;`)**:
  Allows writing multiple statements on the same line:
  ```python
  x = 10; y = 20; print x + y
  ```
  Trailing semicolons (e.g. `x = 10;`), space-separated empty statements (`; ;`), and semicolons inside blocks or struct declarations are fully supported. Note that two consecutive semicolons without whitespace (`;;`) act as an explicit reset to top-level indentation (see below).
- **Explicit Indentation Semicolons (`+;`, `-;`, `--;`, `;;`)**:
  Allow writing indented blocks on a single line or without physical leading whitespace:
  - `+;`: Emits an `INDENT` token (pushes a virtual indent level). Also supports hybrid multiline blocks by syncing with the next line's physical indentation.
  - `-;` / `--;` / `---;` ...: Emits one `DEDENT` token per leading `-` (clamped at top-level 0).
  - `;;`: Emits `DEDENT` tokens to unwind all active indentation levels back to top-level 0 (when already at level 0, acts as a statement separator).
  ```python
  def abs(x): +; if x < 0: +; -x -; else: +; x;;
  ```
- **Line Continuation (`\`)**:
  Allows a statement to span multiple physical lines. The backslash `\` can optionally be followed by trailing whitespace and an inline comment (`# ...`) before the newline:
  ```python
  total = 10 + \          # base value
          20 + \          # bonus value
          30

  def add(a: Int, \
          b: Int): Int:
      a + b
  ```

### Side Effects & I/O Semantics
- **Output is a Demand Boundary (Strict Evaluation)**:
  - Demand-driven strict evaluation at built-in effect points (e.g. `print`, `puts`, `exit`). Arguments are forced immediately when the statement executes.
  - Top-level `return` acts as `exit`, so it also serves as a demand point forcing evaluation of its exit code expression.
  - Ensures prompt output (e.g. `print "Name: "`) displays before subsequent reads.
- **Explicit Demand Points (`expr!` / `force expr`)**:
  - By default, function arguments, variable initializations, tuple elements, and dict values are wrapped in lazy `Thunk`s (while constant literals are evaluated immediately to concrete values in WHNF).
  - Suffixing an expression with `!` (e.g. `expr!`) or prefixing with `force` (e.g. `force expr`) acts as an unconditional syntax-level strict evaluation demand point.
  - When passed as a function argument (`f(force expr)` or `f(expr!)`), bound to a variable (`x = force expr`), or stored in a container (`(force expr,)` or `{"k": force expr}`), the expression is strictly evaluated (forced) immediately at that site instead of creating a deferred lazy `Thunk`.
- **Input is a Deferred Thunk (Lazy Read)**:
  - I/O read operations (e.g. `gets()`) return a deferred `Thunk` without blocking at invocation time.
  - The actual I/O read / system call occurs only when the value is demanded (e.g., printed, compared, or concatenated). Unused inputs never block or perform reads.
- **Evaluation Order**:
  - Binary expressions, tuple elements, and arguments are strictly evaluated and forced **left-to-right**, guaranteeing predictable I/O sequence (e.g. `gets() + gets()`).
- Inside functions/closures, `return` and `break` are NOT side-effect points; values are carried lazily until demanded.
- User-defined functions cannot create raw side-effect primitives.

### Type System, Type Inference & Pattern Matching
- **Static Type Inference & Optional Annotations**:
  - Les features static type inference based on the Hindley-Milner algorithm. Explicit type annotations on variables, parameters, and return types are entirely optional:
    ```python
    x = 10              # inferred as Int
    s = "hello"         # inferred as String
    p = (1, "ok")       # inferred as (Int, String)
    d = {"a": 42}       # inferred as {String: Int}
    ```
  - Unannotated function parameters whose types cannot be uniquely constrained fall back to `Unknown`, allowing flexible dynamic code alongside strictly typed code:
    ```python
    def identity(x): x  # parameter and return type inferred as Unknown
    ```
  - **Inferred Types on Lazy Thunks**:
    Statically inferred types are propagated to lazy `Thunk` values at runtime (`thunk.InferredType`). This allows runtime type tests (`is`) and inspection (`.Type()`) to determine element types without forcing deferred computations or triggering premature side effects.
- **Function Types (`->`)**:
  Function types represent callable types mapping argument types to a return type: `(Arg1, Arg2) -> Ret`.
  - **Syntax**: `(T1, T2) -> Ret`, `(T) -> Ret`, `() -> Ret`, or right-associative higher-order types `(A) -> (B) -> C`.
  - **Variable annotations**:
    ```python
    f: (Int) -> Int = |x|: x * 2
    ```
  - **Higher-order function parameters**:
    ```python
    def apply(fn: (Int) -> String, x: Int): String:
        fn(x)
    ```
  - **Function return type annotations**:
    Return types on `def` can be annotated with `:` or `->`:
    ```python
    def add(a: Int, b: Int): Int:
        a + b

    def multiplier(factor: Int) -> (Int) -> Int:
        |x: Int|: x * factor
    ```
- **Union Types (`A | B`)**:
  - Types can be composed as unions (e.g., `Int | String`, `Int | Nil`).
  - **Branch Type Inference**: When conditional branches (such as `? :` or `if/else`) evaluate to different types, the expression is automatically inferred as their union type (`Int | String`).
- **Type Test Operator `is`**:
  - Binary operator testing dynamic value type: `expr is Type`.
  - Evaluates to `Bool` (`true` or `false`).
  - Supports primitive types, structs, interfaces, unions (`A | B`), and parameterized dictionary types `expr is {KeyType: ValueType}`.
  ```python
  x = 10
  if x is Int:
      print "an integer"

  d = {"count": 42}
  if d is {String: Int}:
      print "dict with String keys and Int values"
  ```
  > [!NOTE]
  > **Dictionary type validation and laziness**:
  > When checking dictionary types with `expr is {K: V}`, keys and values are verified while strictly preserving lazy evaluation semantics:
  > - **Constant literals** (e.g. `{"a": "hello"}`) are evaluated immediately at creation (WHNF), so type mismatches are caught right away (`d is {String: Int}` evaluates to `false`).
  > - **Lazy expressions** (e.g. `{"a": "he" + "llo"}`) carry their statically inferred types (`InferredType`). If the inferred type is incompatible with `V`, `is` returns `false` **without forcing the value**.
  > - **Exceptions & deferred computations** (e.g. `{"a": 1 / 0}`) with matching inferred types are **not forced**, safely evaluating to `true` without triggering runtime exceptions.
  > - **Unconstrained values** (such as returns from unannotated functions inferred as `Unknown`) preserve laziness and are skipped until demanded:
  >   ```python
  >   def get_val(x): x               # inferred return type: Unknown
  >   d = {"a": get_val("hello")}
  >   print (d is {String: Int})       # true (skipped because static type is Unknown)
  >   _ = force d["a"]                 # forces evaluation of the value ("hello")
  >   print (d is {String: Int})       # false (now evaluated as String, mismatching Int)
  >   ```

- **Pattern Match Expression `match ... case`**:
  Supports type patterns (including `{Key: Value}` and unions `A | B`), tuple patterns, singleton literals, regex patterns (with optional `as m` capture binding), and default `_`:
  ```python
  def classify(v):
      match v:
          case /^(?P<num>\d+)$/ as m:
              print "all digits:", m["num"]
          case {String: Int}:
              print "dict of string to int"
          case Int | Float:
              print "number:", v
          case String:
              print "string:", v
          case nil:
              print "nil value"
          case _:
              print "other"

  classify 100
  classify "123"
  classify "les"
  classify {"a": 1}
  classify nil
  ```

### Special Types: `Nil` and `Unknown`
- **`Nil`**:
  - Represents the absence of a value (literal `nil`, type `Nil`).
  - Useful in union types such as `Int | Nil` (nullable/optional semantics).
- **`Unknown` (Internal Inference Type)**:
  - Internal compiler placeholder used when a type cannot be statically constrained (for example, unannotated function parameters or empty dictionary `{}`).
  - Bidirectionally assignable during type inference, allowing unconstrained values to unify with any required type.
  - **Not a type annotation**: Writing `x: Unknown` is prohibited and causes a compile-time error. All user-facing types must be concrete or interfaces.

### Variadic Arguments & Tuple Unpacking

#### Variadic Parameters (`*args`)
- Function parameters can accept 0 or more arguments by prefixing with `*`:
  ```python
  def log(prefix: String, *args):
      print prefix, args
  ```
- **Type Representation**: A variadic parameter `*args` corresponds to the union type `() | (Unknown, ...)`.

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

#### Control Flow: `return`, `break`, and `exit`
- **`return [val]`** (Reserved Keyword):
  - **Inside `def` function**: Immediately terminates the function and returns `val` (or empty tuple `()` if omitted).
  - **Inside closure / lambda**: Performs **non-local return** — unwinds the call stack and exits the innermost enclosing `def` function lexical to the closure definition.
  - **At top level**: Terminates the entire process like `exit [code]`.
- **`break [val]`** (Reserved Keyword):
  - Using `break` outside of a closure is a syntax / parser error.
- **`exit([code])` / `exit`** (Built-in Function):
  - Terminates the process immediately with an integer status code.
  - Accepts 0 or 1 argument (`Int` or `Bool`).
  - Without arguments or bare `exit`: exits with code `0`.
  - With `true`: code `0`; with `false`: code `1`.

---

## 5. Object-Oriented Programming (Structs & Interfaces)

### Structs (`struct`)

Les provides immutable records with methods, Go-style struct embedding, and static ambiguous selector detection.

#### Definition
- Defined using `struct <Name>:` followed by an indented block.
- Closed class: Methods must be defined inside the struct body.
- Fields can have optional type annotations and default values.
- Identifiers starting with uppercase letters (`[A-Z]`) are public; lowercase (`[a-z_]`) are private.
- Methods have an implicit receiver `self` (can be omitted in method parameter list, fields directly accessible in scope; or specified explicitly).

```python
struct Point:
    x: Int = 0
    y: Int = 0

    def DistSq():
        x * x + y * y

    def Add(self, other: Point):
        Point(x: self.x + other.x, y: self.y + other.y)
```

#### Instantiation & Functional Updates
- **Positional Arguments**: `p = Point(10, 20)`
- **Named Arguments**: `p = Point(y: 20, x: 10)`
- **Defaults**: Unspecified fields use default expressions or `()`.
- **Immutable Copy-on-Write Update**: Pass an existing instance as the first positional argument followed by updated named arguments:
  ```python
  p2 = Point(p, x: 30)  # y is preserved from p
  ```

#### Custom Initializer (`def init`) & Argument Auto-Binding
- Users can define custom initializers using `def init(...)` inside the struct body.
- **Auto-binding**: Parameters matching struct field names are automatically assigned to `self` upon entry.
- **Member Assignment**: Fields can be explicitly assigned or overridden via `self.<field> = <expr>` during initialization. Once initialization finishes, struct instances are frozen and strictly immutable (modifying `p.x = ...` outside `init` is a runtime error).
- **Default Values**: Fields omitted from `init` retain their declared default values.

```python
struct PolarPoint:
    x: Float
    y: Float

    def init(r: Float, theta: Float):
        self.x = r * cos(theta)
        self.y = r * sin(theta)

struct Person:
    name: String
    age: Int
    city: String = "Tokyo"

    def init(name, age):
        if age < 0:
            exit(1)
        # name and age are auto-bound; city retains default "Tokyo"
```

#### Dot Access & `@` Syntactic Sugar for `self.`
- Member access: `p.x`, `p.DistSq()`
- Struct fields are lazily evaluated thunks.
- **`@` Syntactic Sugar for `self.`**:
  Inside struct methods and `init`, `@<member>` serves as concise syntactic sugar for receiver access:
  - `@x` desugars to `self.x`
  - `@calc(a, b)` and statement-start `@calc a, b` desugar to `self.calc(a, b)`
  - `@x = expr` desugars to `self.x = expr` (valid inside `init`)
  - `@node.next` desugars to `self.node.next`
  - Whitespace between `@` and the identifier (e.g. `@ x`) and bare `@` without an identifier are syntax errors.

```python
struct Point:
    x: Int
    y: Int

    def init(x: Int, y: Int):
        @x = x
        @y = y

    def Shift(dx: Int, dy: Int) -> Point:
        Point(x: @x + dx, y: @y + dy)

    def DistanceSquared() -> Int:
        @x * @x + @y * @y
```

#### Go-style Struct Embedding & Promotion
- Structs can embed other structs anonymously by listing their type name in the struct body.
- Embedded fields and methods are automatically promoted to the outer struct.
- An embedded type also participates in subtyping: an outer instance satisfies `is Inner`.

```python
struct Reader:
    def Read(): "read data"

struct ReadCloser:
    Reader
    def Close(): "closed"

rc = ReadCloser()
print rc.Read()         # "read data" (promoted method)
print rc is Reader      # true
```

#### Static Ambiguous Selector Detection
When multiple embedded structs define the same member (field or method), Les statically reports a type error (`ambiguous selector`) at type-checking / inference time unless the outer struct explicitly defines/overrides the member.

### Interfaces (`interface`) & Structural Subtyping

Les provides Go-style implicit structural typing (duck typing) via interface declarations.

#### Definition
- Defined using `interface <Name>:` followed by an indented block.
- Contains only method signatures: `<name>(<params...>): <ReturnType>`.
- Receiver (`self`) is omitted from interface method signatures.
- Interfaces can embed other interfaces.

```python
interface Reader:
    Read(n: Int): String

interface Closer:
    Close(): ()

interface ReadCloser:
    Reader
    Closer
```

#### Implicit Satisfaction (Duck Typing)
- Types (`struct`) automatically satisfy an interface if they provide all required methods matching the signature.
- No `implements` keyword required.

```python
struct File:
    path: String

    def Read(n: Int):
        "read bytes"

    def Close():
        ()

# Statically assignable to ReadCloser
rc: ReadCloser = File(path: "/tmp/data")
```

#### Composition & Conflict Resolution
- Embedding multiple interfaces that declare identical method signatures is valid (unified into one).
- Conflicting signatures (same method name but differing parameter or return types) produce a static compilation / type error at interface definition time.

#### Dynamic Runtime Checks (`is`)
- `instance is InterfaceName` dynamically checks whether the target value implements all required methods (including embedded interfaces).

---

## 6. Error Handling

Les treats errors as first-class values rather than throwing exceptions, perfectly avoiding evaluation-order leakage in lazy evaluation.

### `interface Error` & PascalCase Signatures
In accordance with Go-style visibility conventions (uppercase letter indicates exported/public member), interface method signatures use PascalCase:
```python
interface Error:
    Message(): String
    Inspect(): String
```

### Built-in `Error(msg)` Constructor
A convenient built-in `Error` constructor generates standard error instances satisfying `interface Error`:
```python
err = Error("file not found")
print err           # "Error: file not found" (formatted via Inspect)
print err.Message() # "file not found"
print err is Error  # true
```

### Idiomatic Error Handling via Union Types (`Value | Error`) & Pattern Matching
Functions declare union return types (`Type | Error`) and callers inspect results using pattern matching:
```python
def divide(a: Int, b: Int): Int | Error:
    if b == 0:
        Error("division by zero")
    else:
        a / b

# Pattern matching on Union result
res = divide(10, 0)
match res:
    case Int:
        print "Success: ", res
    case Error:
        print "Failed: ", res.Message()
```

### Duck Typing Custom Errors
Any user-defined struct providing `Message(): String` (or `Error(): String`) automatically satisfies `Error` structurally without explicit declarations:
```python
struct ParseError:
    line: Int
    detail: String

    def Message():
        "line " + line + ": " + detail

    def Inspect():
        "ParseError(" + detail + ")"

# Automatically matches `case Error:` and `is Error`!
```
