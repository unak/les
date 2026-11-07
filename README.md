# Les Language

## Overview

Les (Lazy Evaluation Script) is a modern scripting language combining Python-like syntax, Ruby-style developer ergonomics, and Go-inspired static typing with purely demand-driven lazy evaluation.

Les is implemented in Go.

---

## Key Features & Philosophy

- **Python-like Syntax**: Clean, indentation-based block syntax without noisy braces or semicolons (while optional `;` and `\` line continuation are fully supported).
- **Ruby-style Ergonomics**:
  - Optional parentheses for function application (`print x + 1`).
  - Functional pipeline operator (`|>`).
  - Expressive block parameters (`do |x|: ...`).
  - Convenient `@` sugar for receiver access (`@field`, `@method()`, `@field = val`).
  - Module scope resolution using `::` (e.g. `math::Pi`).
- **Go-like Type System & Structural Interfaces**:
  - Go-style implicit structural typing (duck typing) for interfaces.
  - Struct embedding with automatic field and method promotion.
  - Clear visibility rules based on identifier casing (uppercase `[A-Z]` is exported/public; lowercase `[a-z_]` is private/module-local).
- **Hindley-Milner Static Type Inference & Union Types**:
  - Full static type inference: explicit type annotations are optional.
  - First-class union types (`Type1 | Type2`) and exhaustive pattern matching (`match ... case`).
- **Demand-Driven Lazy Evaluation**:
  - Function arguments, container elements, and variables are deferred thunks by default.
  - Strict demand boundaries at side-effect points (`print`, `exit`) or via explicit forcing (`expr!` or `force expr`).
  - Non-blocking lazy I/O reads (`gets()`).
- **First-Class Error Handling**:
  - Errors are first-class values (`interface Error`) rather than exceptions, preventing evaluation-order surprises under lazy semantics.
  - Idiomatic `Value | Error` union returns inspected via pattern matching.

---

## Quick Example

```python
# Struct definition with Go-style embedding and '@' receiver sugar
struct Point:
    x: Int = 0
    y: Int = 0

    def DistSq() -> Int:
        @x * @x + @y * @y

    def Shift(dx: Int, dy: Int) -> Point:
        Point(x: @x + dx, y: @y + dy)

# Structural interfaces (duck typing)
interface Measurable:
    DistSq(): Int

def PrintMeasurement[T: Measurable](item: T):
    print "Distance squared: ", item.DistSq(), "\n"

# Immutable records, pipelines, and paren-omitted calls
p = Point(x: 3, y: 4)
p.Shift(1, -1) |> PrintMeasurement

# Error handling with union types and pattern matching
def safe_divide(a: Int, b: Int): Int | Error:
    if b == 0:
        Error("cannot divide by zero")
    else:
        a / b

val = safe_divide(10, 0)
match val:
    case Int:
        print "Result: ", val, "\n"
    case Error:
        print "Error: ", val.Message(), "\n"
```

---

## Documentation

Comprehensive documentation is organized into dedicated guides in the [`doc/`](doc/) directory:

- [**Language Syntax & Semantics Guide**](doc/syntax.md):
  Covers primitive types, Hindley-Milner type inference, union types, generic type constraints, modules, function call semantics, operator precedence, lambdas, control flow, structs, embedding, interfaces, and error handling.
- [**Standard Library & Built-in Reference**](doc/reference.md):
  Complete reference manual for built-in functions (`print`, `exit`, `gets`), predefined variables (`Args`, `Stdin`, `Stdout`, `Stderr`), primitive type methods, core interfaces (`Show`, `Equatable`, `Comparable`, `Indexable`, `Numeric`), and standard library modules (`io`, `bufio`).

---

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

---

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

### Options
| Option | Description |
|---|---|
| `-v` | Display version and platform information, then exit. |
| `-c` | Check syntax only without type checking or execution. |
| `-t` | Check syntax and static types (including resolving and verifying imported modules) without execution. |
| `-e <code>` | Evaluate the inline script string directly. |

---

## Planned Features / Roadmap

### 1. Hexadecimal Floating-Point Literals
- Exact IEEE 754 representation using hexadecimal floats with binary exponents (`p` / `P` power of 2):
  - Syntax: `0x1.0p0`, `0xAB.CDEFp4`, `0x1.fp-3`
  - Lexer disambiguation: parses `0x...` followed by `.` as float literals where valid, falling back to integer member access otherwise.

### 2. String Interpolation (`#{}`)
- Ruby-style string interpolation within double-quoted strings:
  - `"User: #{name}, Age: #{age}\n"` evaluates embedded expressions via `Inspect()` (`interface Show`).
  - Single-quoted strings (`'...'`) remain literal without interpolation.

### 3. Null-Safe Navigation (`?.`) & Coalescing Operator (`??`)
- Syntactic sugar for handling `Nil` and nullable union types (`Type | Nil`):
  - Safe navigation: `user?.Profile?.Name` (safely yields `nil` if intermediate receiver is `nil`).
  - Null coalescing: `user?.Name ?? "Guest"` (provides fallback expression when operand is `nil`).

---

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

---

## Contributions

By submitting contributions (via pull requests, email, or any other means) to this project:

1. **Copyright Ownership**: You retain the copyright to your contributed code.
2. **Grant of Rights**: You grant the project owner (U.Nakamura) an unrestricted, perpetual, irrevocable, worldwide, royalty-free license to use, copy, modify, merge, distribute, sublicense, and relicense your contributions.
3. **Non-infringement**: You represent and warrant that your contributions are your original work and do not infringe upon any third party's copyright, patent, or other intellectual property rights.
