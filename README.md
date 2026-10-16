# Les Language Specification & Plan

## Overview
Les (Lazy Evaluation Script) is a statically-typed, aggressively lazy-evaluated scripting language implemented in Go.

## Build & Installation

### Prerequisites
- Go 1.26 or higher

### Build
```bash
# Build the binary
go build -o les ./cmd/les

# Or install to $GOPATH/bin (or $GOBIN)
go install ./cmd/les

# Run test suite
go test ./...
```

## Command-Line Usage

```bash
# Run a script file
les script.les

# Run a script from standard input (pipe, redirection, or explicit '-')
cat script.les | les
echo "print 42" | les
les - arg1 arg2 < script.les

# Run an inline script (one-liner)
les -e "x = 10; y = 20; print x + y"

# Check syntax only (-c)
les -c script.les
les -c -e "x = 10; y = 20"
les -c < script.les

# Check syntax and types only (-t), resolving imported modules
les -t script.les
les -t -e "x: Int = 10"
les -t < script.les

# Display version and exit (-v)
les -v

# Pass arguments to the script
les script.les arg1 arg2
les -e "print Args[0]" foo bar
les - arg1 arg2 < script.les
```

### Options:
- `-v`: Display version and platform information, then exit.
- `-c`: Check syntax only without type checking or execution.
- `-t`: Check syntax and static types (including resolving and verifying imported modules) without execution.
- `-e <code>`: Evaluate the inline script string directly.

## Primitive Types

| Type | Syntax / Examples | Description |
|---|---|---|
| `Int` | `42`, `-10`, `0`, `0b1010`, `0o755`, `0d100`, `0xdead_beef`, `1_000_000` | 64-bit signed integer. Supports binary (`0b`/`0B`), octal (`0o`/`0O`), explicit decimal (`0d`/`0D`), hexadecimal (`0x`/`0X`), and visual digit separators (`_`). |
| `Float` | `3.14`, `-0.5`, `2.0`, `1_000.5`, `3.14159_26535` | 64-bit IEEE 754 floating-point number. Supports visual digit separators (`_`) in both integer and fractional parts. |
| `String` | `"hello"`, `'world'` | Immutable UTF-8 encoded string. Indexed by 0-based rune (Unicode code point) index `s[0]`, returning a 1-rune `String`. Supports negative index `s[-1]` (last rune). |
| `Bytes` | `Bytes("hello")`, `"hello".Bytes()`, `Bytes()` | Immutable raw byte sequence representing binary data. Indexed by 0-based byte offset `b[0]`, returning an `Int` (`0`–`255`). Supports negative index `b[-1]` (last byte). |
| `Bool` | `true`, `false` | Boolean value. |
| `Nil` | `nil` | Represents the absence of a value (singleton literal `nil`). Frequently used in union types (e.g., `String | Nil`). |
| `Tuple` | `()`, `(1,)`, `(1, 2, 3)` | Immutable, lazily evaluated sequence. Replaces array concept. Indexed by integer `t[0]`. Multiline literals and trailing commas are fully supported: `(\n    1,\n    2,\n)`. |
| `Dict` | `{"a": 1, "b": 2}`, `{}` | Immutable persistent map based on HAMT (Hash Array Mapped Trie). Updates use structural sharing with O(log n) overhead, preserving referential transparency without full deep copies. Multiline literals and trailing commas are fully supported: `{\n    "a": 1,\n    "b": 2,\n}`. Indexed by key `d["a"]`. Keys are strictly typed (e.g. `1` and `"1"` are distinct keys without coercion). |

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

Type annotations are optional (full HM-style type inference):
```python
x: Int = 10
p: (Int, String) = (1, "ok")
d: {String: Int} = {"count": 42}
opt: String | Nil = nil

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
      ToString(): String
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
- **Same Scope Rebinding**: **Allowed if the new value has the same or compatible type**. Rebinding with an incompatible type produces a static type error:
  ```python
  x = 10
  x = x + 1  # OK: re-bound to Int
  x = "str"  # Type Error: cannot rebind variable 'x' of type Int with incompatible type String
  ```
- **Nested / Closure Scope Shadowing**: **Allowed**. Inner scopes (such as inside a function, block, or closure) may define a variable that shadows an outer scope variable:
  ```python
  x = 10
  def f():
      x = "hello"  # OK: shadows outer 'x' within f's local scope with any type
  ```
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
  print x |> f     # equivalent to print(f(x)) -- pipeline is consumed greedily as argument
  ```
- To call `f(a) + b`, explicit parentheses are required: `f(a) + b`.
- Similarly, to pass a function call result as the input to a pipeline, use explicit parentheses on the call: `f(x) |> g` (evaluates as `g(f(x))`).

### 3. Block Arguments (Ruby-like `do` blocks)
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

### 6. Statement Separator (`;`) & Line Continuation (`\`)
- **Statement Separator (`;`)**:
  Allows writing multiple statements on the same line:
  ```python
  x = 10; y = 20; print x + y
  ```
  Trailing semicolons (e.g. `x = 10;`), multiple consecutive semicolons, and semicolons inside blocks or struct declarations are fully supported.
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

#### Operators and Precedence
Listed from highest to lowest precedence:

| Precedence | Operator | Syntax / Example | Types | Description |
|---|---|---|---|---|
| 1 (Highest) | `.` | `p.x`, `p.dist()` | `StructInstance` | Member / method access (with promotion) |
| 1 | `[]` | `s[0]`, `t[1]`, `d["k"]` | `String`, `Tuple`, `Dict` | Indexing (0-based rune for String, negative indices supported) |
| 1 | `()` | `f(a, b)` | Function / Struct | Function call or struct constructor |
| 1 | `!` (postfix) | `expr!` | `Any` | Explicit demand point (syntactic sugar for unshadowed built-in `force(expr)`) |
| 2 | `+`, `-`, `!` | `+x`, `-x`, `!flag` | `Int`/`Float` (`+`, `-`), `Bool` (`!`) | Unary plus, negation, and logical NOT |
| 3 | `*`, `/`, `%` | `a * b`, `a / b`, `a % b` | `Int` | Multiplication, division, modulo |
| 4 | `+`, `-` | `a + b`, `a - b` | `Int` (`+`, `-`), `String` (`+`) | Addition, subtraction, string concatenation |
| 5 | `<`, `<=`, `>`, `>=` | `a < b`, `a >= b` | `Int` | Relational comparisons |
| 6 | `is` | `x is Int`, `p is Reader` | `Any` | Dynamic type / interface satisfaction test |
| 7 | `==`, `!=` | `a == b`, `a != b` | `Int`, `String`, `Bool` | Equality / inequality comparison |
| 8 | `&&` | `cond1 && cond2` | `Bool` | Short-circuit logical AND |
| 9 | `\|\|` | `cond1 \|\| cond2` | `Bool` | Short-circuit logical OR |
| 10 | `? :` | `cond ? a : b` | `Bool` (cond) | Inline ternary conditional |
| 11 (Lowest) | *(space)* | `print x * 2 + 1` | Callable | Paren-omitted function call (weakest binding) |

#### Indexing Details (`[]`)
- `String`: `s[i]` accesses rune (Unicode code point) at 0-based index `i`, returning a 1-rune `String`. Supports negative index `s[-1]` (last rune). Out-of-bounds index raises a runtime error.
- `Tuple`: `t[i]` accesses element at 0-based index `i`. Supports negative index `t[-1]`. Out-of-bounds index raises a runtime error.
- `Dict`: `d[key]` accesses value by key (`d["a"]`). Keys are strictly typed (e.g. `1` and `"1"` do not conflict). Non-existent key raises a runtime error.

### 7. Side Effects & I/O Semantics
- **Output is a Demand Boundary (Strict Evaluation)**:
  - Demand-driven strict evaluation at built-in effect points (e.g. `print`, `puts`, `exit`). Arguments are forced immediately when the statement executes.
  - Top-level `return` acts as `exit`, so it also serves as a demand point forcing evaluation of its exit code expression.
  - Ensures prompt output (e.g. `print "Name: "`) displays before subsequent reads.
- **Explicit Demand Points (`expr!` / `force(expr)`)**:
  - By default, function arguments, variable initializations, tuple elements, and dict values are wrapped in lazy `Thunk`s (while constant literals are evaluated immediately to concrete values in WHNF).
  - Suffixing an expression with `!` (e.g. `expr!`) acts as an unconditional syntax-level strict evaluation demand point. Calling the unshadowed built-in `force(expr)` has the same effect.
  - When passed as a function argument (`f(expr!)`), bound to a variable (`x = expr!`), or stored in a container (`(expr!,)` or `{"k": expr!}`), the expression is strictly evaluated (forced) immediately at that site instead of creating a deferred lazy `Thunk`.
  - *Note*: If the identifier `force` is shadowed in local scope (e.g. by a parameter), `expr!` continues to strictly force evaluation at the syntax level, while `force(expr)` resolves to the shadowed local binding without creating an automatic eager demand point.
- **Input is a Deferred Thunk (Lazy Read)**:
  - I/O read operations (e.g. `gets()`) return a deferred `Thunk` without blocking at invocation time.
  - The actual I/O read / system call occurs only when the value is demanded (e.g., printed, compared, or concatenated). Unused inputs never block or perform reads.
- **Evaluation Order**:
  - Binary expressions, tuple elements, and arguments are strictly evaluated and forced **left-to-right**, guaranteeing predictable I/O sequence (e.g. `gets() + gets()`).
- Inside functions/closures, `return` and `break` are NOT side-effect points; values are carried lazily until demanded.
- User-defined functions cannot create raw side-effect primitives.

### 8. Type System, Type Inference & Pattern Matching
- **Static Type Inference & Optional Annotations**:
  - Les features static type inference based on the Hindley-Milner algorithm. Explicit type annotations on variables, parameters, and return types are entirely optional:
    ```python
    x = 10              # inferred as Int
    s = "hello"         # inferred as String
    p = (1, "ok")       # inferred as (Int, String)
    d = {"a": 42}       # inferred as {String: Int}
    ```
  - Unannotated function parameters whose types cannot be uniquely constrained fall back to dynamic typing (`Any`), allowing flexible dynamic code alongside strictly typed code:
    ```python
    def identity(x): x  # parameter and return type inferred as Any
    ```
  - **Inferred Types on Lazy Thunks**:
    Statically inferred types are propagated to lazy `Thunk` values at runtime (`thunk.InferredType`). This allows runtime type tests (`is`) and inspection (`type()`) to determine element types without forcing deferred computations or triggering premature side effects.
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
  > - **Dynamically typed values** (such as returns from unannotated functions inferred as `Any`) preserve laziness and are skipped until demanded:
  >   ```python
  >   def get_val(x): x               # inferred return type: Any
  >   d = {"a": get_val("hello")}
  >   print (d is {String: Int})       # true (skipped because static type is Any)
  >   _ = d["a"]!                      # forces evaluation of the value ("hello")
  >   print (d is {String: Int})       # false (now evaluated as String, mismatching Int)
  >   ```

- **Pattern Match Expression `match ... case`**:
  Supports type patterns (including `{Key: Value}` and unions `A | B`), tuple patterns, singleton literals, and default `_`:
  ```python
  def classify(v):
      match v:
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
  classify "les"
  classify {"a": 1}
  classify nil
  ```

### 9. Special Types: `Nil` and `Any`
- **`Nil`**:
  - Represents the absence of a value (literal `nil`, type `Nil`).
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

#### Control Flow: `return`, `break`, and `exit`
- **`return [val]`** (Reserved Keyword):
  - **Inside `def` function**: Immediately terminates the function and returns `val` (or empty tuple `()` if omitted).
  - **Inside closure / lambda**: Performs **non-local return** — unwinds the call stack and exits the innermost enclosing `def` function lexical to the closure definition.
  - **At top level**: Terminates the entire process like `exit [code]`.
- **`break [val]`** (Reserved Keyword):
  - Immediately exits the current closure (lambda or block) returning `val` (or `()` if omitted).
  - Using `break` outside of a closure is a syntax / parser error.
- **`exit([code])` / `exit`** (Built-in Function):
  - Terminates the process immediately with an integer status code.
  - Accepts 0 or 1 argument (`Int` or `Bool`).
  - Without arguments or bare `exit`: exits with code `0`.
  - With `true`: code `0`; with `false`: code `1`.

### 11. Structs (`struct`)

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

#### Dot Access
- Member access: `p.x`, `p.DistSq()`
- Struct fields are lazily evaluated thunks.

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

### 12. Interfaces (`interface`) & Structural Typing

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

### 13. Error Handling & `interface Error`

Les treats errors as first-class values rather than throwing exceptions, perfectly avoiding evaluation-order leakage in lazy evaluation.

#### `interface Error` & PascalCase Signatures
In accordance with Go-style visibility conventions (uppercase letter indicates exported/public member), interface method signatures use PascalCase:
```python
interface Error:
    Message(): String
    ToString(): String
```

#### Built-in `Error(msg)` Constructor
A convenient built-in `Error` constructor generates standard error instances satisfying `interface Error`:
```python
err = Error("file not found")
print err           # "Error: file not found" (formatted via ToString)
print err.Message() # "file not found"
print err is Error  # true
```

#### Idiomatic Error Handling via Union Types (`Value | Error`) & Pattern Matching
Functions declare union return types (`Type | Error`) and callers inspect results using pattern matching:
```python
def divide(a: Int, b: Int): Int | Error:
    if b == 0:
        Error("division by zero")
    else:
        a / b

# Pattern matching on Union result
match divide(10, 0):
    case Int:
        print "Success: ", it
    case Error:
        print "Failed: ", it.Message()
```

#### Duck Typing Custom Errors
Any user-defined struct providing `Message(): String` (or `Error(): String`) automatically satisfies `Error` structurally without explicit declarations:
```python
struct ParseError:
    line: Int
    detail: String

    def Message():
        "line " + line + ": " + detail

    def ToString():
        "ParseError(" + detail + ")"

# Automatically matches `case Error:` and `is Error`!
```

## Built-in Functions, Variables, Methods, Interfaces and Modules

### 1. Built-in Functions

#### `def print(*args: Show): ()`
Outputs string representations of all arguments directly without inserting spaces or appending a trailing newline.

- **Effect**: Built-in demand boundary; forces evaluation of lazy arguments (`Thunk`s).
- **Arguments**: `*args: Show` — variable number of values satisfying the `Show` interface (`ToString(): String`).
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
  - `type(nil)` -> `"Nil"`
  - `type()` -> `"()"`
  - `type({"key": "value"})` -> `"{String: String}"`
  - `type({})` -> `"{Any: Any}"`
- **Usage**:
  ```python
  t = type(42)       # "Int"
  t2 = type(1, 2)    # "(Int, Int)"
  td = type({"a": 1}) # "{String: Int}"
  ```

#### `def exit(code: Int | Bool = 0)`
Terminates the program execution immediately with the given exit status code.

- **Arguments**: Optional 0 or 1 argument (`Int` or `Bool`).
  - No arguments (`exit` or `exit()`): exits with code `0`.
  - `Int`: exits with the given integer code (e.g. `exit 0`, `exit 42`).
  - `Bool`: `true` exits with code `0`, `false` exits with code `1`.
- **Usage**:
  ```python
  exit           # Bare statement auto-call: exits with 0
  exit 0
  exit true      # exits with 0
  exit false     # exits with 1
  exit 1
  ```

#### `def force(x: Any): Any`
Explicit demand point that strictly evaluates (forces) its argument and returns the evaluated concrete value.

- **Arguments**: `x: Any` — expression or value to force.
- **Returns**: `Any` — concrete evaluated value.
- **Syntactic Sugar**: The postfix exclamation mark `expr!` is syntactic sugar for the built-in `force(expr)`. Unlike function calls which can be shadowed by local bindings or parameters, `expr!` is a dedicated syntax form that always guarantees immediate evaluation.
- **Usage**:
  ```python
  x = force(10 + 20)
  # Forces argument evaluation before function application or container construction:
  pick(42, force(expensive_call()))
  ```

#### `def gets(): String | Nil`
Reads a single line from standard input, stripping any trailing newline (`\n` or `\r\n`). Returns `nil` upon reaching EOF.

- **Effect**: Returns a deferred lazy `Thunk`. Invoking `gets()` does not block immediately; the actual read occurs only when the value is demanded (e.g. forced via `print`, `!`, or string concatenation).
- **Arguments**: None (`()`).
- **Returns**: `String | Nil` — the line content as a `String`, or `nil` on EOF.
- **Usage**:
  ```python
  line = gets()
  if line is Nil:
      print "EOF reached\n"
  else:
      print "Read line: ", line, "\n"
  ```

### 2. Built-in Variables & CLI Arguments

Les provides predefined variables for command-line arguments and file path inspection:

| Variable | Type | Scope | Description |
|---|---|---|---|
| `Args` | `(String, ...)` | Global built-in | Immutable tuple of command-line arguments passed to the script (excluding the script name). Empty tuple `()` if no arguments were provided. |
| `ProgramName` | `String` | Global built-in | Path or name of the main program file executed from CLI (equivalent to Ruby's `$0` or Python's `sys.argv[0]`). |
| `Stdin` | `Reader` | Global built-in | Buffered standard input reader (`@bufio.Reader` wrapping `@io.Stdin`). |
| `Stdout` | `Writer` | Global built-in | Buffered standard output writer (`@bufio.Writer` wrapping `@io.Stdout`). |
| `Stderr` | `Writer` | Global built-in | Unbuffered standard error writer (`@io.Stderr` with no-op `Flush()`). |
| `scriptName` | `String` | Module-local | File path of the currently executing module / script. Differs from `ProgramName` when evaluated inside an imported module. |
| `scriptLine` | `Int` | Lexical / Macro | 1-based source line number where `scriptLine` appears (equivalent to C's `__LINE__`). Expanded at parse time. |

**Example**:
```python
print "Running program: ", ProgramName, "\n"
print "This file: ", scriptName, " line: ", scriptLine, "\n"
print "Argument count: ", len(Args), "\n"

if len(Args) > 0:
    print "First arg: ", Args[0], "\n"
```

### 3. Built-in Methods on Primitive Types

Primitive types provide predefined instance methods, indexing mechanisms, length support, and operators, as well as implementations of the `Show` interface.

#### `String`
- **Methods**:
  - `s.Ord(): Int`: Returns the Unicode code point of the first rune (character) in `s`. Calling `Ord()` on an empty string (`"".Ord()`) raises a runtime error.
    ```python
    "A".Ord()        # 65
    "あ".Ord()       # 12354
    "Hello".Ord()    # 72 (first character 'H')
    ```
  - `s.Bytes(): Bytes`: Returns the UTF-8 byte sequence of `s` as an immutable `Bytes` value.
    ```python
    b = "hello".Bytes()  # Bytes with 5 bytes
    ```
  - `s.ToString(): String`: Returns `s` itself. Satisfies `interface Show`.
- **Index Access**:
  - `s[i]: String`: 0-based Unicode rune (code point) indexing, returning a 1-rune `String`. Supports negative indexing (`s[-1]` is the last rune). Out-of-bounds index raises a runtime error.
- **Length**:
  - `len(s): Int`: Returns the number of runes (Unicode code points) in `s`.
- **Operators**:
  - `s1 + s2 -> String`: Concatenates two strings.
  - `==`, `!=`, `<`, `<=`, `>`, `>=`: Lexicographical comparison based on Unicode code points.

#### `Int`
- **Methods**:
  - `n.Chr(): String`: Returns a 1-rune string corresponding to the Unicode code point `n`. Calling `Chr()` on a negative number, a value above `0x10FFFF`, or a surrogate code point (`0xD800`–`0xDFFF`) raises a runtime error.
    ```python
    65.Chr()         # "A"
    12354.Chr()      # "あ"
    (97).Chr()       # "a"
    ```
  - `n.ToString(): String`: Returns the decimal string representation of `n` (e.g., `"42"`). Satisfies `interface Show`.
- **Operators**:
  - Arithmetic: `+`, `-`, `*`, `/` (integer division; division by zero raises a runtime error), `%` (modulo).
  - Unary: `-n` (negation), `+n`.
  - Comparison: `==`, `!=`, `<`, `<=`, `>`, `>=`.

#### `Float`
- **Methods**:
  - `f.ToString(): String`: Returns the string representation of `f` (e.g., `"3.14"`). Satisfies `interface Show`.
- **Operators**:
  - Arithmetic: `+`, `-`, `*`, `/` (IEEE 754 floating-point operations).
  - Unary: `-f`, `+f`.
  - Comparison: `==`, `!=`, `<`, `<=`, `>`, `>=`.

#### `Bool`
- **Methods**:
  - `b.ToString(): String`: Returns `"true"` or `"false"`. Satisfies `interface Show`.
- **Operators**:
  - Logical: `&&` (short-circuit logical AND), `||` (short-circuit logical OR), `!` (logical NOT).
  - Comparison: `==`, `!=`.
  - Ternary: `cond ? then_val : else_val`.

#### `Nil`
- **Methods**:
  - `nil.ToString(): String`: Returns `"nil"`. Satisfies `interface Show`.
- **Operators**:
  - Comparison: `==`, `!=` (commonly used for nullable union types, e.g. `val == nil` or `val is Nil`).

#### `Bytes`
- **Methods**:
  - `b.Bytes(): Bytes`: Returns the byte sequence itself (identity conversion).
  - `b.String(): String`: Decodes the byte sequence as a UTF-8 string. If `b` contains invalid UTF-8 byte sequences, raises a runtime error.
  - `b.IsValidString(): Bool`: Returns `true` if `b` is a valid UTF-8 sequence, `false` otherwise (useful to check before calling `b.String()`).
  - `b.ToString(): String`: Returns the lowercase hexadecimal string representation (e.g., `"68656c6c6f"`). Satisfies `interface Show`, so `print b` outputs this hexadecimal format.
- **Index Access**:
  - `b[i]: Int`: Returns the single byte at offset `i` as an `Int` in the range `0`–`255`. Supports negative indexing (`b[-1]` is the last byte). Out-of-bounds index raises a runtime error.
- **Length**:
  - `len(b): Int`: Returns the number of bytes in `b`.
- **Operators**:
  - `b1 + b2 -> Bytes`: Concatenates two `Bytes` values.
  - `==`, `!=`, `<`, `<=`, `>`, `>=`: Lexicographical comparison.

#### `Tuple`
- **Methods**:
  - `t.ToString(): String`: Returns the string representation of `t` (e.g., `"(1, 2)"`, `"()"`). Satisfies `interface Show`.
- **Index Access**:
  - `t[i]: Any`: 0-based element indexing. Supports negative indexing (`t[-1]` is the last element). Out-of-bounds index raises a runtime error.
- **Length**:
  - `len(t): Int`: Returns the number of elements in `t`.
- **Operators & Destructuring**:
  - `==`, `!=`: Structural element-wise equality comparison.
  - Pattern matching / unpacking: `(a, b) = t`, `(first, *rest) = t`.

#### `Dict`
- **Methods**:
  - `d.ToString(): String`: Returns the string representation of `d` (e.g., `{"a": 1}`). Satisfies `interface Show`.
- **Key Access**:
  - `d[key]: Value`: Returns the value associated with `key`. Non-existent key lookup raises a runtime error.
- **Length**:
  - `len(d): Int`: Returns the number of key-value pairs in `d`.
- **Operators**:
  - `==`, `!=`: Structural equality comparison of all key-value entries.

### 4. Built-in Interfaces

Les provides predefined standard interfaces in the global scope for polymorphism, string conversion, and error handling:

#### `interface Show`
Declares the contract for converting values to human-readable string representations:
```python
interface Show:
    def ToString(): String
```
- **Implicit Satisfaction**: All primitive types (`String`, `Int`, `Float`, `Bool`, `Nil`, `Bytes`, `Tuple`, `Dict`) implicitly satisfy `interface Show`.
- **Usage**: Built-in functions like `print` accept any value satisfying `Show` (`*args: Show`). Any user-defined `struct` implementing `ToString(): String` automatically satisfies `Show`.

#### `interface Error`
Declares the contract for error values:
```python
interface Error:
    def Message(): String
    def ToString(): String
```
- **Implicit Satisfaction**: Standard error instances created via the built-in `Error(msg)` function satisfy `Error`. In addition, any user-defined `struct` implementing `Message(): String` (or `Error(): String`) automatically satisfies `Error` structurally without explicit declarations.
- **Usage**: Extensively used in union return types (`Type | Error`) and type-narrowing expressions (`val is Error` or `match val: case Error:`).

### 5. Built-in Modules

#### Standard I/O Module (`@io`)

The built-in `@io` module provides stream I/O abstractions, interfaces, and standard stream access.

```python
import io
```

##### Core Interfaces
- **`Reader`**: `Read(n: Int): Bytes | Nil`
- **`Writer`**: `Write(b: Bytes): Int`, `Flush(): ()`
- **`Closer`**: `Close(): ()`
- **`ReadCloser`**: embeds `Reader`, `Closer` (`Read`, `Close`)
- **`WriteCloser`**: embeds `Writer`, `Closer` (`Write`, `Flush`, `Close`)

##### Standard Streams
- **`@io.Stdin`**: Standard input instance satisfying `Reader`, `Closer`, and `ReadCloser`.
  - `Read(n: Int): Bytes | Nil`: reads up to `n` bytes as a `Bytes` sequence (`n == 0` returns empty `Bytes()`, EOF returns `nil`).
  - `Close(): ()`: closes standard input.
- **`@io.Stdout`**: Standard output instance satisfying `Writer`, `Closer`, and `WriteCloser`.
  - `Write(b: Bytes): Int`: writes byte sequence `b` directly to standard output (automatically flushes any buffered `Stdout` first).
  - `Flush(): ()`: flushes standard output.
  - `Close(): ()`: closes standard output (automatically flushes buffered `Stdout` first).
- **`@io.Stderr`**: Standard error instance satisfying `Writer`, `Closer`, and `WriteCloser`.
  - `Write(b: Bytes): Int`: writes byte sequence `b` directly to standard error without buffering (automatically flushes any buffered `Stdout` first).
  - `Flush(): ()`: provides a no-op `Flush(): ()` for `Writer` / `bufio.Writer` interface compatibility.
  - `Close(): ()`: closes standard error.

#### Buffered I/O Module (`@bufio`)

The built-in `@bufio` module wraps stream I/O objects with buffering, line reading, and flushing capabilities.

```python
import bufio
```

##### Structs & Constructors
- **`@bufio.Reader(r: Reader): Reader`**: Constructs a buffered reader wrapping an underlying `Reader` (such as `@io.Stdin` or a custom struct implementing `Read`). If `r` is already a `@bufio.Reader`, returns it directly.
  - `Read(n: Int): Bytes | Nil`: Reads up to `n` bytes from buffer/stream. `n == 0` returns empty `Bytes()`; `n < 0` produces a runtime error.
  - `ReadLine(): String | Nil`: Reads a single line as UTF-8 string, stripping trailing `\n` or `\r\n`.
  - `ReadString(delim: String): String | Nil`: Reads until the given delimiter string (supports multi-character delimiters). Returns accumulated text as UTF-8 string including delimiter; if EOF is reached before the delimiter, returns all accumulated trailing text up to EOF; returns `nil` only if EOF is encountered with 0 bytes read.
  - `Peek(n: Int): Bytes | Nil`: Peeks ahead up to `n` bytes without advancing the reader cursor. `n == 0` returns empty `Bytes()`; `n < 0` produces a runtime error.
- **`@bufio.Writer(w: Writer): Writer`**: Constructs a buffered writer wrapping an underlying `Writer` (such as `@io.Stdout` or a custom struct implementing `Write`). Automatically tracked and flushed on program exit or `exit()`. If `w` is already a `@bufio.Writer`, returns it directly.
  - `Write(b: Bytes): Int`: Appends byte sequence `b` to write buffer.
  - `Flush(): ()`: Flushes buffered data to the underlying stream.

## Planned Features / Future Work

### 1. Regular Expression Type (`Regex`) & `/pattern/flags` Literal

A first-class `Regex` (or `Regexp`) type with `/pattern/flags` literal syntax is planned for rich text processing.

- **Syntax**: `/pattern/flags` (e.g., `/[a-zA-Z0-9_-]+/`, `/[a-z]+/i`)
- **Lexer / Parser Disambiguation**:
  - Disambiguate between division operator (`/`) and regex literal start (`/.../`) using preceding token context:
    - Preceded by an expression operand (ident, literal, `)`, `]`, `}`): parsed as division `/`.
    - Preceded by an operator, punctuation, or statement start (e.g., `=`, `(`, `[`, `{`, `,`, `:`, `case`, `return`, `do`): parsed as regex literal `/.../`.
- **Pattern Matching (`match` / `case`) Integration**:
  - Seamless matching and capture extraction in `match` statements:
    ```python
    match line:
        case /^GET\s+(\S+)/:
            # Capture groups extraction / handling
            print "Matched GET"
        case /^POST\s+(\S+)/:
            print "Matched POST"
        case _:
            print "Unknown"
    ```
- **Methods & Type System**:
  - Native regex matching operations (`s =~ /re/` or `s is /re/`, `re.Find(s)`, `re.Replace(s, rep)`).
  - Implicitly satisfies the `Show` interface for printing.

### 2. Arbitrary-Precision Integers (Multi-precision `Int`)

`Int` will be upgraded from a fixed 64-bit signed integer to an arbitrary-precision integer (multi-precision / big integer backed by Go's `math/big.Int`).

- **Infinite Magnitude**: `Int` will handle arbitrarily large integers up to available memory limits without overflow or wrap-around (similar to Python 3 and Ruby).
- **Unified `Int` Type**: Users continue writing `Int` without needing a distinct `BigInt` type. Fast-path optimizations (e.g. 64-bit int fast-path for small values) will be handled transparently under the hood.
- **Seamless Interop**: Arithmetic operators (`+`, `-`, `*`, `/`, `%`), string formatting, bitwise operations, and Hindley-Milner type inference will continue to work seamlessly without code changes.

### 3. Hexadecimal Floating-Point Literals

- Exact IEEE 754 representation using hexadecimal floats with binary exponents (`p` / `P` power of 2):
  - Syntax: `0x1.0p0`, `0xAB.CDEFp4`, `0x1.fp-3`
  - Disambiguation strategy: Since built-in types have a closed set of methods (no user monkey-patching), the lexer attempts to interpret `0x...` followed by `.` as a floating-point literal first. If it satisfies float syntax, it is treated as a float; otherwise, it falls back to integer dot/method access.
- **Built-in Methods on Numeric Literals & Disambiguation**:
  - Direct method invocation on literals (e.g., `42.ToString()`, `3.14.ToString()`).
  - Decimal floats require a digit immediately following the dot (`42.0`), whereas a dot followed by an identifier letter (`42.ToString()`) is disambiguated as a method call on integer `42`.

### 4. String Interpolation (`#{}`)

Ruby-style string interpolation within double-quoted strings:

- **Double-quoted Strings (`"..."`)**: Expressions inside `#{...}` are evaluated and converted to strings via their `ToString()` method (satisfying `Show`):
  ```python
  name = "Alice"
  age = 30
  print "User: #{name}, Age: #{age}\n"
  print "Result: #{1 + 2 * 3}\n"
  ```
- **Single-quoted Strings (`'...'`)**: Raw / literal strings without expression interpolation; `#{expr}` is treated literally as text:
  ```python
  print 'Literal #{name}\n'  # prints "Literal #{name}\n"
  ```

### 5. Null-Safe Navigation (`?.`) & Coalescing Operator (`??`)

Syntactic sugar for handling `Nil` and nullable union types (`Type | Nil`):

- **Safe Navigation (`?.`)**: Safely navigates fields or methods without runtime errors if the target is `nil`, evaluating to `nil`:
  ```python
  user?.Profile?.Name
  ```
- **Null Coalescing (`??`)**: Fallback expression if the left operand is `nil` (equivalent to `a != nil ? a : b`):
  ```python
  displayName = user?.Name ?? "Guest"
  ```

### 6. Bitwise Operators

Bitwise operations for integers:

- Binary operations: AND (`&`), XOR (`^`), left shift (`<<`), right shift (`>>`), NOT (`~`).
- Bitwise OR: Disambiguated or dedicated operator syntax avoiding grammatical collision with union types (`A | B`) and lambda argument delimiters (`|x|:`).

### 7. User-Defined Generics (Type Parameters `[T]`)

Extending Hindley-Milner type inference to user-defined parameterized structs and functions:

- Syntax: `struct Box[T]: value: T`, `def map[T, U](items: (T, ...), fn: (T) -> U): (U, ...)`
- Complements the existing built-in type parameters like `{K: V}` and tuple stream types `(T, ...)`.

## License

2-Clause BSD License

```
Copyright (c) 2026, U.Nakamura <usa@garbagecollect.jp>
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice, this
   list of conditions and the following disclaimer.

2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
```

## Contributions

By submitting contributions (via pull requests, email, or any other means) to this project:

1. **Copyright Ownership**: You retain the copyright to your contributed code.
2. **Grant of Rights**: You grant the project owner (U.Nakamura) an unrestricted, perpetual, irrevocable, worldwide, royalty-free license to use, copy, modify, merge, distribute, sublicense, and relicense your contributions.
3. **Non-infringement**: You represent and warrant that your contributions are your original work and do not infringe upon any third party's copyright, patent, or other intellectual property rights.
