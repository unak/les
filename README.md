# Les Language Specification & Plan

## Overview
Les (Lazy Evaluation Script) is a statically-typed, aggressively lazy-evaluated scripting language implemented in Go.

## Approved Syntax Specification
1. **Variables & Optional Types**:
   ```python
   x = 10
   y: Int = 20
   ```
2. **Conditionals**:
   - Multi-line block:
     ```python
     if x > 0:
         1
     else:
         0
     ```
   - Inline ternary operator: C-style only (`cond ? then_expr : else_expr`).
     ```python
     res = x > 0 ? 1 : 0
     ```
3. **Tuples & Dicts**:
   - Tuple literal: `(1, 2, 3)`, empty `()`, single `(1,)`.
   - Dict literal: `{"key": "value", "a": 1}`.
   - Indexing: `[]` for both tuple element access (`t[0]`) and dict lookup (`d["key"]`).
4. **Functions**:
   ```python
   def add(a, b):
       a + b
   ```
   - Tail expression implicitly returned (or optional explicit `return`).
5. **Side-Effect Boundary**:
   - `print(...)` built-in implicitly drives strict demand evaluation of arguments.
   - User cannot define raw effect primitives, only wrap them.

## MVP Objectives
1. Basic expressions and `print()` with strict evaluation demand at side-effect points.
2. Lazy Fibonacci and infinite tuple streams taking head `n` elements:
   ```python
   def fib_stream(a, b):
       (a, fib_stream(b, a + b))

   def take(n, s):
       n <= 0 ? () : (s[0], take(n - 1, s[1]))

   print(take(5, fib_stream(0, 1)))
   ```
3. Function definitions with lazy parameter evaluation (unused divergent/error arguments are never evaluated):
   ```python
   def pick_first(a, b):
       a

   # 1 / 0 is never evaluated because second arg is never demanded:
   print(pick_first(42, 1 / 0))
   ```
