package eval

import "testing"

// Runtime type tests (`x is T`, `case T:`) against a type parameter must use
// the type argument of the current call (or of the receiver, for methods of
// generic structs).
func TestRuntimeTypeTestsOnTypeParameters(t *testing.T) {
	const pickDefs = `def pick[T](x: T | Error): String:
    match x:
        case Error:
            "error"
        case T:
            "T"
        case _:
            "fallthrough"
def pick2[T](x: T | Error): String:
    if x is T:
        "T"
    else:
        "not T"
`
	tests := []struct {
		name     string
		code     string
		expected string
	}{
		{
			name:     "explicit type arguments",
			code:     pickDefs + `print pick[String]("hi"), " ", pick2[String]("hi"), " ", pick[Int](3), " ", pick[Int](Error("e")), " ", pick2[Int](Error("e"))`,
			expected: "T T T error not T",
		},
		{
			name:     "inferred type arguments",
			code:     pickDefs + `print pick("hi"), " ", pick2("hi"), " ", pick(3), " ", pick2(3)`,
			expected: "T T T T",
		},
		{
			name: "generic function stored in a variable or called through a pipeline",
			code: pickDefs + `f = pick2
print f("hi"), " ", ("hi" |> pick2)`,
			expected: "T T",
		},
		{
			name: "type arguments referring to the enclosing type parameter",
			code: pickDefs + `def explicit[U](x: U | Error): String:
    pick2[U](x)
def inferred[U](x: U | Error): String:
    pick2(x)
print explicit[String]("a"), " ", inferred[String]("a"), " ", inferred("a"), " ", explicit[Int](Error("e"))`,
			expected: "T T T not T",
		},
		{
			name: "recursive generic call",
			code: `def rec[T](x: T, n: Int): String:
    n == 0 ? (x is T ? "T" : "not T") : rec(x, n - 1)
print rec("a", 3), " ", rec[String]("a", 2)`,
			expected: "T T",
		},
		{
			name: "inner type parameter shadows the outer one",
			code: `def outer[T](x: T): String:
    def inner[T](y: Int): String:
        y is T ? "T" : "not T"
    inner[String](1) + " " + inner[Int](1)
print outer[Int](1)`,
			expected: "not T T",
		},
		{
			name: "default type argument",
			code: `def dflt[T = Int](x: Int | String): String:
    x is T ? "T" : "not T"
print dflt(1), " ", dflt("s"), " ", dflt[String]("s")`,
			expected: "T not T T",
		},
		{
			name:     "union type argument",
			code:     pickDefs + `print pick2[Int | String](3), " ", pick2[Int | String]("s"), " ", pick[Int | String](Error("e"))`,
			expected: "T T error",
		},
		{
			name: "type parameter inside tuple, dict and generic struct patterns",
			code: `struct Box[T]:
    val: T
def tup[T](x: T, y: T): String:
    t = (x, y)
    t is (T, T) ? "tuple" : "no tuple"
def dict[T](d: {String: T}): String:
    d is {String: T} ? "dict" : "no dict"
def box[T](x: T): String:
    b = Box[T](x)
    b is Box[T] ? "box" : "no box"
print tup(1, 2), " ", tup[String]("a", "b"), " ", dict({"a": 1}), " ", box(1), " ", box[String]("s")`,
			expected: "tuple tuple dict box box",
		},
		{
			name: "lambda inside a generic function",
			code: `def lam[T](x: T | Error): String:
    g = |v|: v is T
    g(x) ? "T" : "not T"
print lam[String]("a"), " ", lam[Int](Error("e")), " ", lam("a")`,
			expected: "T not T T",
		},
		{
			name: "binding type arguments keeps arguments lazy",
			code: `def lazy[T](x: T, y: T): String:
    t = (x, y)
    (x is T ? "T" : "not T") + " " + (t is (T, T) ? "tuple" : "no tuple")
print lazy(1, 1 / 0), " ", lazy[Int](2, 1 / 0)`,
			expected: "T tuple T tuple",
		},
		{
			name: "regex literal in a pattern is not substituted",
			code: `def rx[T](x: String): String:
    match x:
        case Int | /T+/:
            "regex"
        case T:
            "T"
        case _:
            "other"
print rx[String]("TT"), " ", rx[String]("ab"), " ", rx[Int]("ab")`,
			expected: "regex T other",
		},
		{
			// A type argument that cannot be determined is Unknown, as in the
			// type checker, so a test against the type parameter matches any value.
			name: "undetermined type argument is Unknown",
			code: pickDefs + `def untyped(v):
    v
apply = |f, v|: f(v)
print pick2(untyped("hi")), " ", pick(untyped(3)), " ", apply(pick2, "hi"), " ", apply(pick2, 1), " ", apply(pick, Error("e"))`,
			expected: "T T T T error",
		},
		{
			name: "methods of a generic struct",
			code: `struct Box[T]:
    val: T
    def Has(x: T | Error): String:
        x is T ? "T" : "not T"
    def Kind(x: T | Error): String:
        match x:
            case Error:
                "error"
            case T:
                "T"
            case _:
                "fallthrough"
    def Same[U](x: U | Error): String:
        (x is U ? "U" : "not U") + "/" + (x is T ? "T" : "not T")
b = Box[Int](1)
c = Box("s")
print b.Has(5), " ", b.Kind(5), " ", b.Kind(Error("e")), " ", c.Has("x"), " ", c.Kind("x"), " ", b.Same("s"), " ", b.Same(5)`,
			expected: "T T error T T U/not T U/T",
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
