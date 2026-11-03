# Les Standard Library & Built-in Reference

This document provides a comprehensive reference for the built-in functions, predefined variables, primitive type methods, core interfaces, and standard library modules of the Les programming language.

---

## Table of Contents
1. [Built-in Functions](#1-built-in-functions)
   - [`print(*args: Show)`](#def-printargs-show-)
   - [`exit(code: Int | Bool = 0)`](#def-exitcode-int--bool--0)
   - [`gets(): String | Nil`](#def-gets-string--nil)
2. [Built-in Variables & CLI Arguments](#2-built-in-variables--cli-arguments)
3. [Built-in Methods on Primitive Types](#3-built-in-methods-on-primitive-types)
   - [`String`](#string)
   - [`Int`](#int)
   - [`Float`](#float)
   - [`Bool`](#bool)
   - [`Nil`](#nil)
   - [`Bytes`](#bytes)
   - [`Tuple`](#tuple)
   - [`Dict`](#dict)
   - [`Regex`](#regex)
   - [`Match`](#match)
4. [Built-in Interfaces](#4-built-in-interfaces)
   - [`interface Show`](#interface-show)
   - [`interface Error`](#interface-error)
   - [`interface Indexable[K, V]`](#interface-indexablek-v)
   - [`interface Equatable[T]`](#interface-equatablet)
   - [`interface Comparable[T]`](#interface-comparablet)
   - [`interface Numeric[T]`](#interface-numerict)
5. [Built-in Modules](#5-built-in-modules)
   - [Standard I/O Module (`io`)](#standard-io-module-io)
   - [Buffered I/O Module (`bufio`)](#buffered-io-module-bufio)

---

## 1. Built-in Functions

### `def print(*args: Show): ()`
Outputs string representations of all arguments directly without inserting spaces or appending a trailing newline.

- **Effect**: Built-in demand boundary; forces evaluation of lazy arguments (`Thunk`s).
- **Arguments**: `*args: Show` — variable number of values satisfying the `Show` interface (`Inspect(): String`).
- **Returns**: `()` (empty tuple).
- **Usage**:
  ```python
  print "Hello, world!\n"
  print "a", "b", 42  # outputs "ab42" without space or newline
  ```

### `def exit(code: Int | Bool = 0)`
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

### `def gets(): String | Nil`

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

---

## 2. Built-in Variables & CLI Arguments

Les provides predefined variables for command-line arguments and file path inspection:

| Variable | Type | Scope | Description |
|---|---|---|---|
| `Args` | `(String, ...)` | Global built-in | Immutable tuple of command-line arguments passed to the script (excluding the script name). Empty tuple `()` if no arguments were provided. |
| `ProgramName` | `String` | Global built-in | Path or name of the main program file executed from CLI (equivalent to Ruby's `$0` or Python's `sys.argv[0]`). |
| `Stdin` | `Reader` | Global built-in | Buffered standard input reader (`bufio::Reader` wrapping `io::Stdin`). |
| `Stdout` | `Writer` | Global built-in | Buffered standard output writer (`bufio::Writer` wrapping `io::Stdout`). |
| `Stderr` | `Writer` | Global built-in | Unbuffered standard error writer (`io::Stderr` with no-op `Flush()`). |
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

---

## 3. Built-in Methods on Primitive Types

Primitive types provide predefined instance methods, indexing mechanisms, length support, and operators, as well as implementations of the `Show` interface.

### `String`

- `=~(r: Regex): Match | Nil`: Matches string against regular expression `r`, returning `Match` or `nil`.
- **Methods**:
  - `s.Len(): Int`: Returns the number of runes (Unicode code points) in `s`.
  - `s.Type(): String`: Returns `"String"`. Satisfies `interface Show`.
  - `s.Ord(): Int`: Returns the Unicode code point of the first rune (character) in `s`. Calling `Ord()` on an empty string (`"".Ord()`) raises a runtime error.
    ```python
    "A".Ord()        # 65
    "あ".Ord()       # 12354
    "Hello".Ord()    # 72 (first character 'H')
    ```
  - `s.Int([radix: Int]): Int`: Parses the integer value from `s` using base `radix` (optional, default: `10`; supported: `2`, `8`, `10`, `16`). Parsing scans characters after an optional leading `+` or `-` sign, stopping at the first invalid digit for the radix (ignoring trailing characters and prefixes like `0x`). Returns `0` if no valid digits are found. Specifying an unsupported radix raises a runtime error.
    ```python
    "123".Int()        # 123
    "123a".Int()       # 123 (stops at non-digit 'a')
    "123a".Int(16)     # 4666 (0x123a)
    "0x".Int()         # 0 (stops at 'x')
    "1010".Int(2)      # 10
    "-42abc".Int()     # -42
    ```
  - `s.Bytes(): Bytes`: Returns the UTF-8 byte sequence of `s` as an immutable `Bytes` value.
    ```python
    b = "hello".Bytes()  # Bytes with 5 bytes
    ```
  - `s.Inspect(): String`: Returns `s` enclosed in double quotes with escape sequences according to LES string literal syntax (e.g., `"\"hello\""`). Satisfies `interface Show`.
- **Index Access**:
  - `s[i]: String`: 0-based Unicode rune (code point) indexing, returning a 1-rune `String`. Supports negative indexing (`s[-1]` is the last rune). Out-of-bounds index raises a runtime error.
- **Operators**:
  - `s1 + s2 -> String`: Concatenates two strings.
  - `==`, `!=`, `<`, `<=`, `>`, `>=`: Lexicographical comparison based on Unicode code points.

### `Int`
- **Methods**:
  - `n.Type(): String`: Returns `"Int"`. Satisfies `interface Show`.
  - `n.Float(): Float`: Returns the 64-bit floating-point representation of `n`. For large multi-precision integers exceeding the `Float` range, returns positive or negative infinity.
  - `n.Int(): Int`: Returns `n` (identity conversion).
  - `n.Abs(): Int`: Returns the absolute value of `n`. If `n == math.MinInt64`, it is automatically promoted to an arbitrary-precision integer.
  - `n.Floor(): Int`: Returns `n` (identity rounding).
  - `n.Ceil(): Int`: Returns `n` (identity rounding).
  - `n.Round(): Int`: Returns `n` (identity rounding).
  - `n.Chr(): String`: Returns a 1-rune string corresponding to the Unicode code point `n`. Calling `Chr()` on a negative number, a value above `0x10FFFF`, or a surrogate code point (`0xD800`–`0xDFFF`) raises a runtime error.
    ```python
    65.Chr()         # "A"
    12354.Chr()      # "あ"
    (97).Chr()       # "a"
    ```
  - `n.String([radix: Int]): String`: Returns the string representation of integer `n` using base `radix` (optional, default: `10`; supported: `2`, `8`, `10`, `16`). Base 16 uses lowercase characters `a`–`f`. Negative numbers include a leading `-`. Specifying an unsupported radix raises a runtime error.
    ```python
    123.String()       # "123"
    255.String(16)     # "ff"
    10.String(2)       # "1010"
    493.String(8)      # "755"
    (-42).String()     # "-42"
    ```
  - `n.Inspect(): String`: Returns the decimal string representation of `n` (e.g., `"42"`). Satisfies `interface Show`.
- **Operators**:
  - Arithmetic: `+`, `-`, `*`, `/` (integer division; division by zero raises a runtime error), `%` (modulo).
  - Unary: `-n` (negation), `+n`, `~n` (bitwise NOT).
  - Bitwise: `&` (AND), `|` (OR), `^` (XOR), `<<` (arithmetic left shift), `>>` (arithmetic right shift). Shifts follow Ruby semantics (negative shift reverses direction; shifts exceeding $2^{26}$ bits raise a runtime error).
  - Comparison: `==`, `!=`, `<`, `<=`, `>`, `>=`.

### `Float`
- **Methods**:
  - `f.Type(): String`: Returns `"Float"`. Satisfies `interface Show`.
  - `f.Int(): Int`: Converts `f` to an integer, truncating fractional digits towards zero (Ruby-style `.to_i`). Exceeding standard 64-bit integer range produces an arbitrary-precision integer (`IntValue`). Calling `Int()` on NaN or infinity raises a runtime error.
  - `f.Float(): Float`: Returns `f` (identity conversion).
  - `f.Abs(): Float`: Returns the absolute value of `f`.
  - `f.Floor(): Int`: Returns the largest integer less than or equal to `f` (Ruby-style). Calling `Floor()` on NaN or infinity raises a runtime error.
  - `f.Ceil(): Int`: Returns the smallest integer greater than or equal to `f` (Ruby-style). Calling `Ceil()` on NaN or infinity raises a runtime error.
  - `f.Round(): Int`: Returns the integer nearest to `f`, rounding half away from zero. Calling `Round()` on NaN or infinity raises a runtime error.
  - `f.IsNaN(): Bool`: Returns `true` if `f` is IEEE 754 NaN (Not-a-Number), `false` otherwise.
  - `f.IsInfinite(): Bool`: Returns `true` if `f` is positive or negative infinity, `false` otherwise.
  - `f.IsFinite(): Bool`: Returns `true` if `f` is a finite number (neither NaN nor infinity), `false` otherwise.
  - `f.Inspect(): String`: Returns the string representation of `f` (e.g., `"3.14"`). Satisfies `interface Show`.
- **Operators**:
  - Arithmetic: `+`, `-`, `*`, `/` (IEEE 754 floating-point operations).
  - Unary: `-f`, `+f`.
  - Comparison: `==`, `!=`, `<`, `<=`, `>`, `>=`.

### `Bool`
- **Methods**:
  - `b.Type(): String`: Returns `"Bool"`. Satisfies `interface Show`.
  - `b.Inspect(): String`: Returns `"true"` or `"false"`. Satisfies `interface Show`.
- **Operators**:
  - Logical: `&&` (short-circuit logical AND), `||` (short-circuit logical OR), `!` (logical NOT).
  - Comparison: `==`, `!=`.
  - Ternary: `cond ? then_val : else_val`.

### `Nil`
- **Methods**:
  - `nil.Type(): String`: Returns `"Nil"`. Satisfies `interface Show`.
  - `nil.Inspect(): String`: Returns `"nil"`. Satisfies `interface Show`.
- **Operators**:
  - Comparison: `==`, `!=` (commonly used for nullable union types, e.g. `val == nil` or `val is Nil`).

### `Bytes`
- **Methods**:
  - `b.Len(): Int`: Returns the number of bytes in `b`.
  - `b.Type(): String`: Returns `"Bytes"`. Satisfies `interface Show`.
  - `b.Bytes(): Bytes`: Returns the byte sequence itself (identity conversion).
  - `b.String(): String`: Decodes the byte sequence as a UTF-8 string. If `b` contains invalid UTF-8 byte sequences, raises a runtime error.
  - `b.IsValidString(): Bool`: Returns `true` if `b` is a valid UTF-8 sequence, `false` otherwise (useful to check before calling `b.String()`).
  - `b.Inspect(): String`: Returns the lowercase hexadecimal string representation (e.g., `"68656c6c6f"`). Satisfies `interface Show`, so `print b` outputs this hexadecimal format.
- **Index Access**:
  - `b[i]: Int`: Returns the single byte at offset `i` as an `Int` in the range `0`–`255`. Supports negative indexing (`b[-1]` is the last byte). Out-of-bounds index raises a runtime error.
- **Operators**:
  - `b1 + b2 -> Bytes`: Concatenates two `Bytes` values.
  - `==`, `!=`, `<`, `<=`, `>`, `>=`: Lexicographical comparison.

### `Tuple`
- **Syntax**:
  - `()` creates an empty tuple (type `()`).
  - `(x,)` creates a single-element tuple (trailing comma required to distinguish from grouping `(x)`). Single-element tuple type annotations are written as `(T,)`.
  - `(x1, x2, ...)` creates a multi-element tuple (e.g. `(1, 2)`).
  - `(a, ...t, b)`: Spread syntax expands elements of tuple `t` inline into a new tuple literal without forcing lazy thunks.
  - Type annotations: `(...)` represents an arbitrary tuple type of any length and element types.
- **Methods**:
  - `t.Append(elem: T): (...)`: Returns a new tuple with `elem` appended as the final element (does not flatten; passing a tuple nests it).
  - `t.Prepend(elem: T): (...)`: Returns a new tuple with `elem` prepended as the first element.
  - `t.Concat(...others: (...)): (...)`: Concatenates all argument tuples to `t` and returns a new tuple.
  - `t.Slice(start: Int, end: Int = -1): (...)`: Extracts sub-tuple from `start` index up to `end` index (supports negative indexing and out-of-bounds clamping on finite tuples; negative index on an infinite tuple raises a runtime error).
  - `t.Len(): Int`: Returns the number of elements in `t`. For infinite tuples (streams), returns negative integers based on explicit prefix element count before `...`: `-(k + 1)` where `k` is the prefix length (e.g. `(...)` -> `-1`, `(1, ...)` -> `-2`, `(1, 2, ...)` -> `-3`).
  - `t.Type(): String`: Returns the string representation of tuple type (e.g., `"(Int, String)"`). Satisfies `interface Show`.
  - `t.Inspect(): String`: Returns the string representation of `t` (e.g., `"(1, 2)"`, `"()"`). Satisfies `interface Show`.
- **Index Access**:
  - `t[i]`: 0-based element indexing. Supports negative indexing (`t[-1]` is the last element) on finite tuples. Attempting negative index on an infinite tuple raises a runtime error (`cannot use negative index on infinite tuple`). Out-of-bounds index raises a runtime error.
- **Operators & Destructuring**:
  - `t1 + t2`: Concatenates two tuples (equivalent to `t1.Concat(t2)`).
  - `t << elem`: Appends `elem` to `t` (equivalent to `t.Append(elem)`).
  - `==`, `!=`: Structural element-wise equality comparison.
  - Pattern matching / unpacking: `(a, b) = t`, `(first, *rest) = t`.

### `Dict`
- **Type Annotations**:
  - `{...}` represents an arbitrary dictionary type of any key and value types.
- **Methods**:
  - `d.Len(): Int`: Returns the number of key-value pairs in `d`.
  - `d.Type(): String`: Returns the string representation of dict type (e.g., `"{String: Int}"`, `"{Unknown: Unknown}"`). Satisfies `interface Show`.
  - `d.Inspect(): String`: Returns the string representation of `d` (e.g., `{"a": 1}`). Satisfies `interface Show`.
- **Key Access**:
  - `d[key]: Value`: Returns the value associated with `key`. Non-existent key lookup raises a runtime error.
- **Operators**:
  - `==`, `!=`: Structural equality comparison of all key-value entries.

### `Regex`
- **Methods**:
  - `r.Pattern(): String`: Returns the underlying pattern string.
  - `r.Flags(): String`: Returns the compiled flag string (e.g. `'i'`).
  - `r.Test(s: String): Bool`: Returns `true` if regex matches target string `s`.
  - `r.Match(s: String): Match | Nil`: Matches `s`, returning `Match` on success or `Nil`.
  - `r.FindAll(s: String, count: Int = -1): (Match, ...)`: Returns a tuple of all non-overlapping matches.
  - `r.Replace(s: String, repl: String): String`: Replaces the first match with replacement string `repl`.
  - `r.ReplaceAll(s: String, repl: String): String`: Replaces all matches with `repl`.
  - `r.Split(s: String, count: Int = -1): (String, ...)`: Splits `s` by regex matches into a tuple of strings.
  - `r.Inspect(): String`: Returns regex string representation. Satisfies interface `Show`.
  - `r.Type(): String`: Returns `'Regex'`. Satisfies interface `Show`.
- **Operators**:
  - `s =~ r`, `r =~ s`: Matches string and regex, returning `Match | Nil`.
  - `==`, `!=`: Checks equality of pattern and flags.

### `Match`
- **Methods**:
  - `m.Len(): Int`: Returns number of capture groups including group 0 (full match).
  - `m.Group(idxOrName: Int | String): String | Nil`: Returns captured string by 0-based index, negative index, or group name; returns `Nil` if unmatched.
  - `m.Groups(): (String | Nil, ...)`: Returns tuple of capture groups (excluding group 0).
  - `m.NamedGroups(): {String: String | Nil}`: Returns dictionary of all named capture groups.
  - `m.Start(idxOrName: Int | String = 0): Int`: Returns 0-based start rune offset, or `-1` if unmatched.
  - `m.End(idxOrName: Int | String = 0): Int`: Returns 0-based end rune offset, or `-1` if unmatched.
  - `m.Span(idxOrName: Int | String = 0): (Int, Int)`: Returns `(Start, End)` rune offset tuple, or `(-1, -1)`.
  - `m.Text(): String`: Returns full matched string (same as `m[0]`).
  - `m.Target(): String`: Returns original target string searched.
  - `m.Regex(): Regex`: Returns compiled Regex object that produced this match.
  - `m.Inspect(): String`: Returns human-readable representation. Satisfies interface `Show`.
  - `m.Type(): String`: Returns `'Match'`. Satisfies interface `Show`.
- **Index Access**:
  - `m[i]`: 0-based capture group index (`m[0]` is full match, `m[1..n]` are captures). Supports negative indexing (`m[-1]`). Implements `Indexable[Int, String | Nil]`.
  - `m["name"]`: Named capture group access. Implements `Indexable[String, String | Nil]`.
- **Operators**:
  - `==`, `!=`: Compares target, regex pattern/flags, and capture spans.

---

## 4. Built-in Interfaces

Les provides predefined standard interfaces in the global scope for polymorphism, string conversion, and error handling:

### `interface Show`
Declares the contract for converting values to human-readable string representations and inspecting their runtime type:
```python
interface Show:
    def Inspect(): String
    def Type(): String
```
- **Implicit Satisfaction**: All primitive types (`String`, `Int`, `Float`, `Bool`, `Nil`, `Bytes`, `Tuple`, `Dict`, `Regex`, `Match`), functions, and struct instances implicitly satisfy `interface Show`.
- **Usage**: Built-in functions like `print` accept any value satisfying `Show` (`*args: Show`). Any user-defined `struct` implementing `Inspect(): String` automatically satisfies `Show`.

### `interface Error`
Declares the contract for error values:
```python
interface Error:
    def Message(): String
    def Inspect(): String
```
- **Implicit Satisfaction**: Standard error instances created via the built-in `Error(msg)` function satisfy `Error`. In addition, any user-defined `struct` implementing `Message(): String` (or `Error(): String`) automatically satisfies `Error` structurally without explicit declarations.
- **Usage**: Extensively used in union return types (`Type | Error`) and type-narrowing expressions (`val is Error` or `match val: case Error:`).

### `interface Indexable[K, V]`
Declares the contract for types supporting index-based element access and length inspection:
```python
interface Indexable[K, V]:
    def [](key: K) -> V
    def Len(): Int
```
- **Implicit Satisfaction**: Built-in collections (`Tuple`, `Dict`, `String`, `Bytes`) and user-defined structs with index operator `def [](key)` and `def Len(): Int` satisfy `Indexable[K, V]`.
  - `(Int, String)` satisfies `Indexable[Int, Int | String]`
  - `(1, ...)` satisfies `Indexable[Int, Int]`
  - `{String: Int}` satisfies `Indexable[String, Int]`
  - `String` satisfies `Indexable[Int, String]`
  - `Bytes` satisfies `Indexable[Int, Int]`
- **Unspecialized Form**: `val is Indexable` matches any indexable collection without type argument specialization.

### `interface Equatable[T]`
Declares the contract for types supporting equality comparison (`==`):
```python
interface Equatable[T]:
    def ==(other: T) -> Bool
```
- **Implicit Satisfaction**: All primitive types (`Int`, `Float`, `String`, `Bytes`, `Bool`, `Nil`, `Regex`, `Match`, `Tuple`, `Dict`) and user-defined structs implementing `def ==(other)` implicitly satisfy `Equatable[T]`.
- **Unspecialized Form**: `val is Equatable` matches any equatable value without type argument specialization.

### `interface Comparable[T]`
Declares the contract for types supporting relational ordering (`<`) and equality (`==`):
```python
interface Comparable[T]:
    Equatable[T]
    def ==(other: T) -> Bool
    def <(other: T) -> Bool
```
- **Implicit Satisfaction**: Naturally ordered types (`Int`, `Float`, `String`, `Bytes`) and user-defined structs implementing `<` and `==` satisfy `Comparable[T]`.
- **Unspecialized Form**: `val is Comparable` matches any comparable value without type argument specialization.

### `interface Numeric[T]`
Declares the contract for numeric types supporting conversion to `Int` and `Float`, absolute value inspection, and arithmetic operators:
```python
interface Numeric[T]:
    Comparable[T]
    def Int() -> Int
    def Float() -> Float
    def Abs() -> T
    def +(other: T) -> T
    def -(other: T) -> T
    def *(other: T) -> T
    def /(other: T) -> T
```
- **Implicit Satisfaction**:
  - `Int` satisfies `Numeric[Int]` and unspecialized `Numeric`.
  - `Float` satisfies `Numeric[Float]` and unspecialized `Numeric`.
  - User-defined structs implementing `.Int()`, `.Float()`, `.Abs()`, `+`, `-`, `*`, `/`, and comparison methods satisfy `Numeric[T]`.
- **Unspecialized Form**: `val is Numeric` matches any numeric type.

---

## 5. Built-in Modules

### Standard I/O Module (`io`)

The built-in `io` module provides stream I/O abstractions, interfaces, and standard stream access.

```python
import io
```

#### Core Interfaces
- **`Reader`**: `Read(n: Int): Bytes | Nil`
- **`Writer`**: `Write(b: Bytes): Int`, `Flush(): ()`
- **`Closer`**: `Close(): ()`
- **`ReadCloser`**: embeds `Reader`, `Closer` (`Read`, `Close`)
- **`WriteCloser`**: embeds `Writer`, `Closer` (`Write`, `Flush`, `Close`)

#### Standard Streams
- **`io::Stdin`**: Standard input instance satisfying `Reader`, `Closer`, and `ReadCloser`.
  - `Read(n: Int): Bytes | Nil`: reads up to `n` bytes as a `Bytes` sequence (`n == 0` returns empty `Bytes()`, EOF returns `nil`).
  - `Close(): ()`: closes standard input.
- **`io::Stdout`**: Standard output instance satisfying `Writer`, `Closer`, and `WriteCloser`.
  - `Write(b: Bytes): Int`: writes byte sequence `b` directly to standard output (automatically flushes any buffered `Stdout` first).
  - `Flush(): ()`: flushes standard output.
  - `Close(): ()`: closes standard output (automatically flushes buffered `Stdout` first).
- **`io::Stderr`**: Standard error instance satisfying `Writer`, `Closer`, and `WriteCloser`.
  - `Write(b: Bytes): Int`: writes byte sequence `b` directly to standard error without buffering (automatically flushes any buffered `Stdout` first).
  - `Flush(): ()`: provides a no-op `Flush(): ()` for `Writer` / `bufio.Writer` interface compatibility.
  - `Close(): ()`: closes standard error.

---

### Buffered I/O Module (`bufio`)

The built-in `bufio` module wraps stream I/O objects with buffering, line reading, and flushing capabilities.

```python
import bufio
```

#### Structs & Constructors
- **`bufio::Reader(r: Reader): Reader`**: Constructs a buffered reader wrapping an underlying `Reader` (such as `io::Stdin` or a custom struct implementing `Read`). If `r` is already a `bufio::Reader`, returns it directly.
  - `Read(n: Int): Bytes | Nil`: Reads up to `n` bytes from buffer/stream. `n == 0` returns empty `Bytes()`; `n < 0` produces a runtime error.
  - `ReadLine(): String | Nil`: Reads a single line as UTF-8 string, stripping trailing `\n` or `\r\n`.
  - `ReadString(delim: String): String | Nil`: Reads until the given delimiter string (supports multi-character delimiters). Returns accumulated text as UTF-8 string including delimiter; if EOF is reached before the delimiter, returns all accumulated trailing text up to EOF; returns `nil` only if EOF is encountered with 0 bytes read.
  - `Peek(n: Int): Bytes | Nil`: Peeks ahead up to `n` bytes without advancing the reader cursor. `n == 0` returns empty `Bytes()`; `n < 0` produces a runtime error.
- **`bufio::Writer(w: Writer): Writer`**: Constructs a buffered writer wrapping an underlying `Writer` (such as `io::Stdout` or a custom struct implementing `Write`). Automatically tracked and flushed on program exit or `exit()`. If `w` is already a `bufio::Writer`, returns it directly.
  - `Write(b: Bytes): Int`: Appends byte sequence `b` to write buffer.
  - `Flush(): ()`: Flushes buffered data to the underlying stream.
