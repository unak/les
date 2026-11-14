package types

import (
	"errors"
	"strings"
	"testing"

	"les/pkg/lexer"
	"les/pkg/parser"
)

func inferDuplicateMemberCode(code string) error {
	p := parser.NewParser(lexer.NewLexer(code))
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		return errors.New(p.Errors()[0])
	}
	return NewInferencer().InferProgram(prog)
}

func TestDuplicateStructMembersRejected(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		wantErr string // expected "<line>: <message> at col <col>"
	}{
		{
			name: "duplicate method",
			code: `struct D:
    x: Int
    def M() -> Int:
        1
    def M() -> Int:
        2
`,
			wantErr: "5: duplicate method 'M' in struct 'D' at col 5",
		},
		{
			name: "duplicate init",
			code: `struct P:
    x: Int
    def init(x: Int):
        @x = x
    def init(x: Int):
        @x = x * 10
`,
			wantErr: "5: duplicate method 'init' in struct 'P' at col 5",
		},
		{
			name: "duplicate operator method",
			code: `struct V:
    n: Int
    +(o: V): V:
        V(n: @n + o.n)
    +(o: V): V:
        V(n: 0)
`,
			wantErr: "5: duplicate method '+' in struct 'V' at col 5",
		},
		{
			name: "duplicate field with different types",
			code: `struct D:
    x: Int
    x: String
`,
			wantErr: "3: duplicate field 'x' in struct 'D' at col 5",
		},
		{
			name: "duplicate field with same type and different defaults",
			code: `struct D:
    x: Int = 1
    x: Int = 2
`,
			wantErr: "3: duplicate field 'x' in struct 'D' at col 5",
		},
		{
			name: "same struct embedded twice",
			code: `struct Inner:
    k: Int
struct Outer:
    Inner
    Inner
`,
			wantErr: "5: duplicate field 'Inner' in struct 'Outer' at col 5",
		},
		{
			name: "generic struct duplicate field",
			code: `struct Box[T]:
    v: T
    v: T
`,
			wantErr: "3: duplicate field 'v' in struct 'Box' at col 5",
		},
		{
			name: "generic struct duplicate method",
			code: `struct Box[T]:
    v: T
    def Get() -> T:
        @v
    def Get() -> T:
        @v
`,
			wantErr: "5: duplicate method 'Get' in struct 'Box' at col 5",
		},
		{
			name: "duplicate unary operator method",
			code: `struct V:
    n: Int
    -(): V:
        V(n: 0)
    -(): V:
        V(n: 1)
`,
			wantErr: "5: duplicate method '-' in struct 'V' at col 5",
		},
		{
			name: "duplicate binary operator method after a unary one",
			code: `struct V:
    n: Int
    +(): V:
        V(n: 0)
    +(o: V): V:
        V(n: 1)
    +(o: V): V:
        V(n: 2)
`,
			wantErr: "7: duplicate method '+' in struct 'V' at col 5",
		},
		{
			name: "unary operator with explicit self and implicit unary",
			code: `struct V:
    n: Int
    -(): V:
        V(n: 0)
    -(self): V:
        V(n: 1)
`,
			wantErr: "5: duplicate method '-' in struct 'V' at col 5",
		},
		{
			name: "same method with and without explicit self",
			code: `struct D:
    def Get(self) -> Int:
        1
    def Get() -> Int:
        2
`,
			wantErr: "4: duplicate method 'Get' in struct 'D' at col 5",
		},
		{
			name: "third operator method after a unary and binary pair",
			code: `struct V:
    n: Int
    +(self): V:
        V(n: 0)
    +(self, o: V): V:
        V(n: 1)
    +(): V:
        V(n: 2)
`,
			wantErr: "7: duplicate method '+' in struct 'V' at col 5",
		},
		{
			name: "error points at the later declaration, not the first",
			code: `struct D:
    a: Int
    b: Int
    a: Int
    c: Int
    c: Int
`,
			wantErr: "4: duplicate field 'a' in struct 'D' at col 5",
		},
		{
			name: "duplicate method after unrelated methods",
			code: `struct D:
    def A() -> Int:
        1
    def B() -> Int:
        2
    def C() -> Int:
        3
    def B() -> Int:
        4
`,
			wantErr: "8: duplicate method 'B' in struct 'D' at col 5",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := inferDuplicateMemberCode(tt.code)
			if err == nil {
				t.Fatalf("expected error %q, got nil", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("expected error %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestDuplicateStructMembersAllowed(t *testing.T) {
	tests := []struct {
		name string
		code string
	}{
		{
			name: "names differing in case are distinct fields",
			code: `struct S:
    size: Int
    Size: Int
`,
		},
		{
			name: "names differing in case are distinct methods",
			code: `struct S:
    def size() -> Int:
        1
    def Size() -> Int:
        2
`,
		},
		{
			name: "same member names in different structs",
			code: `struct A:
    x: Int
    def M() -> Int:
        1
struct B:
    x: Int
    def M() -> Int:
        2
`,
		},
		{
			name: "outer field and method shadow promoted members",
			code: `struct Inner:
    x: Int
    def M() -> Int:
        1
struct Outer:
    Inner
    x: String
    def M() -> Int:
        2
o = Outer(Inner: Inner(x: 1), x: "s")
a = o.x
b = o.M()
`,
		},
		{
			name: "two embedded structs are fine until a promoted name is used",
			code: `struct A:
    x: Int
    def M() -> Int:
        1
struct B:
    x: Int
    def M() -> Int:
        2
struct C:
    A
    B
c = C(A: A(x: 1), B: B(x: 2))
p = c.A.x
q = c.B.M()
`,
		},
		{
			name: "unary and binary operator methods overload",
			code: `struct V:
    n: Int
    +(): V:
        V(n: @n)
    +(o: V): V:
        V(n: @n + o.n)
    -(): V:
        V(n: 0 - @n)
    -(o: V): V:
        V(n: @n - o.n)
`,
		},
		{
			name: "embedded structs with the same name from different type annotations",
			code: `struct Box[T]:
    v: T
struct Pair:
    Box[Int]
    Box[String]
`,
		},
		{
			name: "direct field may share its name with an embedded struct (embedded first)",
			code: `struct Inner:
    k: Int
struct Outer:
    Inner
    Inner: Int
o = Outer(Inner: 5)
x = o.Inner
y = x + 1
`,
		},
		{
			name: "direct field may share its name with an embedded struct (field first)",
			code: `struct Inner:
    k: Int
struct Outer:
    Inner: Int
    Inner
`,
		},
		{
			name: "unary and binary operator methods with explicit self overload",
			code: `struct V:
    n: Int
    +(self): V:
        V(n: @n)
    +(self, o: V): V:
        V(n: @n + o.n)
    -(self): V:
        V(n: 0 - @n)
    -(self, o: V): V:
        V(n: @n - o.n)
a = V(n: 5)
b = (-a).n
c = (a - V(n: 2)).n
`,
		},
		{
			name: "implicit unary with explicit self binary overload",
			code: `struct V:
    n: Int
    -(): V:
        V(n: 0 - @n)
    -(self, o: V): V:
        V(n: @n - o.n)
`,
		},
		{
			name: "explicit self binary before implicit unary overload",
			code: `struct V:
    n: Int
    -(self, o: V): V:
        V(n: @n - o.n)
    -(): V:
        V(n: 0 - @n)
`,
		},
		{
			name: "different operator methods",
			code: `struct V:
    n: Int
    +(o: V): V:
        V(n: @n + o.n)
    -(o: V): V:
        V(n: @n - o.n)
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := inferDuplicateMemberCode(tt.code); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// Two embedded structs that promote the same member name are accepted at the
// definition; the conflict is reported as an ambiguous selector at the use
// site. This pins the behaviour that predates the duplicate member check.
func TestPromotedMemberAmbiguityUnchanged(t *testing.T) {
	const defs = `struct A:
    x: Int
    def M() -> Int:
        1
struct B:
    x: Int
    def M() -> Int:
        2
struct C:
    A
    B
c = C(A: A(x: 1), B: B(x: 2))
`
	tests := []struct {
		name    string
		use     string
		wantErr string
	}{
		{"promoted field", "v = c.x\n", "ambiguous selector 'x' on type C"},
		{"promoted method", "v = c.M()\n", "ambiguous selector 'M' on type C"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := inferDuplicateMemberCode(defs + tt.use)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

// A directly declared field with the same name as an embedded struct is
// accepted; the types below are what upstream produced before the duplicate
// member check existed.
func TestFieldSharingNameWithEmbeddedStructUnchanged(t *testing.T) {
	const defs = `struct Inner:
    k: Int
struct Outer:
    Inner
    Inner: Int
o = Outer(Inner: 5)
x = o.Inner
`
	prog := parser.NewParser(lexer.NewLexer(defs)).ParseProgram()
	inf := NewInferencer()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if xT, ok := inf.Scope().Lookup("x"); !ok || xT != TypeInt {
		t.Fatalf("expected o.Inner to have type Int, got %v", xT)
	}
}
