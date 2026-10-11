# Les Language Specification & Plan

## Overview
Les (Lazy Evaluation Script) is a statically-typed, aggressively lazy-evaluated scripting language implemented in Go.

## Built-in Types

| Type | Syntax / Examples | Description |
|---|---|---|
| `Int` | `42`, `-10`, `0` | 64-bit signed integer. |
| `Float` | `3.14`, `-0.5`, `2.0` | 64-bit IEEE 754 floating-point number. |
| `String` | `"hello"`, `'world'` | Immutable UTF-8 encoded string. Indexed by 0-based rune (Unicode code point) index `s[0]`, returning a 1-rune `String`. Supports negative index `s[-1]` (last rune). |
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
- Tuples: `()`, `(1,)`, `(1, 2)`.
- Dicts: `{"a": 1}`.

#### Operators and Precedence
Listed from highest to lowest precedence:

| Precedence | Operator | Syntax / Example | Types | Description |
|---|---|---|---|---|
| 1 (Highest) | `.` | `p.x`, `p.dist()` | `StructInstance` | Member / method access (with promotion) |
| 1 | `[]` | `s[0]`, `t[1]`, `d["k"]` | `String`, `Tuple`, `Dict` | Indexing (0-based rune for String, negative indices supported) |
| 1 | `()` | `f(a, b)` | Function / Struct | Function call or struct constructor |
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
- `Dict`: `d[key]` accesses value by key (`d["a"]`). Non-existent key raises a runtime error.

### 6. Side Effects
- Demand-driven strict evaluation at built-in effect points (e.g. `print`, `puts`, `exit`).
- Top-level `return` acts as `exit`, so it also serves as a demand point forcing evaluation of its exit code expression.
- Inside functions/closures, `return` and `break` are NOT side-effect points; values are carried lazily until demanded.
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

### 9. Built-in Variables & CLI Arguments

Les provides predefined variables for command-line arguments and file path inspection:

| Variable | Type | Scope | Description |
|---|---|---|---|
| `Args` | `(String, ...)` | Global built-in | Immutable tuple of command-line arguments passed to the script (excluding the script name). Empty tuple `()` if no arguments were provided. |
| `ProgramName` | `String` | Global built-in | Path or name of the main program file executed from CLI (equivalent to Ruby's `$0` or Python's `sys.argv[0]`). |
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

### 10. Special Types: `Nil` and `Any`
- **`Nil`**:
  - Represents the absence of a value (both as the literal `Nil` and the type `Nil`).
  - Useful in union types such as `Int | Nil` (nullable/optional semantics).
- **`Any`**:
  - Safe top type, analogous to Go's `any` or TypeScript's `unknown`.
  - Any value can be assigned to `Any`, but values typed as `Any` cannot be used directly in operations without type refinement via `is` or `match`.

### 11. Variadic Arguments & Tuple Unpacking

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
  - With `True`: code `0`; with `False`: code `1`.

### 12. Structs (`struct`)

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

    def dist_sq():
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

#### Dot Access
- Member access: `p.x`, `p.dist_sq()`
- Struct fields are lazily evaluated thunks.

#### Go-style Struct Embedding & Promotion
- Structs can embed other structs anonymously by listing their type name in the struct body.
- Embedded fields and methods are automatically promoted to the outer struct.
- An embedded type also participates in subtyping: an outer instance satisfies `is Inner`.

```python
struct Reader:
    def read(): "read data"

struct ReadCloser:
    Reader
    def close(): "closed"

rc = ReadCloser()
print rc.read()         # "read data" (promoted method)
print rc is Reader      # True
```

#### Static Ambiguous Selector Detection
When multiple embedded structs define the same member (field or method), Les statically reports a type error (`ambiguous selector`) at type-checking / inference time unless the outer struct explicitly defines/overrides the member.

### 13. Interfaces (`interface`) & Structural Typing

Les provides Go-style implicit structural typing (duck typing) via interface declarations.

#### Definition
- Defined using `interface <Name>:` followed by an indented block.
- Contains only method signatures: `<name>(<params...>): <ReturnType>`.
- Receiver (`self`) is omitted from interface method signatures.
- Interfaces can embed other interfaces.

```python
interface Reader:
    read(n: Int): String

interface Closer:
    close(): ()

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

    def read(n: Int):
        "read bytes"

    def close():
        ()

# Statically assignable to ReadCloser
rc: ReadCloser = File(path: "/tmp/data")
```

#### Composition & Conflict Resolution
- Embedding multiple interfaces that declare identical method signatures is valid (unified into one).
- Conflicting signatures (same method name but differing parameter or return types) produce a static compilation / type error at interface definition time.

#### Dynamic Runtime Checks (`is`)
- `instance is InterfaceName` dynamically checks whether the target value implements all required methods (including embedded interfaces).


