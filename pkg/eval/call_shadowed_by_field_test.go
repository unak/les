package eval

import (
	"strings"
	"testing"
)

// Inside a method the fields of the receiver are visible by bare name. A field
// that is not callable must not hide the function or struct of the same name
// when the name is called, e.g. a struct embedding Base can still call Base(...).
func TestCallNotHiddenByNonCallableField(t *testing.T) {
	const baseDef = `struct Base:
    k: Int
`
	tests := []struct {
		name     string
		code     string
		expected string
	}{
		{
			name: "struct constructor named like the embedded field",
			code: baseDef + `struct Derived:
    Base
    def Again() -> Derived:
        Derived(Base: Base(k: @k + 1))
print Derived(Base: Base(k: 1)).Again().k`,
			expected: "2",
		},
		{
			name: "constructor call as an operand of is",
			code: baseDef + `struct Derived:
    Base
    def T() -> Bool:
        Base(k: 1) is Base
print Derived(Base: Base(k: 1)).T()`,
			expected: "true",
		},
		{
			name: "constructor call in init",
			code: baseDef + `struct Derived:
    Base
    def init(k: Int):
        @Base = Base(k: k)
print Derived(4).k`,
			expected: "4",
		},
		{
			name: "function named like a non-callable field",
			code: baseDef + `def mk(k: Int) -> Base:
    Base(k: k)
struct W:
    mk: Int
    def Go() -> Base:
        mk(3)
print W(mk: 1).Go().k`,
			expected: "3",
		},
		{
			name: "builtin function named like a non-callable field",
			code: `struct S:
    Error: Int
    def F() -> Bool:
        Error("boom") is Error
print S(Error: 1).F()`,
			expected: "true",
		},
		{
			name: "inside a lambda defined in a method",
			code: baseDef + `struct Derived:
    Base
    def Mk() -> (Int) -> Base:
        |x: Int|: Base(k: x)
print Derived(Base: Base(k: 1)).Mk()(7).k`,
			expected: "7",
		},
		{
			name: "lazy local variable used later",
			code: baseDef + `struct Derived:
    Base
    def Lz() -> Int:
        b = Base(k: @k + 10)
        c = Base(k: b.k + 100)
        c.k
print Derived(Base: Base(k: 1)).Lz()`,
			expected: "111",
		},
		{
			name: "operator method",
			code: baseDef + `struct Derived:
    Base
    def +(o: Derived) -> Derived:
        Derived(Base: Base(k: @k + o.k))
print (Derived(Base: Base(k: 1)) + Derived(Base: Base(k: 2))).k`,
			expected: "3",
		},
		{
			name: "struct embedded transitively calls Base and Derived",
			code: baseDef + `struct Derived:
    Base
struct Deep:
    Derived
    def Go() -> Deep:
        Deep(Derived: Derived(Base: Base(k: @k + 1)))
    def Go2() -> Int:
        Base(k: Derived(Base: Base(k: 20)).k + 1).k
d = Deep(Derived: Derived(Base: Base(k: 1)))
print d.Go().k, " ", d.Go2()`,
			expected: "2 21",
		},
		{
			name: "generic function named like a non-callable field",
			code: `def wrap[T](x: T) -> (T, T):
    (x, x)
struct W:
    wrap: Int
    def Go() -> Int:
        a, b = wrap(7)
        a + b
print W(wrap: 1).Go()`,
			expected: "14",
		},
		// Behaviour that must not change.
		{
			name: "bare embedded name is still the embedded value",
			code: baseDef + `struct Derived:
    Base
    def Emb() -> Base:
        Base
    def K2() -> Int:
        Base.k + self.Base.k
d = Derived(Base: Base(k: 5))
print d.Emb().k, " ", d.K2()`,
			expected: "5 10",
		},
		{
			name: "callable field wins over an outer function",
			code: `def conv(x: Int) -> String:
    "outer"
struct Op:
    conv: (Int) -> Int
    def Apply(x: Int) -> Int:
        conv(x)
print Op(conv: |x: Int|: x * 100).Apply(2)`,
			expected: "200",
		},
		{
			name: "callable field wins over an outer struct",
			code: baseDef + `struct Holder:
    Base: (Int) -> Int
    def Apply(x: Int) -> Int:
        Base(x)
print Holder(Base: |x: Int|: x + 1).Apply(2)`,
			expected: "3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := runLesCode(tt.code)
			if err != nil {
				t.Fatalf("run failed: %v", err)
			}
			if out != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, out)
			}
		})
	}
}

// The explicit forms name the field, so they keep failing on a non-callable
// field, and so do a parameter or a local variable that is not callable.
func TestCallOfNonCallableBindingStillFails(t *testing.T) {
	const defs = `struct Base:
    k: Int
`
	tests := []struct {
		name string
		code string
	}{
		{
			name: "@name call",
			code: defs + `struct Derived:
    Base
    def A() -> Derived:
        Derived(Base: @Base(k: 1))
print Derived(Base: Base(k: 1)).A().k`,
		},
		{
			name: "self.name call",
			code: defs + `struct Derived:
    Base
    def A() -> Derived:
        Derived(Base: self.Base(k: 1))
print Derived(Base: Base(k: 1)).A().k`,
		},
		{
			name: "non-callable parameter",
			code: `def mk(k: Int) -> Int:
    k * 2
struct W:
    v: Int
    def Go(mk: Int) -> Int:
        mk(3)
print W(v: 1).Go(1)`,
		},
		{
			name: "non-callable local variable",
			code: `def mk(k: Int) -> Int:
    k * 2
struct W:
    v: Int
    def Go() -> Int:
        mk = 1
        mk(3)
print W(v: 1).Go()`,
		},
		{
			name: "non-callable field and no callable of that name",
			code: `struct W:
    mk: Int
    def Go() -> Int:
        mk(3)
print W(mk: 1).Go()`,
		},
		{
			name: "non-callable global between the field and the function",
			code: `def mk(k: Int) -> Int:
    k * 2
struct W:
    mk: Int
    def Go() -> Int:
        mk(3)
mk = 5
print W(mk: 1).Go()`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := runLesCode(tt.code)
			if err == nil {
				t.Fatalf("expected an error, got output %q", out)
			}
			if !strings.Contains(err.Error(), "cannot call non-function value") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
