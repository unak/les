package types

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"les/pkg/lexer"
	"les/pkg/parser"
)

const embeddedAssignableDefs = `struct Base:
    k: Int
    def Name() -> String:
        "base"
struct Derived:
    Base
    def Name() -> String:
        "derived"
struct Deep:
    Derived
`

func inferEmbeddedAssignable(t *testing.T, code string) error {
	t.Helper()
	p := parser.NewParser(lexer.NewLexer(code))
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse errors: %v", p.Errors())
	}
	return NewInferencer().InferProgram(prog)
}

// Embedding provides no subtype relation: a struct is rejected where a struct
// it embeds (directly or transitively) is expected, in every position that
// checks assignability, even when every member of the embedded struct resolves
// compatibly on the outer one. The embedded value is reached explicitly (d.Base).
func TestEmbeddedStructNotAssignableEvenWhenCompatible(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{
			name: "argument",
			code: embeddedAssignableDefs + `def greet(b: Base) -> String:
    b.Name()
d = Derived(Base: Base(k: 1))
greet(d)
`,
		},
		{
			name: "argument, transitive embedding",
			code: embeddedAssignableDefs + `def greet(b: Base) -> String:
    b.Name()
d = Derived(Base: Base(k: 1))
greet(Deep(Derived: d))
`,
		},
		{
			name: "argument, intermediate embedded struct",
			code: embeddedAssignableDefs + `def greet(b: Derived) -> String:
    b.Name()
d = Derived(Base: Base(k: 1))
greet(Deep(Derived: d))
`,
		},
		{
			name: "annotated variable",
			code: embeddedAssignableDefs + `b: Base = Derived(Base: Base(k: 1))
`,
		},
		{
			name: "return type",
			code: embeddedAssignableDefs + `def mk() -> Base:
    Derived(Base: Base(k: 1))
mk()
`,
		},
		{
			name: "tuple element",
			code: embeddedAssignableDefs + `xs: (Base, ...) = (Derived(Base: Base(k: 1)), Base(k: 2))
`,
		},
		{
			name: "struct field",
			code: embeddedAssignableDefs + `struct Holder:
    b: Base
h = Holder(b: Derived(Base: Base(k: 1)))
`,
		},
		{
			name: "union member",
			code: embeddedAssignableDefs + `def f(b: Base | Nil) -> Int:
    1
f(Derived(Base: Base(k: 1)))
`,
		},
		{
			name: "compatible shadowing of a field",
			code: `struct Inner:
    v: Int
struct Outer:
    Inner
    v: Int
def get(x: Inner) -> Int:
    x.v + 1
get(Outer(Inner: Inner(v: 1), v: 2))
`,
		},
		{
			name: "embedded generic instance",
			code: `struct Box[T]:
    v: T
struct IntBox:
    Box[Int]
def get(b: Box[Int]) -> Int:
    b.v
get(IntBox(Box: Box[Int](v: 1)))
`,
		},
		{
			name: "generic outer struct",
			code: embeddedAssignableDefs + `struct Wrap[T]:
    Base
    w: T
def g(b: Base) -> Int:
    b.k
g(Wrap[Int](Base: Base(k: 1), w: 2))
`,
		},
		{
			name: "self-referential method types",
			code: `struct Node:
    v: Int
    def Next() -> Node:
        self
struct Sub:
    Node
    def Next() -> Sub:
        self
def f(n: Node) -> Int:
    n.Next().v
f(Sub(Node: Node(v: 1)))
`,
		},
		{
			name: "function taking the embedded struct where a function taking the outer is expected",
			code: embeddedAssignableDefs + `def fb(b: Base) -> Int:
    b.k
def use(f: (Derived) -> Int) -> Int:
    f(Derived(Base: Base(k: 1)))
use(fb)
`,
		},
		{
			name: "function returning the outer struct where one returning the embedded is expected",
			code: embeddedAssignableDefs + `def mk() -> Derived:
    Derived(Base: Base(k: 1))
def use(f: () -> Base) -> Int:
    f().k
use(mk)
`,
		},
		{
			name: "compatible overloaded operator methods",
			code: `struct Num:
    n: Int
    +() -> Int:
        @n
    +(o: Int) -> Int:
        @n + o
struct SubNum:
    Num
    +() -> Int:
        @n * 2
    +(o: Int) -> Int:
        @n + o + 1
def f(x: Num) -> Int:
    (+x) + (x + 1)
f(SubNum(Num: Num(n: 3)))
`,
		},
		{
			name: "extra trailing default parameter on outer method",
			code: `struct Base:
    k: Int
    def Foo(x: Int) -> Int:
        @k + x
struct Derived:
    Base
    def Foo(x: Int, y: String = "a") -> Int:
        @k + x
def f(b: Base) -> Int:
    b.Foo(1)
f(Derived(Base: Base(k: 2)))
`,
		},
		{
			name: "self-referential interface methods",
			code: `interface IHolder[T]:
    Get(): T
interface ISubHolder[T]:
    Get(): T
struct Wrap[T]:
    v: T
    def Get() -> T:
        @v
struct Base:
    k: Int
    def Wrap() -> IHolder[Base]:
        Wrap[Base](v: self)
struct Derived:
    Base
    def Wrap() -> ISubHolder[Derived]:
        Wrap[Derived](v: self)
def f(b: Base) -> Int:
    b.Wrap().Get().k
f(Derived(Base: Base(k: 1)))
`,
		},
		{
			name: "variadic method with default parameter",
			code: `struct Base:
    k: Int
    def Foo(*rest) -> Int:
        @k
struct Derived:
    Base
    def Foo(x: Int = 0, *rest) -> Int:
        @k + x
def f(b: Base) -> Int:
    b.Foo()
f(Derived(Base: Base(k: 1)))
`,
		},
		{
			name: "generic method with renamed type parameter",
			code: `struct Base:
    k: Int
    def Id[T](x: T) -> T:
        x
struct Derived:
    Base
    def Id[U](x: U) -> U:
        x
def f(b: Base) -> Int:
    b.Id[Int](1)
f(Derived(Base: Base(k: 1)))
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := inferEmbeddedAssignable(t, tt.code)
			if err == nil || !strings.Contains(err.Error(), "type mismatch") {
				t.Fatalf("expected a type mismatch error, got: %v", err)
			}
		})
	}
}

// Unrelated structs, reverse directions, named (non-embedded) fields, and
// embeddings with shadowed or ambiguous members also remain type mismatches.
func TestEmbeddedStructNotAssignable(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{
			name: "outer struct where the embedded one is expected, incompatible shadowed field",
			code: `struct Inner:
    v: Int
struct Outer:
    Inner
    v: String
def get(x: Inner) -> Int:
    x.v + 1
print get(Outer(Inner: Inner(v: 1), v: "s")), "\n"
`,
		},
		{
			name: "incompatible shadowed field of a transitively embedded struct",
			code: `struct Inner:
    v: Int
struct Mid:
    Inner
struct Outer:
    Mid
    v: String
def get(x: Inner) -> Int:
    x.v
get(Outer(Mid: Mid(Inner: Inner(v: 1)), v: "s"))
`,
		},
		{
			name: "incompatible shadowing method",
			code: `struct Inner:
    def Name() -> String:
        "inner"
struct Outer:
    Inner
    def Name() -> Int:
        1
def get(x: Inner) -> String:
    x.Name()
get(Outer(Inner: Inner()))
`,
		},
		{
			name: "method shadowed by a field",
			code: `struct Inner:
    def Name() -> String:
        "inner"
struct Outer:
    Inner
    Name: String
def get(x: Inner) -> String:
    x.Name()
get(Outer(Inner: Inner(), Name: "s"))
`,
		},
		{
			name: "member made ambiguous",
			code: `struct A:
    x: Int
struct B:
    x: Int
struct C:
    A
    B
def ax(a: A) -> Int:
    a.x
print ax(C(A: A(x: 1), B: B(x: 2))), "\n"
`,
		},
		{
			name: "embedded generic instance with other type arguments",
			code: `struct Box[T]:
    v: T
struct IntBox:
    Box[Int]
def get(b: Box[String]) -> String:
    b.v
get(IntBox(Box: Box[Int](v: 1)))
`,
		},
		{
			name: "embedded generic instance, bare generic target",
			code: `struct Box[T]:
    v: T
struct IntBox:
    Box[Int]
def get(b: Box) -> Int:
    1
get(IntBox(Box: Box[Int](v: 1)))
`,
		},
		{
			name: "embedded struct where the outer one is expected",
			code: embeddedAssignableDefs + `def f(d: Derived) -> String:
    d.Name()
f(Base(k: 1))
`,
		},
		{
			name: "unrelated structs",
			code: embeddedAssignableDefs + `struct Other:
    k: Int
def f(b: Base) -> Int:
    b.k
f(Other(k: 1))
`,
		},
		{
			name: "named field instead of embedding",
			code: embeddedAssignableDefs + `struct Has:
    b: Base
def f(b: Base) -> Int:
    b.k
f(Has(b: Base(k: 1)))
`,
		},
		{
			name: "function taking the outer struct where one taking the embedded is expected",
			code: embeddedAssignableDefs + `def fd(d: Derived) -> Int:
    d.k
def use(f: (Base) -> Int) -> Int:
    f(Base(k: 1))
use(fd)
`,
		},
		{
			name: "function returning the embedded struct where one returning the outer is expected",
			code: embeddedAssignableDefs + `def mk() -> Base:
    Base(k: 1)
def use(f: () -> Derived) -> Int:
    f().k
use(mk)
`,
		},
		{
			name: "outer shadows overloaded operator with single overload",
			code: `struct Num:
    n: Int
    +() -> Int:
        @n
    +(o: Int) -> Int:
        @n + o
struct SubNum:
    Num
    +(o: Int) -> Int:
        @n + o + 1
def f(x: Num) -> Int:
    +x
f(SubNum(Num: Num(n: 1)))
`,
		},
		{
			name: "outer swaps overloaded operator return types",
			code: `struct Num:
    n: Int
    +() -> String:
        "s"
    +(o: Int) -> Int:
        @n + o
struct SubNum:
    Num
    +() -> Int:
        @n
    +(o: Int) -> String:
        "bad"
def f(x: Num) -> String:
    +x
f(SubNum(Num: Num(n: 1)))
`,
		},
		{
			name: "outer drops default parameter from method",
			code: `struct Base:
    k: Int
    def Foo(x: Int = 0) -> Int:
        @k + x
struct Derived:
    Base
    def Foo(x: Int) -> Int:
        @k + x
def f(b: Base) -> Int:
    b.Foo()
f(Derived(Base: Base(k: 1)))
`,
		},
		{
			name: "outer makes optional block parameter required",
			code: `struct Base:
    k: Int
    def Foo(&blk = nil) -> Int:
        @k
struct Derived:
    Base
    def Foo(&blk) -> Int:
        blk()
def f(b: Base) -> Int:
    b.Foo()
f(Derived(Base: Base(k: 1)))
`,
		},
		{
			name: "outer drops default before variadic parameter",
			code: `struct Base:
    k: Int
    def Foo(x: Int = 0, *rest) -> Int:
        @k + x
struct Derived:
    Base
    def Foo(x: Int, *rest) -> Int:
        @k + x
def f(b: Base) -> Int:
    b.Foo()
f(Derived(Base: Base(k: 1)))
`,
		},
		{
			name: "outer tightens generic method constraint",
			code: `struct Base:
    k: Int
    def Id[T](x: T) -> T:
        x
struct Derived:
    Base
    def Id[T: Equatable](x: T) -> T:
        x
def f(b: Base) -> Int:
    b.Id[Int](1)
f(Derived(Base: Base(k: 1)))
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := inferEmbeddedAssignable(t, tt.code)
			if err == nil || !strings.Contains(err.Error(), "type mismatch") {
				t.Fatalf("expected a type mismatch error, got: %v", err)
			}
		})
	}
}

func TestEmbeddedStructNotAssignableConcurrent(t *testing.T) {
	code := `struct Base:
    k: Int
    def Clone() -> Base:
        Base(k: @k)
struct Derived:
    Base
    def Clone() -> Derived:
        Derived(Base: Base(k: @k))
def f(b: Base) -> Base:
    b.Clone()
f(Derived(Base: Base(k: 1)))
`
	const workers = 8
	const iters = 25
	var wg sync.WaitGroup
	errs := make(chan error, workers*iters)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				p := parser.NewParser(lexer.NewLexer(code))
				prog := p.ParseProgram()
				if len(p.Errors()) > 0 {
					errs <- fmt.Errorf("parse errors: %v", p.Errors())
					continue
				}
				err := NewInferencer().InferProgram(prog)
				if err == nil || !strings.Contains(err.Error(), "type mismatch") {
					errs <- fmt.Errorf("expected a type mismatch error, got: %v", err)
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent inference: %v", err)
		}
	}
}
