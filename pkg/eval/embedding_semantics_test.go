package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"les/pkg/lexer"
	"les/pkg/parser"
)

// Runtime side of the struct-embedding semantics (see also
// pkg/types/embedding_semantics_test.go, which uses the same programs).
//
// The tests are split in two groups:
//
//   - Common invariants: properties that must hold under every candidate
//     semantics of embedding.
//   - Distinguishing tests: properties whose expected result depends on the
//     chosen semantics. Here: composition semantics, i.e. embedding gives no
//     subtype relation between named structs, so is and match on an outer
//     instance return false for any embedded struct type.
//     Only the expected output of these tests differs between candidates.

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

type embSemCase struct {
	name string
	code string
	want string
}

func embSemRun(t *testing.T, tests []embSemCase) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := runLesCode(tt.code)
			if err != nil {
				t.Fatalf("run failed: %v", err)
			}
			if out != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, out)
			}
		})
	}
}

func embSemWantErr(t *testing.T, code, substr string) {
	t.Helper()
	_, err := runLesCode(code)
	if err == nil || !strings.Contains(err.Error(), substr) {
		t.Fatalf("expected error containing %q, got: %v", substr, err)
	}
}

// ---------------------------------------------------------------------------
// Common invariants
// ---------------------------------------------------------------------------

func TestEmbeddingCommonIs(t *testing.T) {
	embSemRun(t, []embSemCase{
		{"b is Base", embSemDefs + `print b is Base`, "true"},
		{"i is Base", embSemDefs + `print i is Base`, "false"},
		{"d.Base is Base", embSemDefs + `print d.Base is Base`, "true"},
		{"c.Base is Base", embSemDefs + `print c.Base is Base`, "true"},
		{"i.Base is Base", embSemDefs + `print i.Base is Base`, "true"},
		{"outer is its own type", embSemDefs + `print (d is Derived), (c is Compatible), (i is Incompatible)`, "truetruetrue"},
		{"Base is not an outer type", embSemDefs + `print (b is Derived), (b is Incompatible)`, "falsefalse"},
	})
}

func TestEmbeddingCommonMemberAccess(t *testing.T) {
	embSemRun(t, []embSemCase{
		{"d.X is Int", embSemDefs + `print d.X is Int`, "true"},
		{"c.X is Int", embSemDefs + `print c.X is Int`, "true"},
		{"i.X is String", embSemDefs + `print i.X is String`, "true"},
		{"values", embSemDefs + `print d.X, c.X, i.X, c.Base.X, i.Base.X`, "12s11"},
	})
}

// Issue #7: this used to fail with "unsupported binary operation s + 1".
func TestEmbeddingCommonIssue7Reproduction(t *testing.T) {
	embSemRun(t, []embSemCase{{"issue 7", `struct Inner:
    v: Int
struct Outer:
    Inner
    v: String
def get(x) -> Int:
    x is Inner ? x.v + 1 : 0
o = Outer(Inner: Inner(v: 1), v: "s")
print get(o), " ", get(o.Inner), " ", get(Inner(v: 5))`, "0 2 6"}})
}

func TestEmbeddingCommonNarrowing(t *testing.T) {
	embSemRun(t, []embSemCase{
		{"is then member access", embSemDefs + `def f(x) -> Int:
    x is Base ? x.X : -1
print f(b), " ", f(d.Base), " ", f(i.Base), " ", f(i)`, "1 1 1 -1"},
		{"match then member access", embSemDefs + `def g(x) -> Int:
    match x:
        case Base:
            x.X
        case _:
            -1
print g(b), " ", g(c.Base), " ", g(i)`, "1 1 -1"},
		{"outer narrowing sees outer member", embSemDefs + `def h(x) -> String:
    x is Incompatible ? x.X : "other"
print h(i), " ", h(b)`, "s other"},
	})
}

// Promoted members: ambiguity is reported, an outer declaration or an explicit
// path resolves it.
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
print o.X`, "ambiguous selector")
	embSemRun(t, []embSemCase{
		{"outer declaration", defs + `struct Both:
    A
    B
    X: Int
o = Both(A: A(X: 1), B: B(X: 2), X: 3)
print o.X, o.A.X, o.B.X`, "312"},
		{"is", defs + `struct Both:
    A
    B
o = Both(A: A(X: 1), B: B(X: 2))
print (o is A), (o is B), (o.A is A), (o.B is B)`, "falsefalsetruetrue"},
	})
}

func TestEmbeddingCommonMultiLevel(t *testing.T) {
	embSemRun(t, []embSemCase{
		{"members and fields", embSemDefs + `struct Deep:
    Derived
deep = Deep(Derived: d)
print deep.X, deep.Derived.X, deep.Derived.Base.X`, "111"},
		{"explicit fields are exact", embSemDefs + `struct Deep:
    Derived
deep = Deep(Derived: d)
print (deep.Derived is Derived), (deep.Derived.Base is Base), (deep is Deep)`, "truetruetrue"},
	})
}

// A promoted method keeps its own receiver: Base's Hello sees Base's Name, not
// an outer Name that shadows it. Shadowing with another type is accepted.
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
s = Same(Base: Base(k: 1))
o = Other(Base: Base(k: 1))
`
	embSemRun(t, []embSemCase{
		{"outer overrides, promoted receiver unchanged", defs + `print s.Name(), " ", s.Hello(), " ", s.Base.Name()`, "same hello base base"},
		{"incompatible method shadowing", defs + `print o.Name(), " ", o.Hello(), " ", (o is Base)`, "42 hello base false"},
		{"field of Base via promoted method", defs + `print s.k`, "1"},
	})
}

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
d = Derived(Base: Base(k: 1))
`
	embSemRun(t, []embSemCase{
		{"promoted method satisfies", defs + `print show(d), " ", (d is Named), (d.Base is Named)`, "base truetrue"},
		{"incompatible method does not", defs + `print Wrong(Base: Base(k: 1)) is Named`, "false"},
	})
}

func TestEmbeddingCommonGenericEmbedding(t *testing.T) {
	const defs = `struct Box[T]:
    v: T
struct IntBox:
    Box[Int]
ib = IntBox(Box: Box[Int](v: 1))
`
	embSemRun(t, []embSemCase{
		{"explicit embedded value", defs + `print (ib.Box is Box[Int]), (ib.Box is Box[String]), ib.v, ib.Box.v`, "truefalse11"},
		{"narrowing", defs + `def f(x) -> Int:
    x is Box[Int] ? x.v : -1
print f(ib.Box), " ", f(Box[String](v: "s"))`, "1 -1"},
	})
}

// Module-qualified embedded type.
func TestEmbeddingCommonModuleQualified(t *testing.T) {
	out := embSemRunWithModule(t, `
import base

struct Child:
    base::Base
    name: String

c = Child(Base: base::Base(id: 7), name: "n")
print (c.Base is base::Base), " ", c.id, " ", c.GetId(), " ", (c is Child)
`)
	if out != "true 7 7 true" {
		t.Fatalf("unexpected output %q", out)
	}
}

func embSemRunWithModules(t *testing.T, files map[string]string, mainCode string) string {
	t.Helper()
	dir := t.TempDir()
	for name, code := range files {
		if err := os.WriteFile(filepath.Join(dir, name+".les"), []byte(code), 0644); err != nil {
			t.Fatalf("failed to write %s.les: %v", name, err)
		}
	}
	ev := NewEvaluator()
	var out strings.Builder
	ev.SetOutput(&out)
	ev.SetFileModuleLoader(NewFileModuleLoader(dir))
	prog := parser.NewParser(lexer.NewLexer(mainCode)).ParseProgram()
	if _, err := ev.EvalProgram(prog); err != nil {
		t.Fatalf("eval failed: %v", err)
	}
	return strings.TrimSpace(out.String())
}

func embSemRunWithModule(t *testing.T, mainCode string) string {
	t.Helper()
	const baseCode = `module base

struct Base:
    id: Int = 100

    def GetId(self) -> Int:
        self.id
`
	return embSemRunWithModules(t, map[string]string{"base": baseCode}, mainCode)
}

// Type tests must not force lazy fields nor run side-effecting expressions.
func TestEmbeddingCommonIsDoesNotForceFields(t *testing.T) {
	embSemRun(t, []embSemCase{
		{"lazy field of the outer struct", `struct Base:
    X: Int
struct Lazy:
    Base
    Boom: Int
l = Lazy(Base: Base(X: 1), Boom: 1 / 0)
_r = l is Base
print (l is Lazy), (l.Base is Base), l.X`, "truetrue1"},
		{"side effect in a field", `struct Base:
    X: Int
struct Loud:
    Base
    Y: Int
def noisy() -> Int:
    print "SIDE"
    1
l = Loud(Base: Base(X: 1), Y: noisy())
_r = l is Base
print (l is Loud), (l.Base is Base), l.X`, "truetrue1"},
		{"lazy embedded value", `struct Base:
    X: Int
struct Wrap:
    Base
    Y: Int
w = Wrap(Base: Base(X: 1), Y: 1 / 0)
_r = w is Base
print (w.Base is Base)`, "true"},
	})
}

// Gradual typing: a value of statically unknown type passes the static check;
// whatever the runtime decides is then decided from the actual value.
func TestEmbeddingCommonUnknownPassesStaticCheck(t *testing.T) {
	embSemRun(t, []embSemCase{
		{"embedded value through Unknown", embSemDefs + `def pass(x):
    x
def take(x: Base) -> Int:
    x.X
print take(pass(d.Base)), take(pass(b))`, "11"},
	})
}

// Extra: multi-level embedding with an incompatible shadow at the top.
func TestEmbeddingCommonMultiLevelIncompatibleShadow(t *testing.T) {
	const defs = `struct Base:
    X: Int
struct Mid:
    Base
struct BadTop:
    Mid
    X: String
bt = BadTop(Mid: Mid(Base: Base(X: 2)), X: "s")
`
	embSemRun(t, []embSemCase{
		{"explicit selection", defs + `print bt.X, bt.Mid.X, (bt.Mid is Mid), (bt.Mid.Base is Base)`, "s2truetrue"},
		{"BadTop is neither Mid nor Base", defs + `print (bt is Mid), (bt is Base)`, "falsefalse"},
	})
}

// Extra: `is` and `match` do not force lazy shadowing fields either.
func TestEmbeddingCommonIncompatibleShadowDoesNotForce(t *testing.T) {
	const defs = `struct Base:
    X: Int
struct Incompatible:
    Base
    X: String
def boom() -> Int:
    print "FORCED"
    1
def boomS() -> String:
    print "FORCED"
    "s"
i = Incompatible(Base: Base(X: boom()), X: boomS())
`
	embSemRun(t, []embSemCase{
		{"is", defs + `print (i is Base), (i.Base is Base)`, "falsetrue"},
		{"match", defs + `r = match i:
    case Base:
        "base"
    case _:
        "other"
print r`, "other"},
		{"union pattern naming the outer type", defs + `print (i is Base | Incompatible), (i is Base | Int)`, "truefalse"},
	})
}

// Extra: an interface is never satisfied through the embedded instance when
// the outer struct's own members do not satisfy it.
func TestEmbeddingCommonInterfaceNotSatisfiedThroughShadowedEmbedded(t *testing.T) {
	embSemRun(t, []embSemCase{
		{"interface", `interface Named:
    Name(): String
struct Base:
    def Name() -> String:
        "base"
struct Wrong:
    Base
    def Name() -> Int:
        1
w = Wrong(Base: Base())
print (w is Named), (w.Base is Named)`, "falsetrue"},
		{"generic interface", `interface Getter[T]:
    Get(): T
struct Base:
    def Get() -> Int:
        1
struct Wrong:
    Base
    def Get() -> String:
        "s"
print (Wrong(Base: Base()) is Getter[Int]), (Base() is Getter[Int])`, "falsetrue"},
	})
}

// Extra: an interface is not satisfied when the required method is ambiguous
// across embedded structs or shadowed by a non-callable field.
func TestEmbeddingCommonInterfaceEdgeCases(t *testing.T) {
	embSemRun(t, []embSemCase{
		{"ambiguous promoted method", `interface Named:
    Name(): String
struct A:
    def Name() -> String:
        "a"
struct B:
    def Name() -> String:
        "b"
struct Both:
    A
    B
struct Resolved:
    A
    B
    def Name() -> String:
        "resolved"
print (Both(A: A(), B: B()) is Named), (Resolved(A: A(), B: B()) is Named)`, "falsetrue"},
		{"field shadows method", `interface Named:
    Name(): String
struct Base:
    def Name() -> String:
        "base"
struct FieldShadow:
    Base
    Name: String
print (FieldShadow(Base: Base(), Name: "s") is Named)`, "false"},
	})
}

// Extra: else-branch flow narrowing aligns with runtime nominal `is` matching.
func TestEmbeddingCommonFlowNarrowingElseBranch(t *testing.T) {
	embSemRun(t, []embSemCase{
		{"else branch narrows to Derived", `struct Base:
    X: Int
struct Derived:
    Base
    Extra: Int
def f(x: Base | Derived) -> Int:
    if x is Base:
        x.X
    else:
        x.Extra
b = Base(X: 1)
d = Derived(Base: b, Extra: 9)
print f(b), " ", f(d)`, "1 9"},
	})
}

var embSemGenModFiles = map[string]string{
	"gen": `module gen

struct Box[T]:
    v: T
`,
}

// Extra: module-qualified generic embedding promotes members and matches the
// embedded value explicitly.
func TestEmbeddingCommonModuleQualifiedGeneric(t *testing.T) {
	out := embSemRunWithModules(t, embSemGenModFiles, `
import gen

struct Child:
    gen::Box[Int]

c = Child(Box: gen::Box[Int](v: 7))
print (c.Box is gen::Box[Int]), " ", (c.Box is gen::Box[String]), " ", c.v, " ", c.Box.v
`)
	if out != "true false 7 7" {
		t.Fatalf("unexpected output %q", out)
	}
}

// ---------------------------------------------------------------------------
// Distinguishing tests
//
// Expectations below are those of composition semantics. These are the cases
// whose expected result differs from a candidate that keeps subtyping.
// ---------------------------------------------------------------------------

func TestEmbeddingDistinguishingIs(t *testing.T) {
	embSemRun(t, []embSemCase{
		{"d is Base", embSemDefs + `print d is Base`, "false"},
		{"c is Base", embSemDefs + `print c is Base`, "false"},
		{"deep is Base", embSemDefs + `struct Deep:
    Derived
print Deep(Derived: d) is Base`, "false"},
		{"deep is Derived", embSemDefs + `struct Deep:
    Derived
print Deep(Derived: d) is Derived`, "false"},
		{"generic narrowing", `struct Box[T]:
    v: T
struct IntBox:
    Box[Int]
def f(x) -> Int:
    x is Box[Int] ? x.v : -1
print f(IntBox(Box: Box[Int](v: 1)))`, "-1"},
		{"generic", `struct Box[T]:
    v: T
struct IntBox:
    Box[Int]
print IntBox(Box: Box[Int](v: 1)) is Box[Int]`, "false"},
		{"match", embSemDefs + `def g(x) -> String:
    match x:
        case Base:
            "base"
        case _:
            "other"
print g(b), " ", g(d), " ", g(c)`, "base other other"},
		{"unknown receiver", embSemDefs + `def f(x) -> Int:
    x is Base ? x.X : -1
print f(d), " ", f(c)`, "-1 -1"},
	})
}

// Passing d, c, i to a Base parameter is rejected by the type checker, which is
// tested in pkg/types (the evaluator does not re-check annotated parameters).

func TestEmbeddingDistinguishingModuleQualified(t *testing.T) {
	out := embSemRunWithModule(t, `
import base

struct Child:
    base::Base
    name: String

c = Child(Base: base::Base(id: 7), name: "n")
print c is base::Base
`)
	if out != "false" {
		t.Fatalf("expected false, got %q", out)
	}
}

// Extra: union patterns and unannotated members follow the same rule.
func TestEmbeddingDistinguishingUnionPatternAndUnknownMember(t *testing.T) {
	embSemRun(t, []embSemCase{
		{"union pattern", embSemDefs + `print (d is Base | Int), (c is Base | Int), (i is Base | Int)`, "falsefalsefalse"},
		{"mid-level instance of a multi-level chain", embSemDefs + `struct Deep:
    Derived
print (Deep(Derived: d).Derived is Base)`, "false"},
		{"unannotated override is not incompatible", `struct Base:
    def Get(v: Int) -> Int:
        v
struct Loose:
    Base
    def Get(v):
        v
print (Loose(Base: Base()) is Base)`, "false"},
	})
}

// Extra: a module-qualified generic embedded struct does not match the
// instantiated embedded type at run time.
func TestEmbeddingDistinguishingModuleQualifiedGeneric(t *testing.T) {
	out := embSemRunWithModules(t, embSemGenModFiles, `
import gen

struct Child:
    gen::Box[Int]

def f(x) -> Int:
    x is gen::Box[Int] ? x.v : -1

c = Child(Box: gen::Box[Int](v: 7))
print (c is gen::Box[Int]), " ", f(c)
`)
	if out != "false -1" {
		t.Fatalf("expected \"false -1\", got %q", out)
	}
}
