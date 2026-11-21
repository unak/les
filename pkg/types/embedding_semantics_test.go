package types

import (
	"strings"
	"testing"
)

// Static side of the struct-embedding semantics (see also
// pkg/eval/embedding_semantics_test.go, which uses the same programs).
//
// The tests are split in two groups:
//
//   - Common invariants: properties that must hold under every candidate
//     semantics of embedding (composition only, or composition plus a safe
//     subtype relation).
//   - Distinguishing tests: properties whose expected result depends on the
//     chosen semantics. Here: composition semantics, i.e. embedding gives no
//     subtype relation between named structs, so an outer struct is never
//     assignable to an embedded struct type even when members match
//     compatibly. Only the wantErr column of these tests differs between
//     candidates.

const embSemDefs = `struct Base:
    X: Int
struct Derived:
    Base
struct Compatible:
    Base
    X: Int
struct Incompatible:
    Base
    X: String
b = Base(X: 1)
d = Derived(Base: b)
c = Compatible(Base: b, X: 2)
i = Incompatible(Base: b, X: "s")
`

func embSemCheck(t *testing.T, code string) (*Inferencer, error) {
	t.Helper()
	inf, err := inferWithModules(t, nil, code)
	return inf, err
}

func embSemWantOK(t *testing.T, code string) *Inferencer {
	t.Helper()
	inf, err := embSemCheck(t, code)
	if err != nil {
		t.Fatalf("expected no type error, got: %v", err)
	}
	return inf
}

func embSemWantErr(t *testing.T, code, substr string) {
	t.Helper()
	_, err := embSemCheck(t, code)
	if err == nil || !strings.Contains(err.Error(), substr) {
		t.Fatalf("expected error containing %q, got: %v", substr, err)
	}
}

func embSemTypeOf(t *testing.T, inf *Inferencer, name string) Type {
	t.Helper()
	typ, ok := inf.Scope().Lookup(name)
	if !ok {
		t.Fatalf("%s is not defined", name)
	}
	return typ
}

// ---------------------------------------------------------------------------
// Common invariants
// ---------------------------------------------------------------------------

// The declarations themselves are accepted, including an incompatible shadowing
// of a promoted field (as in Go), and member access resolves to the outermost
// declaration.
func TestEmbeddingCommonMemberAccess(t *testing.T) {
	inf := embSemWantOK(t, embSemDefs+`dx = d.X
cx = c.X
ix = i.X
bb = d.Base
`)
	for name, want := range map[string]Type{"dx": TypeInt, "cx": TypeInt, "ix": TypeString} {
		if got := embSemTypeOf(t, inf, name); got != want {
			t.Errorf("%s: expected %v, got %v", name, want, got)
		}
	}
	if got := embSemTypeOf(t, inf, "bb"); got.String() != "Base" {
		t.Errorf("d.Base: expected Base, got %v", got)
	}
}

// The embedded value is always passable where the embedded struct is expected.
func TestEmbeddingCommonExplicitEmbeddedValueIsAssignable(t *testing.T) {
	embSemWantOK(t, embSemDefs+`def take(x: Base) -> Int:
    x.X
take(b)
take(d.Base)
take(c.Base)
take(i.Base)
y: Base = d.Base
`)
}

// Incompatible shadowing is rejected for assignment under every semantics.
func TestEmbeddingCommonIncompatibleNotAssignable(t *testing.T) {
	embSemWantErr(t, embSemDefs+`def take(x: Base) -> Int:
    x.X
take(i)
`, "type mismatch")
}

// Issue #7: the program type-checks (the value is accepted by an unannotated
// parameter) and the narrowed access sees the type the checker computed.
func TestEmbeddingCommonIssue7Reproduction(t *testing.T) {
	embSemWantOK(t, `struct Inner:
    v: Int
struct Outer:
    Inner
    v: String
def get(x) -> Int:
    x is Inner ? x.v + 1 : 0
o = Outer(Inner: Inner(v: 1), v: "s")
print get(o), "\n"
`)
}

// Ambiguous promoted members are reported; an outer declaration resolves them.
func TestEmbeddingCommonAmbiguity(t *testing.T) {
	const defs = `struct A:
    X: Int
struct B:
    X: Int
`
	embSemWantErr(t, defs+`struct Both:
    A
    B
o = Both(A: A(X: 1), B: B(X: 2))
o.X
`, "ambiguous selector")
	embSemWantOK(t, defs+`struct Both:
    A
    B
    X: Int
o = Both(A: A(X: 1), B: B(X: 2), X: 3)
o.X
`)
	// An explicit path is never ambiguous.
	embSemWantOK(t, defs+`struct Both:
    A
    B
o = Both(A: A(X: 1), B: B(X: 2))
o.A.X + o.B.X
`)
}

// Multi-level embedding: members are promoted transitively and each level's
// embedded value is reachable through its field.
func TestEmbeddingCommonMultiLevel(t *testing.T) {
	inf := embSemWantOK(t, embSemDefs+`struct Deep:
    Derived
deep = Deep(Derived: d)
dx = deep.X
mid = deep.Derived
bb = deep.Derived.Base
`)
	if got := embSemTypeOf(t, inf, "dx"); got != TypeInt {
		t.Errorf("deep.X: expected Int, got %v", got)
	}
	if got := embSemTypeOf(t, inf, "bb"); got.String() != "Base" {
		t.Errorf("deep.Derived.Base: expected Base, got %v", got)
	}
}

// Methods: promotion, overriding with a different signature, and a promoted
// method keeping its own receiver are all accepted by the checker.
func TestEmbeddingCommonMethods(t *testing.T) {
	const defs = `struct Base:
    k: Int
    def Name() -> String:
        "base"
    def Hello() -> String:
        "hello " + @Name()
struct Same:
    Base
    def Name() -> String:
        "same"
struct Other:
    Base
    def Name() -> Int:
        42
`
	inf := embSemWantOK(t, defs+`s = Same(Base: Base(k: 1))
o = Other(Base: Base(k: 1))
sn = s.Name()
on = o.Name()
sh = s.Hello()
oh = o.Hello()
`)
	for name, want := range map[string]Type{"sn": TypeString, "on": TypeInt, "sh": TypeString, "oh": TypeString} {
		if got := embSemTypeOf(t, inf, name); got != want {
			t.Errorf("%s: expected %v, got %v", name, want, got)
		}
	}
}

// Interfaces are satisfied structurally by the members an outer struct has,
// promoted ones included, and not by members it lacks.
func TestEmbeddingCommonInterfaceStructuralSatisfaction(t *testing.T) {
	const defs = `interface Named:
    Name(): String
struct Base:
    k: Int
    def Name() -> String:
        "base"
struct Derived:
    Base
struct Wrong:
    Base
    def Name() -> Int:
        1
def show(n: Named) -> String:
    n.Name()
`
	embSemWantOK(t, defs+`show(Derived(Base: Base(k: 1)))
`)
	embSemWantErr(t, defs+`show(Wrong(Base: Base(k: 1)))
`, "type mismatch")
}

// Generic embedding: promoted members are substituted, and the explicit
// embedded field has the instantiated type.
func TestEmbeddingCommonGenericEmbedding(t *testing.T) {
	inf := embSemWantOK(t, `struct Box[T]:
    v: T
struct IntBox:
    Box[Int]
ib = IntBox(Box: Box[Int](v: 1))
v = ib.v
inner = ib.Box
def take(x: Box[Int]) -> Int:
    x.v
r = take(ib.Box)
`)
	if got := embSemTypeOf(t, inf, "v"); got != TypeInt {
		t.Errorf("ib.v: expected Int, got %v", got)
	}
	if got := embSemTypeOf(t, inf, "inner"); got.String() != "Box[Int]" {
		t.Errorf("ib.Box: expected Box[Int], got %v", got)
	}
}

// Module-qualified embedded type: promotion and the explicit field work, and a
// struct of the same name from another module stays a different type.
func TestEmbeddingCommonModuleQualified(t *testing.T) {
	inf := wantIdentityOK(t, identityFiles, `
import m1
struct Outer:
    m1::Base
o = Outer(Base: m1::Base(X: 1))
x = o.X
b: m1::Base = o.Base
r = m1::Getx(o.Base)
`)
	if got := embSemTypeOf(t, inf, "x"); got != TypeInt {
		t.Errorf("o.X: expected Int, got %v", got)
	}
}

// Narrowing by `is` and `match` on an unknown-typed value leaves member access
// well typed. A statically known outer type is never narrowed to the embedded
// struct, so no member access on it can resolve to a stale type.
func TestEmbeddingCommonNarrowing(t *testing.T) {
	embSemWantOK(t, embSemDefs+`def f(x) -> Int:
    x is Base ? x.X : 0
def g(x) -> Int:
    match x:
        case Base:
            x.X
        case _:
            0
f(b)
g(b)
f(d)
g(i)
`)
}

// Gradual typing: Unknown is statically compatible with every struct, so
// nothing is rejected at check time; the runtime decides.
func TestEmbeddingCommonUnknownIsNotAStaticMismatch(t *testing.T) {
	embSemWantOK(t, embSemDefs+`def pass(x):
    x
def take(x: Base) -> Int:
    x.X
take(pass(d))
take(pass(i))
`)
}

// Extra: an Incompatible outer struct can never be a Base at run time, so it
// never overlaps Base statically, and `is Base` removes it from a union.
func TestEmbeddingCommonIncompatibleNeverOverlapsBase(t *testing.T) {
	inf := embSemWantOK(t, embSemDefs)
	base := embSemTypeOf(t, inf, "Base")
	incomp := embSemTypeOf(t, inf, "Incompatible")
	if TypesCanOverlap(incomp, base) || TypesCanOverlap(base, incomp) {
		t.Errorf("Incompatible and Base must not overlap")
	}
	if narrowTypeByPattern(incomp, base) != TypeUnknown {
		t.Errorf("narrowing Incompatible by Base must leave no struct type")
	}
	if got := narrowTypeByPattern(NewUnionType(base, incomp), base); got != base {
		t.Errorf("narrowing Base | Incompatible by Base: expected Base, got %v", got)
	}
	// End to end: x.X is an Int after the test, so Incompatible was excluded.
	embSemWantOK(t, embSemDefs+`def get(x: Base | Incompatible) -> Int:
    x is Base ? x.X : 0
def get2(x: Base | Incompatible) -> Int:
    match x:
        case Base:
            x.X
        case _:
            0
`)
	embSemWantOK(t, embSemDefs+`def get(x: Base | Incompatible) -> String:
    x is Incompatible ? x.X : "none"
`)
}

// Extra: an interface is not satisfied when the required method is ambiguous
// across embedded structs or shadowed by a non-callable field.
func TestEmbeddingCommonInterfaceEdgeCases(t *testing.T) {
	const defs = `interface Named:
    Name(): String
def show(n: Named) -> String:
    n.Name()
`
	embSemWantErr(t, defs+`struct A:
    def Name() -> String:
        "a"
struct B:
    def Name() -> String:
        "b"
struct Both:
    A
    B
show(Both(A: A(), B: B()))
`, "type mismatch")
	embSemWantOK(t, defs+`struct A:
    def Name() -> String:
        "a"
struct B:
    def Name() -> String:
        "b"
struct Resolved:
    A
    B
    def Name() -> String:
        "resolved"
show(Resolved(A: A(), B: B()))
`)
	embSemWantErr(t, defs+`struct Base:
    def Name() -> String:
        "base"
struct FieldShadow:
    Base
    Name: String
show(FieldShadow(Base: Base(), Name: "s"))
`, "type mismatch")
}

// Extra: else-branch flow narrowing removes Base from Base | Derived because
// runtime `is` matches struct types nominally.
func TestEmbeddingCommonFlowNarrowingElseBranch(t *testing.T) {
	embSemWantOK(t, `struct Base:
    X: Int
struct Derived:
    Base
    Extra: Int
def f(x: Base | Derived) -> Int:
    if x is Base:
        x.X
    else:
        x.Extra
`)
}

var embSemGenModFiles = map[string]string{
	"gen": `module gen

struct Box[T]:
    v: T

def Unbox(b: Box[Int]) -> Int:
    b.v
`,
}

// Extra: module-qualified generic embedding promotes substituted members and
// preserves the exact instantiated type on the embedded field.
func TestEmbeddingCommonModuleQualifiedGeneric(t *testing.T) {
	inf := wantIdentityOK(t, embSemGenModFiles, `
import gen

struct Child:
    gen::Box[Int]

c = Child(Box: gen::Box[Int](v: 7))
v = c.v
r = gen::Unbox(c.Box)
`)
	if got := embSemTypeOf(t, inf, "v"); got != TypeInt {
		t.Errorf("c.v: expected Int, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// Distinguishing tests
//
// Expectations below are those of composition semantics (no subtype relation
// between named structs through embedding). wantErr is the only column that
// differs from a candidate that keeps subtyping.
// ---------------------------------------------------------------------------

func TestEmbeddingDistinguishingPassToBaseParameter(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		wantErr bool
	}{
		{"Base", "b", false},
		{"Derived", "d", true},
		{"Compatible", "c", true},
		{"Incompatible", "i", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := embSemDefs + `def take(x: Base) -> Int:
    x.X
take(` + tt.arg + `)
`
			if tt.wantErr {
				embSemWantErr(t, code, "type mismatch")
			} else {
				embSemWantOK(t, code)
			}
		})
	}
}

// The other positions that check assignability behave the same way.
func TestEmbeddingDistinguishingOtherPositions(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		wantErr bool
	}{
		{"annotated variable, Derived", `v: Base = d`, true},
		{"annotated variable, Compatible", `v: Base = c`, true},
		{"return, Derived", "def mk() -> Base:\n    d\n", true},
		{"tuple element, Derived", `v: (Base, Int) = (d, 1)`, true},
		{"union member, Derived", `def f(x: Base | Nil) -> Int:
    1
f(d)
`, true},
		{"transitive, Deep to Base", `struct Deep:
    Derived
def take(x: Base) -> Int:
    x.X
take(Deep(Derived: d))
`, true},
		{"transitive, Deep to Derived", `struct Deep:
    Derived
def take(x: Derived) -> Int:
    x.X
take(Deep(Derived: d))
`, true},
		{"explicit field, Deep", `struct Deep:
    Derived
def take(x: Base) -> Int:
    x.X
take(Deep(Derived: d).Base)
`, false},
		{"generic embedding", `struct Box[T]:
    v: T
struct IntBox:
    Box[Int]
def take(x: Box[Int]) -> Int:
    x.v
take(IntBox(Box: Box[Int](v: 1)))
`, true},
		{"reverse direction stays an error", `def take(x: Derived) -> Int:
    x.X
take(b)
`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := embSemDefs + tt.code + "\n"
			if tt.wantErr {
				embSemWantErr(t, code, "type mismatch")
			} else {
				embSemWantOK(t, code)
			}
		})
	}
}

// A module-qualified embedded struct is no subtype, like a local one.
func TestEmbeddingDistinguishingModuleQualified(t *testing.T) {
	wantIdentityErr(t, identityFiles, `
import m1
struct Outer:
    m1::Base
o = Outer(Base: m1::Base(X: 1))
r = m1::Getx(o)
`, "type mismatch")
}

// Methods: whether the outer method is compatible with the embedded one is
// irrelevant to assignability.
func TestEmbeddingDistinguishingMethodCompatibility(t *testing.T) {
	const defs = `struct Base:
    k: Int
    def Name() -> String:
        "base"
struct Same:
    Base
    def Name() -> String:
        "same"
struct Other:
    Base
    def Name() -> Int:
        42
def take(x: Base) -> Int:
    x.k
`
	embSemWantErr(t, defs+`take(Same(Base: Base(k: 1)))`, "type mismatch")
	embSemWantErr(t, defs+`take(Other(Base: Base(k: 1)))`, "type mismatch")
}

// Extra: an unannotated (Unknown) member does not change the outcome: the
// outer struct is still not usable as the embedded one.
func TestEmbeddingDistinguishingUnknownMemberIsGradual(t *testing.T) {
	embSemWantErr(t, `struct Base:
    def Get(v: Int) -> Int:
        v
struct Loose:
    Base
    def Get(v):
        v
def f(x: Base) -> Int:
    x.Get(1)
f(Loose(Base: Base()))
`, "type mismatch")
}

// Extra: static overlap and narrowing follow the same rule as assignability:
// no overlap, hence narrowing to Unknown.
func TestEmbeddingDistinguishingOverlapAndNarrowing(t *testing.T) {
	inf := embSemWantOK(t, embSemDefs)
	base := embSemTypeOf(t, inf, "Base")
	derived := embSemTypeOf(t, inf, "Derived")
	comp := embSemTypeOf(t, inf, "Compatible")
	incomp := embSemTypeOf(t, inf, "Incompatible")
	if TypesCanOverlap(derived, base) || TypesCanOverlap(base, comp) {
		t.Errorf("Derived/Compatible and Base must not overlap")
	}
	if got := narrowTypeByPattern(derived, base); got != TypeUnknown {
		t.Errorf("narrowing Derived by Base: expected Unknown, got %v", got)
	}
	if got := narrowTypeByPattern(base, derived); got != TypeUnknown {
		t.Errorf("narrowing Base by Derived: expected Unknown, got %v", got)
	}
	got := narrowTypeByPattern(NewUnionType(derived, comp, incomp), base)
	if got != TypeUnknown {
		t.Errorf("expected Unknown, got %v", got)
	}
}

// Extra: ambiguous embeddings are no subtype of either, and an outer field
// resolving the ambiguity does not make them one.
func TestEmbeddingDistinguishingAmbiguousEmbeddingIsNotSubtype(t *testing.T) {
	const defs = `struct A:
    X: Int
struct B:
    X: Int
def take(x: A) -> Int:
    x.X
`
	embSemWantErr(t, defs+`struct Both:
    A
    B
take(Both(A: A(X: 1), B: B(X: 2)))
`, "type mismatch")
	embSemWantErr(t, defs+`struct Both:
    A
    B
    X: Int
take(Both(A: A(X: 1), B: B(X: 2), X: 3))
`, "type mismatch")
}

// Extra: a module-qualified generic embedded struct is no subtype of the
// instantiated embedded type.
func TestEmbeddingDistinguishingModuleQualifiedGeneric(t *testing.T) {
	wantIdentityErr(t, embSemGenModFiles, `
import gen

struct Child:
    gen::Box[Int]

c = Child(Box: gen::Box[Int](v: 7))
r = gen::Unbox(c)
`, "type mismatch")
}
